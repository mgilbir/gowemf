package gowemf

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"testing"
)

func gifFixture(t testing.TB, transparent, animated bool) []byte {
	t.Helper()
	palette := color.Palette{color.NRGBA{R: 255, A: 255}, color.NRGBA{G: 255, A: 255}, color.NRGBA{B: 255, A: 255}}
	if transparent {
		palette = append(palette, color.NRGBA{})
	}
	first := image.NewPaletted(image.Rect(1, 1, 3, 2), palette)
	first.SetColorIndex(1, 1, 0)
	first.SetColorIndex(2, 1, 1)
	if transparent {
		first.SetColorIndex(2, 1, 3)
	}
	g := &gif.GIF{Image: []*image.Paletted{first}, Delay: []int{0}, LoopCount: -1, BackgroundIndex: 2, Config: image.Config{ColorModel: palette, Width: 4, Height: 3}}
	if animated {
		next := image.NewPaletted(image.Rect(0, 0, 4, 3), palette)
		for i := range next.Pix {
			next.Pix[i] = 1
		}
		g.Image = append(g.Image, next)
		g.Delay = append(g.Delay, 0)
	}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, g); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestGIFLogicalCanvasAndTransparency(t *testing.T) {
	for _, transparent := range []bool{false, true} {
		data := gifFixture(t, transparent, true)
		p := PlusImage{Type: 1, BitmapType: 1, Width: -1, Height: -1, Data: data}
		im, err := p.Image(ImageLimits{})
		if err != nil {
			t.Fatal(err)
		}
		out := im.(*image.NRGBA)
		if out.Bounds() != image.Rect(0, 0, 4, 3) || out.NRGBAAt(1, 1) != (color.NRGBA{R: 255, A: 255}) {
			t.Fatal("first GIF frame/offset", out.Bounds(), out.NRGBAAt(1, 1))
		}
		if transparent {
			if out.NRGBAAt(0, 0).A != 0 || out.NRGBAAt(2, 1).A != 0 {
				t.Fatal("GIF transparency lost")
			}
		} else if out.NRGBAAt(0, 0) != (color.NRGBA{B: 255, A: 255}) || out.NRGBAAt(2, 1) != (color.NRGBA{G: 255, A: 255}) {
			t.Fatal("GIF background/local pixels")
		}
		if _, err := p.Image(ImageLimits{MaxPixels: 11}); !errors.Is(err, ErrLimit) {
			t.Fatal("logical canvas bypassed limit", err)
		}
		if _, err := p.Image(ImageLimits{MaxBytes: uint64(len(data) - 1)}); !errors.Is(err, ErrLimit) {
			t.Fatal("encoded GIF bytes", err)
		}
	}
}

func TestGIFPreflightAndFirstFrameOnly(t *testing.T) {
	data := gifFixture(t, false, false)
	for n := 0; n < 13; n++ {
		p := PlusImage{Type: 1, BitmapType: 1, Data: data[:n]}
		if _, err := p.Image(ImageLimits{}); err == nil {
			t.Fatal("accepted GIF header prefix", n)
		}
	}
	pos := bytes.Index(data, []byte{0x2c, 1, 0, 1, 0, 2, 0, 1, 0})
	if pos < 0 {
		t.Fatal("missing generated image descriptor")
	}
	for _, offset := range []int{1, 3, 5, 7} {
		bad := append([]byte(nil), data...)
		put16(bad, pos+offset, 65535)
		if _, err := (PlusImage{Type: 1, BitmapType: 1, Data: bad}).Image(ImageLimits{}); !errors.Is(err, ErrMalformed) {
			t.Fatal("out-of-canvas frame", offset, err)
		}
	}
	for n := 13; n < pos+10; n++ {
		if _, err := (PlusImage{Type: 1, BitmapType: 1, Data: data[:n]}).Image(ImageLimits{}); err == nil {
			t.Fatal("accepted GIF descriptor prefix", n)
		}
	}
	// No animation parser is invoked: malformed bytes after a complete first
	// image cannot trigger allocation for a second, attacker-sized frame.
	firstOnly := append(append([]byte(nil), data[:len(data)-1]...), 0x2c, 0, 0, 0, 0, 255, 255, 255, 255, 0)
	if _, err := (PlusImage{Type: 1, BitmapType: 1, Data: firstOnly}).Image(ImageLimits{}); err != nil {
		t.Fatal("decoded later animation data", err)
	}
	bad := append([]byte(nil), data...)
	put16(bad, 6, 65535)
	put16(bad, 8, 65535)
	if _, err := (PlusImage{Type: 1, BitmapType: 1, Data: bad}).Image(ImageLimits{}); !errors.Is(err, ErrLimit) {
		t.Fatal("GIF dimension bomb", err)
	}
}

func TestGIFLocalPalette(t *testing.T) {
	palette := color.Palette{color.Black, color.White}
	frame := image.NewPaletted(image.Rect(0, 0, 1, 1), palette)
	frame.Pix[0] = 1
	var b bytes.Buffer
	err := gif.EncodeAll(&b, &gif.GIF{Image: []*image.Paletted{frame}, Delay: []int{0}, LoopCount: -1, Config: image.Config{Width: 1, Height: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if b.Bytes()[10]&128 != 0 {
		t.Fatal("fixture unexpectedly has a global palette")
	}
	im, err := (PlusImage{Type: 1, BitmapType: 1, Data: b.Bytes()}).Image(ImageLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if im.(*image.NRGBA).NRGBAAt(0, 0) != (color.NRGBA{255, 255, 255, 255}) {
		t.Fatal("local palette ignored")
	}
}
