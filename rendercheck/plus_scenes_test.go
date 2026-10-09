package rendercheck

import (
	"encoding/binary"
	"math"
	"unicode/utf16"

	"github.com/mgilbir/forme/shape"
)

// EMF+ records are written from the MS-EMFPLUS layouts.

func le32u(v ...uint32) []byte {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[4*i:], x)
	}
	return b
}

func plusRec(typ, flags uint16, body ...[]byte) []byte {
	b := join(body...)
	r := join(le16(int16(typ), int16(flags)), le32(int32((12+len(b)+3)&^3), int32(len(b))), b)
	return append(r, make([]byte, (4-len(r)%4)%4)...)
}

// plusDoc wraps each EMF+ record in its own EMR_COMMENT after an EMF+ Only
// header at 96 DPI.
func plusDoc(w, h int32, records ...[]byte) []byte {
	comment := func(r []byte) []byte {
		return emfRec(70, le32(int32(4+len(r))), le32u(0x2b464d45), r)
	}
	all := [][]byte{comment(plusRec(0x4001, 0, le32u(0xdbc01002, 1, 96, 96)))}
	for _, r := range records {
		all = append(all, comment(r))
	}
	all = append(all, comment(plusRec(0x4002, 0)))
	return emfDoc(w, h, all...)
}

// plusFont is an EmfPlusFont object for Liberation Sans.
func plusFont(id uint8, em float64, unit, style uint32) []byte {
	u := utf16.Encode([]rune("Liberation Sans"))
	name := make([]byte, 2*len(u))
	for i, v := range u {
		binary.LittleEndian.PutUint16(name[2*i:], v)
	}
	return plusRec(0x4008, uint16(id)|6<<8, le32u(0xdbc01002), f32(em), le32u(unit, style, 0, uint32(len(u))), name)
}

// plusDriver is EmfPlusDrawDriverString in a solid ARGB color. Glyphs are
// code units with DriverStringOptionsCmapLookup, otherwise glyph indexes.
func plusDriver(font uint8, argb, options uint32, glyphs []uint16, positions []float64, matrix []float64) []byte {
	g := make([]byte, 2*len(glyphs))
	for i, v := range glyphs {
		binary.LittleEndian.PutUint16(g[2*i:], v)
	}
	present, m := uint32(0), []byte(nil)
	if matrix != nil {
		present, m = 1, f32(matrix...)
	}
	return plusRec(0x4036, 0x8000|uint16(font), le32u(argb, options, present, uint32(len(glyphs))), g, f32(positions...), m)
}

// row places n glyph origins dx apart on a baseline.
func row(n int, x, y, dx float64) []float64 {
	out := make([]float64, 0, 2*n)
	for i := 0; i < n; i++ {
		out = append(out, x+float64(i)*dx, y)
	}
	return out
}

func units(s string) []uint16 { return utf16.Encode([]rune(s)) }

func plusTextScenes(face *shape.Face) []textScene {
	const w, h = 192, 96
	glyphs := func(s string) []uint16 {
		var out []uint16
		for _, r := range s {
			g, ok := face.GlyphID(r)
			if !ok {
				panic("reference font lacks " + string(r))
			}
			out = append(out, uint16(g))
		}
		return out
	}
	s, c := math.Sincos(20 * math.Pi / 180)
	return []textScene{
		{name: "plus-driver.emf", data: plusDoc(w, h,
			plusFont(1, 24, 2, 0),
			plusDriver(1, 0xffff0000, 1, units("Hamburg"), row(7, 6, 30, 15), nil),
			plusDriver(1, 0xff0000ff, 1, units("Center"), row(6, 40, 80, 16), nil),
		),
			// The H stems rise from baseline 30 at x 6; nothing is above y 8.
			probes: []ink{{7, 14, 11, 29, true}, {0, 0, 192, 8, false}, {41, 62, 56, 80, true}}},
		// The G of the glyph-index string spans x 40-57 above baseline 80.
		{name: "lo-plus-driver-glyphs.emf", divergence: "EmfPlusDrawDriverString with glyph indexes (no DriverStringOptionsCmapLookup) draws nothing",
			data:   plusDoc(w, h, plusFont(1, 24, 2, 0), plusDriver(1, 0xff0000ff, 0, glyphs("Glyphs"), row(6, 40, 80, 16), nil)),
			probes: []ink{{41, 62, 56, 80, true}}, libreOffice: []ink{{41, 62, 56, 80, false}}},
		// 18 points at 96 DPI is 24 pixels; the translation matrix applies
		// in world space, before the rotated world transform.
		{name: "plus-driver-world.emf", data: plusDoc(w, h,
			plusRec(0x402a, 0, f32(c, s, -s, c, 60, 10)),
			plusFont(1, 18, 3, 0),
			plusDriver(1, 0xff00a000, 1, units("Turn"), row(4, 0, 0, 16), []float64{1, 0, 0, 1, 0, 30}),
		),
			// The T stem at world (6, 22) lands near (58.1, 32.7); without the
			// matrix the run would sit 30 units higher.
			probes: []ink{{55, 29, 62, 36, true}}},
	}
}
