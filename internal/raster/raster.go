// Package raster is the reference rasterizer used only by tests and render
// oracles. It is written independently of gowemf's playback geometry: it
// flattens its own curves, strokes in pen space and takes coverage on 16
// sub-scanlines per pixel with exact horizontal spans. Compositing is
// source-over in sRGB space on an opaque white canvas, like the LibreOffice
// PNG export it is compared with. It deliberately does not import gowemf.
package raster

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
)

type Point struct{ X, Y float64 }

// Matrix is a row-vector affine transform: x'=x*M11+y*M21+Dx, y'=x*M12+y*M22+Dy.
type Matrix struct{ M11, M12, M21, M22, Dx, Dy float64 }

func Identity() Matrix { return Matrix{M11: 1, M22: 1} }
func (m Matrix) Apply(p Point) Point {
	return Point{p.X*m.M11 + p.Y*m.M21 + m.Dx, p.X*m.M12 + p.Y*m.M22 + m.Dy}
}

// Then composes m followed by n.
func (m Matrix) Then(n Matrix) Matrix {
	return Matrix{m.M11*n.M11 + m.M12*n.M21, m.M11*n.M12 + m.M12*n.M22, m.M21*n.M11 + m.M22*n.M21, m.M21*n.M12 + m.M22*n.M22, m.Dx*n.M11 + m.Dy*n.M21 + n.Dx, m.Dx*n.M12 + m.Dy*n.M22 + n.Dy}
}

type Verb uint8

const (
	MoveTo Verb = iota + 1
	LineTo
	CubicTo
	Close
)

// Path follows gowemf.Path: MoveTo and LineTo take one point, CubicTo three.
type Path struct {
	Verbs  []Verb
	Points []Point
}

type Rule uint8

const (
	EvenOdd Rule = iota + 1
	NonZero
)

type Cap uint8
type Join uint8

const (
	CapRound Cap = iota + 1
	CapSquare
	CapFlat
)
const (
	JoinRound Join = iota + 1
	JoinBevel
	JoinMiter
)

// Paint is a solid color, a pattern repeated by Wrap through the inverse of
// PatternTransform when Pattern is set, or a linear gradient.
type Paint struct {
	Color            color.NRGBA
	Pattern          image.Image
	PatternTransform Matrix
	Wrap             Wrap
	Gradient         *Gradient
}

// Wrap selects how patterns and gradients continue outside their tile.
type Wrap uint8

const (
	WrapTile Wrap = iota
	WrapFlipX
	WrapFlipY
	WrapFlipXY
	WrapClamp // nothing is painted outside the tile
)

// Stop is a gradient color at an offset in [0,1].
type Stop struct {
	Offset float64
	Color  color.NRGBA
}

// Gradient interpolates Stops (ascending offsets) linearly in straight-alpha
// sRGB by the x coordinate of Transform applied to the pixel center. The
// parameter repeats as Wrap describes; for this one-dimensional parameter
// FlipY behaves as Tile and FlipXY as FlipX, and Clamp extends the end colors.
type Gradient struct {
	Transform Matrix
	Stops     []Stop
	Wrap      Wrap
}

func (g *Gradient) at(p Point) color.NRGBA {
	t := g.Transform.Apply(p).X
	switch g.Wrap {
	case WrapTile, WrapFlipY:
		t -= math.Floor(t)
	case WrapFlipX, WrapFlipXY:
		t = math.Mod(math.Abs(t), 2)
		if t > 1 {
			t = 2 - t
		}
	}
	s := g.Stops
	if t <= s[0].Offset {
		return s[0].Color
	}
	for i := 1; i < len(s); i++ {
		if t <= s[i].Offset {
			a, b := s[i-1], s[i]
			if b.Offset == a.Offset {
				return b.Color
			}
			f := (t - a.Offset) / (b.Offset - a.Offset)
			l := func(x, y uint8) uint8 { return uint8(math.Round(float64(x) + (float64(y)-float64(x))*f)) }
			return color.NRGBA{l(a.Color.R, b.Color.R), l(a.Color.G, b.Color.G), l(a.Color.B, b.Color.B), l(a.Color.A, b.Color.A)}
		}
	}
	return s[len(s)-1].Color
}

