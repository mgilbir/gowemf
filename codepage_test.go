package gowemf

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"unicode/utf16"

	"github.com/mgilbir/gowemf/internal/codepage"
	"github.com/mgilbir/gowemf/internal/corpus"
)

// TestCodePageTables regenerates codepage_tables.go from the pinned best-fit
// files and requires the committed table to match byte for byte.
func TestCodePageTables(t *testing.T) {
	if os.Getenv("GOWEMF_EXTERNAL") != "1" {
		t.Skip("run make test-external to verify downloaded code page files")
	}
	out := filepath.Join(t.TempDir(), "tables.go")
	cmd := exec.Command("go", "run", "./internal/codepagegen", "-o", out)
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(msg))
	}
	want, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("codepage_tables.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("codepage_tables.go differs from the pinned code page files; run make codepages")
	}
}

func TestCodePageSpotValues(t *testing.T) {
	// Independent spot checks of well-known assignments.
	for _, c := range []struct {
		cp   uint16
		b    byte
		want uint16
	}{{1252, 0x80, 0x20ac}, {1252, 0xe9, 0x00e9}, {1251, 0xc0, 0x0410}, {1253, 0xc1, 0x0391}, {1250, 0x8a, 0x0160}, {874, 0xa1, 0x0e01}, {1255, 0xe0, 0x05d0}, {1256, 0xc7, 0x0627}} {
		if got := codePageHigh[c.cp][c.b-0x80]; got != c.want {
			t.Errorf("code page %d byte %#x = %#04x, want %#04x", c.cp, c.b, got, c.want)
		}
	}
}

// TestDBCSTables decodes every byte and lead/trail pair of the generated
// double-byte tables and compares them with the pinned best-fit files.
func TestDBCSTables(t *testing.T) {
	if os.Getenv("GOWEMF_EXTERNAL") != "1" {
		t.Skip("run make test-external to verify downloaded code page files")
	}
	for _, f := range corpus.DBCSCodePages {
		data, err := os.ReadFile(filepath.Join(".external", "codepages", f.Path))
		if err != nil {
			t.Fatal(err)
		}
		want, err := codepage.ParseDBCS(data)
		if err != nil {
			t.Fatal(f.Path, err)
		}
		c := dbcsCodePages[uint16(want.CodePage)]
		if c == nil || c.defaultChar != want.DefaultChar {
			t.Fatal(f.Path, "missing or wrong default character")
		}
		pairs := 0
		for b := 0; b < 256; b++ {
			lead := byte(b)
			if c.isLead(lead) != want.IsLead(lead) {
				t.Fatalf("%s: lead 0x%02x", f.Path, b)
			}
			if !want.IsLead(lead) {
				if got, spans := c.decode([]byte{lead}); got[0] != want.Single[lead] || spans[0] != 1 {
					t.Fatalf("%s: byte 0x%02x = %#04x", f.Path, b, got[0])
				}
				continue
			}
			for trail := 0; trail < 256; trail++ {
				got, spans := c.decode([]byte{lead, byte(trail)})
				u, ok := want.Pairs[uint16(b)<<8|uint16(trail)]
				if ok {
					pairs++
				} else {
					u = want.DefaultChar
				}
				if trail == 0 {
					if len(got) != 2 || got[0] != want.DefaultChar || got[1] != 0 {
						t.Fatalf("%s: 0x%02x00 = %#04x", f.Path, b, got)
					}
					continue
				}
				if len(got) != 1 || got[0] != u || spans[0] != 2 {
					t.Fatalf("%s: 0x%02x%02x = %#04x, want %#04x", f.Path, b, trail, got, u)
				}
			}
		}
		if pairs != len(want.Pairs) {
			t.Fatal(f.Path, pairs, len(want.Pairs))
		}
	}
}

// TestDBCSDecode checks well-known characters, the default character for
// invalid sequences (MS-UCODEREF 3.1.5.1.1) and concurrent first use.
func TestDBCSDecode(t *testing.T) {
	for _, c := range []struct {
		cp    uint16
		in    string
		want  string
		spans []int
	}{
		{950, "\xa4\xa4\xa4\xe5", "中文", []int{2, 2}},
		{950, "\xb7\x73\xb2\xd3\xa9\xfa\xc5\xe9", "新細明體", []int{2, 2, 2, 2}},
		{932, "\x82\x6c\x82\x72\x20\x83\x53\x83\x56\x83\x62\x83\x4e", "ＭＳ ゴシック", []int{2, 2, 1, 2, 2, 2, 2}},
		{932, "A\xb1", "Aｱ", []int{1, 1}}, // a half-width katakana single byte
		{936, "\xcb\xce\xcc\xe5", "宋体", []int{2, 2}},
		{949, "\xb1\xbc\xb8\xb2", "굴림", []int{2, 2}},
		{932, "\x81\x20A", "・A", []int{2, 1}}, // invalid trail: both bytes become the default
		{932, "A\x81", "A・", []int{1, 1}},     // a lead byte at the end
		{936, "\x81\x7f", "?", []int{2}},
		{932, "\x81\x00A", "\u30fb\x00A", []int{1, 1, 1}}, // NUL is not taken as a trail byte
	} {
		got, spans := dbcsCodePages[c.cp].decode([]byte(c.in))
		if string(utf16.Decode(got)) != c.want || fmt.Sprint(spans) != fmt.Sprint(c.spans) {
			t.Errorf("%d %q: %q %v, want %q %v", c.cp, c.in, string(utf16.Decode(got)), spans, c.want, c.spans)
		}
	}
	fresh := &dbcsCodePage{defaultChar: '?', leads: dbcsCodePages[949].leads, single: dbcsCodePages[949].single, pairs: dbcsCodePages[949].pairs}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got, _ := fresh.decode([]byte("\xb1\xbc")); got[0] != 0xad74 {
				t.Error("concurrent decode", got)
			}
		}()
	}
	wg.Wait()
}
