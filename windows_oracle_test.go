package gowemf

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mgilbir/gowemf/internal/oracle"
)

// windowsScene is an input for the Windows GDI/GDI+ oracle run
// (tools/windows/oracle.ps1), which is not part of CI.
type windowsScene struct {
	File string `json:"file"`
	W    int    `json:"w"`
	H    int    `json:"h"`
}

// windowsOnlyScenes are generated inputs whose Windows rendering settles
// interpretations that LibreOffice cannot.
func windowsOnlyScenes() map[string][]byte {
	half := plusRec(PlusSetPixelOffsetModeRecord, 4)
	square := plusPathObj([]float64{8, 8, 56, 8, 56, 56, 8, 56}, []byte{0, 1, 1, 0x81})
	fill := plusRec(PlusFillRectsRecord, 0, dwords(1, 1), fl(0, 0, 64, 64))
	red, blue := uint32(0xffff0000), uint32(0xff0000ff)
	scenes := map[string][]byte{
		// Path gradient blends (#23): position 0 at the boundary.
		"win-pathgrad-factors.emf": plusScene96(half, plusObj(1, 1, pathGradientBrush(1|8, 4, red, 32, 32, []uint32{blue}, square, nil, dwords(3), fl(0, .5, 1, 0, .8, 1))), fill),
		"win-pathgrad-presets.emf": plusScene96(half, plusObj(1, 1, pathGradientBrush(1|4, 4, red, 32, 32, []uint32{blue}, square, nil, dwords(3), fl(0, .5, 1), dwords(blue, 0xff00ff00, red))), fill),
		"win-pathgrad-plain.emf":   plusScene96(half, plusObj(1, 1, pathGradientBrush(1, 4, red, 32, 32, []uint32{blue}, square, nil)), fill),
	}
	// The pictures embedded by plus-metafile-image.emf, for GDI+ to embed.
	scenes["win-inner.emf"] = sceneInnerEMF()
	scenes["win-inner.wmf"] = sceneInnerWMF()
	// The same picture at 1440 logical units per inch: 600 x 300 units.
	fine := wmfScene(600, 300, 3, wmfRec(MetaSetMapMode, 8), wmfRec(MetaSetWindowOrg, 0, 0), wmfRec(MetaSetWindowExt, 300, 600),
		wmfPen(5, 0, 0), wmfRec(MetaSelectObject, 0), wmfBrush(0, 0x0000ff, 0), wmfRec(MetaSelectObject, 1), wmfPoly(MetaPolygon, 30, 30, 270, 30, 270, 270, 30, 270))
	put16(fine, 14, 1440)
	var sum uint16
	for i := 0; i < 20; i += 2 {
		sum ^= u16(fine[i:])
	}
	put16(fine, 20, sum)
	scenes["win-inner-1440.wmf"] = fine
	// Frame mapping: polygons with edges at device x 48, 60 and 90 and y 32
	// on a 96 x 64 frame of 0.25 mm pixels; whether the frame counts one
	// more pixel shows at the far edges.
	scenes["win-frame-edge.emf"] = emfScene(96, 64, 2, emfSelect(nullPen), emfBrush(1, 0, 0), emfSelect(1),
		emfPoints(EMRPolygon, 0, 0, 48, 0, 48, 32, 0, 32), emfPoints(EMRPolygon, 60, 32, 90, 32, 90, 64, 60, 64))
	// szlMicrometers refining szlMillimeters by 0.33%: a polygon edge at
	// device x 900 lands at 900 with millimeters and near 903 with
	// micrometers, on a frame of the whole 270 x 203 mm device.
	refine := emfFixture(emfSelect(nullPen), emfBrush(1, 0, 0), emfSelect(1), emfPoints(EMRPolygon, 0, 0, 900, 0, 900, 700, 0, 700))
	copy(refine[8:], longs(0, 0, 1023, 767, 0, 0, 27000, 20300))
	copy(refine[72:], longs(1024, 768, 270, 203))
	header := cat(refine[:88], longs(0, 0, 0, 270900, 203700))
	put32(header, 4, 108)
	put16(header, 56, 2)
	refine = append(header, refine[88:]...)
	put32(refine, 48, uint32(len(refine)))
	scenes["win-micrometers-refine.emf"] = refine
	// EMF+ hatch styles 0-52 at the rendering origin, 8x8 cells of 12 px.
	var hatches [][]byte
	hatches = append(hatches, half)
	for s := 0; s <= 52; s++ {
		x, y := float64(s%8)*12, float64(s/8)*9
		hatches = append(hatches, plusObj(1, 1, hatchBrush(uint32(s), 0xff000000, 0xffffffff)), plusRec(PlusFillRectsRecord, 0, dwords(1, 1), fl(x, y, 8, 8)))
	}
	scenes["win-hatches.emf"] = plusScene96(hatches...)
	// szlMicrometers disagreeing with szlMillimeters (#15).
	for name, um := range map[string]*Size{"win-micrometers-agree.emf": {210000, 297000}, "win-micrometers-disagree.emf": {344000, 194000}} {
		b := emfFixture(emfRecord(EMRSelectObject, longs(-0x7ffffff8)), emfBox(EMRRectangle, 0, 0, 50, 50))
		copy(b[8:], longs(0, 0, 99, 99, 0, 0, 2646, 2646))
		copy(b[72:], longs(794, 1123, 210, 297))
		header := cat(b[:88], longs(0, 0, 0, um.X, um.Y))
		put32(header, 4, 108)
		b = append(header, b[88:]...)
		put32(b, 48, uint32(len(b)))
		scenes[name] = b
	}
	return scenes
}

