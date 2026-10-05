package gowemf

import (
	"encoding/binary"
	"fmt"

	lcms2 "github.com/mgilbir/golittlecms"
)

// ColorTransformOptions adds profile/transform limits. Defaults: 16 MiB profile
// bytes (also the sum of referenced tag sizes), 4096 tags, 16 million pixels per
// TransformRGBA call. golittlecms retains its own allocation and parser guards.
// ResolveProfile is optional and is invoked only for a named profile without
// embedded bytes. The application decides whether/how to resolve the opaque name;
// neither this adapter nor the parser opens a path supplied by a metafile.
type ColorTransformOptions struct {
	MaxProfileBytes, MaxPixels uint64
	MaxProfileTags             uint32
	BlackPointCompensation     bool
	ResolveProfile             func(ColorSpace) ([]byte, error)
}

func (o ColorTransformOptions) defaults() ColorTransformOptions {
	if o.MaxProfileBytes == 0 {
		o.MaxProfileBytes = 16 << 20
	}
	if o.MaxProfileTags == 0 {
		o.MaxProfileTags = 4096
	}
	if o.MaxPixels == 0 {
		o.MaxPixels = 16_000_000
	}
	return o
}

// NewColorTransform creates a golittlecms RGB-to-sRGB converter. It supports
// embedded RGB ICC profiles, calibrated RGB endpoints/gamma and sRGB. Windows
// intent values are explicitly mapped to ICC intents rather than cast.
func NewColorTransform(space ColorSpace) (RGBTransform, error) {
	return NewColorTransformWithOptions(space, ColorTransformOptions{})
}

func NewColorTransformWithOptions(space ColorSpace, options ColorTransformOptions) (RGBTransform, error) {
	o := options.defaults()
	intent, err := space.ICCIntent()
	if err != nil {
		return nil, err
	}
	input, err := openColorProfile(space, o)
	if err != nil {
		return nil, err
	}
	defer input.CloseProfile()
	if input.GetColorSpace() != lcms2.SigRgbData {
		return nil, failure(0, "RGB source profile required", ErrUnsupported)
	}
	output, err := lcms2.Create_sRGBProfile()
	if err != nil {
		return nil, err
	}
	defer output.CloseProfile()
	flags := lcms2.FlagsCopyAlpha | lcms2.FlagsNoCache
	if o.BlackPointCompensation {
		flags |= lcms2.FlagsBlackPointCompensation
	}
	transform, err := lcms2.CreateTransform(input, lcms2.TypeRGBA8, output, lcms2.TypeRGBA8, intent, flags)
	if err != nil {
		return nil, fmt.Errorf("color transform: %w", err)
	}
	return &cmsRGBTransform{transform, o.MaxPixels}, nil
}

type cmsRGBTransform struct {
	transform *lcms2.Transform
	maxPixels uint64
}

func (t *cmsRGBTransform) TransformRGBA(dst, src []byte) error {
	if len(src)%4 != 0 || len(dst) != len(src) {
		return malformed(0, "RGBA transform buffer length")
	}
	n := uint64(len(src) / 4)
	if n > t.maxPixels || n > uint64(^uint32(0))/4 {
		return failure(0, "color transform pixels", ErrLimit)
	}
	t.transform.DoTransform(src, dst, uint32(n))
	return nil
}

