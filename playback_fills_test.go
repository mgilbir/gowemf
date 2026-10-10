package gowemf

import (
	"errors"
	"image"
	"image/color"
	"testing"
)

type gradientRecorder struct {
	recordingBackend
	meshes [][]GradientTriangle
}

func (b *gradientRecorder) FillGradient(mesh []GradientTriangle, clip Clip) error {
	b.note(checkClip(clip))
	b.meshes = append(b.meshes, append([]GradientTriangle(nil), mesh...))
	return nil
}

// emfPalette creates a logical palette from PALETTEENTRY bytes.
func emfPalette(id uint32, entries ...[4]byte) []byte {
	var e []byte
	for _, x := range entries {
		e = append(e, x[:]...)
	}
	return emfRecord(EMRCreatePalette, cat(longs(int32(id)), words(0x300, int16(len(entries))), e))
}

func reasonsOf(o *PlayOptions) *[]string {
	var reasons []string
	o.Unsupported = func(u UnsupportedOperation) error {
		reasons = append(reasons, u.Reason)
		return nil
	}
	return &reasons
}

func TestPlayPaletteColors(t *testing.T) {
	box := emfBox(EMRRectangle, 0, 0, 10, 10)
	pal := emfPalette(1, [4]byte{10, 20, 30, 0}, [4]byte{40, 50, 60, 1})
	brush := emfBrush(2, 0, 0x01000001) // PALETTEINDEX(1)
	b := record(t, emfScene(96, 64, 3, emfSelect(nullPen), pal, emfValue(EMRSelectPalette, 1), brush, emfSelect(2), box,
		emfRecord(EMRSetPaletteEntries, cat(longs(1, 1, 1), []byte{70, 80, 90, 0})), box,
		emfBrush(3, 0, 0x02112233), emfSelect(3), box), PlayOptions{})
	want := []color.NRGBA{{40, 50, 60, 255}, {70, 80, 90, 255}, {0x33, 0x22, 0x11, 255}}
	for i, w := range want {
		if b.fills[i].paint.Color != w {
			t.Fatalf("fill %d = %v, want %v", i, b.fills[i].paint.Color, w)
		}
	}
	// Resizing keeps entries; the default palette, out-of-range indexes and
	// reserved COLORREF bytes are reported.
	o := PlayOptions{}
	reasons := reasonsOf(&o)
	record(t, emfScene(96, 64, 4, emfSelect(nullPen), pal, emfValue(EMRSelectPalette, 1), emfRecord(EMRResizePalette, longs(1, 1)), brush, emfSelect(2), box,
		emfValue(EMRSelectPalette, -0x7ffffff1), box, emfBrush(3, 0, 0x04000000), emfSelect(3), box), o)
	if len(*reasons) != 3 {
		t.Fatal(*reasons)
	}
	// A palette deleted while a saved state holds it is restored with it,
	// as Windows does, even after its slot is reused.
	o = PlayOptions{}
	reasons = reasonsOf(&o)
	b = record(t, emfScene(96, 64, 4, emfSelect(nullPen), pal, emfValue(EMRSelectPalette, 1), brush, emfSelect(2), emfEmpty(EMRSaveDC), emfValue(EMRSelectPalette, -0x7ffffff1), emfDelete(1),
		emfPalette(1, [4]byte{70, 80, 90, 0}, [4]byte{100, 110, 120, 0}), emfValue(EMRRestoreDC, -1), box), o)
	if len(*reasons) != 0 || len(b.fills) != 1 || b.fills[0].paint.Color != (color.NRGBA{40, 50, 60, 255}) {
		t.Fatal("restored palette", *reasons, b.fills)
	}
	// WMF palettes: SetPalEntries, AnimatePalette (PC_RESERVED entries only)
	// and ResizePalette act on the selected palette.
	wpal := testRecord(WMF, MetaCreatePalette, 0, cat(words(0x300, 2), []byte{1, 2, 3, 0, 4, 5, 6, 1}))
	wb := wmfBrush(0, 0x01000000, 0)
	wbox := wmfBox(MetaRectangle, 0, 0, 10, 10)
	animate := testRecord(WMF, MetaAnimatePalette, 0, cat(words(0, 2), []byte{9, 9, 9, 0, 7, 7, 7, 1}))
	set := testRecord(WMF, MetaSetPaletteEntries, 0, cat(words(0, 1), []byte{11, 12, 13, 0}))
	b = record(t, wmfScene(96, 64, 3, wmfPen(5, 0, 0), wmfRec(MetaSelectObject, 0), wpal, wmfRec(MetaSelectPalette, 1), wb, wmfRec(MetaSelectObject, 2), wbox, animate, wbox, set, wbox), PlayOptions{})
	for i, w := range []color.NRGBA{{1, 2, 3, 255}, {1, 2, 3, 255}, {11, 12, 13, 255}} {
		if b.fills[i].paint.Color != w {
			t.Fatalf("WMF fill %d = %v, want %v", i, b.fills[i].paint.Color, w)
		}
	}
}

