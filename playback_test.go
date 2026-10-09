package gowemf

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"testing"
)

// recordingBackend keeps every operation and checks the geometry contract:
// paths start with PathMoveTo, verbs and points agree, and all coordinates,
// transforms and opacities are finite and in range.
type recordingBackend struct {
	fills   []recordedFill
	strokes []recordedStroke
	images  []recordedImage
	bad     error
}
type recordedFill struct {
	path  Path
	rule  FillRule
	paint Paint
	clip  Clip
}
type recordedStroke struct {
	path   Path
	stroke Stroke
	clip   Clip
}
type recordedImage struct {
	draw ImageDraw
	clip Clip
}

func checkPath(p Path) error {
	n := 0
	for i, v := range p.Verbs {
		if i == 0 && v != PathMoveTo {
			return errors.New("path does not start with PathMoveTo")
		}
		switch v {
		case PathMoveTo, PathLineTo:
			n++
		case PathCubicTo:
			n += 3
		case PathClose:
		default:
			return fmt.Errorf("path verb %d", v)
		}
	}
	if n != len(p.Points) {
		return fmt.Errorf("path verbs consume %d points, have %d", n, len(p.Points))
	}
	for _, q := range p.Points {
		if !finite(q.X) || !finite(q.Y) {
			return fmt.Errorf("non-finite path point %v", q)
		}
	}
	return nil
}

func checkClip(c Clip) error {
	for _, r := range c {
		for ; r != nil; r = r.Base {
			if r.Op < ClipIntersect || r.Op > ClipComplement {
				return fmt.Errorf("clip op %d", r.Op)
			}
			if r.Operand != nil {
				if err := checkClip(Clip{r.Operand}); err != nil {
					return err
				}
			} else if r.Op != ClipOffset {
				if err := checkPath(r.Area); err != nil {
					return err
				}
			} else if r.Base == nil || !finite(r.Offset.X) || !finite(r.Offset.Y) {
				return errors.New("invalid clip offset")
			}
		}
	}
	return nil
}

// checkPaint enforces the Paint contract for each kind.
func checkPaint(p Paint) error {
	switch p.Kind {
	case PaintSolid:
	case PaintHatch:
		if p.Hatch > 5 || !p.PatternTransform.Finite() {
			return fmt.Errorf("hatch paint %+v", p)
		}
	case PaintPattern:
		if p.Pattern == nil || p.Pattern.Bounds().Empty() || !p.PatternTransform.Finite() || p.Wrap > WrapClamp {
			return fmt.Errorf("pattern paint %+v", p)
		}
	case PaintLinearGradient:
		g := p.Gradient
		if g == nil || !g.Transform.Finite() || g.Wrap >= WrapClamp || len(g.Stops) < 2 || g.Stops[0].Offset != 0 || g.Stops[len(g.Stops)-1].Offset != 1 {
			return fmt.Errorf("gradient paint %+v", g)
		}
		for i, s := range g.Stops {
			if (i > 0 && s.Offset < g.Stops[i-1].Offset) || s.Color.A != g.Stops[0].Color.A {
				return fmt.Errorf("gradient stops %+v", g.Stops)
			}
		}
	default:
		return fmt.Errorf("paint kind %d", p.Kind)
	}
	return nil
}

func (b *recordingBackend) note(err error) {
	if err != nil && b.bad == nil {
		b.bad = err
	}
}

func (b *recordingBackend) FillPath(path Path, rule FillRule, paint Paint, clip Clip) error {
	b.note(checkPath(path))
	b.note(checkClip(clip))
	if rule != EvenOdd && rule != NonZero {
		b.note(fmt.Errorf("fill rule %d", rule))
	}
	b.note(checkPaint(paint))
	b.fills = append(b.fills, recordedFill{path, rule, paint, clip})
	return nil
}
func (b *recordingBackend) StrokePath(path Path, s Stroke, clip Clip) error {
	b.note(checkPath(path))
	b.note(checkClip(clip))
	if !s.Transform.Finite() || !finite(s.Width) || s.Width < 0 || !finite(s.DashOffset) || (s.Dash != DashUser && len(s.Dashes) > 0) {
		b.note(fmt.Errorf("stroke %+v", s))
	}
	for _, d := range s.Dashes {
		if !finite(d) || d < 0 {
			b.note(fmt.Errorf("dash lengths %v", s.Dashes))
		}
	}
	b.note(checkPaint(s.Paint))
	if s.Gap != nil {
		b.note(checkPaint(*s.Gap))
	}
	b.strokes = append(b.strokes, recordedStroke{path, s, clip})
	return nil
}
func (b *recordingBackend) DrawImage(d ImageDraw, clip Clip) error {
	b.note(checkClip(clip))
	if !d.Transform.Finite() || !(d.Opacity >= 0 && d.Opacity <= 1) || !d.Source.In(d.Image.Bounds()) || d.Source.Empty() {
		b.note(fmt.Errorf("image draw %+v", d))
	}
	b.images = append(b.images, recordedImage{d, clip})
	return nil
}

