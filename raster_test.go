package gowemf

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"testing"
)

// rasterBackend is a test-only reference renderer for Play. It is written
// independently of the playback geometry code: it flattens its own curves,
// strokes in pen space and samples coverage on 16 sub-scanlines per pixel with
// exact horizontal span coverage. Compositing is source-over in sRGB space on
// an opaque white canvas, like the LibreOffice PNG export it is compared with.
type rasterBackend struct {
	canvas *image.NRGBA
	masks  map[clipKey][]float64
	calls  int
}

type clipKey struct {
	node   *ClipRegion
	dx, dy float64
}

func newRasterBackend(w, h int) *rasterBackend {
	c := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range c.Pix {
		c.Pix[i] = 255
	}
	return &rasterBackend{canvas: c, masks: make(map[clipKey][]float64)}
}

const subScanlines = 16

// polygon is a closed ring in destination coordinates.
type polygon []Point

// flattenPath converts a path into rings. Open figures are returned as well;
// filling treats every ring as closed. closed reports explicit PathClose.
func flattenPath(p Path, tolerance float64) (rings []polygon, closed []bool) {
	var cur polygon
	var start Point // figure start, for segments following PathClose
	flush := func(c bool) {
		if len(cur) > 1 {
			rings = append(rings, cur)
			closed = append(closed, c)
		}
		cur = nil
	}
	i := 0
	for _, v := range p.Verbs {
		if (v == PathLineTo || v == PathCubicTo) && cur == nil {
			cur = polygon{start}
		}
		switch v {
		case PathMoveTo:
			flush(false)
			start = p.Points[i]
			cur = polygon{start}
			i++
		case PathLineTo:
			cur = append(cur, p.Points[i])
			i++
		case PathCubicTo:
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
		case PathClose:
			flush(true)
		}
	}
	flush(false)
	return rings, closed
}

