# Decoding images and converting color

How to turn the bitmaps inside metafiles into Go images, and how to convert
color-managed ones to sRGB. Reach for this when you handle bitmap records
yourself (with `Stream` or `Decode`) or when `Play` reports a color-managed
bitmap. The format inventory is in
[images and color coverage](../coverage/images-and-color.md).

## Bitmaps

`ParseDIB` handles separate EMF bitmap-info/pixel buffers; `ParsePackedDIB`
handles WMF packed DIBs. `DIB.Image()` decodes RGB, bitfields, indexed colors,
RLE4/RLE8, PNG and JPEG. `DIB.AlphaImage()` applies the premultiplied BGRA
interpretation required by AlphaBlend; ordinary RGB32 is opaque.
`PlusImage.Image(limits)` decodes PNG/JPEG/GIF/TIFF and all defined raw EMF+
formats, including indexed palettes, grayscale, RGB555/565/ARGB1555 and
48/64-bit RGB/ARGB/PARGB. High-depth pixels retain 16-bit channels in Go's
`Gray16`, `NRGBA64`, or `RGBA64` image types. These use only Go's standard
library. Image dimensions and byte/pixel budgets are checked before
allocation.

GIF decoding returns the first frame positioned on its logical canvas. Opaque
GIFs use the global background color where specified; transparent first
frames use a transparent canvas. Later animation frames are neither loaded nor
validated. Plain-text rendering extensions are explicitly unsupported.
Canvas/frame bounds and palette spans are checked before the standard-library
decoder runs.

`ParseTIFF` supports bounded classic stripped gray/palette/RGB TIFFs,
including PackBits, TIFF LZW, Deflate, orientation, predictors, separate
planes and alpha. It processes only the first IFD and requires explicit ICC
conversion when tagged. Unsupported TIFF layouts are listed in the
[TIFF profile](../coverage/images-and-color.md#tiff-profile); no extra Go
dependency is used.

## Image effects

`ApplyImageEffect` executes the EMF+ lookup-table and color-matrix effects
with bounded straight-RGBA output. Other filter algorithms remain explicitly
unsupported by that helper.

## Color management

V4/V5 DIB color-space metadata and bounded embedded/linked profile spans are
exposed by `DIB.ColorSpace()`. `Image()` and `AlphaImage()` reject
color-managed input when conversion is required. To convert it explicitly:

```go
img, err := dib.ImageWithColorTransform(gowemf.NewColorTransform)
// For AC_SRC_ALPHA input:
// img, err := dib.AlphaImageWithColorTransform(gowemf.NewColorTransform)
```

`Play` does the same when `PlayOptions.ColorTransform` is set; without it,
color-managed bitmaps are reported.

The golittlecms adapter supports RGB ICC profiles, calibrated XYZ
endpoints/gamma, Windows-to-ICC rendering intents, and optional black-point
compensation, with straight-alpha RGBA output. For a color space selected by
EMF records, use `ConvertToSRGB` with the selected `ColorSpace`. Applying DC
profile selection and halftone color adjustments remains the renderer's
responsibility. `NewPixelColorTransform` supports gray/CMYK buffers, and
`NewProofingColorTransform` provides explicit target-profile soft proofing.

Named profile paths are never opened automatically.
`NewColorTransformWithOptions` accepts an explicit resolver callback if the
application wants to supply profile bytes for a name. ICC envelopes, tag
ranges, tag counts, referenced tag bytes and pixel buffers are checked before
invoking the backend; tag semantics are handled by golittlecms with its own
allocation guards still enabled. See
[THIRD_PARTY_NOTICES.md](../../THIRD_PARTY_NOTICES.md) for its MIT notice.
