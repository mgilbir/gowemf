package rendercheck

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mgilbir/gowemf"
	"github.com/mgilbir/gowemf/internal/oracle"
	"github.com/mgilbir/gowemf/internal/raster"
)

// Agreement tolerance for text scenes; see raster.CompareNeighborhood.
const textDelta, textBadFraction = 40, 0.005

func fontPath(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := oracle.Run(ctx, "fc-match", "Liberation Sans:style=Regular", "--format=%{file}\n%{family}\n")
	if err != nil {
		t.Fatal("fc-match", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[1], "Liberation Sans") {
		t.Fatalf("Liberation Sans is required, fc-match gave %q", out)
	}
	return lines[0]
}

func checkInk(im image.Image, p ink) error {
	found := false
	for y := p.y0; y < p.y1 && !found; y++ {
		for x := p.x0; x < p.x1; x++ {
			c := color.GrayModel.Convert(im.At(x, y)).(color.Gray)
			if c.Y < 128 {
				found = true
				break
			}
		}
	}
	if found != p.want {
		return fmt.Errorf("ink in [%d,%d)x[%d,%d) = %v, want %v", p.x0, p.x1, p.y0, p.y1, found, p.want)
	}
	return nil
}

func TestLibreOfficeText(t *testing.T) {
	work, version := oracle.Environment(t, filepath.Join("..", ".external", "render"))
	face, err := loadFace(fontPath(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range append(append(textScenes(), wmfScenes()...), plusTextScenes(face)...) {
		t.Run(s.name, func(t *testing.T) {
			dir := filepath.Join(work, s.name)
			lo := oracle.LibreOffice(t, dir, s.name, s.data, 192, 96)
			b := newBackend(192, 96, face)
			if _, err := gowemf.Play(s.data, gowemf.PlayOptions{Destination: gowemf.Box{Width: 192, Height: 96}}, b); err != nil {
				t.Fatal(err)
			}
			ours := b.image()
			oracle.WritePNG(t, filepath.Join(dir, "gowemf.png"), ours)
			oracle.WritePNG(t, filepath.Join(dir, "comparison.png"), oracle.Montage(ours, lo, oracle.Diff(ours, lo)))
			for _, p := range s.probes {
				if err := checkInk(ours, p); err != nil {
					t.Error("playback:", err)
				}
			}
			if s.partial != "" {
				for _, p := range s.libreOffice {
					if err := checkInk(lo, p); err != nil {
						t.Error("LibreOffice:", err)
					}
				}
				t.Log("probes only:", s.partial)
				return
			}
			if s.divergence == "" {
				bad, total, _ := raster.NeighborhoodMismatch(ours, lo, textDelta, 1)
				t.Logf("unmatched pixels: %d/%d", bad, total)
				if err := raster.CompareNeighborhood(ours, lo, textDelta, 1, textBadFraction); err != nil {
					t.Error(err, "artifacts:", dir)
				}
				return
			}
			// Observed with LibreOffice 24.2.7.2; a change needs review.
			for _, p := range s.libreOffice {
				if err := checkInk(lo, p); err != nil {
					t.Errorf("LibreOffice %s behavior changed (%s): %v", version, s.divergence, err)
				}
			}
			t.Logf("known LibreOffice divergence: %s", s.divergence)
		})
	}
	t.Log("render artifacts:", work)
}
