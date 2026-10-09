package gowemf

import (
	"bytes"
	"errors"
	"math"
	"testing"

	lcms2 "github.com/mgilbir/golittlecms"
)

func generatedGrayICC(t testing.TB) []byte {
	t.Helper()
	curve, err := lcms2.BuildGamma(1)
	if err != nil {
		t.Fatal(err)
	}
	white := lcms2.CIExyY{X: .3457, Y: .3585, YY: 1}
	p, err := lcms2.CreateGrayProfile(&white, curve)
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
func generatedCMYKICC(t testing.TB) []byte {
	t.Helper()
	p := lcms2.CreateProfilePlaceholder()
	defer p.CloseProfile()
	p.SetProfileVersion(2.1)
	p.SetDeviceClass(lcms2.SigInputClass)
	p.SetColorSpace(lcms2.SigCmykData)
	p.SetPCS(lcms2.SigXYZData)
	white := lcms2.CIEXYZ{X: .9642, Y: 1, Z: .8249}
	if err := p.WriteTag(lcms2.SigMediaWhitePointTag, &white); err != nil {
		t.Fatal(err)
	}
	// A generated four-channel test device, not a real printer characterization.
	// At cube corners C/M/Y suppress the corresponding linear RGB primary; K
	// suppresses all three. The PCS table uses standard D50-adapted RGB columns.
	var table []uint16
	for c := 0; c < 2; c++ {
		for m := 0; m < 2; m++ {
			for y := 0; y < 2; y++ {
				for k := 0; k < 2; k++ {
					r, g, b := float64((1-c)*(1-k)), float64((1-m)*(1-k)), float64((1-y)*(1-k))
					for _, v := range []float64{.4360747*r + .3850649*g + .1430804*b, .2225045*r + .7168786*g + .0606169*b, .0139322*r + .0971045*g + .7141733*b} {
						table = append(table, uint16(math.Round(v*32768)))
					}
				}
			}
		}
	}
	pipe, err := lcms2.PipelineAlloc(4, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Free()
	in, err := lcms2.StageAllocToneCurves(4, nil)
	if err != nil {
		t.Fatal(err)
	}
	clut, err := lcms2.StageAllocCLut16bit(2, 4, 3, table)
	if err != nil {
		t.Fatal(err)
	}
	out, err := lcms2.StageAllocToneCurves(3, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range []*lcms2.Stage{in, clut, out} {
		if err := pipe.InsertStage(lcms2.AtEnd, stage); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.WriteTag(lcms2.SigAToB0Tag, pipe); err != nil {
		t.Fatal(err)
	}
	data, err := p.SaveProfileToMem()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGrayAndCMYKColorTransforms(t *testing.T) {
	gray := ColorSpace{Type: ColorProfileEmbedded, Profile: generatedGrayICC(t), Intent: IntentGraphics}
	tr, err := NewPixelColorTransform(gray, ColorGray8, ColorTransformOptions{})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]byte, 12)
	if err := tr.TransformPixels(out, []byte{0, 128, 255}); err != nil {
		t.Fatal(err)
	}
	if out[0] != 0 || out[4] < 186 || out[4] > 190 || out[8] != 255 || out[3] != 255 || out[7] != 255 || out[11] != 255 {
		t.Fatal("gray ICC ramp", out)
	}
	cmyk := ColorSpace{Type: ColorProfileEmbedded, Profile: generatedCMYKICC(t), Intent: IntentGraphics}
	tr, err = NewPixelColorTransform(cmyk, ColorCMYK8, ColorTransformOptions{})
	if err != nil {
		t.Fatal(err)
	}
	out = make([]byte, 12)
	if err := tr.TransformPixels(out, []byte{0, 0, 0, 0, 0, 0, 0, 255, 255, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	if out[0] < 250 || out[1] < 250 || out[2] < 250 || out[4] > 3 || out[5] > 3 || out[6] > 3 || out[8] > 5 || out[9] < 250 || out[10] < 250 {
		t.Fatal("CMYK channel order/ink convention", out)
	}
	if _, err := NewPixelColorTransform(gray, ColorCMYK8, ColorTransformOptions{}); !errors.Is(err, ErrMalformed) {
		t.Fatal("profile/channel mismatch", err)
	}
	if err := tr.TransformPixels(make([]byte, 4), []byte{1, 2, 3}); !errors.Is(err, ErrMalformed) {
		t.Fatal(err)
	}
}

func TestSoftProofingIdentity(t *testing.T) {
	s := ColorSpace{Type: ColorSRGB, Intent: IntentGraphics}
	proof := ColorSpace{Type: ColorProfileEmbedded, Profile: generatedICC(t), Intent: IntentAbsoluteColorimetric}
	tr, err := NewProofingColorTransform(s, proof, ColorTransformOptions{BlackPointCompensation: true})
	if err != nil {
		t.Fatal(err)
	}
	src := []byte{20, 40, 60, 80, 200, 150, 100, 255}
	dst := make([]byte, len(src))
	if err := tr.TransformRGBA(dst, src); err != nil {
		t.Fatal(err)
	}
	for i, v := range src {
		d := int(v) - int(dst[i])
		if d < -2 || d > 2 {
			t.Fatal("sRGB proof changed color", src, dst)
		}
	}
	if !bytes.Equal([]byte{src[3], src[7]}, []byte{dst[3], dst[7]}) {
		t.Fatal("proofing lost alpha")
	}
	if _, err := NewProofingColorTransform(s, proof, ColorTransformOptions{MaxProfileBytes: 1}); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
}
