# gowemfdump

`gowemfdump` prints one JSON object per record of a metafile, for diagnosing
files and comparing decoders. Reach for it to see what `Walk` and `Decode`
make of a file without writing code.

```sh
go run ./cmd/gowemfdump [-summary] [-strict] [-offset n] file
```

| Flag | Effect |
| --- | --- |
| `-summary` | Omit decoded fields (`body`) |
| `-strict` | Fail on records `Decode` does not support, instead of marking them `"unsupported"` |
| `-offset n` | Print only the record at byte offset `n` (default `-1`: all records) |

Each line has these fields:

| Field | Meaning |
| --- | --- |
| `format` | `WMF`, `EMF` or `EMF+` |
| `type` | Record type in that format's namespace |
| `flags` | EMF+ record flags (0 otherwise) |
| `offset` | Byte offset of the record |
| `parent` | Offset of the enclosing EMR_COMMENT for EMF+ records, -1 otherwise |
| `bytes` | Record size including framing |
| `status` | `decoded` or `unsupported` |
| `bodyType` | Go type of the decoded body, when there is one |
| `body` | Decoded fields, unless `-summary` |

Example, on a pinned corpus file:

```console
$ go run ./cmd/gowemfdump -offset 128 .external/poi/slideshow/wrench.emf
{"format":"EMF","type":18,"flags":0,"offset":128,"parent":-1,"bytes":12,"status":"decoded","bodyType":"gowemf.Value","body":{"Value":1}}
```

Every record is still framed and decoded with `-offset`, so a malformed
record anywhere in the file fails the run. Input is limited to 64 MiB and
output to 128 MiB (`ErrLimit`). On any error the command prints it to stderr
and exits with status 1.
