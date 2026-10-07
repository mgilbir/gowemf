package gowemf

import (
	"bytes"
	"errors"
	"math"
	"testing"
)

func testRecord(f Format, typ uint32, flags uint16, body []byte) Record {
	var raw []byte
	switch f {
	case WMF:
		raw = make([]byte, (6+len(body)+1)&^1)
		put32(raw, 0, uint32(len(raw)/2))
		put16(raw, 4, uint16(typ))
		copy(raw[6:], body)
	case EMF:
		raw = emfRecord(typ, body)
	case EMFPlus:
		raw = plusRecord(uint16(typ), body)
		put16(raw, 2, flags)
	}
	return Record{Format: f, Type: typ, Flags: flags, Raw: raw, ParentOffset: -1}
}
func words(v ...int16) []byte {
	b := make([]byte, len(v)*2)
	for i, x := range v {
		put16(b, i*2, uint16(x))
	}
	return b
}
func longs(v ...int32) []byte {
	b := make([]byte, len(v)*4)
	for i, x := range v {
		put32(b, i*4, uint32(x))
	}
	return b
}
func floats(v ...float32) []byte {
	b := make([]byte, len(v)*4)
	for i, x := range v {
		put32(b, i*4, math.Float32bits(x))
	}
	return b
}
func mustDecode(t *testing.T, r Record) any {
	t.Helper()
	v, err := Decode(r, DecodeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestDecodeCoordinates(t *testing.T) {
	r := testRecord(WMF, 0x9914, 0, words(-20, 30)) // High function byte is not the opcode.
	if got := mustDecode(t, r).(PointRecord).Point; got != (Point{30, -20}) {
		t.Fatal(got)
	}
	r = testRecord(WMF, 0x041b, 0, words(40, 30, -20, -10))
	if got := mustDecode(t, r).(RectRecord).Rect; got != (Rect{-10, -20, 30, 40}) {
		t.Fatal(got)
	}
	r = testRecord(WMF, 0x0817, 0, words(8, 7, 6, 5, 40, 30, -20, -10))
	a := mustDecode(t, r).(Arc)
	if a.Start != (Point{5, 6}) || a.End != (Point{7, 8}) || a.Rect.Left != -10 {
		t.Fatal(a)
	}
	r = testRecord(EMF, 44, 0, longs(-10, -20, 30, 40, 5, 6))
	if got := mustDecode(t, r).(RoundRect); got.Corner != (Point{5, 6}) || got.Rect.Bottom != 40 {
		t.Fatal(got)
	}
}

func TestDecodePolygons(t *testing.T) {
	r := testRecord(WMF, 0x0538, 0, words(2, 2, 3, -1, 2, 3, 4, 5, 6, 7, 8, 9, 10))
	p := mustDecode(t, r).(Poly)
	if p.Counts.Len() != 2 || p.Counts.At(1) != 3 || p.Points.Len() != 5 || p.Points.At(0) != (Point{-1, 2}) || p.Points.At(4) != (Point{9, 10}) {
		t.Fatal(p)
	}
	body := append(longs(0, 0, 20, 20, 2, 5, 2, 3), words(-1, 2, 3, 4, 5, 6, 7, 8, 9, 10)...)
	r = testRecord(EMF, 91, 0, body)
	p = mustDecode(t, r).(Poly)
	if p.Points.Len() != 5 || p.Points.At(4) != (Point{9, 10}) {
		t.Fatal(p)
	}
	put32(r.Raw, 28, 6)
	if _, err := Decode(r, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("accepted inconsistent polygon counts", err)
	}
	put32(r.Raw, 28, 5)
	if _, err := Decode(r, DecodeLimits{MaxElements: 4}); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
}

func TestDecodeTextSpans(t *testing.T) {
	// One UTF-16 glyph, two signed advances; non-adjacent offsets exercise
	// record-relative offsets and ETO_PDY's doubled count.
	body := make([]byte, 84)
	put32(body, 16, 2)
	put32(body, 28, 10)
	put32(body, 32, 20)
	put32(body, 36, 1)
	put32(body, 40, 80)
	put32(body, 44, 0x2010)
	put32(body, 64, 84)
	put16(body, 72, 0x1234)
	put32(body, 76, 0xfffffffd)
	put32(body, 80, 7)
	r := testRecord(EMF, 84, 0, body)
	txt := mustDecode(t, r).(Text)
	if !txt.Unicode || txt.Reference != (Point{10, 20}) || !bytes.Equal(txt.Bytes, []byte{0x34, 0x12}) || txt.Advances.Len() != 2 || txt.Advances.SignedAt(0) != -3 || txt.Advances.At(1) != 7 {
		t.Fatal(txt)
	}
	put32(r.Raw, 48, 0xfffffffe)
	if _, err := Decode(r, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("accepted wrapped string offset", err)
	}
	// WMF odd strings are WORD padded before x/y or optional advances.
	r = testRecord(WMF, 0x0521, 0, append(append(words(3), []byte{'a', 'b', 'c', 0xff}...), words(-5, 9)...))
	txt = mustDecode(t, r).(Text)
	if string(txt.Bytes) != "abc" || txt.Reference != (Point{9, -5}) {
		t.Fatal(txt)
	}
	r = testRecord(WMF, 0x0a32, 0, append(append(words(-5, 9, 3, 0), []byte{'a', 'b', 'c', 0}...), words(1, -2, 3)...))
	txt = mustDecode(t, r).(Text)
	if txt.Advances.SignedAt(1) != -2 {
		t.Fatal(txt)
	}
}

func TestDecodePlusCoordinates(t *testing.T) {
	r := testRecord(EMFPlus, 0x400a, 0xc000, append(longs(-65536, 1), words(-5, -6, 20, 30)...))
	v := mustDecode(t, r).(PlusRects)
	if !v.Solid || v.BrushID != 0xffff0000 || v.Rectangles.At(0) != (Box{-5, -6, 20, 30}) {
		t.Fatal(v)
	}
	// 7- and 15-bit signed relative values, crossing byte boundaries. P
	// overrides C; first point is relative to (0,0), all others to predecessor.
	r = testRecord(EMFPlus, 0x400d, 0x4801, append(longs(3), []byte{0x3f, 0x40, 0x80, 0x40, 0xff, 0xbf, 0x7f, 1}...))
	p := mustDecode(t, r).(PlusPoly).Points
	if p.At(0) != (Point{63, -64}) || p.At(1) != (Point{127, -129}) || p.At(2) != (Point{126, -128}) {
		t.Fatal(p.At(0), p.At(1), p.At(2))
	}
	r = testRecord(EMFPlus, 0x400d, 1, append(longs(2), floats(1, 2, 3, float32(math.Inf(1)))...))
	if _, err := Decode(r, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("accepted non-finite coordinate", err)
	}
}

func TestDecodeTruncatedBodies(t *testing.T) {
	cases := []Record{
		testRecord(WMF, 0x041b, 0, words(1, 2, 3, 4)),
		testRecord(WMF, 0x02fa, 0, append(words(0, 1, 0), longs(255)...)),
		testRecord(EMF, 38, 0, longs(1, 0, 1, 0, 255)),
		testRecord(EMF, 35, 0, floats(1, 0, 0, 1, 20, 30)),
		testRecord(EMFPlus, 0x4012, 1, append(floats(10, 20), floats(1, 2, 3, 4)...)),
	}
	for _, r := range cases {
		mustDecode(t, r)
		h := 8
		step := 4
		if r.Format == WMF {
			h = 6
			step = 2
		}
		if r.Format == EMFPlus {
			h = 12
		}
		for n := h; n < len(r.Raw); n += step {
			raw := append([]byte(nil), r.Raw[:n]...)
			broken := r
			broken.Raw = raw
			if r.Format == WMF {
				put32(raw, 0, uint32(n/2))
			} else {
				put32(raw, 4, uint32(n))
				if r.Format == EMFPlus {
					put32(raw, 8, uint32(n-12))
				}
			}
			if _, err := Decode(broken, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
				t.Fatalf("format %d type %x size %d: %v", r.Format, r.Type, n, err)
			}
		}
	}
}

func TestMatrixComposition(t *testing.T) {
	scale := Matrix{M11: 2, M22: 3}
	translate := Matrix{M11: 1, M22: 1, Dx: 7, Dy: 11}
	if got := scale.Then(translate).Apply(Point{4, 5}); got != (Point{15, 26}) {
		t.Fatal(got)
	}
	if got := translate.Then(scale).Apply(Point{4, 5}); got != (Point{22, 48}) {
		t.Fatal(got)
	}
}

func TestRasterSpansAndIgnoredTransform(t *testing.T) {
	body := make([]byte, 144)
	copy(body, longs(0, 0, 30, 40, 10, 20, 30, 40))
	put32(body, 32, 0x01800000)
	copy(body[36:], longs(1, 2))
	copy(body[44:], floats(1, 2, 3, 4, 5, 6))
	copy(body[76:], longs(108, 40, 148, 4, 1, 1))
	copy(body[100:], dibHeader(1, -1, 32, 0))
	copy(body[140:], []byte{10, 20, 30, 40})
	r := testRecord(EMF, 114, 0, body)
	v := mustDecode(t, r).(RasterTransfer)
	if v.Destination != (Point{10, 20}) || v.Source != (Point{1, 2}) || v.SourceTransform != (Matrix{1, 2, 3, 4, 5, 6}) || v.SourceSize != (Point{1, 1}) || len(v.Info) != 40 || len(v.Bits) != 4 {
		t.Fatal(v)
	}
	put32(r.Raw, 84, 8)
	if _, err := Decode(r, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("bitmap info span overlaps record header", err)
	}
	mat := append(floats(float32(math.NaN()), 0, 0, 1, 0, 0), longs(1)...)
	if got := mustDecode(t, testRecord(EMF, 36, 0, mat)).(Transform); got.Matrix != Identity() {
		t.Fatal(got)
	}
	put32(mat, 24, 2)
	if _, err := Decode(testRecord(EMF, 36, 0, mat), DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("used non-finite transform", err)
	}
}

func TestAdvancedTextIgnoresScale(t *testing.T) {
	b := make([]byte, 68)
	put32(b, 16, 2)
	put32(b, 20, math.Float32bits(float32(math.NaN())))
	put32(b, 24, math.Float32bits(float32(math.Inf(1))))
	text := mustDecode(t, testRecord(EMF, 84, 0, b)).(Text)
	if text.ScaleX != 1 || text.ScaleY != 1 {
		t.Fatal("ignored scale must be normalized", text)
	}
	put32(b, 16, 1)
	if _, err := Decode(testRecord(EMF, 84, 0, b), DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("GM_COMPATIBLE non-finite scales", err)
	}
}

func TestWMFStretchDIBCanonicalOpcode(t *testing.T) {
	pixels := append(dibHeader(1, 1, 24, 0), []byte{1, 2, 3, 0}...)
	body := append(append(longs(0x00cc0020), words(0, 1, 1, 0, 0, 20, 30, -4, -5)...), pixels...)
	v := mustDecode(t, testRecord(WMF, 0x0f43, 0, body)).(PackedDIBTransfer)
	if v.Destination != (Point{-5, -4}) || v.DestinationSize != (Point{30, 20}) || v.SourceSize != (Point{1, 1}) || v.Usage != 0 || !bytes.Equal(v.DIB, pixels) {
		t.Fatal(v)
	}
}

func TestWMFDIBTransferVariants(t *testing.T) {
	b := append(longs(0x00f00021), words(2, 1, 777, 4, 3, 6, 5)...)
	v := mustDecode(t, testRecord(WMF, 0x0940, 0, b)).(PackedDIBTransfer)
	if !v.DeviceSource || v.Source != (Point{1, 2}) || v.SourceSize != (Point{3, 4}) || v.Destination != (Point{5, 6}) || len(v.DIB) != 0 {
		t.Fatal(v)
	}
	b = append(longs(0x00f00021), words(4, 3, 2, 1, 777, 8, 7, 6, 5)...)
	v = mustDecode(t, testRecord(WMF, 0x0b41, 0, b)).(PackedDIBTransfer)
	if !v.DeviceSource || v.DestinationSize != (Point{7, 8}) || v.Destination != (Point{5, 6}) || v.SourceSize != (Point{3, 4}) {
		t.Fatal(v)
	}
	pixels := append(dibHeader(1, 1, 24, 0), []byte{1, 2, 3, 0}...)
	b = append(append(longs(0x00cc0020), words(1, 1, 0, 0, 8, 7, 6, 5)...), pixels...)
	v = mustDecode(t, testRecord(WMF, 0x0b41, 0, b)).(PackedDIBTransfer)
	if v.DeviceSource || v.Usage != 0 || v.Destination != (Point{5, 6}) || !bytes.Equal(v.DIB, pixels) {
		t.Fatal(v)
	}
	if _, err := Decode(testRecord(WMF, 0x0141, 0, b), DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("ignored significant high function byte", err)
	}
	b = append(words(0, 1, 0, -1, -2, 1, 1, -3, -4), pixels...)
	v = mustDecode(t, testRecord(WMF, 0x0d33, 0, b)).(PackedDIBTransfer)
	if v.Source != (Point{65534, 65535}) || v.Destination != (Point{65532, 65533}) || v.Scans != 1 {
		t.Fatal(v)
	}
}

func TestWMFEnhancedEnvelope(t *testing.T) {
	emf := emfFixture()
	var sum uint16
	for i := 0; i < len(emf); i += 2 {
		sum ^= u16(emf[i:])
	}
	body := append(words(15, int16(34+len(emf))), longs(0x43464d57, 1, 0x10000)...)
	body = append(body, words(int16(^sum))...)
	body = append(body, longs(0, 1, int32(len(emf)), 0, int32(len(emf)))...)
	body = append(body, emf...)
	r := testRecord(WMF, 0x0626, 0, body)
	v := mustDecode(t, r).(WMFEnhancedMetafile)
	if v.Records != 1 || v.Checksum != ^sum || !bytes.Equal(v.Data, emf) {
		t.Fatal(v)
	}
	put32(r.Raw, 32, 8193)
	if _, err := Decode(r, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("oversized enhanced fragment", err)
	}
}

func TestPlusObjectsAndContinuation(t *testing.T) {
	brush := longs(int32(-608169982), 0, -65536) // 0xdbc01002 graphics version
	put32(brush, 0, 0xdbc01002)
	v, err := DecodePlusObject(1, brush, DecodeLimits{})
	if err != nil || v.(PlusBrush).Color != 0xffff0000 {
		t.Fatal(v, err)
	}
	a := PlusAssembler{}
	if data, err := a.Add(PlusObjectFragment{ID: 2, Type: 1, Continued: true, TotalSize: 12, Data: brush[:8]}); data != nil || err != nil {
		t.Fatal(data, err)
	}
	if err := a.Finish(); !errors.Is(err, ErrMalformed) {
		t.Fatal(err)
	}
	data, err := a.Add(PlusObjectFragment{ID: 2, Type: 1, Data: brush[8:]})
	if err != nil || !bytes.Equal(data, brush) {
		t.Fatal(data, err)
	}
	if err := a.Finish(); err != nil {
		t.Fatal(err)
	}
	for _, f := range []PlusObjectFragment{
		{ID: 2, Type: 1, Continued: true, TotalSize: 4, Data: brush},
		{ID: 2, Type: 1, Continued: true, TotalSize: 0xffffffff, Data: brush[:1]},
	} {
		if _, err := a.Add(f); err == nil {
			t.Fatal("accepted invalid continuation")
		}
		if err := a.Finish(); err != nil {
			t.Fatal("error did not reset assembler")
		}
	}
	font := append(longs(0), floats(12)...)
	put32(font, 0, 0xdbc01002)
	font = append(font, longs(3, 1, 0, 2)...)
	font = append(font, words('A', 'B')...)
	v, err = DecodePlusObject(6, font, DecodeLimits{})
	if err != nil || v.(PlusFont).EmSize != 12 || len(v.(PlusFont).FamilyUTF16) != 4 {
		t.Fatal(v, err)
	}
}

func TestPlusAlignedPlaceable(t *testing.T) {
	wmf := wmfFixture(true)
	// GDI+ variant: 22-byte placeable header, 2-byte alignment gap, then
	// standard WMF; the object count excludes the 24-byte placeable prefix.
	payload := append(append(append([]byte(nil), wmf[:22]...), 0, 0), wmf[22:]...)
	body := append(longs(0, 2, 2, int32(len(wmf)-22)), payload...)
	put32(body, 0, 0xdbc01002)
	v, err := DecodePlusObject(5, body, DecodeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	p := v.(PlusImage)
	if !p.AlignedPlaceable {
		t.Fatal("alignment variant not identified")
	}
	normalized, err := p.MetafileBytes(Limits{})
	if err != nil || !bytes.Equal(normalized, wmf) {
		t.Fatal(err)
	}
	if _, err := Walk(normalized, Limits{}, nil); err != nil {
		t.Fatal(err)
	}
	put32(body, 12, 0xffffffff)
	if _, err := DecodePlusObject(5, body, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("unbounded compatibility interpretation", err)
	}
}

func TestContinuationFinalPadding(t *testing.T) {
	a := PlusAssembler{Limits: DecodeLimits{MaxObjectBytes: 5}}
	if _, err := a.Add(PlusObjectFragment{ID: 1, Type: 5, Continued: true, TotalSize: 5, Data: []byte{1, 2, 3, 4}}); err != nil {
		t.Fatal(err)
	}
	got, err := a.Add(PlusObjectFragment{ID: 1, Type: 5, Data: []byte{5, 0xcc, 0xdd, 0xee}})
	if err != nil || !bytes.Equal(got, []byte{1, 2, 3, 4, 5}) {
		t.Fatalf("final alignment padding leaked into object: %x, %v", got, err)
	}
}

func TestPlusFixedRecordNoPayload(t *testing.T) {
	for _, typ := range []uint32{0x4002, 0x4004, 0x401e, 0x4031} {
		if _, err := Decode(testRecord(EMFPlus, typ, 0, []byte{1, 2, 3}), DecodeLimits{}); !errors.Is(err, ErrMalformed) {
			t.Fatalf("type %x accepted forbidden data: %v", typ, err)
		}
	}
}

func FuzzDecode(f *testing.F) {
	for kind, payload := range effectFixtures() {
		f.Add(uint8(EMFPlus), effectRecord(effectGUIDBytes(kind), payload).Raw)
	}
	gifData := gifFixture(f, true, true)
	gifObject := append(longs(0, 1, 0, 0, 0, 0, 1), gifData...)
	put32(gifObject, 0, 0xdbc01002)
	f.Add(uint8(0), append([]byte{5}, gifObject...))
	for _, r := range []Record{testRecord(WMF, 0x0538, 0, words(1, 2, 1, 2, 3, 4)), testRecord(EMF, 35, 0, floats(1, 0, 0, 1, 0, 0)), testRecord(EMFPlus, 0x400d, 0x800, append(longs(2), []byte{1, 2, 3, 4}...))} {
		f.Add(uint8(r.Format), r.Raw)
	}
	f.Add(uint8(WMF), testRecord(WMF, 0x06ff, 0, wmfRegionFixture()).Raw)
	f.Add(uint8(WMF), wmfPaletteFixture(2).Raw)
	f.Add(uint8(EMFPlus), driverStringFixture(false, true).Raw)
	f.Add(uint8(EMFPlus), driverStringFixture(true, true).Raw)
	f.Add(uint8(EMF), colorSpaceRecord(false).Raw)
	f.Add(uint8(EMF), colorSpaceRecord(true).Raw)
	texture := longs(0, 2, 0, 0, 0, 1, 1, 1, 4, 0x26200a, 0, 0)
	put32(texture, 0, 0xdbc01002)
	put32(texture, 16, 0xdbc01002)
	f.Add(uint8(0), append([]byte{1}, texture...))
	gradient := append(longs(0, 0, 10, 10, 2, 1, 0), make([]byte, 32)...)
	gradient = append(gradient, longs(0, 1)...)
	f.Add(uint8(EMF), testRecord(EMF, EMRGradientFill, 0, gradient).Raw)
	texts := make([]byte, 56)
	put32(texts, 16, 2)
	put32(texts, 28, 1)
	put32(texts, 48, 0x100)
	f.Add(uint8(EMF), testRecord(EMF, EMRPolyTextOutW, 0, texts).Raw)
	relativePath := append(longs(0, 3, 0x800), []byte{1, 2, 3, 4, 5, 6, 0x41, 0, 0x42, 1}...)
	put32(relativePath, 0, 0xdbc01002)
	f.Add(uint8(0), append([]byte{3}, relativePath...))
	f.Add(uint8(0), append([]byte{9}, arrowCapFixture()...))
	f.Add(uint8(0), append([]byte{9}, defaultCapFixture(relativePathFixture(10), relativePathFixture(10))...))
	f.Add(uint8(0), append([]byte{2}, penWithCapsFixture(arrowCapFixture(), arrowCapFixture())...))
	for _, format := range []uint32{PixelFormat1bppIndexed, PixelFormat4bppIndexed, PixelFormat8bppIndexed, PixelFormat16bppGrayScale, PixelFormat16bppRGB555, PixelFormat16bppRGB565, PixelFormat16bppARGB1555, PixelFormat24bppRGB, PixelFormat32bppRGB, PixelFormat32bppARGB, PixelFormat32bppPARGB, PixelFormat48bppRGB, PixelFormat64bppARGB, PixelFormat64bppPARGB} {
		stride := int32(4)
		if (format>>8)&255 > 32 {
			stride = 8
		}
		data := make([]byte, int(stride))
		if format&0x10000 != 0 {
			data = append(longs(0, 1, -1), data...)
		}
		p := PlusImage{Type: 1, Width: 1, Height: 1, Stride: stride, PixelFormat: format, Data: data}
		f.Add(uint8(0), append([]byte{5}, rawPlusImageObject(p)...))
	}
	for typ, body := range map[byte][]byte{
		1: gradientBody(0),
		3: append(append(longs(0, 2, 0x4000), words(-1, 2, 3, -4)...), 0, 1, 0, 0),
		4: longs(0, 0, 0x10000003),
		5: longs(0, 1, 1, 1, 4, 0x26200a, 0, 0),
		6: append(append(longs(0), floats(12)...), longs(2, 0, 0, 0)...),
		8: longs(0, 0, 0, 0, 0, 0),
	} {
		put32(body, 0, 0xdbc01002)
		f.Add(uint8(0), append([]byte{typ}, body...))
	}
	f.Fuzz(func(t *testing.T, format uint8, b []byte) {
		if len(b) > 1<<20 {
			return
		}
		r := Record{Format: Format(format), Raw: b}
		if len(b) >= 6 && r.Format == WMF {
			r.Type = uint32(u16(b[4:]))
		} else if len(b) >= 4 && r.Format == EMF {
			r.Type = u32(b)
		} else if len(b) >= 4 && r.Format == EMFPlus {
			r.Type = uint32(u16(b))
			r.Flags = u16(b[2:])
		}
		_, _ = Decode(r, DecodeLimits{MaxElements: 4096, MaxObjectBytes: 1 << 20})
		if len(b) > 0 {
			v, err := DecodePlusObject(b[0], b[1:], DecodeLimits{MaxElements: 4096, MaxObjectBytes: 1 << 20})
			if err == nil {
				if p, ok := v.(PlusImage); ok && p.Type == 1 {
					_, _ = p.Image(ImageLimits{MaxBytes: 1 << 20, MaxPixels: 4096})
				}
			}
		}
	})
}

func BenchmarkDecodeLargePolygonView(b *testing.B) {
	const count = 65536
	body := make([]byte, 20+count*8)
	put32(body, 16, count)
	for i := 0; i < count; i++ {
		put32(body, 20+i*8, uint32(i))
		put32(body, 24+i*8, uint32(-i))
	}
	r := testRecord(EMF, 3, 0, body)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, err := Decode(r, DecodeLimits{})
		if err != nil || v.(Poly).Points.Len() != count {
			b.Fatal(v, err)
		}
	}
}
