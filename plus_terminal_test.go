package gowemf

import (
	"errors"
	"testing"
)

func terminalGraphicsFixture() Record {
	b := append([]byte{4, 5, 1, 3}, words(-4, 7, 12)...)
	b = append(b, 2, 4)
	b = append(b, floats(1, 0, 0, 1, 10, 20)...)
	b = append(b, longs(0, 2, -65536, -16711936)...)
	return testRecord(EMFPlus, PlusSetTSGraphicsRecord, 0xfffd, b)
}
func TestTerminalGraphics(t *testing.T) {
	r := terminalGraphicsFixture()
	v := mustDecode(t, r).(PlusTSGraphics)
	if !v.HasPalette || v.VGAOnly || v.Origin != (Point{-4, 7}) || v.WorldToDevice.Dx != 10 || v.Palette.Len() != 2 || v.Palette.At(1) != 0xff00ff00 || v.TextContrast != 12 {
		t.Fatal(v)
	}
	put16(r.Raw, 20, 13)
	if _, err := Decode(r, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("invalid terminal text contrast", err)
	}
}
func TestTerminalClipDifferences(t *testing.T) {
	for _, small := range []bool{false, true} {
		var b []byte
		flags := uint16(2)
		if small {
			flags |= 0x8000
		}
		for _, v := range []int16{10, 20, 30, 40, -3, 5, 10, 8} {
			if small {
				b = append(b, byte(v)&127|128)
			} else {
				n := uint16(v) & 0x7fff
				b = append(b, byte(n>>8), byte(n))
			}
		}
		r := testRecord(EMFPlus, PlusSetTSClipRecord, flags, b)
		v := mustDecode(t, r).(PlusTSClip)
		if len(v.Rectangles) != 2 || v.Rectangles[0] != (Rect{10, 20, 30, 60}) || v.Rectangles[1] != (Rect{7, 25, 40, 33}) {
			t.Fatal(v)
		}
		if _, err := Decode(r, DecodeLimits{MaxElements: 1}); !errors.Is(err, ErrLimit) {
			t.Fatal(err)
		}
		r.Raw[12] ^= 128
		if _, err := Decode(r, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
			t.Fatal("inconsistent terminal coordinate marker", err)
		}
	}
}