// wrapIndex maps v into [0,n) per wrap, or reports it outside a clamped tile.
func wrapIndex(v, n int, flip, clamp bool) (int, bool) {
	if clamp {
		return v, v >= 0 && v < n
	}
	if !flip {
		v %= n
		if v < 0 {
			v += n
		}
		return v, true
	}
	v %= 2 * n
	if v < 0 {
		v += 2 * n
	}
	if v >= n {
		v = 2*n - 1 - v
	}
	return v, true
}

// Stroke is a pen of Width in pen space; Transform maps pen space to the
// canvas. Hairlines are one pixel wide. Geometry is first translated by
// PixelCenter, as GDI draws lines through device pixel centers.
type Stroke struct {
	Width       float64
	Transform   Matrix
	Hairline    bool
	Cap         Cap
	EndCap      Cap       // when nonzero, Cap is the start cap
	Compound    []float64 // symmetric band fractions; see gowemf.Stroke
	Join        Join
	MiterLimit  float64
	PixelCenter Point
}

type ClipOp uint8

const (
	ClipIntersect ClipOp = iota + 1
	ClipUnion
	ClipXor
	ClipDifference
	ClipReplace
	ClipOffset
	ClipComplement
)

// ClipNode mirrors gowemf.ClipRegion. Nodes are cached by pointer.
type ClipNode struct {
	Base    *ClipNode
	Op      ClipOp
	Area    Path
	Rule    Rule
	Operand *ClipNode
	Offset  Point
}

// Canvas is an opaque white RGBA canvas.
type Canvas struct {
	Image *image.NRGBA
	masks map[clipKey]spanSet
}

type clipKey struct {
	node   *ClipNode
	dx, dy float64
}

func New(w, h int) *Canvas {
	c := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range c.Pix {
		c.Pix[i] = 255
	}
	return &Canvas{Image: c, masks: make(map[clipKey]spanSet)}
}

const subScanlines = 16

// polygon is a closed ring in destination coordinates.
type polygon []Point

// flattenPath converts a path into rings. Open figures are returned as well;
// filling treats every ring as closed. closed reports explicit Close.
func flattenPath(p Path, tolerance float64) (rings []polygon, closed []bool) {
	var cur polygon
	var start Point // figure start, for segments following Close
	flush := func(c bool) {
		if len(cur) > 1 {
			rings = append(rings, cur)
			closed = append(closed, c)
		}
		cur = nil
	}
	i := 0
	for _, v := range p.Verbs {
		if (v == LineTo || v == CubicTo) && cur == nil {
			cur = polygon{start}
		}
		switch v {
		case MoveTo:
			flush(false)
			start = p.Points[i]
			cur = polygon{start}
			i++
		case LineTo:
			cur = append(cur, p.Points[i])
			i++
		case CubicTo:
			p0 := cur[len(cur)-1]
			p1, p2, p3 := p.Points[i], p.Points[i+1], p.Points[i+2]
			length := math.Hypot(p1.X-p0.X, p1.Y-p0.Y) + math.Hypot(p2.X-p1.X, p2.Y-p1.Y) + math.Hypot(p3.X-p2.X, p3.Y-p2.Y)
			n := int(math.Ceil(length / tolerance))
			if n < 1 {
				n = 1
			}
			if n > 4096 {
				n = 4096
			}
			for k := 1; k <= n; k++ {
				t := float64(k) / float64(n)
				u := 1 - t
				cur = append(cur, Point{
					u*u*u*p0.X + 3*u*u*t*p1.X + 3*u*t*t*p2.X + t*t*t*p3.X,
					u*u*u*p0.Y + 3*u*u*t*p1.Y + 3*u*t*t*p2.Y + t*t*t*p3.Y,
				})
			}
			i += 3
		case Close:
			flush(true)
		}
	}
	flush(false)
	return rings, closed
}

