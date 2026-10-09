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
| `1`, `14` | Header/EOF control records; description, recognized extensions, micrometer size and bounded pixel-format bytes |
| `2`–`8`, `56`, `85`–`92` | Poly/Bezier/PolyDraw, 32-bit and 16-bit coordinates; polygon sum and Bezier grouping checks |
| `9`–`13`, `26`, `27`, `54` | Coordinate/mapping/current-position records |
| `15` | SetPixelV |
| `16`–`22`, `24`, `25`, `37`, `40`, `48`, `57`, `67`, `98`, `115` | Scalar modes/colors/alignment/object IDs/clip mode/ICM/layout |
| `28`, `33`, `52`, `59`–`61`, `65`, `66`, `68` | Meta region, SaveDC, palette realization, path-bracket control |
| `29`, `30`, `42`, `43`, `62`–`64` | Clip rectangles, ellipse/rectangle, path bounds |
| `31`, `32`, `34`–`36`, `58` | Scaling, RestoreDC, affine transforms, miter limit |
| `38`, `39`, `82`, `93`–`95` | Pens, brushes, Unicode fonts including PANOSE/extended names/design axes, DIB/mono pattern brushes, extended pens/dash arrays |
| `41`, `44`–`47`, `55` | AngleArc, round rectangle, arc/chord/pie/ArcTo |
| `49`–`51` | Palette creation, updates, resizing |
| `70`, `75` | Comment data and ExtSelectClipRgn region rectangles |
| `53`, `71`–`74` | Flood fill and inline-region fill/frame/invert/paint, including explicit brush reference checks |
| `76`–`81`, `114`, `116` | BitBlt, StretchBlt, MaskBlt, PlgBlt, SetDIBitsToDevice, StretchDIBits, AlphaBlend, TransparentBlt; bounded source/mask bitmap spans |
| `83`, `84`, `96`, `97`, `108`, `120` | ExtTextOutA/W, PolyTextOutA/W, SmallTextOut, ETO_PDY advances, ETO_NO_RECT, text justification |
| `118` | Gradient vertex/color views and bounded rectangle/triangle indexes; unused vertex alpha is explicitly identified |
| `23`, `99`–`101`, `111`–`113`, `121`, `122` | Color adjustment; logical color-space creation/selection/deletion; palette correction; ICM/profile and target-matching metadata |

Header extensions are recognized only before the earliest variable buffer, so a
long description is not misread as fixed fields. Pixel-format descriptor bytes
remain uninterpreted. Font PANOSE fields and extended name/style/script/design
vectors are decoded in their size-selected layouts; unknown short tails remain
available as raw extension bytes. Text bytes are not
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

- Records: `4001`–`4004`, `4008`–`4036`, `4038`–`403a` (hex).
  These cover header/EOF/comment/GetDC, object fragments, clear, rectangles,
  polygons/lines/Beziers, ellipses/arcs/pies, path/region drawing, image placement,
  strings/driver strings, cardinal splines, rendering properties, save/restore, containers,
  world/page transforms, and clip operations.
- Coordinates: finite float32, signed int16, and signed variable-width 7/15-bit
  relative points. Relative arrays are cumulatively decoded under an allocation
  budget. Path point-type RLE is expanded with bounded run counts.
- Objects: solid/hatch/linear-gradient/path-gradient/texture brushes; pens including optional
  transform/cap/join/dash/compound fields and inline custom caps; paths; binary region trees; bitmap and
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

Custom line caps support both default path-based and adjustable-arrow forms,
as standalone type-9 objects and in pens. Fill/stroke paths, styles, insets,
scales, finite dimensions and mandatory zero hotspots are checked. Nested cap,
brush, image and path decoders share an allocation allowance and nesting budget;
errors retain their enclosing-object byte offsets.

`EmfPlusSerializableObject` decodes all eleven standard image-effect blocks:
blur, brightness/contrast, color balance, color curve, color lookup table,
color matrix, hue/saturation/lightness, levels, red-eye correction, sharpening
and tint. GUIDs are matched in full, declared buffer sizes and array spans are
checked, floats must be finite, and parameters must lie in their defined domains.
Lookup tables and red-eye rectangles are zero-copy views. Color-matrix wire
rows and their affine constraints are explicit. `ApplyImageEffect` executes lookup
tables and affine color matrices in straight RGBA8 with checked output budgets,
clamping and rounding. Other pixel-effect algorithms remain unsupported.

