package gowemf

import (
	"errors"
	"fmt"
	"math"
	"testing"
	"unicode/utf16"
)

// fakeText measures with exact, font-size-derived metrics so expected
// positions can be computed by hand: em = |Height| (or 20 when zero), ascent
// 0.8 em, descent 0.2 em, advance 0.5 em, and 0.25 em for spaces.
type fakeText struct {
	recordingBackend
	measured []TextRun
	drawn    []TextRun
	clips    []Clip
	bad      *TextMetrics
}

func em(f FontRequest) float64 {
	if f.Height == 0 {
		return 20
	}
	return math.Abs(f.Height)
}

func (b *fakeText) MeasureText(run TextRun) (TextMetrics, error) {
	if run.Origins != nil || run.Advances != nil {
		return TextMetrics{}, errors.New("measure received placement")
	}
	b.measured = append(b.measured, run)
	if b.bad != nil {
		return *b.bad, nil
	}
	e := em(run.Font)
	m := TextMetrics{Ascent: .8 * e, Descent: .2 * e, Advances: make([]float64, len(run.Text))}
	for i, u := range run.Text {
		m.Advances[i] = .5 * e
		if !run.Glyphs && u == ' ' {
			m.Advances[i] = .25 * e
		}
	}
	return m, nil
}

func (b *fakeText) DrawText(run TextRun, clip Clip) error {
	b.note(checkClip(clip))
	if len(run.Origins) != len(run.Text) || len(run.Advances) != len(run.Text) || !run.Transform.Finite() {
		b.note(errors.New("inconsistent text run"))
	}
	b.drawn = append(b.drawn, run)
	b.clips = append(b.clips, clip)
	return nil
}

// FillGradient lets fuzzing exercise gradient meshes through fakeText.
func (b *fakeText) FillGradient(mesh []GradientTriangle, clip Clip) error {
	b.note(checkClip(clip))
	for _, tri := range mesh {
		for _, q := range tri.Points {
			if !finite(q.X) || !finite(q.Y) {
				b.note(errors.New("non-finite gradient vertex"))
			}
		}
	}
	return nil
}

func playText(t *testing.T, data []byte, o PlayOptions) *fakeText {
	t.Helper()
	if o.Destination == (Box{}) {
		o.Destination = Box{Width: 96, Height: 64}
	}
	b := &fakeText{}
	if _, err := Play(data, o, b); err != nil {
		t.Fatal(err)
	}
	if b.recordingBackend.bad != nil {
		t.Fatal(b.recordingBackend.bad)
	}
	return b
}

// emfFont creates EMR_EXTCREATEFONTINDIRECTW with a UTF-16 face name.
func emfFont(id uint32, height, escapement, orientation int32, charset byte, face string) []byte {
	name := make([]byte, 64)
	for i, u := range utf16.Encode([]rune(face)) {
		put16(name, i*2, u)
	}
	return emfRecord(EMRExtCreateFontIndirectW, cat(longs(int32(id), height, 0, escapement, orientation, 400), []byte{0, 0, 0, charset, 0, 0, 0, 0}, name))
}

// emfText encodes EMR_EXTTEXTOUTW (or A when ansi is non-nil) with an
// optional rectangle and advance array.
func emfText(mode int32, x, y int32, options uint32, rect *Rect, text string, ansi []byte, dx []int32) []byte {
	typ := uint32(EMRExtTextOutW)
	var str []byte
	n := 0
	if ansi != nil {
		typ, str, n = EMRExtTextOutA, ansi, len(ansi)
	} else {
		u := utf16.Encode([]rune(text))
		n = len(u)
		str = make([]byte, 2*n)
		for i, v := range u {
			put16(str, 2*i, v)
		}
	}
	fixed := 8 + 16 + 4 + 8 + 8 + 12 + 4
	if rect != nil {
		fixed += 16
	} else {
		options |= 0x100
	}
	str = append(str, make([]byte, (4-len(str)%4)%4)...)
	offDx := 0
	if dx != nil {
		offDx = fixed + len(str)
	}
	body := cat(longs(0, 0, 0, 0, mode), fl(1, 1), longs(x, y, int32(n), int32(fixed), int32(options)))
	if rect != nil {
		body = cat(body, longs(rect.Left, rect.Top, rect.Right, rect.Bottom))
	}
	return emfRecord(typ, cat(body, longs(int32(offDx)), str, longs(dx...)))
}