func record(t *testing.T, data []byte, o PlayOptions) *recordingBackend {
	t.Helper()
	if o.Destination == (Box{}) {
		o.Destination = Box{Width: 96, Height: 64}
	}
	b := &recordingBackend{}
	if _, err := Play(data, o, b); err != nil {
		t.Fatal(err)
	}
	if b.bad != nil {
		t.Fatal(b.bad)
	}
	return b
}

// emfSplit splits concatenated EMF records so the header counts each one.
func emfSplit(blob []byte) [][]byte {
	var out [][]byte
	for len(blob) >= 8 {
		n := int(u32(blob[4:]))
		out = append(out, blob[:n])
		blob = blob[n:]
	}
	return out
}

func near(a, b Point) bool { return math.Abs(a.X-b.X) < 1e-6 && math.Abs(a.Y-b.Y) < 1e-6 }

func TestPlayRejectsInvalidOptions(t *testing.T) {
	data := emfScene(96, 64, 1)
	if _, err := Play(data, PlayOptions{Destination: Box{Width: 1, Height: 1}}, nil); err == nil {
		t.Fatal("nil backend accepted")
	}
	for _, d := range []Box{{}, {Width: -1, Height: 1}, {Width: math.Inf(1), Height: 1}, {X: math.NaN(), Width: 1, Height: 1}} {
		if _, err := Play(data, PlayOptions{Destination: d}, &recordingBackend{}); err == nil {
			t.Fatal("destination accepted:", d)
		}
	}
	// A WMF without a placeable header has no picture frame unless the
	// caller supplies one.
	plain := wmfDocument(1, wmfBox(MetaRectangle, 0, 0, 10, 10))
	if _, err := Play(plain, PlayOptions{Destination: Box{Width: 10, Height: 10}}, &recordingBackend{}); !errors.Is(err, ErrUnsupported) {
		t.Fatal("WMF without bounds:", err)
	}
	b := record(t, plain, PlayOptions{Destination: Box{Width: 20, Height: 20}, Placeable: &PlaceableHeader{Bounds: Rect{0, 0, 10, 10}, UnitsPerInch: 96}})
	// Device units are destination units; the right/bottom edge is excluded.
	if len(b.fills) != 1 || !near(b.fills[0].path.Points[2], Point{19, 19}) {
		t.Fatal("caller-supplied bounds were not mapped onto the destination", b.fills)
	}
}