// TestWriteWindowsInputs writes every generated scene for the Windows oracle
// run when GOWEMF_WINDOWS_INPUTS names a directory.
func TestWriteWindowsInputs(t *testing.T) {
	dir := os.Getenv("GOWEMF_WINDOWS_INPUTS")
	if dir == "" {
		t.Skip("GOWEMF_WINDOWS_INPUTS is not set")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	var manifest []windowsScene
	write := func(name string, data []byte, w, h int) {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			t.Fatal(err)
		}
		manifest = append(manifest, windowsScene{name, w, h})
	}
	for _, s := range renderScenes() {
		write(s.name, s.data, 96, 64)
	}
	for name, data := range windowsOnlyScenes() {
		switch {
		case name == "win-micrometers-refine.emf":
			write(name, data, 1024, 768)
		case strings.HasPrefix(name, "win-micrometers"):
			write(name, data, 100, 100)
		case strings.HasPrefix(name, "win-inner"):
			write(name, data, 40, 20)
		default:
			write(name, data, 96, 64)
		}
	}
	data, err := json.MarshalIndent(manifest, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

// TestCompareWindowsOutputs compares playback with the downloaded Windows
// renderings in GOWEMF_WINDOWS_OUT, writing montages beside them.
func TestCompareWindowsOutputs(t *testing.T) {
	root := os.Getenv("GOWEMF_WINDOWS_OUT")
	if root == "" {
		t.Skip("GOWEMF_WINDOWS_OUT is not set")
	}
	scenes := map[string][]byte{}
	for _, s := range renderScenes() {
		scenes[s.name] = s.data
	}
	for name, data := range windowsOnlyScenes() {
		scenes[name] = data
	}
	recorded, _ := filepath.Glob(filepath.Join(root, "recorded", "rec-*.emf"))
	for _, f := range recorded {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		scenes[filepath.Base(f)] = data
	}
	names := make([]string, 0, len(scenes))
	for name := range scenes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		w, h := 96, 64
		switch {
		case name == "win-micrometers-refine.emf":
			w, h = 1024, 768
		case strings.HasPrefix(name, "win-micrometers"):
			w, h = 100, 100
		case name == "rec-inner.emf" || strings.HasPrefix(name, "win-inner"):
			w, h = 40, 20
		}
		ours, err := playRenderOptions(scenes[name], w, h, PlayOptions{CustomLineCaps: true})
		line := name + ":"
		if err != nil {
			line += " play error " + err.Error()
		}
		for _, kind := range []string{"gdiplus", "gdi"} {
			f, err := os.Open(filepath.Join(root, kind, name+".png"))
			if err != nil {
				continue
			}
			win, err := png.Decode(f)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			bad, total, _ := neighborhoodMismatch(ours, win, sceneDelta, 1)
			line += fmt.Sprintf(" %s %d/%d", kind, bad, total)
			oracle.WritePNG(t, filepath.Join(root, "compare", kind+"-"+name+".png"), oracle.Montage(ours, win, oracle.Diff(ours, win)))
		}
		t.Log(line)
	}
}

// TestCompareWindowsDecoding checks every byte, byte pair and invalid
// sequence of the double-byte code pages against Windows' MultiByteToWideChar.
func TestCompareWindowsDecoding(t *testing.T) {
	root := os.Getenv("GOWEMF_WINDOWS_OUT")
	if root == "" {
		t.Skip("GOWEMF_WINDOWS_OUT is not set")
	}
	hex := func(u []uint16) string {
		f := make([]string, len(u))
		for i, v := range u {
			f[i] = fmt.Sprintf("%04x", v)
		}
		return strings.Join(f, " ")
	}
	for _, cp := range []uint16{932, 936, 949, 950} {
		data, err := os.ReadFile(filepath.Join(root, fmt.Sprintf("mbtowc-%d.txt", cp)))
		if err != nil {
			t.Fatal(err)
		}
		bad := 0
		for _, line := range strings.Split(strings.TrimSpace(strings.ReplaceAll(string(data), "\r", "")), "\n") {
			key, want, _ := strings.Cut(line, ": ")
			var inputs [][]byte
			var wants []string
			if len(key) == 2 {
				var b byte
				fmt.Sscanf(key, "%02x", &b)
				alone, withA, _ := strings.Cut(want, " | ")
				inputs, wants = [][]byte{{b}, {b, 'A'}}, []string{alone, withA}
			} else {
				var lead, trail byte
				fmt.Sscanf(key, "%02x%02x", &lead, &trail)
				inputs, wants = [][]byte{{lead, trail, 'A'}}, []string{want}
			}
			for i, in := range inputs {
				got, _, _ := decodeANSI(cp, in)
				if hex(got) != wants[i] {
					if bad++; bad <= 10 {
						t.Errorf("%d %x: %s, Windows %s", cp, in, hex(got), wants[i])
					}
				}
			}
		}
		t.Logf("code page %d: %d differences", cp, bad)
	}
}