Terminal-server graphics snapshots include validated modes, signed origins,
transforms and optional palettes. Terminal clip rectangles decode the uniform
4-byte/8-byte forms described by the Size/DataSize tables, with signed differences
and bottom-relative-to-current-top handling. Inconsistent per-coordinate markers
are rejected; mixed-width interpretations are not guessed from ambiguous prose.
StrokeFillPath remains unsupported: it is named in the RecordType enumeration but
has no record-layout section in the consulted MS-EMFPLUS revision.
Reserved MultiFormat records (`4005`–`4007`) are explicitly malformed for typed
decoding, rather than mistaken for an unimplemented valid drawing operation.
Path type flags, starting points, complete Bezier triples, figure closures and
RLE Bezier indicators are validated before exposing a path, including nested cap
paths. Dash/marker flags remain available to the renderer. Embedded metafiles are
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
Effect-enabled `DrawImagePoints` requires an earlier serialized effect and receives
the latest description in `Command.Effect`. The stream retains only that latest
description. `DrawImage` ignores the bit reserved at the corresponding position;
it neither requires nor applies an effect. Supported pixel effects are applied
explicitly through `ApplyImageEffect`; `Stream` does not mutate image pixels.
GDI commands carry `ColorState` snapshots for selected source spaces, ICM mode,
output profiles, proofing metadata and color adjustments. Save/restore and deleted
color-object lifetimes are handled without mutating previously emitted snapshots.

## GDI playback

`Play` interprets the WMF and EMF GDI streams; EMF+ files play only their Dual
GDI fallback on request. Coverage of each family:

| Family | Played | Reported as unsupported |
| --- | --- | --- |
| State | SaveDC/RestoreDC (relative and WMF absolute), map modes 1–8, window/viewport origin/extent/offset/scale, Set/ModifyWorldTransform (all four modes), background mode/color, poly-fill mode, ROP2, stretch mode, arc direction, miter limit, brush origin, current position | Right-to-left layout; ICM conversion with a non-sRGB source, output profile or proofing target |
| Objects | Pens (LogPen and ExtCreatePen styles, caps, joins, user dashes, hatched pen brushes), solid/null/hatch and DIB pattern brushes, stock objects including DC_PEN/DC_BRUSH defaults, WMF lowest-free-slot reuse, EMF handle reuse | Monochrome and Bitmap16 pattern brushes, DIB pattern pens, dithered hatch styles, palette-relative COLORREFs and DIB colors, selecting a WMF region |
| Geometry | Polygons/polylines/polypolygons/polypolylines (16/32-bit), Bézier and "To" forms, PolyDraw, LineTo/MoveTo, Rectangle, RoundRect, Ellipse, Arc/ArcTo/Chord/Pie, AngleArc, SetPixel | — |
| Paths | Begin/End/Abort, CloseFigure, FlattenPath, FillPath, StrokePath, StrokeAndFillPath, SelectClipPath | WidenPath; text inside a path bracket |
| Clipping | IntersectClipRect, ExcludeClipRect, OffsetClipRgn, SelectClipPath and ExtSelectClipRgn with all five modes, omitted-region reset, SetMetaRgn, save/restore | WMF SelectClipRegion |
| Bitmaps | StretchDIBits, SetDIBitsToDevice, BitBlt, StretchBlt, MaskBlt without a mask, PlgBlt without a mask, AlphaBlend (constant and per-pixel alpha), TransparentBlt; WMF DIBBitBlt, DIBStretchBlt, StretchDIB, SetDIBToDev, PatBlt; mirroring, partial and clamped sources, scale/translate source transforms, HALFTONE hint | Masks, Bitmap16 and device-to-device sources, partial scan-line buffers, rotated/sheared source transforms, ROP3 other than SRCCOPY, NOTSRCCOPY, PATCOPY, BLACKNESS, WHITENESS and DSTCOPY, halftone with a color adjustment |
| Text | TextOut, ExtTextOut (A/W, WMF), PolyTextOut, SmallTextOut through a `TextBackend`: fonts and stock fonts, text color, all alignment flags, TA_UPDATECP, explicit advances (and ETO_PDY without vertical displacement), character extra, justification, escapement and orientation, opaque and clip rectangles, OPAQUE background cells, glyph indexes, UTF-16, the ten single-byte Windows code pages and symbol fonts | Backends without `TextBackend`; DEFAULT_CHARSET without `DefaultCharSet`; double-byte, OEM and Mac character sets; right-to-left reading order; vertical (`@`) fonts; ETO_PDY vertical displacement; text in path brackets |
| Fills | — | Region painting, flood fill, gradient fill |

Interpretations where the specifications leave room or conflict:

- EMF records no graphics mode. A non-identity world transform implies
  GM_ADVANCED, because GDI records world transforms only in that mode; otherwise
  GM_COMPATIBLE applies. Under GM_COMPATIBLE, bounding-rectangle shapes are built
  in device space with the right and bottom edges excluded and the arc direction
  unreflected (MS-EMF 2.1.16); under GM_ADVANCED they are built in world space
  with edges included. WMF is always GM_COMPATIBLE.
- LogPen widths are logical units scaled by the logical x-axis (MS-WMF 3.1.4.2)
  and are round in device space under GM_COMPATIBLE; geometric pens follow the
  full world transform under GM_ADVANCED. MS-EMF 2.2.19's statement that
  non-geometric LogPen widths are device units conflicts with its own MUST that
  they be 1, and with the GDI call EMR_CREATEPEN records. A zero width is a
  hairline, as are cosmetic extended pens.
- Deleting a selected object activates the default stock object (MS-EMF
  3.1.1.1). The same rule applies to WMF, whose specification releases the
  object's resources on deletion. A restored selection whose slot was deleted
  or reused also falls back to the default instead of selecting another object.
- MM_ISOTROPIC adjusts the stored viewport extent whenever an extent changes,
  keeping the smaller physical scale (MS-WMF 2.1.1.16). Switching to a scalable
  mode retains the current extents; zero extents and invalid modes are ignored,
  as the GDI calls they record would fail. Extents are kept as floating-point
  values rather than GDI's integers.
- WMF device units are destination units: the placeable bounds are the initial
  window and the destination is the viewport (MS-WMF 3.1.3); records may then
  replace either. Fixed mapping modes use the resolution those bounds imply.
  EMF device units are reference-device pixels mapped onto the destination by
  the header frame (or the inclusive bounds when the frame is empty).
- ExtSelectClipRgn regions are logical units, as MS-EMF 2.3.2.2 specifies. The
  "no effect" rule for bitmap records whose Bounds miss the clip is not applied;
  the drawing itself is clipped.
- StretchDIBits sources use an upper-left origin and SetDIBitsToDevice a
  lower-left origin, per MS-EMF 2.3.1.7 and 2.3.1.5; the WMF StretchDIB and
  SetDIBToDev records follow the same rules. A lower-left source in a top-down
  DIB is reported unless the source is the whole bitmap. Source rectangles are
  clamped to the bitmap and only existing pixels are drawn.
- ROP3 operations are classified by their index byte. SRCCOPY ignores DIB
  alpha. PolyDraw, PolylineTo and the "To" records continue from the current
  position, starting a new figure after a closed one.
- Text placement: EMF text records state their own graphics mode, which
  selects GM_COMPATIBLE (device space, upright, only the height scaled by the
  y-axis and advances by the x-axis) or GM_ADVANCED (world space, full
  transform, orientation relative to the escapement); WMF is GM_COMPATIBLE.
  The string extent for alignment and TA_UPDATECP is the sum of the advances
  used. With TA_UPDATECP the position moves to the string's end in its drawing
  direction: forward for TA_LEFT, back for TA_RIGHT, unchanged for TA_CENTER.
  Character extra and justification apply only without explicit advances and
  are rounded to device pixels under GM_COMPATIBLE; break extra goes to U+0020,
  with any remainder one unit at a time to the first breaks. The WMF character
  extra is read as signed, as GDI's SetTextCharacterExtra takes it, although
  MS-WMF 2.3.5.25 describes the field as unsigned. Symbol-charset bytes map to
  U+F000+byte, the private-use range symbol fonts' Windows cmaps use. Charsets
  map to code pages as Windows' TranslateCharsetInfo documents.
- GDI's integer pixel rules beyond edge exclusion belong to the backend.
  `Stroke.PixelCenter` gives half a device pixel in destination units for
  backends that center lines on device pixels, as GDI does.

Hatches and predefined dash patterns are delivered as styles with their device
pattern grid; their pixel patterns are drawn by the backend. Path and clip
geometry is retained in destination coordinates, so a path survives transform
changes made after it is recorded. Each clip step is an immutable node shared by
saved states; the chain length is bounded across the clip and metaregions.

## Pixel decoding

- DIB headers: CORE, INFO, V4, V5. V4/V5 retain calibrated RGB or bounded
  linked/embedded profiles; profile spans cannot overlap palettes or packed pixels.
  Managed images require explicit conversion instead of silently assuming sRGB.
- DIB pixels: 1/4/8-bit indexed, 16-bit RGB555, 24-bit BGR, 32-bit BGRX,
  contiguous non-overlapping bitfields, RGB/logical palettes, RLE4/RLE8, PNG/JPEG.
