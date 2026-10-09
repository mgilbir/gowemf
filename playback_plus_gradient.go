package gowemf

import (
	"image/color"
	"math"
)

// plusPathGradientFill fills path with a path gradient brush (MS-EMFPLUS
// 2.2.2.29): the color changes along each line from the center point to the
// boundary, from the center color to the boundary color, which varies
// linearly between boundary points. For a boundary that is star-shaped from
// the center this is exactly a fan of Gouraud triangles. The area outside the
// boundary is not painted, as WrapModeClamp specifies.
//
// Reported instead: point boundaries, which MS-EMFPLUS 2.2.2.7 calls a closed
// cardinal spline without giving its tension; blend factors and preset
// colors, whose positions 2.2.2.4 and 2.2.2.5 describe only as running from
// the "midpoint" to an "endpoint"; focus scales, whose scaling origin 2.2.2.18
// does not give; gamma correction; tiled wraps; colors of differing alpha;
// and boundaries that are not star-shaped from the center.
func (p *player) plusPathGradientFill(r Record, g *PlusPathGradient, fill Path, rule FillRule, m Matrix, clip Clip) error {
	gb, ok := p.backend.(GradientBackend)
	switch {
	case g == nil:
		return malformed(r.Offset, "EMF+ path gradient brush")
	case !ok:
		return p.unsupported(r, "EMF+ path gradient without a GradientBackend")
	case g.Flags&1 == 0 || g.BoundaryPath == nil:
		return p.unsupported(r, "EMF+ path gradient with a point (cardinal spline) boundary")
	case g.Flags&0x0c != 0:
		return p.unsupported(r, "EMF+ path gradient blend factors or preset colors")
	case g.Flags&0x40 != 0:
		return p.unsupported(r, "EMF+ path gradient focus scales")
	case g.Flags&0x80 != 0:
		return p.unsupported(r, "EMF+ gamma-corrected gradient")
	case g.WrapMode != uint32(WrapClamp):
		return p.unsupported(r, "EMF+ tiled path gradient")
	}
	bm := g.Transform.Then(m)
	if !bm.Finite() {
		return malformed(r.Offset, "non-finite EMF+ path gradient transform")
	}
	boundary := *g.BoundaryPath
	curved := false
	for _, t := range boundary.Types {
		curved = curved || t&7 == 3
	}
	n := g.SurroundingColors.Len()
	if n != 1 && (curved || n != boundary.Points.Len()) {
		// Colors belong to path points; on curves the points include
		// control points, whose role MS-EMFPLUS does not give.
		return p.unsupported(r, "EMF+ path gradient surrounding colors not one per boundary vertex")
	}
	center := argb(g.CenterColor)
	colors := make([]color.NRGBA, boundary.Points.Len())
	for i := range colors {
		colors[i] = argb(g.SurroundingColors.At(min(i, n-1)))
		if colors[i].A != center.A {
			return p.unsupported(r, "EMF+ gradient with varying alpha")
		}
	}
	if p.plus.compositing == 1 && center.A != 255 {
		return p.unsupported(r, "EMF+ SourceCopy compositing of translucent paint")
	}
	path, err := p.plusPath(r, boundary, bm, true)
	if err != nil {
		return err
	}
	b := pathBuilder{path: path, limit: p.options.MaxPathPoints}
	if curved {
		b.flatten()
		if b.err {
			return failure(r.Offset, "path points", ErrLimit)
		}
	}
	// One closed figure of points, each with its color.
	var pts []Point
	var cols []color.NRGBA
	for i, v := range b.path.Verbs {
		switch {
		case v == PathMoveTo && i != 0:
			return p.unsupported(r, "EMF+ path gradient boundary with several figures")
		case v == PathMoveTo || v == PathLineTo:
			q := b.path.Points[len(pts)]
			c := colors[0]
			if !curved {
				c = colors[len(pts)]
			}
			pts, cols = append(pts, q), append(cols, c)
		}
	}
	c := bm.Apply(g.Center)
	mesh := make([]GradientTriangle, 0, len(pts))
	var turn float64
	sign := 0.0
	wide := func(c color.NRGBA) color.NRGBA64 {
		return color.NRGBA64{uint16(c.R) * 257, uint16(c.G) * 257, uint16(c.B) * 257, uint16(c.A) * 257}
	}
	for i := range pts {
		a, e := pts[i], pts[(i+1)%len(pts)]
		if a == e {
			continue
		}
		u, v := Point{a.X - c.X, a.Y - c.Y}, Point{e.X - c.X, e.Y - c.Y}
		cross := u.X*v.Y - u.Y*v.X
		if cross == 0 || (sign != 0 && math.Signbit(cross) != math.Signbit(sign)) {
			return p.unsupported(r, "EMF+ path gradient boundary not star-shaped from its center")
		}
		sign = cross
		turn += math.Atan2(cross, u.X*v.X+u.Y*v.Y)
		mesh = append(mesh, GradientTriangle{Points: [3]Point{c, a, e}, Colors: [3]color.NRGBA64{wide(center), wide(cols[i]), wide(cols[(i+1)%len(pts)])}})
	}
	if len(mesh) < 3 || math.Abs(math.Abs(turn)-2*math.Pi) > 1e-6 {
		return p.unsupported(r, "EMF+ path gradient boundary not star-shaped from its center")
	}
	if err := p.spendPixels(r, len(mesh), 1); err != nil {
		return err
	}
	// The filled shape bounds the gradient as one more clip layer.
	layers := append(clip[:len(clip):len(clip)], &ClipRegion{Op: ClipReplace, Area: fill, Rule: rule, depth: 1})
	return gb.FillGradient(mesh, layers)
}