func sameOrigins(got []Point, want ...Point) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if !near(got[i], want[i]) {
			return false
		}
	}
	return true
}

func TestPlayTextAlignment(t *testing.T) {
	font := emfFont(1, -20, 0, 0, 0, "Liberation Sans")
	for _, tc := range []struct {
		align uint32
		want  []Point
	}{
		{0, []Point{{0, 16}, {10, 16}, {20, 16}}},       // TA_LEFT|TA_TOP
		{2 | 24, []Point{{-30, 0}, {-20, 0}, {-10, 0}}}, // TA_RIGHT|TA_BASELINE
		{6 | 8, []Point{{-15, -4}, {-5, -4}, {5, -4}}},  // TA_CENTER|TA_BOTTOM
	} {
		b := playText(t, emfScene(96, 64, 2, font, emfSelect(1), emfValue(EMRSetTextAlign, int32(tc.align)), emfValue(EMRSetTextColor, blue), emfText(1, 10, 20, 0, nil, "abc", nil, nil)), PlayOptions{})
		run := b.drawn[0]
		if !sameOrigins(run.Origins, tc.want...) || run.Transform != (Matrix{M11: 1, M22: 1, Dx: 10, Dy: 20}) {
			t.Fatalf("align %#x: %v %v", tc.align, run.Origins, run.Transform)
		}
		if run.Font.FaceName != "Liberation Sans" || run.Font.Height != -20 || run.Paint.Color != cBlue || string(utf16.Decode(run.Text)) != "abc" {
			t.Fatalf("run %+v", run)
		}
	}
}

func TestPlayTextSpacing(t *testing.T) {
	font := emfFont(1, -20, 0, 0, 0, "")
	// Explicit advances are logical units scaled by the x-axis under
	// GM_COMPATIBLE; the font height is scaled by the y-axis only.
	aniso := emfSplit(cat(emfValue(EMRSetMapMode, 8), emfPoint(EMRSetWindowExtEx, 2, 1), emfPoint(EMRSetViewportExtEx, 1, 2)))
	b := playText(t, emfScene(96, 64, 2, append(aniso, font, emfSelect(1), emfValue(EMRSetTextAlign, 24), emfText(1, 4, 4, 0, nil, "ab", nil, []int32{30, 8}))...), PlayOptions{})
	run := b.drawn[0]
	if !sameOrigins(run.Origins, Point{0, 0}, Point{15, 0}) || run.Font.Height != -40 || run.Transform != (Matrix{M11: 1, M22: 1, Dx: 2, Dy: 8}) {
		t.Fatalf("Dx: %v %+v %v", run.Origins, run.Font, run.Transform)
	}
	if len(run.Advances) != 2 || run.Advances[0] != 15 || run.Advances[1] != 4 {
		t.Fatal("advances passed to the backend", run.Advances)
	}
	// ETO_PDY pairs: zero vertical displacement is accepted, nonzero reported.
	b = playText(t, emfScene(96, 64, 2, font, emfSelect(1), emfValue(EMRSetTextAlign, 24), emfText(1, 0, 0, 0x2000, nil, "ab", nil, []int32{7, 0, 9, 0})), PlayOptions{})
	if !sameOrigins(b.drawn[0].Origins, Point{0, 0}, Point{7, 0}) {
		t.Fatal("ETO_PDY", b.drawn[0].Origins)
	}
	var reasons []string
	playText(t, emfScene(96, 64, 2, font, emfSelect(1), emfText(1, 0, 0, 0x2000, nil, "ab", nil, []int32{7, 1, 9, 0})), PlayOptions{Unsupported: func(u UnsupportedOperation) error {
		reasons = append(reasons, u.Reason)
		return nil
	}})
	if len(reasons) != 1 {
		t.Fatal(reasons)
	}
	// Justification: 11 extra units over two breaks, 6 then 5.
	b = playText(t, emfScene(96, 64, 2, font, emfSelect(1), emfValue(EMRSetTextAlign, 24), emfRecord(EMRSetTextJustification, longs(11, 2)), emfText(1, 0, 0, 0, nil, "a b c", nil, nil)), PlayOptions{})
	if !sameOrigins(b.drawn[0].Origins, Point{0, 0}, Point{10, 0}, Point{21, 0}, Point{31, 0}, Point{41, 0}) {
		t.Fatal("justification", b.drawn[0].Origins)
	}
	// WMF character extra, rounded in device units: 3 logical units at x2.
	wfont := testRecord(WMF, MetaCreateFontIndirect, 0, cat(words(-10, 0, 0, 0, 400), []byte{0, 0, 0, 0, 0, 0, 0, 0}, face32("Arial")))
	wb := playText(t, wmfScene(48, 32, 1, wfont, wmfRec(MetaSelectObject, 0), wmfRec(MetaSetTextAlign, 24), wmfRec(MetaSetTextCharExtra, 3), wmfTextOut(5, 6, "ab")), PlayOptions{})
	if run := wb.drawn[0]; !sameOrigins(run.Origins, Point{0, 0}, Point{16, 0}) || run.Font.Height != -20 || run.Transform != (Matrix{M11: 1, M22: 1, Dx: 10, Dy: 12}) {
		t.Fatalf("char extra: %v %+v %v", run.Origins, run.Font, run.Transform)
	}
	// A wrong advance count is malformed.
	bad := emfScene(96, 64, 2, font, emfSelect(1), emfText(1, 0, 0, 0, nil, "abc", nil, []int32{1, 2}))
	if _, err := Play(bad, PlayOptions{Destination: Box{Width: 1, Height: 1}}, &fakeText{}); err == nil {
		t.Fatal("short advance array accepted")
	}
}

