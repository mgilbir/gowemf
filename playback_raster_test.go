package gowemf

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"strings"
	"testing"
)

// TestRasterOperation checks the ROP3 truth-table reading of MS-WMF
// 2.1.1.31 against the Boolean formulas of named operations.
func TestRasterOperation(t *testing.T) {
	named := []struct {
		op      RasterOperation
		f       func(p, s, d uint8) uint8
		p, s, d bool
	}{
		{0x00, func(p, s, d uint8) uint8 { return 0 }, false, false, false},              // BLACKNESS
		{0x11, func(p, s, d uint8) uint8 { return ^(s | d) }, false, true, true},         // NOTSRCERASE
		{0x33, func(p, s, d uint8) uint8 { return ^s }, false, true, false},              // NOTSRCCOPY
		{0x44, func(p, s, d uint8) uint8 { return s &^ d }, false, true, true},           // SRCERASE
		{0x55, func(p, s, d uint8) uint8 { return ^d }, false, false, true},              // DSTINVERT
		{0x5a, func(p, s, d uint8) uint8 { return p ^ d }, true, false, true},            // PATINVERT
		{0x66, func(p, s, d uint8) uint8 { return s ^ d }, false, true, true},            // SRCINVERT
		{0x88, func(p, s, d uint8) uint8 { return s & d }, false, true, true},            // SRCAND
		{0xaa, func(p, s, d uint8) uint8 { return d }, false, false, true},               // DSTCOPY
		{0xbb, func(p, s, d uint8) uint8 { return ^s | d }, false, true, true},           // MERGEPAINT
		{0xc0, func(p, s, d uint8) uint8 { return p & s }, true, true, false},            // MERGECOPY
		{0xcc, func(p, s, d uint8) uint8 { return s }, false, true, false},               // SRCCOPY
		{0xee, func(p, s, d uint8) uint8 { return s | d }, false, true, true},            // SRCPAINT
		{0xf0, func(p, s, d uint8) uint8 { return p }, true, false, false},               // PATCOPY
		{0xfb, func(p, s, d uint8) uint8 { return p | ^s | d }, true, true, true},        // PATPAINT
		{0xff, func(p, s, d uint8) uint8 { return 0xff }, false, false, false},           // WHITENESS
		{0x1a, func(p, s, d uint8) uint8 { return p ^ (d | (s & p)) }, true, true, true}, // PDSPAOX
	}
	r := rand.New(rand.NewSource(1))
	for _, c := range named {
		if c.op.UsesPattern() != c.p || c.op.UsesSource() != c.s || c.op.UsesDestination() != c.d {
			t.Fatalf("%#02x uses P %v S %v D %v", uint8(c.op), c.op.UsesPattern(), c.op.UsesSource(), c.op.UsesDestination())
		}
		for i := 0; i < 1000; i++ {
			p, s, d := uint8(r.Intn(256)), uint8(r.Intn(256)), uint8(r.Intn(256))
			if got, want := c.op.Apply(p, s, d), c.f(p, s, d); got != want {
				t.Fatalf("%#02x(%#02x, %#02x, %#02x) = %#02x, want %#02x", uint8(c.op), p, s, d, got, want)
			}
		}
	}
}

// TestRasterOperationReference checks every operation through the scene
// backend, whose bitwise evaluation is written independently.
func TestRasterOperationReference(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	unit := Path{Verbs: []PathVerb{PathMoveTo, PathLineTo, PathLineTo, PathLineTo, PathClose}, Points: []Point{{0, 0}, {1, 0}, {1, 1}, {0, 1}}}
	for op := 0; op < 256; op++ {
		pc := color.NRGBA{uint8(r.Intn(256)), uint8(r.Intn(256)), uint8(r.Intn(256)), 255}
		sc := color.NRGBA{uint8(r.Intn(256)), uint8(r.Intn(256)), uint8(r.Intn(256)), 255}
		dc := color.NRGBA{uint8(r.Intn(256)), uint8(r.Intn(256)), uint8(r.Intn(256)), 255}
		rb := newRasterBackend(1, 1)
		if err := rb.FillPath(unit, NonZero, Paint{Kind: PaintSolid, Color: dc}, nil); err != nil {
			t.Fatal(err)
		}
		src := image.NewNRGBA(image.Rect(0, 0, 1, 1))
		src.SetNRGBA(0, 0, sc)
		d := RasterDraw{Operation: RasterOperation(op), Area: unit, Source: &ImageDraw{Image: src, Source: src.Bounds(), Transform: Identity(), Opacity: 1}, Pattern: &Paint{Kind: PaintSolid, Color: pc}}
		if err := rb.DrawRaster(d, nil); err != nil {
			t.Fatal(err)
		}
		got := rb.canvas.NRGBAAt(0, 0)
		want := color.NRGBA{d.Operation.Apply(pc.R, sc.R, dc.R), d.Operation.Apply(pc.G, sc.G, dc.G), d.Operation.Apply(pc.B, sc.B, dc.B), 255}
		if got != want {
			t.Fatalf("%#02x: %v, want %v", op, got, want)
		}
	}
}

