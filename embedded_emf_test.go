package gowemf

import (
	"bytes"
	"errors"
	"testing"
)

func embeddedEMFFixture(data []byte, split int) []byte {
	var sum uint16
	for off := 0; off < len(data); off += 2 {
		sum ^= u16(data[off:])
	}
	fragment := func(chunk []byte, remaining int, checksum uint16) Record {
		b := append(words(15, int16(34+len(chunk))), longs(0x43464d57, 1, 0x10000)...)
		b = append(b, words(int16(checksum))...)
		b = append(b, longs(0, 2, int32(len(chunk)), int32(remaining), int32(len(data)))...)
		b = append(b, chunk...)
		return testRecord(WMF, MetaEscape, 0, b)
	}
	return wmfDocument(0, fragment(data[:split], len(data)-split, ^sum), fragment(data[split:], 0, 0))
}

func TestExtractEnhancedMetafile(t *testing.T) {
	emf := emfFixture()
	wmf := embeddedEMFFixture(emf, 31) // fragments need not end on a WORD boundary
	out, err := ExtractEnhancedMetafile(wmf, Limits{})
	if err != nil || !bytes.Equal(out, emf) {
		t.Fatal(err)
	}
	out[0] = 2
	again, err := ExtractEnhancedMetafile(wmf, Limits{})
	if err != nil || again[0] != 1 {
		t.Fatal("assembled data aliases input", err)
	}
	if got, err := ExtractEnhancedMetafile(wmfFixture(false), Limits{}); err != nil || got != nil {
		t.Fatal(got, err)
	}
	for _, off := range []int{18 + 22, 18 + 28, 18 + 36, 18 + 40} {
		bad := append([]byte(nil), wmf...)
		bad[off] ^= 1
		if _, err := ExtractEnhancedMetafile(bad, Limits{}); !errors.Is(err, ErrMalformed) {
			t.Fatalf("accepted bad envelope at %d: %v", off, err)
		}
	}
	if _, err := ExtractEnhancedMetafile(wmf, Limits{MaxBytes: uint64(len(wmf) - 1)}); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
}

// TestExtractEnhancedMetafilePrivateComments interleaves private MFCOMMENT
// records and other escapes with the WMFC fragments.
func TestExtractEnhancedMetafilePrivateComments(t *testing.T) {
	emf := emfFixture()
	plain := embeddedEMFFixture(emf, 31)
	var fragments []Record
	if _, err := Walk(plain, Limits{}, func(r Record) error {
		if r.Type == MetaEscape {
			fragments = append(fragments, r)
		}
		return nil
	}); err != nil || len(fragments) != 2 {
		t.Fatal(err, len(fragments))
	}
	comment := wmfEscape(15, []byte("MathType"))
	wmf := wmfDocument(0, comment, fragments[0], wmfEscape(38, []byte{1, 0}), comment, fragments[1], comment)
	out, err := ExtractEnhancedMetafile(wmf, Limits{})
	if err != nil || !bytes.Equal(out, emf) {
		t.Fatal(err)
	}
	if got, err := ExtractEnhancedMetafile(wmfDocument(0, comment), Limits{}); err != nil || got != nil {
		t.Fatal(got, err)
	}
}