// face32 pads a WMF face name to its 32-byte field.
func face32(s string) []byte { return append([]byte(s), make([]byte, 32-len(s))...) }

func wmfTextOut(x, y int16, s string) Record {
	b := []byte(s)
	if len(b)%2 == 1 {
		b = append(b, 0)
	}
	return testRecord(WMF, MetaTextOut, 0, cat(words(int16(len(s))), b, words(y, x)))
}

func TestPlayTextUpdatesCurrentPosition(t *testing.T) {
	font := emfFont(1, -20, 0, 0, 0, "")
	text := emfText(1, 999, 999, 0, nil, "ab", nil, nil)
	b := playText(t, emfScene(96, 64, 2, font, emfSelect(1), emfPoint(EMRMoveToEx, 5, 7), emfValue(EMRSetTextAlign, 24|1), text, text, emfValue(EMRSetTextAlign, 24|1|2), text, emfPoint(EMRLineTo, 0, 0)), PlayOptions{})
	for i, want := range []Point{{5, 7}, {25, 7}, {45, 7}} {
		if got := (Point{b.drawn[i].Transform.Dx, b.drawn[i].Transform.Dy}); !near(got, want) {
			t.Fatalf("string %d at %v, want %v", i, got, want)
		}
	}
	// TA_RIGHT moved the position back over the third string.
	if got := b.strokes[0].path.Points[0]; !near(got, Point{25, 7}) {
		t.Fatal("current position after TA_RIGHT", got)
	}
}

