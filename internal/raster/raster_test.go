package raster

import (
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
