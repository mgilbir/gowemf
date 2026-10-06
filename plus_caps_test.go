package gowemf

import (
	"errors"
	"math"
	"testing"
)

func arrowCapFixture() []byte {
	b := append(longs(0, 1), floats(3, 5, .25)...)
	put32(b, 0, 0xdbc01002)
	b = append(b, longs(1, 0, 2, 1)...)
	return append(b, floats(10, 2, 0, 0, 0, 0)...)
}

func TestPlusAdjustableArrowCap(t *testing.T) {
	v, err := DecodePlusObject(9, arrowCapFixture(), DecodeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	cap := v.(PlusCustomLineCap)
	if cap.Type != 1 || cap.Default != nil || cap.Arrow.Width != 3 || cap.Arrow.Height != 5 || cap.Arrow.MiddleInset != .25 || !cap.Arrow.Filled || cap.Arrow.LineEndCap != 2 || cap.Arrow.LineJoin != 1 || cap.Arrow.WidthScale != 2 {
		t.Fatal(cap)
	}
}

func TestPlusCustomCapPenEnvelope(t *testing.T) {
	cap := arrowCapFixture()
	pen := penWithCapsFixture(cap, cap)
	v, err := DecodePlusObject(2, pen, DecodeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	p := v.(PlusPen)
	if p.CustomStartCap == nil || p.CustomEndCap == nil || p.CustomStartCap.Arrow.Width != 3 || p.CustomEndCap.Arrow.Height != 5 || p.Brush.Color != 0xffff0000 {
		t.Fatal(p)
	}
	put32(pen, 20, 0xffffffff)
	if _, err := DecodePlusObject(2, pen, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("unbounded custom-cap span", err)
	}
}

func penWithCapsFixture(start, end []byte) []byte {
	flags := int32(0)
	if start != nil {
		flags |= 0x800
	}
	if end != nil {
		flags |= 0x1000
	}
	pen := append(longs(0, 0, flags, 2), floats(1)...)
	put32(pen, 0, 0xdbc01002)
	for _, cap := range [][]byte{start, end} {
		if cap != nil {
			pen = append(pen, longs(int32(len(cap)))...)
			pen = append(pen, cap...)
		}
	}
	brush := longs(0, 0, -65536)
	put32(brush, 0, 0xdbc01002)
	pen = append(pen, brush...)
	return pen
}

func relativePathFixture(n int) []byte {
	b := append(longs(0, int32(n), 0x800), make([]byte, n*2)...)
	put32(b, 0, 0xdbc01002)
	return append(b, 0x41, 0, byte(0x40|(n-1)), 1)
}

func defaultCapFixture(fill, stroke []byte) []byte {
	flags := int32(0)
	if fill != nil {
		flags |= 1
	}
	if stroke != nil {
		flags |= 2
	}
	b := append(longs(0, 0, flags, 0), floats(.5)...)
	put32(b, 0, 0xdbc01002)
	b = append(b, longs(1, 2, 3)...)
	b = append(b, floats(10, 2, 0, 0, 0, 0)...)
	for _, path := range [][]byte{fill, stroke} {
		if path != nil {
			b = append(b, longs(int32(len(path)))...)
			b = append(b, path...)
		}
	}
	return b
}

func TestPlusDefaultCapsAndSharedBudget(t *testing.T) {
	path := relativePathFixture(10)
	cap := defaultCapFixture(path, path)
	v, err := DecodePlusObject(9, cap, DecodeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	d := v.(PlusCustomLineCap).Default
	if d.Flags != 3 || d.BaseInset != .5 || d.StrokeStartCap != 1 || d.StrokeEndCap != 2 || d.StrokeJoin != 3 || d.FillPath.Points.Len() != 10 || d.StrokePath.Types[9] != 1 {
		t.Fatal(d)
	}
	// Each expanded path costs 170 bytes; the 136-byte cap fits the wire
	// budget, but two nested decoders must share the 300-byte expansion budget.
	if _, err := DecodePlusObject(9, cap, DecodeLimits{MaxObjectBytes: 300}); !errors.Is(err, ErrLimit) {
		t.Fatal("nested cap paths reset allocation budget", err)
	}
	if _, err := DecodePlusObject(9, cap, DecodeLimits{MaxObjectBytes: 340}); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodePlusObject(9, cap, DecodeLimits{MaxNesting: 1}); !errors.Is(err, ErrLimit) {
		t.Fatal("nested cap path depth", err)
	}
	pen := append(longs(0, 0, 0x800, 2), floats(1)...)
	put32(pen, 0, 0xdbc01002)
	pen = append(pen, longs(int32(len(cap)))...)
	pen = append(pen, cap...)
	brush := longs(0, 0, 0)
	put32(brush, 0, 0xdbc01002)
	pen = append(pen, brush...)
	if _, err := DecodePlusObject(2, pen, DecodeLimits{MaxNesting: 2}); !errors.Is(err, ErrLimit) {
		t.Fatal("pen/cap/path nesting", err)
	}
	if _, err := DecodePlusObject(2, pen, DecodeLimits{MaxNesting: 3}); err != nil {
		t.Fatal(err)
	}
	put32(cap, 56, 0xffffffff)
	if _, err := DecodePlusObject(9, cap, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("negative cap path length", err)
	}
}

func TestPlusCapTruncationAndFloats(t *testing.T) {
	cap := arrowCapFixture()
	for n := 0; n < len(cap); n++ {
		if _, err := DecodePlusObject(9, cap[:n], DecodeLimits{}); !errors.Is(err, ErrMalformed) {
			t.Fatalf("accepted cap prefix %d: %v", n, err)
		}
	}
	put32(cap, 8, math.Float32bits(float32(math.NaN())))
	if _, err := DecodePlusObject(9, cap, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("non-finite cap width", err)
	}
	cap = arrowCapFixture()
	put32(cap, 44, math.Float32bits(1))
	if _, err := DecodePlusObject(9, cap, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("nonzero reserved cap hotspot", err)
	}
	cap = arrowCapFixture()
	put32(cap, 24, 5)
	if _, err := DecodePlusObject(9, cap, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("unknown line-cap style", err)
	}
}

func TestNestedTextureObjectDepth(t *testing.T) {
	imageData := rawPlusImageObject(PlusImage{Type: 1, Width: 1, Height: 1, Stride: 4, PixelFormat: PixelFormat32bppARGB, Data: []byte{1, 2, 3, 255}})
	brush := append(longs(0, 2, 0, 0), imageData...)
	put32(brush, 0, 0xdbc01002)
	pen := append(longs(0, 0, 0, 2), floats(1)...)
	put32(pen, 0, 0xdbc01002)
	pen = append(pen, brush...)
	if _, err := DecodePlusObject(2, pen, DecodeLimits{MaxNesting: 2}); !errors.Is(err, ErrLimit) {
		t.Fatal("pen/brush/image nesting was not counted", err)
	}
	if _, err := DecodePlusObject(2, pen, DecodeLimits{MaxNesting: 3}); err != nil {
		t.Fatal(err)
	}
}

func TestNestedCapPathErrorOffset(t *testing.T) {
	path := absolutePathFixture([]byte{0, 1})
	put32(path, 0, 0)
	_, err := DecodePlusObject(9, defaultCapFixture(path, nil), DecodeLimits{})
	var pe *ParseError
	if !errors.As(err, &pe) || pe.Offset != 64 || !errors.Is(err, ErrMalformed) {
		t.Fatal("nested error did not retain outer byte offset", err)
	}
}
