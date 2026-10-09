# Oracle evidence and compatibility decisions

## Apache POI

`make test-oracle` downloads pinned POI 5.4.1 execution dependencies from Maven
Central, verifies exact sizes and SHA-256 digests, compiles the locally written
`tools/POIRecordDump.java`, and compares its output with gowemf. The Java adapter
uses public APIs only; no POI implementation source was ported. The library and
ordinary Go tests require no Java or third-party runtime.

Every oracle process has a 30-second timeout, a 256 MiB JVM heap, a 32 MiB stdout
limit and a 64 KiB stderr limit. JARs and compiled classes live in ignored
`.external/oracle/`; their embedded licenses/NOTICE resources remain intact.
The pinned versions, sizes and digests are in `internal/corpus/oracle.go`.

The six fixtures are pinned to Apache POI commit
`732120980140d5ed64b482c470e0b625cdb1ab15`:

| Input | Outer records | EMF+ records |
| --- | ---: | ---: |
| `slideshow/santa.wmf` | 582 | 0 |
| `slideshow/wrench.emf` | 153 | 0 |
| `spreadsheet/SimpleEMF_windows.emf` | 31 | 9 |
| `spreadsheet/SimpleEMF_mac.emf` | 2852 | 0 |
| `document/vector_image.emf` | 279 | 0 |
| `slideshow/nested_wmf.emf` | 588 | 18 |

Current agreement checks compare record order/types and selected decoded fields:
line/polygon vertices, origins and extents, object handles, text reference points,
WMF region size/count metadata, palette selection, EMF+ DPI and generated path
point/type arrays. They do **not** compare all fields,
Bezier normalization, text shaping, complete object semantics, or rendered pixels.

Generated pen records additionally compare both adjustable-arrow custom caps
against POI's public record dump: dimensions, inset, fill state, styles, miter
limit and width scale. Default cap paths and extended raw pixel values have
spec-derived generated tests; there is no Windows render-equivalence claim.

Intentional normalization:

- POI excludes WMF EOF from its record list; gowemf includes it.
- Java serializes some DWORD handles as signed values; comparison preserves their
  exact 32-bit patterns.
- POI's PolylineTo diagnostic path adds an implicit initial move. That synthetic
  point is excluded when comparing the explicit wire point array.

### Known POI discrepancies

- POI 5.4.1 reads EMF and WMF palette entries as flags, blue, green, red,
  following MS-EMF 2.2.18's drawing. Playback uses GDI's PALETTEENTRY order,
  which MS-WMF 2.2.2.13 specifies. A generated palette pins POI's reading.

- In `wrench.emf`, POI interprets bytes belonging to a variable-length header
  description as header-extension fields. These values are not used as an oracle
  for extension support. gowemf currently exposes extensions without claiming to
  decode them.
- POI 5.4.1's DrawLines record exposes only flags, so it is not a coordinate oracle
  for that record.
- Its SerializableObject record also exposes only flags, not effect parameters.
  A generated record pins this limitation; no POI agreement is claimed for the
  eleven decoded effect parameter blocks. Their tests are specification-derived.
- A generated relative-path object demonstrates that POI treats Integer7 `0x40`
  as +64 rather than the -64 required by MS-EMFPLUS §2.2.2.21, and exposes RLE run
  bytes as point types rather than expanding §2.2.2.32. The test pins this observed
  disagreement separately from agreement totals. gowemf follows the signed/RLE
  definitions, with generated expected-value tests. Updating POI must trigger a
  review of this discrepancy, not silently change the parser.
- A spec-derived legacy `META_CREATEPATTERNBRUSH` followed by `META_MOVETO`
  makes POI 5.4.1 lose the following record boundary. A separate test pins this
  observed failure; legacy Bitmap16 layout assertions are specification-derived,
  not counted as independent POI field agreement.

## Embedded placeable-WMF compatibility

The EMF+ image at byte 928 of pinned `nested_wmf.emf` uses:

1. A placeable WMF header padded from 22 to 24 bytes.
2. A standard WMF stream following that header.
3. A MetafileDataSize that counts only the standard WMF stream.

This differs from the straightforward interpretation of MS-EMFPLUS §2.2.2.27.
gowemf accepts this specific representation only when the placeable signature,
standard WMF type/header-size, independently declared WMF word count, enclosing
object length and maximum three-byte final padding all agree. `AlignedPlaceable`
reports the interpretation explicitly. `MetafileBytes` removes exactly the two
alignment bytes; ordinary `Walk` remains strict about standard WMF framing.
There is a generated regression fixture; the third-party bytes are never tracked.

