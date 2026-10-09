package gowemf

import (
	"errors"
	"image"
	"image/color"
	"math"
)

// PathVerb identifies one path element. PathMoveTo and PathLineTo consume one
// point, PathCubicTo consumes two control points and an end point, and
// PathClose consumes none.
type PathVerb uint8

const (
	PathMoveTo PathVerb = iota + 1
	PathLineTo
	PathCubicTo
	PathClose
)

// Path is geometry in destination coordinates. Every figure starts with
// PathMoveTo. Curves remain cubic Béziers; arcs and ellipses are converted to
// cubic segments of at most 90 degrees. Backends must not modify a Path.
type Path struct {
	Verbs  []PathVerb
	Points []Point
}

// FillRule selects GDI ALTERNATE (EvenOdd) or WINDING (NonZero) filling.
type FillRule uint8

const (
	EvenOdd FillRule = iota + 1
	NonZero
)

// PaintKind distinguishes resolved brush paints.
type PaintKind uint8

const (
	PaintSolid PaintKind = iota + 1
	// PaintHatch draws Color in an 8x8 device-pixel hatch selected by Hatch
	// (HS_HORIZONTAL=0 through HS_DIAGCROSS=5). Background is the background
	// color, or nil for TRANSPARENT background mode. GDI colors are opaque;
	// EMF+ hatch colors may be translucent.
	PaintHatch
	// PaintPattern repeats Pattern as Wrap describes. GDI patterns are opaque
	// and tiled; EMF+ texture brushes may carry alpha and other wrap modes.
	PaintPattern
	// PaintLinearGradient is an EMF+ linear gradient brush, in Gradient.
	PaintLinearGradient
)

// Paint is an effective brush or pen color after ROP2 and background mode
// resolution. Colors are straight-alpha sRGB values; GDI colors are opaque.
// For hatches and patterns, PatternTransform maps pattern space to
// destination coordinates. GDI hatch and pattern space is in device pixels
// anchored at the brush origin; EMF+ hatches are anchored at the rendering
// origin, and EMF+ texture space is the image's pixel grid placed by the brush
// and world transforms.
type Paint struct {
	Kind             PaintKind
	Color            color.NRGBA
	Hatch            uint32
	Background       *color.NRGBA
	Pattern          image.Image
	PatternTransform Matrix
	Wrap             WrapMode
	Gradient         *LinearGradient
}

// WrapMode is how a pattern or gradient continues outside its tile
// (MS-EMFPLUS 2.1.1.34). WrapClamp paints nothing outside a pattern tile.
type WrapMode uint8

const (
	WrapTile WrapMode = iota
	WrapTileFlipX
	WrapTileFlipY
	WrapTileFlipXY
	WrapClamp
)

// GradientStop is a color at an offset in [0,1] along a gradient.
type GradientStop struct {
	Offset float64
	Color  color.NRGBA
}

// LinearGradient is an EMF+ linear gradient. Transform maps destination
// coordinates to gradient space, where the x coordinate is the gradient
// parameter: 0 at the start color and 1 at the end. Stops have ascending
// offsets from 0 to 1 and interpolate linearly in sRGB between neighbors; all
// stops share one alpha. Outside [0,1] the parameter repeats (WrapTile or
// WrapTileFlipY) or mirrors (WrapTileFlipX or WrapTileFlipXY).
type LinearGradient struct {
	Transform Matrix
	Stops     []GradientStop
	Wrap      WrapMode
}

type LineCap uint8
type LineJoin uint8
type DashStyle uint8

const (
	CapRound LineCap = iota + 1
	CapSquare
	CapFlat
)
const (
	JoinRound LineJoin = iota + 1
	JoinBevel
	JoinMiter
)

// Dash styles are GDI pen styles. Predefined patterns are device dependent and
// are left to the backend. DashUser lengths are in pen space for geometric
// pens and in device pixels (2*PixelCenter destination units) for hairlines.
// Stroke.DashOffset, in the same units, is how far into the pattern each
// figure starts.
const (
	DashSolid DashStyle = iota + 1
	DashDash
	DashDot
	DashDashDot
	DashDashDotDot
	DashAlternate
	DashUser
)

