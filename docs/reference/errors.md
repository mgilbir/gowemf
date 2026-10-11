# Errors, partial output and input ownership

What gowemf returns when something fails, what has already happened by then,
and what the caller must keep stable. Reach for this when you handle errors,
need all-or-nothing output, or retain decoded values.

## Error values

Parser errors support `errors.Is` with:

| Error | Meaning |
| --- | --- |
| `ErrFormat` | Not a recognized metafile, or a record of an unknown format |
| `ErrMalformed` | The bytes contradict the specification's layout |
| `ErrLimit` | A [limit](limits.md) was exceeded |
| `ErrUnsupported` | Valid input that gowemf does not decode or draw; `Play` returns it when no `Unsupported` callback is set |

`errors.As` exposes a `*ParseError` with the byte `Offset` in the original
input and the failing `Field`. Callback and backend errors are returned
unchanged. `Header` metadata is returned only on success.

## Reported operations

`Play` delivers each operation it cannot draw faithfully to
`PlayOptions.Unsupported` as an `UnsupportedOperation`: the `Source` record
and a human-readable `Reason`. Omissions inside an embedded metafile are
reported against the drawing record, with the reason prefixed
`embedded metafile: `. Returning nil skips the operation; returning an error
stops `Play` with that error.

## Partial output

Callbacks and backend calls run as parsing progresses, so a later failure can
follow earlier callbacks. Do not publish partial output as a successfully
parsed document. For transactional output, validate with a nil `Stream`
callback (`Stream(data, options, nil)`, or `options.Stream` for `Play`) before
streaming the same immutable bytes into the renderer.

## Input ownership

Record views alias the input, with slice capacity restricted to the view. The
caller and callbacks must keep the input immutable during parsing. Retaining a
view keeps its input backing array alive; copy it if independent ownership is
needed. Keep borrowed input immutable for the entire lifetime of decoded
views, including delayed bitmap decoding. Independent calls share no mutable
parser state.