func TestPlayReportsUnsupportedOperations(t *testing.T) {
	text := emfRecord(EMRExtTextOutW, cat(longs(0, 0, 0, 0, 1), fl(1, 1), longs(5, 5, 1, 60, 0x100, 0), []byte{'A', 0}))
	data := emfScene(96, 64, 2, text, emfBox(EMRRectangle, 0, 0, 10, 10))
	_, err := Play(data, PlayOptions{Destination: Box{Width: 96, Height: 64}}, &recordingBackend{})
	var pe *ParseError
	if !errors.Is(err, ErrUnsupported) || !errors.As(err, &pe) || pe.Offset != 88 {
		t.Fatal("text output must stop playback by default:", err)
	}
	var skipped []UnsupportedOperation
	b := record(t, data, PlayOptions{Unsupported: func(u UnsupportedOperation) error {
		skipped = append(skipped, u)
		return nil
	}})
	if len(skipped) != 1 || skipped[0].Reason != "text output" || skipped[0].Source.Type != EMRExtTextOutW {
		t.Fatal(skipped)
	}
	if len(b.fills) != 1 {
		t.Fatal("playback did not continue after an accepted omission")
	}
	stop := errors.New("stop")
	if _, err := Play(data, PlayOptions{Destination: Box{Width: 1, Height: 1}, Unsupported: func(UnsupportedOperation) error { return stop }}, &recordingBackend{}); err != stop {
		t.Fatal("callback error was not returned unchanged:", err)
	}
	// Every unimplemented drawing family is reported, never silently dropped.
	for name, rec := range map[string][]byte{
		"flood fill":       emfRecord(EMRExtFloodFill, longs(1, 1, 0, 0)),
		"gradient fill":    emfRecord(EMRGradientFill, longs(0, 0, 0, 0, 0, 0, 0)),
		"region inversion": emfRecord(EMRInvertRgn, cat(longs(0, 0, 0, 0, 32), longs(32, 1, 0, 0, 0, 0, 0, 0))),
		"WidenPath":        cat(emfEmpty(EMRBeginPath), emfEmpty(EMREndPath), emfEmpty(EMRWidenPath)),
		"ROP2":             cat(emfValue(EMRSetROP2, 7), emfBox(EMRRectangle, 0, 0, 1, 1)),
		"layout":           cat(emfValue(EMRSetLayout, 1), emfBox(EMRRectangle, 0, 0, 1, 1)),
		"COLORREF":         cat(emfBrush(1, 0, 0x01000003), emfSelect(1), emfBox(EMRRectangle, 0, 0, 1, 1)),
		"raster op":        emfBlt(EMRBitBlt, 0, 0, 4, 4, 0x00660046, 0, 0, 0, 0, nil, nil),
		"hatch":            cat(emfRecord(EMRCreateBrushIndirect, longs(1, 2, 0, 7)), emfSelect(1), emfBox(EMRRectangle, 0, 0, 1, 1)),
		"monochrome": func() []byte {
			i, b := sceneDIB(2, 2, false, quadrantsN(2, 2)) // 24-bit, not monochrome
			return cat(emfRecord(EMRCreateMonoBrush, cat(longs(1, 0, 32, int32(len(i)), int32(32+len(i)), int32(len(b))), i, b)), emfSelect(1), emfBox(EMRRectangle, 0, 0, 1, 1))
		}(),
		"ICM":    cat(colorSpaceRecord(false).Raw, emfValue(EMRSetColorSpace, 1), emfValue(EMRSetICMMode, 2), emfBox(EMRRectangle, 0, 0, 1, 1)),
		"masked": maskBltFixture(true),
		"partial scans": func() []byte {
			i, b := sceneDIB(4, 4, false, quadrantsN(4, 4))
			return emfSetDIBitsToDevice(0, 0, 0, 0, 4, 4, 2, i, b)
		}(),
		"palette colors": func() []byte {
			i, b := sceneDIB(4, 4, false, quadrantsN(4, 4))
			r := emfStretchDIBits(0, 0, 4, 4, 0, 0, 4, 4, 0x00cc0020, i, b)
			put32(r, 64, 1)
			return r
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			var reasons []string
			record(t, emfScene(96, 64, 2, emfSplit(rec)...), PlayOptions{Unsupported: func(u UnsupportedOperation) error {
				reasons = append(reasons, u.Reason)
				return nil
			}})
			if len(reasons) != 1 {
				t.Fatal(reasons)
			}
		})
	}
}

func maskBltFixture(mask bool) []byte {
	info, bits := sceneDIB(4, 4, false, quadrantsN(4, 4))
	const fixed = 120
	head := cat(longs(0, 0, 3, 3, 0, 0, 4, 4, 0x00cc0020, 0, 0), fl(1, 0, 0, 1, 0, 0), longs(0, 0, 8+fixed, int32(len(info)), int32(8+fixed+len(info)), int32(len(bits)), 0, 0, 0))
	if mask {
		head = cat(head, longs(int32(8+fixed), int32(len(info)), int32(8+fixed+len(info)), int32(len(bits))))
	} else {
		head = cat(head, longs(0, 0, 0, 0))
	}
	return emfRecord(EMRMaskBlt, cat(head, info, bits))
}

func TestPlayWMFDeviceSourceTransfers(t *testing.T) {
	// The no-bitmap DibBitBlt form is a pattern operation for PATCOPY and a
	// device-to-device copy for SRCCOPY.
	patcopy := testRecord(WMF, MetaDIBBitBlt, 0, cat(longs(0x00f00021), words(0, 0, 0, 8, 8, 2, 2)))
	srccopy := testRecord(WMF, MetaDIBBitBlt, 0, cat(longs(0x00cc0020), words(0, 0, 0, 8, 8, 2, 2)))
	var reasons []string
	b := record(t, wmfScene(96, 64, 1, patcopy, srccopy), PlayOptions{Unsupported: func(u UnsupportedOperation) error {
		reasons = append(reasons, u.Reason)
		return nil
	}})
	if len(b.fills) != 1 || b.fills[0].paint.Color != cWhite || len(reasons) != 1 || reasons[0] != "device-to-device bitmap transfer" {
		t.Fatal(b.fills, reasons)
	}
	// LogPen style bits select caps and joins in WMF pens too.
	b = record(t, wmfScene(96, 64, 1, wmfPen(0x0200|0x1000, 4, 0), wmfRec(MetaSelectObject, 0), wmfPoly(MetaPolyline, 0, 0, 10, 0)), PlayOptions{})
	if s := b.strokes[0].stroke; s.Cap != CapFlat || s.Join != JoinBevel {
		t.Fatalf("WMF pen style bits: %+v", s)
	}
}

