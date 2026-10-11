# Regression log

The planted defects that each regression test and oracle was seen to catch.
AGENTS.md requires every new assertion to be proven against the defect it
watches; this log is that record. Reach for it to judge how strong a test is
before relying on it, or to add the evidence for a new one.

## Decoding and images

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

## GDI playback

Seventeen planted playback defects were each detected by both the offline probes
and the LibreOffice comparison before being restored: swapped world-transform
multiplication order, an off-by-one RestoreDC level, unsigned extents, an
uninverted fixed-mode y axis, ignored clip paths, difference treated as
intersection, clip not saved, a stale reused object slot, ignored stock objects,
ignored destination mirroring, a wrong partial-source origin, a wrong
StretchDIBits origin, ignored arc direction and
fill rule, removed compatible-mode edge exclusion, unscaled pen widths and
ignored constant alpha. Nineteen further planted defects were caught by the
playback unit tests (dash aliasing, ROP2 inversion, pattern reuse, clip offset
scaling, advanced-mode edges, inside-frame pens, PolyDraw closure, AngleArc
direction, dithered hatches, metaregion budget, transparent key order, source
origins, caller bounds, HALFTONE, silent text omission, isotropic adjustment,
PlgBlt corners, device-source transfers reported as malformed and ignored LogPen
cap/join bits).

## Fills

Twelve planted fill
defects (palette byte order, PALETTERGB, AnimatePalette scope, monochrome
colors, frame erosion and orientation, RECT_V, WMF scans, DIB_PAL_COLORS,
region work budget, multi-rectangle regions and WMF clip regions) were each
caught by the offline tests; those touching a scene also fail its probes.

## Raster operations

Thirteen planted defects were each caught by the offline
tests: truth-table bit order, pattern use, source alpha, SRCAND routed to an
image copy, an unmapped destination area, null-brush and TRANSPARENT-hatch
handling, and for merged sprites an inverted mask, a skipped black-image or
black-and-white check, ignored placement, a mask kept across another record,
and a mask kept across another transfer.

## EMF+ playback

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

## EMF+ caps, compound pens and path gradients

Fifteen planted
custom-cap defects were each caught by the offline tests, including both axis
directions, insets, scales, line-versus-fill preference, the arrow geometry
and the opt-in itself.

Twenty-six planted defects in caps, compound pens and path gradients were each
caught by the offline tests, and those that change a scene also fail the
LibreOffice comparison. Three more were caught by the reference rasterizer's
compound and mixed-cap test.

## Text

Fourteen planted text defects were
each caught by both the offline text tests and the LibreOffice scenes.

Five planted double-byte decoding defects were each caught by the offline tests.

Planted defects in bidi rules W4, W5, N0, L1, BD16, the paragraph level and
the sequence boundaries each failed the Unicode conformance tests, and each
also fails an offline test. Seven planted defects in the playback integration
(reading order, reordering, surrogate pairs, removed characters, paragraph
splitting, brackets, glyph placement) were each caught by the offline tests.

Fourteen planted driver-string defects were each caught
by the offline tests, and the five that change the scenes also fail the
LibreOffice comparison. They covered font units and page scale, styles,
glyph/code-unit selection, realized advances, matrix order and sign, vertical
and matrix reporting, decoration extents and the brush.

## TIFF

TIFF tests: a planted late-change LZW defect failed the width-transition test.
A transposed-matrix defect exposed an initially symmetric test fixture;
replacing it with a non-symmetric channel cycle made that regression fail as
intended before restoring the implementation.