// coverage rasterizes rings with a fill rule into a w*h buffer in [0,1].
// span is a covered interval [x0,x1) of one sub-scanline.
type span struct{ x0, x1 float64 }

// spanSet lists, for each of h*subScanlines sub-scanlines, the sorted
// disjoint covered intervals within [0,w). Boolean operations on span sets
// are exact, so regions sharing an edge combine without anti-aliasing slivers.
type spanSet [][]span

// scan samples rings under rule at the center of each sub-scanline,
// computing exact horizontal intervals.
func scan(rings []polygon, rule Rule, w, h int) spanSet {
	type edge struct {
		x0, y0, x1, y1 float64
		dir            int
	}
	var edges []edge
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, r := range rings {
		for i := range r {
			a, b := r[i], r[(i+1)%len(r)]
			if a.Y == b.Y {
				continue
			}
			e := edge{a.X, a.Y, b.X, b.Y, 1}
			if a.Y > b.Y {
				e = edge{b.X, b.Y, a.X, a.Y, -1}
			}
			edges = append(edges, e)
			minY, maxY = math.Min(minY, e.y0), math.Max(maxY, e.y1)
		}
	}
	out := make(spanSet, h*subScanlines)
	if len(edges) == 0 {
		return out
	}
	type crossing struct {
		x   float64
		dir int
	}
	row0 := int(math.Max(0, math.Floor(minY)))
	row1 := int(math.Min(float64(h), math.Ceil(maxY)))
	var xs []crossing
	for row := row0; row < row1; row++ {
		for s := 0; s < subScanlines; s++ {
			y := float64(row) + (float64(s)+0.5)/subScanlines
			xs = xs[:0]
			for _, e := range edges {
				if y >= e.y0 && y < e.y1 {
					xs = append(xs, crossing{e.x0 + (y-e.y0)*(e.x1-e.x0)/(e.y1-e.y0), e.dir})
				}
			}
			sort.Slice(xs, func(i, j int) bool { return xs[i].x < xs[j].x })
			var line []span
			wind := 0
			for i := 0; i+1 < len(xs); i++ {
				wind += xs[i].dir
				inside := wind != 0
				if rule == EvenOdd {
					inside = wind%2 != 0
				}
				a, b := math.Max(0, xs[i].x), math.Min(float64(w), xs[i+1].x)
				if !inside || b <= a {
					continue
				}
				if n := len(line); n > 0 && a <= line[n-1].x1 {
					line[n-1].x1 = math.Max(line[n-1].x1, b)
				} else {
					line = append(line, span{a, b})
				}
			}
			out[row*subScanlines+s] = line
		}
	}
	return out
}

// full is the whole canvas.
func full(w, h int) spanSet {
	out := make(spanSet, h*subScanlines)
	for i := range out {
		out[i] = []span{{0, float64(w)}}
	}
	return out
}

// combine applies keep to membership in a and b along each sub-scanline.
func combine(a, b spanSet, keep func(inA, inB bool) bool) spanSet {
	out := make(spanSet, len(a))
	bound := func(s []span, k int) float64 {
		if k%2 == 0 {
			return s[k/2].x0
		}
		return s[k/2].x1
	}
	for r := range a {
		sa, sb := a[r], b[r]
		var line []span
		i, j := 0, 0
		inA, inB, open := false, false, false
		start := 0.0
		for i < 2*len(sa) || j < 2*len(sb) {
			var x float64
			if j >= 2*len(sb) || (i < 2*len(sa) && bound(sa, i) <= bound(sb, j)) {
				x = bound(sa, i)
			} else {
				x = bound(sb, j)
			}
			for i < 2*len(sa) && bound(sa, i) == x {
				inA = !inA
				i++
			}
			for j < 2*len(sb) && bound(sb, j) == x {
				inB = !inB
				j++
			}
			if k := keep(inA, inB); k && !open {
				start, open = x, true
			} else if !k && open {
				line = append(line, span{start, x})
				open = false
			}
		}
		out[r] = line
	}
	return out
}

func intersect(a, b spanSet) spanSet {
	return combine(a, b, func(x, y bool) bool { return x && y })
}