type recordedRaster struct {
	draw RasterDraw
	clip Clip
}

// rasterRecorder is a recordingBackend that also implements RasterBackend.
type rasterRecorder struct {
	recordingBackend
	rasters []recordedRaster
}

func (b *rasterRecorder) DrawRaster(d RasterDraw, clip Clip) error {
	b.note(checkClip(clip))
	b.note(checkPath(d.Area))
	if d.Source != nil && (!d.Source.Transform.Finite() || !d.Source.Source.In(d.Source.Image.Bounds()) || d.Source.Source.Empty()) {
		b.note(fmt.Errorf("raster source %+v", d.Source))
	}
	b.rasters = append(b.rasters, recordedRaster{d, clip})
	return nil
}

func TestPlayRasterOperations(t *testing.T) {
	play := func(data []byte, raster bool) (*rasterRecorder, []string) {
		t.Helper()
		var skipped []string
		o := PlayOptions{Destination: Box{Width: 96, Height: 64}, Unsupported: func(u UnsupportedOperation) error {
			skipped = append(skipped, u.Reason)
			return nil
		}}
		b := &rasterRecorder{}
		var be Backend = &b.recordingBackend
		if raster {
			be = b
		}
		if _, err := Play(data, o, be); err != nil {
			t.Fatal(err)
		}
		if b.bad != nil {
			t.Fatal(b.bad)
		}
		return b, skipped
	}
	for _, wmf := range []bool{false, true} {
		data := sceneRasterSprite(wmf)
		b, skipped := play(data, true)
		if len(skipped) != 0 || len(b.rasters) != 4 || len(b.images) != 0 {
			t.Fatal(wmf, skipped, len(b.rasters))
		}
		for i, r := range b.rasters {
			d := r.draw
			want := RasterOperation(0x88)
			if i%2 == 1 {
				want = 0xee
			}
			if d.Operation != want || d.Pattern != nil || d.Source == nil || d.Source.Source != image.Rect(0, 0, 16, 16) || !isOpaque(d.Source.Image) {
				t.Fatalf("%v raster %d: %+v", wmf, i, d)
			}
		}
		first := b.rasters[0].draw
		if !pointsNear(first.Area.Points, Point{8, 8}, Point{40, 8}, Point{40, 40}, Point{8, 40}) || first.Source.Transform != (Matrix{M11: 2, M22: 2, Dx: 8, Dy: 8}) {
			t.Fatalf("%v: area %v transform %v", wmf, first.Area.Points, first.Source.Transform)
		}
		// Without a RasterBackend each pair is one image, transparent where
		// the mask is white.
		b, skipped = play(data, false)
		if len(skipped) != 0 || len(b.images) != 2 {
			t.Fatal(wmf, skipped, len(b.images))
		}
		d := b.images[0].draw
		if d.Source != image.Rect(0, 0, 16, 16) || d.Transform != (Matrix{M11: 2, M22: 2, Dx: 8, Dy: 8}) || d.Opacity != 1 || d.Smooth {
			t.Fatalf("%v sprite %+v", wmf, d)
		}
		for _, c := range []struct {
			x, y int
			want color.NRGBA
		}{{8, 8, cRed}, {4, 4, cRed}, {11, 11, cRed}, {3, 8, color.NRGBA{}}, {12, 8, color.NRGBA{}}, {0, 0, color.NRGBA{}}} {
			if got := color.NRGBAModel.Convert(d.Image.At(c.x, c.y)); got != c.want {
				t.Fatalf("%v sprite pixel (%d, %d) = %v, want %v", wmf, c.x, c.y, got, c.want)
			}
		}
	}
	// Pairs that are not a mask and its sprite are reported, in order.
	mask, sprite := spriteDIBs()
	split := func(dib []byte) ([]byte, []byte) { return dib[:40], dib[40:] }
	mi, mb := split(mask)
	si, sb := split(sprite)
	gray := cat(mi, mb)
	gray[40+100] = 0x80 // one gray mask byte
	gi, gb := split(gray)
	notBlack := cat(si, sb)
	notBlack[40] = 1 // the image is not black under a white mask pixel
	ni, nb := split(notBlack)
	and := func(x int32, info, bits []byte) []byte {
		return emfStretchDIBits(x, 8, 32, 32, 0, 0, 16, 16, codeSrcAnd, info, bits)
	}
	paint := func(x int32, info, bits []byte) []byte {
		return emfStretchDIBits(x, 8, 32, 32, 0, 0, 16, 16, codeSrcPaint, info, bits)
	}
	both := []string{"raster operation 0x88", "raster operation 0xee"}
	for _, c := range []struct {
		name    string
		records [][]byte
		want    []string
	}{
		{"gray mask", [][]byte{and(8, gi, gb), paint(8, si, sb)}, both},
		{"image not black", [][]byte{and(8, mi, mb), paint(8, ni, nb)}, both},
		{"moved", [][]byte{and(8, mi, mb), paint(9, si, sb)}, both},
		{"halftone", [][]byte{emfValue(EMRSetStretchBltMode, 4), and(8, mi, mb), paint(8, si, sb)}, both},
		{"record between", [][]byte{and(8, mi, mb), emfValue(EMRSetStretchBltMode, 3), paint(8, si, sb)}, both},
		{"clip between", [][]byte{and(8, mi, mb), emfBox(EMRIntersectClipRect, 0, 0, 20, 20), paint(8, si, sb)}, both},
		{"mask alone", [][]byte{and(8, mi, mb)}, both[:1]},
		{"image alone", [][]byte{paint(8, si, sb)}, both[1:]},
		{"two masks", [][]byte{and(8, mi, mb), and(8, mi, mb), paint(8, si, sb)}, both[:1]},
		{"mask, copy", [][]byte{and(8, mi, mb), emfStretchDIBits(8, 8, 32, 32, 0, 0, 16, 16, 0x00cc0020, si, sb)}, both[:1]},
	} {
		b, skipped := play(emfScene(96, 64, 1, c.records...), false)
		if strings.Join(skipped, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s: %q", c.name, skipped)
		}
		if drawn := len(b.images); (c.name == "two masks" || c.name == "mask, copy") != (drawn == 1) || drawn > 1 {
			t.Errorf("%s: %d images", c.name, drawn)
		}
	}
	// The EOF record reports a mask held at the end of a WMF and of the GDI
	// records of an EMF+ GetDC interval.
	wmfMask := testRecord(WMF, MetaStretchDIB, 0, cat(longs(codeSrcAnd), words(0, 16, 16, 0, 0, 32, 32, 8, 8), mask))
	getDC := cat(plusComment(plusHeaderDPI(true, 96, 96)), plusComment(plusRec(PlusGetDCRecord, 0)), and(8, mi, mb), plusComment(plusRec(PlusEndOfFileRecord, 0)))
	for name, data := range map[string][]byte{"WMF": wmfScene(96, 64, 1, wmfMask), "GetDC": emfScene(96, 64, 1, emfSplit(getDC)...)} {
		if _, skipped := play(data, false); strings.Join(skipped, ",") != "raster operation 0x88" {
			t.Errorf("%s: %q", name, skipped)
		}
	}
	// The held mask is reported before a later record draws.
	var events []string
	o := PlayOptions{Destination: Box{Width: 96, Height: 64}, Unsupported: func(u UnsupportedOperation) error {
		events = append(events, u.Reason)
		return nil
	}}
	eb := &eventBackend{events: &events}
	if _, err := Play(emfScene(96, 64, 1, and(8, mi, mb), emfBox(EMRRectangle, 0, 0, 4, 4)), o, eb); err != nil {
		t.Fatal(err)
	}
	if strings.Join(events, ",") != "raster operation 0x88,fill" {
		t.Fatal(events)
	}
	// Without an Unsupported callback, an unmatched mask stops playback.
	if _, err := Play(emfScene(96, 64, 1, and(8, mi, mb)), PlayOptions{Destination: Box{Width: 96, Height: 64}}, &recordingBackend{}); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
	b, skipped := play(sceneRasterOps(), true)
	if len(skipped) != 0 || len(b.rasters) != 3 {
		t.Fatal(skipped, len(b.rasters))
	}
	if d := b.rasters[0].draw; d.Operation != 0x66 || d.Source == nil || d.Pattern != nil {
		t.Fatalf("SRCINVERT %+v", d)
	}
	if d := b.rasters[1].draw; d.Operation != 0x55 || d.Source != nil || d.Pattern != nil || !pointsNear(d.Area.Points, Point{40, 24}, Point{56, 24}, Point{56, 32}, Point{40, 32}) {
		t.Fatalf("DSTINVERT %+v", d)
	}
	if d := b.rasters[2].draw; d.Operation != 0x5a || d.Source != nil || d.Pattern == nil || d.Pattern.Kind != PaintSolid || d.Pattern.Color != cGreen {
		t.Fatalf("PATINVERT %+v", d)
	}
	// The source is opaque, as for SRCCOPY, even when the DIB has alpha.
	clear := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	clear.SetNRGBA(1, 0, color.NRGBA{R: 200, A: 0})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, clear); err != nil {
		t.Fatal(err)
	}
	pngInfo := dibHeader(2, 1, 0, 5)
	put32(pngInfo, 20, uint32(encoded.Len()))
	b, skipped = play(emfScene(96, 64, 1, emfStretchDIBits(0, 0, 2, 1, 0, 0, 2, 1, 0x008800c6, pngInfo, encoded.Bytes())), true)
	if len(skipped) != 0 || len(b.rasters) != 1 || color.NRGBAModel.Convert(b.rasters[0].draw.Source.Image.At(1, 0)) != (color.NRGBA{R: 200, A: 255}) {
		t.Fatal("source alpha", skipped, b.rasters)
	}
	// The area and placement follow the page and world transforms.
	b, skipped = play(emfScene(96, 64, 1, emfWorld(Matrix{M11: 2, M22: 3, Dx: 1}), emfValue(EMRSetMapMode, 1), emfBlt(EMRBitBlt, 4, 4, 8, 8, 0x00550009, 0, 0, 0, 0, nil, nil),
		emfStretchDIBits(0, 0, 2, 1, 0, 0, 2, 1, 0x008800c6, pngInfo, encoded.Bytes())), true)
	if len(skipped) != 0 || len(b.rasters) != 2 || !pointsNear(b.rasters[0].draw.Area.Points, Point{9, 12}, Point{25, 12}, Point{25, 36}, Point{9, 36}) || b.rasters[1].draw.Source.Transform != (Matrix{M11: 2, M22: 3, Dx: 1}) {
		t.Fatal("transformed raster", skipped, b.rasters)
	}
	// A null brush leaves the destination alone; a hatch in TRANSPARENT
	// mode has no background bits to combine.
	patInvert := emfBlt(EMRBitBlt, 0, 0, 8, 8, 0x005a0049, 0, 0, 0, 0, nil, nil)
	b, skipped = play(emfScene(96, 64, 2, emfSelect(0x80000005), patInvert), true)
	if len(skipped) != 0 || len(b.rasters) != 0 {
		t.Fatal("null brush", skipped, len(b.rasters))
	}
	b, skipped = play(emfScene(96, 64, 2, emfBrush(1, 2, 0), emfSelect(1), emfValue(EMRSetBkMode, 1), patInvert), true)
	if len(b.rasters) != 0 || len(skipped) != 1 || !strings.Contains(skipped[0], "hatch") {
		t.Fatal("transparent hatch", skipped)
	}
	b, skipped = play(emfScene(96, 64, 2, emfBrush(1, 2, 0), emfSelect(1), emfValue(EMRSetBkMode, 2), patInvert), true)
	if len(skipped) != 0 || len(b.rasters) != 1 || b.rasters[0].draw.Pattern.Kind != PaintHatch || b.rasters[0].draw.Pattern.Background == nil {
		t.Fatal("opaque hatch", skipped)
	}
}

