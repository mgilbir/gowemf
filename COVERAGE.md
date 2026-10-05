# Coverage inventory

This inventory distinguishes **framing**, **typed layout decoding**, **stream
state checks**, and **pixel decoding**. It is not a list of fully rendered GDI
operations. `Walk` preserves every record body; `Decode` fails explicitly for
unknown types/encodings. `Stream` requires successful typed decoding for its
selected playback stream. Drawing modes and all semantic relationships are not
exhaustively validated by `Decode` alone.

## WMF typed bodies

MS-WMF record dispatch uses the low byte of RecordFunction, as required by the
record definitions. The full wire value remains in `Record.Type`.

| Function low bytes (hex) | Body / coverage |
| --- | --- |
| `00`, `05`, `1e`, `35` | EOF, ignored SetRelAbs, SaveDC, RealizePalette (`Empty`) |
| `02`–`04`, `06`, `07`, `2e`, `2d`, `2a`–`2c`, `34`, `f0`, `49` | Mode/layout/alignment values, object/palette/region selection/deletion and region painting indexes |
| `08`, `27` | Signed character spacing and RestoreDC |
| `01`, `09`, `31` | Background/text COLORREF and mapper flags |
| `0b`–`0f`, `11`, `13`, `14`, `20` | Window/viewport origins/extents/offsets, LineTo, MoveTo, clip offset |
| `10`, `12` | Scale window/viewport extents; zero denominators rejected |
| `15`, `16`, `18`, `1b`, `1c` | Clip rectangles, ellipse, rectangle, round rectangle |
| `17`, `1a`, `30`, `1f` | Arc, pie, chord, SetPixel |
| `24`, `25`, `38` | Polygon, polyline, polypolygon; checked counts and signed point arrays |
| `fa`, `fc`, `fb` | Pen, brush, basic font fields/ANSI face name |
| `21`, `32`, `0a` | TextOut, ExtTextOut, text justification; padding and optional advances |
| `33`, `40`, `41`, `43` | SetDIBToDev, DIBBitBlt, DIBStretchBlt, StretchDIB; source/no-source forms, scan metadata, ROP and packed DIB |
| `1d`, `42` | Pattern blit and packed DIB/Bitmap16 pattern-brush creation |
| `26` with escape `000f` | WMFC enhanced-metafile fragment envelopes: counts, lengths, remaining bytes, version and checksum value |
| `19`, `48`, `28`, `29` | Flood/extended flood fill, region fill and frame |
| `f7`, `36`, `37`, `39` | Palette creation, animation, entry updates and resizing |
| `ff` | Region objects, including bounded scan arrays and mirrored count checks |
| `f9`, `22`, `23` | Legacy pattern-brush and Bitmap16 BitBlt/StretchBlt layouts, checked row sizes and pixel spans |

Core WMF record families are decoded; escape subtypes other than enhanced-metafile
fragments remain unsupported. Pixel colors in device-dependent Bitmap16 data are
not guessed from the host platform. Palette entries retain their RGBA-independent
wire layout (red, green, blue, usage flags).
The significant high function bytes of BitBlt/StretchBlt and DIBBitBlt/DIBStretchBlt are checked rather
than discarded. SetDIBToDev's coordinates are unsigned WORDs, unlike other WMF
drawing coordinates. Partial scanline transfers remain transfer metadata; pixel
decoders expect a complete pixel buffer. Bitmap16 pattern bytes are retained but
are not yet pixel-decoded.

WMFC fragments are metadata in the WMF fallback stream. `ExtractEnhancedMetafile`
provides explicit, bounded assembly with count/remaining-size consistency,
complete checksum verification, and inner EMF framing validation. It returns
nil when no embedded EMF exists and rejects multiple streams or bad checksums.
Nothing is automatically drawn twice or substituted for the WMF fallback.

## EMF typed bodies

Record IDs in this table are decimal.

