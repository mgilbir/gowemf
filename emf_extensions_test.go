package gowemf

import (
	"bytes"
	"errors"
	"testing"
)

func emfHeaderFixture(size int) []byte {
	base := emfFixture()
	header := make([]byte, size)
	copy(header, base[:88])
	put32(header, 4, uint32(size))
	b := append(header, base[88:]...)
	put32(b, 48, uint32(len(b)))
	return b
}
func TestEMFHeaderExtensions(t *testing.T) {
	b := emfHeaderFixture(112)
	put32(b, 60, 2)
	put32(b, 64, 108)
	put32(b, 96, 1)
	put32(b, 100, 254000)
	put32(b, 104, 127000)
	copy(b[108:], words('x', 0))
	h, err := Walk(b, Limits{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if h.EMF.Extension1 == nil || !h.EMF.Extension1.OpenGL || h.EMF.Micrometers == nil || h.EMF.Micrometers.X != 254000 || !bytes.Equal(h.EMF.Description, words('x', 0)) {
		t.Fatal(h.EMF)
	}
	b = emfHeaderFixture(112)
	put32(b, 60, 12)
	put32(b, 64, 88)
	for i := 88; i < 112; i += 2 {
		put16(b, i, 'z')
	}
	h, err = Walk(b, Limits{}, nil)
	if err != nil || h.EMF.Extension1 != nil || h.EMF.Micrometers != nil {
		t.Fatal("description mistaken for extensions", h, err)
	}
	b = emfHeaderFixture(144)
	put32(b, 88, 40)
	put32(b, 92, 104)
	h, err = Walk(b, Limits{}, nil)
	if err != nil || len(h.EMF.Extension1.PixelFormat) != 40 || h.EMF.Micrometers != nil {
		t.Fatal(h, err)
	}
	put32(b, 92, 0xffffffff)
	if _, err := Walk(b, Limits{}, nil); !errors.Is(err, ErrMalformed) {
		t.Fatal("pixel format offset", err)
	}
}
func TestEMFFontExtensions(t *testing.T) {
	panose := make([]byte, 4+320)
	put32(panose, 0, 1)
	copy(panose[4+92:], words('F', 0))
	copy(panose[4+220:], words('S', 0))
	put32(panose, 4+288, 12)
	copy(panose[4+308:], []byte{2, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	f := mustDecode(t, testRecord(EMF, EMRExtCreateFontIndirectW, 0, panose)).(Font)
	if f.Extended == nil || f.Extended.StyleSize != 12 || len(f.Extended.FullName) != 128 || f.Extended.Panose[9] != 9 {
		t.Fatal(f)
	}
	design := make([]byte, 4+356+8)
	put32(design, 0, 1)
	copy(design[4+284:], words('L', 0))
	put32(design, 4+348, 0x08007664)
	put32(design, 4+352, 2)
	copy(design[4+356:], longs(-10, 20))
	f = mustDecode(t, testRecord(EMF, EMRExtCreateFontIndirectW, 0, design)).(Font)
	if f.Extended.DesignAxes.Len() != 2 || f.Extended.DesignAxes.SignedAt(0) != -10 || len(f.Extended.Script) != 64 {
		t.Fatal(f)
	}
	put32(design, 4+352, 17)
	if _, err := Decode(testRecord(EMF, EMRExtCreateFontIndirectW, 0, design), DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("unbounded font axes", err)
	}
}
