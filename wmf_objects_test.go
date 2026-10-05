package gowemf

import (
	"bytes"
	"errors"
	"testing"
)

func wmfRegionFixture() []byte {
	b := append(words(0, 6), longs(0)...)
	b = append(b, words(34, 1, 2, 0, 0, 10, 10)...)
	return append(b, words(2, 5, 6, 3, 9, 2)...)
}
func wmfPaletteFixture(count uint16) Record {
	b := append(words(0x300, int16(count)), make([]byte, int(count)*4)...)
	return testRecord(WMF, 0x00f7, 0, b)
}

func TestWMFRegionScans(t *testing.T) {
	body := wmfRegionFixture()
	v := mustDecode(t, testRecord(WMF, 0x06ff, 0, body)).(WMFRegion)
	if v.DeclaredBytes != 34 || v.Bounds != (Rect{0, 0, 10, 10}) || len(v.Scans) != 1 || v.Scans[0].Top != 5 || v.Scans[0].Bottom != 6 || v.Scans[0].Endpoints.At(1) != 9 {
		t.Fatal(v)
	}
	for _, off := range []int{8, 12, 22, 32} {
		bad := append([]byte(nil), body...)
		put16(bad, off, 3)
		if _, err := Decode(testRecord(WMF, 0x06ff, 0, bad), DecodeLimits{}); !errors.Is(err, ErrMalformed) {
			t.Fatalf("bad region field %d: %v", off, err)
		}
	}
	region := testRecord(WMF, 0x06ff, 0, body)
	brush := testRecord(WMF, 0x02fc, 0, append(append(words(0), longs(0)...), words(0)...))
	file := wmfDocument(2, region, brush, testRecord(WMF, 0x0228, 0, words(0, 1)), testRecord(WMF, 0x012c, 0, words(0)))
	if _, err := Stream(file, StreamOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	file = wmfDocument(2, region, brush, testRecord(WMF, 0x0228, 0, words(1, 0)))
	if _, err := Stream(file, StreamOptions{}, nil); !errors.Is(err, ErrMalformed) {
		t.Fatal("region/brush type confusion", err)
	}
}

func TestWMFPaletteState(t *testing.T) {
	selectPalette := func(id int16) Record { return testRecord(WMF, 0x0234, 0, words(id)) }
	update := testRecord(WMF, 0x0037, 0, append(words(1, 1), []byte{0, 10, 20, 30}...))
	file := wmfDocument(2, wmfPaletteFixture(2), selectPalette(0), testRecord(WMF, 0x001e, 0, nil), wmfPaletteFixture(1), selectPalette(1), testRecord(WMF, 0x0127, 0, words(-1)), update)
	updates := 0
	if _, err := Stream(file, StreamOptions{}, func(c Command) error {
		if c.Source.Type == 0x0037 {
			updates++
			p := c.Body.(Palette)
			if p.Handle != 0 || p.Start != 1 || p.Count != 1 {
				t.Fatal(p)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if updates != 1 {
		t.Fatal(updates)
	}
	for name, file := range map[string][]byte{
		"no selection":          wmfDocument(1, wmfPaletteFixture(2), update),
		"outside palette":       wmfDocument(1, wmfPaletteFixture(1), selectPalette(0), update),
		"stale restored handle": wmfDocument(1, wmfPaletteFixture(2), selectPalette(0), testRecord(WMF, 0x001e, 0, nil), testRecord(WMF, 0x01f0, 0, words(0)), wmfPaletteFixture(2), testRecord(WMF, 0x0127, 0, words(-1)), update),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Stream(file, StreamOptions{}, nil); !errors.Is(err, ErrMalformed) {
				t.Fatal(err)
			}
		})
	}
}

func TestWMFBitmap16Records(t *testing.T) {
	header := append(words(0, 8, 1, 2), 1, 1)
	bits := []byte{0x80, 0}
	pattern := append(append(append([]byte(nil), header...), make([]byte, 22)...), bits...)
	v := mustDecode(t, testRecord(WMF, 0x01f9, 0, pattern)).(BitmapPatternBrush)
	if v.Bitmap.Width != 8 || v.Bitmap.Height != 1 || v.Bitmap.WidthBytes != 2 || !bytes.Equal(v.Bitmap.Bits, bits) {
		t.Fatal(v)
	}
	payload := append(append(append(longs(0x00cc0020), words(0, 0, 1, 8, 2, 1)...), header...), bits...)
	r := testRecord(WMF, 0x0922, 0, payload)
	blt := mustDecode(t, r).(Bitmap16Transfer)
	if blt.Bitmap == nil || blt.DeviceSource || blt.Destination != (Point{1, 2}) || blt.SourceSize != (Point{8, 1}) || !bytes.Equal(blt.Bitmap.Bits, bits) {
		t.Fatal(blt)
	}
	put16(r.Raw, 28, 4) // inconsistent Bitmap16 WidthBytes
	if _, err := Decode(r, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("inconsistent device bitmap row", err)
	}
	if _, ok := mustDecode(t, testRecord(WMF, 0x0105, 0, nil)).(Empty); !ok {
		t.Fatal("SetRelAbs must be ignored")
	}
	if p := mustDecode(t, testRecord(WMF, 0x0325, 0, words(1, 2, 3))).(Poly); p.Points.Len() != 1 || p.Points.At(0) != (Point{2, 3}) {
		t.Fatal(p)
	}
}