| IDs | Body / coverage |
| --- | --- |
| `1`, `14` | Header/EOF control records; common header metadata is in `Header` |
| `2`–`8`, `56`, `85`–`92` | Poly/Bezier/PolyDraw, 32-bit and 16-bit coordinates; polygon sum and Bezier grouping checks |
| `9`–`13`, `26`, `27`, `54` | Coordinate/mapping/current-position records |
| `15` | SetPixelV |
| `16`–`22`, `24`, `25`, `37`, `40`, `48`, `57`, `67`, `98`, `115` | Scalar modes/colors/alignment/object IDs/clip mode/ICM/layout |
| `28`, `33`, `52`, `59`–`61`, `65`, `66`, `68` | Meta region, SaveDC, palette realization, path-bracket control |
| `29`, `30`, `42`, `43`, `62`–`64` | Clip rectangles, ellipse/rectangle, path bounds |
| `31`, `32`, `34`–`36`, `58` | Scaling, RestoreDC, affine transforms, miter limit |
| `38`, `39`, `82`, `93`–`95` | Pens, brushes, basic Unicode fonts, DIB/mono pattern brushes, extended pens/dash arrays |
| `41`, `44`–`47`, `55` | AngleArc, round rectangle, arc/chord/pie/ArcTo |
| `49`–`51` | Palette creation, updates, resizing |
| `70`, `75` | Comment data and ExtSelectClipRgn region rectangles |
| `53`, `71`–`74` | Flood fill and inline-region fill/frame/invert/paint, including explicit brush reference checks |
| `76`–`81`, `114`, `116` | BitBlt, StretchBlt, MaskBlt, PlgBlt, SetDIBitsToDevice, StretchDIBits, AlphaBlend, TransparentBlt; bounded source/mask bitmap spans |
| `83`, `84`, `96`, `97`, `108`, `120` | ExtTextOutA/W, PolyTextOutA/W, SmallTextOut, ETO_PDY advances, ETO_NO_RECT, text justification |
| `118` | Gradient vertex/color views and bounded rectangle/triangle indexes; unused vertex alpha is explicitly identified |
| `23`, `99`–`101`, `111`–`113`, `121`, `122` | Color adjustment; logical color-space creation/selection/deletion; palette correction; ICM/profile and target-matching metadata |

EMF header extensions and extended font fields remain raw. Text bytes are not
converted using guessed code pages or mistaken for Unicode when glyph-index mode
is set. SmallTextOut's packed Unicode low bytes are distinguished from ANSI bytes.
PolyTextOut strings and advances cannot overlap any descriptor in the text array.
Bitmap spans need not have aligned offsets: record alignment does not impose
alignment on arbitrary byte buffers. Span extent and fixed-field-overlap checks
remain enforced. Unused gradient tail padding may be omitted under MS-EMF §2.3.
Escapes/OpenGL are not yet typed. Color-space object lifetimes and palette
correction ranges are checked; named profiles are preserved without file access.
Font shaping,
palette color realization and raster/compositing operations belong to playback.

## EMF+ typed records and objects

- Records: `4001`–`4004`, `4008`–`4036` (hex).
  These cover header/EOF/comment/GetDC, object fragments, clear, rectangles,
  polygons/lines/Beziers, ellipses/arcs/pies, path/region drawing, image placement,
  strings/driver strings, cardinal splines, rendering properties, save/restore, containers,
  world/page transforms, and clip operations.
- Coordinates: finite float32, signed int16, and signed variable-width 7/15-bit
  relative points. Relative arrays are cumulatively decoded under an allocation
  budget. Path point-type RLE is expanded with bounded run counts.
- Objects: solid/hatch/linear-gradient/path-gradient/texture brushes; pens including standard optional
  transform/cap/join/dash/compound fields; paths; binary region trees; bitmap and
  embedded-metafile image envelopes; fonts; string formats; image attributes.
- Linear gradients retain transforms, gamma flag, wrap mode, preset ARGB colors,
  and vertical/horizontal blend factors. Proportions and factor endpoints are
  checked. Color interpolation itself belongs to the renderer.
- Region trees are iteratively decoded into indexed nodes, with node counts,
  child links, path lengths, storage budget and maximum depth checked.
- Object continuation checks ID/type/total-size consistency and final alignment
  padding. Incomplete objects cannot enter the stream's object table.

