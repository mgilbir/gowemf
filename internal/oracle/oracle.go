// Package oracle runs LibreOffice as an execution-only render oracle for
// tests. No LibreOffice source is read or ported. Every process has a
// timeout, an address-space and CPU limit, and capped output.
package oracle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type capped struct {
	bytes.Buffer
	max int
}

func (b *capped) Write(p []byte) (int, error) {
	if len(p) > b.max-b.Len() {
		return 0, fmt.Errorf("oracle output limit")
	}
	return b.Buffer.Write(p)
}

// Run executes an oracle tool with a headless LibreOffice environment and a
// 64 KiB combined output cap.
func Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "SAL_USE_VCLPLUGIN=svp", "LC_ALL=C.UTF-8")
	out := &capped{max: 64 << 10}
	cmd.Stdout = out
	cmd.Stderr = out
	err := cmd.Run()
	return out.Bytes(), err
}

// Environment checks prerequisites, creates a fresh run directory under root
// and records tool versions. It skips unless GOWEMF_RENDER=1.
func Environment(t testing.TB, root string) (work, version string) {
	t.Helper()
	if os.Getenv("GOWEMF_RENDER") != "1" {
		t.Skip("run make test-render")
	}
	for _, tool := range []string{"libreoffice", "fc-match", "prlimit"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatal("required render tool", tool, err)
		}
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	work, err := os.MkdirTemp(root, "run-")
	if err != nil {
		t.Fatal(err)
	}
	work, err = filepath.Abs(work)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	font, err := Run(ctx, "fc-match", "Liberation Sans", "--format=%{family}\n")
	cancel()
	if err != nil || !strings.Contains(string(font), "Liberation Sans") {
		t.Fatal("font prerequisite", string(font), err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	out, err := Run(ctx, "libreoffice", "--version")
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	version = strings.TrimSpace(string(out))
	metadata, _ := json.MarshalIndent(map[string]string{"libreoffice": version, "font": strings.TrimSpace(string(font)), "coverage": "generated raster transfers and GDI playback scenes"}, "", "  ")
	if err := os.WriteFile(filepath.Join(work, "environment.json"), metadata, 0600); err != nil {
		t.Fatal(err)
	}
	return work, version
}

// LibreOffice exports one generated input through LibreOffice Draw at
// exactly w x h pixels and returns the decoded PNG.
func LibreOffice(t testing.TB, dir, name string, data []byte, w, h int) image.Image {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(dir, name)
	if err := os.WriteFile(input, data, 0600); err != nil {
		t.Fatal(err)
	}
	// Importing a metafile alone creates a default letter-sized Draw page.
	// Give the oracle an explicit zero-margin page/frame with the output's
	// aspect ratio instead of cropping its output after the fact.
	const mmPerPixel = 169.333333 / 64
	pw, ph := fmt.Sprintf("%.6fmm", float64(w)*mmPerPixel), fmt.Sprintf("%.6fmm", float64(h)*mmPerPixel)
	uri := (&url.URL{Scheme: "file", Path: input}).String()
	document := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<office:document xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0" xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0" xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0" xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0" xmlns:xlink="http://www.w3.org/1999/xlink" office:version="1.2" office:mimetype="application/vnd.oasis.opendocument.graphics">
<office:font-face-decls><style:font-face style:name="Liberation Sans" svg:font-family="Liberation Sans"/></office:font-face-decls>
<office:styles/>
<office:automatic-styles><style:page-layout style:name="RenderPage"><style:page-layout-properties fo:page-width="%[2]s" fo:page-height="%[3]s" fo:margin-left="0mm" fo:margin-right="0mm" fo:margin-top="0mm" fo:margin-bottom="0mm" style:print-orientation="landscape"/></style:page-layout></office:automatic-styles>
<office:master-styles><style:master-page style:name="RenderMaster" style:page-layout-name="RenderPage"/></office:master-styles>
<office:body><office:drawing><draw:page draw:name="page1" draw:master-page-name="RenderMaster"><draw:frame svg:x="0mm" svg:y="0mm" svg:width="%[2]s" svg:height="%[3]s"><draw:image xlink:href="%[1]s" xlink:type="simple" xlink:show="embed" xlink:actuate="onLoad"/></draw:frame></draw:page></office:drawing></office:body></office:document>`, html.EscapeString(uri), pw, ph)
	input = filepath.Join(dir, "scene.fodg")
	if err := os.WriteFile(input, []byte(document), 0600); err != nil {
		t.Fatal(err)
	}
	profile := (&url.URL{Scheme: "file", Path: filepath.Join(dir, "profile")}).String()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	filter := fmt.Sprintf(`png:draw_png_Export:{"PixelWidth":{"type":"long","value":"%d"},"PixelHeight":{"type":"long","value":"%d"},"Translucent":{"type":"boolean","value":"false"}}`, w, h)
	log, err := Run(ctx, "prlimit", "--as=2147483648", "--cpu=60", "--", "libreoffice", "-env:UserInstallation="+profile, "--headless", "--convert-to", filter, "--outdir", dir, input)
	if err != nil {
		t.Fatal("LibreOffice render", err, string(log))
	}
	file, err := os.Open(filepath.Join(dir, "scene.png"))
	if err != nil {
		t.Fatal(err, string(log))
	}
	defer file.Close()
	pngBytes, err := io.ReadAll(io.LimitReader(file, 4<<20))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(pngBytes))
	if err != nil || cfg.Width != w || cfg.Height != h {
		t.Fatal("oracle PNG dimensions", cfg, err)
	}
	oracle, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatal(err)
	}
	return oracle
}

// Montage places images side by side, enlarged for inspection.
func Montage(ims ...image.Image) *image.NRGBA {
	const k = 5
	w, h := ims[0].Bounds().Dx(), ims[0].Bounds().Dy()
	out := image.NewNRGBA(image.Rect(0, 0, (w*k+4)*len(ims), h*k))
	for i, im := range ims {
		for y := 0; y < h*k; y++ {
			for x := 0; x < w*k; x++ {
				out.Set(i*(w*k+4)+x, y, im.At(x/k, y/k))
			}
		}
	}
	return out
}

// Diff shows the largest per-channel difference, white for none.
func Diff(a, b image.Image) *image.NRGBA {
	r := a.Bounds()
	out := image.NewNRGBA(r)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			ar, ag, ab, _ := a.At(x, y).RGBA()
			br, bg, bb, _ := b.At(x, y).RGBA()
			d := func(p, q uint32) uint8 {
				if p > q {
					return uint8((p - q) >> 8)
				}
				return uint8((q - p) >> 8)
			}
			v := 255 - max(d(ar, br), d(ag, bg), d(ab, bb))
			out.Set(x, y, color.NRGBA{v, v, v, 255})
		}
	}
	return out
}

func WritePNG(t testing.TB, path string, im image.Image) {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}
