// SPDX-License-Identifier: Apache-2.0

package zipentry

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

const payload = `<?xml version="1.0" encoding="utf-8"?><package/>`

// buildZip writes one entry with Go's writer, which always streams it with a
// data descriptor (general-purpose flag bit 3), the shape Studio Pro's
// .NET writer gives the platform widgets it ships.
func buildZip(t *testing.T, method uint16) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "package.xml", Method: method})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, payload); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// descriptorOffset locates the data descriptor that follows the entry's data.
func descriptorOffset(t *testing.T, b []byte) int {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	f := zr.File[0]
	if f.Flags&0x8 == 0 {
		t.Fatal("fixture has no data descriptor flag")
	}
	dataOff, err := f.DataOffset()
	if err != nil {
		t.Fatal(err)
	}
	off := int(dataOff) + int(f.CompressedSize64)
	if binary.LittleEndian.Uint32(b[off:]) != 0x08074b50 {
		t.Fatal("no data descriptor signature after the entry data")
	}
	return off
}

func readFirst(t *testing.T, b []byte, open func(*zip.File) (io.ReadCloser, error)) ([]byte, error) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	rc, err := open(zr.File[0])
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func stdlibOpen(f *zip.File) (io.ReadCloser, error) { return f.Open() }

// mendixlabs/mxcli#1488: "warning: skipping com.mendix.widget.web.PopupMenu.mpk:
// package.xml: zip: checksum error" for a package unzip -t, .NET's ZipFile and
// Studio Pro all read. Go's reader also requires the data descriptor's CRC to
// equal the central directory's; the others ignore the descriptor.
func TestOpen_DescriptorCRCDisagrees(t *testing.T) {
	b := buildZip(t, zip.Deflate)
	off := descriptorOffset(t, b)
	binary.LittleEndian.PutUint32(b[off+4:], 0) // descriptor CRC only; content and central directory intact

	// Control: the stdlib rejects it with the reported error.
	if _, err := readFirst(t, b, stdlibOpen); !errors.Is(err, zip.ErrChecksum) {
		t.Fatalf("control: stdlib err = %v, want zip.ErrChecksum", err)
	}
	got, err := readFirst(t, b, Open)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if string(got) != payload {
		t.Fatalf("content = %q, want %q", got, payload)
	}
}

// Flag bit 3 set but no descriptor written: the stdlib reads the next record's
// signature as the descriptor CRC.
func TestOpen_DescriptorFlagWithoutDescriptor(t *testing.T) {
	b := buildZip(t, zip.Deflate)
	off := descriptorOffset(t, b)
	const ddLen = 16
	b = append(b[:off:off], b[off+ddLen:]...)
	// Shift the central directory offset in the end record back by the removed bytes.
	eocd := bytes.LastIndex(b, []byte{0x50, 0x4b, 0x05, 0x06})
	cdOff := binary.LittleEndian.Uint32(b[eocd+16:])
	binary.LittleEndian.PutUint32(b[eocd+16:], cdOff-ddLen)

	if _, err := readFirst(t, b, stdlibOpen); err == nil {
		t.Fatal("control: stdlib accepted an entry with no descriptor")
	}
	got, err := readFirst(t, b, Open)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if string(got) != payload {
		t.Fatalf("content = %q, want %q", got, payload)
	}
}

// Real corruption must still fail: the content is checked against the central
// directory CRC, as unzip -t does.
func TestOpen_CorruptContentStillFails(t *testing.T) {
	b := buildZip(t, zip.Store)
	off := descriptorOffset(t, b)
	b[off-1] ^= 0xff // last stored content byte

	if _, err := readFirst(t, b, Open); !errors.Is(err, zip.ErrChecksum) {
		t.Fatalf("Open err = %v, want zip.ErrChecksum", err)
	}
}

func TestOpen_Valid(t *testing.T) {
	for _, m := range []uint16{zip.Store, zip.Deflate} {
		got, err := readFirst(t, buildZip(t, m), Open)
		if err != nil || string(got) != payload {
			t.Fatalf("method %d: content = %q, err = %v", m, got, err)
		}
	}
}