func TestPlayTextTransforms(t *testing.T) {
	// Escapement rotates the baseline counterclockwise as displayed.
	b := playText(t, emfScene(96, 64, 2, emfFont(1, -20, 900, 900, 0, ""), emfSelect(1), emfText(1, 10, 50, 0, nil, "a", nil, nil)), PlayOptions{})
	m := b.drawn[0].Transform
	if !near(m.Apply(Point{1, 0}), Point{10, 49}) || !near(m.Apply(Point{0, 1}), Point{11, 50}) {
		t.Fatal("escapement", m)
	}
	// GM_COMPATIBLE text stays upright under a y-up fixed mapping mode and
	// scales only its height; MM_LOMETRIC is 0.4 px per unit here.
	b = playText(t, emfScene(96, 64, 2, emfValue(EMRSetMapMode, 2), emfFont(1, -50, 0, 0, 0, ""), emfSelect(1), emfText(1, 10, -20, 0, nil, "a", nil, nil)), PlayOptions{})
	run := b.drawn[0]
	if run.Transform != (Matrix{M11: 1, M22: 1, Dx: 4, Dy: 8}) || run.Font.Height != -20 {
		t.Fatalf("compatible y-up: %v %+v", run.Transform, run.Font)
	}
	// GM_ADVANCED text follows the world transform; the font stays in logical
	// units and orientation is relative to the escapement.
	b = playText(t, emfScene(96, 64, 2, emfWorld(Matrix{M11: 2, M22: 3}), emfFont(1, -10, 300, 450, 0, ""), emfSelect(1), emfText(2, 4, 5, 0, nil, "a", nil, nil)), PlayOptions{})
	run = b.drawn[0]
	s, c := math.Sincos(30 * math.Pi / 180)
	want := Matrix{M11: 2 * c, M12: -3 * s, M21: 2 * s, M22: 3 * c, Dx: 8, Dy: 15}
	if math.Abs(run.Transform.M11-want.M11)+math.Abs(run.Transform.M12-want.M12)+math.Abs(run.Transform.M21-want.M21)+math.Abs(run.Transform.M22-want.M22)+math.Abs(run.Transform.Dx-want.Dx)+math.Abs(run.Transform.Dy-want.Dy) > 1e-9 {
		t.Fatal("advanced transform", run.Transform, want)
	}
	if run.Font.Height != -10 || math.Abs(run.Font.Orientation-15*math.Pi/180) > 1e-12 {
		t.Fatalf("advanced font %+v", run.Font)
	}
}

func TestPlayTextRectanglesAndBackground(t *testing.T) {
	font := emfFont(1, -20, 0, 0, 0, "")
	rect := &Rect{2, 3, 40, 30}
	b := playText(t, emfScene(96, 64, 2, font, emfSelect(1), emfValue(EMRSetBkColor, green), emfValue(EMRSetBkMode, 2), emfValue(EMRSetTextAlign, 24), emfText(1, 10, 20, 6, rect, "ab", nil, nil)), PlayOptions{})
	// The opaque rectangle, then the character-cell background, then text.
	if len(b.fills) != 2 || b.fills[0].paint.Color != cGreen || !near(b.fills[0].path.Points[2], Point{40, 30}) {
		t.Fatal("opaque rectangle", b.fills)
	}
	if cell := b.fills[1].path.Points; !near(cell[0], Point{10, 4}) || !near(cell[2], Point{30, 24}) {
		t.Fatal("background cell", cell)
	}
	clip := b.clips[0]
	if len(clip) != 1 || clip[0].Op != ClipReplace || !near(clip[0].Area.Points[2], Point{40, 30}) {
		t.Fatal("ETO_CLIPPED", clip)
	}
	// Transparent background mode leaves the cells alone.
	b = playText(t, emfScene(96, 64, 2, font, emfSelect(1), emfValue(EMRSetBkMode, 1), emfText(1, 10, 20, 0, nil, "ab", nil, nil)), PlayOptions{})
	if len(b.fills) != 0 {
		t.Fatal("transparent background filled", b.fills)
	}
}

