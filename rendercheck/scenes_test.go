package rendercheck

import (
	"encoding/binary"
	"math"
	"unicode/utf16"
)

// Scene files are generated here from the MS-EMF/MS-WMF layouts; no external
// document is used.

func le32(v ...int32) []byte {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[4*i:], uint32(x))
	}
	return b
}
func le16(v ...int16) []byte {
	b := make([]byte, 2*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint16(b[2*i:], uint16(x))
	}
	return b
}
func f32(v ...float64) []byte {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(float32(x)))
	}
	return b
}
func join(parts ...[]byte) []byte {
	var b []byte
	for _, p := range parts {
		b = append(b, p...)
	}
	return b
}

func emfRec(typ uint32, body ...[]byte) []byte {
	b := join(body...)
	r := join(le32(int32(typ), int32((8+len(b)+3)&^3)), b)
	return append(r, make([]byte, (4-len(r)%4)%4)...)
}

// emfDoc writes a header whose reference device has 0.25 mm pixels and whose
// frame covers w x h of them, so device pixels are destination pixels.
func emfDoc(w, h int32, records ...[]byte) []byte {
	header := emfRec(1, le32(0, 0, w-1, h-1, 0, 0, w*25, h*25, 0x464d4520, 0x10000, 0, 0, 16, 0, 0, 0, 1000, 1000, 250, 250))
	out := header
	for _, r := range records {
		out = append(out, r...)
	}
	out = append(out, emfRec(14, le32(0, 16, 20))...)
	binary.LittleEndian.PutUint32(out[48:], uint32(len(out)))
	binary.LittleEndian.PutUint32(out[52:], uint32(len(records)+2))
	return out
}

const (
	cRed   = 0x0000ff
	cBlue  = 0xff0000
	cGreen = 0x00a000
	cBlack = 0
	cYell  = 0x00e0ff
)

func emfSelect(id uint32) []byte             { return emfRec(37, le32(int32(id))) }
func emfValue(typ uint32, v int32) []byte    { return emfRec(typ, le32(v)) }
func emfPoint(typ uint32, x, y int32) []byte { return emfRec(typ, le32(x, y)) }

// emfFont is EMR_EXTCREATEFONTINDIRECTW for Liberation Sans.
func emfFont(id uint32, height, escapement int32, underline, strike byte) []byte {
	name := make([]byte, 64)
	for i, u := range utf16.Encode([]rune("Liberation Sans")) {
		binary.LittleEndian.PutUint16(name[2*i:], u)
	}
	return emfRec(82, le32(int32(id), height, 0, escapement, escapement, 400), []byte{0, underline, strike, 0, 0, 0, 0, 0}, name)
}

// emfText is EMR_EXTTEXTOUTW with advances. Like Windows writers it always
// records the rectangle, zero when unused; noRectText sets ETO_NO_RECT.
func emfText(mode int32, x, y int32, options uint32, rect []int32, text string, dx []int32) []byte {
	if rect == nil {
		rect = []int32{0, 0, 0, 0}
	}
	return textRecord(mode, x, y, options, rect, text, dx)
}

func noRectText(mode int32, x, y int32, text string, dx []int32) []byte {
	return textRecord(mode, x, y, 0, nil, text, dx)
}

func textRecord(mode int32, x, y int32, options uint32, rect []int32, text string, dx []int32) []byte {
	u := utf16.Encode([]rune(text))
	str := make([]byte, 2*len(u))
	for i, v := range u {
		binary.LittleEndian.PutUint16(str[2*i:], v)
	}
	str = append(str, make([]byte, (4-len(str)%4)%4)...)
	fixed := 8 + 16 + 4 + 8 + 8 + 12 + 4
	if rect != nil {
		fixed += 16
	} else {
		options |= 0x100
	}
	offDx := 0
	if dx != nil {
		offDx = fixed + len(str)
	}
	body := join(le32(0, 0, 0, 0, mode), f32(1, 1), le32(x, y, int32(len(u)), int32(fixed), int32(options)))
	if rect != nil {
		body = join(body, le32(rect...))
	}
	return emfRec(84, body, le32(int32(offDx)), str, le32(dx...))
}

func spacing(n int, dx int32) []int32 {
	out := make([]int32, n)
	for i := range out {
		out[i] = dx
	}
	return out
}

// ink is a probe region: whether any pixel in [X0,X1)x[Y0,Y1) is darker
// than mid-grey. Positions follow from the layout rules being tested.
type ink struct {
	x0, y0, x1, y1 int
	want           bool
}

