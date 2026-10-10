package gowemf

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"
)

// playRender replays data into the reference rasterizer at w x h pixels.
func playRender(data []byte, w, h int) (*image.NRGBA, error) {
	return playRenderOptions(data, w, h, PlayOptions{})
}

func playRenderOptions(data []byte, w, h int, o PlayOptions) (*image.NRGBA, error) {
	rb := newRasterBackend(w, h)
	o.Destination = Box{Width: float64(w), Height: float64(h)}
	_, err := Play(data, o, rb)
	return rb.canvas, err
}

// probe is a pixel whose color is derived from the specifications by hand.
type probe struct {
	x, y int
	want color.NRGBA
}

// renderScene is a generated metafile with spec-derived probe pixels. A scene
// with libreOffice probes pins a known LibreOffice divergence: it is excluded
// from whole-image agreement, and the oracle must still show the observed
// colors so a behavior change is reviewed rather than silently absorbed.
type renderScene struct {
	name        string
	data        []byte
	probes      []probe
	libreOffice []probe
	divergence  string
	customCaps  bool // PlayOptions.CustomLineCaps
}

func renderScenes() []renderScene {
	return append(append(append(append(append(agreementScenes(), fillScenes()...), wmfFillScenes()...), divergenceScenes()...), plusScenes()...), rasterScenes()...)
}

const (
	red     = 0x0000ff
	green   = 0x00c000
	blue    = 0xff0000
	black   = 0x000000
	magenta = 0xff00ff
	orange  = 0x0080ff
)

// emfScene uses a reference device with 0.25 mm pixels and a frame covering
// exactly w x h device pixels, so destination and device pixels coincide.
func emfScene(w, h int32, handles uint16, records ...[]byte) []byte {
	b := emfFixture(records...)
	copy(b[8:], longs(0, 0, w-1, h-1, 0, 0, w*25, h*25))
	copy(b[72:], longs(1000, 1000, 250, 250))
	put16(b, 56, handles)
	return b
}

// wmfScene adds a placeable header whose bounds cover w x h logical units.
// The META_HEADER type is MEMORYMETAFILE: LibreOffice 24.2 renders the
// equally valid DISKMETAFILE type blank (see ORACLES.md).
func wmfScene(w, h int16, slots uint16, records ...Record) []byte {
	place := make([]byte, 22)
	put32(place, 0, 0x9ac6cdd7)
	put16(place, 10, uint16(w))
	put16(place, 12, uint16(h))
	put16(place, 14, 96)
	var sum uint16
	for i := 0; i < 20; i += 2 {
		sum ^= u16(place[i:])
	}
	put16(place, 20, sum)
	doc := wmfDocument(slots, records...)
	put16(doc, 0, 1)
	return append(place, doc...)
}

func fl(v ...float64) []byte {
	f := make([]float32, len(v))
	for i, x := range v {
		f[i] = float32(x)
	}
	return floats(f...)
}

func cat(parts ...[]byte) []byte {
	var b []byte
	for _, p := range parts {
		b = append(b, p...)
	}
	return b
}

func emfSelect(id uint32) []byte { return emfRecord(EMRSelectObject, longs(int32(id))) }
func emfDelete(id uint32) []byte { return emfRecord(EMRDeleteObject, longs(int32(id))) }
func emfBrush(id, style, c uint32) []byte {
	return emfRecord(EMRCreateBrushIndirect, longs(int32(id), int32(style), int32(c), 0))
}
func emfPen(id, style uint32, width int32, c uint32) []byte {
	return emfRecord(EMRCreatePen, longs(int32(id), int32(style), width, 0, int32(c)))
}
func emfExtPen(id, style uint32, width int32, c uint32) []byte {
	return emfRecord(EMRExtCreatePen, longs(int32(id), 0, 0, 0, 0, int32(style), width, 0, int32(c), 0, 0))
}
func emfBox(typ uint32, l, t, r, b int32) []byte { return emfRecord(typ, longs(l, t, r, b)) }
func emfPoints(typ uint32, pts ...int32) []byte {
	return emfRecord(typ, cat(longs(0, 0, 0, 0, int32(len(pts)/2)), longs(pts...)))
}
func emfPolyPolygon(typ uint32, polys ...[]int32) []byte {
	var counts, pts []int32
	for _, p := range polys {
		counts = append(counts, int32(len(p)/2))
		pts = append(pts, p...)
	}
	return emfRecord(typ, cat(longs(0, 0, 0, 0, int32(len(polys)), int32(len(pts)/2)), longs(counts...), longs(pts...)))
}
func emfValue(typ uint32, v int32) []byte    { return emfRecord(typ, longs(v)) }
func emfPoint(typ uint32, x, y int32) []byte { return emfRecord(typ, longs(x, y)) }
func emfEmpty(typ uint32) []byte             { return emfRecord(typ, nil) }
func emfWorld(m Matrix) []byte {
	return emfRecord(EMRSetWorldTransform, fl(m.M11, m.M12, m.M21, m.M22, m.Dx, m.Dy))
}
func emfModifyWorld(m Matrix, mode int32) []byte {
	return emfRecord(EMRModifyWorldTransform, cat(fl(m.M11, m.M12, m.M21, m.M22, m.Dx, m.Dy), longs(mode)))
}
func emfArc(typ uint32, l, t, r, b, sx, sy, ex, ey int32) []byte {
	return emfRecord(typ, longs(l, t, r, b, sx, sy, ex, ey))
}

func rotation(deg, dx, dy float64) Matrix {
	s, c := math.Sincos(deg * math.Pi / 180)
	return Matrix{M11: c, M12: s, M21: -s, M22: c, Dx: dx, Dy: dy}
}

// sceneDIB is a 24-bit DIB with DWORD-padded rows. Rows are stored
// bottom-up unless topDown is set. pixel receives top-down coordinates.
func sceneDIB(w, h int, topDown bool, pixel func(x, y int) color.NRGBA) (info, bits []byte) {
	height := int32(h)
	if topDown {
		height = -height
	}
	info = dibHeader(int32(w), height, 24, 0)
	stride := (w*3 + 3) &^ 3
	bits = make([]byte, stride*h)
	for y := 0; y < h; y++ {
		row := y
		if !topDown {
			row = h - 1 - y
		}
		for x := 0; x < w; x++ {
			c := pixel(x, y)
			o := row*stride + x*3
			bits[o], bits[o+1], bits[o+2] = c.B, c.G, c.R
		}
	}
	return info, bits
}

var (
	qRed, qGreen   = color.NRGBA{230, 30, 30, 255}, color.NRGBA{30, 200, 60, 255}
	qBlue, qYellow = color.NRGBA{40, 60, 220, 255}, color.NRGBA{230, 200, 30, 255}
	cWhite, cBlack = color.NRGBA{255, 255, 255, 255}, color.NRGBA{0, 0, 0, 255}
	cRed, cGreen   = color.NRGBA{255, 0, 0, 255}, color.NRGBA{0, 192, 0, 255}
	cBlue, cOrange = color.NRGBA{0, 0, 255, 255}, color.NRGBA{255, 128, 0, 255}
	cMagenta       = color.NRGBA{255, 0, 255, 255}
)

