package gowemf

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
)

func put16(b []byte, off int, v uint16) { binary.LittleEndian.PutUint16(b[off:], v) }
func put32(b []byte, off int, v uint32) { binary.LittleEndian.PutUint32(b[off:], v) }

// Fixtures are constructed here from the specifications, not external files.
func wmfFixture(placeable bool) []byte {
	b := make([]byte, 34)
	put16(b, 0, 2)
	put16(b, 2, 9)
	put16(b, 4, 0x300)
	put32(b, 6, 17)
	put32(b, 12, 5)
	put32(b, 18, 5)
	put16(b, 22, 0x214) // META_MOVETO: y, x.
	put16(b, 24, 20)
	put16(b, 26, 10)
	put32(b, 28, 3)
	if !placeable {
		return b
	}
	h := make([]byte, 22)
	put32(h, 0, 0x9ac6cdd7)
	put16(h, 6, 0xfff6)
	put16(h, 10, 100)
	put16(h, 12, 200)
	put16(h, 14, 1440)
	var sum uint16
	for i := 0; i < 20; i += 2 {
		sum ^= u16(h[i:])
	}
	put16(h, 20, sum)
	return append(h, b...)
}

func emfRecord(typ uint32, body []byte) []byte {
	b := make([]byte, (8+len(body)+3)&^3)
	put32(b, 0, typ)
	put32(b, 4, uint32(len(b)))
	copy(b[8:], body)
	return b
}

// plusRecord pads the body: DataSize is 32-bit aligned (MS-EMFPLUS 2.3), and
// GDI+ abandons a file at a record whose DataSize is not.
func plusRecord(typ uint16, body []byte) []byte {
	b := make([]byte, (12+len(body)+3)&^3)
	put16(b, 0, typ)
	put32(b, 4, uint32(len(b)))
	put32(b, 8, uint32(len(b)-12))
	copy(b[12:], body)
	return b
}

func plusComment(records ...[]byte) []byte {
	body := make([]byte, 8)
	put32(body, 4, 0x2b464d45)
	for _, r := range records {
		body = append(body, r...)
	}
	put32(body, 0, uint32(len(body)-4))
	return emfRecord(70, body)
}

func plusHeader() []byte {
	b := make([]byte, 16)
	put32(b, 0, 0xdbc01002)
	put32(b, 4, 1)
	put32(b, 8, 96)
	put32(b, 12, 120)
	r := plusRecord(0x4001, b)
	put16(r, 2, 1)
	return r
}

func emfFixture(records ...[]byte) []byte {
	b := emfRecord(1, make([]byte, 80))
	put32(b, 16, 100)
	put32(b, 20, 200)
	put32(b, 40, 0x464d4520)
	put32(b, 44, 0x10000)
	put16(b, 56, 1)
	put32(b, 52, uint32(len(records)+2))
	for _, r := range records {
		b = append(b, r...)
	}
	eof := emfRecord(14, make([]byte, 12))
	put32(eof, 16, 20)
	b = append(b, eof...)
	put32(b, 48, uint32(len(b)))
	return b
}

func plusFixture() []byte {
	return emfFixture(plusComment(plusHeader()), plusComment(plusRecord(0x4002, nil)))
}