// coverageOf converts spans to per-pixel area coverage in [0,1].
func coverageOf(set spanSet, w, h int) []float64 {
	out := make([]float64, w*h)
	for r, line := range set {
		acc := out[r/subScanlines*w : (r/subScanlines+1)*w]
		for _, sp := range line {
			for px := int(math.Floor(sp.x0)); px < w && float64(px) < sp.x1; px++ {
				if o := math.Min(sp.x1, float64(px+1)) - math.Max(sp.x0, float64(px)); o > 0 {
					acc[px] += o / subScanlines
				}
			}
		}
	}
	for i := range out {
		out[i] = math.Min(out[i], 1)
	}
	return out
}

// coverage is the per-pixel area coverage of rings under rule.
func coverage(rings []polygon, rule Rule, w, h int) []float64 {
	return coverageOf(scan(rings, rule, w, h), w, h)
}

func transformRings(rings []polygon, m Matrix) []polygon {
	out := make([]polygon, len(rings))
	for i, r := range rings {
		out[i] = make(polygon, len(r))
		for j, q := range r {
			out[i][j] = m.Apply(q)
		}
	}
	return out
}

func invert(m Matrix) (Matrix, bool) {
	det := m.M11*m.M22 - m.M12*m.M21
	if det == 0 || math.IsNaN(det) || math.IsInf(det, 0) {
		return Matrix{}, false
	}
	inv := Matrix{M11: m.M22 / det, M12: -m.M12 / det, M21: -m.M21 / det, M22: m.M11 / det}
	inv.Dx = -(m.Dx*inv.M11 + m.Dy*inv.M21)
	inv.Dy = -(m.Dx*inv.M12 + m.Dy*inv.M22)
	return inv, true
}

func circle(c Point, r float64) polygon {
	const n = 48
	out := make(polygon, n)
	for i := range out {
		a := 2 * math.Pi * float64(i) / n
		out[i] = Point{c.X + r*math.Cos(a), c.Y + r*math.Sin(a)}
	}
	return out
}

// strokeRings builds stroke pieces in pen space. Each piece is filled with
// NonZero independently and unioned by maximum coverage.
func strokeRings(rings []polygon, closed []bool, s Stroke) [][]polygon {
	half := s.Width / 2
	var pieces [][]polygon
	add := func(p polygon) { pieces = append(pieces, []polygon{p}) }
	normal := func(a, b Point) Point {
		l := math.Hypot(b.X-a.X, b.Y-a.Y)
		return Point{-(b.Y - a.Y) / l * half, (b.X - a.X) / l * half}
	}
	for ri, r := range rings {
		// Drop repeated points; they have no direction.
		var pts polygon
		for _, q := range r {
			if len(pts) == 0 || q != pts[len(pts)-1] {
				pts = append(pts, q)
			}
		}
		isClosed := closed[ri]
		if isClosed && len(pts) > 1 && pts[0] == pts[len(pts)-1] {
			pts = pts[:len(pts)-1]
		}
		if len(pts) == 1 {
			if s.Cap == CapRound {
				add(circle(pts[0], half))
			}
			continue
		}
		segs := len(pts) - 1
		if isClosed {
			segs = len(pts)
		}
		for i := 0; i < segs; i++ {
			a, b := pts[i], pts[(i+1)%len(pts)]
			n := normal(a, b)
			add(polygon{{a.X + n.X, a.Y + n.Y}, {b.X + n.X, b.Y + n.Y}, {b.X - n.X, b.Y - n.Y}, {a.X - n.X, a.Y - n.Y}})
		}
		joins := len(pts) - 2
		first := 1
		if isClosed {
			joins, first = len(pts), 0
		}
		for k := 0; k < joins; k++ {
			i := first + k
			prev, v, next := pts[(i-1+len(pts))%len(pts)], pts[i%len(pts)], pts[(i+1)%len(pts)]
			n1, n2 := normal(prev, v), normal(v, next)
			switch s.Join {
			case JoinRound:
				add(circle(v, half))
			default:
				add(polygon{v, {v.X + n1.X, v.Y + n1.Y}, {v.X + n2.X, v.Y + n2.Y}})
				add(polygon{v, {v.X - n1.X, v.Y - n1.Y}, {v.X - n2.X, v.Y - n2.Y}})
				if s.Join == JoinMiter {
					for _, sign := range []float64{1, -1} {
						a := Point{v.X + sign*n1.X, v.Y + sign*n1.Y}
						b := Point{v.X + sign*n2.X, v.Y + sign*n2.Y}
						d1 := Point{v.X - prev.X, v.Y - prev.Y}
						d2 := Point{next.X - v.X, next.Y - v.Y}
						den := d1.X*d2.Y - d1.Y*d2.X
						if den == 0 {
							continue
						}
						t := ((b.X-a.X)*d2.Y - (b.Y-a.Y)*d2.X) / den
						m := Point{a.X + t*d1.X, a.Y + t*d1.Y}
						if math.Hypot(m.X-v.X, m.Y-v.Y) <= s.MiterLimit*half && t >= 0 {
							add(polygon{v, a, m, b})
						}
					}
				}
			}
		}
		if !isClosed {
			for k, end := range [][2]Point{{pts[1], pts[0]}, {pts[len(pts)-2], pts[len(pts)-1]}} {
				from, at := end[0], end[1]
				capStyle := s.Cap
				if k == 1 && s.EndCap != 0 {
					capStyle = s.EndCap
				}
				switch capStyle {
				case CapRound:
					add(circle(at, half))
				case CapSquare:
					n := normal(from, at)
					d := Point{n.Y, -n.X} // forward along the segment, length half
					add(polygon{{at.X + n.X, at.Y + n.Y}, {at.X + n.X + d.X, at.Y + n.Y + d.Y}, {at.X - n.X + d.X, at.Y - n.Y + d.Y}, {at.X - n.X, at.Y - n.Y}})
				}
			}
		}
	}
	return pieces
}

