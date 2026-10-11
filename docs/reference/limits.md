# Limits

Every resource bound gowemf enforces, with its default. Reach for this when
you process untrusted input or a legitimate file hits `ErrLimit`. A zero
field selects the default; there is no unlimited mode, and limits can be
raised explicitly but never disabled.

| Option field | Default | Bounds |
| --- | --- | --- |
| `Limits.MaxBytes` | 64 MiB | Input file size |
| `Limits.MaxRecordBytes` | 16 MiB | One record |
| `Limits.MaxRecords` | 1,000,000 | Records, outer and nested EMF+ together |
| `DecodeLimits.MaxElements` | 1,000,000 | Elements of one typed array |
| `DecodeLimits.MaxObjectBytes` | 16 MiB | Typed record and assembled object bytes, and separately the cumulative expanded-array allocation of one decode |
| `DecodeLimits.MaxNesting` | 256 | Nested EMF+ objects and region-tree levels |
| `StreamOptions.MaxObjects` | 65,536 | Object slots |
| `StreamOptions.MaxSavedStates` | 1,024 | Saved states |
| `PlayOptions.MaxPathPoints` | 4,000,000 | Points per path |
| `PlayOptions.MaxImagePixels` | 64,000,000 | Cumulative decoded bitmap pixels (and palette entries) per `Play` call |
| `PlayOptions.MaxClipSteps` | 4,096 | Clip steps across the clip and metaregions |
| `PlayOptions.MaxMetafileDepth` | 4 | Nesting of EMF+ metafile images |
| `PlayOptions.MaxEmbeddedRecords` | 1,000,000 | Records of embedded metafiles framed and played per `Play` call, however often each is drawn |
| (fixed) | 16,000,000 steps | FrameRgn band sweeps per `Play` call |
| `ImageLimits.MaxBytes` | 16 MiB | Encoded image input |
| `ImageLimits.MaxPixels` | 16,000,000 | Output pixels per image (64 MiB as NRGBA) |
| `ImageLimits.MaxDecodedBytes` | 128 MiB | TIFF inflation and effect working storage |
| `ColorTransformOptions.MaxProfileBytes` | 16 MiB | ICC profile bytes, and the sum of referenced tag sizes |
| `ColorTransformOptions.MaxProfileTags` | 4,096 | ICC tags |
| `ColorTransformOptions.MaxPixels` | 16,000,000 | Pixels per transform call |
| `gowemfdump` (fixed) | 64 MiB in, 128 MiB out | Input file and JSON output |

`PlayOptions.Stream` and `PlayOptions.Images` apply the stream and image
limits during playback.

## What is and is not counted

- Nested EMF+ pens, caps, brushes, paths and images share the enclosing
  decoded-allocation budget rather than resetting it for each nested object.
  Region-node storage is accounted conservatively.
- Arc construction is bounded per record. Backends retaining paths or clip
  masks own that memory.
- Returned images and codec temporary storage are additional to the input
  buffer. TIFF byte products are checked before multiplication or allocation,
  including on 32-bit hosts.
- Color conversion has a separate output image and one staging row; backend
  profile/transform storage is additional.
- Embedded metafiles share the `Play` call's pixel and region budgets.

## How the bounds are applied

Sizes are checked using widened arithmetic before conversion to native
indexes. The walker does not allocate from declared counts, copy record
bodies, recurse into embedded images, or collect records. Typed
relative-coordinate/RLE arrays and image decoding allocate only after budget
checks. WMF slot allocation uses a min-heap to avoid quadratic
lowest-free-slot scans. Continued-object buffers are capped and cannot grow
from an unchecked declared size.

Callers reading untrusted streams must also bound input acquisition; `Walk`
cannot undo allocation performed before it is called.