The normalized WMF also contains two enhanced-metafile escape fragments. Their
ByteCounts are 8,226 and 7,354: 34 bytes of metadata plus the **current fragment**
length, not 34 plus the entire embedded stream's length as the ByteCount prose in
MS-WMF §2.3.6.25 states. The typed envelope follows the bounded fragment layout.
The whole embedded-EMF checksum is not checked by a single-record decoder; the
sample's first checksum is `0x006f`, while the one's-complement XOR of the joined
EMF bytes is `0x4c6f`. The WMF fallback is independently framed and streamed;
no unverified embedded EMF is automatically selected or played.

## Driver-string layout interpretation

MS-EMFPLUS §2.3.4.6 gives an all-glyph position-array size formula, but says that
RealizedAdvance specifies only the first position. The decoder accepts either
unambiguously sized layout within the record, reports `StoredPositions`, and
retains only the first meaningful position when that option is set. Ignored
positions in a full array are not treated as active coordinates. Generated tests
cover both forms, unaligned glyph/float boundaries, optional matrices and ignored
non-finite positions. This interpretation has not yet been checked against Windows.

## Regression strength

Planted-and-restored defects have demonstrated failures for WMF coordinate order,
DIB row orientation, and aligned-placeable normalization. The WMF coordinate
mutation also fails the independent POI comparison. Additional regressions were
observed failing before fixes for final continuation padding and forbidden payload
bytes on fixed-size EMF+ records. Bounds and resource limits remain enabled.
Further observed regressions cover canonical WMF StretchDIB opcode dispatch,
reserved EMF+ drawing flags, ignored advanced-text scales, stale restored palette
handles, and explicit 8-byte-per-pixel native-index bounds for compressed DIBs.
An unaligned bitmap-buffer regression also failed before removing the unjustified
offset-alignment restriction; record alignment and all byte-range bounds remain
enforced. Multi-string text tests reject spans that overlap a later descriptor.
Custom-cap tests were observed failing before decoder support was added. Planted
and restored defects also demonstrated that tests reject reset nested-allocation
budgets and palette bytes incorrectly counted as bitmap pixels. Malformed path
topology was accepted before its regression and validation fix.
Image-effect tests also failed before serializable-object support and earlier-effect
validation were added. Planted lookup-channel swaps and dropped GIF frame offsets
caused their regression tests to fail, and were restored before verification.

## Render oracles

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

### GDI playback scenes

`Play` output is rasterized by a test-only reference backend written separately
from the playback code. It flattens its own curves, strokes in pen space, takes
coverage on 16 sub-scanlines with exact horizontal spans, and composites in sRGB
onto white like the LibreOffice export. A stroke is the union of its segment,
join and cap pieces. Clip regions are combined as exact span sets, so regions
sharing a fractional edge leave no anti-aliased sliver. Coverage, stroke width,
curved-stroke solidity, exact clip combination and the comparison metric each
have generated tests. It centers lines on device pixels using
`Stroke.PixelCenter`.

There are eight 96x64 agreement scenes. Four EMF scenes combine the requested
features: nested save/restore with set, left- and right-multiplied and reset
world transforms; anisotropic mappings with each and both axes reflected, an arc
in reflected space, MM_LOMETRIC, inherited fixed extents and a pen scaled by the
mapping; pen/brush selection, stock objects, deleted and reused slots, geometric
pens and restored selections; and clipped paths and bitmap transfers with
rectangle, path and region clipping, union, difference, offset and save/restore.
Two EMF scenes cover path brackets (alternate fill, Bézier/arc construction,
closure, stroke-and-fill, a clockwise pie) and bitmap placement (full, mirrored,
partial-width and partial-height sources, BitBlt, NOTSRCCOPY, PATCOPY, constant
and per-pixel alpha). Two WMF scenes cover the placeable mapping, reflected
windows with SaveDC, lowest-free-slot reuse, geometric and hairline pens, and
clipped DIB transfers.

Comparison is symmetric: each channel of every pixel must lie within 40/255 of
the range spanned by the other image's 3x3 neighborhood, with at most 0.5% of
pixels failing in either direction. This tolerates half-pixel edge placement,
LibreOffice's bilinear bitmap smoothing at 2x and differing anti-aliasing ramps,
but not displaced, missing or recolored content; a generated three-pixel
displacement test proves that. Observed worst cases are 0–17 of 6,144 pixels.
Every scene also carries hand-computed probe pixels derived from the
specifications, checked offline by `make check`.

