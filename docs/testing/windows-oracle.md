# Windows GDI and GDI+ oracle

Windows is the primary playback oracle. Its results settled most of the
[GDI](../interpretations/gdi.md) and [EMF+](../interpretations/emfplus.md)
interpretations, but the run is manual and not part of CI. Reach for this
after changing playback behavior that Windows has measured, or to check the
evidence behind a ruling.

## Running it

1. On Linux or Windows, write the generated inputs:
   `GOWEMF_WINDOWS_INPUTS=<dir>/in go test -run TestWriteWindowsInputs .`
   and, from `rendercheck/`,
   `GOWEMF_WINDOWS_INPUTS=<dir>/text go test -run TestWriteWindowsInputs .`.
   Each directory gets the scenes and a `manifest.json` of their sizes.
2. On Windows (PowerShell 5.1), render them:
   `powershell -File tools\windows\oracle.ps1 -In <dir>\in,<dir>\text -Out <dir>\out`.
   The output holds `gdiplus/`, `gdi/` and `gdiwmf/` PNGs, GDI+-recorded
   `recorded/rec-*.emf` scenes, `hatches.txt`, `mbtowc-<codepage>.txt`,
   `environment.txt` and any `errors.txt`.
3. Copy the output back under `.external/` (ignored) and compare:
   `GOWEMF_WINDOWS_OUT=<out> go test -run 'TestCompareWindows' -v .` and,
   from `rendercheck/`, the same with `TestCompareWindowsText`.

`TestCompareWindowsDecoding` fails on any decoding difference. The image
comparisons only log `unmatched/total` per scene and backend and write
montages under `<out>/compare/`; they never fail, so read the log against
the counts below.

## Environment and method