// coverage rasterizes rings with a fill rule into a w*h buffer in [0,1].
func coverage(rings []polygon, rule FillRule, w, h int) []float64 {
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
	if det == 0 || !finite(det) {
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

func (rb *rasterBackend) size() (int, int) { return rb.canvas.Rect.Dx(), rb.canvas.Rect.Dy() }

func (rb *rasterBackend) clipMask(clip Clip) []float64 {
	w, h := rb.size()
	mask := make([]float64, w*h)
	for i := range mask {
		mask[i] = 1
	}
	for _, c := range clip {
		m := rb.region(c, 0, 0)
		for i := range mask {
			mask[i] = math.Min(mask[i], m[i])
		}
	}
	return mask
}

func (rb *rasterBackend) region(c *ClipRegion, dx, dy float64) []float64 {
	w, h := rb.size()
	if c == nil {
		m := make([]float64, w*h)
		for i := range m {
			m[i] = 1
		}
		return m
	}
	key := clipKey{c, dx, dy}
	if m, ok := rb.masks[key]; ok {
		return m
	}
	var out []float64
	if c.Op == ClipOffset {
		out = rb.region(c.Base, dx+c.Offset.X, dy+c.Offset.Y)
	} else {
		rings, _ := flattenPath(c.Area, 0.1)
		area := coverage(transformRings(rings, Matrix{M11: 1, M22: 1, Dx: dx, Dy: dy}), c.Rule, w, h)
		for i := range area {
			area[i] = math.Min(area[i], 1)
		}
		if c.Op == ClipReplace {
			out = area
		} else {
			base := rb.region(c.Base, dx, dy)
			out = make([]float64, w*h)
			for i := range out {
				a, b := base[i], area[i]
				switch c.Op {
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
	rb.masks[key] = out
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

func (rb *rasterBackend) paint(cov []float64, p Paint, clip Clip) error {
	w, h := rb.size()
	mask := rb.clipMask(clip)
	var inv Matrix
	if p.Kind == PaintPattern {
		var ok bool
		if inv, ok = invert(p.PatternTransform); !ok {
			return errors.New("singular pattern transform")
		}
	} else if p.Kind != PaintSolid {
		return fmt.Errorf("test rasterizer does not implement paint kind %d", p.Kind)
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := math.Min(cov[y*w+x], 1) * mask[y*w+x]
			if a <= 0 {
				continue
			}
			c := p.Color
			if p.Kind == PaintPattern {
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
				c = color.NRGBAModel.Convert(p.Pattern.At(px, py)).(color.NRGBA)
			}
			blend(rb.canvas, y*rb.canvas.Stride+x*4, c, a)
		}
	}
	return nil
}

func (rb *rasterBackend) FillPath(path Path, rule FillRule, paint Paint, clip Clip) error {
	rb.calls++
	w, h := rb.size()
	rings, _ := flattenPath(path, 0.05)
	return rb.paint(coverage(rings, rule, w, h), paint, clip)
}

func (rb *rasterBackend) StrokePath(path Path, s Stroke, clip Clip) error {
	rb.calls++
	if s.Dash != DashSolid {
		return fmt.Errorf("test rasterizer does not implement dash style %d", s.Dash)
	}
	w, h := rb.size()
	// GDI draws lines through device pixel centers.
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
		c := coverage(transformRings(piece, fromPen), NonZero, w, h)
		for i := range cov {
			cov[i] = math.Max(cov[i], math.Min(c[i], 1))
		}
	}
	return rb.paint(cov, s.Paint, clip)
}

func (rb *rasterBackend) DrawImage(d ImageDraw, clip Clip) error {
	rb.calls++
	inv, ok := invert(d.Transform)
	if !ok {
		return nil // a degenerate placement covers no area
	}
	w, h := rb.size()
	mask := rb.clipMask(clip)
	const n = 4 // 4x4 samples per destination pixel
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, b, a float64
			for sy := 0; sy < n; sy++ {
				for sx := 0; sx < n; sx++ {
					q := inv.Apply(Point{float64(x) + (float64(sx)+.5)/n, float64(y) + (float64(sy)+.5)/n})
					ix, iy := int(math.Floor(q.X)), int(math.Floor(q.Y))
					if q.X < float64(d.Source.Min.X) || q.Y < float64(d.Source.Min.Y) || q.X >= float64(d.Source.Max.X) || q.Y >= float64(d.Source.Max.Y) {
						continue
					}
					c := color.NRGBAModel.Convert(d.Image.At(ix, iy)).(color.NRGBA)
					ca := float64(c.A) / 255
					r += float64(c.R) * ca
					g += float64(c.G) * ca
					b += float64(c.B) * ca
					a += ca
				}
			}
			if a == 0 {
				continue
			}
			c := color.NRGBA{uint8(math.Round(r / a)), uint8(math.Round(g / a)), uint8(math.Round(b / a)), 255}
			blend(rb.canvas, y*rb.canvas.Stride+x*4, c, a/(n*n)*d.Opacity*mask[y*w+x])
		}
	}
	return nil
}

// compareNeighborhood is a symmetric tolerance for renderer pixel
// conventions: each channel of every pixel in each image must lie, within
// delta, inside the range spanned by that channel over the other image's
// pixels within radius. Displaced, missing or recolored content fails;
// half-pixel edge placement and differing anti-aliasing ramps do not. At most
// maxBadFraction of pixels may fail in either direction.
func compareNeighborhood(a, b image.Image, delta uint32, radius int, maxBadFraction float64) error {
	bad, total, err := neighborhoodMismatch(a, b, delta, radius)
	if err == nil && float64(bad) > float64(total)*maxBadFraction {
		err = fmt.Errorf("%d/%d pixels have no match within %d px and %d per channel (allowed fraction %.4f)", bad, total, radius, delta, maxBadFraction)
	}
	return err
}

// neighborhoodMismatch returns the larger directional mismatch count.
func neighborhoodMismatch(a, b image.Image, delta uint32, radius int) (worst, total int, err error) {
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

func TestNeighborhoodComparisonRejectsDisplacement(t *testing.T) {
	a := newRasterBackend(32, 32)
	b := newRasterBackend(32, 32)
	square := func(rb *rasterBackend, x float64) {
		p := Path{Verbs: []PathVerb{PathMoveTo, PathLineTo, PathLineTo, PathLineTo, PathClose}, Points: []Point{{x, 8}, {x + 12, 8}, {x + 12, 20}, {x, 20}}}
		if err := rb.FillPath(p, NonZero, Paint{Kind: PaintSolid, Color: color.NRGBA{A: 255}}, nil); err != nil {
			t.Fatal(err)
		}
	}
	square(a, 8)
	square(b, 8.5)
	if err := compareNeighborhood(a.canvas, b.canvas, 40, 1, 0); err != nil {
		t.Fatal("half-pixel edge shift rejected:", err)
	}
	c := newRasterBackend(32, 32)
	square(c, 11)
	if err := compareNeighborhood(a.canvas, c.canvas, 40, 1, .01); err == nil {
		t.Fatal("comparison missed a three-pixel displacement")
	}
}

func TestRasterCoverageIsExact(t *testing.T) {
	// A half-pixel-aligned rectangle covers interior pixels fully and edge
	// pixels by exactly half; even-odd removes a nested square.
	rect := func(l, t, r, b float64) polygon { return polygon{{l, t}, {r, t}, {r, b}, {l, b}} }
	c := coverage([]polygon{rect(1.5, 1, 6.5, 5)}, NonZero, 8, 6)
	for x, want := range []float64{0, .5, 1, 1, 1, 1, .5, 0} {
		if math.Abs(c[2*8+x]-want) > 1e-9 {
			t.Fatalf("x=%d coverage %v want %v", x, c[2*8+x], want)
		}
	}
	if c[0] != 0 || c[5*8+3] != 0 {
		t.Fatal("coverage outside the rectangle")
	}
	nested := []polygon{rect(0, 0, 8, 8), rect(2, 2, 6, 6)}
	if c := coverage(nested, EvenOdd, 8, 8); c[4*8+4] != 0 || c[1*8+1] != 1 {
		t.Fatal("even-odd nested square", c[4*8+4], c[1*8+1])
	}
	if c := coverage(nested, NonZero, 8, 8); c[4*8+4] != 1 {
		t.Fatal("nonzero same-direction nested square", c[4*8+4])
	}
}

func TestRasterStrokeWidth(t *testing.T) {
	rb := newRasterBackend(20, 20)
	path := Path{Verbs: []PathVerb{PathMoveTo, PathLineTo}, Points: []Point{{2, 10}, {18, 10}}}
	// A 4-unit pen in pen space, scaled by 0.5 vertically, is 2 pixels tall.
	s := Stroke{Paint: Paint{Kind: PaintSolid, Color: color.NRGBA{A: 255}}, Width: 4, Transform: Matrix{M11: 1, M22: .5}, Cap: CapFlat, Join: JoinMiter, MiterLimit: 10, Dash: DashSolid}
	if err := rb.StrokePath(path, s, nil); err != nil {
		t.Fatal(err)
	}
	for y, want := range map[int]uint8{8: 255, 9: 0, 10: 0, 11: 255} {
		if got := rb.canvas.NRGBAAt(10, y).R; got != want {
			t.Fatalf("row %d = %d, want %d", y, got, want)
		}
	}
	if got := rb.canvas.NRGBAAt(1, 10).R; got != 255 {
		t.Fatal("flat cap extended past the end point", got)
	}
}
