package gowemf

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"github.com/mgilbir/gowemf/internal/oracle"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func renderBitmapFixture() ([]byte, *image.NRGBA) {
	const width, height = 64, 32
	info := dibHeader(width, -height, 24, 0)
	pixels := make([]byte, width*height*3)
	expected := image.NewNRGBA(image.Rect(0, 0, width, height))
	colors := []color.NRGBA{{240, 20, 40, 255}, {20, 230, 60, 255}, {30, 40, 220, 255}, {210, 190, 25, 255}}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := colors[x/16]
			off := (y*width + x) * 3
			pixels[off], pixels[off+1], pixels[off+2] = c.B, c.G, c.R
			expected.SetNRGBA(x, y, c)
		}
	}
	return append(info, pixels...), expected
}
func renderEMFFixture() []byte {
	dib, _ := renderBitmapFixture()
	body := make([]byte, 72)
	copy(body, longs(0, 0, 63, 31, 0, 0, 0, 0, 64, 32, 80, 40, 120, int32(len(dib)-40), 0, 0x00cc0020, 64, 32))
	body = append(body, dib...)
	b := emfFixture(emfRecord(EMRStretchDIBits, body))
	copy(b[8:], longs(0, 0, 63, 31, 0, 0, 1693, 847))
	copy(b[72:], longs(960, 960, 254, 254)) // exact 96 dpi without rounded millimeters
	return b
}
func renderWMFFixture() []byte {
	dib, _ := renderBitmapFixture()
	put32(dib, 8, 32) // bottom-up; this fixture has identical rows
	body := append(longs(0x00cc0020), words(32, 64, 0, 0, 32, 64, 0, 0)...)
	body = append(body, dib...)
	b := wmfDocument(1, testRecord(WMF, MetaSetMapMode, 0, words(8)), testRecord(WMF, MetaSetWindowOrg, 0, words(0, 0)), testRecord(WMF, MetaSetWindowExt, 0, words(32, 64)), testRecord(WMF, MetaSetViewportExt, 0, words(32, 64)), testRecord(WMF, MetaDIBStretchBlt, 0, body))
	place := make([]byte, 22)
	put32(place, 0, 0x9ac6cdd7)
	put16(place, 10, 960)
	put16(place, 12, 480)
	put16(place, 14, 1440)
	var sum uint16
	for i := 0; i < 20; i += 2 {
		sum ^= u16(place[i:])
	}
	put16(place, 20, sum)
	return append(place, b...)
}

func renderTIFFFixtures() map[string][]byte {
	_, im := renderBitmapFixture()
	rgb := make([]byte, 64*32*3)
	for y := 0; y < 32; y++ {
		for x := 0; x < 64; x++ {
			p := im.NRGBAAt(x, y)
			i := (y*64 + x) * 3
			rgb[i], rgb[i+1], rgb[i+2] = p.R, p.G, p.B
		}
	}
	var packed []byte
	for row := 0; row < 32; row++ {
		line := rgb[row*192 : (row+1)*192]
		packed = append(packed, 127)
		packed = append(packed, line[:128]...)
		packed = append(packed, 63)
		packed = append(packed, line[128:]...)
	}
	var codes []int
	var widths []uint
	for start := 0; start < len(rgb); start += 1000 {
		end := start + 1000
		if end > len(rgb) {
			end = len(rgb)
		}
		clearWidth := uint(9)
		if start != 0 {
			clearWidth = 11
		}
		codes = append(codes, 256)
		widths = append(widths, clearWidth)
		for i, p := range rgb[start:end] {
			width := uint(9)
			if i >= 254 {
				width = 10
			}
			if i >= 766 {
				width = 11
			}
			codes = append(codes, int(p))
			widths = append(widths, width)
		}
	}
	last := len(rgb) % 1000
	width := uint(9)
	if last >= 254 {
		width = 10
	}
	if last >= 766 {
		width = 11
	}
	codes = append(codes, 257)
	widths = append(widths, width)
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	_, _ = writer.Write(rgb)
	_ = writer.Close()
	result := make(map[string][]byte)
	for compression, data := range map[uint32][]byte{1: rgb, 5: packTIFFCodes(codes, widths), 8: compressed.Bytes(), 32773: packed} {
		result[fmt.Sprintf("bitmap-%d.tiff", compression)] = tiffFixture(tiffTestConfig{width: 64, height: 32, photo: 2, compression: compression, strips: [][]byte{data}})
	}
	return result
}