func openColorProfile(s ColorSpace, o ColorTransformOptions) (*lcms2.Profile, error) {
	if uint64(len(s.Name))+uint64(len(s.Profile)) > o.MaxProfileBytes {
		return nil, failure(0, "color profile bytes", ErrLimit)
	}
	if s.IsSRGB() {
		return lcms2.Create_sRGBProfile()
	}
	if s.Type != ColorCalibratedRGB && s.Type != ColorProfileEmbedded && s.Type != ColorProfileLinked {
		return nil, failure(0, "source color space", ErrUnsupported)
	}
	data := s.Profile
	hasName := len(s.Name) > 0 && s.Name[0] != 0
	if s.Unicode {
		if len(s.Name)%2 != 0 {
			return nil, malformed(0, "UTF-16 profile name")
		}
		hasName = len(s.Name) >= 2 && u16(s.Name) != 0
	}
	if len(data) == 0 && hasName {
		if o.ResolveProfile == nil {
			return nil, failure(0, "named color profile requires a resolver", ErrUnsupported)
		}
		var err error
		data, err = o.ResolveProfile(s)
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			return nil, malformed(0, "empty resolved profile")
		}
	}
	if len(data) != 0 {
		checked, err := checkICCEnvelope(data, o)
		if err != nil {
			return nil, err
		}
		return lcms2.OpenProfileFromMem(checked)
	}
	if s.Type != ColorCalibratedRGB {
		return nil, malformed(0, "missing ICC profile")
	}
	var primaries [3]lcms2.CIExyY
	var white XYZ
	curves := make([]*lcms2.ToneCurve, 3)
	for i, p := range s.Endpoints {
		sum := p.X + p.Y + p.Z
		if !finite(p.X) || !finite(p.Y) || !finite(p.Z) || !finite(sum) || sum == 0 || !finite(s.Gamma[i]) || s.Gamma[i] <= 0 {
			return nil, malformed(0, "calibrated RGB endpoints/gamma")
		}
		primaries[i] = lcms2.XYZ2xyY(lcms2.CIEXYZ{X: p.X, Y: p.Y, Z: p.Z})
		if !finite(primaries[i].X) || !finite(primaries[i].Y) {
			return nil, malformed(0, "calibrated RGB chromaticity")
		}
		white.X += p.X
		white.Y += p.Y
		white.Z += p.Z
		var err error
		curves[i], err = lcms2.BuildGamma(s.Gamma[i])
		if err != nil {
			return nil, err
		}
	}
	if !finite(white.X+white.Y+white.Z) || white.X+white.Y+white.Z == 0 || white.Y <= 0 {
		return nil, malformed(0, "calibrated RGB white point")
	}
	wp := lcms2.XYZ2xyY(lcms2.CIEXYZ{X: white.X, Y: white.Y, Z: white.Z})
	wp.YY = 1
	return lcms2.CreateRGBProfile(&wp, &lcms2.CIExyYTRIPLE{Red: primaries[0], Green: primaries[1], Blue: primaries[2]}, curves)
}

// This is an ICC envelope/resource check, not a second ICC implementation.
// Tag semantics and transform construction are delegated to golittlecms.
func checkICCEnvelope(data []byte, o ColorTransformOptions) ([]byte, error) {
	if uint64(len(data)) > o.MaxProfileBytes {
		return nil, failure(0, "ICC bytes", ErrLimit)
	}
	if len(data) < 132 {
		return nil, malformed(0, "ICC header")
	}
	be := binary.BigEndian
	n := uint64(be.Uint32(data))
	count := uint64(be.Uint32(data[128:]))
	if n < 132 || n > uint64(len(data)) || be.Uint32(data[36:]) != 0x61637370 {
		return nil, malformed(0, "ICC size/signature")
	}
	if count > uint64(o.MaxProfileTags) {
		return nil, failure(128, "ICC tag count", ErrLimit)
	}
	end := uint64(132) + count*12
	if end > n {
		return nil, malformed(128, "ICC tag table")
	}
	var referenced uint64
	for i := uint64(0); i < count; i++ {
		entry := data[132+int(i)*12:]
		off, size := uint64(be.Uint32(entry[4:])), uint64(be.Uint32(entry[8:]))
		if off < end || off%4 != 0 || size < 8 || off > n || size > n-off {
			return nil, malformed(132+int(i)*12, "ICC tag span")
		}
		if size > o.MaxProfileBytes-referenced {
			return nil, failure(132+int(i)*12, "ICC referenced tag bytes", ErrLimit)
		}
		referenced += size
	}
	return data[:int(n):int(n)], nil
}