// quadrantsN returns a w x h bitmap of red, green / blue, yellow quadrants
// with a black top-left pixel that reveals orientation.
func quadrantsN(w, h int) func(x, y int) color.NRGBA {
	return func(x, y int) color.NRGBA {
		switch {
		case x == 0 && y == 0:
			return cBlack
		case y < h/2 && x < w/2:
			return qRed
		case y < h/2:
			return qGreen
		case x < w/2:
			return qBlue
		}
		return qYellow
	}
}

var quadrants = quadrantsN(16, 8)

// bounds returns the inclusive device bounds of a destination rectangle,
// as writers record in the Bounds field of bitmap records.
func bounds(dx, dy, dw, dh int32) []byte {
	l, r, t, b := dx, dx+dw, dy, dy+dh
	if l > r {
		l, r = r, l
	}
	if t > b {
		t, b = b, t
	}
	return longs(l, t, r-1, b-1)
}

func emfStretchDIBits(dx, dy, dw, dh, sx, sy, sw, sh int32, rop uint32, info, bits []byte) []byte {
	const fixed = 72
	body := cat(bounds(dx, dy, dw, dh), longs(dx, dy, sx, sy, sw, sh, 8+fixed, int32(len(info)), int32(8+fixed+len(info)), int32(len(bits)), 0, int32(rop), dw, dh), info, bits)
	return emfRecord(EMRStretchDIBits, body)
}

// emfBlt encodes BitBlt (no source size), StretchBlt, AlphaBlend or
// TransparentBlt with an identity source transform.
func emfBlt(typ uint32, dx, dy, dw, dh int32, op uint32, sx, sy, sw, sh int32, info, bits []byte) []byte {
	fixed := 100
	if typ == EMRBitBlt {
		fixed = 92
	}
	head := cat(bounds(dx, dy, dw, dh), longs(dx, dy, dw, dh, int32(op), sx, sy), fl(1, 0, 0, 1, 0, 0), longs(0, 0))
	var spans []byte
	if info != nil {
		spans = longs(int32(8+fixed), int32(len(info)), int32(8+fixed+len(info)), int32(len(bits)))
	} else {
		spans = longs(0, 0, 0, 0)
	}
	body := cat(head, spans)
	if typ != EMRBitBlt {
		body = cat(body, longs(sw, sh))
	}
	return emfRecord(typ, cat(body, info, bits))
}

func emfSetDIBitsToDevice(dx, dy, sx, sy, sw, sh int32, scans int32, info, bits []byte) []byte {
	const fixed = 68
	return emfRecord(EMRSetDIBitsToDevice, cat(bounds(dx, dy, sw, sh), longs(dx, dy, sx, sy, sw, sh, 8+fixed, int32(len(info)), int32(8+fixed+len(info)), int32(len(bits)), 0, 0, scans), info, bits))
}

func emfClipRegion(mode int32, rects ...Rect) []byte {
	var data []byte
	u := rects[0]
	for _, r := range rects {
		data = append(data, longs(r.Left, r.Top, r.Right, r.Bottom)...)
		u = Rect{min(u.Left, r.Left), min(u.Top, r.Top), max(u.Right, r.Right), max(u.Bottom, r.Bottom)}
	}
	rgn := cat(longs(32, 1, int32(len(rects)), int32(len(data)), u.Left, u.Top, u.Right, u.Bottom), data)
	return emfRecord(EMRExtSelectClipRgn, cat(longs(int32(len(rgn)), mode), rgn))
}

const nullPen, blackBrush, nullBrush = 0x80000008, 0x80000004, 0x80000005

func agreementScenes() []renderScene {
	return []renderScene{
		{name: "transforms.emf", data: sceneTransforms(), probes: []probe{{31, 17, cRed}, {54, 58, cBlue}, {50, 30, cGreen}, {66, 28, cRed}, {86, 54, cBlack}, {90, 58, cBlack}, {91, 54, cWhite}, {86, 59, cWhite}, {2, 60, cWhite}}},
		{name: "mapping.emf", data: sceneMapping(), probes: []probe{{20, 20, cRed}, {44, 20, cWhite}, {37, 8, cBlue}, {64, 10, cWhite}, {80, 10, cGreen}, {64, 22, cGreen}, {80, 22, cGreen}, {24, 50, cRed}, {50, 50, cWhite}, {66, 46, cBlue}, {60, 51, cBlack}, {60, 56, cBlue}, {86, 46, cGreen}}},
		{name: "objects.emf", data: sceneObjects(), probes: []probe{{17, 17, cRed}, {6, 17, cBlue}, {2, 17, cWhite}, {49, 17, cOrange}, {80, 17, cRed}, {17, 48, cBlack}, {49, 46, cWhite}, {49, 55, cMagenta}, {80, 48, cRed}, {70, 48, cBlue}}},
		{name: "clip.emf", data: sceneClip(), probes: []probe{{20, 14, qRed}, {72, 14, qGreen}, {20, 48, qBlue}, {72, 48, qYellow}, {3, 30, cWhite}, {7, 8, cWhite}, {20, 30, cBlue}, {48, 30, qGreen}, {8, 4, cOrange}, {1, 4, cWhite}}},
		{name: "paths.emf", data: scenePaths(), probes: []probe{{8, 8, cRed}, {20, 20, cWhite}, {33, 33, cRed}, {55, 20, cRed}, {70, 20, cWhite}, {24, 50, cBlue}, {24, 61, cWhite}, {54, 50, cBlue}, {54, 44, cBlack}, {87, 57, cBlue}, {73, 45, cWhite}}},
		{name: "bitmaps.emf", data: sceneBitmaps(), probes: []probe{{5, 5, qRed}, {25, 5, qGreen}, {5, 15, qBlue}, {25, 15, qYellow}, {67, 3, cBlack}, {60, 5, qRed}, {40, 5, qGreen}, {72, 4, qRed}, {72, 14, color.NRGBA{25, 225, 225, 255}}, {6, 24, qGreen}, {6, 32, qYellow}, {35, 27, cOrange}, {45, 38, color.NRGBA{148, 158, 238, 255}}, {45, 30, color.NRGBA{243, 79, 15, 255}}, {80, 36, cRed}, {88, 36, cWhite}, {6, 50, qRed}, {26, 50, qGreen}}},
		{name: "mapping.wmf", data: sceneWMFMapping(), probes: []probe{{41, 40, cRed}, {66, 24, cBlue}, {18, 54, cBlue}, {34, 54, cRed}, {82, 11, cRed}}},
		{name: "objects.wmf", data: sceneWMFObjects(), probes: []probe{{16, 16, cRed}, {4, 16, cBlue}, {48, 16, cOrange}, {80, 16, cMagenta}, {60, 44, cWhite}, {54, 38, qRed}, {78, 40, qGreen}, {82, 40, cWhite}, {20, 34, cBlack}}},
	}
}

