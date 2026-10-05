package gowemf

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"testing"
)

func dibHeader(w, h int32, bpp uint16, compression uint32) []byte {
	b := make([]byte, 40)
	put32(b, 0, 40)
	put32(b, 4, uint32(w))
	put32(b, 8, uint32(h))
	put16(b, 12, 1)
	put16(b, 14, bpp)
	put32(b, 16, compression)
	return b
}
func TestDIBOrientationAndAlpha(t *testing.T) {
	// Bottom row blue/white; top row red/green. RGB32's high byte is unused.
	pixels := []byte{255, 0, 0, 0, 255, 255, 255, 0, 0, 0, 255, 0, 0, 255, 0, 0}
	d, err := ParseDIB(dibHeader(2, 2, 32, 0), pixels, 0, nil, ImageLimits{})
	if err != nil {
		t.Fatal(err)
	}
	im, err := d.Image()
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(im.At(0, 0)); got != (color.NRGBA{255, 0, 0, 255}) {
		t.Fatal(got)
	}
	if got := color.NRGBAModel.Convert(im.At(0, 1)); got != (color.NRGBA{0, 0, 255, 255}) {
		t.Fatal(got)
	}
	d, err = ParseDIB(dibHeader(2, -2, 32, 0), pixels, 0, nil, ImageLimits{})
	if err != nil {
		t.Fatal(err)
	}
	im, err = d.Image()
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(im.At(0, 0)); got != (color.NRGBA{0, 0, 255, 255}) {
		t.Fatal(got)
	}
}

func TestDIBPaletteAndMasks(t *testing.T) {
	info := append(dibHeader(2, 1, 1, 0), []byte{0, 0, 255, 0, 0, 255, 0, 0}...)
	d, err := ParsePackedDIB(append(info, 0x40, 0, 0, 0), 0, nil, ImageLimits{})
	if err != nil {
		t.Fatal(err)
	}
	im, err := d.Image()
	if err != nil {
		t.Fatal(err)
	}
	if im.At(0, 0).(color.NRGBA) != (color.NRGBA{255, 0, 0, 255}) || im.At(1, 0).(color.NRGBA) != (color.NRGBA{0, 255, 0, 255}) {
		t.Fatal(im)
	}
	info = append(dibHeader(1, 1, 16, 3), longs(0xf800, 0x7e0, 0x1f)...)
	d, err = ParseDIB(info, []byte{0xe0, 0x07, 0, 0}, 0, nil, ImageLimits{})
	if err != nil {
		t.Fatal(err)
	}
	im, err = d.Image()
	if err != nil {
		t.Fatal(err)
	}
	if got := im.At(0, 0).(color.NRGBA); got != (color.NRGBA{0, 255, 0, 255}) {
		t.Fatal(got)
	}
	put32(info, 44, 0xf800)
	if _, err := ParseDIB(info, []byte{0, 0, 0, 0}, 0, nil, ImageLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("overlapping masks", err)
	}
}

func TestDIBLimitsAndTruncation(t *testing.T) {
	header := dibHeader(3, 2, 24, 0)
	pixels := make([]byte, 24) // DWORD-aligned 12-byte rows.
	for n := 0; n < len(pixels); n++ {
		if _, err := ParseDIB(header, pixels[:n], 0, nil, ImageLimits{}); !errors.Is(err, ErrMalformed) {
			t.Fatal(n, err)
		}
	}
	if _, err := ParseDIB(header, pixels, 0, nil, ImageLimits{MaxPixels: 5}); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	if _, err := ParseDIB(header, pixels, 0, nil, ImageLimits{MaxBytes: 63}); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	if _, err := ParseDIB(dibHeader(0x7fffffff, -2147483648, 32, 0), nil, 0, nil, ImageLimits{}); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	if _, err := new(DIB).Image(); !errors.Is(err, ErrMalformed) {
		t.Fatal(err)
	}
}