func TestPlayTextEncodings(t *testing.T) {
	ansi := func(charset byte, data []byte, o PlayOptions) (*fakeText, []string) {
		var reasons []string
		o.Unsupported = func(u UnsupportedOperation) error {
			reasons = append(reasons, u.Reason)
			return nil
		}
		return playText(t, emfScene(96, 64, 2, emfFont(1, -20, 0, 0, charset, ""), emfSelect(1), emfText(1, 0, 0, 0, nil, "", data, nil)), o), reasons
	}
	if b, _ := ansi(0, []byte{'A', 0x80}, PlayOptions{}); string(utf16.Decode(b.drawn[0].Text)) != "A€" {
		t.Fatal("windows-1252", b.drawn[0].Text)
	}
	if _, reasons := ansi(1, []byte{'A'}, PlayOptions{}); len(reasons) != 1 {
		t.Fatal("DEFAULT_CHARSET without a stated system charset must be reported", reasons)
	}
	cyrillic := uint8(204)
	if b, _ := ansi(1, []byte{0xc0}, PlayOptions{DefaultCharSet: &cyrillic}); string(utf16.Decode(b.drawn[0].Text)) != "А" {
		t.Fatal("DEFAULT_CHARSET as windows-1251", b.drawn[0].Text)
	}
	if b, _ := ansi(2, []byte{0x41}, PlayOptions{}); b.drawn[0].Text[0] != 0xf041 {
		t.Fatal("symbol charset", b.drawn[0].Text)
	}
	for _, cs := range []byte{128, 255, 77} {
		if _, reasons := ansi(cs, []byte{'A'}, PlayOptions{}); len(reasons) != 1 {
			t.Fatal("charset", cs, reasons)
		}
	}
	// Glyph indexes pass through unchanged.
	b := playText(t, emfScene(96, 64, 2, emfFont(1, -20, 0, 0, 0, ""), emfSelect(1), emfText(1, 0, 0, 0x10, nil, "ģ", nil, nil)), PlayOptions{})
	if !b.drawn[0].Glyphs || b.drawn[0].Text[0] != 0x123 {
		t.Fatal("glyph indexes", b.drawn[0])
	}
	// SmallTextOut carries low bytes of UTF-16 code units.
	small := emfRecord(EMRSmallTextOut, cat(longs(3, 4, 2, 0x100|0x200, 1), fl(1, 1), []byte{'h', 0xe9, 0, 0}))
	b = playText(t, emfScene(96, 64, 2, small), PlayOptions{})
	if string(utf16.Decode(b.drawn[0].Text)) != "hé" || b.drawn[0].Font.Stock != systemFont {
		t.Fatal("SmallTextOut with the default stock font", b.drawn[0])
	}
	// WMF face names are ANSI in the font's character set.
	wfont := testRecord(WMF, MetaCreateFontIndirect, 0, cat(words(-10, 0, 0, 0, 400), []byte{0, 0, 0, 0, 0, 0, 0, 0}, face32("Caf\xe9")))
	wb := playText(t, wmfScene(48, 32, 1, wfont, wmfRec(MetaSelectObject, 0), wmfTextOut(0, 0, "x")), PlayOptions{})
	if wb.drawn[0].Font.FaceName != "Café" {
		t.Fatal("WMF face name", wb.drawn[0].Font.FaceName)
	}
}

func TestPlayTextObjectsAndOmissions(t *testing.T) {
	font := emfFont(1, -20, 0, 0, 0, "Face")
	text := emfText(1, 0, 0, 0, nil, "a", nil, nil)
	// Deleting the selected font restores SYSTEM_FONT; stock fonts select.
	b := playText(t, emfScene(96, 64, 2, font, emfSelect(1), emfDelete(1), text, emfSelect(0x8000000c), text), PlayOptions{})
	if b.drawn[0].Font.Stock != systemFont || b.drawn[1].Font.Stock != 12 || b.drawn[1].Font.CharSet != 0 {
		t.Fatal("font selection", b.drawn[0].Font, b.drawn[1].Font)
	}
	// PolyTextOut draws every string at its own reference point.
	poly := emfRecord(EMRPolyTextOutW, cat(longs(0, 0, 0, 0, 1), fl(1, 1), longs(2), longs(1, 2, 1, 88, 0x100, 0), longs(7, 8, 1, 92, 0x100, 0), []byte{'a', 0, 0, 0, 'b', 0, 0, 0}))
	b = playText(t, emfScene(96, 64, 2, poly), PlayOptions{})
	if len(b.drawn) != 2 || b.drawn[1].Transform.Dx != 7 || b.drawn[1].Text[0] != 'b' {
		t.Fatal("PolyTextOut", b.drawn)
	}
	for name, recs := range map[string][][]byte{
		"vertical font":  {emfFont(1, -20, 0, 0, 0, "@MS Mincho"), emfSelect(1), text},
		"path bracket":   {emfEmpty(EMRBeginPath), text, emfEmpty(EMRAbortPath)},
		"text color":     {emfValue(EMRSetTextColor, 0x01000002), text},
		"no TextBackend": nil,
	} {
		t.Run(name, func(t *testing.T) {
			var reasons []string
			o := PlayOptions{Destination: Box{Width: 1, Height: 1}, Unsupported: func(u UnsupportedOperation) error {
				reasons = append(reasons, u.Reason)
				return nil
			}}
			var backend Backend = &fakeText{}
			if recs == nil {
				recs, backend = [][]byte{text}, &recordingBackend{}
			}
			if _, err := Play(emfScene(96, 64, 2, recs...), o, backend); err != nil || len(reasons) != 1 {
				t.Fatal(err, reasons)
			}
		})
	}
	// Inconsistent backend metrics are an error, not silently trusted.
	if _, err := Play(emfScene(96, 64, 2, text), PlayOptions{Destination: Box{Width: 1, Height: 1}}, &fakeText{bad: &TextMetrics{Ascent: 1}}); err == nil {
		t.Fatal("missing advances accepted")
	}
	if _, err := Play(emfScene(96, 64, 2, text), PlayOptions{Destination: Box{Width: 1, Height: 1}}, &fakeText{bad: &TextMetrics{Ascent: math.NaN(), Advances: []float64{1}}}); err == nil {
		t.Fatal("NaN ascent accepted")
	}
}