Texture brushes retain optional transforms/images and gamma/wrap flags. Path
gradients retain center/surrounding colors, point/path boundaries, transforms,
blend patterns and focus scales, with nested path allocations sharing the
containing object's budget. Focus scales and incompatible blend flags are checked.

Custom pen caps, StrokeFillPath,
effects/serializable objects and terminal-server record families remain unsupported.
Reserved MultiFormat records (`4005`–`4007`) are explicitly malformed for typed
decoding, rather than mistaken for an unimplemented valid drawing operation.
Path point types are exposed; their complete
figure/Bezier topology is not yet semantically validated. Embedded metafiles are
not automatically recursed into. The consumer must impose a nesting/aggregate
resource budget before recursive playback.

Driver strings distinguish glyph indexes from Unicode code units and retain
explicit positions and optional transforms. For RealizedAdvance, the decoder
accepts the field-description's first-position-only layout and the size-formula's
full-array layout, records the stored position count, and exposes only the first
meaningful position. This bounded interpretation is documented in ORACLES.md.

## Stream validation

`Stream` checks supported object creation, selection/deletion, palette update
ranges, save/restore references and path-bracket construction/consumption. It
assigns WMF's lowest available object slot using a min-heap, checks EMF handle
bounds/stock-object indexes, and validates EMF+ reference types and configured
slot bounds. WMF palette selection is saved/restored with generation checks so
deleted/reused object slots cannot silently change the restored selection. Save/restore
checks do not substitute for a renderer actually saving its complete context.

Native EMF+ selection includes GDI commands only in GetDC intervals. An explicit
GDI fallback is available only for Dual files. Drawing properties, affine
composition order, clip combination, font metrics and object-style realization
are emitted for the consumer to apply. No completed scene or SVG is synthesized.

## Pixel decoding

- DIB headers: CORE, INFO, V4, V5. V4/V5 retain calibrated RGB or bounded
  linked/embedded profiles; profile spans cannot overlap palettes or packed pixels.
  Managed images require explicit conversion instead of silently assuming sRGB.
- DIB pixels: 1/4/8-bit indexed, 16-bit RGB555, 24-bit BGR, 32-bit BGRX,
  contiguous non-overlapping bitfields, RGB/logical palettes, RLE4/RLE8, PNG/JPEG.
- DIB orientation and DWORD row padding are respected; RLE runs/deltas and
  palette indexes are bounded. Missing RLE terminators are rejected.
- `AlphaImage` handles 32-bit premultiplied BGRA separately from ordinary RGB32.
- EMF+ bitmap output: PNG/JPEG, 24-bit RGB, 32-bit RGB, ARGB, PARGB. Other raw
  EMF+ pixel formats, GIF/TIFF and CMYK DIBs are unsupported.
- Encoded-image dimensions are read and checked before invoking full decoders.
  Pixel decoding is distinct from destination scaling, clipping, ROP3, blending,
  transparency-color treatment and other drawing operations.

## Color conversion

golittlecms is the sole approved external Go dependency. The adapter supports
RGB ICC-to-sRGB and calibrated RGB-to-sRGB transforms, explicit Windows/ICC intent
mapping, alpha preservation and black-point compensation. Resolvers are explicit
callbacks; no metafile-provided paths are opened automatically. ICC tag count,
byte ranges and aggregate referenced bytes are bounded before backend parsing.
Source and destination image buffers and backend transform storage have distinct
budgets. CMYK/gray source-profile adaptation, WCS-specific profile formats,
halftone ColorAdjustment rendering, and target soft-proofing remain unsupported
by this adapter even though their relevant EMF record layouts are decoded.

## Evidence

Generated tests cover signed coordinates, variable offsets/counts, text padding,
object reuse, path/save-state errors, stream selection, continuation padding,
region/gradient bounds, bitmap orientation, masks, palettes, RLE, alpha and image
budgets. Five fuzz targets cover framing, typed records/objects, DIBs, streams and
ICC color transform integration. Generated color tests include analytic linear-RGB
to-sRGB expectations, alpha preservation, and concurrent backend calls.
The pinned corpus and POI comparisons are described in ORACLES.md. Passing them
does not establish full specification conformance or render equivalence.
