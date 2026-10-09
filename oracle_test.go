package gowemf

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mgilbir/gowemf/internal/corpus"
)

type oracleRecord struct {
	Format string                     `json:"format"`
	Type   uint32                     `json:"type"`
	Body   map[string]json.RawMessage `json:"body"`
}
type cappedOutput struct {
	bytes.Buffer
	max int
}

func (b *cappedOutput) Write(p []byte) (int, error) {
	if len(p) > b.max-b.Len() {
		return 0, fmt.Errorf("oracle output limit")
	}
	return b.Buffer.Write(p)
}

func poiProcess(path string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "java", "-Xmx256m", "-Djava.awt.headless=true", "-Dlog4j2.statusLoggerLevel=OFF", "-cp", filepath.Join(".external", "oracle", "*")+string(os.PathListSeparator)+filepath.Join(".external", "oracle"), "POIRecordDump", path)
	out, stderr := &cappedOutput{max: 32 << 20}, &cappedOutput{max: 64 << 10}
	cmd.Stdout = out
	cmd.Stderr = stderr
	err := cmd.Run()
	return out.Bytes(), stderr.String(), err
}

func poiRecords(t *testing.T, path string) []oracleRecord {
	t.Helper()
	out, stderr, err := poiProcess(path)
	if err != nil {
		t.Fatalf("POI oracle: %v: %s", err, stderr)
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	var records []oracleRecord
	for {
		var r oracleRecord
		if err := dec.Decode(&r); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("POI JSON: %v", err)
		}
		records = append(records, r)
	}
	return records
}