// A scene with a partial reason is checked by its probes in both renderers
// rather than by whole-image agreement.
type textScene struct {
	name, divergence, partial string
	data                      []byte
	probes                    []ink
	libreOffice               []ink
}

func textScenes() []textScene {
	const w, h = 192, 96
	return []textScene{
		{name: "text-align.emf", data: emfDoc(w, h,
			emfFont(1, -24, 0, 0, 0), emfSelect(1), emfValue(18, 1),
			emfValue(24, cRed), emfValue(22, 0), emfText(1, 8, 4, 0, nil, "Left", spacing(4, 14)),
			emfValue(24, cBlue), emfValue(22, 6|24), emfText(1, 96, 56, 0, nil, "Center", nil),
			emfValue(24, cGreen), emfValue(22, 2|8), emfText(1, 184, 92, 0, nil, "Right", nil),
		)},
		{name: "text-natural.emf", data: emfDoc(w, h,
			emfFont(1, -24, 0, 0, 0), emfSelect(1), emfValue(18, 1),
			emfText(1, 6, 6, 0, nil, "Hamburg", nil),
			emfFont(2, 40, 0, 0, 0), emfSelect(2), emfValue(22, 24), emfText(1, 6, 86, 0, nil, "Cell", nil),
		)},
		{name: "text-rotated.emf", data: emfDoc(w, h,
			emfFont(1, -20, 900, 0, 0), emfFont(2, -20, 300, 0, 0), emfValue(18, 1), emfValue(22, 24),
			emfSelect(1), emfText(1, 24, 88, 0, nil, "Up", spacing(2, 14)),
			emfSelect(2), emfText(1, 60, 80, 0, nil, "Slope", spacing(5, 12)),
		)},
		{name: "text-background.emf", data: emfDoc(w, h,
			emfFont(1, -24, 0, 0, 0), emfSelect(1), emfValue(25, cYell), emfValue(18, 2),
			emfText(1, 6, 4, 0, nil, "Cell", spacing(4, 14)),
			emfValue(18, 1),
			emfText(1, 6, 50, 2|4, []int32{4, 48, 70, 90}, "Clipped", spacing(7, 14)),
		)},
		{name: "text-updatecp.emf", data: emfDoc(w, h,
			emfFont(1, -24, 0, 0, 0), emfSelect(1), emfValue(18, 1), emfPoint(27, 8, 30), emfValue(22, 1|24),
			emfText(1, 0, 0, 0, nil, "AB", spacing(2, 16)), emfText(1, 0, 0, 0, nil, "CD", spacing(2, 16)),
			emfPoint(54, 120, 80),
		)},
		{name: "text-mapping.emf", data: emfDoc(w, h,
			emfValue(17, 2), emfPoint(12, 0, 96),
			emfFont(1, -60, 0, 0, 0), emfSelect(1), emfValue(18, 1), emfValue(22, 24),
			emfText(1, 20, 40, 0, nil, "Upright", spacing(7, 45)),
		)},
		// GM_COMPATIBLE with an anisotropic page: the font height follows the
		// y-axis (x0.96) and the advances the x-axis (x0.48).
		{name: "text-anisotropic.emf", data: emfDoc(w, h,
			emfValue(17, 8), emfPoint(9, 200, 100), emfPoint(11, 96, 96),
			emfFont(1, -25, 0, 0, 0), emfSelect(1), emfValue(18, 1), emfValue(22, 24),
			emfText(1, 10, 60, 0, nil, "Squeeze", spacing(7, 40)),
		)},
		{name: "text-advanced.emf", data: emfDoc(w, h,
			emfRec(35, f32(math.Cos(0.3), math.Sin(0.3), -math.Sin(0.3), math.Cos(0.3), 60, 30)),
			emfFont(1, -24, 0, 0, 0), emfSelect(1), emfValue(18, 1), emfValue(22, 24),
			emfText(2, 0, 0, 0, nil, "Turn", spacing(4, 14)),
		)},
		{name: "text-decoration.emf", partial: "decoration stroke position and thickness are each renderer's font policy; playback supplies the extent",
			data: emfDoc(w, h,
				emfFont(1, -24, 0, 1, 0), emfFont(2, 30, 0, 0, 1), emfValue(18, 1), emfValue(22, 24),
				emfSelect(1), emfText(1, 8, 32, 0, nil, "Under", spacing(5, 15)),
				emfSelect(2), emfText(1, 8, 80, 0, nil, "Strike", spacing(6, 15)),
			),
			// Bars end at the sum of the advances: 8+5*15 and 8+6*15.
			probes:      []ink{{79, 32, 83, 38, true}, {84, 30, 92, 40, false}, {96, 68, 98, 76, true}, {99, 66, 106, 78, false}},
			libreOffice: []ink{{79, 32, 83, 38, true}, {84, 30, 92, 40, false}, {96, 68, 98, 76, true}, {99, 66, 106, 78, false}}},
		// Right-to-left reading order: Hebrew read right to left with the
		// digits and Latin letters inside it left to right, and the same
		// characters in a left-to-right paragraph.
		{name: "text-bidi.emf", data: emfDoc(w, h,
			emfFont(1, -24, 0, 0, 0), emfSelect(1), emfValue(18, 1),
			emfText(1, 8, 4, 0x80, nil, "\u05e9\u05dc\u05d5\u05dd 2024 ok", spacing(12, 14)),
			emfText(1, 8, 52, 0, nil, "ok \u05e9\u05dc\u05d5\u05dd 2024", spacing(12, 14)),
		),
			// The right-to-left line ends with the first Hebrew letter
			// (shin, 9 units wide at 14-unit spacing) at 8+11*14; the
			// left-to-right line starts with "ok".
			probes: []ink{{162, 8, 174, 26, true}, {8, 8, 34, 26, true}, {8, 56, 34, 74, true}, {176, 4, 192, 30, false}}},
		{name: "text-bidi-decoration.emf", partial: "decoration stroke position and thickness are each renderer's font policy; playback supplies the extent",
			data: emfDoc(w, h, emfFont(1, -24, 0, 1, 0), emfSelect(1), emfValue(18, 1), emfValue(22, 24),
				emfText(1, 8, 40, 0x80, nil, "\u05e9\u05dc\u05d5\u05dd ab", spacing(7, 15))),
			// The underline runs from 8 to 8+7*15 whatever the order.
			probes:      []ink{{9, 40, 12, 46, true}, {109, 40, 112, 46, true}, {114, 38, 122, 48, false}},
			libreOffice: []ink{{9, 40, 12, 46, true}, {109, 40, 112, 46, true}, {114, 38, 122, 48, false}}},
		{name: "lo-text-no-rect.emf", divergence: "ETO_NO_RECT text is misparsed: the absent rectangle is read anyway and glyphs are stacked",
			data:   emfDoc(w, h, emfFont(1, -24, 0, 0, 0), emfSelect(1), emfValue(18, 1), noRectText(1, 8, 4, "Left", spacing(4, 14))),
			probes: []ink{{48, 4, 60, 30, true}}, libreOffice: []ink{{48, 4, 60, 30, false}}},
		{name: "lo-text-justification.emf", divergence: "SetTextJustification break extra is not applied",
			data:   emfDoc(w, h, emfFont(1, -24, 0, 0, 0), emfSelect(1), emfValue(18, 1), emfRec(120, le32(30, 2)), emfText(1, 6, 50, 0, nil, "a b c", nil)),
			probes: []ink{{78, 50, 87, 76, true}}, libreOffice: []ink{{78, 50, 87, 76, false}}},
		{name: "lo-text-right-dx.emf", divergence: "right alignment with explicit advances ends at the last glyph's own advance instead of the sum of the advances",
			data:   emfDoc(w, h, emfFont(1, -24, 0, 0, 0), emfSelect(1), emfValue(18, 1), emfValue(22, 2|24), emfText(1, 184, 60, 0, nil, "Right", spacing(5, 14))),
			probes: []ink{{178, 30, 184, 62, false}}, libreOffice: []ink{{178, 30, 184, 62, true}}},
		// Right-aligned text ends at the current position; playback then
		// moves the position to the string's left end (118), so the line
		// starts there. LibreOffice leaves it at 150.
		{name: "lo-text-updatecp-right.emf", divergence: "TA_UPDATECP with TA_RIGHT leaves the current position at the string's right end",
			data: emfDoc(w, h, emfFont(1, -24, 0, 0, 0), emfSelect(1), emfValue(18, 1), emfPoint(27, 150, 50), emfValue(22, 1|2|24),
				emfText(1, 0, 0, 0, nil, "AB", spacing(2, 16)), emfPoint(54, 150, 90)),
			probes: []ink{{120, 54, 127, 59, true}, {149, 60, 152, 64, false}}, libreOffice: []ink{{120, 54, 127, 59, false}, {149, 60, 152, 64, true}}},
		{name: "lo-text-world-stretch.emf", divergence: "GM_ADVANCED glyphs are not stretched by an anisotropic world transform",
			data:   emfDoc(w, h, emfRec(35, f32(1.5, 0, 0, 1, 0, 0)), emfFont(1, -24, 0, 0, 0), emfSelect(1), emfValue(18, 1), emfValue(22, 24), emfText(2, 20, 50, 0, nil, "W", nil)),
			probes: []ink{{57, 30, 62, 50, true}}, libreOffice: []ink{{57, 30, 62, 50, false}}},
	}
}

