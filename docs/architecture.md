# Architecture

gowemf reads a metafile in layers. Each layer checks more than the one below
and hands the caller something closer to a picture. Reach for this to choose
an entry point, or to see how `Play` drives a backend. All four functions take
the whole file as a byte slice and are bounded by
[limits](reference/limits.md).

```mermaid
flowchart LR
    B[(metafile bytes)] --> W["Walk(data, Limits, visit)<br/>framing only; zero-copy Record views"]
    W -->|Record| D["Decode(record, DecodeLimits)<br/>typed body or ErrUnsupported"]
    W --> S["Stream(data, StreamOptions, visit)<br/>typed Commands + object, save/restore<br/>and path-bracket checks"]
    S --> P["Play(data, PlayOptions, backend)<br/>device-context playback"]
    P --> BE["your Backend<br/>FillPath / StrokePath / DrawImage<br/>(+ Text / Gradient / Raster)"]
    P -.->|cannot draw faithfully| U["PlayOptions.Unsupported<br/>nil callback: ErrUnsupported"]
```

| Layer | Use it to | You get | You own |
| --- | --- | --- | --- |
| `Walk` | Validate framing, index records, extract embedded data | `Record` views (format, type, flags, offsets, raw bytes) and a `Header` | Interpreting every record |
| `Decode` | Read one record's fields | A typed body (see [decoding coverage](coverage/decoding.md)) | Object tables, state, drawing |
| `Stream` | Drive your own GDI/GDI+ interpreter | Typed `Command`s in playback order, with WMF object slots allocated, references checked and continued EMF+ objects assembled | Transforms, clipping, styles, text, drawing |
| `Play` | Render through a small backend | Fills, strokes, images and positioned text in destination coordinates | Rasterization, font realization, shaping |

## How Play works

`Play` checks the framing, maps the picture onto `PlayOptions.Destination`,
and then runs `Stream` with its own player as the visitor. The stream
selects the records to play: EMF+ records by default for EMF+ files, plus the
GDI records inside GetDC intervals, or only the GDI fallback of a Dual file
with `PlayOptions.Stream.PreferGDI`.

```mermaid
flowchart TD
    A["Play(data, opts, backend)"] --> V{"backend set and<br/>Destination finite, non-empty?"}
    V -- no --> E1[error]
    V -- yes --> W["Walk: framing pre-pass, Header"]
    W --> M["map EMF frame or WMF placeable bounds<br/>(or opts.Placeable) onto Destination"]
    M --> S["Stream(data, opts.Stream, player)"]
    S -->|EMF+ command| PP["EMF+ player<br/>GDI+ graphics state"]
    S -->|WMF/EMF command| GP["GDI player<br/>device context"]
    PP & GP --> R{"drawable?"}
    R -- yes --> BK["backend call"]
    R -- no --> UN{"opts.Unsupported set?"}
    UN -- no --> EU["stop: ErrUnsupported"]
    UN -- yes --> CB["callback: nil skips,<br/>an error stops Play"]
```

Backend calls happen as records play, so a later failure can follow
output that was already drawn. When output must be all-or-nothing, run
`Stream(data, opts.Stream, nil)` first (see [errors](reference/errors.md)).

## Backend capabilities

A backend implements `Backend` and may add optional interfaces. `Play`
checks for them per operation; whatever the backend cannot take is reported.

```mermaid
classDiagram
    class Backend {
      FillPath(Path, FillRule, Paint, Clip)
      StrokePath(Path, Stroke, Clip)
      DrawImage(ImageDraw, Clip)
    }
    class TextBackend {
      MeasureText(TextRun) TextMetrics
      DrawText(TextRun, Clip)
    }
    class GradientBackend {
      FillGradient([]GradientTriangle, Clip)
    }
    class RasterBackend {
      DrawRaster(RasterDraw, Clip)
    }
    Backend <|-- TextBackend
    Backend <|-- GradientBackend
    Backend <|-- RasterBackend
```

| Interface | Without it |
| --- | --- |
| `TextBackend` | All text is reported |
| `GradientBackend` | EMR_GRADIENTFILL and EMF+ path-gradient fills are reported |
| `RasterBackend` | Bitmap and pattern transfers whose raster operation combines source, pattern and destination are reported, except an exact SRCAND/SRCPAINT sprite pair, which is drawn as one masked image ([ruling](interpretations/gdi.md#srcand-and-srcpaint-sprite-pairs)) |

The [rendering guide](guide/rendering.md) describes what each call receives.

## Repository layout

| Path | Contents |
| --- | --- |
| `*.go` (root) | The `gowemf` package: framing (`metafile.go`, `wmf.go`, `emf.go`, `emfplus.go`), decoding (`decode*.go`, `plus_*.go`), streaming (`stream*.go`), playback (`playback*.go`), images (`dib*.go`, `image.go`, `gif.go`, `tiff*.go`, `effects.go`) and color (`color*.go`) |
| `cmd/gowemfdump` | JSON record dump for diagnostics ([reference](reference/gowemfdump.md)) |
| `internal/bidi`, `internal/codepage` | UAX #9 and code-page table support, with their generators `internal/bidigen` and `internal/codepagegen` |
| `internal/corpus`, `internal/corpusfetch` | Pinned download manifests and the downloader |
| `internal/oracle`, `internal/raster` | Test-only LibreOffice runner and reference rasterizer |
| `rendercheck/` | Separate test-only module for text render oracles (needs Go ≥ 1.26) |
| `tools/` | Java POI adapters and the Windows oracle script |
