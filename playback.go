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
	// (HS_HORIZONTAL=0 through HS_DIAGCROSS=5). Background is the opaque
	// background color, or nil for TRANSPARENT background mode.
	PaintHatch
	// PaintPattern tiles Pattern, which is opaque.
	PaintPattern
)

// Paint is an effective brush or pen color after ROP2 and background mode
// resolution. Colors are straight-alpha sRGB values; GDI colors are opaque.
// For hatches and patterns, PatternTransform maps device-pixel pattern space,
// anchored at the brush origin, to destination coordinates.
type Paint struct {
	Kind             PaintKind
	Color            color.NRGBA
	Hatch            uint32
	Background       *color.NRGBA
	Pattern          image.Image
	PatternTransform Matrix
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
// translates stroke geometry by it. Fill geometry needs no such offset.
type Stroke struct {
	Paint       Paint
	Hairline    bool
	Width       float64
	Transform   Matrix
	Cap         LineCap
	Join        LineJoin
	MiterLimit  float64
	Dash        DashStyle
	Dashes      []float64
	Gap         *Paint
	PixelCenter Point
}

// ImageDraw places Source pixels from Image. Transform maps image pixel
// coordinates (pixel x covers [x,x+1)) to destination coordinates and may
// mirror, scale, rotate or shear. Pixels outside Source are not drawn. The
// image is composited source-over with its alpha multiplied by Opacity.
// Smooth is the HALFTONE interpolation hint. Image must not be modified.
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
)

// ClipRegion is an immutable clipping step in destination coordinates. A nil
// Base denotes the whole drawing surface, which is GDI's default clip region.
// Nodes are shared by saved states and later steps; backends may cache derived
// masks by pointer identity.
type ClipRegion struct {
	Base   *ClipRegion
	Op     ClipOp
	Area   Path
	Rule   FillRule
	Offset Point
	depth  uint32
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
// Unsupported is called for each operation that cannot be drawn faithfully.
// If it is nil, Play stops with an ErrUnsupported error. If it returns nil,
// the operation is skipped; the caller then knows the output is incomplete.
type PlayOptions struct {
	Stream         StreamOptions
	Images         ImageLimits
	Destination    Box
	Placeable      *PlaceableHeader
	ColorTransform ColorTransformFactory
	MaxPathPoints  uint64
	MaxImagePixels uint64
	MaxClipSteps   uint32
	Unsupported    func(UnsupportedOperation) error
}

// errSkip marks an operation skipped with the Unsupported callback's consent.
var errSkip = errors.New("gowemf: skipped unsupported operation")

// Play replays the GDI stream of a WMF or EMF file through backend. It tracks
// the playback device context (save/restore, mapping modes, window/viewport
// and world transforms, selected objects, paths and clipping) and resolves
// drawing records into destination-space operations. Text, region painting,
// gradients, flood fill, EMF+ drawing and other unimplemented operations are
// reported through PlayOptions.Unsupported; see COVERAGE.md for the inventory.
// EMF+ Dual files play their GDI fallback only when Stream.PreferGDI is set.
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
	if h.EMFPlus != nil && !options.Stream.PreferGDI {
		return Header{}, failure(0, "EMF+ drawing playback", ErrUnsupported)
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
	generations  []uint64
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