func TestPlayMaskBltWithoutMaskCopiesSource(t *testing.T) {
	b := record(t, emfScene(96, 64, 1, maskBltFixture(false)), PlayOptions{})
	if len(b.images) != 1 || b.images[0].draw.Source != image.Rect(0, 0, 4, 4) {
		t.Fatal(b.images)
	}
}

func TestPlayResourceLimits(t *testing.T) {
	poly := emfPoints(EMRPolygon, 0, 0, 10, 0, 10, 10, 0, 10)
	if _, err := Play(emfScene(96, 64, 1, poly), PlayOptions{Destination: Box{Width: 1, Height: 1}, MaxPathPoints: 3}, &recordingBackend{}); !errors.Is(err, ErrLimit) {
		t.Fatal("path point limit:", err)
	}
	var clips [][]byte
	for i := 0; i < 5; i++ {
		clips = append(clips, emfBox(EMRIntersectClipRect, 0, 0, 50, 50))
	}
	if _, err := Play(emfScene(96, 64, 1, clips...), PlayOptions{Destination: Box{Width: 1, Height: 1}, MaxClipSteps: 4}, &recordingBackend{}); !errors.Is(err, ErrLimit) {
		t.Fatal("clip step limit:", err)
	}
	// Metaregion steps count toward the same budget.
	meta := emfScene(96, 64, 1, clips[0], clips[1], emfEmpty(EMRSetMetaRgn), clips[2], clips[3])
	if _, err := Play(meta, PlayOptions{Destination: Box{Width: 1, Height: 1}, MaxClipSteps: 3}, &recordingBackend{}); !errors.Is(err, ErrLimit) {
		t.Fatal("metaregion steps escaped the budget:", err)
	}
	info, bits := sceneDIB(16, 8, false, quadrants)
	blit := emfStretchDIBits(0, 0, 16, 8, 0, 0, 16, 8, 0x00cc0020, info, bits)
	if _, err := Play(emfScene(96, 64, 1, blit, blit), PlayOptions{Destination: Box{Width: 1, Height: 1}, MaxImagePixels: 200}, &recordingBackend{}); !errors.Is(err, ErrLimit) {
		t.Fatal("cumulative bitmap pixel limit:", err)
	}
	record(t, emfScene(96, 64, 1, blit), PlayOptions{MaxImagePixels: 128})
}

func TestPlayResolvesPens(t *testing.T) {
	line := emfPoints(EMRPolyline, 0, 0, 10, 0)
	// LogPen: geometric with round caps/joins, width scaled by the logical
	// x-axis under GM_COMPATIBLE (MS-WMF 3.1.4.2), here x0.5 and y x2.
	aniso := []byte{}
	for _, r := range [][]byte{emfValue(EMRSetMapMode, 8), emfPoint(EMRSetWindowExtEx, 2, 1), emfPoint(EMRSetViewportExtEx, 1, 2)} {
		aniso = append(aniso, r...)
	}
	b := record(t, emfScene(96, 64, 4, append(emfSplit(aniso), emfPen(1, 0, 8, blue), emfSelect(1), line)...), PlayOptions{})
	s := b.strokes[0].stroke
	if s.Hairline || s.Width != 4 || s.Transform != Identity() || s.Cap != CapRound || s.Join != JoinRound || s.Dash != DashSolid || s.Paint.Color != cBlue {
		t.Fatalf("LogPen: %+v", s)
	}
	if s.PixelCenter != (Point{.5, .5}) {
		t.Fatal("pixel center", s.PixelCenter)
	}
	// ExtCreatePen style bits and a GM_ADVANCED world transform.
	world := emfWorld(Matrix{M11: 2, M22: 3})
	b = record(t, emfScene(96, 64, 4, world, emfExtPen(1, 0x10000|0x200|0x1000, 5, red), emfSelect(1), line), PlayOptions{})
	s = b.strokes[0].stroke
	if s.Width != 5 || s.Transform != (Matrix{M11: 2, M22: 3}) || s.Cap != CapFlat || s.Join != JoinBevel {
		t.Fatalf("ExtCreatePen: %+v", s)
	}
	// Zero-width and cosmetic pens are hairlines.
	for _, pen := range [][]byte{emfPen(1, 0, 0, red), emfExtPen(1, 0x200, 9, red)} {
		b = record(t, emfScene(96, 64, 4, pen, emfSelect(1), line), PlayOptions{})
		if !b.strokes[0].stroke.Hairline {
			t.Fatal("hairline", b.strokes[0].stroke)
		}
	}
	// A user style copies its dashes before scaling them, keeps them in pen
	// units, and receives the opaque background color for gaps.
	user := emfRecord(EMRExtCreatePen, longs(1, 0, 0, 0, 0, 0x10000|7, 4, 0, red, 0, 2, 3, 5))
	b = record(t, emfScene(96, 64, 4, append(emfSplit(aniso), user, emfSelect(1), emfValue(EMRSetBkColor, green), line, line)...), PlayOptions{})
	for _, op := range b.strokes {
		s := op.stroke
		if s.Dash != DashUser || len(s.Dashes) != 2 || s.Dashes[0] != 1.5 || s.Dashes[1] != 2.5 || s.Gap == nil || s.Gap.Color != cGreen {
			t.Fatalf("user style: %+v", s)
		}
	}
	b = record(t, emfScene(96, 64, 4, emfValue(EMRSetBkMode, 1), emfPen(1, 2, 0, red), emfSelect(1), line), PlayOptions{})
	if s := b.strokes[0].stroke; s.Dash != DashDot || s.Gap != nil {
		t.Fatalf("transparent dotted pen: %+v", s)
	}
	// EMF reference pixels scale with the destination.
	b = record(t, emfScene(96, 64, 4, emfPen(1, 0, 3, red), emfSelect(1), line), PlayOptions{Destination: Box{Width: 192, Height: 32}})
	if s := b.strokes[0].stroke; s.PixelCenter != (Point{1, .25}) || s.Width != 6 {
		t.Fatalf("scaled destination: %+v", s)
	}
}