// wmfRec encodes a WMF record from 16-bit words and raw bytes.
func wmfRec(fn uint16, body ...[]byte) []byte {
	b := join(body...)
	if len(b)%2 == 1 {
		b = append(b, 0)
	}
	return join(le32(int32(3+len(b)/2)), le16(int16(fn)), b)
}

// wmfDoc writes a placeable MEMORYMETAFILE whose bounds are w x h logical
// units at 96 per inch, with the window set to the bounds.
func wmfDoc(w, h int16, records ...[]byte) []byte {
	place := join(le32(int32(-1698247209)), le16(0, 0, 0, w, h, 96), le32(0))
	var sum uint16
	for i := 0; i < 20; i += 2 {
		sum ^= binary.LittleEndian.Uint16(place[i:])
	}
	place = append(place, le16(int16(sum))...)
	records = append([][]byte{wmfRec(0x0103, le16(8)), wmfRec(0x020b, le16(0, 0)), wmfRec(0x020c, le16(h, w))}, records...)
	records = append(records, wmfRec(0))
	body := join(records...)
	var max int32
	for off := 0; off < len(body); {
		n := int32(binary.LittleEndian.Uint32(body[off:]))
		if n > max {
			max = n
		}
		off += int(n) * 2
	}
	header := join(le16(1, 9, 0x300), le32(int32(9+len(body)/2)), le16(4), le32(max), le16(0))
	return join(place, header, body)
}