// palDIB is an 8-bit DIB whose color table holds WORD palette indexes.
func palDIB() (info, bits []byte) {
	info = dibHeader(2, -1, 8, 0)
	put32(info, 32, 2) // two color table entries
	info = append(info, words(1, 0)...)
	return info, []byte{0, 1, 0, 0}
}

func TestPlayPaletteDIBs(t *testing.T) {
	info, bits := palDIB()
	blit := emfStretchDIBits(0, 0, 2, 1, 0, 0, 2, 1, 0x00cc0020, info, bits)
	put32(blit, 64, 1) // DIB_PAL_COLORS
	pal := emfPalette(1, [4]byte{10, 20, 30, 0}, [4]byte{40, 50, 60, 0})
	b := record(t, emfScene(96, 64, 2, pal, emfValue(EMRSelectPalette, 1), blit), PlayOptions{})
	im := b.images[0].draw.Image
	if c := color.NRGBAModel.Convert(im.At(0, 0)); c != (color.NRGBA{40, 50, 60, 255}) {
		t.Fatal("pixel 0 maps through entry 1", c)
	}
	if c := color.NRGBAModel.Convert(im.At(1, 0)); c != (color.NRGBA{10, 20, 30, 255}) {
		t.Fatal("pixel 1 maps through entry 0", c)
	}
	o := PlayOptions{}
	reasons := reasonsOf(&o)
	record(t, emfScene(96, 64, 2, blit), o)
	if len(*reasons) != 1 {
		t.Fatal("DIB_PAL_COLORS without a palette", *reasons)
	}
}

