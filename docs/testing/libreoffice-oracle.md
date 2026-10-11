# LibreOffice render oracle

`make test-render` renders generated metafiles with LibreOffice and compares
every pixel with `Play` through the test-only reference rasterizer. Reach for
this to see which scenes agree, which LibreOffice behaviors are pinned as
divergences, and how much tolerance the comparison allows. Prerequisites are
in [testing](README.md#prerequisites). Why playback differs from LibreOffice is
in the linked [interpretations](../interpretations/gdi.md).

## Setup and metric

`make test-render` generates EMF/WMF bitmap transfers and TIFFs in code, then runs
LibreOffice with an isolated user profile, headless display, a 90-second timeout,
2 GiB address-space limit and 60 CPU-second limit per process. A generated flat
OpenDocument wrapper supplies an explicit zero-margin page and image frame; the
result is not cropped after rendering. Exact 96-dpi EMF device metrics avoid a
rounded-millimeter scaling error in the fixture. Liberation Sans availability is
checked and the tool/font versions are recorded in `.external/render/run-*/`.

Raster-transfer comparisons include every pixel, with a 3/255 per-channel
threshold and a maximum 1% mismatch fraction. Changed-row tests prove that the
metric rejects meaningful defects. Generated EMF and TIFF samples match
LibreOffice for uncompressed, PackBits, Deflate and early-change LZW data. The
LZW render sample crosses code-width changes and resets its dictionary. The EMF
and WMF bitmap fixtures must also come out of `Play` pixel-identical to the
direct decode.

LibreOffice **24.2.7.2** renders a WMF whose META_HEADER type is DISKMETAFILE (2)
blank; the same file with MEMORYMETAFILE (1) renders. MS-WMF 2.3.2.2 allows both.
The generated DISKMETAFILE bitmap fixture therefore keeps its pinned blank result
and exact POI agreement, and `bitmap-memory.wmf` checks LibreOffice agreement.
Generated WMF playback scenes use MEMORYMETAFILE.

## GDI playback scenes

`Play` output is rasterized by a test-only reference backend written separately
from the playback code. It flattens its own curves, strokes in pen space, takes
coverage on 16 sub-scanlines with exact horizontal spans, and composites in sRGB
onto white like the LibreOffice export. A stroke is the union of its segment,
join and cap pieces. Clip regions are combined as exact span sets, so regions
sharing a fractional edge leave no anti-aliased sliver. Coverage, stroke width,
curved-stroke solidity, exact clip combination and the comparison metric each
have generated tests. It centers lines on device pixels using
`Stroke.PixelCenter`.

There are eleven 96x64 agreement scenes. Four EMF scenes combine the requested
features: nested save/restore with set, left- and right-multiplied and reset
world transforms; anisotropic mappings with each and both axes reflected, an arc
in reflected space, MM_LOMETRIC, inherited fixed extents and a pen scaled by the
mapping; pen/brush selection, stock objects, deleted and reused slots, geometric
pens and restored selections; and clipped paths and bitmap transfers with
rectangle, path and region clipping, union, difference, offset and save/restore.
Two EMF scenes cover path brackets (alternate fill, Bézier construction,
closure, stroke-and-fill) and bitmap placement (full, mirrored,
partial-width and partial-height sources, BitBlt, NOTSRCCOPY, PATCOPY, constant
and per-pixel alpha). Two WMF scenes cover the placeable mapping, reflected
windows with SaveDC, lowest-free-slot reuse, geometric and hairline pens, and
clipped DIB transfers. `arc-reflected.emf` and `.wmf` draw a counterclockwise
chord under a reflected y axis, which EMF and WMF playback orient differently,
and `restore-deleted.emf` restores a pen deleted while a saved state held it.

Comparison is symmetric: each channel of every pixel must lie within 40/255 of
the range spanned by the other image's 3x3 neighborhood, with at most 0.5% of
pixels failing in either direction. This tolerates half-pixel edge placement,
LibreOffice's bilinear bitmap smoothing at 2x and differing anti-aliasing ramps,
but not displaced, missing or recolored content; a generated three-pixel
displacement test proves that. Observed worst cases are 0–17 of 6,144 pixels.
Every scene also carries hand-computed probe pixels derived from the
specifications, checked offline by `make check`.

## Pinned LibreOffice divergences

Each divergence has its own generated scene. Probes assert the result the
specification requires from `Play` and the pixels LibreOffice 24.2.7.2 was
observed to draw; if LibreOffice changes, the test fails for review.

| Scene | LibreOffice 24.2.7.2 behavior | Playback behavior |
| --- | --- | --- |
| `lo-isotropic.emf` | MM_ISOTROPIC viewport left anisotropic | Adjusted to square units (MS-WMF 2.1.1.16) |
| `lo-createpen-width.emf` | EMR_CREATEPEN width drawn as a hairline | Logical width (see [pen widths](../interpretations/gdi.md#pen-widths)) |
| `lo-world-pen.emf` | Geometric pen not transformed by an anisotropic world transform | Pen follows the world transform under GM_ADVANCED |
| `lo-delete-selected.emf`/`.wmf` | Deleted selected brush keeps painting | Default stock brush (MS-EMF 3.1.1.1) |
| `lo-shape-edges.emf` | EMF RoundRect, ArcTo and a clockwise Pie built without their right and bottom edges | Edges included, as Windows' EMF playback does; LibreOffice includes them for Rectangle and Ellipse |
| `lo-stretchdib-partial.emf`/`.wmf` | Partial StretchDIBits and StretchDIB sources measured from the top of the image | Measured from the bottom row in either row order, as Windows does |
| `lo-pen-aniso.emf` | Pen under an anisotropic page mapping drawn round with the x-scaled width | Pen transformed with the mapping, as Windows draws it |
| `lo-winding.emf` | WINDING fill drawn as ALTERNATE | Nonzero fill |
| `lo-exclude-clip.emf` | ExcludeClipRect ignored | Rectangle excluded |
| `lo-region-copy.emf` | ExtSelectClipRgn RGN_COPY ignored | Region replaces the clip |
| `lo-region-and.emf` | RGN_AND combined like RGN_XOR | Intersection |
| `lo-metaregion.emf` | SetMetaRgn does not constrain later clipping | Clip intersected with the metaregion (MS-EMF 2.3.2) |
| `lo-rotated-bitmap.emf` | World-rotated StretchDIBits drawn unrotated in an axis-aligned box | Affine placement |
| `lo-wmf-no-window.wmf` | WMF without window records scaled to its drawn content | Placeable bounds as the window (MS-WMF 3.1.3) |
| `lo-setdibits.emf` | EMR_SETDIBITSTODEVICE draws nothing | 1:1 device pixels, lower-left source origin |
| `lo-transparentblt.emf` | EMR_TRANSPARENTBLT draws nothing | Color-keyed transfer |

LibreOffice agrees with playback for RGN_OR and RGN_DIFF
region clipping, OffsetClipRgn, clip paths, AlphaBlend and PATCOPY. Hatch rendering is not compared: LibreOffice and backends draw
device patterns differently, and the test backend does not implement them.
Dashes are likewise left to backends and not compared.

Generated inputs, PNGs, wrapper documents and environment information remain
ignored under `.external/`; CI retains them as short-lived diagnostic artifacts.
No GPL/MPL implementation source is read or ported. Comparisons with Windows
are in the [Windows oracle](windows-oracle.md).

## Fill scenes

Region fills (FillRgn on an L shape and PaintRgn) match LibreOffice exactly
with a null pen. LibreOffice 24.2.7.2 diverges on every other fill:

| Scene | LibreOffice 24.2.7.2 behavior | Playback behavior |
| --- | --- | --- |
| `lo-gradient.emf` | EMR_GRADIENTFILL draws nothing | Rectangle and triangle Gouraud meshes |
| `lo-frame-region.emf` | EMR_FRAMERGN draws nothing | Exact 3 by 2 unit border |
| `lo-region-outline.emf` | FillRgn and PaintRgn also stroke the outline with the selected pen | Brush fill only |
| `lo-mono-brush.emf` | Monochrome brush fills with the background color only | Text color for clear bits, background for set bits |
| `lo-palette-index.emf` | PALETTEINDEX colors drawn black; DIB_PAL_COLORS bitmap not drawn | Selected logical palette |
| `lo-regions.wmf` | WMF FillRegion, PaintRegion and SelectClipRegion ignored | Region fills and clip |

Because LibreOffice draws palette colors black, it cannot settle the palette
byte order; POI's reading is pinned separately (see
[POI discrepancies](poi-oracle.md#known-poi-discrepancies)).

## Raster operation scenes

The scene backend implements `RasterBackend` on its own: it point-samples
pixel centers (GDI has no anti-aliasing), takes pixels that the area and clip
cover by at least half, and evaluates the truth table bit by bit, separately
from `RasterOperation.Apply`. A generated test checks all 256 operations
against `Apply`.

`raster-sprite.emf` (StretchDIBits) and `raster-sprite.wmf` (StretchDIB and
DIBStretchBlt) draw a sprite through an SRCAND mask and an SRCPAINT image, at
2x and across a background edge. Both match LibreOffice with no unmatched
pixels; drawing SRCAND and SRCPAINT as copies leaves 940.
`lo-raster-ops.emf` pins the divergences:

| Scene | LibreOffice 24.2.7.2 behavior | Playback behavior |
| --- | --- | --- |
| `lo-raster-ops.emf` | SRCINVERT drawn as SRCCOPY; DSTINVERT inverts with the selected brush, like PATINVERT; PATINVERT draws nothing | Source XOR destination, inverted destination, brush XOR destination |

Rendering both sprite scenes through a backend without `RasterBackend`,
where each pair becomes one masked image, gives pixels identical to the
bitwise operations.

## EMF+ playback scenes

Nine EMF+ Only agreement scenes at 96 DPI match LibreOffice under the same
tolerance:
- `plus-shapes.emf`: rectangles, an ellipse, a clockwise pie, an
  alternate-filled path, a polygon, a wide line and a translucent fill.
- `plus-transforms.emf`: a rotated world, a container mapping, Save/Restore and
  an inch page unit.
- `plus-clip.emf`: an excluded rectangle and a Complement region tree.
- `plus-clip-path.emf`: a path clip united with a rectangle, and xor.
- `plus-order.emf`: prepended and appended transform records, a Bézier and a
  closed cardinal spline.
- `plus-pens.emf`: wide pens, round caps, a clockwise arc and an open spline.
- `plus-image.emf`: a PNG-encoded bitmap placed in a rectangle and on a
  parallelogram.
- `plus-metafile-image.emf`: an embedded EMF scaled into a rectangle and a
  placeable WMF, drawn whole as metafile images. Without the embedded pictures'
  mapping, 2,559 pixels are unmatched.
- `plus-image-fraction.emf`: source rectangles with half-pixel edges, scaled
  3 times into a rectangle and onto points. Drawing the enclosing whole
  pixels without the exact clip leaves 216 unmatched pixels.

Geometry follows the default PixelOffsetMode (pixel centers at integer
coordinates), half a pixel from LibreOffice's placement; the scenes keep that
within the tolerance and away from the canvas border.

| Scene | LibreOffice 24.2.7.2 behavior | Playback behavior |
| --- | --- | --- |
| `lo-plus-page-change.emf` | A second SetPageTransform is ignored; drawing keeps the first page units | Each SetPageTransform replaces the page transform |
| `lo-plus-raw-bitmap.emf` | Uncompressed (BitmapDataTypePixel) bitmaps draw nothing | Raw 32bppARGB pixels placed like encoded ones |
| `lo-plus-texture.emf` | Texture brushes paint nothing | Image tiled through the brush transform |
| `lo-plus-gradients.emf` | Linear gradients drawn in about eleven discrete bands | Continuous interpolation |
| `lo-plus-nearest.emf` | NearestNeighbor interpolation ignored; images smoothed | Nearest-neighbor hint passed to the backend |
| `lo-plus-metafile-source.emf` | A metafile image's source rectangle is ignored; the whole picture is drawn | Only the source rectangle, mapped onto the destination |

LibreOffice agrees with playback for page units, containers, Save/Restore, all
combine modes exercised, region trees, path clips, the clockwise angle
convention, cardinal splines, pen caps and image placement. Its banded
gradients follow the same direction, preset stops and mirrored wrap.

Caps, compound pens and path gradients have their own scenes. `plus-caps.emf`
(square and round caps chosen separately at each end) agrees with LibreOffice.
LibreOffice 24.2.7.2 diverges on the rest:

| Scene | LibreOffice 24.2.7.2 behavior | Playback behavior |
| --- | --- | --- |
| `lo-plus-square-anchor.emf` | SquareAnchor drawn wider than the line | A square of the line width (MS-EMFPLUS 2.1.1.17) |
| `lo-plus-compound.emf` | Compound pens drawn as one solid full-width line | Parallel bands |
| `lo-plus-compound-joins.emf` | The same, at round and miter corners | Bands joined like the pen: round bands at their distances from the corner, as Windows GDI+ draws them |
| `lo-plus-path-gradient.emf` | An elliptical blend that also covers the filled area outside the boundary | A center-to-boundary fan inside the boundary only |

LibreOffice could not settle the custom-cap convention. A file with an
adjustable arrow cap fails to convert ("Unspecified Application Error"). Path
caps are drawn reversed relative to the line, with an asymmetric cap
re-centered. Custom caps therefore stay reported unless
`PlayOptions.CustomLineCaps` opts into the
[line-cap interpretation](../interpretations/emfplus.md#line-caps-and-custom-caps).
`lo-plus-custom-cap.emf` pins LibreOffice's reversed path cap against that
interpretation. Windows GDI+ draws the caps it records itself as the
interpretation does (`rec-caps.emf` in the [Windows oracle](windows-oracle.md)).

## Text scenes

Text needs a typesetter, so its oracle tests live in the separate `rendercheck`
module, which depends on forme (pinned in `rendercheck/go.mod`) and requires Go 1.26 without either
reaching gowemf's own module. Its backend shares `internal/raster` with the
scene tests and sets text from the system's Liberation Sans with GDI's
TrueType conventions: a negative LOGFONT height is the em, a positive one the
usWinAscent + usWinDescent cell; ascent and descent are usWin*; advances are
hmtx widths. It refuses font requests it cannot honor instead of substituting.

Ten agreement scenes (192x96) cover the alignment flags, natural and explicit
spacing, cell-height fonts, rotated escapements, OPAQUE background cells,
opaque and clip rectangles, TA_UPDATECP, upright text under MM_LOMETRIC and an
anisotropic GM_COMPATIBLE page, rotated GM_ADVANCED text and ANSI WMF text
through the code-page tables. Nine match LibreOffice within 0–8 of 18,432
pixels under the same neighborhood metric; the decoration scene asserts the
underline and strikeout extents by probe, because stroke position and
thickness are each renderer's font policy.

`text-dbcs.wmf` draws GB2312 text whose double-byte characters map into
Latin-1, with one advance per byte, and matches LibreOffice with no unmatched
pixels; giving a double-byte character only its lead byte's advance leaves
234. The double-byte tables are checked against every byte and lead/trail
pair of the pinned best-fit files under make test-external.

`text-bidi.emf` draws Hebrew with digits and Latin letters at explicit
spacing, once in right-to-left reading order and once left to right, and
matches LibreOffice with no unmatched pixels; `text-bidi-decoration.emf`
checks by probe that an underline spans the run's whole extent. Drawing in
logical order leaves 902 unmatched pixels, and placing right-to-left glyphs at
the left of their advances 454. The Unicode Bidirectional Algorithm itself
passes all 770,241 cases of BidiTest.txt and all 91,707 of
BidiCharacterTest.txt (Unicode 18.0.0) under make test-external.

LibreOffice 24.2.7.2 renders ETO_NO_RECT records wrongly: it appears to read
the absent rectangle anyway and misplaces or stacks the glyphs. The agreement
scenes therefore record a zero rectangle, as Windows writers do, and the flag
has its own pinned scene. Further pinned text divergences:

| Scene | LibreOffice 24.2.7.2 behavior | Playback behavior |
| --- | --- | --- |
| `lo-text-no-rect.emf` | ETO_NO_RECT misparsed; glyphs stacked | Advances read from the record's offDx |
| `lo-text-justification.emf` | SetTextJustification ignored | Break extra added to spaces |
| `lo-text-charextra.wmf` | META_SETTEXTCHAREXTRA ignored | Extra added to each character (MS-WMF 2.3.5.25) |
| `lo-text-right-dx.emf` | Right alignment ends at the last glyph's own advance | Ends at the sum of the explicit advances |
| `lo-text-updatecp-right.emf` | TA_RIGHT with TA_UPDATECP leaves the position at the right end | Position moves to the string's left end |
| `lo-text-world-stretch.emf` | Glyphs not stretched by an anisotropic GM_ADVANCED world transform | Glyphs follow the full transform (MS-EMF 2.1.16) |

EMF+ driver strings are compared in the same text harness:
- `plus-driver.emf`: code units at explicit origins.
- `plus-driver-world.emf`: a point-sized font under a rotated world transform,
  with a translation matrix.

Both agree with LibreOffice and carry hand-derived ink probes.
`lo-plus-driver-glyphs.emf` pins a divergence: LibreOffice 24.2.7.2 draws
nothing for glyph-index driver strings (without DriverStringOptionsCmapLookup),
while playback hands the indexes to the backend.

LibreOffice applies a non-translation driver-string matrix to the glyph shapes
and the first position only; playback reports such matrices instead of
adopting that reading.
