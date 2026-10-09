package gowemf

import (
	"errors"
	"image"
	"image/color"
	"math"
	"testing"
)

func TestLookupEffectPlayback(t *testing.T) {
	src := image.NewNRGBA(image.Rect(5, 7, 7, 8))
	src.SetNRGBA(5, 7, color.NRGBA{10, 20, 30, 40})
	src.SetNRGBA(6, 7, color.NRGBA{200, 100, 50, 0})
	var red, green, blue, alpha [256]byte
	for i := 0; i < 256; i++ {
		red[i] = byte(255 - i)
		green[i] = byte(i)
		blue[i] = byte(i / 2)
		alpha[i] = byte(255 - i)
	}
	e := PlusEffect{Kind: EffectColorLookupTable, Parameters: ColorLookupTableEffect{blue[:], green[:], red[:], alpha[:]}}
	out, err := ApplyImageEffect(src, e, ImageLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Bounds() != image.Rect(0, 0, 2, 1) || out.NRGBAAt(0, 0) != (color.NRGBA{245, 20, 15, 215}) || out.NRGBAAt(1, 0) != (color.NRGBA{55, 100, 25, 255}) {
		t.Fatal(out)
	}
	if src.NRGBAAt(5, 7) != (color.NRGBA{10, 20, 30, 40}) {
		t.Fatal("source modified")
	}
	if _, err := ApplyImageEffect(src, e, ImageLimits{MaxDecodedBytes: 7}); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
}
func TestColorMatrixEffectPlayback(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	src.SetNRGBA(0, 0, color.NRGBA{64, 128, 32, 128})
	m := ColorMatrixEffect{Rows: [5][5]float64{{0, 0, 1, 0, 0}, {1, 0, 0, 0, 0}, {0, -1, 0, 0, 0}, {0, 0, 0, 1, 0}, {0, 1, 0, 0, 1}}}
	out, err := ApplyImageEffect(src, PlusEffect{Kind: EffectColorMatrix, Parameters: m}, ImageLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if got := out.NRGBAAt(0, 0); got != (color.NRGBA{128, 223, 64, 128}) {
		t.Fatal("matrix direction/translation", got)
	}
	m.Rows[4][0] = 100
	m.Rows[4][1] = -100
	out, err = ApplyImageEffect(src, PlusEffect{Kind: EffectColorMatrix, Parameters: m}, ImageLimits{})
	if err != nil || out.NRGBAAt(0, 0).R != 255 || out.NRGBAAt(0, 0).G != 0 {
		t.Fatal(out, err)
	}
	m.Rows[0][0] = math.NaN()
	if _, err := ApplyImageEffect(src, PlusEffect{Kind: EffectColorMatrix, Parameters: m}, ImageLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal(err)
	}
	if _, err := ApplyImageEffect(src, PlusEffect{Kind: EffectBlur, Parameters: BlurEffect{1, false}}, ImageLimits{}); !errors.Is(err, ErrUnsupported) {
		t.Fatal("unimplemented filter silently accepted", err)
	}
}