// Nested save/restore with world transforms set, left- and right-multiplied,
// restored two levels at once and reset.
func sceneTransforms() []byte {
	return emfScene(96, 64, 4,
		emfSelect(nullPen),
		emfBrush(1, 0, red), emfBrush(2, 0, blue), emfBrush(3, 0, green),
		emfSelect(1),
		emfWorld(rotation(20, 20, 6)),
		emfBox(EMRRectangle, 0, 0, 30, 14),
		emfEmpty(EMRSaveDC),
		emfSelect(2),
		emfModifyWorld(Matrix{M11: .5, M22: .5, Dx: 40, Dy: 30}, 2),
		emfBox(EMREllipse, 0, 0, 40, 30),
		emfEmpty(EMRSaveDC),
		emfSelect(3),
		emfModifyWorld(Matrix{M11: 1, M22: 1, Dy: -22}, 3),
		emfBox(EMRRectangle, 0, 0, 16, 12),
		emfValue(EMRRestoreDC, -2),
		emfBox(EMRRectangle, 40, 0, 62, 10),
		emfModifyWorld(Identity(), 1),
		emfSelect(blackBrush),
		emfBox(EMRRectangle, 80, 48, 92, 60),
	)
}

// Anisotropic mapping with both axes reflected, an arc in the reflected
// space, fixed MM_LOMETRIC, and MM_ANISOTROPIC inheriting fixed extents.
func sceneMapping() []byte {
	return emfScene(96, 64, 4,
		emfSelect(nullPen),
		emfBrush(1, 0, red), emfBrush(2, 0, blue), emfBrush(3, 0, green),
		emfValue(EMRSetMapMode, 8),
		emfPoint(EMRSetWindowExtEx, 200, -100),
		emfPoint(EMRSetViewportOrgEx, 0, 32),
		emfPoint(EMRSetViewportExtEx, 48, 32),
		emfSelect(1),
		emfPoints(EMRPolygon, 10, 10, 190, 10, 60, 90),
		emfSelect(2),
		emfBox(EMREllipse, 120, 50, 190, 95),
		emfPoint(EMRSetWindowExtEx, -200, -100),
		emfPoint(EMRSetViewportOrgEx, 96, 32),
		emfSelect(3),
		emfArc(EMRPie, 10, 10, 190, 90, 190, 50, 100, 90),
		emfValue(EMRSetMapMode, 2),
		emfPoint(EMRSetViewportOrgEx, 0, 64),
		emfSelect(1),
		emfBox(EMRRectangle, 10, 10, 110, 60),
		emfValue(EMRSetMapMode, 8),
		emfPoint(EMRSetWindowExtEx, 100, 100),
		emfPoint(EMRSetViewportExtEx, 40, -20),
		emfPoint(EMRSetViewportOrgEx, 56, 62),
		emfSelect(2),
		emfBox(EMRRectangle, 0, 0, 50, 100),
		emfSelect(3),
		emfBox(EMREllipse, 50, 0, 100, 100),
		emfExtPen(4, 0x10000|0x200, 10, black),
		emfSelect(4),
		emfPoints(EMRPolyline, 0, 50, 100, 50),
	)
}

// Pen/brush selection, stock objects, deletion and reuse of an unselected
// slot, geometric pens with caps/joins, and selections restored by RestoreDC.
func sceneObjects() []byte {
	return emfScene(96, 64, 5,
		emfBrush(1, 0, red),
		emfSelect(1),
		emfExtPen(2, 0x10000, 4, blue),
		emfSelect(2),
		emfBox(EMRRectangle, 6, 6, 28, 28),
		emfBrush(3, 0, green),
		emfDelete(3),
		emfBrush(3, 0, orange),
		emfSelect(3),
		emfBox(EMRRectangle, 38, 6, 60, 28),
		emfSelect(1),
		emfBox(EMRRectangle, 70, 6, 90, 28),
		emfEmpty(EMRSaveDC),
		emfSelect(blackBrush),
		emfSelect(nullPen),
		emfBox(EMREllipse, 4, 36, 30, 60),
		emfExtPen(4, 0x10000|0x100|0x2000, 6, magenta),
		emfSelect(4),
		emfPoints(EMRPolyline, 38, 42, 49, 56, 60, 42),
		emfValue(EMRRestoreDC, -1),
		emfBox(EMRRectangle, 70, 38, 90, 58),
	)
}

// Rectangle, path and region clipping with save/restore, union and offset,
// applied to a stretched bitmap and to vector fills.
func sceneClip() []byte {
	info, bits := sceneDIB(48, 32, false, quadrantsN(48, 32))
	return emfScene(96, 64, 4,
		emfSelect(nullPen),
		emfBrush(1, 0, blue), emfBrush(2, 0, orange),
		emfSelect(1),
		emfBox(EMRIntersectClipRect, 6, 4, 90, 60),
		emfEmpty(EMRBeginPath),
		emfBox(EMREllipse, 0, 0, 96, 64),
		emfEmpty(EMREndPath),
		emfValue(EMRSelectClipPath, 1),
		emfEmpty(EMRSaveDC),
		emfBox(EMRIntersectClipRect, 0, 0, 48, 64),
		emfValue(EMRRestoreDC, -1),
		emfStretchDIBits(0, 0, 96, 64, 0, 0, 48, 32, 0x00cc0020, info, bits),
		emfClipRegion(4, Rect{40, 20, 56, 44}),
		emfBox(EMRRectangle, 0, 24, 96, 40),
		emfClipRegion(2, Rect{0, 0, 12, 12}),
		emfPoint(EMROffsetClipRgn, 2, 0),
		emfSelect(2),
		emfBox(EMRRectangle, 0, 0, 16, 8),
	)
}

