package gowemf

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// EMF+ scenes are EMF+ Only pictures at 96 DPI, one device pixel per
// destination pixel. Under the default PixelOffsetMode, world coordinate x
// is the center of device pixel x, so geometry lands half a pixel right and
// down of the destination pixel grid; probes are placed clear of edges.
func plusScene96(records ...[]byte) []byte {
	all := plusComment(plusHeaderDPI(false, 96, 96))
	for _, r := range records {
		all = append(all, plusComment(r)...)
	}
	all = append(all, plusComment(plusRec(PlusEndOfFileRecord, 0))...)
	return emfScene(96, 64, 1, emfSplit(all)...)
}

func plusScenes() []renderScene {
	return []renderScene{
		{name: "plus-shapes.emf", data: scenePlusShapes(), probes: []probe{{10, 10, cRed}, {2, 2, cWhite}, {42, 12, cBlue}, {31, 5, cWhite}, {83, 27, cGreen}, {69, 27, cWhite}, {83, 13, cWhite}, {8, 32, cMagenta}, {19, 43, cWhite}, {29, 53, cMagenta}, {43, 34, cOrange}, {57, 56, cWhite}, {78, 50, cBlack}, {78, 46, cWhite}, {78, 58, color.NRGBA{127, 127, 255, 255}}}},
		{name: "plus-transforms.emf", data: scenePlusTransforms(), probes: []probe{{25, 27, cRed}, {12, 22, cWhite}, {60, 18, cBlue}, {45, 18, cWhite}, {82, 46, cGreen}, {75, 39, cWhite}, {12, 48, cBlack}}},
		{name: "plus-clip.emf", data: scenePlusClip(), probes: []probe{{18, 30, cRed}, {38, 30, cWhite}, {4, 4, cWhite}, {80, 30, cBlue}, {65, 30, cWhite}, {55, 30, cWhite}}},
		{name: "plus-pens.emf", data: scenePlusPens(), probes: []probe{{8, 18, cBlack}, {23, 18, cWhite}, {82, 10, cGreen}, {86, 10, cWhite}, {84, 54, cBlue}, {84, 26, cWhite}, {24, 56, cRed}, {24, 48, cWhite}}},
		{name: "plus-clip-path.emf", data: scenePlusClipPath(), probes: []probe{{6, 10, cOrange}, {40, 50, cWhite}, {75, 50, cOrange}, {75, 20, cWhite}, {64, 8, cGreen}, {80, 24, cWhite}, {93, 40, cGreen}}},
		{name: "plus-order.emf", data: scenePlusOrder(), probes: []probe{{28, 14, cRed}, {14, 14, cWhite}, {52, 8, cBlue}, {24, 41, cBlack}, {24, 48, cWhite}, {75, 45, cGreen}, {75, 27, cGreen}, {75, 23, cWhite}}},
		{name: "plus-caps.emf", data: scenePlusCaps(false), probes: []probe{{73, 12, cBlack}, {18, 12, cWhite}, {17, 32, cBlack}, {72, 32, cWhite}}},
		{name: "lo-plus-square-anchor.emf", divergence: "LineCapTypeSquareAnchor is drawn wider than the line; MS-EMFPLUS 2.1.1.17 gives it the line width",
			data:   scenePlusCaps(true),
			probes: []probe{{73, 52, cBlack}, {18, 52, cWhite}, {70, 46, cWhite}}, libreOffice: []probe{{70, 46, cBlack}}},
		{name: "lo-plus-compound.emf", divergence: "compound pens are drawn as one solid line of the full width",
			data:   scenePlusCompound(),
			probes: []probe{{48, 12, cBlack}, {48, 16, cWhite}, {48, 21, cBlack}, {48, 8, cWhite}, {48, 24, cWhite}, {48, 32, cBlack}, {48, 34, cWhite}, {48, 37, cBlack}, {48, 46, cWhite}}, libreOffice: []probe{{48, 16, cBlack}, {48, 34, cBlack}}},
		{name: "lo-plus-path-gradient.emf", divergence: "path gradients are drawn as an elliptical blend that also covers the filled area outside the boundary",
			data:   scenePlusPathGradient(),
			probes: []probe{{32, 32, cRed}, {10, 32, color.NRGBA{21, 0, 234, 255}}, {70, 32, cWhite}, {32, 5, cWhite}}, libreOffice: []probe{{10, 32, color.NRGBA{153, 0, 102, 255}}, {70, 32, color.NRGBA{77, 0, 179, 255}}}},
		// Opt-in interpretation: the triangle's tip reaches 8 units past the
		// line end at x 60; LibreOffice draws it pointing back over the line.
		{name: "lo-plus-custom-cap.emf", customCaps: true, divergence: "a custom path cap is drawn pointing back along the line (the coordinate system is unspecified; playback's opt-in interpretation points it outward)",
			data:   scenePlusCustomCap(),
			probes: []probe{{65, 12, cBlack}, {69, 12, cWhite}, {40, 12, cBlack}}, libreOffice: []probe{{65, 12, cWhite}, {40, 12, cBlack}}},
		{name: "plus-image.emf", data: scenePlusImage(false), probes: []probe{{16, 16, cRed}, {32, 16, cLime}, {16, 32, cBlue}, {32, 32, cYellow}, {4, 4, cWhite}, {62, 16, cRed}, {74, 32, cYellow}, {90, 40, cWhite}}},
		{name: "plus-image-fraction.emf", data: scenePlusImageFraction(), probes: []probe{
			{12, 12, cRed}, {26, 12, cLime}, {12, 26, cBlue}, {26, 26, cYellow}, {7, 16, cWhite}, {32, 16, cWhite}, {16, 7, cWhite}, {16, 32, cWhite},
			{52, 12, cRed}, {68, 12, cLime}, {52, 28, cBlue}, {68, 28, cYellow}, {47, 20, cWhite}, {72, 20, cWhite}, {60, 7, cWhite}, {60, 32, cWhite}}},
		{name: "plus-metafile-image.emf", data: scenePlusMetafileImage(false), probes: []probe{
			{12, 12, cRed}, {28, 24, cRed}, {52, 20, cBlue}, {6, 30, cWhite}, {44, 6, cWhite},
			{56, 52, cRed}, {80, 52, cBlue}, {70, 52, cWhite}, {48, 52, cWhite}}},
		{name: "lo-plus-metafile-source.emf", divergence: "the source rectangle of a metafile image is ignored; the whole picture is drawn",
			data:   scenePlusMetafileImage(true),
			probes: []probe{{55, 52, cRed}, {80, 52, cBlue}, {62, 52, cWhite}, {12, 24, cRed}, {23, 24, cWhite}, {40, 24, cBlue}}, libreOffice: []probe{{62, 52, cRed}, {23, 24, cRed}}},
		{name: "lo-plus-nearest.emf", divergence: "InterpolationModeNearestNeighbor is ignored; scaled images are smoothed",
			data:   scenePlusNearest(),
			probes: []probe{{10, 10, cRed}, {22, 16, cRed}, {26, 16, cLime}, {38, 38, cYellow}}, libreOffice: []probe{{10, 10, cRed}, {22, 16, color.NRGBA{149, 103, 4, 255}}, {26, 16, color.NRGBA{90, 167, 2, 255}}}},
		{name: "lo-plus-page-change.emf", divergence: "a second EmfPlusSetPageTransform does not take effect; later drawing keeps the first page units (here, off the page)",
			data:   scenePlusPageChange(),
			probes: []probe{{60, 18, cBlue}, {12, 48, cBlack}}, libreOffice: []probe{{60, 18, cBlue}, {12, 48, cWhite}}},
		{name: "lo-plus-raw-bitmap.emf", divergence: "uncompressed EMF+ bitmaps (BitmapDataTypePixel) draw nothing",
			data:   scenePlusImage(true),
			probes: []probe{{16, 16, cRed}, {32, 32, cYellow}, {62, 16, cRed}}, libreOffice: []probe{{16, 16, cWhite}, {32, 32, cWhite}, {62, 16, cWhite}}},
		{name: "lo-plus-texture.emf", divergence: "EMF+ texture brushes paint nothing",
			data:   scenePlusTexture(),
			probes: []probe{{10, 42, cRed}, {14, 42, cGreen}, {10, 46, cBlue}, {14, 46, cYellow}}, libreOffice: []probe{{10, 42, cWhite}, {14, 46, cWhite}}},
		{name: "lo-plus-gradients.emf", divergence: "linear gradient brushes are drawn in discrete color bands",
			data: scenePlusGradients(),
			probes: []probe{
				{48, 12, gray(128)}, {12, 12, gray(13)}, {84, 12, gray(242)},
				{48, 30, cGreen}, {28, 30, color.NRGBA{128, 96, 0, 255}}, {68, 30, color.NRGBA{0, 96, 128, 255}},
				{62, 50, gray(191)}, {70, 50, gray(64)}},
			// Observed bands: 23 over [12,18], then 46, ... 232; mirrored 204 and 51.
			libreOffice: []probe{{12, 12, gray(23)}, {16, 12, gray(23)}, {20, 12, gray(46)}, {48, 12, gray(138)}, {84, 12, gray(232)}, {48, 30, cGreen}, {62, 50, gray(204)}, {70, 50, gray(51)}}},
	}
}