func TestPOIOracle(t *testing.T) {
	if os.Getenv("GOWEMF_ORACLE") != "1" {
		t.Skip("run make test-oracle")
	}
	t.Run("known-serialized-effect-oracle-gap", func(t *testing.T) {
		r := effectRecord(blurGUIDWire, append(floats(3.5), longs(1)...))
		data := emfFixture(plusComment(plusHeader(), r.Raw, plusRecord(PlusEndOfFileRecord, nil)))
		path := filepath.Join(t.TempDir(), "effects.emf")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, record := range poiRecords(t, path) {
			if record.Format == "EMF+" && record.Type == PlusSerializableObjectRecord {
				found = true
				if len(record.Body) != 1 || string(record.Body["flags"]) != "0" {
					t.Fatal("POI serialized-effect support changed; review parameter oracle coverage", record.Body)
				}
			}
		}
		if !found {
			t.Fatal("POI omitted serialized effect record")
		}
		t.Log("POI 5.4.1 exposes only flags for serialized effects; no parameter agreement is claimed")
	})
	t.Run("generated-custom-caps", func(t *testing.T) {
		cap := arrowCapFixture()
		pen := penWithCapsFixture(cap, cap)
		object := testRecord(EMFPlus, PlusObjectRecord, 0x0201, pen)
		data := emfFixture(plusComment(plusHeader(), object.Raw, plusRecord(PlusEndOfFileRecord, nil)))
		path := filepath.Join(t.TempDir(), "caps.emf")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		v, err := DecodePlusObject(2, pen, DecodeLimits{})
		if err != nil {
			t.Fatal(err)
		}
		decoded := v.(PlusPen)
		type arrow struct {
			Width, Height, MiddleInset, MiterLimit, WidthScale float64
			IsFilled                                           bool
			StartCap, EndCap, Join                             string
		}
		found := false
		for _, record := range poiRecords(t, path) {
			if record.Format == "EMF+" && record.Type == PlusObjectRecord {
				found = true
				var props struct {
					Flags                        uint32
					PenWidth                     float64
					CustomStartCap, CustomEndCap *arrow
				}
				if err := json.Unmarshal(record.Body["objectData"], &props); err != nil {
					t.Fatal(err)
				}
				if props.Flags != decoded.Flags || props.PenWidth != decoded.Width {
					t.Fatal("pen fields differ", props)
				}
				for i, cap := range []*arrow{props.CustomStartCap, props.CustomEndCap} {
					want := []*PlusCustomLineCap{decoded.CustomStartCap, decoded.CustomEndCap}[i].Arrow
					capNames := map[uint32]string{0: "FLAT", 1: "SQUARE", 2: "ROUND", 3: "TRIANGLE"}
					joinNames := map[uint32]string{0: "MITER", 1: "BEVEL", 2: "ROUND", 3: "MITER_CLIPPED"}
					if cap == nil || cap.Width != want.Width || cap.Height != want.Height || cap.MiddleInset != want.MiddleInset || cap.IsFilled != want.Filled || cap.MiterLimit != want.LineMiterLimit || cap.WidthScale != want.WidthScale || cap.StartCap != capNames[want.LineStartCap] || cap.EndCap != capNames[want.LineEndCap] || cap.Join != joinNames[want.LineJoin] {
						t.Fatal("custom cap differs from POI", i, cap, want)
					}
				}
			}
		}
		if !found {
			t.Fatal("POI omitted pen object")
		}
		t.Log("compared pen envelope and both adjustable-arrow custom cap field sets")
	})
	for _, f := range corpus.Files {
		t.Run(f.Path, func(t *testing.T) {
			path := filepath.Join(".external", "poi", filepath.FromSlash(f.Path))
			file, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			b, err := io.ReadAll(io.LimitReader(file, f.Bytes+1))
			if err != nil {
				t.Fatal(err)
			}
			if int64(len(b)) != f.Bytes || fmt.Sprintf("%x", sha256.Sum256(b)) != f.SHA256 {
				t.Fatal("oracle fixture integrity")
			}
			comparePOI(t, b, poiRecords(t, path))
		})
	}
	t.Run("generated-wmf-objects", func(t *testing.T) {
		brush := testRecord(WMF, 0x02fc, 0, append(append(words(0), longs(0x123456)...), words(0)...))
		records := []Record{testRecord(WMF, 0x06ff, 0, wmfRegionFixture()), brush, wmfPaletteFixture(2), testRecord(WMF, 0x0234, 0, words(2)), testRecord(WMF, 0x0228, 0, words(0, 1)), testRecord(WMF, 0x0214, 0, words(-7, 9))}
		for n := 1; n <= len(records); n++ {
			t.Run(fmt.Sprint(n), func(t *testing.T) {
				b := wmfDocument(4, records[:n]...)
				path := filepath.Join(t.TempDir(), "objects.wmf")
				if err := os.WriteFile(path, b, 0600); err != nil {
					t.Fatal(err)
				}
				comparePOI(t, b, poiRecords(t, path))
			})
		}
	})
	t.Run("known-legacy-pattern-disagreement", func(t *testing.T) {
		bitmap := append(append(append(words(0, 8, 1, 2), 1, 1), make([]byte, 22)...), 0x80, 0)
		b := wmfDocument(1, testRecord(WMF, 0x01f9, 0, bitmap), testRecord(WMF, 0x0214, 0, words(2, 1)))
		if _, err := Stream(b, StreamOptions{}, nil); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "legacy-pattern.wmf")
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
		_, stderr, err := poiProcess(path)
		if err == nil || !strings.Contains(stderr, "unexpected record type") {
			t.Fatalf("POI legacy-pattern behavior changed; review discrepancy: %v %s", err, stderr)
		}
		t.Log("documented POI 5.4.1 mismatch: legacy pattern brush misframes the following record; excluded from agreement totals")
	})
	t.Run("known-palette-entry-order", func(t *testing.T) {
		// POI 5.4.1 reads palette entries as flags, blue, green, red in both
		// formats, following MS-EMF 2.2.18's LogPaletteEntry drawing. MS-WMF
		// 2.2.2.13 and GDI's PALETTEENTRY order them red, green, blue, flags,
		// which playback follows. Pin the observed reading; review on change.
		for name, data := range map[string][]byte{
			"palette.emf": emfScene(96, 64, 2, emfPalette(1, [4]byte{200, 40, 0, 0})),
			"palette.wmf": wmfScene(96, 64, 1, testRecord(WMF, MetaCreatePalette, 0, cat(words(0x300, 1), []byte{200, 40, 0, 0}))),
		} {
			path := filepath.Join(t.TempDir(), name)
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			var entry struct {
				Flags uint8
				Color int32
			}
			for _, r := range poiRecords(t, path) {
				if (r.Format == "EMF" && r.Type == 49) || (r.Format == "WMF" && r.Type == 0xf7) {
					var entries []json.RawMessage
					if err := json.Unmarshal(r.Body["pallete"], &entries); err != nil || len(entries) != 1 {
						t.Fatal(name, err, string(r.Body["pallete"]))
					}
					if err := json.Unmarshal(entries[0], &entry); err != nil {
						t.Fatal(err)
					}
				}
			}
			if entry.Flags != 200 || uint32(entry.Color)&0xffffff != 0x000028 {
				t.Fatalf("POI palette reading changed for %s; review the documented order discrepancy: %+v", name, entry)
			}
		}
		t.Log("documented POI 5.4.1 mismatch: palette entries read flags-first; playback uses PALETTEENTRY order")
	})
	// A generated path object exercises signed compressed coordinates. POI
	// 5.4.1 has only a flags-only stub for DrawLines, so that record cannot be
	// used as a relative-coordinate oracle (covered by spec-derived unit tests).
	pathData := append(longs(0, 2, 0x4000), words(-63, 64, 129, -130)...)
	put32(pathData, 0, 0xdbc01002)
	pathData = append(pathData, 0, 1, 0, 0)
	poly := testRecord(EMFPlus, 0x4008, 0x0301, pathData).Raw
	generated := emfFixture(plusComment(plusHeader(), poly, plusRecord(0x4002, nil)))
	t.Run("generated-path-plus", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "path.emf")
		if err := os.WriteFile(path, generated, 0600); err != nil {
			t.Fatal(err)
		}
		comparePOI(t, generated, poiRecords(t, path))
	})
	t.Run("generated-relative-path-plus", func(t *testing.T) {
		body := append(longs(0, 3, 0x800), []byte{0x3f, 0x40, 0x80, 0x40, 0xff, 0xbf, 0x7f, 1, 0x41, 0, 0x42, 1}...)
		put32(body, 0, 0xdbc01002)
		r := testRecord(EMFPlus, 0x4008, 0x0301, body)
		b := emfFixture(plusComment(plusHeader(), r.Raw, plusRecord(0x4002, nil)))
		path := filepath.Join(t.TempDir(), "relative.emf")
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
		// POI 5.4.1 disagrees with MS-EMFPLUS 2.2.2.21 here: 0x40 is a
		// signed 7-bit -64, not +64. It also exposes the RLE run byte as a
		// point type instead of expanding PathPointTypeRLE. Pin the observed
		// discrepancy explicitly; do not change the decoder to match it.
		oracle := poiRecords(t, path)
		var points []struct {
			Flags uint8
			Point Point
		}
		for _, r := range oracle {
			if r.Format == "EMF+" && r.Type == 0x4008 {
				var props map[string]json.RawMessage
				if err := json.Unmarshal(r.Body["objectData"], &props); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(props["points"], &points); err != nil {
					t.Fatal(err)
				}
			}
		}
		if len(points) != 3 || points[0].Point != (Point{63, 64}) || points[0].Flags != 0x41 {
			t.Fatal("POI behavior changed; review documented relative/RLE discrepancy", points)
		}
		v, err := DecodePlusObject(3, body, DecodeLimits{})
		if err != nil {
			t.Fatal(err)
		}
		got := v.(PlusPath)
		if got.Points.At(0) != (Point{63, -64}) || !bytes.Equal(got.Types, []byte{0, 1, 1}) {
			t.Fatal("spec-derived relative path", got)
		}
		t.Log("documented POI 5.4.1 mismatch: signed Integer7 and PathPointTypeRLE; excluded from agreement totals")
	})
}