func TestPlayMonochromeBrushes(t *testing.T) {
	// A 2x2 checkerboard: set bits take the background color, clear bits the
	// text color.
	info := dibHeader(2, -2, 1, 0)
	info = append(info, 0, 0, 0, 0, 255, 255, 255, 0)
	bits := []byte{0x40, 0, 0, 0, 0x80, 0, 0, 0} // rows 01, 10
	mono := emfRecord(EMRCreateMonoBrush, cat(longs(1, 0, 32, int32(len(info)), int32(32+len(info)), int32(len(bits))), info, bits))
	b := record(t, emfScene(96, 64, 2, emfSelect(nullPen), mono, emfSelect(1), emfValue(EMRSetTextColor, red), emfValue(EMRSetBkColor, blue), emfBox(EMRRectangle, 0, 0, 10, 10)), PlayOptions{})
	p := b.fills[0].paint
	if p.Kind != PaintPattern || p.Pattern.Bounds() != image.Rect(0, 0, 2, 2) {
		t.Fatalf("%+v", p)
	}
	for _, c := range []struct {
		x, y int
		want color.NRGBA
	}{{0, 0, cRed}, {1, 0, cBlue}, {0, 1, cBlue}, {1, 1, cRed}} {
		if got := color.NRGBAModel.Convert(p.Pattern.At(c.x, c.y)); got != c.want {
			t.Fatalf("pattern (%d,%d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
	// Writers record monochrome brushes as DIB_PAL_INDICES without a color
	// table; the bits are read the same way.
	bare := dibHeader(2, -2, 1, 0)
	indices := emfRecord(EMRCreateMonoBrush, cat(longs(1, 2, 32, int32(len(bare)), int32(32+len(bare)), int32(len(bits))), bare, bits))
	b = record(t, emfScene(96, 64, 2, emfSelect(nullPen), indices, emfSelect(1), emfValue(EMRSetTextColor, red), emfValue(EMRSetBkColor, blue), emfBox(EMRRectangle, 0, 0, 10, 10)), PlayOptions{})
	if got := color.NRGBAModel.Convert(b.fills[0].paint.Pattern.At(1, 0)); got != cBlue {
		t.Fatal("DIB_PAL_INDICES monochrome brush", got)
	}
	// WMF monochrome Bitmap16 patterns, from CreatePatternBrush and from
	// DIBCreatePatternBrush with BS_PATTERN.
	bm := cat(words(0, 2, 2, 2), []byte{1, 1}, []byte{0x40, 0, 0x80, 0})
	legacy := testRecord(WMF, MetaCreatePatternBrush, 0, cat(bm[:10], make([]byte, 22), bm[10:]))
	dibStyle := testRecord(WMF, MetaDIBCreatePatternBrush, 0, cat(words(3, 0), bm))
	for _, brush := range []Record{legacy, dibStyle} {
		wb := record(t, wmfScene(96, 64, 2, wmfPen(5, 0, 0), wmfRec(MetaSelectObject, 0), brush, wmfRec(MetaSelectObject, 1), wmfRec(MetaSetTextColor, 0, 0), wmfRec(MetaSetBkColor, -1, 255), wmfBox(MetaRectangle, 0, 0, 10, 10)), PlayOptions{})
		pat := wb.fills[0].paint.Pattern
		if got := color.NRGBAModel.Convert(pat.At(1, 0)); got != (color.NRGBA{255, 255, 255, 255}) {
			t.Fatal("set bit is not the background color", got)
		}
		if got := color.NRGBAModel.Convert(pat.At(0, 0)); got != (color.NRGBA{0, 0, 0, 255}) {
			t.Fatal("clear bit is not the text color", got)
		}
	}
	// Colored Bitmap16 patterns depend on the recording device.
	o := PlayOptions{}
	reasons := reasonsOf(&o)
	color4 := testRecord(WMF, MetaDIBCreatePatternBrush, 0, cat(words(3, 0), words(0, 2, 2, 2), []byte{1, 4}, make([]byte, 4)))
	record(t, wmfScene(96, 64, 2, color4, wmfRec(MetaSelectObject, 0), wmfBox(MetaRectangle, 0, 0, 10, 10)), o)
	if len(*reasons) != 1 {
		t.Fatal(*reasons)
	}
}

func TestPlayRegions(t *testing.T) {
	region := func(rects ...Rect) []byte {
		var data []byte
		for _, r := range rects {
			data = append(data, longs(r.Left, r.Top, r.Right, r.Bottom)...)
		}
		return cat(longs(32, 1, int32(len(rects)), int32(len(data)), 0, 0, 0, 0), data)
	}
	// An L-shaped region: [4,24)x[4,12) and [4,12)x[12,30).
	l := region(Rect{4, 4, 24, 12}, Rect{4, 12, 12, 30})
	fill := emfRecord(EMRFillRgn, cat(longs(0, 0, 0, 0, int32(len(l)), 1), l))
	frame := emfRecord(EMRFrameRgn, cat(longs(0, 0, 0, 0, int32(len(l)), 1, 2, 3), l))
	paint := emfRecord(EMRPaintRgn, cat(longs(0, 0, 0, 0, int32(len(l))), l))
	for name, recs := range map[string][][]byte{"fill": {fill}, "paint": {emfSelect(1), paint}, "frame": {frame}} {
		data := emfScene(32, 32, 2, append([][]byte{emfBrush(1, 0, blue)}, recs...)...)
		im, err := playRender(data, 32, 32)
		if err != nil {
			t.Fatal(name, err)
		}
		inside := []image.Point{{5, 5}, {20, 10}, {5, 28}, {10, 13}}
		hollow := []image.Point{{14, 7}, {7, 22}} // more than 2 px from vertical edges and 3 px from horizontal ones
		outside := []image.Point{{2, 2}, {20, 20}, {26, 6}}
		for _, q := range inside {
			if c := im.NRGBAAt(q.X, q.Y); c != cBlue {
				t.Errorf("%s: %v = %v, want region color", name, q, c)
			}
		}
		for _, q := range hollow {
			want := cBlue
			if name == "frame" {
				want = cWhite
			}
			if c := im.NRGBAAt(q.X, q.Y); c != want {
				t.Errorf("%s: %v = %v, want %v", name, q, c, want)
			}
		}
		for _, q := range outside {
			if c := im.NRGBAAt(q.X, q.Y); c != cWhite {
				t.Errorf("%s: %v = %v, want background", name, q, c)
			}
		}
	}
	// The frame is exactly 2 units wide on vertical edges and 3 on
	// horizontal ones: column 5 (inside the left edge) is framed, column 6
	// is not; row 6 is framed, row 7 is not.
	im, err := playRender(emfScene(32, 32, 2, emfBrush(1, 0, blue), frame), 32, 32)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		x, y int
		want color.NRGBA
	}{{5, 20, cBlue}, {6, 20, cWhite}, {16, 6, cBlue}, {16, 7, cWhite}, {10, 20, cBlue}, {9, 20, cWhite}, {16, 9, cBlue}, {16, 8, cWhite}} {
		if got := im.NRGBAAt(c.x, c.y); got != c.want {
			t.Errorf("frame (%d,%d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
	// A frame of zero width draws nothing, and InvertRgn is reported.
	zero := emfRecord(EMRFrameRgn, cat(longs(0, 0, 0, 0, int32(len(l)), 1, 0, 3), l))
	if b := record(t, emfScene(32, 32, 2, emfBrush(1, 0, blue), zero), PlayOptions{}); len(b.fills) != 0 {
		t.Fatal("zero-width frame drew", b.fills)
	}
	// WMF FillRegion, FrameRegion and PaintRegion use region objects.
	wregion := testRecord(WMF, MetaCreateRegion, 0, wmfRegionFixture())
	wfill := testRecord(WMF, MetaFillRegion, 0, words(0, 1))
	wframe := testRecord(WMF, MetaFrameRegion, 0, words(0, 1, 1, 1))
	wpaint := wmfRec(MetaPaintRegion, 0)
	b := record(t, wmfScene(96, 64, 3, wregion, wmfBrush(0, red, 0), wmfRec(MetaSelectObject, 1), wfill, wframe, wpaint), PlayOptions{})
	if len(b.fills) != 3 || !near(b.fills[0].path.Points[2], Point{9, 6}) || len(b.fills[1].clip) != 1 || b.fills[2].paint.Color != cRed {
		t.Fatalf("WMF region painting %+v", b.fills)
	}
	o := PlayOptions{}
	reasons := reasonsOf(&o)
	record(t, wmfScene(96, 64, 1, wregion, wmfRec(MetaInvertRegion, 0)), o)
	if len(*reasons) != 1 {
		t.Fatal(*reasons)
	}
}

func TestPlayRegionFrameWorkIsBounded(t *testing.T) {
	// A staircase of 5,000 rectangles has a quadratic sweep; framing it must
	// stop at the work budget instead of running unbounded.
	var data []byte
	for i := int32(0); i < 5000; i++ {
		data = append(data, longs(i, i, i+2, i+1)...)
	}
	rgn := cat(longs(32, 1, 5000, int32(len(data)), 0, 0, 0, 0), data)
	frame := emfRecord(EMRFrameRgn, cat(longs(0, 0, 0, 0, int32(len(rgn)), -0x7ffffffc, 1, 1), rgn)) // BLACK_BRUSH
	if _, err := Play(emfScene(96, 64, 1, frame), PlayOptions{Destination: Box{Width: 1, Height: 1}}, &recordingBackend{}); !errors.Is(err, ErrLimit) {
		t.Fatal("region frame work must stop at its budget:", err)
	}
}

func TestPlayGradients(t *testing.T) {
	vertex := func(x, y int32, r, g, b uint16) []byte {
		return cat(longs(x, y), words(int16(r), int16(g), int16(b), -1))
	}
	rect := func(mode int32) []byte {
		return emfRecord(EMRGradientFill, cat(longs(0, 0, 0, 0, 2, 1, mode), vertex(10, 20, 0x1000, 0, 0), vertex(50, 40, 0xf000, 0, 0), longs(0, 1), longs(0)))
	}
	tri := emfRecord(EMRGradientFill, cat(longs(0, 0, 0, 0, 3, 1, 2), vertex(0, 0, 0xffff, 0, 0), vertex(30, 0, 0, 0xffff, 0), vertex(0, 30, 0, 0, 0xffff), longs(0, 1, 2)))
	b := &gradientRecorder{}
	if _, err := Play(emfScene(96, 64, 1, rect(0), rect(1), tri), PlayOptions{Destination: Box{Width: 192, Height: 128}}, b); err != nil {
		t.Fatal(err)
	}
	c0, c1 := color.NRGBA64{0x1000, 0, 0, 0xffff}, color.NRGBA64{0xf000, 0, 0, 0xffff}
	h, v := b.meshes[0], b.meshes[1]
	// Horizontal: the left edge has the upper-left color, the right edge the
	// lower-right color; coordinates are mapped to the destination (x2).
	if len(h) != 2 || h[0].Points != [3]Point{{20, 40}, {100, 40}, {100, 80}} || h[0].Colors != [3]color.NRGBA64{c0, c1, c1} || h[1].Colors != [3]color.NRGBA64{c0, c1, c0} {
		t.Fatalf("RECT_H %+v", h)
	}
	if v[0].Colors != [3]color.NRGBA64{c0, c0, c1} || v[1].Colors != [3]color.NRGBA64{c0, c1, c1} {
		t.Fatalf("RECT_V %+v", v)
	}
	if tr := b.meshes[2]; len(tr) != 1 || tr[0].Points[1] != (Point{60, 0}) || tr[0].Colors[2] != (color.NRGBA64{0, 0, 0xffff, 0xffff}) {
		t.Fatalf("triangle %+v", tr)
	}
	// Without GradientBackend the fill is reported.
	o := PlayOptions{}
	reasons := reasonsOf(&o)
	record(t, emfScene(96, 64, 1, tri), o)
	if len(*reasons) != 1 {
		t.Fatal(*reasons)
	}
}