var cYellow, cLime = color.NRGBA{255, 255, 0, 255}, color.NRGBA{0, 255, 0, 255}

func gray(v uint8) color.NRGBA { return color.NRGBA{v, v, v, 255} }

// Rectangles, an ellipse, a quarter pie (clockwise from the x axis), an
// alternate-filled path of two overlapping squares, a polygon, a wide line and
// a translucent fill.
func scenePlusShapes() []byte {
	squares := plusPathObj([]float64{4, 28, 24, 28, 24, 48, 4, 48, 14, 38, 34, 38, 34, 58, 14, 58}, []byte{0, 1, 1, 0x81, 0, 1, 1, 0x81})
	return plusScene96(
		fillRect(0xffff0000, 4, 4, 20, 16),
		plusRec(PlusFillEllipseRecord, 0x8000, dwords(0xff0000ff), fl(30, 4, 24, 16)),
		plusRec(PlusFillPieRecord, 0x8000, dwords(0xff00c000), fl(0, 90), fl(60, 4, 32, 32)),
		plusObj(2, 3, squares),
		plusRec(PlusFillPathRecord, 0x8000|2, dwords(0xffff00ff)),
		plusRec(PlusFillPolygonRecord, 0x8000, dwords(0xffff8000, 3), fl(40, 30, 60, 30, 40, 60)),
		plusObj(1, 2, plusPen(0, 0, 3, nil, solidBrush(0xff000000))),
		plusRec(PlusDrawLinesRecord, 1, dwords(2), fl(64, 50, 92, 50)),
		fillRect(0x800000ff, 64, 54, 28, 8),
	)
}