func TestWalk(t *testing.T) {
	for _, placeable := range []bool{false, true} {
		b := wmfFixture(placeable)
		var records []Record
		h, err := Walk(b, Limits{}, func(r Record) error { records = append(records, r); return nil })
		if err != nil {
			t.Fatal(err)
		}
		start := 18
		if placeable {
			start += 22
			if h.Placeable.Bounds.Left != -10 || h.Placeable.UnitsPerInch != 1440 {
				t.Fatal(h.Placeable)
			}
		}
		if h.Format != WMF || h.WMF.SizeWords != 17 || len(records) != 2 {
			t.Fatal(h, records)
		}
		r := records[0]
		if r.Type != 0x214 || r.Offset != start || r.ParentOffset != -1 || !bytes.Equal(r.Data, []byte{20, 0, 10, 0}) {
			t.Fatal(r)
		}
		if cap(r.Raw) != len(r.Raw) || cap(r.Data) != len(r.Data) || &r.Raw[0] != &b[start] {
			t.Fatal("record must be capacity-limited and zero-copy")
		}
	}
	var records []Record
	h, err := Walk(plusFixture(), Limits{}, func(r Record) error { records = append(records, r); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if h.Format != EMF || h.EMF.Records != 4 || h.EMF.Bounds.Right != 100 || !h.EMFPlus.Dual || h.EMFPlus.LogicalDpiY != 120 {
		t.Fatalf("bad metadata: %+v", h)
	}
	if len(records) != 6 || records[2].Format != EMFPlus || records[2].Offset != 104 || records[2].ParentOffset != 88 || records[4].Type != 0x4002 {
		t.Fatal(records)
	}
}

func TestTruncation(t *testing.T) {
	for name, b := range map[string][]byte{"wmf": wmfFixture(false), "placeable": wmfFixture(true), "emf": emfFixture(), "plus": plusFixture()} {
		t.Run(name, func(t *testing.T) {
			for i := 0; i < len(b); i++ {
				if _, err := Walk(b[:i], Limits{}, nil); err == nil {
					t.Fatalf("accepted truncation at %d", i)
				}
			}
			if _, err := Walk(append(b, 0), Limits{}, nil); err == nil {
				t.Fatal("accepted trailing byte")
			}
		})
	}
}

func TestMalformed(t *testing.T) {
	cases := []struct {
		name  string
		input func() []byte
		off   int
		value uint32
	}{
		{"wmf zero record", func() []byte { return wmfFixture(false) }, 18, 0},
		{"wmf overflowing words", func() []byte { return wmfFixture(false) }, 18, 0xffffffff},
		{"wmf max record", func() []byte { return wmfFixture(false) }, 12, 4},
		{"wmf missing EOF", func() []byte { return wmfFixture(false) }, 30, 0x12340000},
		{"placeable checksum", func() []byte { return wmfFixture(true) }, 6, 42},
		{"emf record count", func() []byte { return emfFixture() }, 52, 3},
		{"emf signature", func() []byte { return emfFixture() }, 40, 0},
		{"emf short header", func() []byte { return emfFixture() }, 4, 84},
		{"emf unaligned record", func() []byte { return emfFixture() }, 92, 19},
		{"emf SizeLast", func() []byte { return emfFixture() }, 104, 19},
		{"emf description overflow", func() []byte { return emfFixture() }, 60, 0xffffffff},
		{"emf palette count", func() []byte { return emfFixture() }, 96, 1},
		{"comment size", plusFixture, 96, 0xffffffff},
		{"plus record size", plusFixture, 108, 0xfffffffc},
		{"plus data size", plusFixture, 112, 0xffffffff},
		{"plus graphics signature", plusFixture, 116, 0},
		{"plus missing header", plusFixture, 104, 0x4003},
		{"plus missing EOF", plusFixture, 148, 0x4003},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			b := tt.input()
			put32(b, tt.off, tt.value)
			_, err := Walk(b, Limits{}, nil)
			var pe *ParseError
			if !errors.As(err, &pe) || (!errors.Is(err, ErrMalformed) && !errors.Is(err, ErrLimit)) {
				t.Fatalf("got %v", err)
			}
			if pe.Offset < 0 || pe.Offset > len(b) {
				t.Fatal(pe)
			}
		})
	}
}