Windows is the primary playback oracle. One series of runs on a GitHub
Windows Server 2025 runner (build 26100, gdiplus.dll 10.0.26100.33438,
gdi32.dll 10.0.26100.32995, 96 DPI) settled the interpretations below; the
job was then removed and is not part of CI. `tools/windows/oracle.ps1`
reproduces it by hand. It renders every generated scene with GDI and GDI+
(`Graphics.DrawImage` of a `Metafile`). GDI draws into a 32-bit DIB section
in a memory DC, with no GDI+ involved: EMF with `PlayEnhMetaFile`, and WMF
both natively with `PlayMetaFile` (outputs under `gdiwmf`) and converted
with `SetWinMetaFileBits`. The script also has GDI+ itself record metafiles
of the features in question, extracts the hatch patterns, and decodes byte
sequences with `MultiByteToWideChar`. A second series isolated GDI
behaviors with throwaway generated scenes, one question per scene; their
results are recorded under [corrected after the comparison](#corrected-after-the-comparison).
`TestCompareWindowsOutputs`, `TestCompareWindowsDecoding` and
`rendercheck`'s `TestCompareWindowsText` compare the downloaded outputs
(`GOWEMF_WINDOWS_OUT`) with playback under the same neighborhood metric as
the LibreOffice scenes. The reference backend anti-aliases where GDI and
GDI+ fill aliased, so edge pixels differ even where geometry agrees; the
findings rest on scenes that match exactly or on measured edges.

## Confirmed interpretations

Unmatched pixels against GDI+ (of 6,144 unless stated):

| Interpretation | Evidence |
| --- | --- |
| Fractional image source rectangles (exact mapping, clipped) ([ruling](../interpretations/emfplus.md#image-source-rectangles)) | `plus-image-fraction.emf`: 0 |
| Path gradient blend positions run from the boundary (0) to the center (1); factors move from the boundary color toward the center color; preset colors replace both ([ruling](../interpretations/emfplus.md#path-gradients)) | `win-pathgrad-factors`, `-presets`, `-plain`: 0 each; a GDI+-recorded `Blend` and `ColorBlend` draw the same |
| Compound pens with round and miter joins as differences of strokes ([ruling](../interpretations/emfplus.md#compound-lines)) | `lo-plus-compound-joins.emf` (round and miter): 0; `lo-plus-compound.emf`: 0 |
| Custom line caps under `CustomLineCaps`: an adjustable arrow, an asymmetric fill-path cap and a stroke-path start cap, recorded by GDI+ ([ruling](../interpretations/emfplus.md#line-caps-and-custom-caps)) | `rec-caps.emf`: 0 |
| Bidirectional text, right-to-left reading order and right-to-left glyphs placed at the right of their advances ([ruling](../interpretations/gdi.md#bidirectional-text)) | `text-bidi.emf`: same order and positions under GDI (Arial substituted for Liberation Sans) |
| Double-byte decoding, including invalid sequences, and per-byte advances ([ruling](../interpretations/gdi.md#double-byte-text)) | `MultiByteToWideChar`: 0 differences over every byte, byte pair and lead/trail/`A` triple of code pages 932, 936, 949 and 950; `text-dbcs.wmf`: same positions |
| Hatch patterns anchored at the rendering origin ([ruling](../interpretations/emfplus.md#hatch-styles)) | shifting the origin to (3,5) shifts every cell by (3,5); `win-hatches.emf`: 11 |
| Top-level EMF frame mapping without an extra pixel ([ruling](../interpretations/gdi.md#device-units-and-the-destination)) | `win-frame-edge.emf`: polygon edges at device x 48, 60, 90 and y 32 land on those destination columns and rows under both |

## Corrected after the comparison

### szlMicrometers

Ruling: [szlMicrometers](../interpretations/gdi.md#device-units-and-the-destination). Tracked in #15.

An edge at device x 900 on a 270 mm device whose
szlMicrometers refines szlMillimeters by 0.3% lands at 900 under GDI and
GDI+ (szlMicrometers would put it near 903). Play now always uses
szlMillimeters; `win-micrometers-refine.emf` (1024x768): 0 of 786,432.

### Metafile images

Ruling: [Metafile images](../interpretations/emfplus.md#metafile-images). Tracked in #19.

GDI+ records the whole of a 40 x 20 pixel EMF frame
as a 41 x 21 source, and of a placeable WMF as its logical bounds (600 x
300 at 1440 units per inch). Drawn content shows the EMF frame filling
the larger image. Embedding through GDI+ the EMF and WMF of
`plus-metafile-image.emf`: 32 each, from 2,112 and 2,308. Without image
attributes GDI+ draws nothing where a source rectangle extends past a
bitmap or metafile image (`rec-image-outside.emf`), so Play no longer
reports that case.

### Shape construction

Ruling: [Shape construction](../interpretations/gdi.md#graphics-mode-and-shape-construction).

GDI's EMF playback builds Rectangle, RoundRect,
Ellipse, Arc, Chord, Pie and ArcTo as under GM_ADVANCED even with an
identity world transform: in logical space with the right and bottom edges
included, so a page mapping that reflects one axis reverses the displayed
arc direction and reflecting both does not. MS-EMF 2.1.16's GM_COMPATIBLE
rules (device space, edges excluded, arc direction unreflected) are what
GDI applies to WMF, played natively with `PlayMetaFile` or converted with
`SetWinMetaFileBits`. GDI+ draws EMF arcs in logical space too but
excludes the edges. Play follows GDI. Measured pixel for pixel, null-pen
EMF rectangles under identity, scaled, reflected and world-transformed
mappings fill the same 40 x 30 pixels as GDI, 1-pixel outlines land on
the same columns (x 10 and 50 in EMF, 10 and 49 in WMF), and WINDING
paths of a polygon and a rectangle show that the rectangle reverses with
the mapping. Unmatched against GDI, the reflected arc, chord and pie
scenes drop from up to 2,152 to 0, as do `clip.emf`, `objects.emf`,
`lo-exclude-clip.emf`, `lo-raster-ops.emf` and a rounded rectangle; no
GDI scene got worse. Six-unit pens still differ by one anti-aliased
pixel on their outer top and left edges.

### StretchDIBits sources

Ruling: [StretchDIBits sources](../interpretations/gdi.md#bitmap-source-origins).

GDI measures the source y of EMR_STRETCHDIBITS and
WMF META_STRETCHDIB from the bottom row of the image, for bottom-up and
top-down DIBs, although MS-EMF 2.3.1.7 calls it the upper-left corner.
EMR_STRETCHBLT and WMF META_DIBSTRETCHBLT use the top row. Unmatched
against GDI, `bitmaps.emf` drops from 240 to 0 and the partial-source
experiments (both row orders, EMF, and WMF played natively and converted)
to 0. For SetDIBitsToDevice from a top-down DIB, GDI drew the top rows
for both source rows 0 and 4 of eight, which neither origin explains, so
Play still reports partial sources in top-down DIBs there.

### Saved selections

Ruling: [Saved selections](../interpretations/gdi.md#deleted-and-saved-selections).

A pen deleted while a saved state holds it comes back
with RestoreDC, in EMF whether or not its slot was reused meanwhile and in
WMF played natively; GDI and GDI+ draw the blue 8-unit line, not the
default pen (unmatched against GDI from 475 to 0, against GDI+ from 472 to
3). GDI restores a deleted logical palette the same way, and a
PALETTEINDEX brush then takes its entry; GDI+ uses the default palette. Deleting the current selection still
selects the default object (`lo-delete-selected.emf`/`.wmf`: 0).

### Object table size

Ruling: [Object table size](../interpretations/gdi.md#the-object-index-equal-to-handles).

GDI fails to create an EMF object at the index equal
to the header's Handles count, and selecting the index leaves the previous
object selected, though MS-EMF 3.1.1.1 sizes the table for Handles+1
entries. Play now ignores such creations: unmatched against GDI,
a brush at that index drops from 960 to 0, and `mapping.emf`, whose
fixture selected pen 4 of 4 handles, from 192 to 3. The scene fixtures
that used the index (`mapping.emf`, `lo-palette-index.emf`) now declare
one more handle; the old `lo-palette-index.emf` matched GDI once the
index was ignored.

### Monochrome brushes

Ruling: [Monochrome brushes](../interpretations/gdi.md#monochrome-pattern-brushes).

GDI and GDI+ create no EMR_CREATEMONOBRUSH brush from
a top-down DIB, which is why they drew nothing for the earlier
`lo-mono-brush.emf` fixture; selecting its handle keeps the previous
brush. From a bottom-up DIB, DIB_PAL_INDICES bits are used as they are and
DIB_PAL_COLORS paints the text color alone, with or without a selected
palette. DIB_RGB_COLORS consults the color table: of 20 tables tried, the
bits are kept exactly when the second color's R+G+B exceeds the first's
(black/white, gray/white, green/magenta, blue 9/red 10 versus white/black,
equal colors, red/blue, blue 10/red 9), which rules out luminance
weights. All 32 monochrome scenes, including the bottom-up
`lo-mono-brush.emf`, now match GDI and GDI+ exactly.

### Pen widths

Ruling: [Pen widths](../interpretations/gdi.md#pen-widths).

GDI and GDI+ transform geometric pens, from EMR_CREATEPEN,
EMR_EXTCREATEPEN and WMF META_CREATEPENINDIRECT alike, with the page
mapping: under an anisotropic mapping a 10-unit pen draws a horizontal
line as thick as the y scale makes it and a vertical one as the x scale
does, in EMF and natively played WMF. Play no longer draws such pens
round with the x-scaled width: unmatched against GDI, the anisotropic pen
experiments drop from up to 259 to 0 (native WMF from 248 to 0) and
`mapping.emf` from 42 to 3; isotropic ones matched before and after.

### Double-byte decoding

Ruling: [Double-byte decoding](../interpretations/gdi.md#double-byte-text). Tracked in #21.

A NUL after a lead byte is not taken as a trail
byte; Windows yields the default character and then U+0000.

### Compound pens with bevel joins

Ruling: [Compound pens with bevel joins](../interpretations/emfplus.md#compound-lines). Tracked in #20.

GDI+ connects the inner sides of
bevelled band edges with crossing segments that reach into the gaps
between bands. Those pens are reported again.

### Hatch styles

Ruling: [Hatch styles](../interpretations/emfplus.md#hatch-styles). Tracked in #22.

The 8x8 patterns of all 53 styles, with the coverage
of the anti-aliased diagonals of styles 2, 3 and 5, are in
`plus_hatch_tables.go`. Red on blue blends linearly in sRGB (233,0,21 for
coverage 234). GDI+'s 90-percent hatch covers 62 of 64 pixels.

### Test fixtures

Related: [fixture encoding](../interpretations/decoding.md#embedded-placeable-wmf-compatibility).

GDI+ abandons a metafile at an EMF+ record whose DataSize is
not 32-bit aligned, and draws nothing for a compressed bitmap whose header
fields are zero, though MS-EMFPLUS 2.2.2.2 calls them undefined. The
generated fixtures now write both as GDI+ does; the decoder still accepts
the lenient forms. GDI+ serializes a placeable WMF image with the 24-byte
padded header described under [embedded placeable-WMF compatibility](../interpretations/decoding.md#embedded-placeable-wmf-compatibility).