// A rotated world transform, a container mapping a 4-unit square onto 20
// pixels, a restored saved state and an inch page unit at 1/8 scale (last:
// see lo-plus-page-change.emf).
func scenePlusTransforms() []byte {
	s, c := math.Sincos(math.Pi / 6)
	return plusScene96(
		plusRec(PlusSetWorldTransformRecord, 0, fl(c, s, -s, c, 20, 20)),
		fillRect(0xffff0000, 0, 0, 16, 8),
		plusRec(PlusResetWorldTransformRecord, 0),
		plusRec(PlusBeginContainerRecord, 2<<8, fl(72, 36, 20, 20, 0, 0, 4, 4), dwords(1)),
		fillRect(0xff00c000, 1, 1, 2, 2),
		plusRec(PlusEndContainerRecord, 0, dwords(1)),
		plusRec(PlusSaveRecord, 0, dwords(2)),
		plusRec(PlusTranslateWorldTransformRecord, 0, fl(100, 100)),
		plusRec(PlusRestoreRecord, 0, dwords(2)),
		fillRect(0xff000000, 4, 40, 16, 16),
		plusRec(PlusSetPageTransformRecord, 4, fl(0.125)),
		fillRect(0xff0000ff, 4, 0.5, 2, 2),
	)
}

// A triangular path clip united with a rectangle, then two rectangles
// combined by xor. Clip edges stay off the canvas border, where the
// comparison neighborhood is one-sided.
func scenePlusClipPath() []byte {
	return plusScene96(
		plusObj(1, 3, plusPathObj([]float64{2, 2, 48, 2, 2, 62}, []byte{0, 1, 1})),
		plusRec(PlusSetClipPathRecord, 1),
		plusRec(PlusSetClipRectRecord, 2<<8, fl(60, 40, 30, 20)),
		fillRect(0xffff8000, 0, 0, 96, 64),
		plusRec(PlusSetClipRectRecord, 0, fl(60, 4, 30, 30)),
		plusRec(PlusSetClipRectRecord, 3<<8, fl(70, 14, 30, 30)),
		fillRect(0xff00c000, 0, 0, 96, 64),
	)
}

