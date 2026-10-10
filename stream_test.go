package gowemf

import (
	"errors"
	"testing"
)

func wmfDocument(slots uint16, records ...Record) []byte {
	b := make([]byte, 18)
	put16(b, 0, 2)
	put16(b, 2, 9)
	put16(b, 4, 0x300)
	put16(b, 10, slots)
	records = append(append([]Record(nil), records...), testRecord(WMF, 0, 0, nil))
	var max uint32
	for _, r := range records {
		b = append(b, r.Raw...)
		n := uint32(len(r.Raw) / 2)
		if n > max {
			max = n
		}
	}
	put32(b, 6, uint32(len(b)/2))
	put32(b, 12, max)
	return b
}

func TestStreamWMFHandles(t *testing.T) {
	pen := testRecord(WMF, 0x02fa, 0, append(words(0, 1, 0), longs(0)...))
	brush := testRecord(WMF, 0x02fc, 0, append(append(words(0), longs(255)...), words(0)...))
	file := wmfDocument(2, pen, brush, testRecord(WMF, 0x01f0, 0, words(0)), pen, testRecord(WMF, 0x012d, 0, words(0)))
	var ids []uint32
	if _, err := Stream(file, StreamOptions{}, func(c Command) error {
		if c.HasObjectID {
			ids = append(ids, c.ObjectID)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 || ids[0] != 0 || ids[1] != 1 || ids[2] != 0 {
		t.Fatal(ids)
	}
	for name, file := range map[string][]byte{
		"undefined":     wmfDocument(1, testRecord(WMF, 0x012d, 0, words(0))),
		"full":          wmfDocument(1, pen, brush),
		"double delete": wmfDocument(1, pen, testRecord(WMF, 0x01f0, 0, words(0)), testRecord(WMF, 0x01f0, 0, words(0))),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Stream(file, StreamOptions{}, nil); !errors.Is(err, ErrMalformed) {
				t.Fatal(err)
			}
		})
	}
}

func TestStreamSaveRestoreAndPaths(t *testing.T) {
	file := emfFixture(emfRecord(33, nil), emfRecord(33, nil), emfRecord(34, longs(-2)))
	if _, err := Stream(file, StreamOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Stream(file, StreamOptions{MaxSavedStates: 1}, nil); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	for _, records := range [][][]byte{
		{emfRecord(34, longs(-1))},
		{emfRecord(59, nil)},
		{emfRecord(60, nil)},
		{emfRecord(59, nil), emfRecord(59, nil)},
		{emfRecord(62, make([]byte, 16))},
	} {
		if _, err := Stream(emfFixture(records...), StreamOptions{}, nil); !errors.Is(err, ErrMalformed) {
			t.Fatal(err)
		}
	}
	file = emfFixture(emfRecord(59, nil), emfRecord(27, longs(1, 2)), emfRecord(54, longs(3, 4)), emfRecord(60, nil), emfRecord(64, make([]byte, 16)))
	if _, err := Stream(file, StreamOptions{}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestStreamPlusPlaybackSelection(t *testing.T) {
	// The unsupported GDI record is deliberately only fallback data. Native
	// EMF+ playback must not execute it; explicit GDI fallback must encounter it.
	file := emfFixture(plusComment(plusHeader()), emfRecord(0xffff, nil), plusComment(plusRecord(0x4002, nil)))
	if _, err := Stream(file, StreamOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Stream(file, StreamOptions{PreferGDI: true}, nil); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
	file = emfFixture(plusComment(plusHeader(), plusRecord(0x4004, nil)), emfRecord(0xffff, nil), plusComment(plusRecord(0x4002, nil)))
	if _, err := Stream(file, StreamOptions{}, nil); !errors.Is(err, ErrUnsupported) {
		t.Fatal("GetDC data was skipped", err)
	}
	// A reference to a missing pen must fail even if the geometry is valid.
	file = emfFixture(plusComment(plusHeader(), plusRecord(0x400f, floats(1, 2, 3, 4)), plusRecord(0x4002, nil)))
	if _, err := Stream(file, StreamOptions{}, nil); !errors.Is(err, ErrMalformed) {
		t.Fatal(err)
	}
}

func TestStreamAssembledObject(t *testing.T) {
	brush := longs(0, 0, -65536)
	put32(brush, 0, 0xdbc01002)
	first := plusRecord(0x4008, append(longs(12), brush[:8]...))
	put16(first, 2, 0x8102)
	last := plusRecord(0x4008, brush[8:])
	put16(last, 2, 0x0102)
	fill := plusRecord(0x400a, append(longs(2, 1), floats(1, 2, 3, 4)...))
	file := emfFixture(plusComment(plusHeader(), first), plusComment(last, fill, plusRecord(0x4002, nil)))
	objects := 0
	if _, err := Stream(file, StreamOptions{}, func(c Command) error {
		if c.HasObjectID {
			objects++
			if c.ObjectID != 2 || c.Body.(PlusBrush).Color != 0xffff0000 {
				t.Fatal(c)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if objects != 1 {
		t.Fatal(objects)
	}
}

func TestStreamObjectTableBoundaries(t *testing.T) {
	// MS-EMF 3.1.1.1 sizes the table for Handles+1 slots: index zero is
	// reserved and the highest index equal to Handles is structurally valid.
	// Play follows Windows, which ignores that index (TestPlayHandleCountBound).
	file := emfFixture(emfRecord(38, longs(1, 0, 1, 0, 0)), emfRecord(37, longs(1)))
	if _, err := Stream(file, StreamOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	brush := longs(0, 0, -65536)
	put32(brush, 0, 0xdbc01002)
	r := testRecord(EMFPlus, 0x4008, 0x0103, brush)
	file = emfFixture(plusComment(plusHeader(), r.Raw, plusRecord(0x4002, nil)))
	if _, err := Stream(file, StreamOptions{MaxObjects: 3}, nil); !errors.Is(err, ErrLimit) {
		t.Fatal("EMF+ bypassed configured slot limit", err)
	}
	if _, err := Stream(file, StreamOptions{MaxObjects: 4}, nil); err != nil {
		t.Fatal(err)
	}
}

func FuzzStream(f *testing.F) {
	object, draw := effectImageDrawFixture()
	effect := effectRecord(blurGUIDWire, append(floats(1), longs(0)...)).Raw
	f.Add(emfFixture(plusComment(plusHeader(), object, effect, draw, plusRecord(PlusEndOfFileRecord, nil))))
	f.Add(wmfFixture(false))
	f.Add(embeddedEMFFixture(emfFixture(), 31))
	f.Add(wmfDocument(2, wmfPaletteFixture(2), testRecord(WMF, 0x0234, 0, words(0)), testRecord(WMF, 0x001e, 0, nil), wmfPaletteFixture(1), testRecord(WMF, 0x0234, 0, words(1)), testRecord(WMF, 0x0127, 0, words(-1)), testRecord(WMF, 0x0037, 0, append(words(1, 1), []byte{10, 20, 30, 0}...))))
	f.Add(emfFixture())
	f.Add(plusFixture())
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = ExtractEnhancedMetafile(b, Limits{MaxBytes: 1 << 20, MaxRecordBytes: 1 << 18, MaxRecords: 4096})
		_, _ = Stream(b, StreamOptions{Framing: Limits{MaxBytes: 1 << 20, MaxRecordBytes: 1 << 18, MaxRecords: 4096}, Decoding: DecodeLimits{MaxElements: 4096, MaxObjectBytes: 1 << 18}, MaxObjects: 1024, MaxSavedStates: 64}, nil)
	})
}

func BenchmarkStreamCommands(b *testing.B) {
	records := make([]Record, 10000)
	for i := range records {
		records[i] = testRecord(WMF, 0x0214, 0, words(int16(i), int16(-i)))
	}
	file := wmfDocument(0, records...)
	b.SetBytes(int64(len(file)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Stream(file, StreamOptions{}, nil); err != nil {
			b.Fatal(err)
		}
	}
}
