package bidi

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func external(t *testing.T) string {
	t.Helper()
	if os.Getenv("GOWEMF_EXTERNAL") != "1" {
		t.Skip("run make test-external to check the downloaded Unicode files")
	}
	return filepath.Join("..", "..", ".external", "unicode")
}

// TestTables regenerates tables.go from the pinned Unicode files and requires
// the committed file to match byte for byte.
func TestTables(t *testing.T) {
	external(t)
	out := filepath.Join(t.TempDir(), "tables.go")
	cmd := exec.Command("go", "run", "./internal/bidigen", "-o", out)
	cmd.Dir = filepath.Join("..", "..")
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(msg))
	}
	want, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("tables.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("tables.go differs from the pinned Unicode files; run make bidi")
	}
}

func classesOf(runes []rune) ([]Class, []Bracket) {
	c := make([]Class, len(runes))
	b := make([]Bracket, len(runes))
	for i, r := range runes {
		c[i], b[i] = Lookup(r), LookupBracket(r)
	}
	return c, b
}

func levelString(levels []int8) string {
	f := make([]string, len(levels))
	for i, l := range levels {
		if l == Removed {
			f[i] = "x"
		} else {
			f[i] = strconv.Itoa(int(l))
		}
	}
	return strings.Join(f, " ")
}

func orderString(order []int) string {
	f := make([]string, len(order))
	for i, o := range order {
		f[i] = strconv.Itoa(o)
	}
	return strings.Join(f, " ")
}

