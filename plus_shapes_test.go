package gowemf

import (
	"errors"
	"math"
	"testing"
)

func TestPlusRegionTree(t *testing.T) {
	b := append(longs(0, 2, 2, 0x10000000), floats(1, 2, 3, 4)...)
	b = append(b, longs(0x10000003)...)
	put32(b, 0, 0xdbc01002)
	v, err := DecodePlusObject(4, b, DecodeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	nodes := v.(PlusRegion).Nodes
	if len(nodes) != 3 || nodes[0].Left != 1 || nodes[0].Right != 2 || nodes[1].Rect != (Box{1, 2, 3, 4}) || nodes[2].Type != 0x10000003 {
		t.Fatal(nodes)
	}
	if _, err := DecodePlusObject(4, b, DecodeLimits{MaxNesting: 1}); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	if _, err := DecodePlusObject(4, b, DecodeLimits{MaxObjectBytes: 512}); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	put32(b, 4, 0)
	if _, err := DecodePlusObject(4, b, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("unbounded child nodes", err)
	}
	put32(b, 4, 0xffffffff)
	if _, err := DecodePlusObject(4, b, DecodeLimits{}); !errors.Is(err, ErrLimit) {
		t.Fatal("wrapped child count", err)
	}
}

func gradientBody(flags int32) []byte {
	b := append(longs(0, 4, flags, 0), floats(0, 0, 100, 200)...)
	put32(b, 0, 0xdbc01002)
	return append(b, longs(-65536, -16711936, 0, 0)...)
}

func TestPlusGradientPatterns(t *testing.T) {
	b := append(gradientBody(4), longs(2)...)
	b = append(b, floats(0, 1)...)
	b = append(b, longs(-65536, -16711936)...)
	v, err := DecodePlusObject(1, b, DecodeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	g := v.(PlusBrush).LinearGradient
	if g.Rect != (Box{0, 0, 100, 200}) || g.PresetPositions[1] != 1 || g.PresetColors.At(0) != 0xffff0000 || g.Transform != Identity() {
		t.Fatal(g)
	}
	put32(b, 52, math.Float32bits(-0.5))
	if _, err := DecodePlusObject(1, b, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("negative gradient position", err)
	}
	b = gradientBody(0x18)
	for _, f := range [][]float32{{0, 1, 1, 0}, {0, 1, 0, 1}} {
		b = append(b, longs(2)...)
		b = append(b, floats(f...)...)
	}
	v, err = DecodePlusObject(1, b, DecodeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	g = v.(PlusBrush).LinearGradient
	if g.Vertical.Factors[0] != 1 || g.Horizontal.Factors[0] != 0 {
		t.Fatal("swapped blend axes", g)
	}
}

func TestDecodedAllocationBudget(t *testing.T) {
	// Two eight-element float arrays occupy 128 decoded bytes, although their
	// wire bytes and surrounding linear-gradient header fit in 116 bytes.
	b := append(gradientBody(8), longs(8)...)
	b = append(b, floats(0, .1, .2, .3, .4, .5, .6, 1)...)
	b = append(b, floats(0, 0, 0, 0, 1, 1, 1, 1)...)
	if _, err := DecodePlusObject(1, b, DecodeLimits{MaxObjectBytes: 120}); !errors.Is(err, ErrLimit) {
		t.Fatal("aggregate decoded float budget", err)
	}
}

func TestPlusReservedDrawingFlags(t *testing.T) {
	r := testRecord(EMFPlus, 0x4019, 0xa000, append(longs(4), floats(0, 0, 1, 1, 2, 1, 3, 0)...))
	p := mustDecode(t, r).(PlusPoly)
	if p.Closed || p.Solid {
		t.Fatal("Bezier reserved flags changed drawing properties", p)
	}
	r = testRecord(EMFPlus, 0x400b, 0x8000, append(longs(1), floats(0, 0, 1, 1)...))
	if p := mustDecode(t, r).(PlusRects); p.Solid {
		t.Fatal("DrawRects reserved solid flag", p)
	}
	r = testRecord(EMFPlus, 0x402a, 0x2000, floats(1, 0, 0, 1, 0, 0))
	if tr := mustDecode(t, r).(Transform); tr.Mode != 0 {
		t.Fatal("SetWorldTransform reserved order flag", tr)
	}
}

func TestPlusCardinalCurves(t *testing.T) {
	b := append(append(longs(-16776961), floats(.5)...), longs(3)...)
	b = append(b, 0, 0, 10, 20, 10, 0x76)
	v := mustDecode(t, testRecord(EMFPlus, 0x4016, 0xe800, b)).(PlusCurve)
	if !v.Solid || !v.Winding || !v.Closed || v.Tension != .5 || v.Points.At(2) != (Point{20, 10}) {
		t.Fatal(v)
	}
	b = append(append(floats(.5), longs(0, 1, 2)...), floats(1, 2, 3, 4)...)
	v = mustDecode(t, testRecord(EMFPlus, 0x4018, 0x0802, b)).(PlusCurve)
	if v.Closed || v.ObjectID != 2 || v.Segments != 1 || v.Points.At(1) != (Point{3, 4}) {
		t.Fatal(v)
	}
	put32(b, 4, 0xffffffff)
	if _, err := Decode(testRecord(EMFPlus, 0x4018, 2, b), DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("wrapped spline range", err)
	}
}

func driverStringFixture(realized, full bool) Record {
	options := int32(0)
	if realized {
		options = 4
	}
	b := append(longs(-65536, options, 1, 3), words(10, 20, 30)...)
	b = append(b, floats(1, 2)...)
	if !realized {
		b = append(b, floats(3, 4, 5, 6)...)
	} else if full {
		b = append(b, floats(float32(math.NaN()), float32(math.Inf(1)), float32(math.NaN()), 0)...)
	}
	b = append(b, floats(1, 0, 0, 1, 7, 8)...)
	return testRecord(EMFPlus, 0x4036, 0x8003, b)
}

func TestPlusDriverString(t *testing.T) {
	for _, tc := range []struct{ realized, full bool }{{false, true}, {true, false}, {true, true}} {
		v := mustDecode(t, driverStringFixture(tc.realized, tc.full)).(PlusDriverString)
		n := 3
		if tc.realized {
			n = 1
		}
		stored := uint32(3)
		if tc.realized && !tc.full {
			stored = 1
		}
		if v.FontID != 3 || v.BrushID != 0xffff0000 || !v.Solid || v.Glyphs.Len() != 3 || v.Glyphs.At(2) != 30 || v.Positions.Len() != n || v.StoredPositions != stored || v.Positions.At(0) != (Point{1, 2}) || v.Matrix.Dx != 7 || v.Matrix.Dy != 8 {
			t.Fatal(tc, v)
		}
	}
}

func TestPlusParameterizedContainer(t *testing.T) {
	b := append(floats(0, 0, 100, 100, 1, 2, 3, 4), longs(17)...)
	r := testRecord(EMFPlus, 0x4027, 0x0200, b)
	v := mustDecode(t, r).(PlusContainer)
	if v.Unit != 2 || v.Source != (Box{1, 2, 3, 4}) || v.StackIndex != 17 {
		t.Fatal(v)
	}
	file := emfFixture(plusComment(plusHeader(), r.Raw, plusRecord(0x4029, longs(17)), plusRecord(0x4002, nil)))
	if _, err := Stream(file, StreamOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	file = emfFixture(plusComment(plusHeader(), r.Raw, plusRecord(0x4029, longs(18)), plusRecord(0x4002, nil)))
	if _, err := Stream(file, StreamOptions{}, nil); !errors.Is(err, ErrMalformed) {
		t.Fatal("unmatched container index", err)
	}
}
