package gowemf

import (
	"errors"
	"image/color"
	"testing"
)

func TestPlusTextureBrush(t *testing.T) {
	imageData := append(longs(0, 1, 1, 1, 4, 0x26200a, 0), []byte{10, 20, 30, 40}...)
	put32(imageData, 0, 0xdbc01002)
	b := append(append(longs(0, 2, 2, 0), floats(1, 0, 0, 1, 5, 6)...), imageData...)
	put32(b, 0, 0xdbc01002)
	v, err := DecodePlusObject(1, b, DecodeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	texture := v.(PlusBrush).Texture
	if texture == nil || texture.Transform.Dx != 5 || texture.Image == nil {
		t.Fatal(texture)
	}
	im, err := texture.Image.Image(ImageLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(im.At(0, 0)); got != (color.NRGBA{30, 20, 10, 40}) {
		t.Fatal(got)
	}
	if _, err := DecodePlusObject(1, b[:len(b)-1], DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("truncated texture pixels", err)
	}
	empty := longs(0, 2, 0, 0)
	put32(empty, 0, 0xdbc01002)
	v, err = DecodePlusObject(1, empty, DecodeLimits{})
	if err != nil || v.(PlusBrush).Texture.Image != nil {
		t.Fatal(v, err)
	}
}

func TestPlusPathGradient(t *testing.T) {
	b := append(longs(0, 3, 0x40, 0, -65536), floats(2, 3)...)
	put32(b, 0, 0xdbc01002)
	b = append(b, longs(1, -16711936, 3)...)
	b = append(b, floats(0, 0, 10, 0, 0, 10)...)
	b = append(b, longs(2)...)
	b = append(b, floats(.25, .75)...)
	v, err := DecodePlusObject(1, b, DecodeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	g := v.(PlusBrush).PathGradient
	if g.Center != (Point{2, 3}) || g.BoundaryPoints.Len() != 3 || g.FocusScale != (Point{.25, .75}) || g.SurroundingColors.At(0) != 0xff00ff00 {
		t.Fatal(g)
	}
	put32(b, len(b)-8, 0)
	if _, err := DecodePlusObject(1, b, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("invalid focus scale", err)
	}
	path := append(longs(0, 2, 0x4000), words(0, 0, 1, 1)...)
	put32(path, 0, 0xdbc01002)
	path = append(path, 0, 1, 0, 0)
	b = append(longs(0, 3, 1, 0, -65536), floats(0, 0)...)
	put32(b, 0, 0xdbc01002)
	b = append(b, longs(1, -1, int32(len(path)))...)
	b = append(b, path...)
	v, err = DecodePlusObject(1, b, DecodeLimits{})
	if err != nil || v.(PlusBrush).PathGradient.BoundaryPath == nil || v.(PlusBrush).PathGradient.BoundaryPath.Points.Len() != 2 {
		t.Fatal(v, err)
	}
}