- DIB orientation and DWORD row padding are respected; RLE runs/deltas and
  palette indexes are bounded. Missing RLE terminators are rejected.
- `AlphaImage` handles 32-bit premultiplied BGRA separately from ordinary RGB32.
- EMF+ bitmap output: PNG/JPEG/GIF/TIFF and all 14 defined raw formats: 1/4/8-bit indexed,
  16-bit grayscale/RGB555/RGB565/ARGB1555, RGB24, RGB32/ARGB32/PARGB32, RGB48,
  ARGB64/PARGB64. Indexed palettes are separate from pixel storage; palette
  bounds, flags and indexes are checked. ARGB palette alpha is preserved.
  Gray16/NRGBA64/RGBA64 output preserves extended channel precision and byte
  order. Premultiplication and 32-bit native allocation bounds are checked.
  PixelFormatUndefined and CMYK DIBs remain unsupported.
- GIF87a/89a return the first frame on its logical canvas with frame offsets,
  local/global palettes and transparency preserved. Uncovered opaque canvas uses
  the global background where available; transparent first frames use a clear
  canvas. Logical/frame bounds and palette spans are checked before decoding.
  Later animation frames are neither loaded nor validated. Plain-text rendering
  extensions and unsupported control/rendering extensions are explicit errors.
- Encoded-image dimensions are read and checked before invoking full decoders.
  Pixel decoding is distinct from destination scaling, clipping, ROP3, blending,
  transparency-color treatment and other drawing operations.

### TIFF profile

`ParseTIFF` reads the first classic II/MM IFD, with a 4096-entry metadata cap and
16-sample cap. It does not follow later pages, SubIFDs or private pointer tags.
Supported stripped layouts are unsigned 1/4/8/16-bit gray, palette and RGB, with
contiguous/separate planes, associated/unassociated alpha, orientations 1–8,
horizontal prediction, uncompressed/PackBits/LZW/Deflate data and exact expanded
sizes. LZW uses TIFF early-width changes and fixed dictionaries. Packed runs cannot
cross rows; decompression cannot exceed the declared/budgeted output. Decoded
storage defaults to 128 MiB and has explicit native-index checks.

Embedded ICC profiles are exposed and require explicit conversion. TIFF color
maps retain 16-bit precision. BigTIFF, tiled images, CCITT/JPEG compression, mixed
sample widths, floating/signed samples, YCbCr/CMYK/Lab TIFF color layouts remain
unsupported. This is a bounded TIFF subset, not a claim of complete baseline TIFF
support (which would also require CCITT Modified Huffman).

## Color conversion

golittlecms is the sole approved external Go dependency. The adapter supports
RGB ICC-to-sRGB and calibrated RGB-to-sRGB transforms, explicit Windows/ICC intent
mapping, alpha preservation and black-point compensation. Resolvers are explicit
callbacks; no metafile-provided paths are opened automatically. ICC tag count,
byte ranges and aggregate referenced bytes are bounded before backend parsing.
Source and destination image buffers and backend transform storage have distinct
budgets. `NewPixelColorTransform` additionally converts explicit RGB/RGBA/gray/CMYK
8-bit buffers to sRGB RGBA8, checking that the profile and channel model agree.
`NewProofingColorTransform` performs explicit RGB soft proofing through a supplied
target profile. Generated gray and CMYK profiles exercise the actual backend;
they are test devices, not printer characterizations. WCS profile formats and
halftone ColorAdjustment pixel rendering remain unsupported; those adjustments
are exposed as renderer state rather than approximated.

## Evidence

Generated tests cover signed coordinates, variable offsets/counts, text padding,
object reuse, path/save-state errors, stream selection, continuation padding,
region/gradient bounds, bitmap orientation, masks, palettes, RLE, alpha and image
budgets. Seven fuzz targets cover framing, typed records/objects, DIBs, streams,
playback, TIFF and ICC color transform integration. Generated color tests include analytic linear-RGB
to-sRGB expectations, alpha preservation, and concurrent backend calls.
The pinned corpus and POI comparisons are described in ORACLES.md. Passing them
does not establish full specification conformance or render equivalence. Linux
render comparisons cover generated raster transfers, TIFFs and GDI playback
scenes for paths, transforms, mapping modes, objects, clipping and bitmaps, but
not text. Known LibreOffice divergences are pinned separately in ORACLES.md.
Playback unit tests and offline scene probes check spec-derived geometry and
pixels in `make check`; the pinned corpus must play with every omission reported.