// Stroke is an effective pen. For a geometric pen, Width is measured in pen
// space and Transform is the linear map from pen space to destination
// coordinates: the outline is the pen-space stroke mapped through Transform.
// Hairline pens are one destination unit wide and ignore Width/Transform.
// Gap is the paint for gaps between dashes in OPAQUE background mode.
// PixelCenter is half a device pixel in destination units: GDI rasterizes
// lines through device pixel centers, so a backend emulating GDI pixels
// translates stroke geometry by it. Fill geometry needs no such offset. EMF+
// strokes have a zero PixelCenter: their geometry already follows the EMF+
// pixel offset mode.
//
// Cap applies to both ends of open figures unless EndCap is set, in which
// case Cap is the start cap and EndCap the end cap (EMF+ pens only). A
// non-empty Compound divides the pen across its width into parallel bands
// (MS-EMFPLUS 2.2.2.9): pairs of fractions [a,b] of the width. Play passes
// only arrays symmetric about the center, with miter joins, so each band is
// exact as a difference of ordinary strokes: a band with b <= 1/2 covers the
// stroke of width (1-2a)*Width minus the stroke of width (1-2b)*Width, and a
// band containing the center covers the stroke of width (1-2a)*Width; open
// figures then have flat caps and no dashes.
type Stroke struct {
	Paint       Paint
	Hairline    bool
	Width       float64
	Transform   Matrix
	Cap         LineCap
	EndCap      LineCap
	Compound    []float64
	Join        LineJoin
	MiterLimit  float64
	Dash        DashStyle
	Dashes      []float64
	DashOffset  float64
	Gap         *Paint
	PixelCenter Point
}

// ImageDraw places Source pixels from Image. Transform maps image pixel
// coordinates (pixel x covers [x,x+1)) to destination coordinates and may
// mirror, scale, rotate or shear. Pixels outside Source are not drawn. The
// image is composited source-over with its alpha multiplied by Opacity.
// Smooth is the interpolation hint: GDI's HALFTONE stretch mode, or an EMF+
// interpolation mode other than NearestNeighbor. Image must not be modified.
type ImageDraw struct {
	Image     image.Image
	Source    image.Rectangle
	Transform Matrix
	Opacity   float64
	Smooth    bool
}

// ClipOp combines a region with its Base.
type ClipOp uint8

const (
	ClipIntersect ClipOp = iota + 1
	ClipUnion
	ClipXor
	ClipDifference
	// ClipReplace ignores Base and selects Area alone.
	ClipReplace
	// ClipOffset translates Base by Offset; Area is unused.
	ClipOffset
	// ClipComplement keeps the operand minus Base (EMF+ CombineModeComplement).
	ClipComplement
)

// ClipRegion is an immutable clipping step in destination coordinates: the
// region of Base combined by Op with its operand. The operand is Area under
// Rule, or the region Operand when Operand is non-nil (EMF+ region trees). A
// nil Base denotes the whole drawing surface, which is GDI's default clip
// region. Nodes are shared by saved states and later steps; backends may cache
// derived masks by pointer identity.
type ClipRegion struct {
	Base    *ClipRegion
	Op      ClipOp
	Area    Path
	Rule    FillRule
	Operand *ClipRegion
	Offset  Point
	depth   uint32
}

// Clip lists regions that all constrain drawing: metaregions followed by the
// current clipping region. An empty Clip leaves drawing unclipped.
type Clip []*ClipRegion

// Backend receives drawing operations in destination coordinates. Returning an
// error stops playback and Play returns it unchanged.
type Backend interface {
	FillPath(path Path, rule FillRule, paint Paint, clip Clip) error
	StrokePath(path Path, stroke Stroke, clip Clip) error
	DrawImage(image ImageDraw, clip Clip) error
}

// UnsupportedOperation identifies content that Play could not draw.
type UnsupportedOperation struct {
	Source Record
	Reason string
}

