package gowemf

import (
	"errors"
	"testing"
)

func absolutePathFixture(types []byte) []byte {
	b := append(longs(0, int32(len(types)), 0x4000), make([]byte, len(types)*4)...)
	put32(b, 0, 0xdbc01002)
	return append(b, types...)
}

func TestPlusPathTopology(t *testing.T) {
	for _, types := range [][]byte{{}, {0}, {0, 1, 0x81, 0, 3, 3, 0x83}, {0, 0, 0x80}, {0, 0x11, 0x21, 0x81}} {
		if _, err := DecodePlusObject(3, absolutePathFixture(types), DecodeLimits{}); err != nil {
			t.Fatalf("valid path %x: %v", types, err)
		}
	}
	for _, types := range [][]byte{{1}, {0, 2}, {0, 0x41}, {0, 3}, {0, 3, 1, 3}, {0, 3, 0x83, 3}, {0, 0x81, 1}, {0, 3, 3, 0}} {
		if _, err := DecodePlusObject(3, absolutePathFixture(types), DecodeLimits{}); !errors.Is(err, ErrMalformed) {
			t.Fatalf("accepted path %x: %v", types, err)
		}
	}
}

func TestPlusBezierTypeRuns(t *testing.T) {
	b := append(longs(0, 4, 0x800), make([]byte, 8)...)
	put32(b, 0, 0xdbc01002)
	b = append(b, 0x41, 0, 0xc3, 3)
	if _, err := DecodePlusObject(3, b, DecodeLimits{}); err != nil {
		t.Fatal(err)
	}
	b[len(b)-2] = 0x43
	if _, err := DecodePlusObject(3, b, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("Bezier run marked as line", err)
	}
}

// TestPlusPathUndefinedFlags checks that PathPointFlags bits other than R and
// C (MS-EMFPLUS 2.2.1.6) neither reject a path nor change its encoding.
// Writers set 0x2000 in practice.
func TestPlusPathUndefinedFlags(t *testing.T) {
	types := []byte{0, 1, 1, 0x81}
	encodings := map[uint32][]byte{
		0:      cat(fl(10, 10, 90, 10, 90, 90, 10, 90), types),
		0x4000: cat(dwords(10|10<<16, 90|10<<16, 90|90<<16, 10|90<<16), types),
		// EmfPlusPointR: 7-bit 10 and 0, 15-bit +80 and -80; then
		// EmfPlusPathPointTypeRLE runs and padding.
		0x0800: {10, 10, 0x80, 80, 0, 0, 0x80, 80, 0xff, 0xb0, 0, 0x41, 0, 0x42, 1, 0x41, 0x81, 0, 0, 0},
	}
	encodings[0x4800] = encodings[0x0800] // R makes C undefined.
	want := []Point{{10, 10}, {90, 10}, {90, 90}, {10, 90}}
	for flags, body := range encodings {
		for bit := uint32(1); bit != 0; bit <<= 1 {
			if bit&0x4800 != 0 {
				continue
			}
			for _, f := range []uint32{flags, flags | bit} {
				v, err := DecodePlusObject(3, cat(dwords(plusVersion, 4, f), body), DecodeLimits{})
				if err != nil {
					t.Fatalf("flags %#x: %v", f, err)
				}
				p := v.(PlusPath)
				if p.Flags != f || !pointsNear(pathPoints(p.Points), want...) || string(p.Types) != string(types) {
					t.Fatalf("flags %#x: %+v", f, p)
				}
			}
		}
	}
	b, skipped := plusPlay(t, plusScene(96, 64,
		plusObj(0, 3, cat(dwords(plusVersion, 4, 0x2000), encodings[0])),
		plusRec(PlusFillPathRecord, 0x8000, dwords(0xff0000ff))), PlayOptions{})
	if len(skipped) != 0 || len(b.fills) != 1 {
		t.Fatal(skipped, b.fills)
	}
}

func pathPoints(p Points) []Point {
	out := make([]Point, p.Len())
	for i := range out {
		out[i] = p.At(i)
	}
	return out
}

// TestPlayContinuedObjectFinalC plays a path split over two EmfPlusObject
// records that both set C, as writers do; the object ends at its total size.
func TestPlayContinuedObjectFinalC(t *testing.T) {
	path := plusPathObj([]float64{10, 10, 90, 10, 90, 90, 10, 90}, []byte{0, 1, 1, 0x81})
	total := uint32(len(path))
	for _, final := range []uint16{0, 0x8000} {
		last := path[20:]
		if final != 0 {
			last = cat(dwords(total), last)
		}
		b, skipped := plusPlay(t, plusScene(96, 64,
			plusRec(PlusObjectRecord, 0x8000|3<<8, dwords(total), path[:20]),
			plusRec(PlusObjectRecord, final|3<<8, last),
			plusRec(PlusFillPathRecord, 0x8000, dwords(0xff0000ff))), PlayOptions{})
		if len(skipped) != 0 || len(b.fills) != 1 || !pointsNear(b.fills[0].path.Points[:4], Point{10.5, 10.5}, Point{90.5, 10.5}, Point{90.5, 90.5}, Point{10.5, 90.5}) {
			t.Fatal(final, skipped, b.fills)
		}
	}
}
