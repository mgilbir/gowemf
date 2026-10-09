package gowemf

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
