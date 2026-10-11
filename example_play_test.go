package gowemf_test

import (
	"encoding/binary"
	"fmt"

	"github.com/mgilbir/gowemf"
)

// sampleEMF builds a 100 x 100 pixel EMF that draws one rectangle with the
// default white brush and black pen. Real input comes from a file or an
// embedded document image.
func sampleEMF() []byte {
	record := func(typ uint32, body ...uint32) []byte {
		b := binary.LittleEndian.AppendUint32(nil, typ)
		b = binary.LittleEndian.AppendUint32(b, uint32(8+4*len(body)))
		for _, v := range body {
			b = binary.LittleEndian.AppendUint32(b, v)
		}
		return b
	}
	b := record(gowemf.EMRHeader,
		0, 0, 99, 99, // bounds, device pixels
		0, 0, 2500, 2500, // frame, 0.01 mm
		0x464d4520, 0x10000, // signature " EMF", version
		0, 3, // file size (set below), record count
		1,       // handles
		0, 0, 0, // description and palette
		1000, 1000, 250, 250, // reference device: pixels, then millimeters
	)
	b = append(b, record(gowemf.EMRRectangle, 10, 10, 60, 40)...)
	b = append(b, record(gowemf.EMREOF, 0, 0, 20)...)
	binary.LittleEndian.PutUint32(b[48:], uint32(len(b)))
	return b
}

// canvas is a Backend that prints what it is asked to draw. A real backend
// rasterizes the path with the paint or stroke, inside every clip region.
type canvas struct{}

func (canvas) FillPath(p gowemf.Path, rule gowemf.FillRule, paint gowemf.Paint, clip gowemf.Clip) error {
	fmt.Printf("fill %d points from %v, color %v\n", len(p.Points), p.Points[0], paint.Color)
	return nil
}

func (canvas) StrokePath(p gowemf.Path, s gowemf.Stroke, clip gowemf.Clip) error {
	fmt.Printf("stroke %d points from %v, hairline %v, color %v\n", len(p.Points), p.Points[0], s.Hairline, s.Paint.Color)
	return nil
}

func (canvas) DrawImage(img gowemf.ImageDraw, clip gowemf.Clip) error {
	fmt.Println("image", img.Source)
	return nil
}

func ExamplePlay() {
	var reported []gowemf.UnsupportedOperation
	_, err := gowemf.Play(sampleEMF(), gowemf.PlayOptions{
		// The 100 x 100 pixel frame is drawn at twice its size.
		Destination: gowemf.Box{Width: 200, Height: 200},
		Unsupported: func(u gowemf.UnsupportedOperation) error {
			reported = append(reported, u) // or return an error to stop
			return nil
		},
	}, canvas{})
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("reported:", len(reported))
	// Output:
	// fill 4 points from {20 20}, color {255 255 255 255}
	// stroke 4 points from {20 20}, hairline true, color {0 0 0 255}
	// reported: 0
}
