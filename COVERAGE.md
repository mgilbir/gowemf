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
  padding. An object ends once TotalObjectSize bytes are read (MS-EMFPLUS
  2.3.5.1), even when its final record keeps the C bit, as writers do.
  Incomplete objects cannot enter the stream's object table.

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
paths. PathPointFlags bits other than R and C have no defined meaning
(MS-EMFPLUS 2.2.1.6); writers set 0x2000, so those bits are kept in
`PlusPath.Flags` but ignored. Dash/marker flags remain available to the renderer. Embedded metafiles are
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

`Play` interprets the WMF and EMF GDI streams, and the GDI records inside EMF+
GetDC intervals. EMF+ playback is described in the next section. Coverage of
each GDI family:

| Family | Played | Reported as unsupported |
| --- | --- | --- |
| State | SaveDC/RestoreDC (relative and WMF absolute), map modes 1–8, window/viewport origin/extent/offset/scale, Set/ModifyWorldTransform (all four modes), background mode/color, poly-fill mode, ROP2, stretch mode, arc direction, miter limit, brush origin, current position | Right-to-left layout; ICM conversion with a non-sRGB source, output profile or proofing target |
| Objects | Pens (LogPen and ExtCreatePen styles, caps, joins, user dashes, hatched pen brushes), solid/null/hatch, DIB and monochrome pattern brushes (EMR_CREATEMONOBRUSH and 1-bit Bitmap16 patterns), stock objects including DC_PEN/DC_BRUSH defaults, logical palettes (create, select, set, animate, resize) with PALETTEINDEX and PALETTERGB colors and DIB_PAL_COLORS bitmaps, WMF lowest-free-slot reuse, EMF handle reuse | Colored Bitmap16 patterns, DIB pattern pens, dithered hatch styles, DIB_PAL_INDICES bitmaps, palettes after EMR_COLORCORRECTPALETTE, PALETTEINDEX with the default palette |
| Geometry | Polygons/polylines/polypolygons/polypolylines (16/32-bit), Bézier and "To" forms, PolyDraw, LineTo/MoveTo, Rectangle, RoundRect, Ellipse, Arc/ArcTo/Chord/Pie, AngleArc, SetPixel | — |
| Paths | Begin/End/Abort, CloseFigure, FlattenPath, FillPath, StrokePath, StrokeAndFillPath, SelectClipPath | WidenPath; text inside a path bracket |
| Clipping | IntersectClipRect, ExcludeClipRect, OffsetClipRgn, SelectClipPath and ExtSelectClipRgn with all five modes, omitted-region reset, SetMetaRgn, WMF SelectClipRegion and region SelectObject, save/restore | — |
| Bitmaps | StretchDIBits, SetDIBitsToDevice, BitBlt, StretchBlt, MaskBlt without a mask, PlgBlt without a mask, AlphaBlend (constant and per-pixel alpha), TransparentBlt; WMF DIBBitBlt, DIBStretchBlt, StretchDIB, SetDIBToDev, PatBlt; mirroring, partial and clamped sources, scale/translate source transforms, HALFTONE hint | Masks, Bitmap16 and device-to-device sources, partial scan-line buffers, rotated/sheared source transforms, ROP3 other than SRCCOPY, NOTSRCCOPY, PATCOPY, BLACKNESS, WHITENESS and DSTCOPY, halftone with a color adjustment |
| Text | TextOut, ExtTextOut (A/W, WMF), PolyTextOut, SmallTextOut through a `TextBackend`: fonts and stock fonts, text color, all alignment flags, TA_UPDATECP, explicit advances (and ETO_PDY without vertical displacement), character extra, justification, escapement and orientation, opaque and clip rectangles, OPAQUE background cells, glyph indexes, UTF-16, the ten single-byte Windows code pages and symbol fonts | Backends without `TextBackend`; DEFAULT_CHARSET without `DefaultCharSet`; double-byte, OEM and Mac character sets; right-to-left reading order; vertical (`@`) fonts; ETO_PDY vertical displacement; text in path brackets |
| Fills | FillRgn, PaintRgn, FrameRgn; WMF FillRegion, PaintRegion, FrameRegion; EMR_GRADIENTFILL rectangle and triangle modes through a `GradientBackend` | InvertRgn and InvertRegion and flood fill, which read the destination; gradients for backends without `GradientBackend` |

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
- Palette entries are read as GDI PALETTEENTRY values (red, green, blue,
  flags), as MS-WMF 2.2.2.13 specifies. MS-EMF 2.2.18 draws LogPaletteEntry as
  reserved, blue, green, red, which is that structure read as a most-
  significant-first DWORD; Apache POI follows the drawing (see ORACLES.md).
  Palette updates modify the palette object in place, so every selection of it
  sees them. AnimatePalette changes only PC_RESERVED entries. COLORREF's
  reserved byte must be zero (MS-WMF 2.2.2.8); GDI's PALETTEINDEX (0x01) and
  PALETTERGB (0x02) forms are resolved through the selected logical palette and
  as plain RGB on a true-color device. Palette entry allocations share the
  playback pixel budget.