// Path brackets: alternate fill of overlapping and nested figures, a figure
// built from Bézier, line and arc segments with explicit closure,
// StrokeAndFillPath, and a clockwise pie outside a path.
func scenePaths() []byte {
	return emfScene(96, 64, 4,
		emfSelect(nullPen),
		emfBrush(1, 0, red), emfBrush(2, 0, blue),
		emfExtPen(3, 0x10000, 3, black),
		emfSelect(1),
		emfEmpty(EMRBeginPath),
		emfPolyPolygon(EMRPolyPolygon, []int32{4, 4, 28, 4, 28, 28, 4, 28}, []int32{14, 14, 38, 14, 38, 38, 14, 38}),
		emfEmpty(EMREndPath),
		emfValue(EMRSetPolyFillMode, 1),
		emfBox(EMRFillPath, 0, 0, 0, 0),
		emfEmpty(EMRBeginPath),
		emfBox(EMRRectangle, 0, 0, 96, 64),
		emfEmpty(EMRAbortPath),
		emfEmpty(EMRBeginPath),
		emfPolyPolygon(EMRPolyPolygon, []int32{50, 4, 90, 4, 90, 38, 50, 38}, []int32{60, 12, 80, 12, 80, 30, 60, 30}),
		emfEmpty(EMREndPath),
		emfBox(EMRFillPath, 0, 0, 0, 0),
		emfSelect(2),
		emfSelect(3),
		emfEmpty(EMRBeginPath),
		emfPoint(EMRMoveToEx, 8, 60),
		emfPoints(EMRPolyBezierTo, 8, 40, 40, 40, 40, 60),
		emfArc(EMRArcTo, 44, 44, 64, 60, 64, 52, 44, 52),
		emfEmpty(EMRCloseFigure),
		emfEmpty(EMREndPath),
		emfBox(EMRStrokeAndFillPath, 0, 0, 0, 0),
		emfValue(EMRSetArcDirection, 2),
		emfArc(EMRPie, 66, 40, 94, 62, 94, 51, 80, 62),
	)
}

// Bitmap placement: full, mirrored and partial StretchDIBits sources,
// BitBlt, NOTSRCCOPY StretchBlt, PATCOPY, and constant and per-pixel alpha.
func sceneBitmaps() []byte {
	info, bits := sceneDIB(16, 8, false, quadrants)
	topInfo, topBits := sceneDIB(16, 8, true, quadrants)
	alphaInfo, alphaBits := alphaDIB()
	return emfScene(96, 64, 4,
		emfBrush(1, 0, orange), emfSelect(1),
		emfStretchDIBits(2, 2, 32, 16, 0, 0, 16, 8, 0x00cc0020, info, bits),
		emfStretchDIBits(68, 2, -32, 16, 0, 0, 16, 8, 0x00cc0020, topInfo, topBits),
		emfBlt(EMRBitBlt, 70, 2, 16, 8, 0x00cc0020, 0, 0, 0, 0, info, bits),
		emfBlt(EMRStretchBlt, 70, 12, 16, 8, 0x00330008, 0, 0, 16, 8, info, bits),
		emfStretchDIBits(2, 20, 16, 16, 8, 0, 8, 8, 0x00cc0020, info, bits),
		emfStretchDIBits(2, 46, 32, 8, 0, 0, 16, 4, 0x00cc0020, info, bits),
		emfBlt(EMRBitBlt, 20, 20, 30, 14, 0x00f00021, 0, 0, 0, 0, nil, nil),
		emfBlt(EMRAlphaBlend, 40, 26, 32, 16, 0x00800000, 0, 0, 16, 8, info, bits),
		emfBlt(EMRAlphaBlend, 76, 30, 16, 16, 0x01ff0000, 0, 0, 8, 8, alphaInfo, alphaBits),
	)
}

// alphaDIB is an 8x8 premultiplied BGRA bitmap: opaque red on the left half,
// fully transparent on the right half.
func alphaDIB() (info, bits []byte) {
	info = dibHeader(8, 8, 32, 0)
	bits = make([]byte, 8*8*4)
	for i := 0; i < 64; i++ {
		if i%8 < 4 {
			copy(bits[i*4:], []byte{0, 0, 255, 255})
		}
	}
	return info, bits
}

func wmfRec(typ uint32, v ...int16) Record { return testRecord(WMF, typ, 0, words(v...)) }
func wmfBrush(style uint16, c uint32, hatch uint16) Record {
	return testRecord(WMF, MetaCreateBrushIndirect, 0, cat(words(int16(style)), longs(int32(c)), words(int16(hatch))))
}
func wmfPen(style uint16, width int16, c uint32) Record {
	return testRecord(WMF, MetaCreatePenIndirect, 0, cat(words(int16(style), width, 0), longs(int32(c))))
}

// WMF rectangles and points are stored in reverse field order.
func wmfBox(typ uint32, l, t, r, b int16) Record { return wmfRec(typ, b, r, t, l) }
func wmfPoly(typ uint32, pts ...int16) Record {
	return wmfRec(typ, append([]int16{int16(len(pts) / 2)}, pts...)...)
}
func wmfArc(typ uint32, l, t, r, b, sx, sy, ex, ey int16) Record {
	return wmfRec(typ, ey, ex, sy, sx, b, r, t, l)
}

// wmfRoundRect stores the corner size before the rectangle, both reversed.
func wmfRoundRect(l, t, r, b, w, h int16) Record { return wmfRec(MetaRoundRect, h, w, b, r, t, l) }

// WMF mapping: the placeable bounds define the picture while the metafile
// redefines its window with reflected axes, an arc in an x-reflected space
// and nested SaveDC state.
func sceneWMFMapping() []byte {
	return wmfScene(96, 64, 4,
		wmfRec(MetaSetMapMode, 8),
		wmfRec(MetaSetWindowOrg, 100, 0),
		wmfRec(MetaSetWindowExt, -100, 200),
		wmfPen(5, 0, 0), wmfRec(MetaSelectObject, 0),
		wmfBrush(0, red, 0), wmfRec(MetaSelectObject, 1),
		wmfPoly(MetaPolygon, 10, 10, 190, 10, 60, 90),
		wmfRec(MetaSaveDC),
		wmfRec(MetaSetWindowOrg, 0, 200),
		wmfRec(MetaSetWindowExt, 100, -200),
		wmfBrush(0, blue, 0), wmfRec(MetaSelectObject, 2),
		wmfBox(MetaEllipse, 10, 10, 90, 50),
		wmfArc(MetaPie, 100, 50, 190, 95, 190, 72, 145, 95),
		wmfRec(MetaRestoreDC, -1),
		wmfBox(MetaRectangle, 150, 70, 190, 95),
	)
}

// WMF object table: lowest-free-slot assignment after deleting unselected
// objects, geometric and hairline pens, and a clipped DIB transfer.
func sceneWMFObjects() []byte {
	info, bits := sceneDIB(16, 8, false, quadrants)
	dib := cat(info, bits)
	stretch := testRecord(WMF, MetaDIBStretchBlt, 0, cat(longs(0x00cc0020), words(8, 16, 0, 0, 16, 32, 36, 52), dib))
	return wmfScene(96, 64, 4,
		wmfRec(MetaSetMapMode, 8),
		wmfRec(MetaSetWindowOrg, 0, 0),
		wmfRec(MetaSetWindowExt, 64, 96),
		wmfBrush(0, red, 0), wmfRec(MetaSelectObject, 0),
		wmfPen(0, 3, blue), wmfRec(MetaSelectObject, 1),
		wmfBox(MetaRectangle, 4, 4, 28, 28),
		wmfBrush(0, green, 0), wmfRec(MetaDeleteObject, 2),
		wmfBrush(0, orange, 0), wmfRec(MetaSelectObject, 2),
		wmfBox(MetaRectangle, 36, 4, 60, 28),
		wmfRec(MetaDeleteObject, 0),
		wmfBrush(0, magenta, 0), wmfRec(MetaSelectObject, 0),
		wmfRoundRect(68, 4, 92, 28, 12, 8),
		wmfBox(MetaIntersectClipRect, 0, 32, 80, 64),
		wmfBox(MetaExcludeClipRect, 56, 40, 64, 48),
		stretch,
		wmfPen(0, 0, black), wmfRec(MetaSelectObject, 3),
		wmfRec(MetaMoveTo, 34, 4),
		wmfRec(MetaLineTo, 34, 40),
	)
}

