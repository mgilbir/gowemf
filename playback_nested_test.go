package gowemf

import (
	"errors"
	"image"
	"strings"
	"testing"
)

// metafileImage writes an EmfPlusImage holding a metafile (MS-EMFPLUS
// 2.2.1.4, 2.2.2.27) of the given MetafileDataType.
func metafileImageObj(typ uint32, data []byte) []byte {
	return cat(dwords(plusVersion, 2, typ, uint32(len(data))), data)
}

// innerEMF is a 40 x 20 pixel picture with a red rectangle over (0,0)-(20,10).
func innerEMF() []byte {
	return emfScene(40, 20, 2, emfSelect(nullPen), emfBrush(1, 0, red), emfSelect(1), emfBox(EMRRectangle, 0, 0, 21, 11))
}

func drawImage(id uint16, src, dst [4]float64) []byte {
	return plusRec(PlusDrawImageRecord, id, dwords(0xffffffff, 2), fl(src[:]...), fl(dst[:]...))
}

func TestPlayEMFPlusMetafileImage(t *testing.T) {
	half := plusRec(PlusSetPixelOffsetModeRecord, 4)
	img := plusObj(1, 5, metafileImageObj(3, innerEMF()))
	for _, c := range []struct {
		name     string
		src, dst [4]float64
		fill     []Point // the red rectangle, its right and bottom edges excluded
		area     []Point // the source rectangle's image
	}{
		// The 40 x 20 pixel frame fills a 41 x 21 pixel image, as in GDI+.
		{"whole", [4]float64{0, 0, 41, 21}, [4]float64{10, 10, 82, 42}, []Point{{10, 10}, {10, 31}, {51, 31}, {51, 10}}, []Point{{10, 10}, {92, 10}, {92, 52}, {10, 52}}},
		{"part", [4]float64{10, 0, 20, 20}, [4]float64{0, 0, 40, 40}, []Point{{-20, 0}, {-20, 21}, {21, 21}, {21, 0}}, []Point{{0, 0}, {40, 0}, {40, 40}, {0, 40}}},
	} {
		b, skipped := plusPlay(t, plusScene(96, 64, half, img, drawImage(1, c.src, c.dst)), PlayOptions{})
		if len(skipped) != 0 || len(b.fills) != 1 {
			t.Fatal(c.name, skipped, len(b.fills))
		}
		f := b.fills[0]
		if !pointsNear(f.path.Points, c.fill...) || f.paint.Color != cRed {
			t.Fatalf("%s: fill %v %v", c.name, f.path.Points, f.paint)
		}
		if len(f.clip) != 1 || f.clip[0].Op != ClipReplace || !pointsNear(f.clip[0].Area.Points, c.area...) {
			t.Fatalf("%s: clip %+v", c.name, f.clip)
		}
	}
	// A placeable WMF's image pixels are its logical units: 48 x 32 here,
	// placed on a parallelogram sheared by half a unit per row. The
	// rectangle (0,0)-(24,16) loses its right and bottom edges.
	wmf := wmfScene(48, 32, 2, wmfPen(5, 0, 0), wmfRec(MetaSelectObject, 0), wmfBrush(0, blue, 0), wmfRec(MetaSelectObject, 1), wmfBox(MetaRectangle, 0, 0, 24, 16))
	points := plusRec(PlusDrawImagePointsRecord, 1, dwords(0xffffffff, 2), fl(0, 0, 48, 32), dwords(3), fl(10, 10, 58, 10, 26, 42))
	b, skipped := plusPlay(t, plusScene(96, 64, half, plusObj(1, 5, metafileImageObj(2, wmf)), points), PlayOptions{})
	if len(skipped) != 0 || len(b.fills) != 1 || !pointsNear(b.fills[0].path.Points, Point{10, 10}, Point{17.5, 25}, Point{40.5, 25}, Point{33, 10}) {
		t.Fatal("placeable WMF", skipped, b.fills)
	}
}