func TestPlayAppliesROP2(t *testing.T) {
	box := emfBox(EMRRectangle, 0, 0, 10, 10)
	paint := func(rop int32) *recordingBackend {
		return record(t, emfScene(96, 64, 4, emfSelect(nullPen), emfBrush(1, 2, 0x204060), emfSelect(1), emfValue(EMRSetBkColor, 0x0a0b0c), emfValue(EMRSetROP2, rop), box), PlayOptions{})
	}
	if b := paint(11); len(b.fills) != 0 {
		t.Fatal("R2_NOP drew", b.fills)
	}
	p := paint(4).fills[0].paint
	if p.Kind != PaintHatch || p.Color != (color.NRGBA{0x9f, 0xbf, 0xdf, 255}) || *p.Background != (color.NRGBA{0xf3, 0xf4, 0xf5, 255}) {
		t.Fatalf("R2_NOTCOPYPEN: %+v %v", p, p.Background)
	}
	p = paint(1).fills[0].paint
	if p.Color != cBlack || *p.Background != cBlack {
		t.Fatalf("R2_BLACK: %+v", p)
	}
	if p := paint(16).fills[0].paint; p.Color != cWhite {
		t.Fatalf("R2_WHITE: %+v", p)
	}
}

func TestPlayResolvesBrushes(t *testing.T) {
	box := emfBox(EMRRectangle, 0, 0, 10, 10)
	b := record(t, emfScene(96, 64, 4, emfSelect(nullPen), emfRecord(EMRCreateBrushIndirect, longs(1, 2, red, 3)), emfSelect(1), emfPoint(EMRSetBrushOrgEx, 3, 4), emfValue(EMRSetBkMode, 1), box), PlayOptions{Destination: Box{Width: 192, Height: 128}})
	p := b.fills[0].paint
	if p.Kind != PaintHatch || p.Hatch != 3 || p.Background != nil || p.PatternTransform != (Matrix{M11: 2, M22: 2, Dx: 6, Dy: 8}) {
		t.Fatalf("hatch: %+v", p)
	}
	info, bits := sceneDIB(2, 2, true, quadrantsN(2, 2))
	pattern := emfRecord(EMRCreateDIBPatternBrushPT, cat(longs(1, 0, 32, int32(len(info)), int32(32+len(info)), int32(len(bits))), info, bits))
	b = record(t, emfScene(96, 64, 4, emfSelect(nullPen), pattern, emfSelect(1), box, box), PlayOptions{MaxImagePixels: 4})
	p = b.fills[0].paint
	if p.Kind != PaintPattern || p.Pattern.Bounds() != image.Rect(0, 0, 2, 2) || color.NRGBAModel.Convert(p.Pattern.At(1, 1)) != qYellow {
		t.Fatalf("pattern: %+v", p)
	}
	if b.fills[1].paint.Pattern == nil {
		t.Fatal("decoded pattern was not reused")
	}
	b = record(t, emfScene(96, 64, 4, emfSelect(nullPen), emfSelect(nullBrush), box), PlayOptions{})
	if len(b.fills) != 0 {
		t.Fatal("NULL_BRUSH filled")
	}
}

// verbs renders a path's verbs compactly, e.g. "MLLC" with Z for close.
func verbs(p Path) string {
	var sb strings.Builder
	for _, v := range p.Verbs {
		sb.WriteByte(" MLCZ"[v])
	}
	return sb.String()
}

