package gowemf

import (
	"errors"
	"image"
	"image/color"
	"math"
	"strings"
	"testing"
	"unicode/utf16"
)

// EMF+ fixtures are written field by field from MS-EMFPLUS.

const plusVersion = 0xdbc01002

func dwords(v ...uint32) []byte {
	b := make([]byte, len(v)*4)
	for i, x := range v {
		put32(b, i*4, x)
	}
	return b
}

func plusRec(typ, flags uint16, body ...[]byte) []byte {
	r := plusRecord(typ, cat(body...))
	put16(r, 2, flags)
	return r
}

// plusObj defines object id of the given ObjectType; body includes the
// object's version field.
func plusObj(id, typ uint8, body ...[]byte) []byte {
	return plusRec(PlusObjectRecord, uint16(id)|uint16(typ)<<8, body...)
}

func solidBrush(argb uint32) []byte { return dwords(plusVersion, 0, argb) }
func hatchBrush(style, fore, back uint32) []byte {
	return dwords(plusVersion, 1, style, fore, back)
}

// linearBrush writes EmfPlusLinearGradientBrushData; extra holds the
// optional fields selected by flags.
func linearBrush(flags, wrap uint32, rect [4]float64, start, end uint32, extra ...[]byte) []byte {
	return cat(dwords(plusVersion, 4, flags, wrap), fl(rect[:]...), dwords(start, end, 0, 0), cat(extra...))
}

// plusPen writes EmfPlusPen with PenDataFlags, unit, width, the optional
// fields selected by flags, and a brush object.
func plusPen(flags, unit uint32, width float64, optional []byte, brush []byte) []byte {
	return cat(dwords(plusVersion, 0, flags, unit), fl(width), optional, brush)
}

// plusPathObj writes an uncompressed EmfPlusPath; types are padded.
func plusPathObj(points []float64, types []byte) []byte {
	t := append([]byte(nil), types...)
	for len(t)%4 != 0 {
		t = append(t, 0)
	}
	return cat(dwords(plusVersion, uint32(len(types)), 0), fl(points...), t)
}

// regionObj writes an EmfPlusRegion from preorder nodes.
func regionObj(nodes ...[]byte) []byte {
	return cat(dwords(plusVersion, uint32(len(nodes)-1)), cat(nodes...))
}
func rectNode(x, y, w, h float64) []byte { return cat(dwords(0x10000000), fl(x, y, w, h)) }

// argbImage writes a 32bppARGB bitmap object from rows of ARGB pixels.
func argbImage(w, h int32, pixels ...uint32) []byte {
	return cat(dwords(plusVersion, 1), longs(w, h, w*4), dwords(PixelFormat32bppARGB, 0), dwords(pixels...))
}

func plusHeaderDPI(dual bool, dpiX, dpiY uint32) []byte {
	r := plusRec(PlusHeaderRecord, 0, dwords(plusVersion, 1, dpiX, dpiY))
	if dual {
		put16(r, 2, 1)
	}
	return r
}

// plusScene is an EMF+ Only picture of w x h device pixels, mapped one to one
// onto the destination, at 96 DPI horizontally and 120 DPI vertically. Each
// record goes in its own comment.
func plusScene(w, h int32, records ...[]byte) []byte {
	all := []byte{}
	all = append(all, plusComment(plusHeaderDPI(false, 96, 120))...)
	for _, r := range records {
		all = append(all, plusComment(r)...)
	}
	all = append(all, plusComment(plusRec(PlusEndOfFileRecord, 0))...)
	return emfScene(w, h, 1, emfSplit(all)...)
}

func fillRect(argb uint32, x, y, w, h float64) []byte {
	return plusRec(PlusFillRectsRecord, 0x8000, dwords(argb, 1), fl(x, y, w, h))
}

func plusPlay(t *testing.T, data []byte, o PlayOptions) (*recordingBackend, []string) {
	t.Helper()
	var skipped []string
	if o.Destination == (Box{}) {
		o.Destination = Box{Width: 96, Height: 64}
	}
	o.Unsupported = func(u UnsupportedOperation) error {
		skipped = append(skipped, u.Reason)
		return nil
	}
	b := &recordingBackend{}
	if _, err := Play(data, o, b); err != nil {
		t.Fatal(err)
	}
	if b.bad != nil {
		t.Fatal(b.bad)
	}
	return b, skipped
}

func pointsNear(got []Point, want ...Point) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if math.Abs(got[i].X-want[i].X) > 1e-4 || math.Abs(got[i].Y-want[i].Y) > 1e-4 {
			return false
		}
	}
	return true
}

func TestPlayEMFPlusRectsAndPixelOffset(t *testing.T) {
	b, skipped := plusPlay(t, plusScene(96, 64, fillRect(0xff0000ff, 10, 20, 30, 40)), PlayOptions{})
	if len(skipped) != 0 || len(b.fills) != 1 {
		t.Fatal(skipped, b.fills)
	}
	f := b.fills[0]
	// PixelOffsetModeDefault: integer coordinates are pixel centers.
	if !pointsNear(f.path.Points, Point{10.5, 20.5}, Point{40.5, 20.5}, Point{40.5, 60.5}, Point{10.5, 60.5}) || f.paint.Color != (color.NRGBA{0, 0, 255, 255}) || f.paint.Kind != PaintSolid {
		t.Fatal(f.path.Points, f.paint)
	}
	b, _ = plusPlay(t, plusScene(96, 64, plusRec(PlusSetPixelOffsetModeRecord, 4), fillRect(0x80102030, 10, 20, 30, 40)), PlayOptions{})
	if !pointsNear(b.fills[0].path.Points[:1], Point{10, 20}) || b.fills[0].paint.Color != (color.NRGBA{0x10, 0x20, 0x30, 0x80}) {
		t.Fatal("PixelOffsetModeHalf", b.fills[0])
	}
	// Overlapping rectangles fill as one region, painting each pixel once.
	two := plusRec(PlusFillRectsRecord, 0x8000, dwords(0x80000000, 2), fl(0, 0, 10, 10, 5, 5, 10, 10))
	b, _ = plusPlay(t, plusScene(96, 64, two), PlayOptions{})
	if len(b.fills) != 1 || len(b.fills[0].path.Points) != 8 || b.fills[0].rule != NonZero {
		t.Fatal("FillRects must be one nonzero fill", b.fills)
	}
	// The brush scales with the picture: a frame twice the device size.
	b, _ = plusPlay(t, plusScene(96, 64, fillRect(0xff000000, 0, 0, 1, 1)), PlayOptions{Destination: Box{X: 5, Width: 192, Height: 128}})
	if !pointsNear(b.fills[0].path.Points[:3], Point{6, 1}, Point{8, 1}, Point{8, 3}) {
		t.Fatal("destination mapping", b.fills[0].path.Points)
	}
}

