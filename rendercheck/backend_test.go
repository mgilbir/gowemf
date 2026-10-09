package rendercheck

import (
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"unicode/utf16"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/gowemf"
	"github.com/mgilbir/gowemf/internal/raster"
)

// backend draws Play output with the reference rasterizer and sets text with
// forme. Text metrics follow GDI's TrueType conventions: a negative LOGFONT
// height is the em size, a positive one the cell height usWinAscent +
// usWinDescent; ascent and descent are usWinAscent and usWinDescent; advances
// are the hmtx widths, unhinted. Only the regular Liberation Sans face is
// loaded, and requests it cannot honor are errors rather than substitutions.
type backend struct {
	canvas *raster.Canvas
	face   *shape.Face
	nodes  map[*gowemf.ClipRegion]*raster.ClipNode
}

func newBackend(w, h int, face *shape.Face) *backend {
	return &backend{canvas: raster.New(w, h), face: face, nodes: make(map[*gowemf.ClipRegion]*raster.ClipNode)}
}

func loadFace(path string) (*shape.Face, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return shape.Load(data)
}

func rpath(p gowemf.Path) raster.Path {
	out := raster.Path{Verbs: make([]raster.Verb, len(p.Verbs)), Points: make([]raster.Point, len(p.Points))}
	for i, v := range p.Verbs {
		out.Verbs[i] = raster.Verb(v)
	}
	for i, q := range p.Points {
		out.Points[i] = raster.Point(q)
	}
	return out
}

func (b *backend) clip(c gowemf.Clip) []*raster.ClipNode {
	var convert func(*gowemf.ClipRegion) *raster.ClipNode
	convert = func(r *gowemf.ClipRegion) *raster.ClipNode {
		if r == nil {
			return nil
		}
		if n, ok := b.nodes[r]; ok {
			return n
		}
		n := &raster.ClipNode{Base: convert(r.Base), Op: raster.ClipOp(r.Op), Area: rpath(r.Area), Rule: raster.Rule(r.Rule), Operand: convert(r.Operand), Offset: raster.Point(r.Offset)}
		b.nodes[r] = n
		return n
	}
	out := make([]*raster.ClipNode, len(c))
	for i, r := range c {
		out[i] = convert(r)
	}
	return out
}

func rpaint(p gowemf.Paint) (raster.Paint, error) {
	switch p.Kind {
	case gowemf.PaintSolid:
		return raster.Paint{Color: p.Color}, nil
	case gowemf.PaintPattern:
		return raster.Paint{Pattern: p.Pattern, PatternTransform: raster.Matrix(p.PatternTransform), Wrap: raster.Wrap(p.Wrap)}, nil
	case gowemf.PaintLinearGradient:
		g := &raster.Gradient{Transform: raster.Matrix(p.Gradient.Transform), Wrap: raster.Wrap(p.Gradient.Wrap)}
		for _, s := range p.Gradient.Stops {
			g.Stops = append(g.Stops, raster.Stop{Offset: s.Offset, Color: s.Color})
		}
		return raster.Paint{Gradient: g}, nil
	}
	return raster.Paint{}, fmt.Errorf("reference backend does not implement paint kind %d", p.Kind)
}

func (b *backend) FillPath(path gowemf.Path, rule gowemf.FillRule, paint gowemf.Paint, clip gowemf.Clip) error {
	p, err := rpaint(paint)
	if err != nil {
		return err
	}
	return b.canvas.Fill(rpath(path), raster.Rule(rule), p, b.clip(clip))
}

func (b *backend) StrokePath(path gowemf.Path, s gowemf.Stroke, clip gowemf.Clip) error {
	if s.Dash != gowemf.DashSolid {
		return fmt.Errorf("reference backend does not implement dash style %d", s.Dash)
	}
	p, err := rpaint(s.Paint)
	if err != nil {
		return err
	}
	pen := raster.Stroke{Width: s.Width, Transform: raster.Matrix(s.Transform), Hairline: s.Hairline, Cap: raster.Cap(s.Cap), Join: raster.Join(s.Join), MiterLimit: s.MiterLimit, PixelCenter: raster.Point(s.PixelCenter)}
	return b.canvas.Stroke(rpath(path), pen, p, b.clip(clip))
}

func (b *backend) DrawImage(d gowemf.ImageDraw, clip gowemf.Clip) error {
	return b.canvas.DrawImage(d.Image, d.Source, raster.Matrix(d.Transform), d.Opacity, b.clip(clip))
}

// em returns the em size of a request in text-space units.
func (b *backend) em(f gowemf.FontRequest) (float64, error) {
	if f.FaceName != "Liberation Sans" || f.Weight > 550 || f.Italic || f.Width != 0 || f.Stock != 0 {
		return 0, fmt.Errorf("reference backend cannot realize font %+v", f)
	}
	d := b.face.Descriptor()
	switch {
	case f.Height < 0:
		return -f.Height, nil
	case f.Height > 0:
		return f.Height * float64(b.face.UnitsPerEm()) / float64(d.WinAscent+d.WinDescent), nil
	}
	return 0, errors.New("reference backend has no default font size")
}

// runes decodes a run's code units; the second unit of a pair maps to -1.
func runes(text []uint16) []rune {
	out := make([]rune, len(text))
	for i := 0; i < len(text); i++ {
		if utf16.IsSurrogate(rune(text[i])) && i+1 < len(text) {
			out[i] = utf16.DecodeRune(rune(text[i]), rune(text[i+1]))
			out[i+1] = -1
			i++
			continue
		}
		out[i] = rune(text[i])
	}
	return out
}

