# Apache POI record oracle

`make test-oracle` compares gowemf's decoded records with Apache POI's for the
pinned corpus and generated records. Reach for this to see what agreement is
claimed, how the oracle is bounded, and which POI readings are known to be
wrong.

`make test-oracle` downloads pinned POI 5.4.1 execution dependencies from Maven
Central, verifies exact sizes and SHA-256 digests, compiles the locally written
`tools/POIRecordDump.java`, and compares its output with gowemf. The Java adapter
uses public APIs only; no POI implementation source was ported. The library and
ordinary Go tests require no Java or third-party runtime.

The record-dump process has a 30-second timeout, a 256 MiB JVM heap, a 32 MiB
stdout limit and a 64 KiB stderr limit. The render oracles' POI and LibreOffice
processes share a 64 KiB combined output limit. JARs and compiled classes live in ignored
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
spec-derived generated tests. Cap rendering is compared with Windows GDI+ in the
[Windows oracle](windows-oracle.md).

Intentional normalization:

- POI excludes WMF EOF from its record list; gowemf includes it.
- Java serializes some DWORD handles as signed values; comparison preserves their
  exact 32-bit patterns.
- POI's PolylineTo diagnostic path adds an implicit initial move. That synthetic
  point is excluded when comparing the explicit wire point array.

## Known POI discrepancies

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
