// SPDX-License-Identifier: Apache-2.0

// Package zipentry opens zip entries the way Mendix's own tooling reads them.
//
// archive/zip checks an entry's content against the central directory CRC and,
// when the entry has a data descriptor (general-purpose flag bit 3), also
// requires the descriptor's CRC to equal it. unzip, .NET's ZipArchive and
// Studio Pro check only the content, so a package whose descriptor is missing
// or disagrees opens everywhere except in mxcli, as "zip: checksum error"
// (mendixlabs/mxcli#1488). Open drops the descriptor check and keeps the one
// that detects corruption.
package zipentry

import (
	"archive/zip"
	"compress/flate"
	"hash"
	"hash/crc32"
	"io"
)

const flagDataDescriptor = 0x8

// Open returns a reader for f's content. Entries without a data descriptor, or
// with a compression method other than store/deflate, go through f.Open.
func Open(f *zip.File) (io.ReadCloser, error) {
	if f.Flags&flagDataDescriptor == 0 || (f.Method != zip.Store && f.Method != zip.Deflate) {
		return f.Open()
	}
	raw, err := f.OpenRaw()
	if err != nil {
		return nil, err
	}
	var rc io.ReadCloser
	if f.Method == zip.Deflate {
		rc = flate.NewReader(raw)
	} else {
		rc = io.NopCloser(raw)
	}
	return &checkReader{rc: rc, hash: crc32.NewIEEE(), f: f}, nil
}

// checkReader verifies size and CRC against the central directory at EOF.
type checkReader struct {
	rc    io.ReadCloser
	hash  hash.Hash32
	nread uint64
	f     *zip.File
	err   error
}

func (r *checkReader) Read(b []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	n, err := r.rc.Read(b)
	r.hash.Write(b[:n])
	r.nread += uint64(n)
	if r.nread > r.f.UncompressedSize64 {
		return 0, zip.ErrFormat
	}
	if err == nil {
		return n, nil
	}
	if err == io.EOF {
		if r.nread != r.f.UncompressedSize64 {
			err = io.ErrUnexpectedEOF
		} else if r.hash.Sum32() != r.f.CRC32 {
			err = zip.ErrChecksum
		}
	}
	r.err = err
	return n, err
}

func (r *checkReader) Close() error { return r.rc.Close() }
