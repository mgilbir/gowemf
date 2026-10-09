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