// area is the signed area of a ring.
func area(r polygon) float64 {
	var a float64
	for i := range r {
		p, q := r[i], r[(i+1)%len(r)]
		a += p.X*q.Y - q.X*p.Y
	}
	return a / 2
}

func (c *Canvas) size() (int, int) { return c.Image.Rect.Dx(), c.Image.Rect.Dy() }

// clipSpans intersects every clip chain; it returns nil, meaning
// unclipped, for an empty clip.
func (c *Canvas) clipSpans(clip []*ClipNode) spanSet {
	var out spanSet
	for _, n := range clip {
		r := c.region(n, 0, 0)
		if out == nil {
			out = r
		} else {
			out = intersect(out, r)
		}
	}
	return out
}

// clipped restricts set to the clip and returns its pixel coverage.
func (c *Canvas) clipped(set spanSet, clip []*ClipNode) []float64 {
	w, h := c.size()
	if m := c.clipSpans(clip); m != nil {
		set = intersect(set, m)
	}
	return coverageOf(set, w, h)
}

// mask is the pixel coverage of the clip.
func (c *Canvas) mask(clip []*ClipNode) []float64 {
	w, h := c.size()
	return c.clipped(full(w, h), clip)
}

func (c *Canvas) region(n *ClipNode, dx, dy float64) spanSet {
	w, h := c.size()
	if n == nil {
		return full(w, h)
	}
	key := clipKey{n, dx, dy}
	if m, ok := c.masks[key]; ok {
		return m
	}
	var out spanSet
	if n.Op == ClipOffset {
		out = c.region(n.Base, dx+n.Offset.X, dy+n.Offset.Y)
	} else {
		var area spanSet
		if n.Operand != nil {
			area = c.region(n.Operand, dx, dy)
		} else {
			rings, _ := flattenPath(n.Area, 0.1)
			area = scan(transformRings(rings, Matrix{M11: 1, M22: 1, Dx: dx, Dy: dy}), n.Rule, w, h)
		}
		if n.Op == ClipReplace {
			out = area
		} else {
			keep := map[ClipOp]func(a, b bool) bool{
				ClipIntersect:  func(a, b bool) bool { return a && b },
				ClipUnion:      func(a, b bool) bool { return a || b },
				ClipXor:        func(a, b bool) bool { return a != b },
				ClipDifference: func(a, b bool) bool { return a && !b },
				ClipComplement: func(a, b bool) bool { return b && !a },
			}[n.Op]
			out = combine(c.region(n.Base, dx, dy), area, keep)
		}
	}
	c.masks[key] = out
	return out
}