func TestPlayEMFPlusMetafileImageLimits(t *testing.T) {
	report := func(data []byte, o PlayOptions) (*recordingBackend, []UnsupportedOperation, error) {
		var ops []UnsupportedOperation
		o.Destination = Box{Width: 96, Height: 64}
		o.Unsupported = func(u UnsupportedOperation) error { ops = append(ops, u); return nil }
		b := &recordingBackend{}
		_, err := Play(data, o, b)
		return b, ops, err
	}
	draw := drawImage(1, [4]float64{0, 0, 40, 20}, [4]float64{0, 0, 40, 20})
	after := fillRect(0xff00ff00, 50, 50, 4, 4)
	// The embedded picture's omissions are this record's, and prefixed.
	text := emfScene(40, 20, 2, emfFont(1, -10, 0, 0, 0, "F"), emfSelect(1), emfText(1, 0, 0, 0, nil, "a", nil, nil))
	data := plusScene(96, 64, plusObj(1, 5, metafileImageObj(3, text)), draw, after)
	b, ops, err := report(data, PlayOptions{})
	if err != nil || len(ops) != 1 || ops[0].Reason != "embedded metafile: text output" || ops[0].Source.Type != PlusDrawImageRecord || len(b.fills) != 1 {
		t.Fatal("prefixed report", err, ops)
	}
	// Without a callback the omission stops playback.
	if _, err := Play(data, PlayOptions{Destination: Box{Width: 96, Height: 64}}, &recordingBackend{}); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
	// A text backend receives the composed transform.
	tb := playText(t, data, PlayOptions{})
	if len(tb.drawn) != 1 || tb.drawn[0].Transform.Dx != 0.5 {
		t.Fatal("nested text", tb.drawn)
	}
	// A malformed embedded file is reported and the picture continues.
	bad := innerEMF()
	put32(bad, 48, 12) // nBytes too small
	b, ops, err = report(plusScene(96, 64, plusObj(1, 5, metafileImageObj(3, bad)), draw, after), PlayOptions{})
	if err != nil || len(ops) != 1 || !strings.HasPrefix(ops[0].Reason, "EMF+ metafile image: ") || len(b.fills) != 1 {
		t.Fatal("malformed embedded file", err, ops)
	}
	// A WMF without placeable bounds has no size.
	if _, ops, err = report(plusScene(96, 64, plusObj(1, 5, metafileImageObj(1, wmfDocument(0))), draw), PlayOptions{}); err != nil || len(ops) != 1 || !strings.Contains(ops[0].Reason, "placeable") {
		t.Fatal("WMF without bounds", err, ops)
	}
	// Nesting deeper than MaxMetafileDepth is reported: here three
	// embedded levels.
	nested := innerEMF()
	for i := 0; i < 3; i++ {
		nested = plusScene(40, 20, plusObj(1, 5, metafileImageObj(4, nested)), draw)
	}
	for depth, want := range map[uint32]int{2: 1, 3: 0} {
		b, ops, err := report(nested, PlayOptions{MaxMetafileDepth: depth})
		if err != nil || len(ops) != want || len(b.fills) != 1-want {
			t.Fatal("depth", depth, err, ops, len(b.fills))
		}
	}
	// Records are budgeted across every drawing of every embedded file.
	twice := plusScene(96, 64, plusObj(1, 5, metafileImageObj(3, innerEMF())), draw, draw)
	if _, _, err := report(twice, PlayOptions{MaxEmbeddedRecords: 12}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := report(twice, PlayOptions{MaxEmbeddedRecords: 11}); !errors.Is(err, ErrLimit) {
		t.Fatal("record budget", err)
	}
	// So are pixels: the embedded picture spends the same image budget.
	info, bits := sceneDIB(4, 4, false, quadrants)
	pic := emfScene(40, 20, 1, emfStretchDIBits(0, 0, 4, 4, 0, 0, 4, 4, 0x00cc0020, info, bits))
	if _, _, err := report(plusScene(96, 64, plusObj(1, 5, metafileImageObj(3, pic)), draw, draw), PlayOptions{MaxImagePixels: 31}); !errors.Is(err, ErrLimit) {
		t.Fatal("pixel budget", err)
	}
	// SourceCopy cannot know the picture is opaque.
	if _, ops, _ := report(plusScene(96, 64, plusRec(PlusSetCompositingModeRecord, 1), plusObj(1, 5, metafileImageObj(3, innerEMF())), draw), PlayOptions{}); len(ops) != 1 || !strings.Contains(ops[0].Reason, "SourceCopy") {
		t.Fatal("SourceCopy", ops)
	}
}

// allBackend records every call, implementing each optional interface.
type allBackend struct {
	fakeText
	gradients [][]GradientTriangle
	rasters   []RasterDraw
	clipsSeen []Clip
}

func (b *allBackend) FillGradient(mesh []GradientTriangle, clip Clip) error {
	b.gradients = append(b.gradients, mesh)
	b.clipsSeen = append(b.clipsSeen, clip)
	return nil
}
func (b *allBackend) DrawRaster(r RasterDraw, clip Clip) error {
	b.rasters = append(b.rasters, r)
	b.clipsSeen = append(b.clipsSeen, clip)
	return nil
}

// TestNestedBackend checks that every coordinate an embedded picture hands
// its backend is mapped into the enclosing destination.
func TestNestedBackend(t *testing.T) {
	m := Matrix{M11: 2, M12: 1, M21: -1, M22: 3, Dx: 5, Dy: 7}
	inv, _ := invertMatrix(m)
	outer := &ClipRegion{Op: ClipReplace, Area: Path{Verbs: []PathVerb{PathMoveTo, PathClose}, Points: []Point{{0, 0}}}, Rule: NonZero, depth: 1}
	inner := &allBackend{}
	n := &nestedBackend{inner: inner, m: m, inv: inv, outer: Clip{outer}, nodes: map[*ClipRegion]*ClipRegion{}}
	unit := Path{Verbs: []PathVerb{PathMoveTo, PathLineTo, PathClose}, Points: []Point{{1, 0}, {0, 1}}}
	base := &ClipRegion{Op: ClipReplace, Area: unit, Rule: EvenOdd, depth: 1}
	shifted := &ClipRegion{Base: base, Op: ClipOffset, Offset: Point{1, 1}, depth: 2}
	clip := Clip{shifted}
	gradient := &LinearGradient{Transform: Matrix{M11: 1, M22: 1, Dx: -3}, Stops: []GradientStop{{0, cRed}, {1, cBlue}}}
	paint := Paint{Kind: PaintLinearGradient, Gradient: gradient}
	if err := n.FillPath(unit, EvenOdd, paint, clip); err != nil {
		t.Fatal(err)
	}
	f := inner.fills[0]
	if !pointsNear(f.path.Points, m.Apply(Point{1, 0}), m.Apply(Point{0, 1})) {
		t.Fatal("path", f.path.Points)
	}
	// The gradient parameter at an enclosing point equals the embedded one.
	q := Point{4, -2}
	if got, want := f.paint.Gradient.Transform.Apply(m.Apply(q)), gradient.Transform.Apply(q); !pointsNear([]Point{got}, want) {
		t.Fatal("gradient", got, want)
	}
	if gradient.Transform.Dx != -3 {
		t.Fatal("the embedded paint was modified")
	}
	// Clip: the enclosing layer first, then mapped nodes, shared by identity.
	if len(f.clip) != 2 || f.clip[0] != outer || f.clip[1].Op != ClipOffset || f.clip[1].Offset != linear(m).Apply(Point{1, 1}) || !pointsNear(f.clip[1].Base.Area.Points, m.Apply(Point{1, 0}), m.Apply(Point{0, 1})) {
		t.Fatalf("clip %+v", f.clip)
	}
	pattern := Paint{Kind: PaintPattern, PatternTransform: Matrix{M11: 1, M22: 1, Dx: 2}}
	stroke := Stroke{Paint: pattern, Width: 2, Transform: Matrix{M11: 1, M22: 1}, PixelCenter: Point{0.5, 0.5}, Gap: &pattern}
	if err := n.StrokePath(unit, stroke, clip); err != nil {
		t.Fatal(err)
	}
	s := inner.strokes[0]
	if s.clip[1] != f.clip[1] || s.stroke.Transform != linear(m) || s.stroke.PixelCenter != linear(m).Apply(Point{0.5, 0.5}) ||
		s.stroke.Paint.PatternTransform != (Matrix{M11: 1, M22: 1, Dx: 2}).Then(m) || s.stroke.Gap.PatternTransform != s.stroke.Paint.PatternTransform {
		t.Fatalf("stroke %+v", s.stroke)
	}
	pixel := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img := ImageDraw{Image: pixel, Source: pixel.Bounds(), Opacity: 1, Transform: Matrix{M11: 3, M22: 3}}
	if err := n.DrawImage(img, nil); err != nil {
		t.Fatal(err)
	}
	if inner.images[0].draw.Transform != img.Transform.Then(m) || len(inner.images[0].clip) != 1 {
		t.Fatal("image", inner.images[0])
	}
	run := TextRun{Text: []uint16{'a'}, Origins: []Point{{0, 0}}, Advances: []float64{1}, Transform: Matrix{M11: 1, M22: 1, Dx: 1}}
	if err := n.DrawText(run, clip); err != nil {
		t.Fatal(err)
	}
	if inner.drawn[0].Transform != run.Transform.Then(m) {
		t.Fatal("text", inner.drawn[0].Transform)
	}
	if err := n.FillGradient([]GradientTriangle{{Points: [3]Point{{1, 0}, {0, 1}, {1, 1}}}}, clip); err != nil {
		t.Fatal(err)
	}
	if got := inner.gradients[0][0].Points; got != [3]Point{m.Apply(Point{1, 0}), m.Apply(Point{0, 1}), m.Apply(Point{1, 1})} {
		t.Fatal("mesh", got)
	}
	if err := n.DrawRaster(RasterDraw{Operation: 0x88, Area: unit, Source: &img, Pattern: &pattern}, clip); err != nil {
		t.Fatal(err)
	}
	if r := inner.rasters[0]; !pointsNear(r.Area.Points, m.Apply(Point{1, 0}), m.Apply(Point{0, 1})) || r.Source.Transform != img.Transform.Then(m) || r.Pattern.PatternTransform != pattern.PatternTransform.Then(m) {
		t.Fatalf("raster %+v", r)
	}
	// Optional interfaces follow the enclosing backend.
	if _, ok := backendAs[TextBackend](&nestedBackend{inner: &recordingBackend{}}); ok {
		t.Fatal("text without TextBackend")
	}
	if _, ok := backendAs[RasterBackend](&nestedBackend{inner: &nestedBackend{inner: inner}}); !ok {
		t.Fatal("raster through two levels")
	}
}
