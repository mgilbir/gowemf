package gowemf

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestPremultipliedImages(t *testing.T) {
	b := []byte{10, 20, 30, 40}
	d, err := ParseDIB(dibHeader(1, -1, 32, 0), b, 0, nil, ImageLimits{})
	if err != nil {
		t.Fatal(err)
	}
	im, err := d.AlphaImage()
	if err != nil {
		t.Fatal(err)
	}
	if got := im.RGBAAt(0, 0); got != (color.RGBA{30, 20, 10, 40}) {
		t.Fatal(got)
	}
	fields, err := ParseDIB(append(dibHeader(1, -1, 32, 3), longs(0xff0000, 0xff00, 0xff)...), b, 0, nil, ImageLimits{})
	if err != nil {
		t.Fatal(err)
	}
	fieldImage, err := fields.AlphaImage()
	if err != nil || fieldImage.RGBAAt(0, 0) != im.RGBAAt(0, 0) {
		t.Fatal("standard bitfield alpha", err)
	}
	p := PlusImage{Type: 1, Width: 1, Height: 1, Stride: 4, PixelFormat: 0xe200b, Data: b}
	plus, err := p.Image(ImageLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if got := plus.(*image.RGBA).RGBAAt(0, 0); got != im.RGBAAt(0, 0) {
		t.Fatal(got)
	}
	p.PixelFormat = 0x26200a
	plus, err = p.Image(ImageLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if got := plus.(*image.NRGBA).NRGBAAt(0, 0); got != (color.NRGBA{30, 20, 10, 40}) {
		t.Fatal(got)
	}
	b[3] = 0
	if _, err := d.AlphaImage(); !errors.Is(err, ErrMalformed) {
		t.Fatal("invalid premultiplication", err)
	}
}

func TestPlusCompressedImageLimits(t *testing.T) {
	var b bytes.Buffer
	im := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	p := PlusImage{Type: 1, BitmapType: 1, Width: -1, Height: -1, Data: b.Bytes()}
	decoded, err := p.Image(ImageLimits{})
	if err != nil || decoded.Bounds() != im.Bounds() {
		t.Fatal(decoded, err)
	}
	if _, err := p.Image(ImageLimits{MaxPixels: 3}); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	if _, err := p.Image(ImageLimits{MaxBytes: 1}); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	p = PlusImage{Type: 1, Width: 0x7fffffff, Height: 0x7fffffff, Stride: 4, PixelFormat: 0x26200a}
	if _, err := p.Image(ImageLimits{}); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
}

func TestPhysicalDimensions(t *testing.T) {
	h := Header{Format: EMF, EMF: &EMFHeader{Frame: Rect{-100, 200, 2440, 1470}}}
	w, height, err := h.SizeInPoints()
	if err != nil || w != 72 || height != 36 {
		t.Fatal(w, height, err)
	}
	h = Header{Format: WMF, Placeable: &PlaceableHeader{Bounds: Rect{0, 0, 1440, -720}, UnitsPerInch: 1440}}
	w, height, err = h.SizeInPoints()
	if err != nil || w != 72 || height != 36 {
		t.Fatal(w, height, err)
	}
	h.Placeable = nil
	if _, _, err := h.SizeInPoints(); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
}
