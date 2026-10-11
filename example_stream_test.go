package gowemf_test

import (
	"fmt"

	"github.com/mgilbir/gowemf"
)

func ExampleStream() {
	// sampleEMF is defined in the Play example.
	_, err := gowemf.Stream(sampleEMF(), gowemf.StreamOptions{}, func(c gowemf.Command) error {
		switch body := c.Body.(type) {
		case gowemf.RectRecord:
			fmt.Printf("record %d: rectangle %+v\n", c.Source.Type, body.Rect)
		default:
			fmt.Printf("record %d: %T\n", c.Source.Type, body)
		}
		return nil
	})
	if err != nil {
		fmt.Println(err)
	}
	// Output:
	// record 1: gowemf.Empty
	// record 43: rectangle {Left:10 Top:10 Right:60 Bottom:40}
	// record 14: gowemf.Empty
}
