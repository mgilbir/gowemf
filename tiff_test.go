package gowemf

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"sort"
	"testing"
)

type tiffTestConfig struct {
	width, height, photo, samples, depth, compression, rows, planar, predictor, orientation, fill uint32
	extras, palette                                                                               []uint16
	profile                                                                                       []byte
	big                                                                                           bool
	strips                                                                                        [][]byte
}

func tiffFixture(c tiffTestConfig) []byte {
	var order binary.ByteOrder = binary.LittleEndian
	signature := "II"
	if c.big {
		order = binary.BigEndian
		signature = "MM"
	}
	if c.width == 0 {
		c.width = 2
	}
	if c.height == 0 {
		c.height = 1
	}
	if c.samples == 0 {
		c.samples = 3
	}
	if c.depth == 0 {
		c.depth = 8
	}
	if c.compression == 0 {
		c.compression = 1
	}
	if c.rows == 0 {
		c.rows = c.height
	}
	if c.planar == 0 {
		c.planar = 1
	}
	if c.predictor == 0 {
		c.predictor = 1
	}
	if c.orientation == 0 {
		c.orientation = 1
	}
	if c.fill == 0 {
		c.fill = 1
	}
	type field struct {
		kind  uint16
		count uint32
		data  []byte
	}
	fields := map[uint16]field{}
	long := func(tag uint16, values ...uint32) {
		b := make([]byte, 4*len(values))
		for i, v := range values {
			order.PutUint32(b[i*4:], v)
		}
		fields[tag] = field{4, uint32(len(values)), b}
	}
	short := func(tag uint16, values ...uint16) {
		b := make([]byte, 2*len(values))
		for i, v := range values {
			order.PutUint16(b[i*2:], v)
		}
		fields[tag] = field{3, uint32(len(values)), b}
	}
	long(256, c.width)
	long(257, c.height)
	short(259, uint16(c.compression))
	short(262, uint16(c.photo))
	short(266, uint16(c.fill))
	short(274, uint16(c.orientation))
	short(277, uint16(c.samples))
	long(278, c.rows)
	short(284, uint16(c.planar))
	short(317, uint16(c.predictor))
	bps := make([]uint16, int(c.samples))
	for i := range bps {
		bps[i] = uint16(c.depth)
	}
	short(258, bps...)
	if c.extras != nil {
		short(338, c.extras...)
	}
	if c.palette != nil {
		short(320, c.palette...)
	}
	if c.profile != nil {
		fields[34675] = field{7, uint32(len(c.profile)), c.profile}
	}
	offsets, counts := make([]uint32, len(c.strips)), make([]uint32, len(c.strips))
	for i := range counts {
		counts[i] = uint32(len(c.strips[i]))
	}
	long(273, offsets...)
	long(279, counts...)
	tags := make([]int, 0, len(fields))
	for tag := range fields {
		tags = append(tags, int(tag))
	}
	sort.Ints(tags)
	b := make([]byte, 8+2+12*len(tags)+4)
	copy(b, signature)
	order.PutUint16(b[2:], 42)
	order.PutUint32(b[4:], 8)
	order.PutUint16(b[8:], uint16(len(tags)))
	offsetPos := 0
	for i, tag := range tags {
		f := fields[uint16(tag)]
		p := 10 + i*12
		order.PutUint16(b[p:], uint16(tag))
		order.PutUint16(b[p+2:], f.kind)
		order.PutUint32(b[p+4:], f.count)
		valuePos := p + 8
		if len(f.data) > 4 {
			if len(b)%2 != 0 {
				b = append(b, 0)
			}
			valuePos = len(b)
			order.PutUint32(b[p+8:], uint32(valuePos))
			b = append(b, f.data...)
		} else {
			copy(b[valuePos:valuePos+4], f.data)
		}
		if tag == 273 {
			offsetPos = valuePos
		}
	}
	for i, strip := range c.strips {
		order.PutUint32(b[offsetPos+i*4:], uint32(len(b)))
		b = append(b, strip...)
	}
	return b
}
func TestTIFFRGBStripsAndByteOrder(t *testing.T) {
	for _, big := range []bool{false, true} {
		data := tiffFixture(tiffTestConfig{photo: 2, height: 2, rows: 1, big: big, strips: [][]byte{{255, 0, 0, 0, 255, 0}, {0, 0, 255, 255, 255, 255}}})
		d, err := ParseTIFF(data, ImageLimits{})
		if err != nil {
			t.Fatal(err)
		}
		im, err := d.Image()
		if err != nil {
			t.Fatal(err)
		}
		want := []color.NRGBA{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}, {255, 255, 255, 255}}
		for i, c := range want {
			if got := im.(*image.NRGBA).NRGBAAt(i%2, i/2); got != c {
				t.Fatal(big, i, got, c)
			}
		}
		if _, err := (PlusImage{Type: 1, BitmapType: 1, Data: data}).Image(ImageLimits{}); err != nil {
			t.Fatal(err)
		}
	}
}
func TestTIFFSamplesPaletteAndAlpha(t *testing.T) {
	for _, c := range []tiffTestConfig{
		{photo: 2, planar: 2, strips: [][]byte{{255, 0}, {0, 255}, {0, 0}}},
		{photo: 2, samples: 4, extras: []uint16{2}, strips: [][]byte{{255, 0, 0, 64, 0, 255, 0, 128}}},
		{photo: 2, samples: 4, extras: []uint16{1}, strips: [][]byte{{64, 0, 0, 64, 0, 128, 0, 128}}},
	} {
		d, err := ParseTIFF(tiffFixture(c), ImageLimits{})
		if err != nil {
			t.Fatal(err)
		}
		im, err := d.Image()
		if err != nil {
			t.Fatal(err)
		}
		a, b := im.(*image.NRGBA).NRGBAAt(0, 0), im.(*image.NRGBA).NRGBAAt(1, 0)
		if a.R != 255 || a.G != 0 || b.G != 255 || b.R != 0 {
			t.Fatal(a, b)
		}
		if c.samples == 4 && (a.A != 64 || b.A != 128) {
			t.Fatal("alpha", a, b)
		}
	}
	for _, fill := range []uint32{1, 2} {
		pixel := byte(0x40)
		if fill == 2 {
			pixel = 2
		}
		d, err := ParseTIFF(tiffFixture(tiffTestConfig{samples: 1, depth: 1, photo: 0, fill: fill, strips: [][]byte{{pixel}}}), ImageLimits{})
		if err != nil {
			t.Fatal(err)
		}
		im, err := d.Image()
		if err != nil {
			t.Fatal(err)
		}
		if im.(*image.NRGBA).NRGBAAt(0, 0).R != 255 || im.(*image.NRGBA).NRGBAAt(1, 0).R != 0 {
			t.Fatal("white-is-zero/fill order")
		}
	}
	palette := make([]uint16, 48)
	palette[0] = 0x1234
	palette[17] = 0xabcd
	d, err := ParseTIFF(tiffFixture(tiffTestConfig{samples: 1, depth: 4, photo: 3, palette: palette, strips: [][]byte{{0x01}}}), ImageLimits{})
	if err != nil {
		t.Fatal(err)
	}
	im, err := d.Image()
	if err != nil {
		t.Fatal(err)
	}
	if im.(*image.NRGBA64).NRGBA64At(0, 0).R != 0x1234 || im.(*image.NRGBA64).NRGBA64At(1, 0).G != 0xabcd {
		t.Fatal("palette ordering/precision")
	}
}
func TestTIFFPredictionAndOrientation(t *testing.T) {
	for _, big := range []bool{false, true} {
		var order binary.ByteOrder = binary.LittleEndian
		if big {
			order = binary.BigEndian
		}
		pixels := make([]byte, 12)
		for i, v := range []uint16{1000, 2000, 3000, 500, 65036, 1000} {
			order.PutUint16(pixels[i*2:], v)
		}
		d, err := ParseTIFF(tiffFixture(tiffTestConfig{photo: 2, depth: 16, predictor: 2, big: big, strips: [][]byte{pixels}}), ImageLimits{})
		if err != nil {
			t.Fatal(err)
		}
		im, err := d.Image()
		if err != nil {
			t.Fatal(err)
		}
		if got := im.(*image.NRGBA64).NRGBA64At(1, 0); got != (color.NRGBA64{1500, 1500, 4000, 65535}) {
			t.Fatal(got)
		}
	}
	topLeft := []color.NRGBA{{255, 0, 0, 255}, {0, 255, 0, 255}, {255, 255, 255, 255}, {0, 0, 255, 255}, {255, 0, 0, 255}, {0, 0, 255, 255}, {255, 255, 255, 255}, {0, 255, 0, 255}}
	for o := uint32(1); o <= 8; o++ {
		d, err := ParseTIFF(tiffFixture(tiffTestConfig{photo: 2, height: 2, orientation: o, strips: [][]byte{{255, 0, 0, 0, 255, 0, 0, 0, 255, 255, 255, 255}}}), ImageLimits{})
		if err != nil {
			t.Fatal(err)
		}
		im, err := d.Image()
		if err != nil {
			t.Fatal(err)
		}
		if im.(*image.NRGBA).NRGBAAt(0, 0) != topLeft[o-1] {
			t.Fatal(o, im.At(0, 0))
		}
	}
}
func packTIFFCodes(codes []int, widths []uint) []byte {
	var b []byte
	var accumulator uint64
	var used uint
	for i, code := range codes {
		accumulator = accumulator<<widths[i] | uint64(code)
		used += widths[i]
		for used >= 8 {
			used -= 8
			b = append(b, byte(accumulator>>used))
			accumulator &= (1 << used) - 1
		}
	}
	if used != 0 {
		b = append(b, byte(accumulator<<(8-used)))
	}
	return b
}
func TestTIFFCompression(t *testing.T) {
	pixels := []byte{10, 20, 30, 40, 50, 60}
	var deflated bytes.Buffer
	w := zlib.NewWriter(&deflated)
	if _, err := w.Write(pixels); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	lzw := packTIFFCodes([]int{256, 10, 20, 30, 40, 50, 60, 257}, []uint{9, 9, 9, 9, 9, 9, 9, 9})
	for compression, strip := range map[uint32][]byte{1: pixels, 5: lzw, 8: deflated.Bytes(), 32946: deflated.Bytes(), 32773: append([]byte{5}, pixels...)} {
		d, err := ParseTIFF(tiffFixture(tiffTestConfig{photo: 2, compression: compression, strips: [][]byte{strip}}), ImageLimits{})
		if err != nil {
			t.Fatal(err)
		}
		im, err := d.Image()
		if err != nil {
			t.Fatal(compression, err)
		}
		if im.(*image.NRGBA).NRGBAAt(1, 0) != (color.NRGBA{40, 50, 60, 255}) {
			t.Fatal(compression)
		}
	}
	var codes []int
	var widths []uint
	codes = append(codes, 256)
	widths = append(widths, 9)
	expected := make([]byte, 2200)
	for i := range expected {
		expected[i] = byte(i)
		codes = append(codes, int(expected[i]))
		width := uint(9)
		if i >= 254 {
			width = 10
		}
		if i >= 766 {
			width = 11
		}
		if i >= 1790 {
			width = 12
		}
		widths = append(widths, width)
	}
	codes = append(codes, 257)
	widths = append(widths, 12)
	out := make([]byte, len(expected))
	if err := decodeTIFFLZW(out, packTIFFCodes(codes, widths)); err != nil || !bytes.Equal(out, expected) {
		t.Fatal("early-change LZW widths", err)
	}
	out = make([]byte, 6)
	if err := decodeTIFFLZW(out, packTIFFCodes([]int{256, 65, 258, 259, 257}, []uint{9, 9, 9, 9, 9})); err != nil || string(out) != "AAAAAA" {
		t.Fatal("LZW special code", out, err)
	}
}
func TestTIFFLimitsAndCorruption(t *testing.T) {
	data := tiffFixture(tiffTestConfig{photo: 2, strips: [][]byte{{1, 2, 3, 4, 5, 6}}})
	for n := 0; n < len(data); n++ {
		if d, err := ParseTIFF(data[:n], ImageLimits{}); err == nil {
			if _, err := d.Image(); err == nil {
				t.Fatalf("accepted prefix %d", n)
			}
		}
	}
	for _, l := range []ImageLimits{{MaxPixels: 1}, {MaxBytes: 1}, {MaxDecodedBytes: 5}} {
		if _, err := ParseTIFF(data, l); !errors.Is(err, ErrLimit) {
			t.Fatal(l, err)
		}
	}
	// Width*height fits the configured pixel allowance on 64-bit hosts, but
	// 16 channels of 16-bit data would overflow a naive uint64 byte product.
	huge := tiffFixture(tiffTestConfig{width: 0xffffffff, height: 200000000, photo: 2, samples: 16, depth: 16, strips: [][]byte{{0}}})
	if _, err := ParseTIFF(huge, ImageLimits{MaxPixels: ^uint64(0)}); !errors.Is(err, ErrLimit) {
		t.Fatal("TIFF decoded-size arithmetic", err)
	}
	bad := append([]byte(nil), data...)
	put32(bad, 4, 0xfffffffe)
	if _, err := ParseTIFF(bad, ImageLimits{}); !errors.Is(err, ErrMalformed) {
		t.Fatal(err)
	}
	if err := decodeTIFFStrip(make([]byte, 2), []byte{254, 1}, 32773, 2); !errors.Is(err, ErrMalformed) {
		t.Fatal("PackBits row overflow", err)
	}
	if err := decodeTIFFLZW(make([]byte, 1), packTIFFCodes([]int{256, 300, 257}, []uint{9, 9, 9})); !errors.Is(err, ErrMalformed) {
		t.Fatal("undefined LZW code", err)
	}
	profile := generatedICC(t)
	d, err := ParseTIFF(tiffFixture(tiffTestConfig{photo: 2, profile: profile, strips: [][]byte{{1, 2, 3, 4, 5, 6}}}), ImageLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Image(); !errors.Is(err, ErrUnsupported) {
		t.Fatal("ICC ignored", err)
	}
	if _, err := d.ImageWithColorTransform(NewColorTransform); err != nil {
		t.Fatal(err)
	}
}

func FuzzTIFF(f *testing.F) {
	f.Add(tiffFixture(tiffTestConfig{photo: 2, strips: [][]byte{{1, 2, 3, 4, 5, 6}}}))
	f.Add(tiffFixture(tiffTestConfig{photo: 2, compression: 32773, strips: [][]byte{{5, 1, 2, 3, 4, 5, 6}}}))
	f.Add(tiffFixture(tiffTestConfig{photo: 2, compression: 5, strips: [][]byte{packTIFFCodes([]int{256, 1, 2, 3, 4, 5, 6, 257}, []uint{9, 9, 9, 9, 9, 9, 9, 9})}}))
	f.Add(tiffFixture(tiffTestConfig{photo: 2, depth: 16, big: true, predictor: 2, strips: [][]byte{make([]byte, 12)}}))
	f.Fuzz(func(t *testing.T, data []byte) {
		d, err := ParseTIFF(data, ImageLimits{MaxBytes: 1 << 20, MaxPixels: 4096, MaxDecodedBytes: 64 << 10})
		if err == nil {
			_, _ = d.Image()
		}
	})
}
