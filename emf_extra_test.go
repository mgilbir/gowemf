package gowemf

import (
	"bytes"
	"errors"
	"testing"
)

func TestEMFRegionPainting(t *testing.T) {
	region := append(longs(32, 1, 2, 32, -5, 0, 10, 20), longs(-5, 0, 1, 2, 2, 3, 4, 5)...)
	body := append(longs(-5, 0, 10, 20, int32(len(region)), 1), region...)
	r := testRecord(EMF, EMRFillRgn, 0, body)
	v := mustDecode(t, r).(EMFRegionPaint)
	if !v.HasBrush || v.Brush != 1 || v.Region.Count != 2 || v.Region.RectangleAt(1) != (Rect{2, 3, 4, 5}) {
		t.Fatal(v)
	}
	file := emfFixture(emfRecord(EMRCreateBrushIndirect, longs(1, 0, 0, 0)), r.Raw)
	if _, err := Stream(file, StreamOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	put32(r.Raw, 28, 0x80000006)
	if _, err := Stream(emfFixture(r.Raw), StreamOptions{}, nil); !errors.Is(err, ErrMalformed) {
		t.Fatal("stock pen used as region brush", err)
	}
}

func TestEMFGradientIndexes(t *testing.T) {
	body := longs(0, 0, 10, 10, 3, 1, 2)
	for i := int32(0); i < 3; i++ {
		v := append(longs(i, i+1), words(int16(i*100), 200, 300, 123)...)
		body = append(body, v...)
	}
	body = append(body, longs(0, 1, 2)...)
	r := testRecord(EMF, EMRGradientFill, 0, body)
	v := mustDecode(t, r).(Gradient)
	if v.Vertices.Len() != 3 || v.Indexes.Len() != 3 || v.Vertices.At(2).Point != (Point{2, 3}) || v.Vertices.At(2).IgnoredAlpha != 123 {
		t.Fatal(v)
	}
	put32(r.Raw, len(r.Raw)-4, 3)
	if _, err := Decode(r, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("out-of-range gradient index", err)
	}
	// Rectangle padding is unused tail data and can be truncated under 2.3.
	body = append(longs(0, 0, 10, 10, 2, 1, 0), body[28:60]...)
	body = append(body, longs(0, 1)...)
	if v := mustDecode(t, testRecord(EMF, EMRGradientFill, 0, body)).(Gradient); v.Mode != 0 || v.Indexes.Len() != 2 {
		t.Fatal(v)
	}
}

func TestEMFSmallUnicodeText(t *testing.T) {
	body := append(append(longs(10, -20, 2, 0x300, 2), floats(1, 1)...), 0x80, 0xff)
	v := mustDecode(t, testRecord(EMF, EMRSmallTextOut, 0, body)).(Text)
	if !v.SmallChars || !v.Unicode || v.HasRectangle || v.Reference != (Point{10, -20}) || !bytes.Equal(v.Bytes, []byte{0x80, 0xff}) {
		t.Fatal(v)
	}
}

func TestEMFPolyTextBindings(t *testing.T) {
	body := make([]byte, 124)
	put32(body, 16, 2)
	put32(body, 28, 2)
	copy(body[32:], longs(1, 2, 1, 120, 0))
	put32(body, 68, 124)
	copy(body[72:], longs(3, 4, 1, 122, 0))
	put32(body, 108, 128)
	copy(body[112:], words('A', 'B'))
	copy(body[116:], longs(-3, 5))
	r := testRecord(EMF, EMRPolyTextOutW, 0, body)
	v := mustDecode(t, r).(PolyText)
	if len(v.Strings) != 2 || v.Strings[0].Reference != (Point{1, 2}) || v.Strings[1].Reference != (Point{3, 4}) || !bytes.Equal(v.Strings[0].Bytes, []byte{'A', 0}) || !bytes.Equal(v.Strings[1].Bytes, []byte{'B', 0}) || v.Strings[0].Advances.SignedAt(0) != -3 || v.Strings[1].Advances.SignedAt(0) != 5 {
		t.Fatal(v)
	}
	put32(r.Raw, 52, 80)
	if _, err := Decode(r, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("text string overlapped later descriptor", err)
	}
}

func TestEMFMaskedBitmapSpans(t *testing.T) {
	r := testRecord(EMF, EMRMaskBlt, 0, make([]byte, 240))
	copy(r.Raw[24:], longs(1, 2, 3, 4))
	copy(r.Raw[44:], longs(5, 6))
	copy(r.Raw[52:], floats(1, 0, 0, 1, 0, 0))
	copy(r.Raw[84:], longs(201, 40, 244, 4))
	copy(r.Raw[100:], longs(7, 8))
	copy(r.Raw[112:], longs(128, 48, 176, 4))
	copy(r.Raw[128:], append(dibHeader(1, 1, 1, 0), []byte{0, 0, 0, 0, 255, 255, 255, 0}...))
	r.Raw[176] = 0x80
	copy(r.Raw[201:], dibHeader(1, 1, 24, 0))
	copy(r.Raw[244:], []byte{1, 2, 3, 0})
	v := mustDecode(t, r).(MaskedBitmapTransfer)
	if v.MaskOrigin != (Point{7, 8}) || v.Source != (Point{5, 6}) || len(v.SourceInfo) != 40 || len(v.MaskInfo) != 48 || !bytes.Equal(v.SourceBits, []byte{1, 2, 3, 0}) {
		t.Fatal(v)
	}
	put32(r.Raw, 112, 124)
	if _, err := Decode(r, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("mask span overlaps fixed fields", err)
	}
	r = testRecord(EMF, EMRPlgBlt, 0, make([]byte, 176))
	copy(r.Raw[24:], longs(0, 0, 10, 0, 0, 10))
	copy(r.Raw[56:], longs(1, 1))
	copy(r.Raw[64:], floats(1, 0, 0, 1, 0, 0))
	copy(r.Raw[96:], longs(140, 40, 180, 4))
	copy(r.Raw[140:], dibHeader(1, 1, 24, 0))
	v = mustDecode(t, r).(MaskedBitmapTransfer)
	if v.DestinationPoints.Len() != 3 || v.DestinationPoints.At(2) != (Point{0, 10}) || len(v.SourceBits) != 4 {
		t.Fatal(v)
	}
}

func TestEMFUnalignedBitmapBuffers(t *testing.T) {
	body := make([]byte, 117)
	copy(body[32:], longs(1, 1, 81, 40, 121, 4, 0, 0x00cc0020, 2, 2))
	copy(body[73:], dibHeader(1, -1, 24, 0))
	copy(body[113:], []byte{1, 2, 3, 0})
	v := mustDecode(t, testRecord(EMF, EMRStretchDIBits, 0, body)).(BitmapTransfer)
	if len(v.Info) != 40 || !bytes.Equal(v.Bits, []byte{1, 2, 3, 0}) {
		t.Fatal(v)
	}
	if _, err := ParseDIB(v.Info, v.Bits, 0, nil, ImageLimits{}); err != nil {
		t.Fatal(err)
	}
}
