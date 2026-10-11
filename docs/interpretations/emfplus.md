# EMF+ playback interpretations

How `Play` resolves the places where MS-EMFPLUS leaves room. Read this when
EMF+ output surprises you or before changing a rule. Each ruling lives only
here; other documents link to it. The supported-record inventory is in
[EMF+ playback coverage](../coverage/playback-emfplus.md).

| Ruling | Decision | Windows/oracle evidence |
| --- | --- | --- |
| [Coordinate spaces and page units](#coordinate-spaces-and-page-units) | World → container → page transforms to EMF device pixels; page units use the header DPI | — |
| [Pixel offset mode](#pixel-offset-mode) | Default, HighSpeed and None shift geometry by half a device pixel; Half and HighQuality do not | — |
| [Containers](#containers) | A container maps source onto destination; its clip constrains drawing as a metaregion | — |
| [Fill rules](#fill-rules) | Paths fill alternate (GDI+'s default); FillClosedCurve follows its flag | — |
| [Arc and pie angles](#arc-and-pie-angles) | Geometric angles, clockwise; sweeps limited to ±360 degrees | — |
| [Cardinal splines](#cardinal-splines) | Béziers with tension/3 control offsets; open curves repeat end points | — |
| [Linear gradients](#linear-gradients) | Left edge 0 to right edge 1 in brush space; factors interpolate linearly in sRGB | — |
| [Pen widths](#pen-widths) | World widths follow pen and world transforms; Pixel widths are device pixels; default miter limit 10 | — |
| [Line caps and custom caps](#line-caps-and-custom-caps) | Spec-defined caps drawn; custom and arrow caps only with CustomLineCaps, matching GDI+-recorded caps | [link](../testing/windows-oracle.md#confirmed-interpretations) |
| [Dashed pens and caps](#dashed-pens-and-caps) | Dashed pens need flat line caps, because Stroke has one cap per end | — |
| [Compound lines](#compound-lines) | Symmetric arrays only; bands are differences of strokes; bevel joins reported | [link](../testing/windows-oracle.md#compound-pens-with-bevel-joins) |
| [Path gradients](#path-gradients) | Star-shaped boundaries become Gouraud fans; blend 0 is the boundary, 1 the center | [link](../testing/windows-oracle.md#confirmed-interpretations) |
| [Image source rectangles](#image-source-rectangles) | Fractional sources map exactly, clipped to the exact rectangle | [link](../testing/windows-oracle.md#confirmed-interpretations) |
| [Metafile images](#metafile-images) | Played from default state as GDI+ records them; budgets shared with the Play call | [link](../testing/windows-oracle.md#metafile-images) |
| [Hatch styles](#hatch-styles) | GDI+'s measured 8x8 patterns as PaintPattern from the rendering origin | [link](../testing/windows-oracle.md#hatch-styles) |
| [SourceCopy compositing](#sourcecopy-compositing) | Opaque SourceCopy is drawn; translucent paint is reported | — |
| [Font sizes](#font-sizes) | World-unit sizes are world units; physical sizes convert with the vertical DPI | — |
| [Driver-string matrices](#driver-string-matrices) | Translations are applied in world space; other matrices are reported | — |
| [DrawString](#drawstring) | Reported: its layout rules are not specified | — |
| [Bitmap budgets](#bitmap-budgets) | Bitmaps decode once per object and are charged to the playback pixel budget | — |

## Coordinate spaces and page units

World coordinates map to EMF device pixels through the world, container and
page transforms. Page units convert with the EMF+ header's logical DPI. With
no SetPageTransform the page is one device pixel per unit at scale 1.

## Pixel offset mode

Under PixelOffsetMode Default, HighSpeed and None, integer device
coordinates are pixel centers (MS-EMFPLUS 2.1.1.26). All geometry, clipping
and images shift by half a device pixel; Half and HighQuality do not.

## Containers

A container maps its source rectangle onto its destination rectangle in the
enclosing world space. The world transform and clip start reset, and the
enclosing clip constrains drawing as a metaregion.

## Fill rules

EmfPlusPath carries no fill mode, so FillPath, path clips and path region
nodes use GDI+'s default alternate rule. FillPolygon is alternate, and
FillClosedCurve follows its winding flag.

## Arc and pie angles

Pie and arc angles are clockwise from the x axis to the ray through the
point, so elliptical arcs use geometric, not parametric, angles. Sweeps are
limited to ±360 degrees. An empty DrawArc draws nothing.

## Cardinal splines

Cardinal spline segments become Béziers whose control points are offset by
tension/3 of the neighboring chord; open curves repeat their end points.

## Linear gradients

Linear gradients run from the left edge (0) to the right edge (1) of the
brush rectangle in brush space, which the brush transform places in world
space. Blend factors are fractions of the end color, interpolated linearly
in sRGB.

## Pen widths

World-unit pen widths follow the pen and world transforms. Pixel-unit widths
are device pixels. A zero width is a hairline. Unset miter limits are 10,
GDI+'s default.

## Line caps and custom caps

Line caps follow MS-EMFPLUS 2.1.1.17 where it gives the geometry: Flat,
Square, Round, NoAnchor (ends at the last point, like Flat) and
SquareAnchor (a square of the line width centered on the end, like Square).
The triangle's height and the other anchors' sizes are not given. Neither
MS-EMFPLUS nor Microsoft's GDI+ reference gives the coordinate system of
custom cap paths or where an adjustable arrow's vertex sits, so those caps
are reported unless `PlayOptions.CustomLineCaps` opts into this
interpretation. Windows GDI+ draws the adjustable arrow, fill-path end cap
and stroke-path start cap it records itself the same way (`rec-caps.emf`,
[evidence](../testing/windows-oracle.md#confirmed-interpretations)):

- Cap space has its origin at the figure's end. +y points outward along
  the line (back along the first segment for a start cap), and +x is +y
  turned a quarter clockwise in pen space. Units are the pen width times
  the cap's WidthScale.
- Path caps fill their fill path with the winding rule, or stroke their
  line path with the pen width and the cap's stroke caps and join. The line
  path wins when both are present, as Microsoft's CustomLineCap reference
  states. The figure ends with BaseCap, shortened by BaseInset.
- An adjustable arrow has its vertex at the end. Its base is Width units
  across and Height units back, with its midpoint moved MiddleInset units
  toward the vertex (Microsoft's AdjustableArrowCap reference). It is
  filled, or outlined when FillState is clear, and the figure ends flat at
  the base midpoint.
- Translucent paint is reported, because how GDI+ composites a cap over
  the line it overlaps is unknown.

## Dashed pens and caps

A dashed pen's line caps apply at figure ends and its dash cap at dash
ends, which Stroke cannot express apart; dashed pens therefore need flat
line caps.

## Compound lines

Compound lines (MS-EMFPLUS 2.2.2.9) are parallel bands across the pen
width. Which side fraction 0 lies on is not specified, so only symmetric
arrays are drawn, and how bands meet at joins is not either. Each band
edge is joined like the whole pen, so a band is the full-width stroke at
its outer edge minus the one at its inner edge, with the pen's join. With
round joins that is exactly the points at the band's distances from the
path; with miter joins Play checks that no corner exceeds the miter limit,
where GDI+ would bevel. Open figures need flat caps. Windows GDI+ draws
round and miter compound pens this way. With bevel joins it connects the
inner sides of the band edges with crossing segments that reach into the
gaps between bands; those pens are reported. LibreOffice draws compound
pens as one solid line.

## Path gradients

A path gradient (MS-EMFPLUS 2.2.2.29) changes color along each line from
the center to the boundary. For a boundary that is star-shaped from the
center this is exactly a fan of Gouraud triangles from the center, which Play
delivers to a `GradientBackend` with the filled shape as a final clip layer;
WrapModeClamp leaves the area outside the boundary unpainted. Surrounding
colors belong to the boundary vertices, or one color to all. Point
boundaries are closed cardinal splines of unstated tension, and focus
scales give no scaling origin; those are reported. Blend positions are
described only as running from a "midpoint" to an "endpoint" (2.2.2.4,
2.2.2.5); Microsoft's GDI+ reference (PathGradientBrush::SetBlend and
SetInterpolationColors) makes 0 the boundary and 1 the center point, a
factor the fraction of the way from the boundary color to the center
color, and preset colors replace both. The position is an affine
coordinate in each fan triangle, so Play splits the triangles into exact
Gouraud bands at the positions. Blend factors with several surrounding
colors make the color bilinear, which no such mesh expresses; they are
reported.

## Image source rectangles

Image source rectangles are EmfPlusRectF values (MS-EMFPLUS 2.2.2.39) and
map exactly onto the destination. A fractional source is drawn from the
pixels enclosing it, clipped to the image of the exact rectangle (see
`ImageDraw`), so partial source pixels are cut at the destination's edge.

## Metafile images

A metafile image (MS-EMFPLUS 2.2.2.27) is played from its own default
state into image pixels, which MS-EMFPLUS leaves undefined. As GDI+
records and draws them: the logical units of placeable WMF bounds, or for
an EMF the frame in reference-device pixels plus one, the frame filling an
image one pixel wider and taller. The source rectangle maps onto the
destination like a bitmap's, and only its part of the picture is drawn, as
a final clip layer. Without image attributes, GDI+ draws nothing where a
source rectangle extends past a bitmap or metafile image, as with
transparent clamping. The embedded picture shares the Play call's pixel and
region budgets; its records, framed and played, count against
`MaxEmbeddedRecords` every time it is drawn, and its nesting against
`MaxMetafileDepth`. Its omissions are reported against the drawing record,
prefixed "embedded metafile: ". An embedded file that cannot be read is
reported and skipped, as GDI+ would not draw it; exceeded limits still stop
Play. LibreOffice agrees (`plus-metafile-image.emf`), apart from ignoring
partial source rectangles (`lo-plus-metafile-source.emf`).

## Hatch styles

MS-EMFPLUS 2.1.1.13 describes hatch styles 6–52 only in words. EMF+
hatches are drawn as `PaintPattern` with GDI+'s 8x8 patterns, extracted
from Windows GDI+ ([evidence](../testing/windows-oracle.md#hatch-styles)): device pixels from the rendering origin,
each blending the foreground over the background by its coverage. GDI+
anti-aliases the diagonals of styles 2, 3 and 5, so even those six
styles differ from GDI's hatches, which stay `PaintHatch`.

## SourceCopy compositing

SourceCopy compositing replaces pixels. That equals source-over for opaque
paint, which is drawn; translucent paint is reported. Clear requires an
opaque color for the same reason.

## Font sizes

EMF+ font sizes in World units are world units. Physical sizes are converted
to device pixels with the header's vertical DPI and then to page units, so
the world transform still scales and rotates text. Fonts are requested with
a negative LOGFONT height (the em) and DEFAULT_CHARSET.

## Driver-string matrices

A driver-string matrix is "applied to each value in the text array"
(MS-EMFPLUS 2.3.4.6), which does not settle whether it moves the positions,
the glyphs or both. Translations give the same result under every reading
and are applied in world space; other matrices are reported. Decoration
extents run from the first origin to the last origin plus that glyph's
measured advance.

## DrawString

DrawString is reported. Its layout depends on rules MS-EMFPLUS leaves open:
the unit of the default 1/6 margins, how the 1.03 default tracking applies,
line breaking, trimming and line spacing.

## Bitmap budgets

Bitmap images decode once per object definition and are charged to the
playback pixel budget then: the declared size first, then any excess of the
decoded size. Region trees are converted to depth 256 at most. Clip steps
share the PlayOptions bound.