func wmfFont(height int16, charset byte) []byte {
	face := make([]byte, 32)
	copy(face, "Liberation Sans")
	return wmfRec(0x02fb, le16(height, 0, 0, 0, 400), []byte{0, 0, 0, charset, 0, 0, 0, 0}, face)
}

func wmfTextOut(x, y int16, s []byte) []byte {
	p := append([]byte(nil), s...)
	if len(p)%2 == 1 {
		p = append(p, 0)
	}
	return wmfRec(0x0521, le16(int16(len(s))), p, le16(y, x))
}

func wmfExtTextOut(x, y int16, s []byte, dx []int16) []byte {
	p := append([]byte(nil), s...)
	if len(p)%2 == 1 {
		p = append(p, 0)
	}
	return wmfRec(0x0a32, le16(y, x, int16(len(s)), 0), p, le16(dx...))
}

func wmfScenes() []textScene {
	return []textScene{
		{name: "text-ansi.wmf", data: wmfDoc(192, 96,
			wmfFont(-24, 0), wmfRec(0x012d, le16(0)), wmfRec(0x0102, le16(1)), wmfRec(0x012e, le16(24)),
			wmfTextOut(6, 30, []byte("Caf\xe9 \x80")),
			wmfExtTextOut(6, 70, []byte("Euro\x80"), []int16{16, 16, 16, 16, 16}),
		)},
		// GB2312 (code page 936) text with double-byte characters that map
		// into Latin-1: 0xA1E3 is U+00B0 and 0xA1E8 U+00A4. Each advances by
		// the sum of its bytes' advances (10+10), so "B" starts at 6+14+20
		// and "C" at 6+14+20+14+20.
		{name: "text-dbcs.wmf", data: wmfDoc(192, 96,
			wmfFont(-24, 134), wmfRec(0x012d, le16(0)), wmfRec(0x0102, le16(1)), wmfRec(0x012e, le16(24)),
			wmfExtTextOut(6, 50, []byte("A\xa1\xe3B\xa1\xe8C"), []int16{14, 10, 10, 14, 10, 10, 14}),
		),
			probes: []ink{{40, 30, 52, 50, true}, {74, 30, 86, 50, true}, {96, 20, 130, 60, false}}},
		// With 6 extra units per character the final "e" starts after
		// three natural advances plus 18; LibreOffice leaves it at 48.
		{name: "lo-text-charextra.wmf", divergence: "META_SETTEXTCHAREXTRA is not applied (MS-WMF 2.3.5.25)",
			data: wmfDoc(192, 96,
				wmfFont(-24, 0), wmfRec(0x012d, le16(0)), wmfRec(0x0102, le16(1)), wmfRec(0x012e, le16(24)),
				wmfRec(0x0108, le16(6)), wmfTextOut(6, 50, []byte("Wide")),
			),
			probes: []ink{{62, 30, 72, 52, true}}, libreOffice: []ink{{62, 30, 72, 52, false}}},
	}
}