Seventeen planted playback defects were each detected by both the offline probes
and the LibreOffice comparison before being restored: swapped world-transform
multiplication order, an off-by-one RestoreDC level, unsigned extents, an
uninverted fixed-mode y axis, ignored clip paths, difference treated as
intersection, clip not saved, a stale reused object slot, ignored stock objects,
ignored destination mirroring, a wrong partial-source origin, a lower-left
StretchDIBits origin, ignored arc direction and
fill rule, removed compatible-mode edge exclusion, unscaled pen widths and
ignored constant alpha. Nineteen further planted defects were caught by the
playback unit tests (dash aliasing, ROP2 inversion, pattern reuse, clip offset
scaling, advanced-mode edges, inside-frame pens, PolyDraw closure, AngleArc
direction, dithered hatches, metaregion budget, transparent key order, source
origins, caller bounds, HALFTONE, silent text omission, isotropic adjustment,
PlgBlt corners, device-source transfers reported as malformed and ignored LogPen
cap/join bits).

### Pinned LibreOffice divergences

Each divergence has its own generated scene. Probes assert the result the
specification requires from `Play` and the pixels LibreOffice 24.2.7.2 was
observed to draw; if LibreOffice changes, the test fails for review.

| Scene | LibreOffice 24.2.7.2 behavior | Playback behavior |
| --- | --- | --- |
| `lo-isotropic.emf` | MM_ISOTROPIC viewport left anisotropic | Adjusted to square units (MS-WMF 2.1.1.16) |
| `lo-compatible-arc.emf` | EMF arc direction applied in logical space under a one-axis reflection | Unreflected device-space direction (MS-EMF 2.1.16); LibreOffice's WMF import agrees with playback |
| `lo-createpen-width.emf` | EMR_CREATEPEN width drawn as a hairline | Logical width (see COVERAGE.md) |
| `lo-world-pen.emf` | Geometric pen not transformed by an anisotropic world transform | Pen follows the world transform under GM_ADVANCED |
| `lo-delete-selected.emf`/`.wmf` | Deleted selected brush keeps painting | Default stock brush (MS-EMF 3.1.1.1) |
| `lo-restore-reused.emf` | RestoreDC reselects a deleted pen by value | Default pen |
| `lo-winding.emf` | WINDING fill drawn as ALTERNATE | Nonzero fill |
| `lo-exclude-clip.emf` | ExcludeClipRect ignored | Rectangle excluded |
| `lo-region-copy.emf` | ExtSelectClipRgn RGN_COPY ignored | Region replaces the clip |
| `lo-region-and.emf` | RGN_AND combined like RGN_XOR | Intersection |
| `lo-metaregion.emf` | SetMetaRgn does not constrain later clipping | Clip intersected with the metaregion (MS-EMF 2.3.2) |
| `lo-rotated-bitmap.emf` | World-rotated StretchDIBits drawn unrotated in an axis-aligned box | Affine placement |
| `lo-wmf-no-window.wmf` | WMF without window records scaled to its drawn content | Placeable bounds as the window (MS-WMF 3.1.3) |
| `lo-setdibits.emf` | EMR_SETDIBITSTODEVICE draws nothing | 1:1 device pixels, lower-left source origin |
| `lo-transparentblt.emf` | EMR_TRANSPARENTBLT draws nothing | Color-keyed transfer |

LibreOffice agrees with playback for the WMF arc direction, RGN_OR and RGN_DIFF
region clipping, OffsetClipRgn, clip paths, ExtCreatePen widths under
anisotropic page mappings, StretchDIBits upper-left partial sources, AlphaBlend
and PATCOPY. Hatch rendering is not compared: LibreOffice and backends draw
device patterns differently, and the test backend does not implement them.
Dashes are likewise left to backends and not compared.

Generated inputs, PNGs, wrapper documents and environment information remain
ignored under `.external/`; CI retains them as short-lived diagnostic artifacts.
No GPL/MPL implementation source is read or ported. Windows GDI/GDI+ remains
unavailable and no Windows render-equivalence claim is made.

