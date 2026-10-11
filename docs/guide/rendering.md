# Rendering with Play

How to render a WMF, EMF or EMF+ file into your own renderer with `Play`.
Reach for this when you implement a backend. What `Play` draws and what it
reports is in [GDI](../coverage/playback-gdi.md) and
[EMF+](../coverage/playback-emfplus.md) playback coverage; how the layers fit
together is in the [architecture](../architecture.md).

## Minimal playback

```go
type canvas struct{ /* your renderer */ }

func (c *canvas) FillPath(p gowemf.Path, rule gowemf.FillRule, paint gowemf.Paint, clip gowemf.Clip) error {
	return nil // fill p with paint under rule, inside every region of clip
}
func (c *canvas) StrokePath(p gowemf.Path, s gowemf.Stroke, clip gowemf.Clip) error {
	return nil // stroke p with the effective pen s
}
func (c *canvas) DrawImage(img gowemf.ImageDraw, clip gowemf.Clip) error {
	return nil // composite img.Source of img.Image through img.Transform
}

var reported []gowemf.UnsupportedOperation
_, err := gowemf.Play(data, gowemf.PlayOptions{
	Destination: gowemf.Box{Width: 800, Height: 600},
	Unsupported: func(u gowemf.UnsupportedOperation) error {
		reported = append(reported, u) // or return an error to stop
		return nil
	},
}, &canvas{})
```

The EMF header frame, or the WMF placeable bounds, is mapped onto
`Destination`. WMF files without a placeable header need
`PlayOptions.Placeable`. Without an `Unsupported` callback `Play` stops with
`ErrUnsupported` at the first operation it cannot draw faithfully; with one,
every skipped operation is reported, so a partial picture cannot be mistaken
for a complete one. Flood fill, region inversion and other omissions are never
skipped silently.

## What the backend receives

Backends receive geometry already in destination coordinates:

- **Paths** (`Path`) of moves, lines, cubic Béziers and closures, with a fill
  rule. Arcs and ellipses arrive as Béziers of at most 90 degrees.
- **Paints** (`Paint`): the effective brush after ROP2 and background mode —
  solid, GDI hatch (`PaintHatch`), pattern or EMF+ hatch/texture
  (`PaintPattern`), or EMF+ linear gradient (`PaintLinearGradient`). Colors are
  straight-alpha sRGB.
- **Strokes** (`Stroke`): the effective pen with caps, joins, dashes and a
  pen-space transform; `Stroke.PixelCenter` lets a backend emulate GDI's
  pixel-center line placement.
- **Images** (`ImageDraw`) with an affine placement, source rectangle, opacity
  and smoothing hint.
- **Clips** (`Clip`): an immutable chain of path areas and region trees
  combined with intersect, union, xor, difference, complement, replace and
  offset steps. Nodes are shared between calls, so a backend can cache masks
  by pointer.

Region painting (FillRgn, PaintRgn, FrameRgn and the WMF region records),
monochrome pattern brushes colored by the text and background colors, logical
palettes with PALETTEINDEX colors and DIB_PAL_COLORS bitmaps, and WMF region
clipping are resolved into ordinary fills and clips.

## Optional interfaces

- **`GradientBackend`** receives gradient fills as Gouraud-shaded triangle
  meshes: EMR_GRADIENTFILL, and EMF+ path gradients with the filled shape as a
  final clip layer. Other backends get them reported.
- **`RasterBackend`** receives bitmap and pattern transfers whose raster
  operations combine source, brush and destination bitwise, such as the
  SRCAND and SRCPAINT pair that draws a sprite through a mask. Other backends
  get them reported, except an exact sprite pair (a black and white mask, and
  an image that is black wherever the mask is white), which becomes one masked
  image.
- **`TextBackend`** (`MeasureText` and `DrawText`) receives text, described
  next.

## Text

`Play` does the GDI placement itself: text alignment, explicit and default
spacing with character extra and justification, escapement, GM_COMPATIBLE
versus GM_ADVANCED transforms, background cells, opaque and clip rectangles,
current-position updates, and bidirectional ordering with the Unicode
Bidirectional Algorithm, including right-to-left reading order. It asks the
backend only for each run's ascent, descent and advances, and hands it
positioned code units or glyph indexes with a text-space transform; font
realization, shaping, underline and strikeout are the backend's.

ANSI strings are decoded with the Windows code pages that MS-UCODEREF names.
Text in the system-dependent DEFAULT_CHARSET is reported unless
`PlayOptions.DefaultCharSet` states what the producing system used, and OEM,
Mac and JOHAB character sets are reported. EMF+ driver strings go to the same
`TextBackend`; EMF+ DrawString is reported.

## EMF+ files

EMF+ files play their EMF+ records, together with the GDI records inside GetDC
intervals; `PlayOptions.Stream.PreferGDI` plays the GDI fallback of a Dual file
instead. EMF+ playback keeps the GDI+ graphics state: Save/Restore,
containers, world and page transforms, pixel offset, compositing and
interpolation modes, and clipping with all combine modes and region trees. It
resolves solid, hatch (with GDI+'s patterns), texture and linear-gradient
brushes, pens with caps, joins, dashes and dash offsets, shapes, paths,
cardinal splines and bitmap images. Path gradient fills, symmetric compound
pens and separate start and end caps are drawn where MS-EMFPLUS defines them.
Custom and arrow caps are drawn only with `PlayOptions.CustomLineCaps`, under
an [interpretation](../interpretations/emfplus.md#line-caps-and-custom-caps) of
geometry MS-EMFPLUS leaves open; it matches Windows GDI+ for the caps GDI+
itself records. Metafile images are played into their destination under
nesting and record budgets.

## GDI state Play keeps

For GDI records `Play` keeps the playback device context: SaveDC/RestoreDC,
selected pens and brushes with stock objects and object-slot reuse, all eight
mapping modes, window/viewport origins and extents, world transforms,
background/fill/ROP2/stretch modes, arc direction, brush origin, current
position, EMF path brackets, clipping and metaregions.

## Color-managed bitmaps

Set `PlayOptions.ColorTransform` (for example to `gowemf.NewColorTransform`)
to convert color-managed bitmaps to sRGB during playback; without it they are
reported. See [images and color](images.md).

## Errors and partial output

A backend error stops playback and `Play` returns it unchanged. See
[errors](../reference/errors.md) for transactional playback and error types,
and [limits](../reference/limits.md) for the playback budgets.