- Monochrome pattern brushes draw clear bits in the text color and set bits in
  the background color, as GDI's CreatePatternBrush documents; the DIB's own
  color table and Usage are ignored.
- Region data is in logical units, as for ExtSelectClipRgn; WMF scans are
  logical (MS-WMF 2.2.2.21). FrameRgn draws R minus R eroded by the brush box,
  computed exactly as R intersected with R's complement dilated by that box;
  the band sweep is charged to a 16,000,000-step budget per Play call.
- Rectangle gradients become two triangles whose shading is exactly linear
  along the gradient axis; triangle gradients interpolate in destination
  space. TriVertex alpha is ignored (MS-EMF 2.2.26). Colors keep 16 bits.
- GDI's integer pixel rules beyond edge exclusion belong to the backend.
  `Stroke.PixelCenter` gives half a device pixel in destination units for
  backends that center lines on device pixels, as GDI does.

Hatches and predefined dash patterns are delivered as styles with their device
pattern grid; their pixel patterns are drawn by the backend. Path and clip
geometry is retained in destination coordinates, so a path survives transform
changes made after it is recorded. Each clip step is an immutable node shared by
saved states; the chain length is bounded across the clip and metaregions.

## EMF+ playback

`Play` interprets EMF+ records by default; `StreamOptions.PreferGDI` selects
the GDI fallback of a Dual file instead. GDI records inside GetDC intervals are
played by the GDI player, with its own device context.

| Family | Played | Reported as unsupported |
| --- | --- | --- |
| State | Save/Restore, BeginContainer (World or Pixel units) and BeginContainerNoParams, EndContainer; Set/Reset/Multiply/Translate/Scale/RotateWorldTransform in prepend and append order; SetPageTransform in Pixel, Point, Inch, Document and Millimeter units; pixel offset, interpolation and compositing modes; rendering origin. Anti-aliasing, text hints and compositing quality are backend policy. | Display and World page units; containers in physical units (their contents are skipped as a unit); SetTSGraphics and SetTSClip |
| Clipping | SetClipRect, SetClipPath, SetClipRegion with all six combine modes; region trees with every node type; ResetClip, OffsetClip; container clips as metaregions | — |
| Brushes | Solid colors with alpha, hatch styles 0–5, texture brushes (bitmap images, brush transform, all five wrap modes), linear gradients (two colors, blend factors, preset colors; tile and mirrored wraps), path gradient fills through a `GradientBackend` (path boundary star-shaped from its center, one surrounding color or one per vertex, brush transform, clamped) | Hatch styles 6–52, metafile textures, linear gradients that are gamma corrected, clamped, have vertical blend factors or stops of differing alpha; path gradients with point (cardinal spline) boundaries, blend factors, preset colors, focus scales, gamma correction, tiled wraps, several figures or non-star-shaped boundaries, and path gradients on pens and text |
| Pens | World and Pixel widths, pen transform, flat/square/round caps chosen separately for start and end, NoAnchor and SquareAnchor caps, custom path and adjustable-arrow caps with `PlayOptions.CustomLineCaps` (an unverified interpretation; see below), miter/bevel/round joins, miter limit, predefined and custom dashes with offset, symmetric compound lines with miter joins, any supported brush | Custom and adjustable-arrow caps without `CustomLineCaps`, or with translucent paint, dashes, compound or zero-width pens, inconsistent cap data or insets past a line segment; triangle, round, diamond and arrow anchor caps, dashed pens with non-flat line caps, asymmetric compound lines, compound lines with bevel or round joins, dashes, open-figure caps or corners past the miter limit, clipped miter joins, alignments other than center, dash caps, dashed zero-width pens, other width units |
| Geometry | FillRects/DrawRects, FillPolygon/DrawLines (closed and open), FillEllipse/DrawEllipse, FillPie/DrawPie/DrawArc, FillPath/DrawPath, FillRegion, cardinal splines (open with offset and segments, closed, winding or alternate fill), DrawBeziers, Clear; compressed and relative points | — |
| Images | DrawImage and DrawImagePoints of bitmap images (encoded and raw), with source subrectangles inside the bitmap or clamped to transparent | Metafile images, image effects, fractional source rectangles, sources outside the bitmap without transparent clamping, alpha images under SourceCopy |
| Text | DrawDriverString through a `TextBackend`: Unicode code units or glyph indexes at explicit baseline origins or realized advances, fonts in every defined size unit with bold, italic, underline and strikeout styles, any supported brush, translation matrices | DrawString layout; vertical driver strings; driver-string matrices other than translations; Display-unit fonts; backends without `TextBackend` |

Interpretations where MS-EMFPLUS leaves room:

- World coordinates map to EMF device pixels through the world, container and
  page transforms. Page units convert with the EMF+ header's logical DPI. With
  no SetPageTransform the page is one device pixel per unit at scale 1.
- Under PixelOffsetMode Default, HighSpeed and None, integer device
  coordinates are pixel centers (MS-EMFPLUS 2.1.1.26). All geometry, clipping
  and images shift by half a device pixel; Half and HighQuality do not.
- A container maps its source rectangle onto its destination rectangle in the
  enclosing world space. The world transform and clip start reset, and the
  enclosing clip constrains drawing as a metaregion.
- EmfPlusPath carries no fill mode, so FillPath, path clips and path region
  nodes use GDI+'s default alternate rule. FillPolygon is alternate, and
  FillClosedCurve follows its winding flag.
- Pie and arc angles are clockwise from the x axis to the ray through the
  point, so elliptical arcs use geometric, not parametric, angles. Sweeps are
  limited to ±360 degrees. An empty DrawArc draws nothing.
- Cardinal spline segments become Béziers whose control points are offset by
  tension/3 of the neighboring chord; open curves repeat their end points.
- Linear gradients run from the left edge (0) to the right edge (1) of the
  brush rectangle in brush space, which the brush transform places in world
  space. Blend factors are fractions of the end color, interpolated linearly
  in sRGB.
- World-unit pen widths follow the pen and world transforms. Pixel-unit widths
  are device pixels. A zero width is a hairline. Unset miter limits are 10,
  GDI+'s default.
- Line caps follow MS-EMFPLUS 2.1.1.17 where it gives the geometry: Flat,
  Square, Round, NoAnchor (ends at the last point, like Flat) and
  SquareAnchor (a square of the line width centered on the end, like Square).
  The triangle's height and the other anchors' sizes are not given. Neither
  MS-EMFPLUS nor Microsoft's GDI+ reference gives the coordinate system of
  custom cap paths or where an adjustable arrow's vertex sits, so those caps
  are reported unless `PlayOptions.CustomLineCaps` opts into an interpretation
  that has not been checked against Windows:
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
    the line it overlaps is unknown. A dashed pen's line caps apply at figure ends and its dash
  cap at dash ends, which Stroke cannot express apart; dashed pens therefore
  need flat line caps.
- Compound lines (MS-EMFPLUS 2.2.2.9) are parallel bands across the pen
  width. Which side fraction 0 lies on is not specified, so only symmetric
  arrays are drawn, and how bands meet at bevel and round joins is not either.
  With miter joins, each band is exactly the full-width mitered stroke at its
  outer edge minus the one at its inner edge, provided no corner exceeds the
  miter limit (where GDI+ would bevel); Play checks every corner, and requires
  flat caps on open figures.
- A path gradient (MS-EMFPLUS 2.2.2.29) changes color along each line from
  the center to the boundary. For a boundary that is star-shaped from the
  center this is exactly a fan of Gouraud triangles from the center, which Play
  delivers to a `GradientBackend` with the filled shape as a final clip layer;
  WrapModeClamp leaves the area outside the boundary unpainted. Surrounding
  colors belong to the boundary vertices, or one color to all. Point
  boundaries are closed cardinal splines of unstated tension; blend positions
  are described only as running from a "midpoint" to an "endpoint"; focus
  scales give no scaling origin. Those are reported.
- SourceCopy compositing replaces pixels. That equals source-over for opaque
  paint, which is drawn; translucent paint is reported. Clear requires an
  opaque color for the same reason.
- EMF+ font sizes in World units are world units. Physical sizes are converted
  to device pixels with the header's vertical DPI and then to page units, so
  the world transform still scales and rotates text. Fonts are requested with
  a negative LOGFONT height (the em) and DEFAULT_CHARSET.
- A driver-string matrix is "applied to each value in the text array"
  (MS-EMFPLUS 2.3.4.6), which does not settle whether it moves the positions,
  the glyphs or both. Translations give the same result under every reading
  and are applied in world space; other matrices are reported. Decoration
  extents run from the first origin to the last origin plus that glyph's
  measured advance.
- DrawString is reported. Its layout depends on rules MS-EMFPLUS leaves open:
  the unit of the default 1/6 margins, how the 1.03 default tracking applies,
  line breaking, trimming and line spacing.
- Bitmap images decode once per object definition and are charged to the
  playback pixel budget then: the declared size first, then any excess of the
  decoded size. Region trees are converted to depth 256 at most. Clip steps
  share the PlayOptions bound.

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
render comparisons cover generated raster transfers, TIFFs, GDI playback
scenes for paths, transforms, mapping modes, objects, clipping, bitmaps, fills
and text, and EMF+ playback scenes for shapes, transforms, containers,
clipping, pens, curves, gradients and images. Known LibreOffice divergences
are pinned separately in ORACLES.md.
Playback unit tests and offline scene probes check spec-derived geometry and
pixels in `make check`; the pinned corpus must play with every omission reported.