// Prepended and appended world transform records, a Bézier and a closed
// cardinal spline whose edges bulge outward.
func scenePlusOrder() []byte {
	return plusScene96(
		plusRec(PlusTranslateWorldTransformRecord, 0, fl(20, 10)),
		plusRec(PlusScaleWorldTransformRecord, 0, fl(2, 1)),
		fillRect(0xffff0000, 0, 0, 8, 8),
		plusRec(PlusResetWorldTransformRecord, 0),
		plusRec(PlusRotateWorldTransformRecord, 0, fl(90)),
		plusRec(PlusTranslateWorldTransformRecord, 0x2000, fl(60, 0)),
		fillRect(0xff0000ff, 4, 4, 8, 12),
		plusRec(PlusResetWorldTransformRecord, 0),
		plusObj(1, 2, plusPen(0, 0, 2, nil, solidBrush(0xff000000))),
		plusRec(PlusDrawBeziersRecord, 1, dwords(4), fl(8, 56, 8, 36, 40, 36, 40, 56)),
		plusRec(PlusFillClosedCurveRecord, 0x8000, dwords(0xff00c000), fl(0.5), dwords(4), fl(60, 30, 90, 30, 90, 60, 60, 60)),
	)
}

// An inch page unit followed by a return to pixels.
func scenePlusPageChange() []byte {
	return plusScene96(
		plusRec(PlusSetPageTransformRecord, 4, fl(0.125)),
		fillRect(0xff0000ff, 4, 0.5, 2, 2),
		plusRec(PlusSetPageTransformRecord, 2, fl(1)),
		fillRect(0xff000000, 4, 40, 16, 16),
	)
}

// An excluded rectangle clip, then a region-tree clip keeping B minus A
// (RegionNodeDataTypeComplement).
func scenePlusClip() []byte {
	return plusScene96(
		plusRec(PlusSetClipRectRecord, 0, fl(8, 8, 40, 48)),
		plusRec(PlusSetClipRectRecord, 4<<8, fl(28, 8, 40, 48)),
		fillRect(0xffff0000, 0, 0, 96, 64),
		plusRec(PlusResetClipRecord, 0),
		plusObj(2, 4, regionObj(dwords(5), rectNode(50, 8, 20, 48), rectNode(60, 8, 30, 48))),
		plusRec(PlusSetClipRegionRecord, 2),
		fillRect(0xff0000ff, 0, 0, 96, 64),
	)
}

// A wide rectangle outline, round caps reaching past a line's end, a
// clockwise quarter arc and an open cardinal spline through its points.
func scenePlusPens() []byte {
	return plusScene96(
		plusObj(1, 2, plusPen(0, 0, 4, nil, solidBrush(0xff000000))),
		plusRec(PlusDrawRectsRecord, 1, dwords(1), fl(8, 8, 30, 20)),
		plusObj(2, 2, plusPen(2|4, 0, 6, dwords(2, 2), solidBrush(0xff00c000))),
		plusRec(PlusDrawLinesRecord, 2, dwords(2), fl(50, 10, 80, 10)),
		plusObj(3, 2, plusPen(0, 0, 3, nil, solidBrush(0xff0000ff))),
		plusRec(PlusDrawArcRecord, 3, fl(0, 90), fl(50, 20, 40, 40)),
		plusObj(4, 2, plusPen(0, 0, 3, nil, solidBrush(0xffff0000))),
		plusRec(PlusDrawCurveRecord, 4, fl(0.5), dwords(0, 2, 3), fl(8, 40, 24, 56, 40, 40)),
	)
}

// Linear gradients: two-color, preset colors and mirrored (TileFlipX).
func scenePlusGradients() []byte {
	return plusScene96(
		plusObj(1, 1, linearBrush(0, 0, [4]float64{8, 4, 80, 16}, 0xff000000, 0xffffffff)),
		plusRec(PlusFillRectsRecord, 0, dwords(1, 1), fl(8, 4, 80, 16)),
		plusObj(2, 1, linearBrush(4, 0, [4]float64{8, 24, 80, 12}, 0, 0, dwords(3), fl(0, 0.5, 1), dwords(0xffff0000, 0xff00c000, 0xff0000ff))),
		plusRec(PlusFillRectsRecord, 0, dwords(2, 1), fl(8, 24, 80, 12)),
		plusObj(4, 1, linearBrush(0, 1, [4]float64{56, 0, 8, 1}, 0xff000000, 0xffffffff)),
		plusRec(PlusFillRectsRecord, 0, dwords(4, 1), fl(56, 40, 16, 20)),
	)
}

// A texture brush whose transform scales image pixels to 4x4 world units.
func scenePlusTexture() []byte {
	return plusScene96(
		plusObj(3, 1, cat(dwords(plusVersion, 2, 2, 0), fl(4, 0, 0, 4, 0, 0), pngQuadrants(2, color.NRGBA{0, 192, 0, 255}))),
		plusRec(PlusFillRectsRecord, 0, dwords(3, 1), fl(8, 40, 40, 20)),
	)
}

