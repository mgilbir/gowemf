package raster

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestCoverageIsExact(t *testing.T) {
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

func TestStrokeWidth(t *testing.T) {
	c := New(20, 20)
	path := Path{Verbs: []Verb{MoveTo, LineTo}, Points: []Point{{2, 10}, {18, 10}}}
	// A 4-unit pen in pen space, scaled by 0.5 vertically, is 2 pixels tall.
	s := Stroke{Width: 4, Transform: Matrix{M11: 1, M22: .5}, Cap: CapFlat, Join: JoinMiter, MiterLimit: 10}
	if err := c.Stroke(path, s, Paint{Color: color.NRGBA{A: 255}}, nil); err != nil {
		t.Fatal(err)
	}
	for y, want := range map[int]uint8{8: 255, 9: 0, 10: 0, 11: 255} {
		if got := c.Image.NRGBAAt(10, y).R; got != want {
			t.Fatalf("row %d = %d, want %d", y, got, want)
		}
	}
	if got := c.Image.NRGBAAt(1, 10).R; got != 255 {
		t.Fatal("flat cap extended past the end point", got)
	}
}

func TestNeighborhoodComparisonRejectsDisplacement(t *testing.T) {
	square := func(x float64) *Canvas {
		c := New(32, 32)
		p := Path{Verbs: []Verb{MoveTo, LineTo, LineTo, LineTo, Close}, Points: []Point{{x, 8}, {x + 12, 8}, {x + 12, 20}, {x, 20}}}
		if err := c.Fill(p, NonZero, Paint{Color: color.NRGBA{A: 255}}, nil); err != nil {
			t.Fatal(err)
		}
		return c
	}
	a, b, d := square(8), square(8.5), square(11)
	if err := CompareNeighborhood(a.Image, b.Image, 40, 1, 0); err != nil {
		t.Fatal("half-pixel edge shift rejected:", err)
	}
	if err := CompareNeighborhood(a.Image, d.Image, 40, 1, .01); err == nil {
		t.Fatal("comparison missed a three-pixel displacement")
	}
}

func TestGouraudInterpolatesLinearly(t *testing.T) {
	// Two triangles forming a horizontal gradient from black to white over
	// [0,64): pixel x takes (x+0.5)/64 of white, with no seam on the diagonal.
	c := New(64, 8)
	black, white := color.NRGBA64{0, 0, 0, 0xffff}, color.NRGBA64{0xff00, 0xff00, 0xff00, 0xffff}
	mesh := []Triangle{{[3]Point{{0, 0}, {64, 0}, {64, 8}}, [3]color.NRGBA64{black, white, white}}, {[3]Point{{0, 0}, {64, 8}, {0, 8}}, [3]color.NRGBA64{black, white, black}}}
	if err := c.FillGouraud(mesh, nil); err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 8; y++ {
		for x := 0; x < 64; x++ {
			want := (float64(x) + .5) / 64 * 255
			if got := float64(c.Image.NRGBAAt(x, y).R); math.Abs(got-want) > 1 {
				t.Fatalf("(%d,%d) = %v, want %.1f", x, y, got, want)
			}
		}
	}
}

func TestLinearGradientWraps(t *testing.T) {
	black, white := color.NRGBA{0, 0, 0, 255}, color.NRGBA{255, 255, 255, 255}
	box := Path{Verbs: []Verb{MoveTo, LineTo, LineTo, LineTo, Close}, Points: []Point{{0, 0}, {40, 0}, {40, 1}, {0, 1}}}
	// The parameter is x/16: one gradient period spans 16 pixels.
	for _, c := range []struct {
		wrap Wrap
		want func(t float64) float64
	}{
		{WrapTile, func(t float64) float64 { return t - math.Floor(t) }},
		{WrapFlipY, func(t float64) float64 { return t - math.Floor(t) }},
		{WrapFlipX, func(t float64) float64 { return 1 - math.Abs(math.Mod(t, 2)-1) }},
		{WrapFlipXY, func(t float64) float64 { return 1 - math.Abs(math.Mod(t, 2)-1) }},
		{WrapClamp, func(t float64) float64 { return math.Min(t, 1) }},
	} {
		cv := New(40, 1)
		g := &Gradient{Transform: Matrix{M11: 1. / 16}, Stops: []Stop{{0, black}, {0.5, color.NRGBA{51, 51, 51, 255}}, {1, white}}, Wrap: c.wrap}
		if err := cv.Fill(box, NonZero, Paint{Gradient: g}, nil); err != nil {
			t.Fatal(err)
		}
		for x := 0; x < 40; x++ {
			u := c.want((float64(x) + .5) / 16)
			// Piecewise linear: 0..51 over [0,0.5], 51..255 over [0.5,1].
			want := u * 2 * 51
			if u > .5 {
				want = 51 + (u-.5)*2*204
			}
			if got := float64(cv.Image.NRGBAAt(x, 0).R); math.Abs(got-want) > 1 {
				t.Fatalf("wrap %d: x=%d got %v, want %.1f", c.wrap, x, got, want)
			}
		}
	}
}

func TestPatternWraps(t *testing.T) {
	a, b := color.NRGBA{255, 0, 0, 255}, color.NRGBA{0, 0, 255, 255}
	tile := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	tile.SetNRGBA(0, 0, a)
	tile.SetNRGBA(1, 0, b)
	box := Path{Verbs: []Verb{MoveTo, LineTo, LineTo, LineTo, Close}, Points: []Point{{0, 0}, {6, 0}, {6, 2}, {0, 2}}}
	for _, c := range []struct {
		wrap Wrap
		row0 string // pixel colors on row 0: a, b or - for unpainted
	}{
		{WrapTile, "ababab"},
		{WrapFlipX, "abbaab"},
		{WrapFlipY, "ababab"},
		{WrapClamp, "ab----"},
	} {
		cv := New(6, 2)
		if err := cv.Fill(box, NonZero, Paint{Pattern: tile, PatternTransform: Identity(), Wrap: c.wrap}, nil); err != nil {
			t.Fatal(err)
		}
		for x, ch := range c.row0 {
			want := map[rune]color.NRGBA{'a': a, 'b': b, '-': {255, 255, 255, 255}}[ch]
			if got := cv.Image.NRGBAAt(x, 0); got != want {
				t.Fatalf("wrap %d: x=%d got %v, want %v", c.wrap, x, got, want)
			}
		}
		// Row 1 repeats row 0 vertically except when clamped.
		if got := cv.Image.NRGBAAt(0, 1); (got == a) == (c.wrap == WrapClamp) {
			t.Fatalf("wrap %d: row 1 = %v", c.wrap, got)
		}
	}
}

func TestCurvedStrokeIsSolid(t *testing.T) {
	// A quarter circle of radius 20 as one cubic, stroked 4 wide with miter
	// joins: flattening yields many sub-pixel segments, each covering only a
	// sliver of a pixel, whose union must cover the band fully.
	const k = 0.5522847498
	c := New(40, 40)
	p := Path{Verbs: []Verb{MoveTo, CubicTo}, Points: []Point{{30, 10}, {30, 10 + 20*k}, {10 + 20*k, 30}, {10, 30}}}
	// The arc is centered at (10,10).
	if err := c.Stroke(p, Stroke{Width: 4, Transform: Identity(), Cap: CapFlat, Join: JoinMiter, MiterLimit: 10}, Paint{Color: color.NRGBA{A: 255}}, nil); err != nil {
		t.Fatal(err)
	}
	for a := 0.1; a < math.Pi/2-0.1; a += 0.05 {
		x, y := 10+20*math.Cos(a), 10+20*math.Sin(a)
		if got := c.Image.NRGBAAt(int(x), int(y)).R; got > 8 {
			t.Fatalf("pixel on the arc at %.2f rad (%d,%d) = %d, want black", a, int(x), int(y), got)
		}
	}
}

func TestClipCombinationIsExact(t *testing.T) {
	// A minus B where both share the half-covered top and bottom rows: the
	// excluded part of those rows must stay empty, not half painted.
	rect := func(x0, y0, x1, y1 float64) Path {
		return Path{Verbs: []Verb{MoveTo, LineTo, LineTo, LineTo, Close}, Points: []Point{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}}
	}
	a := &ClipNode{Op: ClipReplace, Area: rect(2.5, 2.5, 10.5, 10.5), Rule: NonZero}
	diff := &ClipNode{Base: a, Op: ClipDifference, Area: rect(6.5, 2.5, 14.5, 10.5), Rule: NonZero}
	c := New(16, 16)
	if err := c.Fill(rect(0, 0, 16, 16), NonZero, Paint{Color: color.NRGBA{A: 255}}, []*ClipNode{diff}); err != nil {
		t.Fatal(err)
	}
	for _, p := range [][2]int{{8, 2}, {8, 10}, {12, 2}} {
		if got := c.Image.NRGBAAt(p[0], p[1]); got.R != 255 {
			t.Errorf("pixel %v = %v, want white", p, got)
		}
	}
	if got := c.Image.NRGBAAt(4, 2).R; got < 120 || got > 135 {
		t.Errorf("half-covered edge of A = %d, want half gray", got)
	}
}

func TestCompoundAndMixedCaps(t *testing.T) {
	line := Path{Verbs: []Verb{MoveTo, LineTo}, Points: []Point{{4, 10}, {30, 10}}}
	black := Paint{Color: color.NRGBA{A: 255}}
	inked := func(c *Canvas, x, y int) bool { return c.Image.NRGBAAt(x, y).R < 128 }
	// Width 10: bands [0,.2] and [.8,1] lie 3 to 5 units from the center,
	// and [.4,.6] covers the center unit on each side.
	c := New(40, 20)
	if err := c.Stroke(line, Stroke{Width: 10, Transform: Identity(), Cap: CapFlat, Join: JoinMiter, MiterLimit: 10, Compound: []float64{0, .2, .4, .6, .8, 1}}, black, nil); err != nil {
		t.Fatal(err)
	}
	for _, p := range []struct {
		y    int
		want bool
	}{{4, false}, {5, true}, {6, true}, {7, false}, {8, false}, {9, true}, {10, true}, {11, false}, {12, false}, {13, true}, {14, true}, {15, false}} {
		if got := inked(c, 15, p.y); got != p.want {
			t.Errorf("compound row %d inked %v, want %v", p.y, got, p.want)
		}
	}
	// A square start cap extends 5 units back; a flat end cap stops.
	c = New(40, 20)
	if err := c.Stroke(line, Stroke{Width: 10, Transform: Identity(), Cap: CapSquare, EndCap: CapFlat, Join: JoinMiter, MiterLimit: 10}, black, nil); err != nil {
		t.Fatal(err)
	}
	if !inked(c, 0, 10) || inked(c, 31, 10) {
		t.Error("mixed caps", inked(c, 0, 10), inked(c, 31, 10))
	}
}
