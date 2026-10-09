package gowemf

import (
	"bytes"
	"errors"
	"math"
	"testing"
)

// Packet GUID form uses little-endian Data1/Data2/Data3 and eight literal bytes.
var blurGUIDWire = []byte{0xa4, 0x80, 0x3c, 0x63, 0x43, 0x18, 0x2b, 0x48, 0x9e, 0xf2, 0xbe, 0x28, 0x34, 0xc5, 0xfd, 0xd4}

func effectRecord(guid, payload []byte) Record {
	b := append(append([]byte(nil), guid...), longs(int32(len(payload)))...)
	b = append(b, payload...)
	return testRecord(EMFPlus, PlusSerializableObjectRecord, 0, b)
}

func TestSerializableBlurEffect(t *testing.T) {
	r := effectRecord(blurGUIDWire, append(floats(3.5), longs(1)...))
	v := mustDecode(t, r).(PlusEffect)
	if v.Kind != EffectBlur || v.GUID.String() != "{633C80A4-1843-482B-9EF2-BE2834C5FDD4}" || v.Parameters.(BlurEffect) != (BlurEffect{3.5, true}) {
		t.Fatal(v)
	}
	put32(r.Raw, 28, 0xffffffff)
	if _, err := Decode(r, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("unbounded effect buffer", err)
	}
}

func effectGUIDBytes(kind EffectKind) []byte {
	g := kind.GUID()
	b := make([]byte, 16)
	put32(b, 0, g.Data1)
	put16(b, 4, g.Data2)
	put16(b, 6, g.Data3)
	copy(b[8:], g.Data4[:])
	return b
}

func effectFixtures() map[EffectKind][]byte {
	matrix := make([]byte, 100)
	for i := 0; i < 5; i++ {
		put32(matrix, (i*5+i)*4, math.Float32bits(1))
	}
	put32(matrix, 80, math.Float32bits(.25))
	put32(matrix, 96, math.Float32bits(7)) // last entry is SHOULD, not MUST, one
	lut := make([]byte, 1024)
	for i := 0; i < 256; i++ {
		lut[i] = byte(i)
		lut[256+i] = byte(255 - i)
		lut[512+i] = byte(i / 2)
		lut[768+i] = 255
	}
	return map[EffectKind][]byte{
		EffectBlur: append(floats(3.5), longs(1)...), EffectBrightnessContrast: longs(-64, 50), EffectColorBalance: longs(-1, 2, -3), EffectColorCurve: longs(7, 2, 42), EffectColorLookupTable: lut, EffectColorMatrix: matrix,
		EffectHueSaturationLightness: longs(-180, 50, 75), EffectLevels: longs(80, -5, 10), EffectRedEyeCorrection: longs(2, -1, -2, 3, 4, 9, 10, 11, 12), EffectSharpen: floats(2.5, 100), EffectTint: longs(10, -50),
	}
}

func TestAllSerializableEffects(t *testing.T) {
	for kind, payload := range effectFixtures() {
		r := effectRecord(effectGUIDBytes(kind), payload)
		r.Flags = 0xffff
		put16(r.Raw, 2, r.Flags)
		v := mustDecode(t, r).(PlusEffect)
		if v.Kind != kind || v.GUID != kind.GUID() {
			t.Fatal(kind, v)
		}
		switch p := v.Parameters.(type) {
		case BlurEffect:
			if p.Radius != 3.5 || !p.ExpandEdge {
				t.Fatal(p)
			}
		case BrightnessContrastEffect:
			if p.Brightness != -64 || p.Contrast != 50 {
				t.Fatal(p)
			}
		case ColorBalanceEffect:
			if p != (ColorBalanceEffect{-1, 2, -3}) {
				t.Fatal(p)
			}
		case ColorCurveEffect:
			if p != (ColorCurveEffect{7, 2, 42}) {
				t.Fatal(p)
			}
		case ColorLookupTableEffect:
			if p.Blue[7] != 7 || p.Green[7] != 248 || p.Red[7] != 3 || p.Alpha[7] != 255 || cap(p.Blue) != 256 || &p.Blue[0] != &r.Raw[32] {
				t.Fatal("lookup channel order/ownership")
			}
		case ColorMatrixEffect:
			if p.Rows[4][0] != .25 || p.Rows[0][4] != 0 || p.Rows[4][4] != 7 {
				t.Fatal(p)
			}
		case HueSaturationLightnessEffect:
			if p != (HueSaturationLightnessEffect{-180, 50, 75}) {
				t.Fatal(p)
			}
		case LevelsEffect:
			if p != (LevelsEffect{80, -5, 10}) {
				t.Fatal(p)
			}
		case RedEyeCorrectionEffect:
			if p.Areas.Len() != 2 || p.Areas.At(0) != (Rect{-1, -2, 3, 4}) || p.Areas.At(1) != (Rect{9, 10, 11, 12}) {
				t.Fatal(p)
			}
		case SharpenEffect:
			if p != (SharpenEffect{2.5, 100}) {
				t.Fatal(p)
			}
		case TintEffect:
			if p != (TintEffect{10, -50}) {
				t.Fatal(p)
			}
		default:
			t.Fatalf("effect %d has unexpected body %T", kind, p)
		}
		for n := 0; n < len(payload); n += 4 {
			bad := effectRecord(effectGUIDBytes(kind), payload[:n])
			if _, err := Decode(bad, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
				t.Fatalf("effect %d accepted %d-byte prefix: %v", kind, n, err)
			}
		}
		extra := effectRecord(effectGUIDBytes(kind), append(append([]byte(nil), payload...), 0, 0, 0, 0))
		if _, err := Decode(extra, DecodeLimits{}); !errors.Is(err, ErrMalformed) {
			t.Fatalf("effect %d accepted extra parameters: %v", kind, err)
		}
	}
}