func (b *backend) glyphs(run gowemf.TextRun) ([]int, error) {
	gids := make([]int, len(run.Text))
	for i, r := range runes(run.Text) {
		switch {
		case run.Glyphs:
			gids[i] = int(run.Text[i])
		case r < 0:
			gids[i] = -1
		default:
			gid, ok := b.face.GlyphID(r)
			if !ok {
				return nil, fmt.Errorf("reference font has no glyph for %U", r)
			}
			gids[i] = gid
		}
	}
	return gids, nil
}

func (b *backend) MeasureText(run gowemf.TextRun) (gowemf.TextMetrics, error) {
	em, err := b.em(run.Font)
	if err != nil {
		return gowemf.TextMetrics{}, err
	}
	gids, err := b.glyphs(run)
	if err != nil {
		return gowemf.TextMetrics{}, err
	}
	d, upm := b.face.Descriptor(), float64(b.face.UnitsPerEm())
	m := gowemf.TextMetrics{Ascent: float64(d.WinAscent) * em / upm, Descent: float64(d.WinDescent) * em / upm, Advances: make([]float64, len(gids))}
	for i, g := range gids {
		if g >= 0 {
			m.Advances[i] = b.face.GlyphAdvance(g) / 1000 * em
		}
	}
	return m, nil
}

func (b *backend) DrawText(run gowemf.TextRun, clip gowemf.Clip) error {
	em, err := b.em(run.Font)
	if err != nil {
		return err
	}
	gids, err := b.glyphs(run)
	if err != nil {
		return err
	}
	paint, err := rpaint(run.Paint)
	if err != nil {
		return err
	}
	upm := float64(b.face.UnitsPerEm())
	s, c := math.Sincos(run.Font.Orientation)
	// Font units (y up) to text space (y down), rotated counterclockwise as
	// displayed by the orientation relative to the baseline.
	glyph := gowemf.Matrix{M11: em / upm * c, M12: -em / upm * s, M21: em / upm * s, M22: -em / upm * c}
	var path raster.Path
	add := func(m gowemf.Matrix, p shape.Point) raster.Point {
		return raster.Point(m.Apply(gowemf.Point{X: p.X, Y: p.Y}))
	}
	for i, g := range gids {
		if g < 0 {
			continue
		}
		m := glyph
		m.Dx, m.Dy = run.Origins[i].X, run.Origins[i].Y
		m = m.Then(run.Transform)
		var cur shape.Point
		err := b.face.GlyphOutline(g, func(seg shape.Segment) bool {
			switch seg.Op {
			case shape.MoveTo:
				if len(path.Verbs) > 0 {
					path.Verbs = append(path.Verbs, raster.Close)
				}
				path.Verbs = append(path.Verbs, raster.MoveTo)
				path.Points = append(path.Points, add(m, seg.Pts[0]))
				cur = seg.Pts[0]
			case shape.LineTo:
				path.Verbs = append(path.Verbs, raster.LineTo)
				path.Points = append(path.Points, add(m, seg.Pts[0]))
				cur = seg.Pts[0]
			case shape.QuadTo:
				q, e := seg.Pts[0], seg.Pts[1]
				c1 := shape.Point{X: cur.X + 2.0/3*(q.X-cur.X), Y: cur.Y + 2.0/3*(q.Y-cur.Y)}
				c2 := shape.Point{X: e.X + 2.0/3*(q.X-e.X), Y: e.Y + 2.0/3*(q.Y-e.Y)}
				path.Verbs = append(path.Verbs, raster.CubicTo)
				path.Points = append(path.Points, add(m, c1), add(m, c2), add(m, e))
				cur = e
			case shape.CubicTo:
				path.Verbs = append(path.Verbs, raster.CubicTo)
				path.Points = append(path.Points, add(m, seg.Pts[0]), add(m, seg.Pts[1]), add(m, seg.Pts[2]))
				cur = seg.Pts[2]
			}
			return true
		})
		if err != nil {
			return err
		}
	}
	if len(path.Verbs) > 0 {
		path.Verbs = append(path.Verbs, raster.Close)
		if err := b.canvas.Fill(path, raster.NonZero, paint, b.clip(clip)); err != nil {
			return err
		}
	}
	// Underline and strikeout span the run from its first origin to the end
	// of its last advance; they are filled separately from the glyphs so
	// contour winding cannot cancel them.
	path = raster.Path{}
	if (run.Font.Underline || run.Font.StrikeOut) && len(gids) > 0 {
		x0, x1 := run.Origins[0].X, run.Origins[0].X
		for _, a := range run.Advances {
			x1 += a
		}
		y := run.Origins[0].Y
		d := b.face.Descriptor()
		bar := func(top, size int) {
			t, h := y-float64(top)*em/upm, float64(size)*em/upm
			for _, p := range []gowemf.Point{{X: x0, Y: t}, {X: x1, Y: t}, {X: x1, Y: t + h}, {X: x0, Y: t + h}} {
				verb := raster.LineTo
				if p == (gowemf.Point{X: x0, Y: t}) {
					verb = raster.MoveTo
				}
				path.Verbs = append(path.Verbs, verb)
				path.Points = append(path.Points, raster.Point(run.Transform.Apply(p)))
			}
			path.Verbs = append(path.Verbs, raster.Close)
		}
		if run.Font.Underline {
			bar(d.UnderlinePosition, d.UnderlineThickness)
		}
		if run.Font.StrikeOut {
			bar(d.StrikeoutPosition, d.StrikeoutSize)
		}
	}
	if len(path.Verbs) == 0 {
		return nil
	}
	return b.canvas.Fill(path, raster.NonZero, paint, b.clip(clip))
}

func (b *backend) image() *image.NRGBA { return b.canvas.Image }