// TestBidiTest runs every case of BidiTest.txt: sequences of classes, each
// under the paragraph directions of its bitset.
func TestBidiTest(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(external(t), "BidiTest.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var levels, order string
	cases, failures := 0, 0
	s := bufio.NewScanner(bytes.NewReader(data))
	for line := 1; s.Scan(); line++ {
		text := strings.TrimSpace(s.Text())
		if i := strings.IndexByte(text, '#'); i >= 0 {
			text = strings.TrimSpace(text[:i])
		}
		switch {
		case text == "":
			continue
		case strings.HasPrefix(text, "@Levels:"):
			levels = strings.Join(strings.Fields(strings.TrimPrefix(text, "@Levels:")), " ")
			continue
		case strings.HasPrefix(text, "@Reorder:"):
			order = strings.Join(strings.Fields(strings.TrimPrefix(text, "@Reorder:")), " ")
			continue
		case strings.HasPrefix(text, "@"):
			continue
		}
		input, bits, ok := strings.Cut(text, ";")
		set, err := strconv.ParseUint(strings.TrimSpace(bits), 16, 8)
		if !ok || err != nil {
			t.Fatalf("line %d: %q", line, text)
		}
		var classes []Class
		for _, f := range strings.Fields(input) {
			c, ok := ParseClass(f)
			if !ok {
				t.Fatalf("line %d: class %q", line, f)
			}
			classes = append(classes, c)
		}
		for bit, dir := range map[uint64]Direction{1: Auto, 2: LeftToRight, 4: RightToLeft} {
			if set&bit == 0 {
				continue
			}
			cases++
			got, _ := Resolve(classes, nil, dir)
			if gl, go_ := levelString(got), orderString(Reorder(got)); gl != levels || go_ != order {
				failures++
				if failures <= 10 {
					t.Errorf("line %d %v dir %d: levels %q order %q, want %q %q", line, classes, dir, gl, go_, levels, order)
				}
			}
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if failures != 0 || cases < 490000 {
		t.Fatalf("%d of %d cases failed", failures, cases)
	}
	t.Logf("%d cases", cases)
}

// TestBidiCharacterTest runs every case of BidiCharacterTest.txt, which uses
// code points, paired brackets and explicit paragraph levels.
func TestBidiCharacterTest(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(external(t), "BidiCharacterTest.txt"))
	if err != nil {
		t.Fatal(err)
	}
	cases, failures := 0, 0
	s := bufio.NewScanner(bytes.NewReader(data))
	s.Buffer(nil, 1<<20)
	for line := 1; s.Scan(); line++ {
		text := s.Text()
		if text == "" || text[0] == '#' {
			continue
		}
		f := strings.Split(text, ";")
		if len(f) != 5 {
			t.Fatalf("line %d: %q", line, text)
		}
		var runes []rune
		for _, h := range strings.Fields(f[0]) {
			v, err := strconv.ParseUint(h, 16, 32)
			if err != nil {
				t.Fatalf("line %d: %q", line, h)
			}
			runes = append(runes, rune(v))
		}
		dir := map[string]Direction{"0": LeftToRight, "1": RightToLeft, "2": Auto}[f[1]]
		classes, brackets := classesOf(runes)
		cases++
		got, para := Resolve(classes, brackets, dir)
		want := strings.Join(strings.Fields(f[3]), " ")
		wantOrder := strings.Join(strings.Fields(f[4]), " ")
		if gl, go_ := levelString(got), orderString(Reorder(got)); gl != want || go_ != wantOrder || strconv.Itoa(int(para)) != f[2] {
			failures++
			if failures <= 10 {
				t.Errorf("line %d: paragraph %d levels %q order %q, want %s %q %q", line, para, gl, go_, f[2], want, wantOrder)
			}
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if failures != 0 || cases < 90000 {
		t.Fatalf("%d of %d cases failed", failures, cases)
	}
	t.Logf("%d cases", cases)
}

// TestExamples checks UAX #9's own examples, in its upper-case-is-R notation,
// so make check covers the algorithm without the downloaded files.
func TestExamples(t *testing.T) {
	// The examples use upper case for strong right-to-left letters.
	toRunes := func(s string) []rune {
		var out []rune
		for _, r := range s {
			if r >= 'A' && r <= 'Z' {
				r = 0x05d0 + (r - 'A') // Hebrew letters stand in for R
			}
			out = append(out, r)
		}
		return out
	}
	visual := func(s string, dir Direction) string {
		runes := toRunes(s)
		classes, brackets := classesOf(runes)
		levels, _ := Resolve(classes, brackets, dir)
		mirror := map[rune]rune{'(': ')', ')': '(', '[': ']', ']': '['}
		var b strings.Builder
		for _, i := range Reorder(levels) {
			r := runes[i]
			if r >= 0x05d0 && r < 0x05d0+26 {
				r = 'A' + (r - 0x05d0)
			}
			if m, ok := mirror[r]; ok && levels[i]%2 == 1 {
				r = m // L4, as the examples display it
			}
			b.WriteRune(r)
		}
		return b.String()
	}
	for _, c := range []struct {
		storage, display string
		dir              Direction
	}{
		{"car means CAR.", "car means RAC.", LeftToRight},
		{"he said \"THE VALUES ARE 123, 456, 789, OK\".", "he said \"KO ,789 ,456 ,123 ERA SEULAV EHT\".", LeftToRight},
		{"IT IS A bmw 500, OK.", ".KO ,bmw 500 A SI TI", RightToLeft},
		{"AB(CD[&ef]!)gh", "gh(![ef&]DC)BA", RightToLeft},
		{"book(s) ARABIC", "book(s) CIBARA", LeftToRight},
		{"ARABIC book(s)", "book(s) CIBARA", RightToLeft},
	} {
		if got := visual(c.storage, c.dir); got != c.display {
			t.Errorf("%q: %q, want %q", c.storage, got, c.display)
		}
	}
}

// TestLevels checks hand-resolved cases for rules the examples above do not
// reach.
func TestLevels(t *testing.T) {
	for _, c := range []struct {
		text   string
		dir    Direction
		levels string
	}{
		{"\u05d0 $12", RightToLeft, "1 1 2 2 2"},        // W5: a terminator before European numbers
		{"\u0661,\u0662", LeftToRight, "2 2 2"},         // W4: a common separator between Arabic numbers
		{"a\u202bb ", LeftToRight, "0 x 2 0"},           // L1: trailing whitespace inside an embedding
		{"\u0661\u202b\u0300", LeftToRight, "2 x 1"},    // sos and eos from the explicit levels
		{"\u05d0\u2066a\u2069b", Auto, "1 1 2 1 2"},     // P2 skips isolates; isolates resolve as neutrals
		{"a\u2067\u05d0", LeftToRight, "0 0 1"},         // an unmatched isolate initiator
		{"\u202ea\u05d0\u202c", LeftToRight, "x 1 1 x"}, // X4: an override
		// BD16: 63 openers fit the stack, so the last pairs with the closer
		// and takes the preceding L context; a 64th empties the pair list.
		{"a" + strings.Repeat("(", 63) + "b)", RightToLeft, strings.TrimSpace(strings.Repeat("2 ", 66))},
		{"a" + strings.Repeat("(", 64) + "b)", RightToLeft, strings.Repeat("2 ", 66) + "1"},
	} {
		classes, brackets := classesOf([]rune(c.text))
		levels, _ := Resolve(classes, brackets, c.dir)
		if got := levelString(levels); got != c.levels {
			t.Errorf("%+q: %q, want %q", c.text, got, c.levels)
		}
	}
}

func TestLookup(t *testing.T) {
	for r, want := range map[rune]Class{'a': L, 0x05d0: R, 0x0627: AL, '1': EN, '+': ES, '$': ET, 0x0661: AN, ',': CS, 0x0300: NSM, 0x200d: BN, '\n': B, '\t': S, ' ': WS, '!': ON, 0x202a: LRE, 0x202d: LRO, 0x202b: RLE, 0x202e: RLO, 0x202c: PDF, 0x2066: LRI, 0x2067: RLI, 0x2068: FSI, 0x2069: PDI, 0x05eb: R, 0x10ffff: BN, -1: L} {
		if got := Lookup(r); got != want {
			t.Errorf("U+%04X: %v, want %v", r, got, want)
		}
	}
	if b := LookupBracket('('); b != (Bracket{BracketOpen, '('}) {
		t.Fatal(b)
	}
	if b := LookupBracket(')'); b != (Bracket{BracketClose, '('}) {
		t.Fatal(b)
	}
	if LookupBracket(0x232a) != LookupBracket(0x3009) || LookupBracket(0x2329).Key != 0x3008 {
		t.Fatal("canonical angle brackets")
	}
	if b := LookupBracket('a'); b.Kind != BracketNone {
		t.Fatal(b)
	}
}

func ExampleReorder() {
	runes := []rune("abc \u05d0\u05d1\u05d2 123")
	classes, brackets := classesOf(runes)
	levels, _ := Resolve(classes, brackets, RightToLeft)
	fmt.Println(levelString(levels))
	fmt.Println(Reorder(levels))
	// Output:
	// 2 2 2 1 1 1 1 1 2 2 2
	// [8 9 10 7 6 5 4 3 0 1 2]
}