func TestEffectParameterValidation(t *testing.T) {
	for _, tc := range []struct {
		kind EffectKind
		data []byte
	}{
		{EffectBlur, append(floats(256), longs(0)...)}, {EffectBlur, append(floats(1), longs(2)...)}, {EffectBlur, append(floats(float32(math.NaN())), longs(0)...)},
		{EffectBrightnessContrast, longs(-256, 0)}, {EffectColorBalance, longs(0, 101, 0)}, {EffectColorCurve, longs(8, 0, 0)}, {EffectColorCurve, longs(0, 4, 0)}, {EffectColorCurve, longs(7, 0, -1)},
		{EffectHueSaturationLightness, longs(181, 0, 0)}, {EffectLevels, longs(101, 0, 0)}, {EffectRedEyeCorrection, longs(-1)}, {EffectSharpen, floats(1, 101)}, {EffectTint, longs(0, -101)},
	} {
		if _, err := Decode(effectRecord(effectGUIDBytes(tc.kind), tc.data), DecodeLimits{}); !errors.Is(err, ErrMalformed) {
			t.Fatal(tc.kind, err)
		}
	}
	matrix := effectFixtures()[EffectColorMatrix]
	put32(matrix, 16, math.Float32bits(1))
	if _, err := Decode(effectRecord(effectGUIDBytes(EffectColorMatrix), matrix), DecodeLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("non-affine color matrix", err)
	}
	badGUID := append([]byte(nil), blurGUIDWire...)
	badGUID[15] ^= 1
	if _, err := Decode(effectRecord(badGUID, append(floats(1), longs(0)...)), DecodeLimits{}); !errors.Is(err, ErrUnsupported) {
		t.Fatal("partially matching GUID", err)
	}
	redEye := effectRecord(effectGUIDBytes(EffectRedEyeCorrection), effectFixtures()[EffectRedEyeCorrection])
	if _, err := Decode(redEye, DecodeLimits{MaxElements: 1}); !errors.Is(err, ErrLimit) {
		t.Fatal("red-eye array limit", err)
	}
	if bytes.Equal(effectGUIDBytes(EffectBlur), effectGUIDBytes(EffectTint)) {
		t.Fatal("effect IDs collide")
	}
}

func TestDrawImageIgnoresReservedEffectFlag(t *testing.T) {
	body := append(longs(-1, 2), floats(0, 0, 1, 1, 0, 0, 10, 10)...)
	v := mustDecode(t, testRecord(EMFPlus, PlusDrawImageRecord, 0x2000, body)).(PlusImageDraw)
	if v.Effect {
		t.Fatal("DrawImage reserved bit was treated as an effect flag")
	}
}

func effectImageDrawFixture() ([]byte, []byte) {
	imageData := rawPlusImageObject(PlusImage{Type: 1, Width: 1, Height: 1, Stride: 4, PixelFormat: PixelFormat32bppARGB, Data: []byte{0, 0, 0, 255}})
	object := testRecord(EMFPlus, PlusObjectRecord, 0x0500, imageData).Raw
	body := append(longs(-1, 2), floats(0, 0, 1, 1)...)
	body = append(body, longs(3)...)
	body = append(body, floats(0, 0, 10, 0, 0, 10)...)
	return object, testRecord(EMFPlus, PlusDrawImagePointsRecord, 0x2000, body).Raw
}

func TestStreamRequiresEarlierImageEffect(t *testing.T) {
	object, draw := effectImageDrawFixture()
	file := emfFixture(plusComment(plusHeader(), object, draw, plusRecord(PlusEndOfFileRecord, nil)))
	if _, err := Stream(file, StreamOptions{}, nil); !errors.Is(err, ErrMalformed) {
		t.Fatal("image effect flag without earlier effect was accepted", err)
	}
}

func TestStreamImageEffectBinding(t *testing.T) {
	object, draw := effectImageDrawFixture()
	blur := effectRecord(blurGUIDWire, append(floats(1), longs(0)...)).Raw
	tint := effectRecord(effectGUIDBytes(EffectTint), longs(10, -50)).Raw
	plain := testRecord(EMFPlus, PlusDrawImageRecord, 0x2000, append(longs(-1, 2), floats(0, 0, 1, 1, 0, 0, 10, 10)...)).Raw
	file := emfFixture(plusComment(plusHeader(), object, blur, draw, tint, plain, draw, plusRecord(PlusEndOfFileRecord, nil)))
	var kinds []EffectKind
	if _, err := Stream(file, StreamOptions{}, func(c Command) error {
		if c.Source.Type == PlusDrawImagePointsRecord {
			if c.Effect == nil {
				t.Fatal("missing bound effect")
			}
			kinds = append(kinds, c.Effect.Kind)
		} else if c.Source.Type == PlusDrawImageRecord && c.Effect != nil {
			t.Fatal("effect applied to reserved DrawImage flag")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(kinds) != 2 || kinds[0] != EffectBlur || kinds[1] != EffectTint {
		t.Fatal(kinds)
	}
}
