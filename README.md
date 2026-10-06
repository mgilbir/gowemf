# gowemf

A pure-Go library for parsing Windows metafiles for embedded-image rendering in
[spine](https://github.com/mgilbir/spine). MIT licensed. Requires Go 1.25 or later.
The sole external Go dependency is [golittlecms](https://github.com/mgilbir/golittlecms)
for color management; no cgo or native runtime is needed.

Supports WMF, EMF, and EMF+ containers, typed record decoding, renderer-facing
command streaming, object/state checks, and bitmap decoding. Vector playback and
text shaping belong to the consuming renderer. The supported record/encoding
inventory and outstanding format coverage are explicit in [COVERAGE.md](COVERAGE.md).

## Current coverage

| Format | Implemented | Still opaque / not implemented |
| --- | --- | --- |
| MS-WMF | Standard/placeable framing; core drawing/state/object records; text; DIB and Bitmap16 layouts; enhanced-EMF fragment decoding and explicit checksummed extraction | Other escape subtypes; device-dependent bitmap color realization |
| MS-EMF | Framing; geometry/transforms/paths; objects; text; regions; palettes; gradients; raster transfers; logical color spaces, ICM/profile and color-adjustment records | Header/font extensions remain partially opaque; driver/OpenGL extensions; playback-level color adjustment/proofing |
| MS-EMFPLUS | Drawing/property/transform records including curves, driver strings and containers; object continuation; all five brush families; pens/custom caps; validated paths; regions; images in every defined raw pixel format; fonts/string formats/image attributes | Effects/terminal-server records, StrokeFillPath, GIF/TIFF compressed images |

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

### Bitmap decoding

`ParseDIB` handles separate EMF bitmap-info/pixel buffers; `ParsePackedDIB` handles
WMF packed DIBs. `DIB.Image()` decodes RGB, bitfields, indexed colors, RLE4/RLE8,
PNG and JPEG. `DIB.AlphaImage()` applies the premultiplied BGRA interpretation
required by AlphaBlend; ordinary RGB32 is opaque. `PlusImage.Image()` decodes
PNG/JPEG and all defined raw EMF+ formats, including indexed palettes, grayscale,
RGB555/565/ARGB1555 and 48/64-bit RGB/ARGB/PARGB. High-depth pixels retain 16-bit
channels in Go's `Gray16`, `NRGBA64`, or `RGBA64` image types. These use only Go's standard
library. Image dimensions and byte/pixel budgets are checked before allocation.

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
halftone color adjustments and soft proofing remains the renderer's responsibility.

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
- Bitmap decoding: 16 MiB encoded input and 16,000,000 output pixels. Returned
  images and codec temporary storage are additional to the input buffer.
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
make fuzz           # five 30-second fuzz targets, two workers each
make bench          # generated-input microbenchmarks with allocation counts
make corpus-download
make test-external  # fetch/verify pinned inputs, then run corpus assertions
make test-oracle    # download pinned POI jars; requires java and javac
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
the typed command stream to spine's renderer; add tolerant pixel comparisons
against LibreOffice and, when available, Windows GDI/GDI+. Windows remains the
primary playback oracle. No pixel-perfect or full-format rendering claim is made
by parser/corpus success. Fonts and metrics must be controlled for meaningful
cross-renderer comparisons.