func TestPlayBuildsPaths(t *testing.T) {
	// PolyDraw: move, line, Bézier closed by its last point, then a line
	// that starts a new figure at the current position.
	draw := emfRecord(EMRPolyDraw, cat(longs(0, 0, 0, 0, 6), longs(1, 1, 5, 1, 5, 5, 3, 7, 1, 5, 9, 9), []byte{6, 2, 4, 4, 5, 2}))
	b := record(t, emfScene(96, 64, 4, draw), PlayOptions{})
	if got := verbs(b.strokes[0].path); got != "MLCZML" || !near(b.strokes[0].path.Points[5], Point{1, 5}) {
		t.Fatal(got, b.strokes[0].path.Points)
	}
	// AngleArc: a line from the current position to the arc start, then a
	// counterclockwise arc that leaves the current position at its end.
	angle := emfRecord(EMRAngleArc, cat(longs(50, 30, 10), fl(0, 90)))
	b = record(t, emfScene(96, 64, 4, emfPoint(EMRMoveToEx, 0, 30), angle, emfPoint(EMRLineTo, 0, 0)), PlayOptions{})
	arc := b.strokes[0].path
	if verbs(arc) != "MLC" || !near(arc.Points[1], Point{60, 30}) || !near(arc.Points[4], Point{50, 20}) || !near(b.strokes[1].path.Points[0], Point{50, 20}) {
		t.Fatal("AngleArc", verbs(arc), arc.Points, b.strokes[1].path.Points)
	}
	// FlattenPath leaves only line segments; StrokePath keeps figures open.
	b = record(t, emfScene(96, 64, 4, emfEmpty(EMRBeginPath), emfBox(EMREllipse, 0, 0, 40, 40), emfEmpty(EMREndPath), emfEmpty(EMRFlattenPath), emfBox(EMRStrokePath, 0, 0, 0, 0)), PlayOptions{})
	if strings.ContainsAny(verbs(b.strokes[0].path), "C") || !strings.HasSuffix(verbs(b.strokes[0].path), "Z") {
		t.Fatal("flattened ellipse", verbs(b.strokes[0].path))
	}
	// Rectangles follow the arc direction; under GM_COMPATIBLE the right and
	// bottom device edges are excluded (MS-EMF 2.1.16).
	for dir, want := range map[int32][]Point{1: {{0, 0}, {0, 9}, {9, 9}, {9, 0}}, 2: {{0, 0}, {9, 0}, {9, 9}, {0, 9}}} {
		b = record(t, emfScene(96, 64, 4, emfValue(EMRSetArcDirection, dir), emfBox(EMRRectangle, 0, 0, 10, 10)), PlayOptions{})
		for i, q := range want {
			if !near(b.fills[0].path.Points[i], q) {
				t.Fatal("rectangle direction", dir, b.fills[0].path.Points)
			}
		}
	}
	// GM_ADVANCED includes the edges.
	b = record(t, emfScene(96, 64, 4, emfWorld(Matrix{M11: 1, M22: 1, Dx: 1}), emfBox(EMRRectangle, 0, 0, 10, 10)), PlayOptions{})
	if !near(b.fills[0].path.Points[2], Point{11, 10}) {
		t.Fatal("advanced rectangle", b.fills[0].path.Points)
	}
	// PS_INSIDEFRAME shrinks the box by the device pen width.
	b = record(t, emfScene(96, 64, 4, emfPen(1, 6, 4, red), emfSelect(1), emfBox(EMRRectangle, 0, 0, 21, 21)), PlayOptions{})
	if !near(b.fills[0].path.Points[0], Point{2, 2}) || !near(b.fills[0].path.Points[2], Point{18, 18}) {
		t.Fatal("inside frame", b.fills[0].path.Points)
	}
	// A path survives transform changes after it is recorded, and an aborted
	// path draws nothing.
	b = record(t, emfScene(96, 64, 4, emfEmpty(EMRBeginPath), emfBox(EMRRectangle, 0, 0, 11, 11), emfEmpty(EMREndPath), emfWorld(Matrix{M11: 3, M22: 3}), emfBox(EMRFillPath, 0, 0, 0, 0), emfEmpty(EMRBeginPath), emfBox(EMRRectangle, 0, 0, 5, 5), emfEmpty(EMRAbortPath)), PlayOptions{})
	if len(b.fills) != 1 || !near(b.fills[0].path.Points[2], Point{10, 10}) {
		t.Fatal("path bracket", b.fills)
	}
}