// Divergence scenes isolate one behavior each. Probes record the result
// required by the specification; libreOffice records what LibreOffice
// 24.2.7.2 was observed to draw instead.
func divergenceScenes() []renderScene {
	info, bits := sceneDIB(16, 8, false, quadrants)
	blueFill := func(recs ...[]byte) []byte {
		pre := [][]byte{emfSelect(nullPen), emfBrush(1, 0, blue), emfSelect(1)}
		return emfScene(96, 64, 4, append(append(pre, recs...), emfBox(EMRRectangle, 0, 0, 96, 64))...)
	}
	wmfDelete := wmfScene(96, 64, 2, wmfPen(5, 0, 0), wmfRec(MetaSelectObject, 0), wmfBrush(0, blue, 0), wmfRec(MetaSelectObject, 1), wmfRec(MetaDeleteObject, 1), wmfBox(MetaRectangle, 0, 0, 96, 64))
	return []renderScene{
		{name: "lo-isotropic.emf", divergence: "MM_ISOTROPIC viewport is not adjusted to square units (MS-WMF 2.1.1.16)",
			data:   emfScene(96, 64, 4, emfSelect(nullPen), emfBrush(1, 0, blue), emfSelect(1), emfValue(EMRSetMapMode, 7), emfPoint(EMRSetWindowExtEx, 100, 100), emfPoint(EMRSetViewportExtEx, 96, 32), emfBox(EMRRectangle, 0, 0, 100, 100)),
			probes: []probe{{16, 16, cBlue}, {60, 16, cWhite}}, libreOffice: []probe{{16, 16, cBlue}, {60, 16, cBlue}}},
		{name: "lo-compatible-arc.emf", divergence: "EMF arc direction is applied in logical space under a one-axis reflection; GM_COMPATIBLE arcs must not reflect the transform (MS-EMF 2.1.16)",
			data:   emfScene(96, 64, 4, emfSelect(nullPen), emfBrush(1, 0, blue), emfSelect(1), emfValue(EMRSetMapMode, 2), emfPoint(EMRSetViewportOrgEx, 0, 64), emfArc(EMRChord, 10, 10, 170, 90, 170, 50, 10, 50)),
			probes: []probe{{36, 36, cBlue}, {36, 52, cWhite}}, libreOffice: []probe{{36, 36, cWhite}, {36, 52, cBlue}}},
		{name: "lo-createpen-width.emf", divergence: "EMR_CREATEPEN widths without PS_GEOMETRIC are drawn as hairlines",
			data:   emfScene(96, 64, 4, emfPen(1, 0, 12, blue), emfSelect(1), emfSelect(nullBrush), emfPoints(EMRPolyline, 10, 30, 86, 30)),
			probes: []probe{{48, 26, cBlue}, {48, 33, cBlue}}, libreOffice: []probe{{48, 26, cWhite}, {48, 33, cWhite}}},
		{name: "lo-world-pen.emf", divergence: "geometric pens are not transformed by an anisotropic world transform (GM_ADVANCED)",
			data:   emfScene(96, 64, 4, emfWorld(Matrix{M11: .5, M22: 1}), emfExtPen(1, 0x10000, 12, blue), emfSelect(1), emfSelect(nullBrush), emfPoints(EMRPolyline, 10, 30, 86, 30)),
			probes: []probe{{24, 25, cBlue}, {24, 34, cBlue}}, libreOffice: []probe{{24, 25, cWhite}, {24, 34, cWhite}}},
		{name: "lo-delete-selected.emf", divergence: "a deleted selected brush stays active; MS-EMF 3.1.1.1 activates the default stock object",
			data:   blueFill(emfDelete(1)),
			probes: []probe{{48, 32, cWhite}}, libreOffice: []probe{{48, 32, cBlue}}},
		{name: "lo-delete-selected.wmf", divergence: "a deleted selected brush stays active; playback releases it and uses the default brush",
			data:   wmfDelete,
			probes: []probe{{48, 32, cWhite}}, libreOffice: []probe{{48, 32, cBlue}}},
		{name: "lo-restore-reused.emf", divergence: "RestoreDC reselects a pen whose slot was deleted and reused; playback falls back to the default pen",
			data:   emfScene(96, 64, 4, emfExtPen(1, 0x10000, 8, blue), emfSelect(1), emfEmpty(EMRSaveDC), emfSelect(nullPen), emfDelete(1), emfExtPen(1, 0x10000, 8, magenta), emfValue(EMRRestoreDC, -1), emfSelect(nullBrush), emfPoints(EMRPolyline, 10, 30, 86, 30)),
			probes: []probe{{48, 27, cWhite}, {48, 33, cWhite}}, libreOffice: []probe{{48, 27, cBlue}, {48, 33, cBlue}}},
		{name: "lo-winding.emf", divergence: "WINDING (nonzero) polygon fill is drawn as ALTERNATE",
			data:   emfScene(96, 64, 4, emfSelect(nullPen), emfBrush(1, 0, blue), emfSelect(1), emfValue(EMRSetPolyFillMode, 2), emfPolyPolygon(EMRPolyPolygon, []int32{4, 4, 60, 4, 60, 60, 4, 60}, []int32{20, 20, 80, 20, 80, 50, 20, 50})),
			probes: []probe{{40, 30, cBlue}, {10, 10, cBlue}, {70, 30, cBlue}}, libreOffice: []probe{{40, 30, cWhite}, {10, 10, cBlue}, {70, 30, cBlue}}},
		{name: "lo-exclude-clip.emf", divergence: "EMR_EXCLUDECLIPRECT has no effect",
			data:   blueFill(emfBox(EMRExcludeClipRect, 40, 20, 60, 40)),
			probes: []probe{{50, 30, cWhite}, {10, 10, cBlue}}, libreOffice: []probe{{50, 30, cBlue}, {10, 10, cBlue}}},
		{name: "lo-region-copy.emf", divergence: "EMR_EXTSELECTCLIPRGN with RGN_COPY has no effect",
			data:   blueFill(emfClipRegion(5, Rect{40, 20, 60, 40})),
			probes: []probe{{50, 30, cBlue}, {10, 10, cWhite}}, libreOffice: []probe{{50, 30, cBlue}, {10, 10, cBlue}}},
		{name: "lo-region-and.emf", divergence: "EMR_EXTSELECTCLIPRGN with RGN_AND combines like RGN_XOR",
			data:   blueFill(emfBox(EMRIntersectClipRect, 0, 0, 60, 64), emfClipRegion(1, Rect{40, 20, 96, 40})),
			probes: []probe{{50, 30, cBlue}, {30, 30, cWhite}, {70, 30, cWhite}}, libreOffice: []probe{{50, 30, cWhite}, {30, 30, cBlue}, {70, 30, cBlue}}},
		{name: "lo-metaregion.emf", divergence: "EMR_SETMETARGN does not intersect later clipping with the metaregion (MS-EMF 2.3.2)",
			data:   blueFill(emfBox(EMRIntersectClipRect, 0, 0, 60, 64), emfEmpty(EMRSetMetaRgn), emfBox(EMRIntersectClipRect, 40, 0, 96, 40)),
			probes: []probe{{50, 30, cBlue}, {30, 30, cWhite}, {70, 30, cWhite}}, libreOffice: []probe{{50, 30, cWhite}, {30, 30, cBlue}, {70, 30, cBlue}}},
		{name: "lo-rotated-bitmap.emf", divergence: "a world-rotated StretchDIBits destination is drawn unrotated, squeezed into an axis-aligned box",
			data:   emfScene(96, 64, 4, emfWorld(rotation(30, 30, 10)), emfStretchDIBits(0, 0, 48, 24, 0, 0, 16, 8, 0x00cc0020, info, bits)),
			probes: []probe{{37, 21, qRed}, {28, 34, qBlue}, {66, 46, cWhite}}, libreOffice: []probe{{28, 34, cWhite}}},
		{name: "lo-wmf-no-window.wmf", divergence: "a WMF that never sets its window is scaled to its drawn content instead of the placeable bounds",
			data:   wmfScene(96, 64, 2, wmfPen(5, 0, 0), wmfRec(MetaSelectObject, 0), wmfBrush(0, red, 0), wmfRec(MetaSelectObject, 1), wmfBox(MetaRectangle, 40, 20, 56, 44)),
			probes: []probe{{48, 32, cRed}, {10, 10, cWhite}}, libreOffice: []probe{{48, 32, cRed}, {10, 10, cRed}}},
		{name: "lo-setdibits.emf", divergence: "EMR_SETDIBITSTODEVICE draws nothing",
			data:   emfScene(96, 64, 4, emfSetDIBitsToDevice(8, 8, 0, 0, 16, 8, 8, info, bits), emfSetDIBitsToDevice(40, 8, 8, 0, 8, 4, 8, info, bits)),
			probes: []probe{{10, 9, qRed}, {20, 14, qYellow}, {44, 10, qYellow}, {44, 13, cWhite}}, libreOffice: []probe{{10, 9, cWhite}, {20, 14, cWhite}, {44, 10, cWhite}}},
		{name: "lo-transparentblt.emf", divergence: "EMR_TRANSPARENTBLT draws nothing",
			data:   emfScene(96, 64, 4, emfBlt(EMRTransparentBlt, 8, 8, 64, 32, 0x001ec8e6, 0, 0, 16, 8, info, bits)),
			probes: []probe{{16, 12, qRed}, {60, 32, cWhite}, {20, 32, qBlue}}, libreOffice: []probe{{16, 12, cWhite}, {60, 32, cWhite}, {20, 32, cWhite}}},
	}
}

