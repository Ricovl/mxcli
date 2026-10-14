// SPDX-License-Identifier: Apache-2.0

package mpk

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// mendixlabs/mxcli#1488: "warning: skipping com.mendix.widget.web.PopupMenu.mpk:
// package.xml: zip: checksum error" for a package that unzip -t, .NET and
// Studio Pro read. The fixture keeps every entry's content and central
// directory CRC intact and zeroes only package.xml's data-descriptor CRC.
func TestParseMPKAll_DataDescriptorCRCDisagrees(t *testing.T) {
	entries := []struct{ name, body string }{
		{"package.xml", `<package><clientModule name="PopupMenu" version="1.0.0"><widgetFiles><widgetFile path="PopupMenu.xml"/></widgetFiles></clientModule></package>`},
		{"PopupMenu.xml", `<widget id="com.mendix.widget.web.popupmenu.PopupMenu" pluginWidget="true"><name>Pop-up menu</name><properties><propertyGroup caption="General"><property key="trigger" type="enumeration" defaultValue="onclick"><caption>Trigger</caption><description/><enumerationValues><enumerationValue key="onclick">Click</enumerationValue></enumerationValues></property></propertyGroup></properties></widget>`},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: e.name, Method: zip.Deflate})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, e.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()

	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	pkg := zr.File[0]
	dataOff, err := pkg.DataOffset()
	if err != nil {
		t.Fatal(err)
	}
	dd := int(dataOff) + int(pkg.CompressedSize64)
	if pkg.Flags&0x8 == 0 || binary.LittleEndian.Uint32(b[dd:]) != 0x08074b50 {
		t.Fatal("fixture: package.xml has no data descriptor")
	}
	binary.LittleEndian.PutUint32(b[dd+4:], 0)

	path := filepath.Join(t.TempDir(), "com.mendix.widget.web.PopupMenu.mpk")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	defs, err := ParseMPKAll(path)
	if err != nil {
		t.Fatalf("ParseMPKAll: %v", err)
	}
	if len(defs) != 1 || defs[0].ID != "com.mendix.widget.web.popupmenu.PopupMenu" || defs[0].Version != "1.0.0" {
		t.Fatalf("defs = %+v", defs)
	}
}