func TestStructure(t *testing.T) {
	for name, b := range map[string][]byte{
		"duplicate EMF header":  emfFixture(emfRecord(1, nil)),
		"short comment":         emfFixture(emfRecord(70, nil)),
		"empty plus comment":    emfFixture(plusComment()),
		"late plus header":      emfFixture(emfRecord(99, nil), plusComment(plusHeader(), plusRecord(0x4002, nil))),
		"duplicate plus header": emfFixture(plusComment(plusHeader(), plusHeader(), plusRecord(0x4002, nil))),
		"after plus EOF":        emfFixture(plusComment(plusHeader(), plusRecord(0x4002, nil), plusRecord(0x4003, nil))),
		"missing plus EOF":      emfFixture(plusComment(plusHeader())),
		"short plus header":     emfFixture(plusComment(plusRecord(0x4001, nil))),
		"long plus EOF":         emfFixture(plusComment(plusHeader(), plusRecord(0x4002, make([]byte, 4)))),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Walk(b, Limits{}, nil); !errors.Is(err, ErrMalformed) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestOpaqueAndPadding(t *testing.T) {
	unknown := plusRecord(0x7fff, []byte{42})
	put32(unknown, 8, 1)           // an unaligned DataSize is tolerated
	unknown[len(unknown)-1] = 0xff // Padding is ignored, not required to be zero.
	b := emfFixture(plusComment(plusHeader(), unknown, plusRecord(0x4002, nil)), emfRecord(0xffffffff, []byte{1, 2, 3, 4}))
	seen := 0
	_, err := Walk(b, Limits{}, func(r Record) error {
		if r.Type == 0x7fff {
			seen++
			if !bytes.Equal(r.Data, []byte{42}) {
				t.Fatal(r)
			}
		}
		if r.Type == 0xffffffff {
			seen++
		}
		return nil
	})
	if err != nil || seen != 2 {
		t.Fatal(seen, err)
	}
}

func TestLimitsAndVisitor(t *testing.T) {
	b := plusFixture()
	for _, l := range []Limits{{MaxBytes: uint64(len(b) - 1)}, {MaxRecordBytes: 87}, {MaxRecords: 5}} {
		if _, err := Walk(b, l, nil); !errors.Is(err, ErrLimit) {
			t.Fatalf("%+v: %v", l, err)
		}
	}
	if _, err := Walk(b, Limits{MaxBytes: uint64(len(b)), MaxRecordBytes: 88, MaxRecords: 6}, nil); err != nil {
		t.Fatal(err)
	}
	stop := errors.New("visitor stop")
	calls := 0
	_, err := Walk(b, Limits{}, func(Record) error { calls++; return stop })
	if err != stop || calls != 1 {
		t.Fatal(calls, err)
	}
	if _, err := Walk([]byte("not a metafile"), Limits{}, nil); !errors.Is(err, ErrFormat) {
		t.Fatal(err)
	}
}

func FuzzWalk(f *testing.F) {
	for _, b := range [][]byte{wmfFixture(false), wmfFixture(true), emfFixture(), plusFixture()} {
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		_, err := Walk(b, Limits{MaxBytes: 1 << 20, MaxRecordBytes: 1 << 18, MaxRecords: 4096}, func(r Record) error {
			if r.Offset < 0 || r.Offset > len(b) || len(r.Raw) > len(b)-r.Offset || len(r.Data) > len(r.Raw) {
				t.Fatal("record outside input")
			}
			if !bytes.Equal(r.Raw, b[r.Offset:r.Offset+len(r.Raw)]) {
				t.Fatal("record provenance")
			}
			return nil
		})
		if err != nil {
			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("unexpected error: %v", err)
			}
		}
	})
}

func BenchmarkWalk(b *testing.B) {
	for name, data := range map[string][]byte{"WMF": wmfFixture(false), "EMF+": plusFixture()} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			for i := 0; i < b.N; i++ {
				if _, err := Walk(data, Limits{}, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func ExampleWalk() {
	// Application code obtains data from an embedded document image.
	data := wmfFixture(false)
	_, err := Walk(data, Limits{}, func(r Record) error {
		fmt.Printf("type=%04x offset=%d bytes=%d\n", r.Type, r.Offset, len(r.Raw))
		return nil
	})
	if err != nil {
		fmt.Println(err)
	}
	// Output:
	// type=0214 offset=18 bytes=10
	// type=0000 offset=28 bytes=6
}