func probeColor(im image.Image, p probe) error {
	got := color.NRGBAModel.Convert(im.At(p.x, p.y)).(color.NRGBA)
	for _, d := range [][2]uint8{{got.R, p.want.R}, {got.G, p.want.G}, {got.B, p.want.B}} {
		if diff := int(d[0]) - int(d[1]); diff > 24 || diff < -24 {
			return fmt.Errorf("pixel (%d,%d) = %v, want %v", p.x, p.y, got, p.want)
		}
	}
	return nil
}

// TestPlaybackScenes replays every generated scene offline and checks the
// spec-derived probe pixels, including those of pinned LibreOffice
// divergences. The LibreOffice comparison itself runs under make test-render.
func TestPlaybackScenes(t *testing.T) {
	for _, s := range renderScenes() {
		t.Run(s.name, func(t *testing.T) {
			im, err := playRenderOptions(s.data, 96, 64, PlayOptions{CustomLineCaps: s.customCaps})
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range s.probes {
				if err := probeColor(im, p); err != nil {
					t.Error(err)
				}
			}
		})
	}
}

func gradVertex(x, y int32, c color.NRGBA) []byte {
	return cat(longs(x, y), words(int16(uint16(c.R)<<8), int16(uint16(c.G)<<8), int16(uint16(c.B)<<8), 0))
}

func emfRegionRecord(typ uint32, head []int32, rects ...Rect) []byte {
	var data []byte
	for _, r := range rects {
		data = append(data, longs(r.Left, r.Top, r.Right, r.Bottom)...)
	}
	u := rects[0]
	for _, r := range rects {
		u = Rect{min(u.Left, r.Left), min(u.Top, r.Top), max(u.Right, r.Right), max(u.Bottom, r.Bottom)}
	}
	rgn := cat(longs(32, 1, int32(len(rects)), int32(len(data)), u.Left, u.Top, u.Right, u.Bottom), data)
	b := u
	h := append([]int32{b.Left, b.Top, b.Right - 1, b.Bottom - 1, int32(len(rgn))}, head...)
	return emfRecord(typ, cat(longs(h...), rgn))
}

// monoDIB is a 1-bit top-down DIB of w x h whose set bits are given by on.
func monoDIB(w, h int, on func(x, y int) bool) (info, bits []byte) {
	info = append(dibHeader(int32(w), int32(-h), 1, 0), 0, 0, 0, 0, 255, 255, 255, 0)
	stride := (w + 31) / 32 * 4
	bits = make([]byte, stride*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if on(x, y) {
				bits[y*stride+x/8] |= 0x80 >> uint(x%8)
			}
		}
	}
	return info, bits
}

