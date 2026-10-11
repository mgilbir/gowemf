# GDI playback coverage

Which WMF and EMF records `Play` draws and which it reports through
`PlayOptions.Unsupported`. Reach for this to find out whether a feature will
appear in your output. Why `Play` draws a feature the way it does is in
[GDI playback interpretations](../interpretations/gdi.md); how the output was
checked is in [testing](../testing/README.md).

`Play` interprets the WMF and EMF GDI streams, and the GDI records inside EMF+
GetDC intervals. EMF+ playback is described in
[EMF+ playback coverage](playback-emfplus.md).

Families: [State](#state) · [Objects](#objects) · [Geometry](#geometry) · [Paths](#paths) · [Clipping](#clipping) · [Bitmaps](#bitmaps) · [Text](#text) · [Fills](#fills).

## State

**Played:** WMF escapes and comments, which do not draw on a display device; SaveDC/RestoreDC (relative and WMF absolute), map modes 1–8, window/viewport origin/extent/offset/scale, Set/ModifyWorldTransform (all four modes), background mode/color, poly-fill mode, ROP2, stretch mode, arc direction, miter limit, brush origin, current position

**Reported as unsupported:** Right-to-left layout; ICM conversion with a non-sRGB source, output profile or proofing target

## Objects

**Played:** Pens (LogPen and ExtCreatePen styles, caps, joins, user dashes, hatched pen brushes), solid/null/hatch, DIB and monochrome pattern brushes (EMR_CREATEMONOBRUSH and 1-bit Bitmap16 patterns), stock objects including DC_PEN/DC_BRUSH defaults, logical palettes (create, select, set, animate, resize) with PALETTEINDEX and PALETTERGB colors and DIB_PAL_COLORS bitmaps, WMF lowest-free-slot reuse, EMF handle reuse

**Reported as unsupported:** Colored Bitmap16 patterns, DIB pattern pens, dithered hatch styles, DIB_PAL_INDICES bitmaps, palettes after EMR_COLORCORRECTPALETTE, PALETTEINDEX with the default palette

## Geometry

**Played:** Polygons/polylines/polypolygons/polypolylines (16/32-bit), Bézier and "To" forms, PolyDraw, LineTo/MoveTo, Rectangle, RoundRect, Ellipse, Arc/ArcTo/Chord/Pie, AngleArc, SetPixel

**Reported as unsupported:** nothing.

## Paths

**Played:** Begin/End/Abort, CloseFigure, FlattenPath, FillPath, StrokePath, StrokeAndFillPath, SelectClipPath

**Reported as unsupported:** WidenPath; text inside a path bracket

## Clipping

**Played:** IntersectClipRect, ExcludeClipRect, OffsetClipRgn, SelectClipPath and ExtSelectClipRgn with all five modes, omitted-region reset, SetMetaRgn, WMF SelectClipRegion and region SelectObject, save/restore

**Reported as unsupported:** nothing.

## Bitmaps

**Played:** StretchDIBits, SetDIBitsToDevice, BitBlt, StretchBlt, MaskBlt without a mask, PlgBlt without a mask, AlphaBlend (constant and per-pixel alpha), TransparentBlt; WMF DIBBitBlt, DIBStretchBlt, StretchDIB, SetDIBToDev, PatBlt; mirroring, partial and clamped sources, scale/translate source transforms, HALFTONE hint; every other ROP3 (SRCAND, SRCPAINT, SRCINVERT, DSTINVERT, PATINVERT…) through a `RasterBackend`; SRCAND and SRCPAINT sprite pairs as masked images for other backends

**Reported as unsupported:** Masks, Bitmap16 and device-to-device sources, partial scan-line buffers, rotated/sheared source transforms, ROP3 other than SRCCOPY, NOTSRCCOPY, PATCOPY, BLACKNESS, WHITENESS and DSTCOPY for backends without `RasterBackend`, apart from exact sprite pairs, such ROP3 with a hatch brush in TRANSPARENT mode, halftone with a color adjustment

## Text

**Played:** TextOut, ExtTextOut (A/W, WMF), PolyTextOut, SmallTextOut through a `TextBackend`: fonts and stock fonts, text color, all alignment flags, TA_UPDATECP, explicit advances (and ETO_PDY without vertical displacement), character extra, justification, escapement and orientation, opaque and clip rectangles, OPAQUE background cells, glyph indexes, UTF-16, the ten single-byte and four double-byte (Shift_JIS, GBK, Unified Hangul, Big5) Windows code pages for text and WMF face names, and symbol fonts, bidirectional text and right-to-left reading order (ETO_RTLREADING, TA_RTLREADING)

**Reported as unsupported:** Backends without `TextBackend`; DEFAULT_CHARSET without `DefaultCharSet`; JOHAB, OEM and Mac character sets; vertical (`@`) fonts; ETO_PDY vertical displacement; text in path brackets

## Fills

**Played:** FillRgn, PaintRgn, FrameRgn; WMF FillRegion, PaintRegion, FrameRegion; EMR_GRADIENTFILL rectangle and triangle modes through a `GradientBackend`

**Reported as unsupported:** InvertRgn and InvertRegion and flood fill, which read the destination; gradients for backends without `GradientBackend`


Hatches and predefined dash patterns are delivered as styles with their device
pattern grid; their pixel patterns are drawn by the backend. Path and clip
geometry is retained in destination coordinates, so a path survives transform
changes made after it is recorded. Each clip step is an immutable node shared by
saved states; the chain length is bounded across the clip and metaregions.