func libraryBitmapRender(data []byte) (image.Image, error) {
	var output image.Image
	_, err := Stream(data, StreamOptions{}, func(c Command) error {
		switch v := c.Body.(type) {
		case BitmapTransfer:
			if output != nil || v.Destination != (Point{}) || v.Source != (Point{}) || v.SourceSize != (Point{64, 32}) || v.DestinationSize != (Point{64, 32}) || v.RasterOperation != 0x00cc0020 {
				return fmt.Errorf("fixture transfer changed")
			}
			d, err := ParseDIB(v.Info, v.Bits, v.Usage, nil, ImageLimits{})
			if err != nil {
				return err
			}
			output, err = d.Image()
			return err
		case PackedDIBTransfer:
			if output != nil || v.Destination != (Point{}) || v.Source != (Point{}) || v.SourceSize != (Point{64, 32}) || v.DestinationSize != (Point{64, 32}) || v.RasterOperation != 0x00cc0020 {
				return fmt.Errorf("fixture transfer changed")
			}
			d, err := ParsePackedDIB(v.DIB, v.Usage, nil, ImageLimits{})
			if err != nil {
				return err
			}
			output, err = d.Image()
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if output == nil {
		return nil, fmt.Errorf("missing bitmap transfer")
	}
	return output, nil
}

// comparePixels uses a per-channel threshold and a maximum fraction of bad
// pixels. All pixels participate; no region is cropped to hide differences.
func comparePixels(a, b image.Image, delta uint32, maxBadFraction float64) error {
	if a.Bounds() != b.Bounds() {
		return fmt.Errorf("bounds %v != %v", a.Bounds(), b.Bounds())
	}
	bad, total := 0, 0
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			ar, ag, ab, aa := a.At(x, y).RGBA()
			br, bg, bb, ba := b.At(x, y).RGBA()
			failed := false
			for _, v := range [][2]uint32{{ar, br}, {ag, bg}, {ab, bb}, {aa, ba}} {
				d := int64(v[0]) - int64(v[1])
				if d < 0 {
					d = -d
				}
				if uint64(d) > uint64(delta)*257 {
					failed = true
				}
			}
			if failed {
				bad++
			}
			total++
		}
	}
	if total == 0 || float64(bad) > float64(total)*maxBadFraction {
		return fmt.Errorf("pixel delta: %d/%d exceed %d channels (allowed fraction %.4f)", bad, total, delta, maxBadFraction)
	}
	return nil
}

func TestPixelComparisonRejectsDefect(t *testing.T) {
	a := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	b := image.NewNRGBA(a.Bounds())
	if err := comparePixels(a, b, 2, .01); err != nil {
		t.Fatal(err)
	}
	for x := 0; x < 16; x++ {
		b.SetNRGBA(x, 0, color.NRGBA{R: 255, A: 255})
	}
	if err := comparePixels(a, b, 2, .01); err == nil {
		t.Fatal("comparison missed a changed image row")
	}
}

func TestLibreOfficeRenderOracle(t *testing.T) {
	work, version := oracle.Environment(t, ".external/render")
	inputs := renderTIFFFixtures()
	inputs["bitmap.emf"] = renderEMFFixture()
	inputs["bitmap.wmf"] = renderWMFFixture()
	// The same WMF with a MEMORYMETAFILE header type must agree; LibreOffice
	// renders the DISKMETAFILE type blank.
	memory := renderWMFFixture()
	put16(memory, 22, 1)
	inputs["bitmap-memory.wmf"] = memory
	for name, data := range inputs {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(work, name)
			lo := oracle.LibreOffice(t, dir, name, data, 64, 32)
			var ours image.Image
			var err error
			if strings.HasSuffix(name, ".tiff") {
				var parsed *TIFF
				parsed, err = ParseTIFF(data, ImageLimits{})
				if err == nil {
					ours, err = parsed.Image()
				}
			} else {
				ours, err = libraryBitmapRender(data)
				if err == nil {
					// The bitmap must also survive placement by Play.
					var played image.Image
					played, err = playRender(data, 64, 32)
					if err == nil {
						if err = comparePixels(ours, played, 0, 0); err != nil {
							err = fmt.Errorf("Play bitmap placement: %w", err)
						}
					}
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if name == "bitmap.wmf" {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				poiPath := filepath.Join(dir, "poi.png")
				log, err := oracle.Run(ctx, "java", "-Xmx256m", "-Djava.awt.headless=true", "-Dlog4j2.statusLoggerLevel=OFF", "-cp", filepath.Join(".external", "oracle", "*")+string(os.PathListSeparator)+filepath.Join(".external", "oracle"), "POIBitmapRender", filepath.Join(dir, name), poiPath)
				if err != nil {
					t.Fatal("POI render", err, string(log))
				}
				f, err := os.Open(poiPath)
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				poi, err := png.Decode(io.LimitReader(f, 1<<20))
				if err != nil {
					t.Fatal(err)
				}
				if err := comparePixels(ours, poi, 1, 0); err != nil {
					t.Fatal("POI WMF pixels:", err)
				}
				if err := comparePixels(ours, lo, 3, .01); err != nil {
					if !strings.Contains(version, "24.2.7.2") {
						t.Fatal(err, "artifacts:", dir)
					}
					for y := 0; y < 32; y++ {
						for x := 0; x < 64; x++ {
							r, g, b, a := lo.At(x, y).RGBA()
							if r != 65535 || g != 65535 || b != 65535 || a != 65535 {
								t.Fatal("LibreOffice WMF behavior changed; review known discrepancy", err)
							}
						}
					}
					t.Log("known LibreOffice 24.2.7.2 behavior: a DISKMETAFILE-typed WMF renders blank; POI matches the generated/library pixels and bitmap-memory.wmf checks LibreOffice agreement.")
				}
			} else if err := comparePixels(ours, lo, 3, .01); err != nil {
				t.Fatal(err, "artifacts:", dir)
			}
		})
	}
	for _, s := range renderScenes() {
		t.Run(s.name, func(t *testing.T) {
			dir := filepath.Join(work, s.name)
			lo := oracle.LibreOffice(t, dir, s.name, s.data, 96, 64)
			ours, err := playRender(s.data, 96, 64)
			if err != nil {
				t.Fatal(err)
			}
			oracle.WritePNG(t, filepath.Join(dir, "gowemf.png"), ours)
			oracle.WritePNG(t, filepath.Join(dir, "comparison.png"), oracle.Montage(ours, lo, oracle.Diff(ours, lo)))
			for _, p := range s.probes {
				if err := probeColor(ours, p); err != nil {
					t.Error("playback:", err)
				}
			}
			if s.divergence == "" {
				bad, total, _ := neighborhoodMismatch(ours, lo, sceneDelta, 1)
				t.Logf("unmatched pixels: %d/%d", bad, total)
				if err := compareNeighborhood(ours, lo, sceneDelta, 1, sceneBadFraction); err != nil {
					t.Error(err, "artifacts:", dir)
				}
				return
			}
			// Observed with LibreOffice 24.2.7.2. Any version must still show
			// the recorded pixels; a change requires reviewing ORACLES.md.
			for _, p := range s.libreOffice {
				if err := probeColor(lo, p); err != nil {
					t.Errorf("LibreOffice %s behavior changed (%s); review the pinned divergence: %v", version, s.divergence, err)
				}
			}
			t.Logf("known LibreOffice divergence: %s", s.divergence)
		})
	}
	t.Log("render artifacts:", work)
}

// Agreement tolerance for generated playback scenes; see compareNeighborhood.
const sceneDelta, sceneBadFraction = 40, 0.005