func fillScenes() []renderScene {
	red8, blue8, green8 := color.NRGBA{255, 0, 0, 255}, color.NRGBA{0, 0, 255, 255}, color.NRGBA{0, 160, 0, 255}
	yellow := color.NRGBA{255, 220, 0, 255}
	vertical := emfRecord(EMRGradientFill, cat(longs(52, 4, 91, 27, 2, 1, 1), gradVertex(52, 4, cBlack), gradVertex(92, 28, yellow), longs(0, 1), longs(0)))
	triangle := emfRecord(EMRGradientFill, cat(longs(4, 34, 91, 59, 3, 1, 2), gradVertex(4, 60, red8), gradVertex(48, 34, green8), gradVertex(92, 60, blue8), longs(0, 1, 2)))
	monoInfo, monoBits := monoDIB(8, 8, func(x, y int) bool { return x >= 4 })
	mono := emfRecord(EMRCreateMonoBrush, cat(longs(1, 0, 32, int32(len(monoInfo)), int32(32+len(monoInfo)), int32(len(monoBits))), monoInfo, monoBits))
	palInfo, palBits := palDIB()
	palBlit := emfStretchDIBits(52, 34, 32, 16, 0, 0, 2, 1, 0x00cc0020, palInfo, palBits)
	put32(palBlit, 64, 1)
	region := func(pen ...[]byte) []byte {
		return emfScene(96, 64, 3, append(pen, emfBrush(1, 0, red), emfBrush(2, 0, green), emfSelect(2),
			emfRegionRecord(EMRFillRgn, []int32{1}, Rect{4, 4, 36, 16}, Rect{4, 16, 16, 40}),
			emfRegionRecord(EMRPaintRgn, nil, Rect{20, 44, 80, 60}))...)
	}
	return []renderScene{
		// FillRgn on an L shape and PaintRgn with the selected brush; the
		// null pen keeps LibreOffice's extra outline (pinned below) away.
		{name: "fills-region.emf", data: region(emfSelect(nullPen)),
			probes: []probe{{10, 10, cRed}, {30, 30, cWhite}, {10, 36, cRed}, {4, 30, cRed}, {16, 30, cWhite}, {50, 52, cGreen}, {20, 50, cGreen}, {19, 50, cWhite}}},
		// Rectangle gradients: RECT_H at pixel x samples t=(x+0.5-4)/40 and
		// RECT_V at row y samples t=(y+0.5-4)/24; the triangle probes use
		// barycentric weights of the pixel centers (48.5,36.5), (48.5,59.5).
		{name: "lo-gradient.emf", divergence: "EMR_GRADIENTFILL draws nothing",
			data:        emfScene(96, 64, 1, emfRecord(EMRGradientFill, cat(longs(4, 4, 43, 27, 2, 1, 0), gradVertex(4, 4, red8), gradVertex(44, 28, blue8), longs(0, 1), longs(0))), vertical, triangle),
			probes:      []probe{{4, 16, color.NRGBA{252, 0, 3, 255}}, {24, 16, color.NRGBA{124, 0, 131, 255}}, {72, 5, color.NRGBA{16, 14, 0, 255}}, {72, 26, color.NRGBA{239, 206, 0, 255}}, {48, 36, color.NRGBA{11, 145, 14, 255}}, {48, 59, color.NRGBA{124, 3, 126, 255}}, {2, 2, cWhite}},
			libreOffice: []probe{{24, 16, cWhite}, {72, 26, cWhite}, {48, 50, cWhite}}},
		// FrameRgn with a 3 by 2 unit border.
		{name: "lo-frame-region.emf", divergence: "EMR_FRAMERGN draws nothing",
			data:        emfScene(96, 64, 2, emfSelect(nullPen), emfBrush(1, 0, blue), emfRegionRecord(EMRFrameRgn, []int32{1, 3, 2}, Rect{44, 4, 88, 40})),
			probes:      []probe{{44, 20, cBlue}, {46, 20, cBlue}, {47, 20, cWhite}, {66, 5, cBlue}, {66, 6, cWhite}, {66, 38, cBlue}, {66, 37, cWhite}},
			libreOffice: []probe{{45, 20, cWhite}, {66, 5, cWhite}}},
		{name: "lo-region-outline.emf", divergence: "FillRgn and PaintRgn also stroke the region outline with the selected pen",
			data:        region(),
			probes:      []probe{{4, 30, cRed}, {50, 44, cGreen}},
			libreOffice: []probe{{4, 30, cBlack}, {50, 44, cBlack}}},
		// Clear bits take the text color, set bits the background color.
		{name: "lo-mono-brush.emf", divergence: "monochrome pattern brushes fill with the background color only",
			data:        emfScene(96, 64, 2, emfSelect(nullPen), mono, emfSelect(1), emfValue(EMRSetTextColor, red), emfValue(EMRSetBkColor, 0x00dcff), emfBox(EMRRectangle, 0, 0, 97, 65)),
			probes:      []probe{{1, 1, cRed}, {5, 1, yellow}, {9, 30, cRed}, {13, 30, yellow}},
			libreOffice: []probe{{1, 1, yellow}, {9, 30, yellow}}},
		// PALETTEINDEX colors and a DIB_PAL_COLORS bitmap read the selected
		// logical palette.
		{name: "lo-palette-index.emf", divergence: "PALETTEINDEX colors are drawn black and DIB_PAL_COLORS bitmaps are not drawn",
			data: emfScene(96, 64, 3, emfSelect(nullPen),
				emfPalette(1, [4]byte{200, 40, 0, 0}, [4]byte{0, 120, 220, 0}), emfValue(EMRSelectPalette, 1),
				emfBrush(2, 0, 0x01000000), emfSelect(2), emfBox(EMRRectangle, 4, 4, 44, 28),
				emfBrush(3, 0, 0x01000001), emfSelect(3), emfBox(EMRRectangle, 52, 4, 92, 28), palBlit),
			probes:      []probe{{20, 16, color.NRGBA{200, 40, 0, 255}}, {70, 16, color.NRGBA{0, 120, 220, 255}}, {60, 40, color.NRGBA{0, 120, 220, 255}}, {76, 40, color.NRGBA{200, 40, 0, 255}}},
			libreOffice: []probe{{20, 16, cBlack}, {70, 16, cBlack}, {60, 40, cWhite}, {76, 40, cWhite}}},
	}
}

// wmfRegionObject encodes META_CREATEREGION with one rectangle per scan
// (MS-WMF 2.2.1.5): each scan is [top, bottom, left, right] in logical units.
func wmfRegionObject(scans ...[4]int16) Record {
	l, t, r, b := scans[0][2], scans[0][0], scans[0][3], scans[0][1]
	var body []byte
	for _, s := range scans {
		l, t, r, b = min(l, s[2]), min(t, s[0]), max(r, s[3]), max(b, s[1])
		body = append(body, words(2, s[0], s[1], s[2], s[3], 2)...)
	}
	head := cat(words(0, 6), longs(0), words(int16(22+len(body)), int16(len(scans)), 2, l, t, r, b))
	return testRecord(WMF, MetaCreateRegion, 0, cat(head, body))
}

