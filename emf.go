package gowemf

// MS-EMF 2.2.9, 2.3.3 and 2.3.4. Header extensions and drawing bodies are
// retained without interpretation; only the common header is decoded here.
func (p *parser) emf() error {
	b := p.data
	if len(b) < 88 {
		return malformed(0, "EMF header")
	}
	if u32(b[40:]) != 0x464d4520 {
		return malformed(40, "EMF signature")
	}
	h := &EMFHeader{
		Bounds: rect(b[8:]), Frame: rect(b[24:]), Version: u32(b[44:]),
		Bytes: u32(b[48:]), Records: u32(b[52:]), Handles: u16(b[56:]),
		PaletteEntries: u32(b[68:]), Device: size(b[72:]), Millimeters: size(b[80:]),
	}
	p.header.Format, p.header.EMF = EMF, h
	if uint64(h.Bytes) != uint64(len(b)) {
		return malformed(48, "EMF file size")
	}
	var records uint64
	for off := 0; off < len(b); {
		if len(b)-off < 8 {
			return malformed(off, "EMF record header")
		}
		typ, n := u32(b[off:]), uint64(u32(b[off+4:]))
		if err := p.record(off, n, len(b)-off, 8, 4); err != nil {
			return err
		}
		end := off + int(n)
		r := b[off:end]
		records++
		if off == 0 {
			if n < 88 {
				return malformed(off, "EMF header size")
			}
			chars, pos := uint64(u32(r[60:])), uint64(u32(r[64:]))
			if chars != 0 && (pos < 88 || pos%2 != 0 || pos > n || chars*2 > n-pos) {
				return malformed(60, "EMF description range")
			}
		} else if typ == 1 {
			return malformed(off, "duplicate EMF header")
		}
		var comment []byte
		if typ == 70 {
			if n < 12 {
				return malformed(off, "EMF comment header")
			}
			dataSize := uint64(u32(r[8:]))
			if dataSize > n-12 {
				return malformed(off+8, "EMF comment data size")
			}
			comment = r[12 : 12+int(dataSize)]
		}
		if typ == 14 {
			if n < 20 || end != len(b) {
				return malformed(off, "EMF EOF")
			}
			entries, pos := uint64(u32(r[8:])), uint64(u32(r[12:]))
			if entries != uint64(h.PaletteEntries) {
				return malformed(off+8, "EMF palette count")
			}
			if entries != 0 && (pos < 16 || pos%4 != 0 || pos > n-4 || entries*4 > n-4-pos) {
				return malformed(off+12, "EMF palette range")
			}
			if uint64(u32(r[len(r)-4:])) != n {
				return malformed(end-4, "EMF EOF SizeLast")
			}
			if records != uint64(h.Records) {
				return malformed(52, "EMF record count")
			}
			if p.header.EMFPlus != nil && !p.plusEnded {
				return malformed(off, "missing EMF+ EOF")
			}
		}
		if err := p.emit(EMF, typ, 0, off, -1, r, r[8:]); err != nil {
			return err
		}
		if len(comment) >= 4 && u32(comment) == 0x2b464d45 {
			if p.header.EMFPlus == nil && records != 2 {
				return malformed(off, "EMF+ header must follow EMF header")
			}
			if err := p.plus(comment[4:], off+16, off); err != nil {
				return err
			}
		}
		if typ == 14 {
			return nil
		}
		off = end
	}
	return malformed(len(b), "missing EMF EOF")
}