// TestPlayTextBidi places text by the Unicode Bidirectional Algorithm, with a
// right-to-left paragraph under ETO_RTLREADING or TA_RTLREADING. The font has
// a 20-unit em, so letters advance 10 and spaces 5; positions are hand
// derived from UAX #9.
func TestPlayTextBidi(t *testing.T) {
	font := emfFont(1, -20, 0, 0, 0, "Face")
	xs := func(run TextRun) []float64 {
		out := make([]float64, len(run.Origins))
		for i, o := range run.Origins {
			out[i] = o.X
		}
		return out
	}
	same := func(a []float64, b ...float64) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if math.Abs(a[i]-b[i]) > 1e-9 {
				return false
			}
		}
		return true
	}
	for _, c := range []struct {
		name    string
		records [][]byte
		levels  []uint8
		x       []float64
	}{
		// Explicit advances are laid right to left; each letter (10 wide)
		// ends at the right of its advance.
		{"ETO_RTLREADING", [][]byte{emfText(1, 0, 0, 0x80, nil, "\u05e9\u05dc\u05d5\u05dd", nil, []int32{10, 20, 30, 40})}, []uint8{1, 1, 1, 1}, []float64{90, 80, 60, 30}},
		{"TA_RTLREADING", [][]byte{emfValue(EMRSetTextAlign, 0x100), emfText(1, 0, 0, 0, nil, "\u05e9\u05dc\u05d5\u05dd", nil, []int32{10, 20, 30, 40})}, []uint8{1, 1, 1, 1}, []float64{90, 80, 60, 30}},
		// Numbers and Latin text keep their order inside Hebrew text.
		{"mixed, right to left", [][]byte{emfText(1, 0, 0, 0x80, nil, "\u05d0\u05d1 12 cd", nil, nil)}, []uint8{1, 1, 1, 2, 2, 1, 2, 2}, []float64{60, 50, 45, 25, 35, 20, 0, 10}},
		{"mixed, left to right", [][]byte{emfText(1, 0, 0, 0, nil, "\u05d0\u05d1 12 cd", nil, nil)}, []uint8{1, 1, 1, 2, 2, 0, 0, 0}, []float64{35, 25, 20, 0, 10, 45, 50, 60}},
		// Right-aligned: the same layout ends at the reference point, the origin of text space.
		{"right aligned", [][]byte{emfValue(EMRSetTextAlign, 2), emfText(1, 70, 0, 0x80, nil, "\u05d0\u05d1 12 cd", nil, nil)}, []uint8{1, 1, 1, 2, 2, 1, 2, 2}, []float64{-10, -20, -25, -45, -35, -50, -70, -60}},
		// A surrogate pair stays together, in logical order.
		{"surrogates", [][]byte{emfText(1, 0, 0, 0x80, nil, "\U00010900\u05d0", nil, nil)}, []uint8{1, 1, 1}, []float64{10, 20, 0}},
		// Each paragraph separator ends a paragraph on the same line.
		{"paragraphs", [][]byte{emfText(1, 0, 0, 0x80, nil, "ab\u2029cd", nil, nil)}, []uint8{2, 2, 1, 2, 2}, []float64{30, 40, 20, 0, 10}},
		// Paired brackets resolve together (UAX #9 N0).
		{"brackets", [][]byte{emfText(1, 0, 0, 0x80, nil, "\u05d0 a(b)", nil, nil)}, []uint8{1, 1, 2, 2, 2, 2}, []float64{45, 40, 0, 10, 20, 30}},
		// An embedding ends with its paragraph (UAX #9 P1, X8).
		{"embedding and paragraph", [][]byte{emfText(1, 0, 0, 0, nil, "\u202bab\u2029cd", nil, nil)}, []uint8{0, 2, 2, 0, 0, 0}, []float64{0, 10, 20, 30, 40, 50}},
		// A joiner removed by rule X9 stays inside its right-to-left run.
		{"joiner", [][]byte{emfText(1, 0, 0, 0, nil, "\u05d0\u200d\u05d1", nil, nil)}, []uint8{1, 1, 1}, []float64{20, 10, 0}},
		// Glyph indexes are laid right to left; without the flag they are
		// left to right with no levels.
		{"glyphs", [][]byte{emfText(1, 0, 0, 0x80|0x10, nil, "\x05\x06\x07", nil, nil)}, []uint8{1, 1, 1}, []float64{20, 10, 0}},
		{"glyphs left to right", [][]byte{emfText(1, 0, 0, 0x10, nil, "\x05\x06\x07", nil, nil)}, nil, []float64{0, 10, 20}},
		{"latin left to right", [][]byte{emfText(1, 0, 0, 0, nil, "ab c", nil, nil)}, nil, []float64{0, 10, 20, 25}},
	} {
		b := playText(t, emfScene(96, 64, 2, append([][]byte{font, emfSelect(1)}, c.records...)...), PlayOptions{})
		if len(b.drawn) != 1 {
			t.Fatal(c.name, len(b.drawn))
		}
		run := b.drawn[0]
		if fmt.Sprint(run.Levels) != fmt.Sprint(c.levels) || fmt.Sprint(b.measured[0].Levels) != fmt.Sprint(c.levels) || !same(xs(run), c.x...) {
			t.Errorf("%s: levels %v (measured %v), x %v; want %v, %v", c.name, run.Levels, b.measured[0].Levels, xs(run), c.levels, c.x)
		}
		left := math.Inf(1)
		for _, x := range c.x {
			left = math.Min(left, x)
		}
		if c.name == "ETO_RTLREADING" || c.name == "TA_RTLREADING" {
			left = 0
		}
		if run.Left != left {
			t.Errorf("%s: Left %v, want %v", c.name, run.Left, left)
		}
	}
	// WMF: TA_RTLREADING with Hebrew ANSI text (code page 1255).
	wfont := testRecord(WMF, MetaCreateFontIndirect, 0, cat(words(-20, 0, 0, 0, 400), []byte{0, 0, 0, 177, 0, 0, 0, 0}, face32("Face")))
	wb := playText(t, wmfScene(96, 64, 1, wfont, wmfRec(MetaSelectObject, 0), wmfRec(MetaSetTextAlign, 0x100), wmfTextOut(0, 0, "\xe0\xe1 1")), PlayOptions{})
	if run := wb.drawn[0]; string(utf16.Decode(run.Text)) != "\u05d0\u05d1 1" || fmt.Sprint(run.Levels) != "[1 1 1 2]" || !same(xs(run), 25, 15, 10, 0) {
		t.Fatalf("WMF: %q %v %v", string(utf16.Decode(run.Text)), run.Levels, xs(run))
	}
}
