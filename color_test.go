package gowemf

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"math"
	"sync"
	"testing"

	lcms2 "github.com/mgilbir/golittlecms"
)

func linearRGBSpace() ColorSpace {
	return ColorSpace{Type: ColorCalibratedRGB, Intent: IntentGraphics, Endpoints: [3]XYZ{{.4124564, .2126729, .0193339}, {.3575761, .7151522, .1191920}, {.1804375, .0721750, .9503041}}, Gamma: [3]float64{1, 1, 1}}
}
func generatedICC(t testing.TB) []byte {
	t.Helper()
	p, err := lcms2.Create_sRGBProfile()
	if err != nil {
		t.Fatal(err)
	}
	defer p.CloseProfile()
	b, err := p.SaveProfileToMem()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func FuzzColorTransform(f *testing.F) {
	f.Add(generatedICC(f))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			return
		}
		transform, err := NewColorTransformWithOptions(ColorSpace{Type: ColorProfileEmbedded, Profile: data}, ColorTransformOptions{MaxProfileBytes: 64 << 10, MaxProfileTags: 64, MaxPixels: 16})
		if err == nil {
			if err := transform.TransformRGBA(make([]byte, 16), []byte{0, 0, 0, 0, 255, 255, 255, 255, 128, 128, 128, 93, 10, 20, 30, 40}); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func colorSpaceRecord(unicode bool) Record {
	n := 328
	typ := uint32(EMRCreateColorSpace)
	if unicode {
		n = 588
		typ = EMRCreateColorSpaceW
	}
	b := make([]byte, 4+n)
	put32(b, 0, 1)
	put32(b, 4, 0x50534f43)
	put32(b, 8, 0x400)
	put32(b, 12, uint32(n))
	put32(b, 16, ColorCalibratedRGB)
	put32(b, 20, IntentGraphics)
	s := linearRGBSpace()
	for i, p := range s.Endpoints {
		for j, v := range []float64{p.X, p.Y, p.Z} {
			put32(b, 24+i*12+j*4, uint32(int32(math.Round(v*(1<<30)))))
		}
	}
	for i := 0; i < 3; i++ {
		put32(b, 60+i*4, 1<<16)
	}
	if unicode {
		b = append(b, longs(0, 0)...)
	}
	return testRecord(EMF, typ, 0, b)
}

func TestColorRecordsAndLifetimes(t *testing.T) {
	for _, unicode := range []bool{false, true} {
		r := colorSpaceRecord(unicode)
		v := mustDecode(t, r).(ColorSpaceObject)
		if v.Handle != 1 || v.Space.Unicode != unicode || v.Space.Gamma != [3]float64{1, 1, 1} || math.Abs(v.Space.Endpoints[0].X-.4124564) > 1e-8 {
			t.Fatal(v)
		}
		file := emfFixture(r.Raw, emfRecord(EMRSetColorSpace, longs(1)), emfRecord(EMRDeleteColorSpace, longs(1)))
		if _, err := Stream(file, StreamOptions{}, nil); err != nil {
			t.Fatal(err)
		}
		file = emfFixture(r.Raw, emfRecord(EMRDeleteObject, longs(1)), emfRecord(EMRSetColorSpace, longs(1)))
		if _, err := Stream(file, StreamOptions{}, nil); !errors.Is(err, ErrMalformed) {
			t.Fatal("deleted color-space handle", err)
		}
		file = emfFixture(r.Raw, emfRecord(EMRSelectObject, longs(1)))
		if _, err := Stream(file, StreamOptions{}, nil); !errors.Is(err, ErrMalformed) {
			t.Fatal("wrong color-space selection opcode", err)
		}
	}
	name := words('x', 0)
	data := []byte{1, 2, 3, 4}
	r := testRecord(EMF, EMRSetICMProfileW, 0, append(append(longs(1, int32(len(name)), int32(len(data))), name...), data...))
	p := mustDecode(t, r).(ColorProfile)
	if !p.Unicode || !bytes.Equal(p.Name, name) || !bytes.Equal(p.Data, data) {
		t.Fatal(p)
	}
	put32(r.Raw, 12, 0xffffffff)
	if _, err := Decode(r, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("unbounded profile name", err)
	}
	r = testRecord(EMF, EMRSetColorAdjustment, 0, words(24, 0, 0, 10000, 10000, 10000, 0, 10000, -100, 100, 0, -1))
	if a := mustDecode(t, r).(ColorAdjustment); a.Contrast != -100 || a.RedGreenTint != -1 || a.ReferenceWhite != 10000 {
		t.Fatal(a)
	}
}

func TestColorTransformAnalyticAndAlpha(t *testing.T) {
	transform, err := NewColorTransform(linearRGBSpace())
	if err != nil {
		t.Fatal(err)
	}
	src := []byte{128, 128, 128, 93, 0, 0, 0, 0, 255, 255, 255, 255}
	dst := make([]byte, len(src))
	if err := transform.TransformRGBA(dst, src); err != nil {
		t.Fatal(err)
	}
	for _, v := range dst[:3] {
		if v < 187 || v > 189 {
			t.Fatalf("linear 0.5 should map to sRGB ~188, got %v", dst)
		}
	}
	if dst[3] != 93 || !bytes.Equal(dst[4:], src[4:]) {
		t.Fatal("alpha/endpoints changed", dst)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out := make([]byte, len(src))
			if err := transform.TransformRGBA(out, src); err != nil || !bytes.Equal(out, dst) {
				t.Error("concurrent transform", err)
			}
		}()
	}
	wg.Wait()
	if err := transform.TransformRGBA(make([]byte, 3), make([]byte, 3)); !errors.Is(err, ErrMalformed) {
		t.Fatal(err)
	}
	limited, err := NewColorTransformWithOptions(ColorSpace{Type: ColorSRGB}, ColorTransformOptions{MaxPixels: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := limited.TransformRGBA(make([]byte, 8), make([]byte, 8)); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
}

func TestEmbeddedICCAndResolver(t *testing.T) {
	profile := generatedICC(t)
	s := ColorSpace{Type: ColorProfileEmbedded, Profile: profile, Intent: IntentImages}
	transform, err := NewColorTransform(s)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte{10, 20, 30, 40, 200, 150, 100, 0}
	dst := make([]byte, len(src))
	if err := transform.TransformRGBA(dst, src); err != nil || !bytes.Equal(src, dst) {
		t.Fatal(dst, err)
	}
	for _, options := range []ColorTransformOptions{{MaxProfileBytes: 1}, {MaxProfileTags: 1}} {
		if _, err := NewColorTransformWithOptions(s, options); !errors.Is(err, ErrLimit) {
			t.Fatal(err)
		}
	}
	bad := append([]byte(nil), profile...)
	binary.BigEndian.PutUint32(bad[136:], 0xfffffffc)
	if _, err := NewColorTransform(ColorSpace{Type: ColorProfileEmbedded, Profile: bad}); !errors.Is(err, ErrMalformed) {
		t.Fatal("ICC tag span", err)
	}
	s = ColorSpace{Type: ColorProfileLinked, Name: []byte("profile.icc\x00")}
	if _, err := NewColorTransform(s); !errors.Is(err, ErrUnsupported) {
		t.Fatal("implicit profile path resolution", err)
	}
	calls := 0
	if _, err := NewColorTransformWithOptions(s, ColorTransformOptions{ResolveProfile: func(got ColorSpace) ([]byte, error) {
		calls++
		if !bytes.Equal(got.Name, s.Name) {
			t.Fatal(got)
		}
		return profile, nil
	}}); err != nil || calls != 1 {
		t.Fatal(calls, err)
	}
	for windows, want := range map[uint32]uint32{1: 2, 2: 1, 4: 0, 8: 3} {
		got, err := (ColorSpace{Intent: windows}).ICCIntent()
		if err != nil || got != want {
			t.Fatal(windows, got, err)
		}
	}
}

func profileDIB(profile []byte, packed bool) ([]byte, []byte) {
	b := make([]byte, 124)
	copy(b, dibHeader(1, -1, 24, 0))
	put32(b, 0, 124)
	put32(b, 56, ColorProfileEmbedded)
	put32(b, 108, IntentGraphics)
	pixels := []byte{10, 20, 30, 0}
	off := 124
	if packed {
		off += 4
		b = append(b, pixels...)
	}
	put32(b, 112, uint32(off))
	put32(b, 116, uint32(len(profile)))
	b = append(b, profile...)
	return b, pixels
}

func TestDIBProfileExtractionAndConversion(t *testing.T) {
	profile := generatedICC(t)
	for _, packed := range []bool{false, true} {
		info, pixels := profileDIB(profile, packed)
		var d *DIB
		var err error
		if packed {
			d, err = ParsePackedDIB(info, 0, nil, ImageLimits{})
		} else {
			d, err = ParseDIB(info, pixels, 0, nil, ImageLimits{})
		}
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(d.ColorSpace().Profile, profile) {
			t.Fatal("wrong profile slice")
		}
		if _, err := d.Image(); !errors.Is(err, ErrUnsupported) {
			t.Fatal("unmanaged tagged pixels returned", err)
		}
		im, err := d.ImageWithColorTransform(NewColorTransform)
		if err != nil {
			t.Fatal(err)
		}
		if got := color.NRGBAModel.Convert(im.At(0, 0)); got != (color.NRGBA{30, 20, 10, 255}) {
			t.Fatal(got)
		}
	}
	for _, off := range []uint32{120, 124, 0xffffffff} {
		info, _ := profileDIB(profile, true)
		put32(info, 112, off)
		if _, err := ParsePackedDIB(info, 0, nil, ImageLimits{}); !errors.Is(err, ErrMalformed) {
			t.Fatal("profile overlap/range", off, err)
		}
	}
	im := image.NewNRGBA(image.Rect(10, 20, 12, 22))
	im.SetNRGBA(10, 20, color.NRGBA{128, 128, 128, 93})
	out, err := ConvertToSRGB(im, linearRGBSpace(), NewColorTransform, ImageLimits{})
	if err != nil || out.Bounds() != image.Rect(0, 0, 2, 2) || out.NRGBAAt(0, 0).A != 93 {
		t.Fatal(out, err)
	}
	if _, err := ConvertToSRGB(im, linearRGBSpace(), NewColorTransform, ImageLimits{MaxPixels: 3}); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
}

func BenchmarkColorTransform(b *testing.B) {
	transform, err := NewColorTransform(linearRGBSpace())
	if err != nil {
		b.Fatal(err)
	}
	src, dst := make([]byte, 4096*4), make([]byte, 4096*4)
	for i := range src {
		src[i] = byte(i)
	}
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := transform.TransformRGBA(dst, src); err != nil {
			b.Fatal(err)
		}
	}
}
