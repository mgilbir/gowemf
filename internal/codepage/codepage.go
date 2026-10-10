// Package codepage parses the MBTABLE of a Windows best-fit code page file
// (MS-UCODEREF 2.2.2.1). It is used only to generate and verify tables.
package codepage

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// Table is a complete single-byte code page.
type Table struct {
	CodePage int
	Bytes    [256]uint16
}

// Parse accepts a single-byte file whose MBTABLE lists all 256 bytes exactly
// once and maps 0x00-0x7F to themselves.
func Parse(data []byte) (*Table, error) {
	t := &Table{CodePage: -1}
	var seen [256]bool
	remaining, inTable, single := -1, false, false
	s := bufio.NewScanner(bytes.NewReader(data))
	for s.Scan() {
		line := s.Text()
		if i := strings.IndexByte(line, ';'); i >= 0 {
			line = line[:i]
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if inTable && remaining > 0 {
			if len(f) != 2 {
				return nil, fmt.Errorf("MBTABLE record %q", line)
			}
			b, err1 := strconv.ParseUint(f[0], 0, 8)
			u, err2 := strconv.ParseUint(f[1], 0, 16)
			if err1 != nil || err2 != nil || seen[b] {
				return nil, fmt.Errorf("MBTABLE record %q", line)
			}
			seen[b] = true
			t.Bytes[b] = uint16(u)
			remaining--
			continue
		}
		inTable = false
		switch f[0] {
		case "CODEPAGE":
			if len(f) != 2 || t.CodePage >= 0 {
				return nil, fmt.Errorf("CODEPAGE tag")
			}
			n, err := strconv.Atoi(f[1])
			if err != nil {
				return nil, err
			}
			t.CodePage = n
		case "CPINFO":
			single = len(f) == 4 && f[1] == "1"
		case "MBTABLE":
			if len(f) != 2 || remaining >= 0 {
				return nil, fmt.Errorf("MBTABLE tag")
			}
			n, err := strconv.Atoi(f[1])
			if err != nil || n != 256 {
				return nil, fmt.Errorf("MBTABLE size %q", f[1])
			}
			remaining, inTable = n, true
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if t.CodePage < 0 || !single || remaining != 0 {
		return nil, fmt.Errorf("incomplete single-byte code page")
	}
	for i := 0; i < 128; i++ {
		if !seen[i] || t.Bytes[i] != uint16(i) {
			return nil, fmt.Errorf("byte 0x%02x is not the ASCII identity", i)
		}
	}
	return t, nil
}

// DBCS is a double-byte code page (MS-UCODEREF 2.2.2.1): single-byte
// mappings, lead-byte ranges and, for each lead byte, its trail-byte mappings.
type DBCS struct {
	CodePage    int
	DefaultChar uint16 // CPINFO's Unicode default character
	Single      map[byte]uint16
	Leads       [][2]byte
	Pairs       map[uint16]uint16 // lead<<8|trail to UTF-16
}

// IsLead reports whether b is in a lead-byte range.
func (d *DBCS) IsLead(b byte) bool {
	for _, r := range d.Leads {
		if b >= r[0] && b <= r[1] {
			return true
		}
	}
	return false
}

// ParseDBCS reads the CPINFO, MBTABLE, DBCSRANGE and DBCSTABLE sections of a
// double-byte file, checking every declared count. Each byte must be either a
// single-byte character or a lead byte, and 0x00-0x7F the ASCII identity.
func ParseDBCS(data []byte) (*DBCS, error) {
	d := &DBCS{CodePage: -1, Single: map[byte]uint16{}, Pairs: map[uint16]uint16{}}
	var records [][]string
	s := bufio.NewScanner(bytes.NewReader(data))
	for s.Scan() {
		line := s.Text()
		if i := strings.IndexByte(line, ';'); i >= 0 {
			line = line[:i]
		}
		if f := strings.Fields(line); len(f) > 0 {
			records = append(records, f)
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	num := func(s string, bits int) (uint64, error) { return strconv.ParseUint(s, 0, bits) }
	pair := func(f []string, bits int) (uint64, uint64, error) {
		if len(f) != 2 {
			return 0, 0, fmt.Errorf("record %q", f)
		}
		a, err1 := num(f[0], 8)
		b, err2 := num(f[1], bits)
		if err1 != nil || err2 != nil {
			return 0, 0, fmt.Errorf("record %q", f)
		}
		return a, b, nil
	}
	count := func(f []string, tag string) (int, error) {
		if len(f) != 2 || !strings.EqualFold(f[0], tag) {
			return 0, fmt.Errorf("expected %s, got %q", tag, f)
		}
		n, err := strconv.Atoi(f[1])
		if err != nil || n < 0 {
			return 0, fmt.Errorf("%s count %q", tag, f[1])
		}
		return n, nil
	}
	dbcs, haveMB, haveRanges := false, false, false
	for i := 0; i < len(records); {
		f := records[i]
		switch strings.ToUpper(f[0]) {
		case "CODEPAGE":
			if len(f) != 2 || d.CodePage >= 0 {
				return nil, fmt.Errorf("CODEPAGE tag")
			}
			n, err := strconv.Atoi(f[1])
			if err != nil {
				return nil, err
			}
			d.CodePage = n
			i++
		case "CPINFO":
			if len(f) != 4 || f[1] != "2" {
				return nil, fmt.Errorf("not a double-byte CPINFO: %q", f)
			}
			v, err := num(f[3], 16)
			if err != nil {
				return nil, err
			}
			d.DefaultChar, dbcs = uint16(v), true
			i++
		case "MBTABLE":
			n, err := count(f, "MBTABLE")
			if err != nil || haveMB || i+n >= len(records) {
				return nil, fmt.Errorf("MBTABLE")
			}
			for _, r := range records[i+1 : i+1+n] {
				b, u, err := pair(r, 16)
				if err != nil {
					return nil, err
				}
				if _, dup := d.Single[byte(b)]; dup {
					return nil, fmt.Errorf("byte 0x%02x listed twice", b)
				}
				d.Single[byte(b)] = uint16(u)
			}
			haveMB = true
			i += 1 + n
		case "DBCSRANGE":
			n, err := count(f, "DBCSRANGE")
			if err != nil || haveRanges || n == 0 {
				return nil, fmt.Errorf("DBCSRANGE")
			}
			i++
			for k := 0; k < n; k++ {
				if i >= len(records) {
					return nil, fmt.Errorf("missing lead byte range")
				}
				lo, hi, err := pair(records[i], 8)
				if err != nil || lo > hi || lo < 0x80 {
					return nil, fmt.Errorf("lead byte range %q", records[i])
				}
				d.Leads = append(d.Leads, [2]byte{byte(lo), byte(hi)})
				i++
				// One DBCSTABLE follows for each lead byte in order.
				for lead := lo; lead <= hi; lead++ {
					if i >= len(records) {
						return nil, fmt.Errorf("missing DBCSTABLE for 0x%02x", lead)
					}
					m, err := count(records[i], "DBCSTABLE")
					if err != nil || i+m >= len(records) {
						return nil, fmt.Errorf("DBCSTABLE for 0x%02x", lead)
					}
					for _, r := range records[i+1 : i+1+m] {
						t, u, err := pair(r, 16)
						if err != nil {
							return nil, err
						}
						key := uint16(lead)<<8 | uint16(t)
						if _, dup := d.Pairs[key]; dup || u == 0 {
							return nil, fmt.Errorf("pair 0x%04x", key)
						}
						d.Pairs[key] = uint16(u)
					}
					i += 1 + m
				}
			}
			haveRanges = true
		case "WCTABLE", "ENDCODEPAGE":
			i = len(records) // the Unicode-to-bytes direction is not needed
		default:
			return nil, fmt.Errorf("unexpected record %q", f)
		}
	}
	if d.CodePage < 0 || !dbcs || !haveMB || !haveRanges {
		return nil, fmt.Errorf("incomplete double-byte code page")
	}
	for b := 0; b < 256; b++ {
		_, single := d.Single[byte(b)]
		if single == d.IsLead(byte(b)) {
			return nil, fmt.Errorf("byte 0x%02x is not exactly one of single and lead", b)
		}
		if b < 0x80 && d.Single[byte(b)] != uint16(b) {
			return nil, fmt.Errorf("byte 0x%02x is not the ASCII identity", b)
		}
	}
	return d, nil
}

// EncodePairs writes the trail mappings of every lead byte, in byte order
// over all 256 trail values, as unsigned LEB128 tokens: 0 followed by a count
// of unmapped trails, or 1 plus the zigzag difference from the previous
// mapped value.
func EncodePairs(d *DBCS) []byte {
	var out []byte
	put := func(v uint64) {
		for v >= 0x80 {
			out = append(out, byte(v)|0x80)
			v >>= 7
		}
		out = append(out, byte(v))
	}
	prev, skip := 0, uint64(0)
	for lead := 0; lead < 256; lead++ {
		if !d.IsLead(byte(lead)) {
			continue
		}
		for trail := 0; trail < 256; trail++ {
			u, ok := d.Pairs[uint16(lead)<<8|uint16(trail)]
			if !ok {
				skip++
				continue
			}
			if skip > 0 {
				put(0)
				put(skip)
				skip = 0
			}
			delta := int(u) - prev
			z := uint64(delta) << 1
			if delta < 0 {
				z = uint64(-delta)<<1 - 1
			}
			put(z + 1)
			prev = int(u)
		}
	}
	if skip > 0 {
		put(0)
		put(skip)
	}
	return out
}