func blend(dst *image.NRGBA, i int, c color.NRGBA, a float64) {
	a *= float64(c.A) / 255
	if a <= 0 {
		return
	}
	if a > 1 {
		a = 1
	}
	for k, v := range [3]uint8{c.R, c.G, c.B} {
		d := float64(dst.Pix[i+k])
		dst.Pix[i+k] = uint8(math.Round(float64(v)*a + d*(1-a)))
	}
}

// paint composites p over pixels with coverage cov, already clipped.
func (c *Canvas) paint(cov []float64, p Paint) error {
	w, h := c.size()
	var inv Matrix
	if p.Pattern != nil {
		var ok bool
		if inv, ok = invert(p.PatternTransform); !ok {
			return errors.New("singular pattern transform")
		}
	}
	if p.Gradient != nil && len(p.Gradient.Stops) == 0 {
		return errors.New("gradient without stops")
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := cov[y*w+x]
			if a <= 0 {
				continue
			}
			col := p.Color
			switch {
			case p.Gradient != nil:
				col = p.Gradient.at(Point{float64(x) + .5, float64(y) + .5})
			case p.Pattern != nil:
				q := inv.Apply(Point{float64(x) + .5, float64(y) + .5})
				b := p.Pattern.Bounds()
				clamp := p.Wrap == WrapClamp
				px, okx := wrapIndex(int(math.Floor(q.X)), b.Dx(), p.Wrap == WrapFlipX || p.Wrap == WrapFlipXY, clamp)
				py, oky := wrapIndex(int(math.Floor(q.Y)), b.Dy(), p.Wrap == WrapFlipY || p.Wrap == WrapFlipXY, clamp)
				if !okx || !oky {
					continue
				}
				col = color.NRGBAModel.Convert(p.Pattern.At(b.Min.X+px, b.Min.Y+py)).(color.NRGBA)
			}
			blend(c.Image, y*c.Image.Stride+x*4, col, a)
		}
	}
	return nil
}

// Fill paints the area of path under rule.
func (c *Canvas) Fill(path Path, rule Rule, paint Paint, clip []*ClipNode) error {
	w, h := c.size()
	rings, _ := flattenPath(path, 0.05)
	return c.paint(c.clipped(scan(rings, rule, w, h), clip), paint)
}

// Stroke paints the outline of path with pen s.
func (c *Canvas) Stroke(path Path, s Stroke, paint Paint, clip []*ClipNode) error {
	w, h := c.size()
	center := Matrix{M11: 1, M22: 1, Dx: s.PixelCenter.X, Dy: s.PixelCenter.Y}
	toPen, fromPen := center, Identity()
	if s.Hairline {
		s.Width, s.Cap, s.Join = 1, CapSquare, JoinMiter
		s.MiterLimit = 2
	} else {
		fromPen = s.Transform
		inv, ok := invert(s.Transform)
		if !ok {
			return errors.New("singular pen transform")
		}
		toPen = center.Then(inv)
	}
	rings, closed := flattenPath(path, 0.05)
	penRings := transformRings(rings, toPen)
	// The outline is the union of segment, join and cap pieces: oriented
	// alike, they cover it exactly under the nonzero rule.
	outline := func(s Stroke) spanSet {
		var all []polygon
		for _, piece := range strokeRings(penRings, closed, s) {
			for _, r := range transformRings(piece, fromPen) {
				if area(r) < 0 {
					for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
						r[i], r[j] = r[j], r[i]
					}
				}
				all = append(all, r)
			}
		}
		return scan(all, NonZero, w, h)
	}
	if len(s.Compound) == 0 {
		return c.paint(c.clipped(outline(s), clip), paint)
	}
	if len(s.Compound)%2 != 0 || s.Hairline {
		return errors.New("invalid compound stroke")
	}
	width := s.Width
	sized := func(f float64) spanSet {
		t := s
		t.Width = width * f
		return outline(t)
	}
	var bands spanSet = make(spanSet, h*subScanlines)
	union := func(a, b bool) bool { return a || b }
	for i := 0; i < len(s.Compound); i += 2 {
		a, b := s.Compound[i], s.Compound[i+1]
		switch {
		case b <= .5:
			band := combine(sized(1-2*a), sized(1-2*b), func(x, y bool) bool { return x && !y })
			bands = combine(bands, band, union)
		case a < .5:
			bands = combine(bands, sized(1-2*a), union)
		}
		// Bands past the center mirror those before it.
	}
	return c.paint(c.clipped(bands, clip), paint)
}

