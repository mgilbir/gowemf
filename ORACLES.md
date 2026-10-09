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

Comparisons include every pixel, with a 3/255 per-channel threshold and a maximum
1% mismatch fraction. Changed-row tests prove that the metric rejects meaningful
defects. These tests compare library-decoded raster output and one-to-one transfer
geometry, not a full vector/text playback backend. Generated EMF and TIFF samples
match LibreOffice for uncompressed, PackBits, Deflate and early-change LZW data.
The LZW render sample crosses code-width changes and resets its dictionary.

LibreOffice **24.2.7.2** imports the generated WMF DIB transfer as a blank graphic.
An execution-only POI raster adapter matches the library/spec-derived pixels
exactly. The known blank LibreOffice result is asserted separately, tied to that
version, and excluded from agreement claims. Unexpected differences on another
version fail the test instead of increasing tolerance or silently skipping WMF.

Generated inputs, PNGs, wrapper documents and environment information remain
ignored under `.external/`; CI retains them as short-lived diagnostic artifacts.
No GPL/MPL implementation source is read or ported. Windows GDI/GDI+ remains
unavailable and no Windows render-equivalence claim is made.

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
