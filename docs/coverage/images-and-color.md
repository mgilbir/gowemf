# Images and color coverage

Which bitmap encodings `ParseDIB`, `ParsePackedDIB`, `PlusImage.Image` and
`ParseTIFF` decode, and what the golittlecms color adapter converts. Reach for
this before handing a decoded image to a renderer or when a color-managed
image is rejected. Limits are in the [README resource model](../../README.md#resource-model).

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

TIFF parsing follows the TIFF 6.0 structure, strip, PackBits, LZW, predictor
and alpha descriptions. `ParseTIFF` reads the first classic II/MM IFD, with a 4096-entry metadata cap and
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
