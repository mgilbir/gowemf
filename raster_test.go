package gowemf

import (
	"fmt"
	"image"

	"github.com/mgilbir/gowemf/internal/raster"
)

// rasterBackend adapts Play's backend calls to the reference rasterizer in
// internal/raster, which is written independently of the playback code.
// Hatches and dashes are drawn differently by every renderer and are not
// implemented; the backend reports them instead of approximating.
type rasterBackend struct {
	canvas *image.NRGBA
	c      *raster.Canvas
	nodes  map[*ClipRegion]*raster.ClipNode
}

func newRasterBackend(w, h int) *rasterBackend {
	c := raster.New(w, h)
	return &rasterBackend{canvas: c.Image, c: c, nodes: make(map[*ClipRegion]*raster.ClipNode)}
}

func toRasterPath(p Path) raster.Path {
	out := raster.Path{Verbs: make([]raster.Verb, len(p.Verbs)), Points: make([]raster.Point, len(p.Points))}
	for i, v := range p.Verbs {
		out.Verbs[i] = raster.Verb(v)
	}
	for i, q := range p.Points {
		out.Points[i] = raster.Point(q)
	}
	return out
}

func toRasterMatrix(m Matrix) raster.Matrix { return raster.Matrix(m) }

// clip converts a clip chain, preserving node identity so masks are cached.
func (rb *rasterBackend) clip(c Clip) []*raster.ClipNode {
	var convert func(*ClipRegion) *raster.ClipNode
	convert = func(r *ClipRegion) *raster.ClipNode {
		if r == nil {
			return nil
		}
		if n, ok := rb.nodes[r]; ok {
			return n
		}
		n := &raster.ClipNode{Base: convert(r.Base), Op: raster.ClipOp(r.Op), Area: toRasterPath(r.Area), Rule: raster.Rule(r.Rule), Operand: convert(r.Operand), Offset: raster.Point(r.Offset)}
		rb.nodes[r] = n
		return n
	}
	out := make([]*raster.ClipNode, len(c))
	for i, r := range c {
		out[i] = convert(r)
	}
	return out
}

func toRasterPaint(p Paint) (raster.Paint, error) {
	switch p.Kind {
	case PaintSolid:
		return raster.Paint{Color: p.Color}, nil
	case PaintPattern:
		return raster.Paint{Pattern: p.Pattern, PatternTransform: toRasterMatrix(p.PatternTransform), Wrap: raster.Wrap(p.Wrap)}, nil
	case PaintLinearGradient:
		return raster.Paint{Gradient: toRasterGradient(p.Gradient)}, nil
	}
	return raster.Paint{}, fmt.Errorf("test rasterizer does not implement paint kind %d", p.Kind)
}

func toRasterGradient(g *LinearGradient) *raster.Gradient {
	out := &raster.Gradient{Transform: toRasterMatrix(g.Transform), Wrap: raster.Wrap(g.Wrap)}
	for _, s := range g.Stops {
		out.Stops = append(out.Stops, raster.Stop{Offset: s.Offset, Color: s.Color})
	}
	return out
}

func (rb *rasterBackend) FillPath(path Path, rule FillRule, paint Paint, clip Clip) error {
	p, err := toRasterPaint(paint)
	if err != nil {
		return err
	}
	return rb.c.Fill(toRasterPath(path), raster.Rule(rule), p, rb.clip(clip))
}

func (rb *rasterBackend) StrokePath(path Path, s Stroke, clip Clip) error {
	if s.Dash != DashSolid {
		return fmt.Errorf("test rasterizer does not implement dash style %d", s.Dash)
	}
	p, err := toRasterPaint(s.Paint)
	if err != nil {
		return err
	}
	pen := raster.Stroke{Width: s.Width, Transform: toRasterMatrix(s.Transform), Hairline: s.Hairline, Cap: raster.Cap(s.Cap), EndCap: raster.Cap(s.EndCap), Compound: s.Compound, Join: raster.Join(s.Join), MiterLimit: s.MiterLimit, PixelCenter: raster.Point(s.PixelCenter)}
	return rb.c.Stroke(toRasterPath(path), pen, p, rb.clip(clip))
}

func (rb *rasterBackend) DrawImage(d ImageDraw, clip Clip) error {
	return rb.c.DrawImage(d.Image, d.Source, toRasterMatrix(d.Transform), d.Opacity, rb.clip(clip))
}

func compareNeighborhood(a, b image.Image, delta uint32, radius int, maxBadFraction float64) error {
	return raster.CompareNeighborhood(a, b, delta, radius, maxBadFraction)
}

func neighborhoodMismatch(a, b image.Image, delta uint32, radius int) (int, int, error) {
	return raster.NeighborhoodMismatch(a, b, delta, radius)
}

func (rb *rasterBackend) FillGradient(mesh []GradientTriangle, clip Clip) error {
	out := make([]raster.Triangle, len(mesh))
	for i, t := range mesh {
		for k := 0; k < 3; k++ {
			out[i].P[k] = raster.Point(t.Points[k])
			out[i].C[k] = t.Colors[k]
		}
	}
	return rb.c.FillGouraud(out, rb.clip(clip))
}

func (rb *rasterBackend) DrawRaster(d RasterDraw, clip Clip) error {
	var src *raster.RasterSource
	if d.Source != nil {
		src = &raster.RasterSource{Image: d.Source.Image, Src: d.Source.Source, M: toRasterMatrix(d.Source.Transform)}
	}
	var pattern *raster.Paint
	if d.Pattern != nil {
		p, err := toRasterPaint(*d.Pattern)
		if err != nil {
			return err
		}
		pattern = &p
	}
	return rb.c.Raster(uint8(d.Operation), toRasterPath(d.Area), src, pattern, rb.clip(clip))
}