func TestPlayEMFPlusTransforms(t *testing.T) {
	half := plusRec(PlusSetPixelOffsetModeRecord, 4)
	at := func(records ...[]byte) Point {
		t.Helper()
		b, skipped := plusPlay(t, plusScene(96, 64, append([][]byte{half}, append(records, fillRect(0xff000000, 1, 0, 1, 1))...)...), PlayOptions{})
		if len(skipped) != 0 || len(b.fills) != 1 {
			t.Fatal(skipped, b.fills)
		}
		return b.fills[0].path.Points[0]
	}
	scale2 := plusRec(PlusSetWorldTransformRecord, 0, fl(2, 0, 0, 3, 5, 7))
	for _, c := range []struct {
		name string
		got  Point
		want Point
	}{
		{"set", at(scale2), Point{7, 7}},
		{"reset", at(scale2, plusRec(PlusResetWorldTransformRecord, 0)), Point{1, 0}},
		// Prepend: the translation applies in world space before scaling.
		{"translate prepend", at(scale2, plusRec(PlusTranslateWorldTransformRecord, 0, fl(10, 1))), Point{27, 10}},
		{"translate append", at(scale2, plusRec(PlusTranslateWorldTransformRecord, 0x2000, fl(10, 1))), Point{17, 8}},
		{"scale prepend", at(scale2, plusRec(PlusScaleWorldTransformRecord, 0, fl(4, 1))), Point{13, 7}},
		{"scale append", at(scale2, plusRec(PlusScaleWorldTransformRecord, 0x2000, fl(4, 1))), Point{28, 7}},
		// Positive rotation turns the x axis toward the y axis.
		{"rotate", at(plusRec(PlusRotateWorldTransformRecord, 0, fl(90))), Point{0, 1}},
		{"rotate append", at(scale2, plusRec(PlusRotateWorldTransformRecord, 0x2000, fl(90))), Point{-7, 7}},
		{"multiply prepend", at(scale2, plusRec(PlusMultiplyWorldTransformRecord, 0, fl(1, 0, 0, 1, 0, 2))), Point{7, 13}},
		{"multiply append", at(scale2, plusRec(PlusMultiplyWorldTransformRecord, 0x2000, fl(1, 0, 0, 1, 0, 2))), Point{7, 9}},
		// Inch pages at 96x120 DPI with scale 0.5.
		{"page inch", at(plusRec(PlusSetPageTransformRecord, 4, fl(0.5))), Point{48, 0}},
		{"page point", at(plusRec(PlusSetPageTransformRecord, 3, fl(1)), plusRec(PlusTranslateWorldTransformRecord, 0, fl(0, 72))), Point{96. / 72, 120}},
		{"page millimeter", at(plusRec(PlusSetPageTransformRecord, 6, fl(25.4))), Point{96, 0}},
		{"page document", at(plusRec(PlusSetPageTransformRecord, 5, fl(300))), Point{96, 0}},
	} {
		if !pointsNear([]Point{c.got}, c.want) {
			t.Errorf("%s: %v, want %v", c.name, c.got, c.want)
		}
	}
	// Display and World page units have no defined size.
	b, skipped := plusPlay(t, plusScene(96, 64, plusRec(PlusSetPageTransformRecord, 1, fl(1)), fillRect(0xff000000, 0, 0, 1, 1)), PlayOptions{})
	if len(b.fills) != 0 || len(skipped) != 1 || !strings.Contains(skipped[0], "page unit") {
		t.Fatal(skipped)
	}
	if _, err := Play(plusScene(96, 64, plusRec(PlusSetPageTransformRecord, 2, fl(0))), PlayOptions{Destination: Box{Width: 1, Height: 1}}, &recordingBackend{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("zero page scale:", err)
	}
	// Repeated scaling overflows float64 after nine factors of 3.4e38.
	huge := [][]byte{}
	for i := 0; i < 9; i++ {
		huge = append(huge, plusRec(PlusScaleWorldTransformRecord, 0, fl(math.MaxFloat32, 1)))
	}
	if _, err := Play(plusScene(96, 64, huge...), PlayOptions{Destination: Box{Width: 1, Height: 1}}, &recordingBackend{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("overflowing world transform:", err)
	}
}

func TestPlayEMFPlusSaveAndContainers(t *testing.T) {
	half := plusRec(PlusSetPixelOffsetModeRecord, 4)
	scale2 := plusRec(PlusSetWorldTransformRecord, 0, fl(2, 0, 0, 2, 0, 0))
	clip := plusRec(PlusSetClipRectRecord, 0, fl(0, 0, 10, 10)) // replace
	// The container maps source (0,0,10,10) onto destination (100,0,50,50)
	// in the enclosing world space, in Pixel units.
	begin := plusRec(PlusBeginContainerRecord, 2<<8, fl(100, 0, 50, 50, 0, 0, 10, 10), dwords(7))
	b, skipped := plusPlay(t, plusScene(400, 64,
		half, scale2, clip,
		plusRec(PlusSaveRecord, 0, dwords(3)),
		plusRec(PlusTranslateWorldTransformRecord, 0, fl(1, 1)),
		plusRec(PlusRestoreRecord, 0, dwords(3)),
		fillRect(0xff000000, 1, 1, 1, 1), // 0: scale 2 restored
		begin,
		fillRect(0xff000000, 1, 1, 1, 1), // 1: container mapping, then scale 2
		plusRec(PlusResetClipRecord, 0),
		plusRec(PlusSetWorldTransformRecord, 0, fl(1, 0, 0, 1, 2, 0)),
		fillRect(0xff000000, 1, 1, 1, 1), // 2: inner world transform first
		plusRec(PlusBeginContainerNoParamsRecord, 0, dwords(8)),
		fillRect(0xff000000, 1, 1, 1, 1), // 3: nested, inherits mapping
		plusRec(PlusEndContainerRecord, 0, dwords(8)),
		plusRec(PlusEndContainerRecord, 0, dwords(7)),
		fillRect(0xff000000, 1, 1, 1, 1), // 4: outer state restored
	), PlayOptions{Destination: Box{Width: 400, Height: 64}})
	if len(skipped) != 0 || len(b.fills) != 5 {
		t.Fatal(skipped, len(b.fills))
	}
	for i, want := range []Point{{2, 2}, {210, 10}, {230, 10}, {230, 10}, {2, 2}} {
		if !pointsNear(b.fills[i].path.Points[:1], want) {
			t.Errorf("fill %d at %v, want %v", i, b.fills[i].path.Points[0], want)
		}
	}
	// The outer clip constrains the container as a metaregion that ResetClip
	// inside does not remove; EndContainer restores it as the clip.
	outer := b.fills[0].clip
	if len(outer) != 1 || len(b.fills[1].clip) != 1 || b.fills[1].clip[0] != outer[0] || len(b.fills[2].clip) != 1 || b.fills[2].clip[0] != outer[0] || len(b.fills[4].clip) != 1 || b.fills[4].clip[0] != outer[0] {
		t.Fatal("container clipping", b.fills[1].clip, b.fills[2].clip, b.fills[4].clip)
	}
	// A clip inside the container adds a layer after the metaregion.
	b, _ = plusPlay(t, plusScene(400, 64, clip, begin, clip, fillRect(0xff000000, 0, 0, 1, 1)), PlayOptions{Destination: Box{Width: 400, Height: 64}})
	if c := b.fills[0].clip; len(c) != 2 || pointsNear(c[0].Area.Points, c[1].Area.Points...) {
		t.Fatal("inner clip must be in container space", c)
	}
	// Physical container units are not defined relative to the page.
	// Its contents are skipped, not drawn misplaced, and EndContainer
	// restores the enclosing state.
	b, skipped = plusPlay(t, plusScene(96, 64, plusRec(PlusBeginContainerRecord, 4<<8, fl(0, 0, 1, 1, 0, 0, 1, 1), dwords(1)),
		fillRect(0xff000000, 0, 0, 1, 1), plusRec(PlusClearRecord, 0, dwords(0xff000000)),
		plusRec(PlusBeginContainerNoParamsRecord, 0, dwords(2)), fillRect(0xff000000, 0, 0, 1, 1), plusRec(PlusEndContainerRecord, 0, dwords(2)),
		plusRec(PlusEndContainerRecord, 0, dwords(1)), fillRect(0xff000000, 0, 0, 1, 1)), PlayOptions{})
	if len(skipped) != 1 || !strings.Contains(skipped[0], "container") || len(b.fills) != 1 {
		t.Fatal(skipped, b.fills)
	}
}

func TestPlayEMFPlusClipping(t *testing.T) {
	const w, h = 40, 20
	probe := func(name string, want map[[2]int]bool, records ...[]byte) {
		t.Helper()
		data := plusScene(w, h, append(append([][]byte{plusRec(PlusSetPixelOffsetModeRecord, 4)}, records...), fillRect(0xff000000, 0, 0, w, h))...)
		rb := newRasterBackend(w, h)
		if _, err := Play(data, PlayOptions{Destination: Box{Width: w, Height: h}}, rb); err != nil {
			t.Fatal(name, err)
		}
		for p, inked := range want {
			if got := rb.canvas.NRGBAAt(p[0], p[1]).R == 0; got != inked {
				t.Errorf("%s: pixel %v inked = %v, want %v", name, p, got, inked)
			}
		}
	}
	a := plusRec(PlusSetClipRectRecord, 0, fl(0, 0, 20, 20))
	combine := func(mode uint16) []byte { return plusRec(PlusSetClipRectRecord, mode<<8, fl(10, 0, 20, 20)) }
	// Probes: only in A, in both, only in B, in neither.
	pts := func(onlyA, both, onlyB, neither bool) map[[2]int]bool {
		return map[[2]int]bool{{5, 5}: onlyA, {15, 5}: both, {25, 5}: onlyB, {35, 5}: neither}
	}
	probe("replace", pts(false, true, true, false), a, combine(0))
	probe("intersect", pts(false, true, false, false), a, combine(1))
	probe("union", pts(true, true, true, false), a, combine(2))
	probe("xor", pts(true, false, true, false), a, combine(3))
	probe("exclude", pts(true, false, false, false), a, combine(4))
	probe("complement", pts(false, false, true, false), a, combine(5))
	probe("reset", pts(true, true, true, true), a, plusRec(PlusResetClipRecord, 0))
	// Without a clip, the infinite region intersects to the new area.
	probe("intersect infinite", pts(false, true, true, false), combine(1))
	probe("complement infinite", pts(false, false, false, false), combine(5))
	probe("offset", pts(false, true, true, false), a, plusRec(PlusOffsetClipRecord, 0, fl(10, 0)))
	// The offset is a world-space vector: translations do not apply.
	probe("offset under translation", pts(false, true, true, false), a, plusRec(PlusSetWorldTransformRecord, 0, fl(1, 0, 0, 1, 50, 0)), plusRec(PlusOffsetClipRecord, 0, fl(10, 0)), plusRec(PlusResetWorldTransformRecord, 0))
	// Region trees: Exclude is left minus right, Complement right minus left.
	region := func(op uint32) []byte {
		return plusObj(1, 4, regionObj(dwords(op), rectNode(0, 0, 20, 20), rectNode(10, 0, 20, 20)))
	}
	setRegion := func(mode uint16) []byte { return plusRec(PlusSetClipRegionRecord, mode<<8|1) }
	probe("region and", pts(false, true, false, false), region(1), setRegion(0))
	probe("region or", pts(true, true, true, false), region(2), setRegion(0))
	probe("region xor", pts(true, false, true, false), region(3), setRegion(0))
	probe("region exclude", pts(true, false, false, false), region(4), setRegion(0))
	probe("region complement", pts(false, false, true, false), region(5), setRegion(0))
	probe("region infinite", pts(true, true, true, true), plusObj(1, 4, regionObj(dwords(0x10000003))), a, setRegion(2))
	probe("region empty", pts(false, false, false, false), plusObj(1, 4, regionObj(dwords(0x10000002))), a, setRegion(0))
	probe("region combined with clip", pts(false, true, false, false), region(2), plusRec(PlusSetClipRectRecord, 0, fl(12, 0, 6, 20)), setRegion(1))
	// A path clip is an alternate-filled closed figure (here a triangle
	// covering the left probes).
	path := plusObj(2, 3, plusPathObj([]float64{0, 0, 30, 0, 0, 30}, []byte{0, 1, 1}))
	probe("path", pts(true, true, false, false), path, plusRec(PlusSetClipPathRecord, 2))
	// FillRegion fills a region within the clip.
	rb := newRasterBackend(w, h)
	data := plusScene(w, h, plusRec(PlusSetPixelOffsetModeRecord, 4), region(5), a, plusRec(PlusFillRegionRecord, 0x8001, dwords(0xff000000)))
	if _, err := Play(data, PlayOptions{Destination: Box{Width: w, Height: h}}, rb); err != nil {
		t.Fatal(err)
	}
	if rb.canvas.NRGBAAt(25, 5).R != 255 || rb.canvas.NRGBAAt(15, 5).R != 255 {
		t.Fatal("FillRegion of B-A inside clip A paints nothing")
	}
	data = plusScene(w, h, plusRec(PlusSetPixelOffsetModeRecord, 4), region(5), plusRec(PlusFillRegionRecord, 0x8001, dwords(0xff000000)))
	rb = newRasterBackend(w, h)
	if _, err := Play(data, PlayOptions{Destination: Box{Width: w, Height: h}}, rb); err != nil {
		t.Fatal(err)
	}
	if rb.canvas.NRGBAAt(25, 5).R != 0 || rb.canvas.NRGBAAt(15, 5).R != 255 || rb.canvas.NRGBAAt(5, 5).R != 255 {
		t.Fatal("FillRegion of B-A")
	}
	// Clear fills the clip.
	b, _ := plusPlay(t, plusScene(w, h, a, plusRec(PlusClearRecord, 0, dwords(0xff00ff00))), PlayOptions{Destination: Box{Width: w, Height: h}})
	if len(b.fills) != 1 || b.fills[0].paint.Color != (color.NRGBA{0, 255, 0, 255}) || len(b.fills[0].clip) != 1 {
		t.Fatal("Clear", b.fills)
	}
}

func TestPlayEMFPlusClipLimits(t *testing.T) {
	records := []byte{}
	for i := 0; i < 5; i++ {
		records = append(records, plusComment(plusRec(PlusSetClipRectRecord, 1<<8, fl(0, 0, 10, 10)))...)
	}
	data := emfScene(96, 64, 1, emfSplit(cat(plusComment(plusHeaderDPI(false, 96, 96)), records, plusComment(plusRec(PlusEndOfFileRecord, 0))))...)
	o := PlayOptions{Destination: Box{Width: 96, Height: 64}, MaxClipSteps: 4}
	if _, err := Play(data, o, &recordingBackend{}); !errors.Is(err, ErrLimit) {
		t.Fatal("clip steps:", err)
	}
	o.MaxClipSteps = 5
	if _, err := Play(data, o, &recordingBackend{}); err != nil {
		t.Fatal(err)
	}
	// Region trees count every node.
	tree := plusObj(1, 4, regionObj(dwords(1), dwords(2), rectNode(0, 0, 1, 1), rectNode(0, 0, 1, 1), rectNode(0, 0, 1, 1)))
	data = plusScene(96, 64, tree, plusRec(PlusSetClipRegionRecord, 1))
	o.MaxClipSteps = 4
	if _, err := Play(data, o, &recordingBackend{}); !errors.Is(err, ErrLimit) {
		t.Fatal("region steps:", err)
	}
	o.MaxClipSteps = 5
	if _, err := Play(data, o, &recordingBackend{}); err != nil {
		t.Fatal(err)
	}
}

func TestPlayEMFPlusPens(t *testing.T) {
	stroke := func(t *testing.T, pen []byte, records ...[]byte) (Stroke, []string) {
		t.Helper()
		all := append([][]byte{plusRec(PlusSetPixelOffsetModeRecord, 4), plusObj(3, 2, pen)}, records...)
		all = append(all, plusRec(PlusDrawRectsRecord, 3, dwords(1), fl(0, 0, 10, 10)))
		b, skipped := plusPlay(t, plusScene(96, 64, all...), PlayOptions{})
		if len(b.strokes) == 0 {
			return Stroke{}, skipped
		}
		return b.strokes[0].stroke, skipped
	}
	red := solidBrush(0xffff0000)
	scale := plusRec(PlusSetWorldTransformRecord, 0, fl(2, 0, 0, 3, 0, 0))
	s, _ := stroke(t, plusPen(0, 0, 4, nil, red), scale)
	if s.Width != 4 || s.Transform != (Matrix{M11: 2, M22: 3}) || s.Hairline || s.Cap != CapFlat || s.Join != JoinMiter || s.MiterLimit != 10 || s.Dash != DashSolid || s.Paint.Color != (color.NRGBA{255, 0, 0, 255}) {
		t.Fatalf("world pen %+v", s)
	}
	// The pen transform applies in pen space before the world transform.
	s, _ = stroke(t, plusPen(1, 0, 4, fl(1, 0, 0, 0.5, 9, 9), red), scale)
	if s.Transform != (Matrix{M11: 2, M22: 1.5}) {
		t.Fatalf("pen transform %+v", s.Transform)
	}
	// Pixel widths ignore the world transform.
	s, _ = stroke(t, plusPen(0, 2, 3, nil, red), scale)
	if s.Width != 3 || s.Transform != Identity() {
		t.Fatalf("pixel pen %+v", s)
	}
	s, _ = stroke(t, plusPen(0, 0, 0, nil, red))
	if !s.Hairline {
		t.Fatal("zero width must be a hairline")
	}
	// Caps (round), join (bevel), miter limit; dash style and offset.
	s, _ = stroke(t, plusPen(2|4|8|16, 0, 2, cat(dwords(2, 2, 1), fl(4)), red))
	if s.Cap != CapRound || s.EndCap != 0 || s.Join != JoinBevel || s.MiterLimit != 4 || s.Dash != DashSolid {
		t.Fatalf("styled pen %+v", s)
	}
	s, _ = stroke(t, plusPen(32|128, 0, 2, cat(dwords(3), fl(1.5)), red))
	if s.Dash != DashUser || !pointsNear([]Point{{s.Dashes[0], s.Dashes[1]}, {s.Dashes[2], s.Dashes[3]}}, Point{6, 2}, Point{2, 2}) || s.DashOffset != 3 {
		t.Fatalf("dashed pen %+v", s)
	}
	// Different start and end caps; NoAnchor ends flat and SquareAnchor is
	// a line-width square centered on the end, like Square.
	s, _ = stroke(t, plusPen(2|4, 0, 2, dwords(2, 0x11), red))
	if s.Cap != CapRound || s.EndCap != CapSquare {
		t.Fatalf("mixed caps %+v", s)
	}
	s, _ = stroke(t, plusPen(2|4, 0, 2, dwords(0x10, 0x10), red))
	if s.Cap != CapFlat || s.EndCap != 0 {
		t.Fatalf("no-anchor caps %+v", s)
	}
	// Symmetric compound pens with miter joins.
	s, _ = stroke(t, plusPen(1024, 0, 2, cat(dwords(4), fl(0, .25, .75, 1)), red))
	if len(s.Compound) != 4 || s.Compound[1] != .25 {
		t.Fatalf("compound %+v", s.Compound)
	}
	s, _ = stroke(t, plusPen(32|256, 0, 2, cat(dwords(5), dwords(2), fl(1, 3)), red))
	if len(s.Dashes) != 2 || s.Dashes[0] != 2 || s.Dashes[1] != 6 {
		t.Fatalf("custom dashes %+v", s.Dashes)
	}
	// Compound bands are exact only for flat caps on open figures and
	// corners within the miter limit (here 1.4 for a right angle, 10 for the
	// default limit).
	compound := plusObj(4, 2, plusPen(2|4|1024, 0, 2, cat(dwords(2, 2), dwords(2), fl(0, 1)), red))
	sharp := plusObj(5, 2, plusPen(1024, 0, 2, cat(dwords(2), fl(0, 1)), red))
	lines := func(id uint16, pts ...float64) []byte {
		return plusRec(PlusDrawLinesRecord, id, dwords(uint32(len(pts)/2)), fl(pts...))
	}
	for _, c := range []struct {
		records [][]byte
		reason  string
	}{
		{[][]byte{compound, lines(4, 0, 0, 10, 0)}, "non-flat caps on an open figure"},
		{[][]byte{sharp, lines(5, 0, 0, 10, 0, 0, 1)}, "miter limit"},
		{[][]byte{sharp, lines(5|0x2000, 0, 0, 10, 0, 0, 1)}, "miter limit"},
		// Only the corner where the figure closes is sharp.
		{[][]byte{sharp, lines(5|0x2000, 0, 0, 20, 1, 20, 2)}, "miter limit"},
		{[][]byte{sharp, lines(5, 0, 0, 10, 0, 10, 10), compound, lines(4|0x2000, 0, 0, 10, 0, 10, 10)}, ""},
	} {
		b, skipped := plusPlay(t, plusScene(96, 64, c.records...), PlayOptions{})
		if c.reason == "" {
			if len(skipped) != 0 || len(b.strokes) != 2 {
				t.Errorf("compound corners within the limit: %v", skipped)
			}
		} else if len(skipped) != 1 || !strings.Contains(skipped[0], c.reason) {
			t.Errorf("%s: %v", c.reason, skipped)
		}
	}
	// A pen brush may be a hatch, which the rendering origin anchors.
	s, _ = stroke(t, plusPen(0, 0, 1, nil, hatchBrush(4, 0xff000000, 0x80ffffff)), plusRec(PlusSetRenderingOriginRecord, 0, longs(3, 4)))
	if s.Paint.Kind != PaintHatch || s.Paint.Hatch != 4 || *s.Paint.Background != (color.NRGBA{255, 255, 255, 128}) || s.Paint.PatternTransform != (Matrix{M11: 1, M22: 1, Dx: 3, Dy: 4}) {
		t.Fatalf("hatch pen %+v", s.Paint)
	}
	for _, c := range []struct {
		pen    []byte
		reason string
	}{
		{plusPen(1024, 0, 2, cat(dwords(4), fl(0, .2, .5, 1)), red), "asymmetric compound"},
		{plusPen(8|1024, 0, 2, cat(dwords(2), dwords(2), fl(0, 1)), red), "compound pen with bevel or round"},
		{plusPen(32|1024, 0, 2, cat(dwords(1), dwords(2), fl(0, 1)), red), "dashed compound"},
		{plusPen(1024, 0, 0, cat(dwords(2), fl(0, 1)), red), "zero-width compound"},
		{plusPen(2|4, 0, 2, dwords(3, 3), red), "triangle"},
		{plusPen(2|4, 0, 2, dwords(0, 0x14), red), "arrow line cap"},
		{plusPen(2|4, 0, 2, dwords(0x12, 0), red), "round, diamond"},
		{plusPen(2|4, 0, 2, dwords(0xff, 0), red), "custom line cap"},
		{plusPen(2|32, 0, 2, dwords(2, 1), red), "dashed pen with non-flat line caps"},
		{plusPen(8, 0, 2, dwords(3), red), "clipped miter"},
		{plusPen(512, 0, 2, dwords(1), red), "alignment"},
		{plusPen(32|64, 0, 2, dwords(1, 2), red), "dash caps"},
		{plusPen(32, 0, 0, dwords(1), red), "dashed zero-width"},
		{plusPen(0, 4, 2, nil, red), "unit"},
		{plusPen(1, 2, 2, fl(2, 0, 0, 2, 0, 0), red), "Pixel-unit pen"},
		{plusPen(0, 0, 2, nil, dwords(plusVersion, 3, 0, 0, 0xff000000, 0, 0, 0, 0)), "path gradient"},
		{plusPen(0, 0, 2, nil, hatchBrush(6, 0xff000000, 0xffffffff)), "hatch style 6"},
	} {
		if _, skipped := stroke(t, c.pen); len(skipped) != 1 || !strings.Contains(skipped[0], c.reason) {
			t.Errorf("%s: %v", c.reason, skipped)
		}
	}
}

func TestPlayEMFPlusShapes(t *testing.T) {
	half := plusRec(PlusSetPixelOffsetModeRecord, 4)
	pen := plusObj(1, 2, plusPen(0, 0, 1, nil, solidBrush(0xff000000)))
	shapes := func(records ...[]byte) *recordingBackend {
		t.Helper()
		b, skipped := plusPlay(t, plusScene(96, 64, append([][]byte{half, pen}, records...)...), PlayOptions{})
		if len(skipped) != 0 {
			t.Fatal(skipped)
		}
		return b
	}
	last := func(p Path) Point { return p.Points[len(p.Points)-1] }
	// Pie angles are clockwise from the x axis; on an ellipse they are
	// geometric: the 45 degree ray meets the 100x50 ellipse at x = y.
	b := shapes(plusRec(PlusFillPieRecord, 0x8000, dwords(0xff000000), fl(0, 90), fl(0, 0, 100, 50)),
		plusRec(PlusFillPieRecord, 0x8000, dwords(0xff000000), fl(45, -45), fl(0, 0, 100, 50)),
		plusRec(PlusDrawArcRecord, 1, fl(180, 360), fl(0, 0, 100, 50)),
		plusRec(PlusDrawArcRecord, 1, fl(0, 0), fl(0, 0, 100, 50)),
		plusRec(PlusFillEllipseRecord, 0x8000, dwords(0xff000000), fl(10, 10, 20, 10)))
	pie := b.fills[0].path
	if !near(pie.Points[0], Point{100, 25}) || !near(pie.Points[len(pie.Points)-2], Point{50, 50}) || !near(last(pie), Point{50, 25}) || pie.Verbs[len(pie.Verbs)-1] != PathClose {
		t.Fatal("pie", pie.Points)
	}
	// The clockwise sweep from 0 degrees leaves the start point downward.
	if pie.Points[1].Y <= 25 {
		t.Fatal("pie direction", pie.Points)
	}
	x := math.Sqrt(500)
	back := b.fills[1].path
	if !near(back.Points[0], Point{50 + x, 25 + x}) || !near(back.Points[len(back.Points)-2], Point{100, 25}) {
		t.Fatal("negative sweep", back.Points)
	}
	arc := b.strokes[0].path
	if len(b.strokes) != 1 || !near(arc.Points[0], Point{0, 25}) || !near(last(arc), Point{0, 25}) || len(arc.Points) != 13 || arc.Verbs[len(arc.Verbs)-1] == PathClose {
		t.Fatal("full arc", len(b.strokes), arc)
	}
	ellipse := b.fills[2].path
	if !near(ellipse.Points[0], Point{30, 15}) || b.fills[2].rule != NonZero {
		t.Fatal("ellipse", ellipse.Points)
	}
	// Lines, polygons and Béziers.
	b = shapes(plusRec(PlusDrawLinesRecord, 1, dwords(3), fl(0, 0, 10, 0, 10, 10)),
		plusRec(PlusDrawLinesRecord, 0x2000|1, dwords(3), fl(0, 0, 10, 0, 10, 10)),
		plusRec(PlusFillPolygonRecord, 0x8000, dwords(0xff000000, 3), fl(0, 0, 10, 0, 10, 10)),
		plusRec(PlusDrawBeziersRecord, 1, dwords(4), fl(0, 0, 1, 1, 2, 1, 3, 0)),
		plusRec(PlusDrawLinesRecord, 0x4000|1, dwords(2), words(-1, 2, 3, 4)))
	if b.strokes[0].path.Verbs[2] != PathLineTo || len(b.strokes[0].path.Verbs) != 3 || b.strokes[1].path.Verbs[3] != PathClose {
		t.Fatal("lines", b.strokes[0].path.Verbs, b.strokes[1].path.Verbs)
	}
	if b.fills[0].rule != EvenOdd || b.fills[0].path.Verbs[3] != PathClose {
		t.Fatal("polygon", b.fills[0])
	}
	if bz := b.strokes[2].path; len(bz.Verbs) != 2 || bz.Verbs[1] != PathCubicTo || !near(bz.Points[3], Point{3, 0}) {
		t.Fatal("beziers", bz)
	}
	if !pointsNear(b.strokes[3].path.Points, Point{-1, 2}, Point{3, 4}) {
		t.Fatal("compressed points", b.strokes[3].path.Points)
	}
	// Cardinal splines: tension 0.5 puts control points a sixth of the
	// neighboring chord from each point; open curves repeat end points.
	b = shapes(plusRec(PlusDrawCurveRecord, 1, fl(0.5), dwords(0, 2, 3), fl(0, 0, 12, 0, 12, 12)),
		plusRec(PlusDrawCurveRecord, 1, fl(0.5), dwords(1, 1, 3), fl(0, 0, 12, 0, 12, 12)),
		plusRec(PlusDrawClosedCurveRecord, 1, fl(1), dwords(3), fl(0, 0, 12, 0, 12, 12)),
		plusRec(PlusFillClosedCurveRecord, 0x8000|0x2000, dwords(0xff000000), fl(0), dwords(3), fl(0, 0, 12, 0, 12, 12)),
		plusRec(PlusFillClosedCurveRecord, 0x8000, dwords(0xff000000), fl(0), dwords(3), fl(0, 0, 12, 0, 12, 12)))
	if !pointsNear(b.strokes[0].path.Points, Point{0, 0}, Point{2, 0}, Point{10, -2}, Point{12, 0}, Point{14, 2}, Point{12, 10}, Point{12, 12}) {
		t.Fatal("open curve", b.strokes[0].path.Points)
	}
	if !pointsNear(b.strokes[1].path.Points, Point{12, 0}, Point{14, 2}, Point{12, 10}, Point{12, 12}) {
		t.Fatal("curve segment", b.strokes[1].path.Points)
	}
	// Closed, tension 1: the first control point is P0 + (P1 - P2)/3.
	closed := b.strokes[2].path
	if !near(closed.Points[1], Point{0, -4}) || len(closed.Points) != 10 || closed.Verbs[len(closed.Verbs)-1] != PathClose {
		t.Fatal("closed curve", closed.Points)
	}
	if b.fills[0].rule != NonZero || b.fills[1].rule != EvenOdd || !near(b.fills[0].path.Points[1], Point{0, 0}) {
		t.Fatal("closed curve fill rules", b.fills[0].rule, b.fills[1].rule)
	}
	// Paths: Bézier and line types with close flags; fills close all
	// figures, strokes keep open figures open.
	obj := plusObj(2, 3, plusPathObj([]float64{0, 0, 10, 0, 10, 10, 20, 20, 30, 20, 30, 30, 40, 30, 40, 40}, []byte{0, 1, 0x81, 0, 3, 3, 3, 1}))
	b = shapes(obj, plusRec(PlusFillPathRecord, 0x8000|2, dwords(0xff000000)), plusRec(PlusDrawPathRecord, 2, dwords(1)))
	fill, line := b.fills[0].path, b.strokes[0].path
	want := []PathVerb{PathMoveTo, PathLineTo, PathLineTo, PathClose, PathMoveTo, PathCubicTo, PathLineTo}
	if b.fills[0].rule != EvenOdd || len(fill.Verbs) != 8 || fill.Verbs[7] != PathClose || len(line.Verbs) != 7 {
		t.Fatal("path", fill.Verbs, line.Verbs)
	}
	for i, v := range want {
		if line.Verbs[i] != v {
			t.Fatal("path verbs", line.Verbs)
		}
	}
}

func TestPlayEMFPlusBrushes(t *testing.T) {
	fillWith := func(t *testing.T, brush []byte, records ...[]byte) (Paint, []string) {
		t.Helper()
		all := append([][]byte{plusRec(PlusSetPixelOffsetModeRecord, 4), plusObj(5, 1, brush)}, records...)
		all = append(all, plusRec(PlusFillRectsRecord, 0, dwords(5, 1), fl(0, 0, 10, 10)))
		b, skipped := plusPlay(t, plusScene(96, 64, all...), PlayOptions{})
		if len(b.fills) == 0 {
			return Paint{}, skipped
		}
		return b.fills[0].paint, skipped
	}
	param := func(g *LinearGradient, x, y float64) float64 { return g.Transform.Apply(Point{x, y}).X }
	rect := [4]float64{10, 0, 100, 50}
	p, _ := fillWith(t, linearBrush(0, 0, rect, 0xffff0000, 0xff0000ff))
	g := p.Gradient
	if p.Kind != PaintLinearGradient || math.Abs(param(g, 10, 7)) > 1e-9 || math.Abs(param(g, 60, 0)-0.5) > 1e-9 || math.Abs(param(g, 110, 30)-1) > 1e-9 || len(g.Stops) != 2 || g.Stops[1].Color != (color.NRGBA{0, 0, 255, 255}) || g.Wrap != WrapTile {
		t.Fatalf("gradient %+v", g)
	}
	// The brush transform places the brush rectangle in world space and the
	// world transform maps it to the device.
	p, _ = fillWith(t, linearBrush(2, 1, rect, 0xffff0000, 0xff0000ff, fl(0, 1, -1, 0, 0, 0)), plusRec(PlusSetWorldTransformRecord, 0, fl(2, 0, 0, 2, 0, 0)))
	if g := p.Gradient; math.Abs(param(g, 0, 20)) > 1e-9 || math.Abs(param(g, 0, 220)-1) > 1e-9 || math.Abs(param(g, 50, 120)-0.5) > 1e-9 || g.Wrap != WrapTileFlipX {
		t.Fatalf("transformed gradient %+v", g)
	}
	p, _ = fillWith(t, linearBrush(4, 0, rect, 0, 0, dwords(3), fl(0, 0.25, 1), dwords(0xff000000, 0xffffffff, 0xff00ff00)))
	if s := p.Gradient.Stops; len(s) != 3 || s[1].Offset != 0.25 || s[1].Color != (color.NRGBA{255, 255, 255, 255}) || s[2].Color != (color.NRGBA{0, 255, 0, 255}) {
		t.Fatalf("preset stops %+v", s)
	}
	p, _ = fillWith(t, linearBrush(8, 0, rect, 0xff000000, 0xffc8c8c8, dwords(3), fl(0, 0.5, 1), fl(0, 0.25, 1)))
	if s := p.Gradient.Stops; len(s) != 3 || s[1].Offset != 0.5 || s[1].Color != (color.NRGBA{50, 50, 50, 255}) {
		t.Fatalf("blend stops %+v", s)
	}
	// A texture brush maps image pixels through the brush and world
	// transforms.
	texture := cat(dwords(plusVersion, 2, 2, 3), fl(2, 0, 0, 2, 1, 1), argbImage(2, 1, 0xffff0000, 0x8000ff00))
	p, _ = fillWith(t, texture, plusRec(PlusSetWorldTransformRecord, 0, fl(1, 0, 0, 1, 10, 0)))
	if p.Kind != PaintPattern || p.Wrap != WrapTileFlipXY || p.PatternTransform != (Matrix{M11: 2, M22: 2, Dx: 11, Dy: 1}) || p.Pattern.Bounds().Dx() != 2 {
		t.Fatalf("texture %+v", p)
	}
	if c := color.NRGBAModel.Convert(p.Pattern.At(1, 0)).(color.NRGBA); c != (color.NRGBA{0, 255, 0, 128}) {
		t.Fatal("texture pixel", c)
	}
	for _, c := range []struct {
		brush  []byte
		reason string
	}{
		{linearBrush(0x10, 0, rect, 0, 0, dwords(2), fl(0, 1), fl(0, 1)), "vertical blend"},
		{linearBrush(0x80, 0, rect, 0xff000000, 0xffffffff), "gamma"},
		{linearBrush(0, 4, rect, 0xff000000, 0xffffffff), "clamped"},
		{linearBrush(0, 0, rect, 0xff000000, 0x00ffffff), "varying alpha"},
	} {
		if _, skipped := fillWith(t, c.brush); len(skipped) != 1 || !strings.Contains(skipped[0], c.reason) {
			t.Errorf("%s: %v", c.reason, skipped)
		}
	}
	// SourceCopy only composites opaque paint faithfully.
	copyMode := plusRec(PlusSetCompositingModeRecord, 1)
	if _, skipped := fillWith(t, solidBrush(0x80000000), copyMode); len(skipped) != 1 || !strings.Contains(skipped[0], "SourceCopy") {
		t.Fatal(skipped)
	}
	if p, skipped := fillWith(t, solidBrush(0xff000000), copyMode); len(skipped) != 0 || p.Kind != PaintSolid {
		t.Fatal(skipped)
	}
	if _, skipped := fillWith(t, texture, copyMode); len(skipped) != 1 {
		t.Fatal("translucent texture under SourceCopy", skipped)
	}
	// A pen's texture decodes once per pen definition.
	pen := plusObj(2, 2, plusPen(0, 0, 1, nil, texture))
	line := plusRec(PlusDrawLinesRecord, 2, dwords(2), fl(0, 0, 10, 0))
	o := PlayOptions{Destination: Box{Width: 96, Height: 64}, MaxImagePixels: 2}
	if _, err := Play(plusScene(96, 64, pen, line, line, line), o, &recordingBackend{}); err != nil {
		t.Fatal("pen texture charged per stroke:", err)
	}
	if _, err := Play(plusScene(96, 64, pen, line, pen, line), o, &recordingBackend{}); !errors.Is(err, ErrLimit) {
		t.Fatal("a redefined pen is charged again:", err)
	}
}

func TestPlayEMFPlusImages(t *testing.T) {
	img := plusObj(4, 5, argbImage(2, 2, 0xffff0000, 0xff00ff00, 0xff0000ff, 0x80000000))
	half := plusRec(PlusSetPixelOffsetModeRecord, 4)
	draw := plusRec(PlusDrawImageRecord, 4, dwords(0xffffffff, 2), fl(0, 0, 2, 2), fl(10, 10, 20, 40))
	b, skipped := plusPlay(t, plusScene(96, 64, half, img, draw, plusRec(PlusSetInterpolationModeRecord, 5), draw), PlayOptions{})
	if len(skipped) != 0 || len(b.images) != 2 {
		t.Fatal(skipped, b.images)
	}
	d := b.images[0].draw
	if d.Transform != (Matrix{M11: 10, M22: 20, Dx: 10, Dy: 10}) || d.Source != d.Image.Bounds() || !d.Smooth || d.Opacity != 1 || b.images[1].draw.Smooth {
		t.Fatalf("draw image %+v", d)
	}
	// Parallelogram placement of a source subrectangle.
	points := plusRec(PlusDrawImagePointsRecord, 4, dwords(0xffffffff, 2), fl(1, 0, 1, 2), dwords(3), fl(10, 10, 20, 15, 5, 30))
	b, _ = plusPlay(t, plusScene(96, 64, half, img, points), PlayOptions{})
	d = b.images[0].draw
	if d.Source.Min.X != 1 || d.Source.Dx() != 1 || d.Source.Dy() != 2 || !pointsNear([]Point{d.Transform.Apply(Point{1, 0}), d.Transform.Apply(Point{2, 0}), d.Transform.Apply(Point{1, 2})}, Point{10, 10}, Point{20, 15}, Point{5, 30}) {
		t.Fatalf("draw image points %+v", d)
	}
	// Source pixels outside the bitmap come from image attributes.
	outside := plusRec(PlusDrawImageRecord, 4, dwords(0xffffffff, 2), fl(0, 0, 4, 2), fl(10, 10, 20, 40))
	_, skipped = plusPlay(t, plusScene(96, 64, img, outside), PlayOptions{})
	if len(skipped) != 1 || !strings.Contains(skipped[0], "outside") {
		t.Fatal(skipped)
	}
	clamp := plusObj(7, 8, dwords(plusVersion, 0, 4, 0x00ffffff, 0, 0))
	attributed := plusRec(PlusDrawImageRecord, 4, dwords(7, 2), fl(0, 0, 4, 2), fl(10, 10, 20, 40))
	b, skipped = plusPlay(t, plusScene(96, 64, img, clamp, attributed), PlayOptions{})
	if len(skipped) != 0 || b.images[0].draw.Source.Dx() != 2 {
		t.Fatal("transparent clamp", skipped)
	}
	// A fractional source (an EmfPlusRectF) is drawn from the enclosing
	// pixels, mapped exactly, with a clip layer for the exact rectangle.
	if len(b.images[0].clip) != 0 {
		t.Fatal("whole-pixel source clipped", b.images[0].clip)
	}
	for _, c := range []struct {
		src, dst [4]float64
		attrs    uint32
		source   image.Rectangle
		area     []Point
	}{
		{[4]float64{0.5, 0, 1, 1}, [4]float64{0, 0, 1, 1}, 0xffffffff, image.Rect(0, 0, 2, 1), []Point{{0, 0}, {1, 0}, {1, 1}, {0, 1}}},
		{[4]float64{0, 0, 1.5, 2}, [4]float64{10, 10, 30, 40}, 0xffffffff, image.Rect(0, 0, 2, 2), []Point{{10, 10}, {40, 10}, {40, 50}, {10, 50}}},
		{[4]float64{0.25, 0.5, 1.5, 1}, [4]float64{0, 0, 60, 40}, 0xffffffff, image.Rect(0, 0, 2, 2), []Point{{0, 0}, {60, 0}, {60, 40}, {0, 40}}},
		// Partly outside the bitmap under transparent clamping: the layer
		// stops at the bitmap's edge.
		{[4]float64{1.5, -0.5, 1, 2}, [4]float64{0, 0, 20, 40}, 7, image.Rect(1, 0, 2, 2), []Point{{0, 10}, {10, 10}, {10, 40}, {0, 40}}},
	} {
		draw := plusRec(PlusDrawImageRecord, 4, dwords(c.attrs, 2), fl(c.src[:]...), fl(c.dst[:]...))
		b, skipped = plusPlay(t, plusScene(96, 64, half, img, clamp, draw), PlayOptions{})
		if len(skipped) != 0 || len(b.images) != 1 {
			t.Fatal(c.src, skipped)
		}
		d, clip := b.images[0].draw, b.images[0].clip
		sx, sy := c.dst[2]/c.src[2], c.dst[3]/c.src[3]
		want := Matrix{M11: sx, M22: sy, Dx: c.dst[0] - c.src[0]*sx, Dy: c.dst[1] - c.src[1]*sy}
		if d.Source != c.source || math.Abs(d.Transform.M11-want.M11) > 1e-9 || math.Abs(d.Transform.M22-want.M22) > 1e-9 || math.Abs(d.Transform.Dx-want.Dx) > 1e-9 || math.Abs(d.Transform.Dy-want.Dy) > 1e-9 {
			t.Fatalf("%v: %+v", c.src, d)
		}
		if len(clip) != 1 || clip[0].Op != ClipReplace || clip[0].Base != nil || !pointsNear(clip[0].Area.Points, c.area...) {
			t.Fatalf("%v: clip %+v", c.src, clip)
		}
	}
	// The pixel budget is charged once per image object.
	data := plusScene(96, 64, img, draw, draw, draw)
	if _, err := Play(data, PlayOptions{Destination: Box{Width: 96, Height: 64}, MaxImagePixels: 4}, &recordingBackend{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Play(data, PlayOptions{Destination: Box{Width: 96, Height: 64}, MaxImagePixels: 3}, &recordingBackend{}); !errors.Is(err, ErrLimit) {
		t.Fatal("pixel budget:", err)
	}
	// Redefining the slot invalidates the cached bitmap.
	small := plusRec(PlusDrawImageRecord, 4, dwords(0xffffffff, 2), fl(0, 0, 1, 1), fl(10, 10, 20, 40))
	b, skipped = plusPlay(t, plusScene(96, 64, img, draw, plusObj(4, 5, argbImage(1, 1, 0xff123456)), small), PlayOptions{})
	if len(skipped) != 0 || len(b.images) != 2 || b.images[1].draw.Image.Bounds().Dx() != 1 {
		t.Fatal("stale image cache", skipped, len(b.images))
	}
	if _, err := Play(plusScene(96, 64, img, draw, plusObj(4, 5, argbImage(1, 1, 0xff123456)), small), PlayOptions{Destination: Box{Width: 96, Height: 64}, MaxImagePixels: 4}, &recordingBackend{}); !errors.Is(err, ErrLimit) {
		t.Fatal("a redefined image is charged again:", err)
	}
}

func TestPlayEMFPlusGetDCAndFallback(t *testing.T) {
	pen := emfSelect(0x80000008) // NULL_PEN
	rect := func(x int32) []byte { return emfBox(EMRRectangle, x, 0, x+10, 10) }
	all := cat(plusComment(plusHeaderDPI(true, 96, 96)), pen, rect(0),
		plusComment(plusRec(PlusGetDCRecord, 0)), rect(20),
		plusComment(fillRect(0xff000000, 40, 0, 10, 10)), rect(60),
		plusComment(plusRec(PlusEndOfFileRecord, 0)))
	data := emfScene(96, 64, 1, emfSplit(all)...)
	b, skipped := plusPlay(t, data, PlayOptions{})
	// The pen selection precedes GetDC, so the GetDC rectangle uses the
	// default black pen; its fill is the default white brush.
	if len(skipped) != 0 || len(b.fills) != 2 || b.fills[0].path.Points[0].X != 20 || b.fills[1].path.Points[0].X != 40.5 {
		t.Fatal("EMF+ with GetDC", skipped, b.fills)
	}
	o := PlayOptions{}
	o.Stream.PreferGDI = true
	b, _ = plusPlay(t, data, o)
	if len(b.fills) != 3 || b.fills[0].path.Points[0].X != 0 || b.fills[2].path.Points[0].X != 60 {
		t.Fatal("GDI fallback", b.fills)
	}
}

func TestPlayEMFPlusUnsupported(t *testing.T) {
	font := plusObj(1, 6, dwords(plusVersion), fl(12), dwords(2, 0, 0, 1), words('A', 0))
	text := plusRec(PlusDrawStringRecord, 0x8001, dwords(0xff000000, 0xffffffff, 1), fl(0, 0, 50, 20), words('A', 0))
	b, skipped := plusPlay(t, plusScene(96, 64, font, text, fillRect(0xff000000, 0, 0, 1, 1)), PlayOptions{})
	if len(skipped) != 1 || skipped[0] != "EMF+ DrawString layout" || len(b.fills) != 1 {
		t.Fatal(skipped)
	}
	if _, err := Play(plusScene(96, 64, font, text), PlayOptions{Destination: Box{Width: 96, Height: 64}}, &recordingBackend{}); !errors.Is(err, ErrUnsupported) {
		t.Fatal("unsupported must stop playback by default:", err)
	}
	_, skipped = plusPlay(t, plusScene(96, 64, plusRec(PlusClearRecord, 0, dwords(0x80ffffff))), PlayOptions{})
	if len(skipped) != 1 || !strings.Contains(skipped[0], "Clear") {
		t.Fatal(skipped)
	}
}

func plusFontObj(id uint8, em float64, unit, style uint32, family string) []byte {
	units := utf16.Encode([]rune(family))
	name := make([]byte, 2*len(units))
	for i, u := range units {
		put16(name, 2*i, u)
	}
	return plusObj(id, 6, dwords(plusVersion), fl(em), dwords(unit, style, 0, uint32(len(units))), name)
}

// driverString writes EmfPlusDrawDriverString with a solid color; positions
// holds x, y pairs and matrix, when non-nil, the six transform elements.
func driverString(font uint8, argb, options uint32, text []uint16, positions, matrix []float64) []byte {
	glyphs := make([]byte, 2*len(text))
	for i, u := range text {
		put16(glyphs, 2*i, u)
	}
	present, m := uint32(0), []byte(nil)
	if matrix != nil {
		present, m = 1, fl(matrix...)
	}
	return plusRec(PlusDrawDriverStringRecord, 0x8000|uint16(font), dwords(argb, options, present, uint32(len(text))), glyphs, fl(positions...), m)
}

func TestPlayEMFPlusDriverString(t *testing.T) {
	half := plusRec(PlusSetPixelOffsetModeRecord, 4)
	ab := utf16.Encode([]rune("AB"))
	play := func(t *testing.T, records ...[]byte) (*fakeText, []string) {
		t.Helper()
		var skipped []string
		b := &fakeText{}
		o := PlayOptions{Destination: Box{Width: 96, Height: 64}, Unsupported: func(u UnsupportedOperation) error {
			skipped = append(skipped, u.Reason)
			return nil
		}}
		if _, err := Play(plusScene(96, 64, append([][]byte{half}, records...)...), o, b); err != nil {
			t.Fatal(err)
		}
		if b.recordingBackend.bad != nil {
			t.Fatal(b.recordingBackend.bad)
		}
		return b, skipped
	}
	// Bold underlined 16-pixel Liberation Sans, code units at explicit
	// baseline origins.
	b, skipped := play(t, plusFontObj(1, 16, 2, 1|4, "Liberation Sans"), driverString(1, 0xff102030, 1, ab, []float64{10, 20, 25, 21}, nil))
	if len(skipped) != 0 || len(b.drawn) != 1 {
		t.Fatal(skipped, b.drawn)
	}
	run := b.drawn[0]
	f := run.Font
	if f.FaceName != "Liberation Sans" || f.Height != -16 || f.Weight != 700 || !f.Underline || f.Italic || f.StrikeOut || f.CharSet != 1 {
		t.Fatalf("font %+v", f)
	}
	if run.Glyphs || !pointsNear(run.Origins, Point{10, 20}, Point{25, 21}) || run.Advances[0] != 15 || run.Advances[1] != 8 || run.Transform != Identity() || run.Paint.Color != (color.NRGBA{0x10, 0x20, 0x30, 0xff}) {
		t.Fatalf("run %+v", run)
	}
	// Glyph indexes, and realized advances from the first position.
	b, _ = play(t, plusFontObj(1, 16, 2, 2|8, "F"), driverString(1, 0xff000000, 4, []uint16{36, 37, 38}, []float64{10, 20}, nil))
	run = b.drawn[0]
	if !run.Glyphs || !run.Font.Italic || !run.Font.StrikeOut || run.Font.Weight != 400 || !pointsNear(run.Origins, Point{10, 20}, Point{18, 20}, Point{26, 20}) {
		t.Fatalf("realized advance %+v", run)
	}
	// Physical font sizes convert through the header's vertical DPI (120)
	// into page units; World sizes are world units. The world transform
	// applies to the text space either way.
	scale := plusRec(PlusSetWorldTransformRecord, 0, fl(2, 0, 0, 2, 0, 0))
	for _, c := range []struct {
		records []byte
		height  float64
	}{
		{plusFontObj(1, 12, 3, 0, "F"), -20},
		{cat(plusFontObj(1, 12, 3, 0, "F"), plusRec(PlusSetPageTransformRecord, 4, fl(2))), -20. / 240},
		{cat(plusFontObj(1, 1, 6, 0, "F"), plusRec(PlusSetPageTransformRecord, 6, fl(0.5))), -2},
		{plusFontObj(1, 7, 0, 0, "F"), -7},
	} {
		recs := append(emfSplitPlus(c.records), scale, driverString(1, 0xff000000, 1, ab, []float64{0, 0, 1, 0}, nil))
		b, skipped = play(t, recs...)
		if len(skipped) != 0 || math.Abs(b.drawn[0].Font.Height-c.height) > 1e-9 || b.drawn[0].Transform.M11 == 1 {
			t.Errorf("font height %v, want %v (%v)", b.drawn[0].Font.Height, c.height, skipped)
		}
	}
	// A translation matrix moves the run; other matrices are reported.
	b, _ = play(t, plusFontObj(1, 16, 2, 0, "F"), driverString(1, 0xff000000, 1, ab, []float64{10, 20, 25, 20}, []float64{1, 0, 0, 1, 5, -3}))
	if !pointsNear([]Point{b.drawn[0].Transform.Apply(b.drawn[0].Origins[0])}, Point{15, 17}) {
		t.Fatal("translated run", b.drawn[0].Transform)
	}
	// The translation is in world space, before the world transform.
	b, _ = play(t, plusFontObj(1, 16, 2, 0, "F"), scale, driverString(1, 0xff000000, 1, ab, []float64{10, 20, 25, 20}, []float64{1, 0, 0, 1, 5, -3}))
	if !pointsNear([]Point{b.drawn[0].Transform.Apply(b.drawn[0].Origins[0])}, Point{30, 34}) {
		t.Fatal("translated run under a world transform", b.drawn[0].Transform)
	}
	// A brush object paints the text.
	b, _ = play(t, plusFontObj(1, 16, 2, 0, "F"), plusObj(2, 1, linearBrush(0, 0, [4]float64{0, 0, 10, 10}, 0xff000000, 0xffffffff)), plusRec(PlusDrawDriverStringRecord, 1, dwords(2, 1, 0, 1), words('A'), fl(1, 1)))
	if b.drawn[0].Paint.Kind != PaintLinearGradient {
		t.Fatal("text brush", b.drawn[0].Paint)
	}
	for _, c := range []struct {
		records [][]byte
		reason  string
	}{
		{[][]byte{plusFontObj(1, 16, 2, 0, "F"), driverString(1, 0xff000000, 1, ab, []float64{0, 0, 1, 0}, []float64{2, 0, 0, 1, 0, 0})}, "transform other than a translation"},
		{[][]byte{plusFontObj(1, 16, 2, 0, "F"), driverString(1, 0xff000000, 1|2, ab, []float64{0, 0, 1, 0}, nil)}, "vertical"},
		{[][]byte{plusFontObj(1, 16, 1, 0, "F"), driverString(1, 0xff000000, 1, ab, []float64{0, 0, 1, 0}, nil)}, "font size unit"},
		{[][]byte{plusFontObj(1, 16, 2, 0, "F"), plusRec(PlusDrawStringRecord, 0x8001, dwords(0xff000000, 0xffffffff, 1), fl(0, 0, 50, 20), words('A', 0))}, "DrawString"},
	} {
		if _, skipped := play(t, c.records...); len(skipped) != 1 || !strings.Contains(skipped[0], c.reason) {
			t.Errorf("%s: %v", c.reason, skipped)
		}
	}
	// Without a text backend, text is reported.
	_, skipped = plusPlay(t, plusScene(96, 64, plusFontObj(1, 16, 2, 0, "F"), driverString(1, 0xff000000, 1, ab, []float64{0, 0, 1, 0}, nil)), PlayOptions{})
	if len(skipped) != 1 || skipped[0] != "text output" {
		t.Fatal(skipped)
	}
}

// emfSplitPlus splits concatenated EMF+ records.
func emfSplitPlus(blob []byte) [][]byte {
	var out [][]byte
	for len(blob) >= 12 {
		n := int(u32(blob[4:]))
		out = append(out, blob[:n])
		blob = blob[n:]
	}
	return out
}

// pathGradientBrush writes EmfPlusPathGradientBrushData with a path boundary
// (BrushDataPath) or, when boundary holds no path, boundary points.
func pathGradientBrush(flags, wrap, center uint32, cx, cy float64, surround []uint32, path []byte, points []float64, optional ...[]byte) []byte {
	b := cat(dwords(plusVersion, 3, flags, wrap, center), fl(cx, cy), dwords(uint32(len(surround))), dwords(surround...))
	if flags&1 != 0 {
		b = cat(b, dwords(uint32(len(path))), path)
	} else {
		b = cat(b, dwords(uint32(len(points)/2)), fl(points...))
	}
	return cat(b, cat(optional...))
}

type meshRecorder struct {
	recordingBackend
	meshes [][]GradientTriangle
	clips  []Clip
}

func (b *meshRecorder) FillGradient(mesh []GradientTriangle, clip Clip) error {
	b.note(checkClip(clip))
	b.meshes = append(b.meshes, mesh)
	b.clips = append(b.clips, clip)
	return nil
}

func TestPlayEMFPlusPathGradient(t *testing.T) {
	half := plusRec(PlusSetPixelOffsetModeRecord, 4)
	square := plusPathObj([]float64{0, 0, 40, 0, 40, 40, 0, 40}, []byte{0, 1, 1, 0x81})
	fill := plusRec(PlusFillRectsRecord, 0, dwords(1, 1), fl(0, 0, 60, 40))
	play := func(t *testing.T, brush []byte, records ...[]byte) (*meshRecorder, []string) {
		t.Helper()
		var skipped []string
		b := &meshRecorder{}
		o := PlayOptions{Destination: Box{Width: 96, Height: 64}, Unsupported: func(u UnsupportedOperation) error {
			skipped = append(skipped, u.Reason)
			return nil
		}}
		all := append([][]byte{half, plusObj(1, 1, brush)}, records...)
		if _, err := Play(plusScene(96, 64, append(all, fill)...), o, b); err != nil {
			t.Fatal(err)
		}
		if b.bad != nil {
			t.Fatal(b.bad)
		}
		return b, skipped
	}
	red, blue := uint32(0xffff0000), uint32(0xff0000ff)
	b, skipped := play(t, pathGradientBrush(1, 4, red, 20, 20, []uint32{blue}, square, nil))
	if len(skipped) != 0 || len(b.meshes) != 1 || len(b.meshes[0]) != 4 {
		t.Fatal(skipped, b.meshes)
	}
	tri := b.meshes[0][0]
	if tri.Points != [3]Point{{20, 20}, {0, 0}, {40, 0}} || tri.Colors[0] != (color.NRGBA64{0xffff, 0, 0, 0xffff}) || tri.Colors[1] != (color.NRGBA64{0, 0, 0xffff, 0xffff}) {
		t.Fatalf("fan triangle %+v", tri)
	}
	// The filled shape becomes the last clip layer.
	if c := b.clips[0]; len(c) != 1 || c[0].Op != ClipReplace || !pointsNear(c[0].Area.Points[2:3], Point{60, 40}) {
		t.Fatal("fill clip", c)
	}
	// Per-vertex colors, and the brush transform before the world.
	b, _ = play(t, pathGradientBrush(1|2, 4, red, 20, 20, []uint32{blue, blue, red, red}, square, nil, fl(1, 0, 0, 1, 10, 0)), plusRec(PlusSetWorldTransformRecord, 0, fl(1, 0, 0, 1, 0, 5)))
	if tri := b.meshes[0][1]; tri.Points != [3]Point{{30, 25}, {50, 5}, {50, 45}} || tri.Colors[1] != (color.NRGBA64{0, 0, 0xffff, 0xffff}) || tri.Colors[2] != (color.NRGBA64{0xffff, 0, 0, 0xffff}) {
		t.Fatalf("vertex colors %+v", tri)
	}
	// Rendered: red at the center, mostly blue near the boundary, nothing
	// outside the boundary within the filled rectangle.
	rb := newRasterBackend(96, 64)
	data := plusScene(96, 64, half, plusObj(1, 1, pathGradientBrush(1, 4, red, 20, 20, []uint32{blue}, square, nil)), fill)
	if _, err := Play(data, PlayOptions{Destination: Box{Width: 96, Height: 64}}, rb); err != nil {
		t.Fatal(err)
	}
	if c := rb.canvas.NRGBAAt(19, 19); c.R < 240 || c.B > 15 {
		t.Error("center", c)
	}
	// Pixel (1,20) is 18.5 of 20 units from the center toward the edge.
	if c := rb.canvas.NRGBAAt(1, 19); math.Abs(float64(c.B)-0.925*255) > 3 || math.Abs(float64(c.R)-0.075*255) > 3 {
		t.Error("near boundary", c)
	}
	if c := rb.canvas.NRGBAAt(50, 20); c != (color.NRGBA{255, 255, 255, 255}) {
		t.Error("outside the boundary", c)
	}
	twoFigures := plusPathObj([]float64{0, 0, 40, 0, 40, 40, 50, 50, 60, 50, 60, 60}, []byte{0, 1, 0x81, 0, 1, 0x81})
	for _, c := range []struct {
		brush  []byte
		reason string
	}{
		{pathGradientBrush(0, 4, red, 20, 20, []uint32{blue}, nil, []float64{0, 0, 40, 0, 40, 40}), "point (cardinal spline) boundary"},
		{pathGradientBrush(1|8, 4, red, 20, 20, []uint32{blue}, square, nil, dwords(2), fl(0, 1, 0, 1)), "blend factors"},
		{pathGradientBrush(1|4, 4, red, 20, 20, []uint32{blue}, square, nil, dwords(2), fl(0, 1), dwords(red, blue)), "preset colors"},
		{pathGradientBrush(1|0x40, 4, red, 20, 20, []uint32{blue}, square, nil, dwords(2), fl(.5, .5)), "focus scales"},
		{pathGradientBrush(1|0x80, 4, red, 20, 20, []uint32{blue}, square, nil), "gamma"},
		{pathGradientBrush(1, 0, red, 20, 20, []uint32{blue}, square, nil), "tiled"},
		{pathGradientBrush(1, 4, red, 50, 20, []uint32{blue}, square, nil), "star-shaped"},
		{pathGradientBrush(1, 4, red, 40, 20, []uint32{blue}, square, nil), "star-shaped"},
		{pathGradientBrush(1, 4, red, 20, 20, []uint32{0x800000ff}, square, nil), "varying alpha"},
		{pathGradientBrush(1, 4, red, 20, 20, []uint32{blue, red}, square, nil), "surrounding colors"},
		{pathGradientBrush(1, 4, red, 20, 20, []uint32{blue}, twoFigures, nil), "several figures"},
		// Winding once around the center, but notched so that the edge
		// from (25,25) to (35,25) faces away from it.
		{pathGradientBrush(1, 4, red, 20, 20, []uint32{blue}, plusPathObj([]float64{0, 0, 25, 0, 25, 25, 35, 25, 35, 0, 40, 0, 40, 40, 0, 40}, []byte{0, 1, 1, 1, 1, 1, 1, 0x81}), nil), "star-shaped"},
		// Consistent edge orientation, but winding twice around the center.
		{pathGradientBrush(1, 4, red, 20, 20, []uint32{blue}, plusPathObj([]float64{0, 0, 40, 0, 40, 40, 0, 40, 0, 0, 40, 0, 40, 40, 0, 40}, []byte{0, 1, 1, 1, 1, 1, 1, 0x81}), nil), "star-shaped"},
	} {
		if _, skipped := play(t, c.brush); len(skipped) != 1 || !strings.Contains(skipped[0], c.reason) {
			t.Errorf("%s: %v", c.reason, skipped)
		}
	}
	// Mesh triangles share the playback budget.
	data = plusScene(96, 64, half, plusObj(1, 1, pathGradientBrush(1, 4, red, 20, 20, []uint32{blue}, square, nil)), fill)
	if _, err := Play(data, PlayOptions{Destination: Box{Width: 96, Height: 64}, MaxImagePixels: 3}, &meshRecorder{}); !errors.Is(err, ErrLimit) {
		t.Fatal("mesh budget:", err)
	}
	// Curved boundaries are flattened; one surrounding color applies.
	circle := plusPathObj([]float64{40, 20, 40, 31, 31, 40, 20, 40, 9, 40, 0, 31, 0, 20, 0, 9, 9, 0, 20, 0, 31, 0, 40, 9, 40, 20}, []byte{0, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 0x83})
	b, skipped = play(t, pathGradientBrush(1, 4, red, 20, 20, []uint32{blue}, circle, nil))
	if len(skipped) != 0 || len(b.meshes[0]) < 16 {
		t.Fatal("curved boundary", skipped, len(b.meshes))
	}
	// Without a GradientBackend, and for pens, path gradients are reported.
	_, skipped = plusPlay(t, plusScene(96, 64, plusObj(1, 1, pathGradientBrush(1, 4, red, 20, 20, []uint32{blue}, square, nil)), fill), PlayOptions{})
	if len(skipped) != 1 || !strings.Contains(skipped[0], "GradientBackend") {
		t.Fatal(skipped)
	}
	_, skipped = plusPlay(t, plusScene(96, 64, plusObj(2, 2, plusPen(0, 0, 1, nil, pathGradientBrush(1, 4, red, 20, 20, []uint32{blue}, square, nil))), plusRec(PlusDrawLinesRecord, 2, dwords(2), fl(0, 0, 9, 9))), PlayOptions{})
	if len(skipped) != 1 || !strings.Contains(skipped[0], "path gradient") {
		t.Fatal(skipped)
	}
}

// pathCapObj is an EmfPlusCustomLineCap with default data: fill (flags 1)
// or line (flags 2) paths, base cap, inset and width scale.
func pathCapObj(flags, baseCap uint32, inset, scale float64, fill, line []byte) []byte {
	b := cat(dwords(plusVersion, 0, flags, baseCap), fl(inset), dwords(0, 0, 0), fl(10, scale), fl(0, 0, 0, 0))
	if flags&1 != 0 {
		b = cat(b, dwords(uint32(len(fill))), fill)
	}
	if flags&2 != 0 {
		b = cat(b, dwords(uint32(len(line))), line)
	}
	return b
}

func arrowCapObj(w, h, inset float64, filled uint32) []byte {
	return cat(dwords(plusVersion, 1), fl(w, h, inset), dwords(filled, 0, 0, 0), fl(10, 1), fl(0, 0, 0, 0))
}

// capPen is a width-2 pen whose start and end caps are given; a non-nil
// custom cap object sets LineCapTypeCustom on that end.
func capPen(id uint8, argb, startCap, endCap uint32, start, end []byte) []byte {
	flags := uint32(2 | 4)
	optional := dwords(startCap, endCap)
	if start != nil {
		flags |= 0x800
		optional = cat(optional, dwords(uint32(len(start))), start)
	}
	if end != nil {
		flags |= 0x1000
		optional = cat(optional, dwords(uint32(len(end))), end)
	}
	return plusObj(id, 2, plusPen(flags, 0, 2, optional, solidBrush(argb)))
}

func TestPlayEMFPlusCustomCaps(t *testing.T) {
	half := plusRec(PlusSetPixelOffsetModeRecord, 4)
	triangle := plusPathObj([]float64{-1, 0, 1, 0, 0, 2}, []byte{0, 1, 0x81})
	line := plusRec(PlusDrawLinesRecord, 1, dwords(2), fl(10, 10, 50, 10))
	on := PlayOptions{CustomLineCaps: true}
	play := func(t *testing.T, o PlayOptions, records ...[]byte) (*recordingBackend, []string) {
		t.Helper()
		return plusPlay(t, plusScene(96, 64, append([][]byte{half}, records...)...), o)
	}
	// Off by default.
	b, skipped := play(t, PlayOptions{}, capPen(1, 0xff000000, 0, 0xff, nil, pathCapObj(1, 0, 0, 1, triangle, nil)), line)
	if len(skipped) != 1 || skipped[0] != "EMF+ custom line cap" || len(b.strokes)+len(b.fills) != 0 {
		t.Fatal(skipped)
	}
	// Cap space: origin at the end, +y outward along the line, units of the
	// pen width; +x is +y turned a quarter clockwise.
	b, skipped = play(t, on, capPen(1, 0xff000000, 0xff, 0xff, pathCapObj(1, 0, 0, 1, triangle, nil), pathCapObj(1, 0, 0, 1, triangle, nil)), line)
	if len(skipped) != 0 || len(b.strokes) != 1 || len(b.fills) != 2 {
		t.Fatal(skipped, b.strokes, b.fills)
	}
	if s := b.strokes[0]; s.stroke.Cap != CapFlat || s.stroke.EndCap != 0 || !pointsNear(s.path.Points, Point{10, 10}, Point{50, 10}) {
		t.Fatalf("capped line %+v", s)
	}
	if !pointsNear(b.fills[0].path.Points, Point{10, 8}, Point{10, 12}, Point{6, 10}) || b.fills[0].rule != NonZero {
		t.Fatalf("start cap %v", b.fills[0].path.Points)
	}
	if !pointsNear(b.fills[1].path.Points, Point{50, 12}, Point{50, 8}, Point{54, 10}) {
		t.Fatalf("end cap %v", b.fills[1].path.Points)
	}
	// BaseInset shortens the line; BaseCap ends it; WidthScale scales.
	b, _ = play(t, on, capPen(1, 0xff000000, 0, 0xff, nil, pathCapObj(1, 2, 1, 2, triangle, nil)), line)
	if s := b.strokes[0]; s.stroke.EndCap != CapRound || !pointsNear(s.path.Points, Point{10, 10}, Point{46, 10}) || !pointsNear(b.fills[0].path.Points[2:], Point{58, 10}) {
		t.Fatalf("inset cap %+v %v", s, b.fills[0].path.Points)
	}
	// A line path is stroked with the pen; it wins over a fill path.
	b, _ = play(t, on, capPen(1, 0xff000000, 0, 0xff, nil, pathCapObj(3, 0, 0, 1, triangle, plusPathObj([]float64{0, 0, 0, 3}, []byte{0, 1}))), line)
	if len(b.fills) != 0 || len(b.strokes) != 2 || !pointsNear(b.strokes[1].path.Points, Point{50, 10}, Point{56, 10}) || b.strokes[1].stroke.Width != 2 {
		t.Fatalf("line-path cap %+v", b.strokes)
	}
	// Adjustable arrow: vertex at the end, base 3x2 widths back, its midpoint
	// pulled 0.5 widths forward; the line stops at that midpoint.
	b, _ = play(t, on, capPen(1, 0xff000000, 0, 0xff, nil, arrowCapObj(3, 2, .5, 1)), line)
	if !pointsNear(b.strokes[0].path.Points, Point{10, 10}, Point{47, 10}) || !pointsNear(b.fills[0].path.Points, Point{50, 10}, Point{46, 7}, Point{47, 10}, Point{46, 13}) {
		t.Fatalf("arrow %v %v", b.strokes[0].path.Points, b.fills[0].path.Points)
	}
	b, _ = play(t, on, capPen(1, 0xff000000, 0, 0xff, nil, arrowCapObj(3, 2, 0, 0)), line)
	if len(b.fills) != 0 || len(b.strokes) != 2 || b.strokes[1].path.Verbs[len(b.strokes[1].path.Verbs)-1] != PathClose {
		t.Fatal("open arrow outline", b.strokes)
	}
	// Closed figures have no caps.
	b, _ = play(t, on, capPen(1, 0xff000000, 0, 0xff, nil, arrowCapObj(3, 2, 0, 1)), plusRec(PlusDrawRectsRecord, 1, dwords(1), fl(0, 0, 10, 10)))
	if len(b.fills) != 0 || len(b.strokes) != 1 {
		t.Fatal("closed figure caps", b.fills)
	}
	for _, c := range []struct {
		records [][]byte
		reason  string
	}{
		{[][]byte{capPen(1, 0x80000000, 0, 0xff, nil, arrowCapObj(3, 2, 0, 1)), line}, "translucent"},
		{[][]byte{capPen(1, 0xff000000, 0, 0xff, nil, nil), line}, "without LineCapTypeCustom"},
		{[][]byte{capPen(1, 0xff000000, 0, 0, nil, arrowCapObj(3, 2, 0, 1)), line}, "without LineCapTypeCustom"},
		{[][]byte{capPen(1, 0xff000000, 0, 0xff, nil, pathCapObj(1, 3, 0, 1, triangle, nil)), line}, "base cap"},
		{[][]byte{capPen(1, 0xff000000, 0, 0xff, nil, pathCapObj(1, 0, 30, 1, triangle, nil)), line}, "inset beyond"},
		{[][]byte{capPen(1, 0xff000000, 0, 0xff, nil, arrowCapObj(3, 2, 0, 1)), plusRec(PlusDrawBeziersRecord, 1, dwords(4), fl(0, 0, 10, 0, 20, 0, 30, 0))}, "inset beyond"},
		{[][]byte{plusObj(1, 2, plusPen(4|32|0x1000, 0, 2, cat(dwords(0xff, 1), dwords(uint32(len(arrowCapObj(3, 2, 0, 1)))), arrowCapObj(3, 2, 0, 1)), solidBrush(0xff000000))), line}, "dashed"},
	} {
		if _, skipped := play(t, on, c.records...); len(skipped) != 1 || !strings.Contains(skipped[0], c.reason) {
			t.Errorf("%s: %v", c.reason, skipped)
		}
	}
}