// PlayOptions configures GDI playback. Destination is the rectangle receiving
// the picture: the EMF header frame or the WMF placeable bounds are mapped
// onto it. WMF input without a placeable header requires Placeable. Zero
// limits select defaults: 4,000,000 points per path, 64,000,000 cumulative
// decoded bitmap pixels and 4,096 clip steps per region chain.
//
// DefaultCharSet is the CharacterSet that DEFAULT_CHARSET fonts resolve to on
// the producing system; nil reports such ANSI text unsupported rather than
// guessing a code page. Text is drawn only by backends implementing
// TextBackend.
//
// CustomLineCaps draws EMF+ custom and adjustable-arrow line caps under an
// interpretation that has not been verified against Windows, because neither
// MS-EMFPLUS nor Microsoft's GDI+ reference defines their geometry (see
// COVERAGE.md). When false, such caps are reported unsupported.
//
// Unsupported is called for each operation that cannot be drawn faithfully.
// If it is nil, Play stops with an ErrUnsupported error. If it returns nil,
// the operation is skipped; the caller then knows the output is incomplete.
type PlayOptions struct {
	Stream         StreamOptions
	Images         ImageLimits
	Destination    Box
	Placeable      *PlaceableHeader
	ColorTransform ColorTransformFactory
	DefaultCharSet *uint8
	MaxPathPoints  uint64
	MaxImagePixels uint64
	MaxClipSteps   uint32
	CustomLineCaps bool
	Unsupported    func(UnsupportedOperation) error
}

// errSkip marks an operation skipped with the Unsupported callback's consent.
var errSkip = errors.New("gowemf: skipped unsupported operation")

// Play replays a WMF, EMF or EMF+ file through backend. For GDI records it
// tracks the playback device context (save/restore, mapping modes,
// window/viewport and world transforms, selected objects, paths and
// clipping); for EMF+ records, the GDI+ graphics state (world and page
// transforms, containers, clipping and objects). It resolves drawing records
// into destination-space operations. EMF+ files play their EMF+ records, plus
// the GDI records inside GetDC intervals; Stream.PreferGDI selects the GDI
// fallback of EMF+ Dual files instead. Flood fill, EMF+ text and other
// unimplemented operations are reported through PlayOptions.Unsupported; see
// COVERAGE.md for the inventory.
//
// Like Stream, backend calls can precede a later failure. Validate first with
// Stream(data, options.Stream, nil) when output must be transactional.
func Play(data []byte, options PlayOptions, backend Backend) (Header, error) {
	if backend == nil {
		return Header{}, errors.New("gowemf: nil playback backend")
	}
	d := options.Destination
	if !finite(d.X) || !finite(d.Y) || !finite(d.Width) || !finite(d.Height) || d.Width <= 0 || d.Height <= 0 {
		return Header{}, errors.New("gowemf: playback destination must be finite and non-empty")
	}
	if options.MaxPathPoints == 0 {
		options.MaxPathPoints = 4_000_000
	}
	if options.MaxImagePixels == 0 {
		options.MaxImagePixels = 64_000_000
	}
	if options.MaxClipSteps == 0 {
		options.MaxClipSteps = 4096
	}
	h, err := Walk(data, options.Stream.Framing, nil)
	if err != nil {
		return Header{}, err
	}
	p := &player{options: options, backend: backend}
	if err := p.start(h); err != nil {
		return Header{}, err
	}
	return Stream(data, options.Stream, p.play)
}

// player holds playback state for one Play call.
type player struct {
	options PlayOptions
	backend Backend
	format  Format
	dc      deviceContext
	saved   []deviceContext
	objects []playObject
	base    Matrix // device to destination
	// Physical device pixel size in millimeters, for fixed mapping modes and
	// MM_ISOTROPIC adjustment.
	pixelMM      Point
	path         *pathBuilder
	constructing bool // inside BeginPath/EndPath
	pixels       uint64
	regionWork   uint64
	generations  []uint64
	// EMF+ playback state; plusDPI is the header's logical resolution.
	plus        plusState
	plusSaved   []plusSaved
	plusObjects [64]plusObject
	plusDPI     Point
}