// pngQuadrants is a PNG-encoded n x n EMF+ bitmap of red, green / blue,
// yellow quadrants.
func pngQuadrants(n int, green color.NRGBA) []byte {
	im := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			im.SetNRGBA(x, y, [2][2]color.NRGBA{{cRed, green}, {cBlue, cYellow}}[2*y/n][2*x/n])
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		panic(err)
	}
	// MS-EMFPLUS 2.2.2.2 calls these fields undefined for compressed
	// bitmaps, but GDI+ draws nothing unless they describe the image.
	return cat(dwords(plusVersion, 1), longs(int32(n), int32(n), int32(4*n)), dwords(PixelFormat32bppARGB, 1), b.Bytes())
}

// A 16x16 bitmap scaled into a rectangle and sheared onto a parallelogram;
// PNG-encoded or raw 32bppARGB pixels.
func scenePlusImage(raw bool) []byte {
	image := pngQuadrants(16, cLime)
	if raw {
		var px []uint32
		for y := 0; y < 16; y++ {
			for x := 0; x < 16; x++ {
				px = append(px, [2][2]uint32{{0xffff0000, 0xff00ff00}, {0xff0000ff, 0xffffff00}}[y/8][x/8])
			}
		}
		image = argbImage(16, 16, px...)
	}
	return plusScene96(
		plusObj(1, 5, image),
		plusRec(PlusDrawImageRecord, 1, dwords(0xffffffff, 2), fl(0, 0, 16, 16), fl(8, 8, 32, 32)),
		plusRec(PlusDrawImagePointsRecord, 1, dwords(0xffffffff, 2), fl(0, 0, 16, 16), dwords(3), fl(56, 8, 88, 8, 48, 40)),
	)
}

// Source rectangles with fractional edges, scaled 3 times: (3.5, 3.5) to
// (11.5, 11.5) of a 16x16 quadrant image onto (8, 8)-(32, 32), with the
// quadrant boundary at 21.5, and (2.5, 2.5) to (10.5, 10.5) onto the points
// (48, 8), (72, 8), (48, 32), with the boundary at 64.5. The whole pixels
// enclosing the sources would reach 1.5 pixels further out on every side.
func scenePlusImageFraction() []byte {
	return plusScene96(
		plusRec(PlusSetPixelOffsetModeRecord, 4),
		plusObj(1, 5, pngQuadrants(16, cLime)),
		plusRec(PlusDrawImageRecord, 1, dwords(0xffffffff, 2), fl(3.5, 3.5, 8, 8), fl(8, 8, 24, 24)),
		plusRec(PlusDrawImagePointsRecord, 1, dwords(0xffffffff, 2), fl(2.5, 2.5, 8, 8), dwords(3), fl(48, 8, 72, 8, 48, 32)),
	)
}

// Embedded metafiles drawn as images: a 40 x 20 pixel EMF, polygons in a
// red left half and a blue right half each with a 2-unit white margin,
// scaled twice onto (4,4)-(84,44), and a placeable WMF of the same picture in
// 40 x 20 logical units onto (50,44)-(90,60). With part, the images show
// only their source rectangles from x 10 onward: (10,0)-(40,20) of the EMF
// onto (4,4)-(64,44), with red to about 21 and blue from about 27 (the
// 40-pixel frame fills a 41-pixel image), and of the WMF onto
// (50,44)-(90,60), where red ends at 50+8*4/3 and blue starts at 50+11*4/3.
func scenePlusMetafileImage(part bool) []byte {
	square := func(x0, x1 int32) []int32 { return []int32{x0, 2, x1, 2, x1, 18, x0, 18} }
	emf := emfScene(40, 20, 3, emfSelect(nullPen), emfBrush(1, 0, red), emfBrush(2, 0, blue),
		emfSelect(1), emfPoints(EMRPolygon, square(2, 18)...), emfSelect(2), emfPoints(EMRPolygon, square(21, 38)...))
	wsquare := func(x0, x1 int16) []int16 { return []int16{x0, 2, x1, 2, x1, 18, x0, 18} }
	wmf := wmfScene(40, 20, 3, wmfRec(MetaSetMapMode, 8), wmfRec(MetaSetWindowOrg, 0, 0), wmfRec(MetaSetWindowExt, 20, 40),
		wmfPen(5, 0, 0), wmfRec(MetaSelectObject, 0), wmfBrush(0, red, 0), wmfRec(MetaSelectObject, 1), wmfPoly(MetaPolygon, wsquare(2, 18)...),
		wmfBrush(0, blue, 0), wmfRec(MetaSelectObject, 2), wmfPoly(MetaPolygon, wsquare(21, 38)...))
	// The whole of the EMF is its 41 x 21 pixel image, as GDI+ records it.
	emfSrc, wmfSrc := [4]float64{0, 0, 41, 21}, [4]float64{0, 0, 40, 20}
	emfDest := [4]float64{4, 4, 80, 40}
	if part {
		emfSrc, wmfSrc, emfDest = [4]float64{10, 0, 30, 20}, [4]float64{10, 0, 30, 20}, [4]float64{4, 4, 60, 40}
	}
	return plusScene96(
		plusRec(PlusSetPixelOffsetModeRecord, 4),
		plusObj(1, 5, metafileImageObj(3, emf)),
		plusObj(2, 5, metafileImageObj(2, wmf)),
		drawImage(1, emfSrc, emfDest),
		drawImage(2, wmfSrc, [4]float64{50, 44, 40, 16}),
	)
}

