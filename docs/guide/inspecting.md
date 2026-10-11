# Inspecting and streaming records

How to read a metafile's records without playing it: validate framing with
`Walk`, decode one record with `Decode`, or drive your own interpreter with
`Stream`. Reach for this to build a diagnostic tool, extract embedded data,
or render with your own GDI state machine. To render through a backend
instead, see [rendering with Play](rendering.md).

## Walk: framing

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
lengths/counts are rejected. Some real-world producers write inconsistent
metadata: the narrow
[embedded placeable-WMF rule](../interpretations/decoding.md#embedded-placeable-wmf-compatibility)
is the only accepted exception, with generated and pinned-corpus regression
tests.

Unknown records are exposed as raw views. Acceptance by the framing parser is
**not** a claim that record bodies are valid or supported for playback.
`Decode` and `Stream` report `ErrUnsupported` instead of accepting opaque
drawing commands. For EMF+ files, `Walk` exposes both the EMF+ records and
the GDI records for inspection.

## Decode: one record

`Decode(record, limits)` returns a typed body when a consumer wants to manage
its own state or inspect unsupported families; `DecodePlusObject` decodes a
complete EMF+ object. The body types per record are listed in
[decoding coverage](../coverage/decoding.md).

Named wire identifiers (`MetaStretchDIB`, `EMRPolygon`, `PlusDrawStringRecord`,
and others in `record_types.go`) avoid hard-coded opcode values. Naming a
record does not imply support for every encoding or extension it can contain.

## Stream: typed commands for your own renderer

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

`Stream` allocates WMF object slots in specification order, verifies EMF and
EMF+ object references, assembles continued EMF+ objects, and validates
save/restore and EMF path-bracket sequencing. `Command.ObjectID` identifies
newly created objects. State records remain commands: the renderer owns
transforms, clipping, selected styles, text metrics, and drawing.

For EMF+ files `Stream` selects the EMF+ records and includes GDI commands
only during `EmfPlusGetDC` intervals. Set `StreamOptions.PreferGDI` to
request the GDI fallback of an EMF+ Dual file; EMF+ Only files reject that
request.

Serializable image-effect records decode all eleven standard effect parameter
blocks. `Stream` requires a prior effect for effect-enabled
`DrawImagePoints` and binds the latest description through `Command.Effect`.
GDI commands carry immutable `ColorState` snapshots reflecting save/restore
and profile selection.

## Embedded data and dimensions

- `Header.SizeInPoints()` returns recorded physical dimensions when present.
- `PlusImage.MetafileBytes(limits)` exposes metafiles embedded in EMF+ images
  without recursively parsing them.
- `ExtractEnhancedMetafile` explicitly assembles an EMF carried by WMFC escape
  fragments, checks the complete stream checksum and both containers'
  framing, and returns owned bytes. It does not replace the WMF fallback
  automatically.

## From the command line

`go run ./cmd/gowemfdump -summary file.emf` prints one JSON line per record;
see the [gowemfdump reference](../reference/gowemfdump.md).

Errors, partial output and input ownership are covered in
[errors](../reference/errors.md); budgets in [limits](../reference/limits.md).
