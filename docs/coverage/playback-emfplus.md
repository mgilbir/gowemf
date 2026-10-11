# EMF+ playback coverage

Which EMF+ records `Play` draws and which it reports through
`PlayOptions.Unsupported`. Why it draws them that way is in
[EMF+ playback interpretations](../interpretations/emfplus.md).

`Play` interprets EMF+ records by default; `PlayOptions.Stream.PreferGDI` selects
the GDI fallback of a Dual file instead. GDI records inside GetDC intervals are
played by the GDI player, with its own device context.

Families: [State](#state) · [Clipping](#clipping) · [Brushes](#brushes) · [Pens](#pens) · [Geometry](#geometry) · [Images](#images) · [Text](#text).

## State

**Played:** Save/Restore, BeginContainer (World or Pixel units) and BeginContainerNoParams, EndContainer; Set/Reset/Multiply/Translate/Scale/RotateWorldTransform in prepend and append order; SetPageTransform in Pixel, Point, Inch, Document and Millimeter units; pixel offset, interpolation and compositing modes; rendering origin. Anti-aliasing, text hints and compositing quality are backend policy.

**Reported as unsupported:** Display and World page units; containers in physical units (their contents are skipped as a unit); SetTSGraphics and SetTSClip

## Clipping

**Played:** SetClipRect, SetClipPath, SetClipRegion with all six combine modes; region trees with every node type; ResetClip, OffsetClip; container clips as metaregions

**Reported as unsupported:** nothing.

## Brushes

**Played:** Solid colors with alpha, hatch styles 0–52 with GDI+'s patterns, texture brushes (bitmap images, brush transform, all five wrap modes), linear gradients (two colors, blend factors, preset colors; tile and mirrored wraps), path gradient fills through a `GradientBackend` (path boundary star-shaped from its center, one surrounding color or one per vertex, preset colors, blend factors with one surrounding color, brush transform, clamped)

**Reported as unsupported:** Translucent hatches with anti-aliased patterns (styles 2, 3 and 5), metafile textures, linear gradients that are gamma corrected, clamped, have vertical blend factors or stops of differing alpha; path gradients with point (cardinal spline) boundaries, blend factors with several surrounding colors, focus scales, gamma correction, tiled wraps, several figures or non-star-shaped boundaries, and path gradients on pens and text

## Pens

**Played:** World and Pixel widths, pen transform, flat/square/round caps chosen separately for start and end, NoAnchor and SquareAnchor caps, custom path and adjustable-arrow caps with `PlayOptions.CustomLineCaps` (see [line caps](../interpretations/emfplus.md#line-caps-and-custom-caps)), miter/bevel/round joins, miter limit, predefined and custom dashes with offset, symmetric compound lines with miter and round joins, any supported brush

**Reported as unsupported:** Custom and adjustable-arrow caps without `CustomLineCaps`, or with translucent paint, dashes, compound or zero-width pens, inconsistent cap data or insets past a line segment; triangle, round, diamond and arrow anchor caps, dashed pens with non-flat line caps, asymmetric compound lines, compound lines with bevel joins or dashes, open-figure caps or mitered corners past the miter limit, clipped miter joins, alignments other than center, dash caps, dashed zero-width pens, other width units

## Geometry

**Played:** FillRects/DrawRects, FillPolygon/DrawLines (closed and open), FillEllipse/DrawEllipse, FillPie/DrawPie/DrawArc, FillPath/DrawPath, FillRegion, cardinal splines (open with offset and segments, closed, winding or alternate fill), DrawBeziers, Clear; compressed and relative points

**Reported as unsupported:** nothing.

## Images

**Played:** DrawImage and DrawImagePoints of bitmap images (encoded and raw) and of EMF, EMF+ and placeable WMF metafile images, with whole-pixel or fractional source subrectangles inside the image or clamped to transparent

**Reported as unsupported:** Metafile images without placeable bounds, nested deeper than `MaxMetafileDepth` or under SourceCopy; image effects, sources outside the image whose image attributes tile or clamp to a visible color, alpha images under SourceCopy

## Text

**Played:** DrawDriverString through a `TextBackend`: Unicode code units or glyph indexes at explicit baseline origins or realized advances, fonts in every defined size unit with bold, italic, underline and strikeout styles, any supported brush, translation matrices

**Reported as unsupported:** DrawString layout; vertical driver strings; driver-string matrices other than translations; Display-unit fonts; backends without `TextBackend`

