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