// Image composites src pixels of img placed by m (image pixels to canvas),
// sampling 4x4 points per canvas pixel with nearest-pixel lookup.
func (c *Canvas) DrawImage(img image.Image, src image.Rectangle, m Matrix, opacity float64, clip []*ClipNode) error {
	inv, ok := invert(m)
	if !ok {
		return nil // a degenerate placement covers no area
	}
	w, h := c.size()
	mask := c.mask(clip)
	const n = 4
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, b, a float64
			for sy := 0; sy < n; sy++ {
				for sx := 0; sx < n; sx++ {
					q := inv.Apply(Point{float64(x) + (float64(sx)+.5)/n, float64(y) + (float64(sy)+.5)/n})
					if q.X < float64(src.Min.X) || q.Y < float64(src.Min.Y) || q.X >= float64(src.Max.X) || q.Y >= float64(src.Max.Y) {
						continue
					}
					col := color.NRGBAModel.Convert(img.At(int(math.Floor(q.X)), int(math.Floor(q.Y)))).(color.NRGBA)
					ca := float64(col.A) / 255
					r += float64(col.R) * ca
					g += float64(col.G) * ca
					b += float64(col.B) * ca
					a += ca
				}
			}
			if a == 0 {
				continue
			}
			col := color.NRGBA{uint8(math.Round(r / a)), uint8(math.Round(g / a)), uint8(math.Round(b / a)), 255}
			blend(c.Image, y*c.Image.Stride+x*4, col, a/(n*n)*opacity*mask[y*w+x])
		}
	}
	return nil
}

// CompareNeighborhood is a symmetric tolerance for renderer pixel
// conventions: each channel of every pixel in each image must lie, within
// delta, inside the range spanned by that channel over the other image's
// pixels within radius. Displaced, missing or recolored content fails;
// half-pixel edge placement and differing anti-aliasing ramps do not. At most
// maxBadFraction of pixels may fail in either direction.
func CompareNeighborhood(a, b image.Image, delta uint32, radius int, maxBadFraction float64) error {
	bad, total, err := NeighborhoodMismatch(a, b, delta, radius)
	if err == nil && float64(bad) > float64(total)*maxBadFraction {
		err = fmt.Errorf("%d/%d pixels have no match within %d px and %d per channel (allowed fraction %.4f)", bad, total, radius, delta, maxBadFraction)
	}
	return err
}