func TestDIBPNG(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(1, 0, color.NRGBA{R: 123, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	header := dibHeader(2, 1, 0, 5)
	put32(header, 20, uint32(buf.Len()))
	d, err := ParseDIB(header, buf.Bytes(), 0, nil, ImageLimits{})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := d.Image()
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(decoded.At(1, 0)); got != (color.NRGBA{R: 123, A: 255}) {
		t.Fatal(got)
	}
	put32(header, 4, 1)
	if _, err := ParseDIB(header, buf.Bytes(), 0, nil, ImageLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("inconsistent PNG dimensions", err)
	}
}

func TestCompressedDIBNativeIndexLimit(t *testing.T) {
	if strconv.IntSize != 32 {
		t.Skip("exercises the 32-bit native image-index boundary")
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA64(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	b := encoded.Bytes()
	// Only metadata is parsed. Never invoke Image on this deliberately huge
	// configuration: a 16-bit/channel result needs 2^31 bytes on a 32-bit host.
	binary.BigEndian.PutUint32(b[16:], 65536)
	binary.BigEndian.PutUint32(b[20:], 4096)
	binary.BigEndian.PutUint32(b[29:], crc32.ChecksumIEEE(b[12:29]))
	header := dibHeader(65536, 4096, 0, 5)
	put32(header, 20, uint32(len(b)))
	if _, err := ParseDIB(header, b, 0, nil, ImageLimits{MaxPixels: 1 << 32, MaxBytes: 1 << 20}); !errors.Is(err, ErrLimit) {
		t.Fatal("16-bit PNG output can overflow native indexing", err)
	}
}

func FuzzDIB(f *testing.F) {
	profile, _ := profileDIB(generatedICC(f), true)
	f.Add(profile)
	f.Add(append(dibHeader(1, 1, 24, 0), []byte{1, 2, 3, 0}...))
	f.Add(rleFixture(4, []byte{0, 3, 0x12, 0x30, 0, 1}))
	f.Add(rleFixture(8, []byte{0, 3, 1, 2, 3, 0, 0, 1}))
	f.Fuzz(func(t *testing.T, b []byte) {
		d, err := ParsePackedDIB(b, 0, nil, ImageLimits{MaxBytes: 1 << 20, MaxPixels: 4096})
		if err == nil {
			_, _ = d.Image()
		}
	})
}

func rleFixture(bpp uint16, pixels []byte) []byte {
	compression := uint32(1)
	if bpp == 4 {
		compression = 2
	}
	b := dibHeader(4, 3, bpp, compression)
	put32(b, 20, uint32(len(pixels)))
	put32(b, 32, 4)
	b = append(b, []byte{0, 0, 0, 0, 0, 0, 255, 0, 0, 255, 0, 0, 255, 0, 0, 0}...)
	return append(b, pixels...)
}
func TestDIBRLE(t *testing.T) {
	for _, tc := range []struct {
		bpp    uint16
		pixels []byte
	}{
		{8, []byte{2, 1, 0, 0, 0, 2, 1, 0, 0, 3, 2, 3, 1, 0, 0, 0, 0, 1}},
		{4, []byte{2, 0x11, 0, 0, 0, 2, 1, 0, 0, 3, 0x23, 0x10, 0, 0, 0, 1}},
	} {
		d, err := ParsePackedDIB(rleFixture(tc.bpp, tc.pixels), 0, nil, ImageLimits{})
		if err != nil {
			t.Fatal(tc.bpp, err)
		}
		im, err := d.Image()
		if err != nil {
			t.Fatal(err)
		}
		if im.At(0, 2).(color.NRGBA) != (color.NRGBA{R: 255, A: 255}) || im.At(1, 1).(color.NRGBA) != (color.NRGBA{G: 255, A: 255}) || im.At(2, 1).(color.NRGBA) != (color.NRGBA{B: 255, A: 255}) || im.At(0, 0).(color.NRGBA) != (color.NRGBA{A: 255}) {
			t.Fatal("RLE decoded pixels differ", tc.bpp)
		}
	}
	for _, b := range [][]byte{{5, 1, 0, 1}, {0, 2, 5, 0, 0, 1}, {0, 2, 0, 3, 0, 1}, {1, 4, 0, 1}, {0, 3, 1}, {0, 0}, {0, 0, 0, 0, 0, 0, 0, 0, 0, 1}} {
		if _, err := ParsePackedDIB(rleFixture(8, b), 0, nil, ImageLimits{}); !errors.Is(err, ErrMalformed) {
			t.Fatalf("accepted malformed RLE %x: %v", b, err)
		}
	}
}
