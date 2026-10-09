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

// Paint is a solid color, or a pattern tiled through the inverse of
// PatternTransform when Pattern is set.
type Paint struct {
	Color            color.NRGBA
	Pattern          image.Image
	PatternTransform Matrix
}

// Stroke is a pen of Width in pen space; Transform maps pen space to the
// canvas. Hairlines are one pixel wide. Geometry is first translated by
// PixelCenter, as GDI draws lines through device pixel centers.
type Stroke struct {
	Width       float64
	Transform   Matrix
	Hairline    bool
	Cap         Cap
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
)

// ClipNode mirrors gowemf.ClipRegion. Nodes are cached by pointer.
type ClipNode struct {
	Base   *ClipNode
	Op     ClipOp
	Area   Path
	Rule   Rule
	Offset Point
}

// Canvas is an opaque white RGBA canvas.
type Canvas struct {
	Image *image.NRGBA
	masks map[clipKey][]float64
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
	return &Canvas{Image: c, masks: make(map[clipKey][]float64)}
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
func coverage(rings []polygon, rule Rule, w, h int) []float64 {
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
	out := make([]float64, w*h)
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
		acc := out[row*w : (row+1)*w]
		for s := 0; s < subScanlines; s++ {
			y := float64(row) + (float64(s)+0.5)/subScanlines
			xs = xs[:0]
			for _, e := range edges {
				if y >= e.y0 && y < e.y1 {
					xs = append(xs, crossing{e.x0 + (y-e.y0)*(e.x1-e.x0)/(e.y1-e.y0), e.dir})
				}
			}
			sort.Slice(xs, func(i, j int) bool { return xs[i].x < xs[j].x })
			wind := 0
			for i := 0; i+1 < len(xs); i++ {
				wind += xs[i].dir
				inside := wind != 0
				if rule == EvenOdd {
					inside = wind%2 != 0
				}
				if !inside {
					continue
				}
				a, b := math.Max(0, xs[i].x), math.Min(float64(w), xs[i+1].x)
				for px := int(math.Floor(a)); px < w && float64(px) < b; px++ {
					o := math.Min(b, float64(px+1)) - math.Max(a, float64(px))
					if o > 0 {
						acc[px] += o / subScanlines
					}
				}
			}
		}
	}
	return out
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
			for _, end := range [][2]Point{{pts[1], pts[0]}, {pts[len(pts)-2], pts[len(pts)-1]}} {
				from, at := end[0], end[1]
				switch s.Cap {
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

func (c *Canvas) size() (int, int) { return c.Image.Rect.Dx(), c.Image.Rect.Dy() }

// mask intersects every clip chain; nil or empty clips leave drawing unclipped.
func (c *Canvas) mask(clip []*ClipNode) []float64 {
	w, h := c.size()
	mask := make([]float64, w*h)
	for i := range mask {
		mask[i] = 1
	}
	for _, n := range clip {
		m := c.region(n, 0, 0)
		for i := range mask {
			mask[i] = math.Min(mask[i], m[i])
		}
	}
	return mask
}

func (c *Canvas) region(n *ClipNode, dx, dy float64) []float64 {
	w, h := c.size()
	if n == nil {
		m := make([]float64, w*h)
		for i := range m {
			m[i] = 1
		}
		return m
	}
	key := clipKey{n, dx, dy}
	if m, ok := c.masks[key]; ok {
		return m
	}
	var out []float64
	if n.Op == ClipOffset {
		out = c.region(n.Base, dx+n.Offset.X, dy+n.Offset.Y)
	} else {
		rings, _ := flattenPath(n.Area, 0.1)
		area := coverage(transformRings(rings, Matrix{M11: 1, M22: 1, Dx: dx, Dy: dy}), n.Rule, w, h)
		for i := range area {
			area[i] = math.Min(area[i], 1)
		}
		if n.Op == ClipReplace {
			out = area
		} else {
			base := c.region(n.Base, dx, dy)
			out = make([]float64, w*h)
			for i := range out {
				a, b := base[i], area[i]
				switch n.Op {
				case ClipIntersect:
					out[i] = math.Min(a, b)
				case ClipUnion:
					out[i] = math.Max(a, b)
				case ClipXor:
					out[i] = math.Abs(a - b)
				case ClipDifference:
					out[i] = math.Min(a, 1-b)
				}
			}
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

func (c *Canvas) paint(cov []float64, p Paint, clip []*ClipNode) error {
	w, h := c.size()
	mask := c.mask(clip)
	var inv Matrix
	if p.Pattern != nil {
		var ok bool
		if inv, ok = invert(p.PatternTransform); !ok {
			return errors.New("singular pattern transform")
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := math.Min(cov[y*w+x], 1) * mask[y*w+x]
			if a <= 0 {
				continue
			}
			col := p.Color
			if p.Pattern != nil {
				q := inv.Apply(Point{float64(x) + .5, float64(y) + .5})
				b := p.Pattern.Bounds()
				px := b.Min.X + int(math.Floor(q.X))%b.Dx()
				py := b.Min.Y + int(math.Floor(q.Y))%b.Dy()
				if px < b.Min.X {
					px += b.Dx()
				}
				if py < b.Min.Y {
					py += b.Dy()
				}
				col = color.NRGBAModel.Convert(p.Pattern.At(px, py)).(color.NRGBA)
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
	return c.paint(coverage(rings, rule, w, h), paint, clip)
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
	cov := make([]float64, w*h)
	for _, piece := range strokeRings(transformRings(rings, toPen), closed, s) {
		pc := coverage(transformRings(piece, fromPen), NonZero, w, h)
		for i := range cov {
			cov[i] = math.Max(cov[i], math.Min(pc[i], 1))
		}
	}
	return c.paint(cov, paint, clip)
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
