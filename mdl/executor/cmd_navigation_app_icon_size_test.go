// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// ako/mxcli#1756: measured on Mendix 11.13.0, a profile's app icon that is not
// a square PNG at least 512 pixels wide fails mx check — CE6859 "The selected
// image must be a square PNG image that is at least 512 pixels wide." at the
// navigation profile. Studio Pro's own Atlas_Core.Content.Mendix passes. So
// `app icon` is refused at exec when the stored image is not one, instead of
// writing a profile the next build rejects.

func encodedImage(t *testing.T, w, h int, asJPEG bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var err error
	if asJPEG {
		err = jpeg.Encode(&buf, img, nil)
	} else {
		err = png.Encode(&buf, img)
	}
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAppIconMustBeASquarePNGOfAtLeast512(t *testing.T) {
	images := []types.Image{
		{Name: "Square", Data: encodedImage(t, 512, 512, false)},
		{Name: "Large", Data: encodedImage(t, 1024, 1024, false)},
		{Name: "Wide", Data: encodedImage(t, 640, 200, false)},
		{Name: "Small", Data: encodedImage(t, 256, 256, false)},
		{Name: "Photo", Data: encodedImage(t, 512, 512, true)},
		{Name: "Unread"}, // no bytes to judge: left to mx check
	}
	for _, c := range []struct {
		image string
		want  string // "" = written
	}{
		{"Square", ""},
		{"Large", ""},
		{"Unread", ""},
		{"Wide", "640×200"},
		{"Small", "256×256"},
		{"Photo", "not a PNG"},
	} {
		wrote := false
		ctx := appTitleCtx(t, nil, &wrote)
		mb := ctx.Backend.(*mock.MockBackend)
		ics, _ := mb.ListImageCollectionsFunc()
		ics[0].Images = images
		err := execNavScript(t, ctx, "create or modify navigation Responsive app icon MyFirstModule.Images."+c.image+";")
		if c.want == "" {
			if err != nil || !wrote {
				t.Errorf("%s: err=%v wrote=%v, want it written", c.image, err, wrote)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), "CE6859") || wrote {
			t.Errorf("%s: err=%v wrote=%v, want a CE6859 refusal containing %q and no write", c.image, err, wrote, c.want)
		}
	}
}
