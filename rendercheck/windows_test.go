package rendercheck

import (
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/gowemf"
	"github.com/mgilbir/gowemf/internal/oracle"
	"github.com/mgilbir/gowemf/internal/raster"
)

// TestWriteWindowsInputs writes the GDI text scenes for the transitional
// Windows oracle run when GOWEMF_WINDOWS_INPUTS names a directory.
func TestWriteWindowsInputs(t *testing.T) {
	dir := os.Getenv("GOWEMF_WINDOWS_INPUTS")
	if dir == "" {
		t.Skip("GOWEMF_WINDOWS_INPUTS is not set")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	type scene struct {
		File string `json:"file"`
		W    int    `json:"w"`
		H    int    `json:"h"`
	}
	var manifest []scene
	for _, s := range append(textScenes(), wmfScenes()...) {
		if err := os.WriteFile(filepath.Join(dir, s.name), s.data, 0644); err != nil {
			t.Fatal(err)
		}
		manifest = append(manifest, scene{s.name, 192, 96})
	}
	data, err := json.MarshalIndent(manifest, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

// TestCompareWindowsText compares the text scenes with their Windows GDI
// renderings in GOWEMF_WINDOWS_OUT, writing montages beside them.
func TestCompareWindowsText(t *testing.T) {
	root := os.Getenv("GOWEMF_WINDOWS_OUT")
	if root == "" {
		t.Skip("GOWEMF_WINDOWS_OUT is not set")
	}
	face, err := loadFace(fontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range append(textScenes(), wmfScenes()...) {
		b := newBackend(192, 96, face)
		if _, err := gowemf.Play(s.data, gowemf.PlayOptions{Destination: gowemf.Box{Width: 192, Height: 96}}, b); err != nil {
			t.Fatal(s.name, err)
		}
		for _, kind := range []string{"gdi", "gdiplus"} {
			f, err := os.Open(filepath.Join(root, kind, s.name+".png"))
			if err != nil {
				continue
			}
			win, err := png.Decode(f)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			bad, total, _ := raster.NeighborhoodMismatch(b.image(), win, textDelta, 1)
			t.Logf("%s %s %d/%d", s.name, kind, bad, total)
			oracle.WritePNG(t, filepath.Join(root, "compare", "text-"+kind+"-"+s.name+".png"), oracle.Montage(b.image(), win, oracle.Diff(b.image(), win)))
		}
	}
}