type eventBackend struct {
	recordingBackend
	events *[]string
}

func (b *eventBackend) FillPath(path Path, rule FillRule, paint Paint, clip Clip) error {
	*b.events = append(*b.events, "fill")
	return nil
}

// paintOnly hides the scene backend's RasterBackend, so sprites are drawn as
// masked images.
type paintOnly struct{ rb *rasterBackend }

func (b paintOnly) FillPath(path Path, rule FillRule, paint Paint, clip Clip) error {
	return b.rb.FillPath(path, rule, paint, clip)
}
func (b paintOnly) StrokePath(path Path, s Stroke, clip Clip) error {
	return b.rb.StrokePath(path, s, clip)
}
func (b paintOnly) DrawImage(d ImageDraw, clip Clip) error { return b.rb.DrawImage(d, clip) }

// TestSpriteMatchesRaster renders the sprite scenes through the bitwise
// raster operations and through merged masked images; the pixels agree.
func TestSpriteMatchesRaster(t *testing.T) {
	for _, wmf := range []bool{false, true} {
		data := sceneRasterSprite(wmf)
		raster, err := playRenderOptions(data, 96, 64, PlayOptions{})
		if err != nil {
			t.Fatal(err)
		}
		plain := paintOnly{newRasterBackend(96, 64)}
		if _, err := Play(data, PlayOptions{Destination: Box{Width: 96, Height: 64}}, plain); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(raster.Pix, plain.rb.canvas.Pix) {
			t.Fatal(wmf, "merged sprite differs from the raster operations")
		}
	}
}