### Fill scenes

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
byte order; POI's reading is pinned separately above. Twelve planted fill
defects (palette byte order, PALETTERGB, AnimatePalette scope, monochrome
colors, frame erosion and orientation, RECT_V, WMF scans, DIB_PAL_COLORS,
region work budget, multi-rectangle regions and WMF clip regions) were each
caught by the offline tests; those touching a scene also fail its probes.
The pinned corpus now plays without reported omissions once a text backend is
present, including the monochrome DIB_PAL_INDICES brushes of `nested_wmf.emf`.

### EMF+ playback scenes

Eight EMF+ Only agreement scenes at 96 DPI match LibreOffice under the same
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
| `lo-plus-path-gradient.emf` | An elliptical blend that also covers the filled area outside the boundary | A center-to-boundary fan inside the boundary only |

LibreOffice could not settle the custom-cap convention. A file with an
adjustable arrow cap fails to convert ("Unspecified Application Error"). Path
caps are drawn reversed relative to the line, with an asymmetric cap
re-centered. Custom caps therefore stay reported unless
`PlayOptions.CustomLineCaps` opts into the interpretation in COVERAGE.md.
`lo-plus-custom-cap.emf` pins LibreOffice's reversed path cap against that
interpretation. Neither has been checked against Windows. Fifteen planted
custom-cap defects were each caught by the offline tests, including both axis
directions, insets, scales, line-versus-fill preference, the arrow geometry
and the opt-in itself.

Twenty-six planted defects in caps, compound pens and path gradients were each
caught by the offline tests, and those that change a scene also fail the
LibreOffice comparison. Three more were caught by the reference rasterizer's
compound and mixed-cap test.

Forty-five planted EMF+ defects were each caught by the offline tests before
being restored, and those touching a scene also fail the LibreOffice comparison:
- the pixel offset and its Half mode, DPI axes and page scale;
- transform order, rotation direction, container mapping and container clips;
- Restore, both combine-mode tables, infinite regions and world-space clip
  offsets;
- the path close flag and FillPath rule;
- the gradient origin, brush transform and blend factors, and the texture
  transform;
- world and pixel pen widths, the default miter limit, dash scaling and caps;
- arc direction and geometric angles, spline tension, closed-curve wrapping
  and winding;
- image shear, source offset, cache invalidation and per-object pixel budget
  for images and pen textures;
- SourceCopy reporting, skipped-container contents, both clip-step limits, the
  hatch rendering origin and ARGB channel order;
- closed DrawLines, multi-rectangle fills, Clear clipping and FillRegion.

Two latent defects in the reference backend surfaced with these scenes: wide
strokes of flattened curves were drawn faint, and clip differences left slivers
on shared fractional edges. Both now have generated tests that fail on the old
code, and every earlier scene was re-verified against LibreOffice afterwards.

### Text scenes

Text needs a typesetter, so its oracle tests live in the separate `rendercheck`
module, which depends on forme (v0.9.0) and requires Go 1.26 without either
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
thickness are each renderer's font policy. Fourteen planted text defects were
each caught by both the offline text tests and the LibreOffice scenes.

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
adopting that reading. Fourteen planted driver-string defects were each caught
by the offline tests, and the five that change the scenes also fail the
LibreOffice comparison. They covered font units and page scale, styles,
glyph/code-unit selection, realized advances, matrix order and sign, vertical
and matrix reporting, decoration extents and the brush.

## Decoder and playback extensions

TIFF parsing follows the TIFF 6.0 structure, strip, PackBits, LZW, predictor and
alpha descriptions. It intentionally does not traverse later IFDs or private
pointer trees. Tests cover both byte orders, planes, orientations, palette/channel
precision, exact decompression sizes and checked arithmetic. A planted late-change
LZW defect failed the width-transition test. A transposed-matrix defect exposed an
initially symmetric test fixture; replacing it with a non-symmetric channel cycle
made that regression fail as intended before restoring the implementation.

Terminal-server clip decoding implements the fixed 4/8-byte rectangle forms in
the MS-EMFPLUS size tables. Its per-coordinate prose can suggest mixed-width data;
that interpretation is not guessed without an independently verified input.
The current specification names StrokeFillPath in its enumeration but supplies
no record-layout section; it remains explicitly unsupported.

Gray and CMYK profile tests generate their ICC data through public golittlecms
APIs. The CMYK profile is a small analytical test device, not an external printer
profile or a claimed colorimetric characterization. Soft-proof tests use an sRGB
identity target and check alpha preservation. Halftone ColorAdjustment algorithms
and filters beyond lookup/matrix effects are not approximated as Windows output.