func comparePOI(t *testing.T, b []byte, want []oracleRecord) {
	t.Helper()
	index, checks := 0, 0
	_, err := Walk(b, Limits{}, func(r Record) error {
		// POI's WMF record list excludes META_EOF; its EMF list includes EOF.
		if r.Format == WMF && r.Type == 0 {
			return nil
		}
		if index >= len(want) {
			t.Fatal("POI record list ended early")
		}
		w := want[index]
		index++
		format := map[Format]string{WMF: "WMF", EMF: "EMF", EMFPlus: "EMF+"}[r.Format]
		if w.Format != format || w.Type != r.Type {
			t.Fatalf("record %d: Go %s/%x; POI %s/%x", index, format, r.Type, w.Format, w.Type)
		}
		v, err := Decode(r, DecodeLimits{})
		if err != nil {
			return err
		}
		checkPoint := func(key string, p Point) bool {
			raw, ok := w.Body[key]
			if !ok {
				return false
			}
			var o struct{ X, Y, Width, Height float64 }
			if err := json.Unmarshal(raw, &o); err != nil {
				t.Fatal(err)
			}
			x, y := o.X, o.Y
			if key == "size" || key == "extents" {
				x, y = o.Width, o.Height
			}
			if p.X != x || p.Y != y {
				t.Fatalf("%s/%x %s: Go %+v; POI (%v,%v)", format, r.Type, key, p, x, y)
			}
			checks += 2
			return true
		}
		checkNumber := func(key string, n uint32) {
			raw, ok := w.Body[key]
			if !ok {
				return
			}
			var got int64
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			if got < -2147483648 || got > 4294967295 || uint32(got) != n {
				t.Fatalf("%s/%x %s: Go %d POI %d", format, r.Type, key, n, got)
			}
			checks++
		}
		switch x := v.(type) {
		case PointRecord:
			for _, key := range []string{"origin", "point", "size", "extents"} {
				if checkPoint(key, x.Point) {
					break
				}
			}
		case Poly:
			checks += comparePOIPoly(t, x, w.Body, r.Format == EMF && (r.Type == 6 || r.Type == 89))
		case Pen:
			checkNumber("penIndex", x.Handle)
		case Brush:
			checkNumber("brushIdx", x.Handle)
		case Font:
			checkNumber("fontIdx", x.Handle)
		case WMFRegion:
			checkNumber("regionSize", uint32(x.DeclaredBytes))
			checkNumber("scanCount", uint32(len(x.Scans)))
			checkNumber("maxScan", uint32(x.MaxScan))
		case Value:
			checkNumber("objectIndex", x.Value)
			if r.Format == WMF && r.Type&255 == 0x34 {
				checkNumber("paletteIndex", x.Value)
			}
		case Text:
			checkPoint("reference", x.Reference)
		case EMFPlusHeader:
			checkNumber("logicalDpiX", x.LogicalDpiX)
			checkNumber("logicalDpiY", x.LogicalDpiY)
		case PlusPoly:
			n := comparePOIPoly(t, Poly{Points: x.Points}, w.Body, false)
			if n == 0 {
				t.Fatalf("POI EMF+ geometry not compared: %s", w.Body)
			}
			checks += n
		case PlusObjectFragment:
			if x.Type == 3 {
				object, err := DecodePlusObject(x.Type, x.Data, DecodeLimits{})
				if err != nil {
					return err
				}
				var props map[string]json.RawMessage
				if err := json.Unmarshal(w.Body["objectData"], &props); err != nil {
					t.Fatalf("POI object data: %v %s", err, w.Body)
				}
				var points []struct {
					Flags uint8
					Point Point
				}
				if err := json.Unmarshal(props["points"], &points); err != nil {
					t.Fatal(err)
				}
				path := object.(PlusPath)
				if len(points) != path.Points.Len() {
					t.Fatal("POI object path point count")
				}
				for i, p := range points {
					if p.Point != path.Points.At(i) || p.Flags != path.Types[i] {
						t.Fatalf("object path point %d: POI %+v; Go %+v/%d", i, p, path.Points.At(i), path.Types[i])
					}
					checks += 3
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if index != len(want) {
		t.Fatalf("Go returned %d records, POI %d", index, len(want))
	}
	if checks == 0 {
		t.Fatal("no independent field comparisons")
	}
	t.Logf("compared %d record types and %d decoded field values", index, checks)
}

func comparePOIPoly(t *testing.T, p Poly, body map[string]json.RawMessage, implicitOrigin bool) int {
	t.Helper()
	type segment struct {
		Type string
		X, Y float64
	}
	var paths [][]segment
	if raw, ok := body["polyList"]; ok {
		if err := json.Unmarshal(raw, &paths); err != nil {
			t.Fatal(err)
		}
	} else {
		var raw json.RawMessage
		for _, key := range []string{"poly", "path"} {
			if b, ok := body[key]; ok {
				raw = b
				break
			}
		}
		if raw == nil {
			return 0
		}
		var path []segment
		if err := json.Unmarshal(raw, &path); err != nil {
			t.Fatal(err)
		}
		paths = [][]segment{path}
	}
	// Only line-based polygon families are compared here. Bezier normalization
	// is a separate oracle concern; cubic control points are not line vertices.
	for _, path := range paths {
		for _, v := range path {
			if v.Type != "move" && v.Type != "lineto" && v.Type != "close" {
				return 0
			}
		}
	}
	pos := 0
	for i, path := range paths {
		if implicitOrigin {
			if len(path) == 0 || path[0].Type != "move" {
				t.Fatal("missing POI PolylineTo implicit origin")
			}
			path = path[1:]
		}
		n := p.Points.Len()
		if p.Counts.Len() != 0 {
			if i >= p.Counts.Len() {
				t.Fatal("POI has extra polygons")
			}
			n = int(p.Counts.At(i))
		}
		if len(path) < n {
			t.Fatalf("POI path too short: %d < %d", len(path), n)
		}
		for j := 0; j < n; j++ {
			q := p.Points.At(pos)
			if path[j].X != q.X || path[j].Y != q.Y {
				t.Fatalf("point %d: Go %+v; POI %+v", pos, q, path[j])
			}
			pos++
		}
	}
	if pos != p.Points.Len() {
		t.Fatalf("POI point count %d; Go %d", pos, p.Points.Len())
	}
	return pos * 2
}