// A 2x2 bitmap scaled 16 times with nearest-neighbor interpolation.
func scenePlusNearest() []byte {
	return plusScene96(
		plusObj(1, 5, pngQuadrants(2, cLime)),
		plusRec(PlusSetInterpolationModeRecord, 5),
		plusRec(PlusDrawImageRecord, 1, dwords(0xffffffff, 2), fl(0, 0, 2, 2), fl(8, 8, 32, 32)),
	)
}

// Width-8 lines with flat/square and round/flat caps at their start and
// end, or one with NoAnchor/SquareAnchor caps.
func scenePlusCaps(anchor bool) []byte {
	black := solidBrush(0xff000000)
	line := func(id uint16, y float64) []byte {
		return plusRec(PlusDrawLinesRecord, id, dwords(2), fl(20, y, 70, y))
	}
	if anchor {
		return plusScene96(plusObj(3, 2, plusPen(2|4, 0, 8, dwords(0x10, 0x11), black)), line(3, 52))
	}
	return plusScene96(
		plusObj(1, 2, plusPen(2|4, 0, 8, dwords(0, 1), black)), line(1, 12),
		plusObj(2, 2, plusPen(2|4, 0, 8, dwords(2, 0), black)), line(2, 32),
	)
}

// Compound pens: two outer quarter bands of a 12-wide line, and an 8-wide
// rectangle outline split by a central gap of a quarter of its width.
func scenePlusCompound() []byte {
	black := solidBrush(0xff000000)
	return plusScene96(
		plusObj(1, 2, plusPen(1024, 0, 12, cat(dwords(4), fl(0, .25, .75, 1)), black)),
		plusRec(PlusDrawLinesRecord, 1, dwords(2), fl(10, 16, 86, 16)),
		plusObj(2, 2, plusPen(1024, 0, 8, cat(dwords(4), fl(0, .375, .625, 1)), black)),
		plusRec(PlusDrawRectsRecord, 2, dwords(1), fl(20, 34, 56, 24)),
	)
}

// A path gradient from a red center to a blue square boundary, filling a
// wider rectangle: nothing is painted outside the boundary.
func scenePlusPathGradient() []byte {
	square := plusPathObj([]float64{8, 8, 56, 8, 56, 56, 8, 56}, []byte{0, 1, 1, 0x81})
	return plusScene96(
		plusObj(1, 1, pathGradientBrush(1, 4, 0xffff0000, 32, 32, []uint32{0xff0000ff}, square, nil)),
		plusRec(PlusFillRectsRecord, 0, dwords(1, 1), fl(8, 8, 80, 48)),
	)
}

// A width-4 line with a triangular custom end cap (base across the end,
// apex 2 widths along the line).
func scenePlusCustomCap() []byte {
	triangle := plusPathObj([]float64{-1, 0, 1, 0, 0, 2}, []byte{0, 1, 0x81})
	cap := pathCapObj(1, 0, 0, 1, triangle, nil)
	return plusScene96(
		plusRec(PlusSetPixelOffsetModeRecord, 4),
		plusObj(1, 2, plusPen(4|0x1000, 0, 4, cat(dwords(0xff), dwords(uint32(len(cap))), cap), solidBrush(0xff000000))),
		plusRec(PlusDrawLinesRecord, 1, dwords(2), fl(20, 12, 60, 12)),
	)
}
