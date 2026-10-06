package gowemf

import (
	"errors"
	"image"
	"image/color"
	"strconv"
	"testing"
)

func rawPlusImageObject(p PlusImage) []byte {
	b := longs(0, 1, p.Width, p.Height, p.Stride, int32(p.PixelFormat), 0)
	put32(b, 0, 0xdbc01002)
	return append(b, p.Data...)
}

func TestPlusIndexedPixels(t *testing.T) {
	palette := longs(1, 3, -65536, 0x4000ff00, -16776961)
	for _, tc := range []struct {
		format uint32
		width  int32
		data   []byte
		want   []color.NRGBA
	}{
		{PixelFormat1bppIndexed, 2, []byte{0x40, 0, 0, 0}, []color.NRGBA{{255, 0, 0, 255}, {0, 255, 0, 64}}},
		{PixelFormat4bppIndexed, 3, []byte{0x12, 0, 0, 0}, []color.NRGBA{{0, 255, 0, 64}, {0, 0, 255, 255}, {255, 0, 0, 255}}},
		{PixelFormat8bppIndexed, 2, []byte{2, 0, 0, 0}, []color.NRGBA{{0, 0, 255, 255}, {255, 0, 0, 255}}},
	} {
		p := PlusImage{Type: 1, Width: tc.width, Height: 1, Stride: 4, PixelFormat: tc.format, Data: append(append([]byte(nil), palette...), tc.data...)}
		v, err := DecodePlusObject(5, rawPlusImageObject(p), DecodeLimits{})
		if err != nil {
			t.Fatal(err)
		}
		im, err := v.(PlusImage).Image(ImageLimits{})
		if err != nil {
			t.Fatal(err)
		}
		for x, want := range tc.want {
			if got := im.(*image.NRGBA).NRGBAAt(x, 0); got != want {
				t.Fatalf("format %x pixel %d: %v != %v", tc.format, x, got, want)
			}
		}
		p.Data = p.Data[:len(p.Data)-4]
		if _, err := DecodePlusObject(5, rawPlusImageObject(p), DecodeLimits{}); !errors.Is(err, ErrMalformed) {
			t.Fatal("palette accepted as pixel storage", err)
		}
	}
	p := PlusImage{Type: 1, Width: 2, Height: 2, Stride: -4, PixelFormat: PixelFormat1bppIndexed, Data: append(palette, 0x40, 0, 0, 0, 0x80, 0, 0, 0)}
	im, err := p.Image(ImageLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if got := im.(*image.NRGBA).NRGBAAt(0, 0); got != (color.NRGBA{0, 255, 0, 64}) {
		t.Fatal("negative-stride orientation", got)
	}
	p = PlusImage{Type: 1, Width: 1, Height: 1, Stride: 4, PixelFormat: PixelFormat8bppIndexed, Data: append(longs(0, 1, -1), 1, 0, 0, 0)}
	if _, err := p.Image(ImageLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("undefined palette index", err)
	}
	put32(p.Data, 4, 0xffffffff)
	if _, err := p.Image(ImageLimits{}); !errors.Is(err, ErrLimit) {
		t.Fatal("unbounded palette", err)
	}
}

func TestPlus16BitPixels(t *testing.T) {
	for _, tc := range []struct {
		format uint32
		pixel  uint16
		want   color.NRGBA
	}{
		{PixelFormat16bppRGB555, 0xfc00, color.NRGBA{R: 255, A: 255}},
		{PixelFormat16bppRGB565, 0x7e0, color.NRGBA{G: 255, A: 255}},
		{PixelFormat16bppARGB1555, 0x801f, color.NRGBA{B: 255, A: 255}},
		{PixelFormat16bppARGB1555, 0x001f, color.NRGBA{B: 255, A: 0}},
	} {
		data := make([]byte, 4)
		put16(data, 0, tc.pixel)
		p := PlusImage{Type: 1, Width: 1, Height: 1, Stride: 4, PixelFormat: tc.format, Data: data}
		im, err := p.Image(ImageLimits{})
		if err != nil {
			t.Fatal(err)
		}
		if got := im.(*image.NRGBA).NRGBAAt(0, 0); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	p := PlusImage{Type: 1, Width: 1, Height: 1, Stride: 4, PixelFormat: PixelFormat16bppGrayScale, Data: []byte{0x34, 0x12, 0, 0}}
	im, err := p.Image(ImageLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if got := im.(*image.Gray16).Gray16At(0, 0).Y; got != 0x1234 {
		t.Fatal("grayscale precision/endianness", got)
	}
}

func TestPlusPaletteCannotSupplyPixelBytes(t *testing.T) {
	p := PlusImage{Type: 1, Width: 1, Height: 1, Stride: 4, PixelFormat: PixelFormat8bppIndexed, Data: longs(0, 2, -65536, -16711936)}
	if _, err := DecodePlusObject(5, rawPlusImageObject(p), DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("palette bytes substituted for absent pixels", err)
	}
}

func TestPlusExtendedPixels(t *testing.T) {
	for _, format := range []uint32{PixelFormat48bppRGB, PixelFormat64bppARGB, PixelFormat64bppPARGB} {
		p := PlusImage{Type: 1, Width: 1, Height: 1, Stride: 8, PixelFormat: format, Data: []byte{0x34, 0x12, 0x45, 0x23, 0x56, 0x34, 0x67, 0x45}}
		im, err := p.Image(ImageLimits{})
		if err != nil {
			t.Fatal(err)
		}
		if format == PixelFormat64bppPARGB {
			if got := im.(*image.RGBA64).RGBA64At(0, 0); got != (color.RGBA64{0x3456, 0x2345, 0x1234, 0x4567}) {
				t.Fatal(got)
			}
			put16(p.Data, 6, 0x1000)
			if _, err := p.Image(ImageLimits{}); !errors.Is(err, ErrMalformed) {
				t.Fatal("invalid 64-bit premultiplication", err)
			}
		} else {
			alpha := uint16(0x4567)
			if format == PixelFormat48bppRGB {
				alpha = 65535
			}
			if got := im.(*image.NRGBA64).NRGBA64At(0, 0); got != (color.NRGBA64{0x3456, 0x2345, 0x1234, alpha}) {
				t.Fatal(got)
			}
		}
		p.Data = p.Data[:7]
		if _, err := p.Image(ImageLimits{}); !errors.Is(err, ErrMalformed) {
			t.Fatal("short high-depth row", err)
		}
	}
}

func TestPlusExtendedNativeIndexLimit(t *testing.T) {
	if strconv.IntSize != 32 {
		t.Skip("native 32-bit image indexing")
	}
	p := PlusImage{Type: 1, Width: 65536, Height: 4096, Stride: 524288, PixelFormat: PixelFormat64bppARGB}
	if _, err := p.Image(ImageLimits{MaxPixels: 1 << 32}); !errors.Is(err, ErrLimit) {
		t.Fatal("8-byte output size overflow", err)
	}
}