func (p *player) start(h Header) error {
	p.format = h.Format
	d := p.options.Destination
	p.dc = defaultDeviceContext()
	if h.WMF != nil {
		place := h.Placeable
		if place == nil {
			place = p.options.Placeable
		}
		if place == nil {
			return failure(0, "WMF playback without placeable bounds", ErrUnsupported)
		}
		b := place.Bounds
		w, ht := float64(b.Right)-float64(b.Left), float64(b.Bottom)-float64(b.Top)
		if w == 0 || ht == 0 || place.UnitsPerInch == 0 {
			return malformed(0, "empty WMF placeable bounds")
		}
		// MS-WMF 3.1.3: the metafile owns the window and the player owns the
		// viewport. Device units are destination units.
		p.base = Identity()
		p.dc.mapMode = 8
		p.dc.windowOrg = Point{float64(b.Left), float64(b.Top)}
		p.dc.windowExt = Point{w, ht}
		p.dc.viewportOrg = Point{d.X, d.Y}
		p.dc.viewportExt = Point{d.Width, d.Height}
		inch := float64(place.UnitsPerInch)
		p.pixelMM = Point{25.4 * math.Abs(w) / inch / d.Width, 25.4 * math.Abs(ht) / inch / d.Height}
		p.objects = make([]playObject, int(h.WMF.Objects))
	} else {
		e := h.EMF
		if e.Device.X <= 0 || e.Device.Y <= 0 || e.Millimeters.X <= 0 || e.Millimeters.Y <= 0 {
			return malformed(0, "EMF reference device size")
		}
		p.pixelMM = Point{float64(e.Millimeters.X) / float64(e.Device.X), float64(e.Millimeters.Y) / float64(e.Device.Y)}
		if e.Micrometers != nil && e.Micrometers.X > 0 && e.Micrometers.Y > 0 {
			p.pixelMM = Point{float64(e.Micrometers.X) / 1000 / float64(e.Device.X), float64(e.Micrometers.Y) / 1000 / float64(e.Device.Y)}
		}
		// The picture frame is in .01 mm; convert it to reference-device pixels.
		f := e.Frame
		left, top := float64(f.Left)/100/p.pixelMM.X, float64(f.Top)/100/p.pixelMM.Y
		w, ht := (float64(f.Right)-float64(f.Left))/100/p.pixelMM.X, (float64(f.Bottom)-float64(f.Top))/100/p.pixelMM.Y
		if w <= 0 || ht <= 0 {
			// Fall back to the inclusive device-unit bounds.
			b := e.Bounds
			left, top = float64(b.Left), float64(b.Top)
			w, ht = float64(b.Right)-float64(b.Left)+1, float64(b.Bottom)-float64(b.Top)+1
			if w <= 0 || ht <= 0 {
				return malformed(8, "empty EMF picture frame")
			}
		}
		sx, sy := d.Width/w, d.Height/ht
		p.base = Matrix{M11: sx, M22: sy, Dx: d.X - left*sx, Dy: d.Y - top*sy}
		p.objects = make([]playObject, int(e.Handles)+1)
	}
	if !p.base.Finite() || !finite(p.pixelMM.X) || !finite(p.pixelMM.Y) || p.pixelMM.X <= 0 || p.pixelMM.Y <= 0 {
		return malformed(0, "playback frame scale")
	}
	p.generations = make([]uint64, len(p.objects))
	p.plus = defaultPlusState()
	if h.EMFPlus != nil {
		p.plusDPI = Point{float64(h.EMFPlus.LogicalDpiX), float64(h.EMFPlus.LogicalDpiY)}
	}
	return nil
}

// unsupported reports an operation that cannot be drawn. It returns errSkip
// when the caller's callback accepted the omission.
func (p *player) unsupported(r Record, reason string) error {
	if p.options.Unsupported == nil {
		return failure(r.Offset, reason, ErrUnsupported)
	}
	if err := p.options.Unsupported(UnsupportedOperation{Source: r, Reason: reason}); err != nil {
		return err
	}
	return errSkip
}

func (p *player) play(c Command) error {
	err := p.dispatch(c)
	if err == errSkip {
		return nil
	}
	return err
}
