# gowemf

A pure-Go library for parsing Windows metafiles for embedded-image rendering in
[spine](https://github.com/mgilbir/spine). MIT licensed. Requires Go 1.25 or later.
The sole external Go dependency is [golittlecms](https://github.com/mgilbir/golittlecms)
for color management; no cgo or native runtime is needed.

Supports WMF, EMF, and EMF+ containers, typed record decoding, renderer-facing
command streaming, object/state checks, bitmap decoding, and GDI and EMF+
playback of paths, transforms, clipping and bitmaps through a small backend
interface. GDI text is laid out with GDI's rules through an optional text
backend. Rasterization, font realization and shaping belong to the consuming
renderer.
The supported record/encoding inventory and outstanding format coverage are
explicit in [COVERAGE.md](COVERAGE.md).

## Current coverage

| Format | Implemented | Still opaque / not implemented |
| --- | --- | --- |
| MS-WMF | Standard/placeable framing; core drawing/state/object records; text; DIB and Bitmap16 layouts; enhanced-EMF fragment decoding and explicit checksummed extraction; other escapes and private comments as untyped data | Typed layouts of printer escapes; device-dependent bitmap color realization |
| MS-EMF | Framing/header extensions; geometry/transforms/paths; objects and extended fonts; text; regions/palettes/gradients/raster transfers; logical color spaces and saved color state | Pixel-format descriptor interpretation; driver/OpenGL extensions; halftone color-adjustment algorithms |
| MS-EMFPLUS | Drawing/state records including terminal-server layouts; objects; all raw bitmap formats plus PNG/JPEG/GIF/TIFF; fonts/images; effect parameters with lookup/matrix playback | StrokeFillPath; remaining effect algorithms; TIFF extensions outside the documented subset |

Unknown records are exposed as raw views. Acceptance by the framing parser is
**not** a claim that record bodies are valid or supported for playback. `Decode`
and `Stream` report `ErrUnsupported` instead of accepting opaque drawing commands.
For EMF+ files, `Walk` exposes both streams for inspection; `Stream` selects EMF+
by default and includes GDI commands only during `EmfPlusGetDC` intervals.

## API

```go
header, err := gowemf.Walk(data, gowemf.Limits{}, func(r gowemf.Record) error {
    // r.Format selects the namespace for r.Type.
    // r.Raw includes framing; r.Data contains the opaque record body.
    // EMF+ records identify the enclosing comment via r.ParentOffset.
    return inspect(r)
})
if err != nil {
    return err
}
_ = header
```

Pass a nil callback to validate only framing. The caller supplies exactly one
complete metafile as a byte slice; trailing bytes and inconsistent declared
lengths/counts are rejected. Some real-world producers may have inconsistent
metadata: the narrow embedded-placeable-WMF compatibility rule is documented in
[ORACLES.md](ORACLES.md), with generated and pinned-corpus regression tests.

### Typed commands for a renderer

```go
header, err := gowemf.Stream(data, gowemf.StreamOptions{}, func(c gowemf.Command) error {
    switch body := c.Body.(type) {
    case gowemf.Poly:
        // Iterate body.Points using Len()/At(); body.Counts splits polypolygons.
        // c.Source.Type distinguishes polygon, polyline, Bezier, and "To" forms.
        return renderer.AcceptPoly(c.Source, body)
    default:
        return renderer.Accept(c)
    }
})
```

`Stream` allocates WMF object slots in specification order, verifies EMF and EMF+
object references, assembles continued EMF+ objects, and validates save/restore
and EMF path-bracket sequencing. `Command.ObjectID` identifies newly created
objects. State records remain commands: the renderer owns transforms, clipping,
selected styles, text metrics, and drawing. Set `PreferGDI` to request the GDI
fallback of an EMF+ Dual file; EMF+ Only files reject that request.

`Decode(record, limits)` is available separately when a consumer wants to manage
its own state or inspect unsupported families. `Header.SizeInPoints()` returns
recorded physical dimensions when present. `PlusImage.MetafileBytes()` exposes
embedded metafiles without recursively parsing them.
`ExtractEnhancedMetafile` explicitly assembles an EMF carried by WMFC escape
fragments, checks the complete stream checksum and both containers' framing,
and returns owned bytes. It does not replace the WMF fallback automatically.
Named wire identifiers (`MetaStretchDIB`, `EMRPolygon`, `PlusDrawStringRecord`,
and others in `record_types.go`) avoid hard-coded opcode values. Naming a record
does not imply support for every encoding or extension it can contain.

### GDI playback

```go
var missing []gowemf.UnsupportedOperation
_, err := gowemf.Play(data, gowemf.PlayOptions{
    Destination: gowemf.Box{Width: 800, Height: 600},
    Unsupported: func(u gowemf.UnsupportedOperation) error {
        missing = append(missing, u) // or return an error to stop
        return nil
    },
}, backend) // FillPath, StrokePath and DrawImage
```

`Play` replays a WMF, EMF or EMF+ file. For GDI records it keeps the playback device
context: SaveDC/RestoreDC, selected pens and brushes with stock objects and
object-slot reuse, all eight mapping modes, window/viewport origins and extents,
world transforms, background/fill/ROP2/stretch modes, arc direction, brush
origin, current position, EMF path brackets, clipping and metaregions. Backends
receive geometry already in destination coordinates: paths of moves, lines,
cubic Béziers and closures with a fill rule; effective paints (solid, hatch or
pattern, after ROP2 and background mode); effective pens with caps, joins,
dashes and a pen-space transform; images with an affine placement, source
rectangle and opacity; and an immutable clip chain of path areas and region
trees combined with intersect/union/xor/difference/complement/replace/offset
steps.

The EMF header frame, or the WMF placeable bounds, is mapped onto `Destination`.
WMF files without a placeable header need `PlayOptions.Placeable`. Flood fill,
region inversion, destination-dependent raster operations and other omissions
are never skipped silently.

EMF+ files play their EMF+ records, together with the GDI records inside GetDC
intervals; `Stream.PreferGDI` plays the GDI fallback of a Dual file instead.
EMF+ playback keeps the GDI+ graphics state: Save/Restore, containers,
world and page transforms, pixel offset, compositing and interpolation modes,
and clipping with all combine modes and region trees. It resolves solid,
hatch, texture and linear-gradient brushes (`PaintLinearGradient`), pens with
caps, joins, dashes and dash offsets, shapes, paths, cardinal splines and
bitmap images. EMF+ driver strings go to a `TextBackend` like GDI text.
Path gradient fills, symmetric compound pens and separate start and end caps
are drawn where MS-EMFPLUS defines them. Custom and arrow caps are drawn only
with `PlayOptions.CustomLineCaps`, under an interpretation not verified against
Windows. DrawString layout and metafile images are reported as unsupported.

Region painting (FillRgn, PaintRgn, FrameRgn and the WMF region records),
monochrome pattern brushes colored by the text and background colors, logical
palettes with PALETTEINDEX colors and DIB_PAL_COLORS bitmaps, and WMF region
clipping are resolved into ordinary fills and clips. Gradient fills are
delivered as Gouraud-shaded triangle meshes to backends that also implement
`GradientBackend`; others get them reported.

Backends that also implement `TextBackend` (`MeasureText` and `DrawText`)
receive text. `Play` does the GDI placement itself: text alignment, explicit
and default spacing with character extra and justification, escapement,
GM_COMPATIBLE versus GM_ADVANCED transforms, background cells, opaque and clip
rectangles and current-position updates. It asks the backend only for each
run's ascent, descent and advances, and hands it positioned code units or glyph
indexes with a text-space transform; font realization, shaping, underline and
strikeout are the backend's. ANSI strings are decoded with the Windows code
pages that MS-UCODEREF names; text in the system-dependent DEFAULT_CHARSET is
reported unless `PlayOptions.DefaultCharSet` states what the producing system
used, and double-byte and OEM character sets are reported. Without an `Unsupported` callback
`Play` stops with `ErrUnsupported`; with one, every skipped operation is reported
so a partial picture cannot be mistaken for a complete one. COVERAGE.md lists
the playback inventory and the interpretations chosen where the specifications
leave room; ORACLES.md records how the output was checked against LibreOffice.

Serializable image-effect records decode all eleven standard effect parameter
blocks. `Stream` requires a prior effect for effect-enabled `DrawImagePoints`
and binds the latest description through `Command.Effect`. `ApplyImageEffect`
executes lookup tables and color matrices with bounded straight-RGBA output.
Other filter algorithms remain explicitly unsupported by that helper.

### Bitmap decoding

`ParseDIB` handles separate EMF bitmap-info/pixel buffers; `ParsePackedDIB` handles
WMF packed DIBs. `DIB.Image()` decodes RGB, bitfields, indexed colors, RLE4/RLE8,
PNG and JPEG. `DIB.AlphaImage()` applies the premultiplied BGRA interpretation
required by AlphaBlend; ordinary RGB32 is opaque. `PlusImage.Image()` decodes
PNG/JPEG/GIF/TIFF and all defined raw EMF+ formats, including indexed palettes, grayscale,
RGB555/565/ARGB1555 and 48/64-bit RGB/ARGB/PARGB. High-depth pixels retain 16-bit
channels in Go's `Gray16`, `NRGBA64`, or `RGBA64` image types. These use only Go's standard
library. Image dimensions and byte/pixel budgets are checked before allocation.
GIF decoding returns the first frame positioned on its logical canvas. Opaque
GIFs use the global background color where specified; transparent first frames
use a transparent canvas. Later animation frames are neither loaded nor validated.
Plain-text rendering extensions are explicitly unsupported. Canvas/frame bounds
and palette spans are checked before the standard-library decoder runs.
`ParseTIFF` supports bounded classic stripped gray/palette/RGB TIFFs, including
PackBits, TIFF LZW, Deflate, orientation, predictors, separate planes and alpha.
It processes only the first IFD and requires explicit ICC conversion when tagged.
See COVERAGE.md for unsupported TIFF layouts; no extra Go dependency is used.

### Color management

V4/V5 DIB color-space metadata and bounded embedded/linked profile spans are
exposed by `DIB.ColorSpace()`. `Image()` and `AlphaImage()` reject color-managed
input when conversion is required. To convert it explicitly:

```go
img, err := dib.ImageWithColorTransform(gowemf.NewColorTransform)
// For AC_SRC_ALPHA input:
// img, err := dib.AlphaImageWithColorTransform(gowemf.NewColorTransform)
```

The golittlecms adapter supports RGB ICC profiles, calibrated XYZ endpoints/gamma,
Windows-to-ICC rendering intents, and optional black-point compensation, with
straight-alpha RGBA output. For a color space selected by EMF records, use
`ConvertToSRGB` with the selected `ColorSpace`. Applying DC profile selection,
halftone color adjustments remains the renderer's responsibility. GDI commands
include immutable `ColorState` snapshots reflecting save/restore and profile
selection. `NewPixelColorTransform` supports gray/CMYK buffers, and
`NewProofingColorTransform` provides explicit target-profile soft proofing.

Named profile paths are never opened automatically. `NewColorTransformWithOptions`
accepts an explicit resolver callback if the application wants to supply profile
bytes for a name. ICC envelopes, tag ranges, tag counts, referenced tag bytes and
pixel buffers are checked before invoking the backend; tag semantics are handled
by golittlecms with its own allocation guards still enabled. See
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for its MIT notice.

Callbacks run as parsing progresses, so a later failure can follow earlier
callbacks. Do not publish partial output as a successfully parsed document.
Callback errors are returned unchanged. Parser errors support `errors.Is` with
`ErrFormat`, `ErrMalformed`, `ErrLimit`, and `ErrUnsupported`; `errors.As` exposes a `ParseError`
with the byte offset and failing field. Metadata is returned only on success.

Record views alias the input, with slice capacity restricted to the view. The
caller and callback must keep the input immutable during parsing. Retaining a
view keeps its input backing array alive; copy it if independent ownership is
needed. Keep borrowed input immutable for the entire lifetime of decoded views,
including delayed bitmap decoding. Independent calls share no mutable parser
state. For transactional output, validate with a nil `Stream` callback before
streaming the same immutable bytes into the renderer.

### Resource model

Zero-valued limits select finite defaults:

- File: 64 MiB.
- Individual record: 16 MiB.
- Records: 1,000,000, including outer and nested EMF+ records.
- Typed arrays: 1,000,000 elements; 16 MiB record/object bytes and a separate
  cumulative 16 MiB expanded-array allocation budget per decode.
- Nested EMF+ objects and region trees: 256 levels, with conservative region-node
  storage accounting. Pens, caps, brushes, paths and images share the enclosing
  decoded-allocation budget rather than resetting it for each nested object.
- Command stream: 65,536 object slots and 1,024 saved states.
- Playback: 4,000,000 points per path, 64,000,000 cumulative decoded bitmap
  pixels per `Play` call and 4,096 clip steps across the clip and metaregions.
  Arc construction is bounded per record; backends retaining paths or clip
  masks own that memory.
- Bitmap decoding: 16 MiB encoded input and 16,000,000 output pixels. Returned
  images and codec temporary storage are additional to the input buffer.
- TIFF/effect decoded storage: 128 MiB by default; TIFF byte products are checked
  before multiplication or allocation, including on 32-bit hosts.
- Color conversion: 16 MiB profile bytes/referenced tag bytes, 4,096 ICC tags,
  and 16,000,000 pixels per transform call. Conversion has a separate output
  image and one staging row; backend profile/transform storage is additional.

Sizes are checked using widened arithmetic before conversion to native indexes.
The walker does not allocate from declared counts, copy record bodies, recurse
into embedded images, or collect records. Typed relative-coordinate/RLE arrays
and image decoding allocate only after budget checks. WMF slot allocation uses
a min-heap to avoid quadratic lowest-free-slot scans. Continued-object buffers
are capped and cannot grow from an unchecked declared size. Limits can be explicitly adjusted but zero
never disables a limit. Callers reading untrusted streams must also bound input
acquisition; `Walk` cannot undo allocation performed before it is called.

## Development

```sh
make check          # format, build, tests, vet, race, 32-bit tests
make fuzz           # seven 30-second fuzz targets, two workers each
make bench          # generated-input microbenchmarks with allocation counts
make corpus-download
make test-external  # fetch/verify pinned inputs, then run corpus assertions
make test-oracle    # download pinned POI jars; requires java and javac
make test-render    # LibreOffice/POI raster, GDI playback and text comparisons; requires Liberation Sans, prlimit and Go 1.26 for rendercheck
make test-external  # also verifies codepage_tables.go against the pinned code page files
make codepages      # regenerate codepage_tables.go from the pinned files
go run ./cmd/gowemfdump -summary file.emf
go run ./cmd/gowemfdump -offset 128 file.emf
```

Ordinary tests are offline and construct fixtures in Go. No external binary
fixtures or document examples are tracked. The opt-in downloader uses only the
Go standard library, fixed upstream revisions, exact byte lengths and SHA-256
digests, bounded reads, timeouts, and verified temporary files before installation.
It refuses to reuse a corrupt cached file. All downloaded inputs live under
`.external/`, which is gitignored. `make test-external` fails on missing or corrupt
inputs rather than silently skipping them.

The manifest is in `internal/corpus/manifest.go`. Its six pinned Apache POI inputs
cover WMF, EMF, EMF+, and an embedded WMF object. Record counts were independently
inspected; corpus tests require typed decoding and native stream validation.
The separate POI oracle compares record sequences and supported field values,
including polygon coordinates, origins/extents, handles, and text reference points.
Generated EMF+ paths add independent point/type checks. See [ORACLES.md](ORACLES.md)
for normalization rules, known POI discrepancies, and unverified render behavior.
Upstream repository licensing does not establish redistribution rights for
every embedded document: downloaded inputs must remain uncommitted and unshipped.

## Specification and licensing policy

Primary sources (published April 23, 2024):

- [MS-WMF, revision 18.0](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-wmf/): §§2.3.2.2–2.3.2.3 for headers.
- [MS-EMF, revision 18.0](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-emf/): §§2.2.9, 2.3, 2.3.3, 2.3.4 for common header, framing, comments and EOF.
- [MS-EMFPLUS, revision 20.0](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-emfplus/): §§2.2.2.19, 2.3.3.1, 2.3.3.3 for graphics version, EOF and header.

The current code was written from these specifications; no third-party
implementation code has been ported.

- Kaitai WMF (CC0) and unidoc/emf (MIT) are potential port sources, subject to
  verification of the particular revision and retention of required notices.
- Apache POI is an execution-only parse oracle. A source port would require a deliberate
  licensing decision and preservation of Apache license/NOTICE obligations.
- LibreOffice, libemf2svg, and Inkscape are behavioral oracles, not port sources.
  Avoid their implementation source when developing this library.
- Verify licenses before using other implementations or corpora.

Required license notices in source are separate from commit/PR attribution.
Commits and PR descriptions must contain no AI attribution.

## Remaining integration and conformance work

Implement the remaining record/encoding families listed in COVERAGE.md; connect
`Play` to spine's renderer; add double-byte text and EMF+ DrawString to playback;
verify the custom line cap interpretation; and compare against Windows GDI/GDI+ when
available. Windows remains
the primary playback oracle. LibreOffice agreement covers the generated scenes in
ORACLES.md and diverges from the specifications in several pinned cases. No
pixel-perfect or full-format rendering claim is made. Fonts and metrics must be
controlled for meaningful cross-renderer text comparisons.