func wmfFillScenes() []renderScene {
	window := []Record{wmfRec(MetaSetMapMode, 8), wmfRec(MetaSetWindowOrg, 0, 0), wmfRec(MetaSetWindowExt, 64, 96)}
	l := wmfRegionObject([4]int16{4, 16, 4, 36}, [4]int16{16, 40, 4, 16})
	return []renderScene{
		// FillRegion and PaintRegion with region objects, then a clip region.
		{name: "lo-regions.wmf", divergence: "WMF FillRegion, PaintRegion and SelectClipRegion have no effect",
			data: wmfScene(96, 64, 7, append(window,
				wmfPen(5, 0, 0), wmfRec(MetaSelectObject, 0),
				l, wmfBrush(0, red, 0), testRecord(WMF, MetaFillRegion, 0, words(1, 2)),
				wmfBrush(0, green, 0), wmfRec(MetaSelectObject, 3), wmfRegionObject([4]int16{44, 60, 20, 80}), wmfRec(MetaPaintRegion, 4),
				wmfRegionObject([4]int16{4, 40, 60, 76}), wmfRec(MetaSelectClipRegion, 5), wmfBrush(0, blue, 0), wmfRec(MetaSelectObject, 6), wmfBox(MetaRectangle, 40, 0, 96, 64))...),
			probes:      []probe{{10, 10, cRed}, {30, 30, cWhite}, {10, 36, cRed}, {50, 52, cGreen}, {68, 20, cBlue}, {58, 20, cWhite}, {80, 20, cWhite}},
			libreOffice: []probe{{10, 10, cWhite}, {30, 52, cWhite}, {50, 20, cBlue}, {80, 20, cBlue}}},
	}
}

// Raster operations need a backend that reads its destination; the scene
// backend implements RasterBackend.
func rasterScenes() []renderScene {
	sprite := []probe{{24, 24, cRed}, {12, 12, cBlue}, {36, 36, cBlue}, {4, 4, cBlue}, {48, 48, cRed}, {42, 42, cBlue}, {54, 42, cWhite}, {70, 20, cWhite}}
	return []renderScene{
		{name: "raster-sprite.emf", data: sceneRasterSprite(false), probes: sprite},
		{name: "raster-sprite.wmf", data: sceneRasterSprite(true), probes: sprite},
		{name: "lo-raster-ops.emf", divergence: "SRCINVERT is drawn as SRCCOPY, DSTINVERT inverts with the selected brush like PATINVERT, and PATINVERT draws nothing",
			data: sceneRasterOps(),
			probes: []probe{
				{56, 4, cWhite}, {57, 5, cWhite}, {58, 6, color.NRGBA{25, 225, 225, 255}}, {62, 8, color.NRGBA{25, 225, 225, 255}}, {80, 8, color.NRGBA{225, 55, 195, 255}}, {62, 16, color.NRGBA{215, 195, 35, 255}}, {80, 16, color.NRGBA{25, 55, 225, 255}},
				{44, 28, color.NRGBA{255, 255, 0, 255}}, {52, 28, cBlack},
				{72, 34, color.NRGBA{255, 63, 255, 255}}, {72, 44, cWhite}},
			libreOffice: []probe{{62, 8, qRed}, {80, 16, qYellow}, {44, 28, cBlack}, {52, 28, color.NRGBA{255, 255, 0, 255}}, {72, 34, cWhite}}},
	}
}

func spriteDIBs() (mask, image []byte) {
	inside := func(x, y int) bool { return x >= 4 && x < 12 && y >= 4 && y < 12 }
	maskInfo, maskBits := sceneDIB(16, 16, false, func(x, y int) color.NRGBA {
		if inside(x, y) {
			return cBlack
		}
		return cWhite
	})
	imageInfo, imageBits := sceneDIB(16, 16, false, func(x, y int) color.NRGBA {
		if inside(x, y) {
			return cRed
		}
		return cBlack
	})
	return cat(maskInfo, maskBits), cat(imageInfo, imageBits)
}

const codeSrcAnd, codeSrcPaint = 0x008800c6, 0x00ee0086

// sceneRasterSprite draws a sprite, a 16x16 mask through SRCAND and its image
// through SRCPAINT, over a blue (0,0)-(48,60): at 2x onto (8,8)-(40,40) and at 1x
// across the blue edge onto (40,40)-(56,56). The EMF uses StretchDIBits; the
// WMF uses StretchDIB for the first and DIBStretchBlt for the second.
func sceneRasterSprite(wmf bool) []byte {
	mask, image := spriteDIBs()
	if wmf {
		stretchDIB := func(rop uint32, x, y, size int16, dib []byte) Record {
			return testRecord(WMF, MetaStretchDIB, 0, cat(longs(int32(rop)), words(0, 16, 16, 0, 0, size, size, y, x), dib))
		}
		dibStretchBlt := func(rop uint32, x, y, size int16, dib []byte) Record {
			return testRecord(WMF, MetaDIBStretchBlt, 0, cat(longs(int32(rop)), words(16, 16, 0, 0, size, size, y, x), dib))
		}
		return wmfScene(96, 64, 2, wmfRec(MetaSetMapMode, 8), wmfRec(MetaSetWindowOrg, 0, 0), wmfRec(MetaSetWindowExt, 64, 96),
			wmfPen(5, 0, 0), wmfRec(MetaSelectObject, 0), wmfBrush(0, blue, 0), wmfRec(MetaSelectObject, 1), wmfBox(MetaRectangle, 0, 0, 48, 60),
			stretchDIB(codeSrcAnd, 8, 8, 32, mask), stretchDIB(codeSrcPaint, 8, 8, 32, image),
			dibStretchBlt(codeSrcAnd, 40, 40, 16, mask), dibStretchBlt(codeSrcPaint, 40, 40, 16, image))
	}
	split := func(dib []byte) ([]byte, []byte) { return dib[:40], dib[40:] }
	mi, mb := split(mask)
	ii, ib := split(image)
	return emfScene(96, 64, 3,
		emfSelect(nullPen), emfBrush(1, 0, blue), emfSelect(1), emfBox(EMRRectangle, 0, 0, 48, 60),
		emfStretchDIBits(8, 8, 32, 32, 0, 0, 16, 16, codeSrcAnd, mi, mb),
		emfStretchDIBits(8, 8, 32, 32, 0, 0, 16, 16, codeSrcPaint, ii, ib),
		emfStretchDIBits(40, 40, 16, 16, 0, 0, 16, 16, codeSrcAnd, mi, mb),
		emfStretchDIBits(40, 40, 16, 16, 0, 0, 16, 16, codeSrcPaint, ii, ib),
	)
}

// sceneRasterOps draws, over a blue left half: SRCINVERT quadrants on white
// at (56,4)-(88,20), DSTINVERT across the blue edge at (40,24)-(56,32), and
// PATINVERT with a green brush at (56,28)-(88,40).
func sceneRasterOps() []byte {
	info, bits := sceneDIB(16, 8, false, quadrants)
	const srcInvert, dstInvert, patInvert = 0x00660046, 0x00550009, 0x005a0049
	return emfScene(96, 64, 3,
		emfSelect(nullPen), emfBrush(1, 0, blue), emfSelect(1), emfBox(EMRRectangle, 0, 0, 48, 64),
		emfStretchDIBits(56, 4, 32, 16, 0, 0, 16, 8, srcInvert, info, bits),
		emfBlt(EMRBitBlt, 40, 24, 16, 8, dstInvert, 0, 0, 0, 0, nil, nil),
		emfBrush(2, 0, green), emfSelect(2),
		emfBlt(EMRBitBlt, 56, 28, 32, 12, patInvert, 0, 0, 0, 0, nil, nil),
	)
}