func TestPlayEmptyOpaqueTextFillsBackground(t *testing.T) {
	text := emfRecord(EMRExtTextOutW, cat(longs(0, 0, 0, 0, 1), fl(1, 1), longs(5, 5, 0, 0, 2, 4, 6, 20, 30, 0)))
	b := record(t, emfScene(96, 64, 2, emfValue(EMRSetBkColor, green), text), PlayOptions{})
	if len(b.fills) != 1 || b.fills[0].paint.Color != cGreen || !near(b.fills[0].path.Points[2], Point{20, 30}) {
		t.Fatal(b.fills)
	}
	// Inside a path bracket the opaque box is reported, and SetPixel, which
	// is not a path operation, still draws.
	var reasons []string
	b = record(t, emfScene(96, 64, 2, emfEmpty(EMRBeginPath), text, emfRecord(EMRSetPixelV, longs(3, 4, red)), emfEmpty(EMRAbortPath)), PlayOptions{Unsupported: func(u UnsupportedOperation) error {
		reasons = append(reasons, u.Reason)
		return nil
	}})
	if len(reasons) != 1 || len(b.fills) != 1 || b.fills[0].paint.Color != cRed || !near(b.fills[0].path.Points[0], Point{3, 4}) {
		t.Fatal(reasons, b.fills)
	}
}

func TestPlayPlacesBitmaps(t *testing.T) {
	info, bits := sceneDIB(16, 8, false, quadrants)
	// PlgBlt maps the source's upper-left, upper-right and lower-left corners.
	const fixed = 132
	plg := emfRecord(EMRPlgBlt, cat(longs(0, 0, 0, 0, 10, 20, 42, 28, 2, 36, 0, 0, 16, 8), fl(1, 0, 0, 1, 0, 0), longs(0, 0, 8+fixed, int32(len(info)), int32(8+fixed+len(info)), int32(len(bits)), 0, 0, 0, 0, 0, 0, 0), info, bits))
	b := record(t, emfScene(96, 64, 1, plg), PlayOptions{})
	m := b.images[0].draw.Transform
	if !near(m.Apply(Point{}), Point{10, 20}) || !near(m.Apply(Point{16, 0}), Point{42, 28}) || !near(m.Apply(Point{0, 8}), Point{2, 36}) {
		t.Fatal("PlgBlt", m)
	}
	// Source rectangles extending beyond the bitmap are clipped to it, and a
	// HALFTONE stretch mode requests smoothing.
	b = record(t, emfScene(96, 64, 1, emfValue(EMRSetStretchBltMode, 4), emfStretchDIBits(0, 0, 40, 20, 8, 4, 20, 10, 0x00cc0020, info, bits)), PlayOptions{})
	d := b.images[0].draw
	if d.Source != image.Rect(8, 4, 16, 8) || !d.Smooth || !near(d.Transform.Apply(Point{8, 4}), Point{0, 0}) {
		t.Fatal("clamped source", d.Source, d.Transform)
	}
	// SRCCOPY ignores DIB alpha; NOTSRCCOPY inverts; TransparentBlt keys.
	if c := color.NRGBAModel.Convert(d.Image.At(9, 5)).(color.NRGBA); c.A != 255 {
		t.Fatal("SRCCOPY alpha", c)
	}
	b = record(t, emfScene(96, 64, 1, emfBlt(EMRTransparentBlt, 0, 0, 16, 8, 0x001e1ee6, 0, 0, 16, 8, info, bits)), PlayOptions{})
	img := b.images[0].draw.Image
	if a := color.NRGBAModel.Convert(img.At(2, 2)).(color.NRGBA).A; a != 0 {
		t.Fatal("keyed pixel visible", a)
	}
	if c := color.NRGBAModel.Convert(img.At(12, 2)).(color.NRGBA); c != qGreen {
		t.Fatal("unkeyed pixel", c)
	}
	// SetDIBitsToDevice copies 1:1 device pixels from a lower-left origin.
	b = record(t, emfScene(96, 64, 1, emfValue(EMRSetMapMode, 8), emfPoint(EMRSetViewportExtEx, 4, 4), emfSetDIBitsToDevice(2, 3, 8, 0, 8, 4, 8, info, bits)), PlayOptions{})
	d = b.images[0].draw
	if d.Source != image.Rect(8, 4, 16, 8) || !near(d.Transform.Apply(Point{8, 4}), Point{8, 12}) || d.Transform.M11 != 1 {
		t.Fatal("SetDIBitsToDevice", d.Source, d.Transform)
	}
}