// NeighborhoodMismatch returns the larger directional mismatch count.
func NeighborhoodMismatch(a, b image.Image, delta uint32, radius int) (worst, total int, err error) {
	if a.Bounds() != b.Bounds() {
		return 0, 0, fmt.Errorf("bounds %v != %v", a.Bounds(), b.Bounds())
	}
	r := a.Bounds()
	near := func(p, q image.Image, x, y int) bool {
		pr, pg, pb, _ := p.At(x, y).RGBA()
		lo, hi := [3]int64{1 << 20, 1 << 20, 1 << 20}, [3]int64{-1, -1, -1}
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				if !(image.Point{x + dx, y + dy}).In(r) {
					continue
				}
				qr, qg, qb, _ := q.At(x+dx, y+dy).RGBA()
				for i, v := range [3]uint32{qr, qg, qb} {
					lo[i], hi[i] = min(lo[i], int64(v)), max(hi[i], int64(v))
				}
			}
		}
		for i, v := range [3]uint32{pr, pg, pb} {
			if int64(v) < lo[i]-int64(delta)*257 || int64(v) > hi[i]+int64(delta)*257 {
				return false
			}
		}
		return true
	}
	total = r.Dx() * r.Dy()
	for _, pair := range [][2]image.Image{{a, b}, {b, a}} {
		bad := 0
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				if !near(pair[0], pair[1], x, y) {
					bad++
				}
			}
		}
		worst = max(worst, bad)
	}
	return worst, total, nil
}

// Triangle is a Gouraud-shaded triangle: colors interpolate linearly over it.
type Triangle struct {
	P [3]Point
	C [3]color.NRGBA64
}

// FillGouraud paints a triangle mesh. Coverage is that of the whole mesh so
// shared edges have no seams; each pixel takes the barycentric color of the
// triangle containing its center, or of the nearest triangle at the mesh edge.
// 16-bit channels map to 8 bits by their high byte, as GDI's true-color
// gradient fills do. The mesh shares the alpha of its first vertex.
func (c *Canvas) FillGouraud(mesh []Triangle, clip []*ClipNode) error {
	w, h := c.size()
	rings := make([]polygon, len(mesh))
	for i, t := range mesh {
		rings[i] = polygon{t.P[0], t.P[1], t.P[2]}
		// Orient every ring the same way so NonZero unions them.
		if (t.P[1].X-t.P[0].X)*(t.P[2].Y-t.P[0].Y)-(t.P[1].Y-t.P[0].Y)*(t.P[2].X-t.P[0].X) < 0 {
			rings[i][1], rings[i][2] = rings[i][2], rings[i][1]
		}
	}
	cov := c.clipped(scan(rings, NonZero, w, h), clip)
	bary := func(t Triangle, p Point) (float64, float64, float64, bool) {
		d := (t.P[1].Y-t.P[2].Y)*(t.P[0].X-t.P[2].X) + (t.P[2].X-t.P[1].X)*(t.P[0].Y-t.P[2].Y)
		if d == 0 {
			return 0, 0, 0, false
		}
		a := ((t.P[1].Y-t.P[2].Y)*(p.X-t.P[2].X) + (t.P[2].X-t.P[1].X)*(p.Y-t.P[2].Y)) / d
		b := ((t.P[2].Y-t.P[0].Y)*(p.X-t.P[2].X) + (t.P[0].X-t.P[2].X)*(p.Y-t.P[2].Y)) / d
		return a, b, 1 - a - b, true
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := cov[y*w+x]
			if a <= 0 {
				continue
			}
			p := Point{float64(x) + .5, float64(y) + .5}
			best, bestScore := -1, math.Inf(-1)
			var wa, wb, wc float64
			for i, t := range mesh {
				ba, bb, bc, ok := bary(t, p)
				if !ok {
					continue
				}
				if score := math.Min(ba, math.Min(bb, bc)); score > bestScore {
					best, bestScore, wa, wb, wc = i, score, ba, bb, bc
				}
			}
			if best < 0 {
				continue
			}
			// Clamp to the triangle for pixels just outside it.
			wa, wb, wc = math.Max(wa, 0), math.Max(wb, 0), math.Max(wc, 0)
			s := wa + wb + wc
			t := mesh[best]
			ch := func(k int) uint8 {
				v := [3]float64{}
				for i, col := range t.C {
					v[i] = float64([3]uint16{col.R, col.G, col.B}[k])
				}
				return uint8(min(255, math.Round((wa*v[0]+wb*v[1]+wc*v[2])/s/256)))
			}
			blend(c.Image, y*c.Image.Stride+x*4, color.NRGBA{ch(0), ch(1), ch(2), uint8(t.C[0].A >> 8)}, a)
		}
	}
	return nil
}
