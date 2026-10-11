# Changelog

Notable changes to the `github.com/mgilbir/gowemf` module, newest first. Full
release notes are on the
[releases page](https://github.com/mgilbir/gowemf/releases).

## Unreleased

### Documentation

- Coverage, interpretations and oracle evidence moved from `COVERAGE.md` and
  `ORACLES.md` into [`docs/`](docs/index.md), with one canonical section per
  specification ruling. The old files now redirect.
- New guides for rendering with `Play`, inspecting records, and images and
  color. New references for limits, errors and `gowemfdump`. Diagrams of the
  architecture, coordinate spaces, embedded metafile playback, test tiers and
  the Windows oracle run.
- The package documentation now describes `Play`. `PlayOptions.ColorTransform`
  and `PlayOptions.Images` are documented.
- Statements that custom line caps and bidirectional text had not been
  compared with Windows were corrected: the Windows GDI/GDI+ run had already
  compared them.
- Runnable `ExamplePlay`, `ExampleStream` and `ExampleWalk` examples replace
  the internal `ExampleWalk`.

### Tests

- `make test-external` now fails on any omission in GDI playback of the
  pinned corpus, and on any change to the pinned EMF+ omissions.

## v0.1.1 — 2026-10-10

Maintenance release; the library is unchanged from v0.1.0.

- The test-only `rendercheck` module uses forme v0.10.0 (#45).

## v0.1.0 — 2026-10-10

First tagged release.

- Framing, typed record decoding and command streaming for MS-WMF, MS-EMF and
  MS-EMFPLUS. Unknown records are exposed as raw views; opaque drawing commands
  return `ErrUnsupported`.
- GDI playback through a small backend interface: paths, transforms, mapping
  modes, clipping, regions, palettes, gradients, raster operations, bitmap
  transfers and text laid out by GDI's rules, including double-byte code pages
  and bidirectional text.
- EMF+ playback: shapes, paths, pens with caps, joins, dashes and compound
  lines, solid, hatch, texture and gradient brushes, bitmap and metafile images,
  containers, clipping and driver strings.
- Bitmap decoding (DIB, raw EMF+ formats, PNG, JPEG, GIF, TIFF), image-effect
  parameters with lookup-table and color-matrix execution, and ICC color
  conversion through golittlecms.
- Configurable resource limits with finite defaults.
- Interpretations settled against Windows GDI and GDI+: EMF shape
  construction, StretchDIBits source origins, selections held by saved
  states, the object index equal to the header's handle count, monochrome
  brushes and pens under anisotropic page mappings.