func TestPlayClipRegions(t *testing.T) {
	box := emfBox(EMRRectangle, 0, 0, 1, 1)
	b := record(t, emfScene(96, 64, 1,
		emfBox(EMRIntersectClipRect, 0, 0, 50, 50),
		emfPoint(EMROffsetClipRgn, 3, 4),
		emfEmpty(EMRSetMetaRgn),
		emfClipRegion(4, Rect{1, 1, 2, 2}),
		box,
		emfRecord(EMRExtSelectClipRgn, longs(0, 5)),
		box,
	), PlayOptions{Destination: Box{Width: 192, Height: 128}})
	c := b.fills[0].clip
	if len(c) != 2 || c[0].Op != ClipOffset || c[0].Offset != (Point{6, 8}) || c[0].Base.Op != ClipIntersect || c[1].Op != ClipDifference || c[1].Base != nil {
		t.Fatalf("clip chain %+v", c)
	}
	if !near(c[1].Area.Points[2], Point{4, 4}) {
		t.Fatal("region units", c[1].Area.Points)
	}
	// An omitted RGN_COPY region restores the default clip; the metaregion
	// remains.
	if c := b.fills[1].clip; len(c) != 1 || c[0] != b.fills[0].clip[0] {
		t.Fatalf("reset clip %+v", c)
	}
	// A WMF region selected with SelectObject or SelectClipRegion replaces
	// the clip; its scans are logical units.
	region := testRecord(WMF, MetaCreateRegion, 0, wmfRegionFixture())
	for _, sel := range []Record{wmfRec(MetaSelectObject, 0), wmfRec(MetaSelectClipRegion, 0)} {
		b := record(t, wmfScene(48, 32, 1, wmfBox(MetaIntersectClipRect, 0, 0, 1, 1), region, sel, wmfBox(MetaRectangle, 0, 0, 40, 30)), PlayOptions{})
		c := b.fills[0].clip
		if len(c) != 1 || c[0].Op != ClipReplace || c[0].Base != nil || !near(c[0].Area.Points[0], Point{6, 10}) || !near(c[0].Area.Points[2], Point{18, 12}) {
			t.Fatalf("WMF region clip %+v", c)
		}
	}
}

func TestPlayIsotropicAndFixedMapping(t *testing.T) {
	// MM_ISOTROPIC keeps the smaller scale, preserving signs; switching to
	// MM_ANISOTROPIC from a fixed mode retains the fixed extents.
	box := emfBox(EMRRectangle, 0, 0, 100, -100)
	b := record(t, emfScene(96, 64, 1, emfSelect(nullPen), emfValue(EMRSetMapMode, 7), emfPoint(EMRSetWindowExtEx, 100, 100), emfPoint(EMRSetViewportExtEx, 50, -20), box), PlayOptions{})
	if !near(b.fills[0].path.Points[2], Point{19, 19}) {
		t.Fatal("isotropic", b.fills[0].path.Points)
	}
	b = record(t, emfScene(96, 64, 1, emfSelect(nullPen), emfValue(EMRSetMapMode, 3), emfValue(EMRSetMapMode, 8), emfBox(EMRRectangle, 0, 0, 2500, -2500)), PlayOptions{})
	if !near(b.fills[0].path.Points[2], Point{99, 99}) {
		t.Fatal("HIMETRIC extents", b.fills[0].path.Points)
	}
}

func FuzzPlay(f *testing.F) {
	for _, s := range renderScenes() {
		f.Add(s.data)
	}
	f.Add(maskBltFixture(false))
	f.Add(emfScene(96, 64, 2, emfFont(1, -20, 300, 0, 0, "F"), emfSelect(1), emfValue(EMRSetTextAlign, 1|6), emfText(1, 4, 5, 6, &Rect{1, 2, 30, 40}, "ab c", nil, []int32{3, 4, 5, 6})))
	f.Fuzz(func(t *testing.T, data []byte) {
		b := &fakeText{}
		o := PlayOptions{
			Stream:         StreamOptions{Framing: Limits{MaxBytes: 1 << 20, MaxRecordBytes: 1 << 18, MaxRecords: 4096}, Decoding: DecodeLimits{MaxElements: 4096, MaxObjectBytes: 1 << 18}, MaxObjects: 1024, MaxSavedStates: 64, PreferGDI: true},
			Images:         ImageLimits{MaxBytes: 1 << 18, MaxPixels: 1 << 16},
			Destination:    Box{Width: 64, Height: 48},
			Placeable:      &PlaceableHeader{Bounds: Rect{0, 0, 100, 100}, UnitsPerInch: 96},
			MaxPathPoints:  1 << 14,
			MaxImagePixels: 1 << 18,
			MaxClipSteps:   64,
			Unsupported:    func(UnsupportedOperation) error { return nil },
		}
		ansi := uint8(0)
		o.DefaultCharSet = &ansi
		_, _ = Play(data, o, b)
		if b.recordingBackend.bad != nil {
			t.Fatal(b.recordingBackend.bad)
		}
		// EMF+ records, when present, play unless the GDI fallback is chosen.
		b = &fakeText{}
		o.Stream.PreferGDI = false
		_, _ = Play(data, o, b)
		if b.recordingBackend.bad != nil {
			t.Fatal(b.recordingBackend.bad)
		}
	})
}
