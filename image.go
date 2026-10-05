package gowemf

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
)

// AlphaImage interprets a 32-bit RGB or standard BGR-bitfield DIB as premultiplied BGRA, as required
// by EMR_ALPHABLEND with AC_SRC_ALPHA. Image instead treats BI_RGB's fourth byte
// as unused/opaque. SourceConstantAlpha and destination compositing are left to
// the renderer. Channels greater than alpha are rejected as non-premultiplied.
func (d *DIB) AlphaImage() (*image.RGBA, error) {
	if d == nil || d.width <= 0 || d.height <= 0 {
		return nil, malformed(0, "uninitialized DIB")
	}
	if d != nil && !d.colorSpace.IsSRGB() {
		return nil, failure(0, "alpha DIB color conversion", ErrUnsupported)
	}
	return d.rawAlphaImage()
}

func (d *DIB) rawAlphaImage() (*image.RGBA, error) {
	if d == nil || d.width <= 0 || d.height <= 0 {
		return nil, malformed(0, "uninitialized DIB")
	}
	standardFields := d.compression == 3 && d.masks[0] == 0xff0000 && d.masks[1] == 0xff00 && d.masks[2] == 0xff && (d.masks[3] == 0 || d.masks[3] == 0xff000000)
	if d.bpp != 32 || (d.compression != 0 && !standardFields) {
		return nil, failure(0, "alpha DIB layout", ErrUnsupported)
	}
	out := image.NewRGBA(image.Rect(0, 0, d.width, d.height))
	for y := 0; y < d.height; y++ {
		sy := y
		if !d.topDown {
			sy = d.height - 1 - y
		}
		row := d.pixels[sy*d.stride : (sy+1)*d.stride]
		for x := 0; x < d.width; x++ {
			p := row[x*4:]
			if p[0] > p[3] || p[1] > p[3] || p[2] > p[3] {
				return nil, malformed(sy*d.stride+x*4, "non-premultiplied alpha DIB")
			}
			i := y*out.Stride + x*4
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = p[2], p[1], p[0], p[3]
		}
	}
	return out, nil
}

// Image decodes EMF+ PNG/JPEG or raw 24-bit RGB / 32-bit RGB, ARGB and PARGB
// bitmaps. Metafiles and other pixel formats return ErrUnsupported. All public
// fields are revalidated before allocation, including caller-constructed values.
func (p PlusImage) Image(limits ImageLimits) (image.Image, error) {
	l := limits.defaults()
	if uint64(len(p.Data)) > l.MaxBytes {
		return nil, failure(0, "EMF+ image bytes", ErrLimit)
	}
	if p.Type != 1 {
		return nil, failure(0, "EMF+ non-bitmap image", ErrUnsupported)
	}
	if p.BitmapType == 1 {
		return decodeEncodedImage(p.Data, l)
	}
	if p.BitmapType != 0 {
		return nil, malformed(0, "EMF+ bitmap data type")
	}
	// MS-EMFPLUS 2.1.1.24: PixelFormat24bppRGB, 32bppRGB, 32bppARGB,
	// and 32bppPARGB. Other formats need separate palette/channel handling.
	bpp := uint64(4)
	alpha, premult := false, false
	switch p.PixelFormat {
	case 0x21808:
		bpp = 3
	case 0x22009:
	case 0x26200a:
		alpha = true
	case 0xe200b:
		alpha = true
		premult = true
	default:
		return nil, failure(0, "EMF+ pixel format", ErrUnsupported)
	}
	if p.Width <= 0 || p.Height <= 0 || p.Stride == 0 || p.Stride%4 != 0 {
		return nil, malformed(0, "EMF+ image dimensions")
	}
	w, h := uint64(p.Width), uint64(p.Height)
	if w*h > l.MaxPixels || w*h > uint64(int(^uint(0)>>1))/4 {
		return nil, failure(0, "EMF+ image pixels", ErrLimit)
	}
	stride := int64(p.Stride)
	if stride < 0 {
		stride = -stride
	}
	if uint64(stride) < w*bpp || uint64(stride)*h > uint64(len(p.Data)) {
		return nil, malformed(0, "EMF+ image buffer")
	}
	var out image.Image
	var dst []byte
	if premult {
		im := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
		out, dst = im, im.Pix
	} else {
		im := image.NewNRGBA(image.Rect(0, 0, int(w), int(h)))
		out, dst = im, im.Pix
	}
	for y := 0; y < int(h); y++ {
		sy := y
		if p.Stride < 0 {
			sy = int(h) - 1 - y
		}
		row := p.Data[int64(sy)*stride : int64(sy+1)*stride]
		for x := 0; x < int(w); x++ {
			src := row[x*int(bpp):]
			a := byte(255)
			if alpha {
				a = src[3]
			}
			if premult && (src[0] > a || src[1] > a || src[2] > a) {
				return nil, malformed(0, "non-premultiplied EMF+ bitmap")
			}
			i := (y*int(w) + x) * 4
			dst[i], dst[i+1], dst[i+2], dst[i+3] = src[2], src[1], src[0], a
		}
	}
	return out, nil
}

func decodeEncodedImage(data []byte, l ImageLimits) (image.Image, error) {
	var cfg image.Config
	var err error
	isPNG := len(data) >= 8 && bytes.Equal(data[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10})
	isJPEG := len(data) >= 2 && data[0] == 255 && data[1] == 216
	if isPNG {
		cfg, err = png.DecodeConfig(bytes.NewReader(data))
	} else if isJPEG {
		cfg, err = jpeg.DecodeConfig(bytes.NewReader(data))
	} else {
		return nil, failure(0, "compressed EMF+ image encoding", ErrUnsupported)
	}
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || uint64(cfg.Width) > l.MaxPixels/uint64(cfg.Height) || uint64(cfg.Width) > uint64(int(^uint(0)>>1))/8/uint64(cfg.Height) {
		return nil, failure(0, "compressed EMF+ image pixels", ErrLimit)
	}
	if isPNG {
		return png.Decode(bytes.NewReader(data))
	}
	return jpeg.Decode(bytes.NewReader(data))
}
