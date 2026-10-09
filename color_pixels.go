package gowemf

import (
	"fmt"

	lcms2 "github.com/mgilbir/golittlecms"
)

// ColorPixelFormat specifies interleaved, unsigned eight-bit source channels.
// CMYK8 uses 0=no ink and 255=full ink. Output is always straight sRGB RGBA8.
type ColorPixelFormat uint8

const (
	ColorRGB8 ColorPixelFormat = iota + 1
	ColorRGBA8
	ColorGray8
	ColorCMYK8
)

type PixelColorTransform struct {
	transform *lcms2.Transform
	channels  int
	copyAlpha bool
	maxPixels uint64
}

func (t *PixelColorTransform) InputChannels() int {
	if t == nil {
		return 0
	}
	return t.channels
}
func (t *PixelColorTransform) TransformPixels(dst, src []byte) error {
	if t == nil || t.transform == nil || len(src)%t.channels != 0 {
		return malformed(0, "color pixel source")
	}
	n := uint64(len(src) / t.channels)
	if n > t.maxPixels || n > uint64(^uint32(0))/4 || n > uint64(int(^uint(0)>>1))/4 {
		return failure(0, "color pixel count", ErrLimit)
	}
	if uint64(len(dst)) != n*4 {
		return malformed(0, "color pixel destination")
	}
	t.transform.DoTransform(src, dst, uint32(n))
	if !t.copyAlpha {
		for i := 3; i < len(dst); i += 4 {
			dst[i] = 255
		}
	}
	return nil
}

// NewPixelColorTransform builds an explicit 8-bit RGB/RGBA/gray/CMYK-to-sRGB
// converter. The ICC profile must agree with the declared source channel model.
func NewPixelColorTransform(space ColorSpace, format ColorPixelFormat, options ColorTransformOptions) (*PixelColorTransform, error) {
	o := options.defaults()
	intent, err := space.ICCIntent()
	if err != nil {
		return nil, err
	}
	var inputFormat uint32
	var signature lcms2.ColorSpaceSignature
	channels := 0
	alpha := false
	switch format {
	case ColorRGB8:
		inputFormat, signature, channels = lcms2.TypeRGB8, lcms2.SigRgbData, 3
	case ColorRGBA8:
		inputFormat, signature, channels, alpha = lcms2.TypeRGBA8, lcms2.SigRgbData, 4, true
	case ColorGray8:
		inputFormat, signature, channels = lcms2.TypeGray8, lcms2.SigGrayData, 1
	case ColorCMYK8:
		inputFormat, signature, channels = lcms2.TypeCMYK8, lcms2.SigCmykData, 4
	default:
		return nil, failure(0, "color pixel format", ErrUnsupported)
	}
	input, err := openColorProfile(space, o)
	if err != nil {
		return nil, err
	}
	defer input.CloseProfile()
	if input.GetColorSpace() != signature {
		return nil, malformed(0, "profile/input channel mismatch")
	}
	output, err := lcms2.Create_sRGBProfile()
	if err != nil {
		return nil, err
	}
	defer output.CloseProfile()
	flags := lcms2.FlagsNoCache
	if alpha {
		flags |= lcms2.FlagsCopyAlpha
	}
	if o.BlackPointCompensation {
		flags |= lcms2.FlagsBlackPointCompensation
	}
	transform, err := lcms2.CreateTransform(input, inputFormat, output, lcms2.TypeRGBA8, intent, flags)
	if err != nil {
		return nil, fmt.Errorf("pixel color transform: %w", err)
	}
	return &PixelColorTransform{transform, channels, alpha, o.MaxPixels}, nil
}

// NewProofingColorTransform converts source RGB through an explicitly supplied
// proofing profile to sRGB. The proof profile can describe a CMYK printer, gray
// device or RGB output. Rendering/proof intents are mapped from each ColorSpace;
// no profile names are opened except through the supplied resolver callback.
func NewProofingColorTransform(source, proof ColorSpace, options ColorTransformOptions) (RGBTransform, error) {
	o := options.defaults()
	intent, err := source.ICCIntent()
	if err != nil {
		return nil, err
	}
	proofIntent, err := proof.ICCIntent()
	if err != nil {
		return nil, err
	}
	in, err := openColorProfile(source, o)
	if err != nil {
		return nil, err
	}
	defer in.CloseProfile()
	if in.GetColorSpace() != lcms2.SigRgbData {
		return nil, malformed(0, "proofing source must be RGB")
	}
	proofProfile, err := openColorProfile(proof, o)
	if err != nil {
		return nil, err
	}
	defer proofProfile.CloseProfile()
	out, err := lcms2.Create_sRGBProfile()
	if err != nil {
		return nil, err
	}
	defer out.CloseProfile()
	flags := lcms2.FlagsSoftProofing | lcms2.FlagsCopyAlpha | lcms2.FlagsNoCache
	if o.BlackPointCompensation {
		flags |= lcms2.FlagsBlackPointCompensation
	}
	transform, err := lcms2.CreateProofingTransform(in, lcms2.TypeRGBA8, out, lcms2.TypeRGBA8, proofProfile, intent, proofIntent, flags)
	if err != nil {
		return nil, fmt.Errorf("proofing transform: %w", err)
	}
	return &cmsRGBTransform{transform, o.MaxPixels}, nil
}
