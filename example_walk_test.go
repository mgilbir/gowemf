package gowemf_test

import (
	"fmt"

	"github.com/mgilbir/gowemf"
)

func ExampleWalk() {
	// sampleEMF is defined in the Play example.
	header, err := gowemf.Walk(sampleEMF(), gowemf.Limits{}, func(r gowemf.Record) error {
		fmt.Printf("%v type=%d offset=%d bytes=%d\n", r.Format, r.Type, r.Offset, len(r.Raw))
		return nil
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	w, h, _ := header.SizeInPoints()
	fmt.Printf("%.2f x %.2f points\n", w, h)
	// Output:
	// EMF type=1 offset=0 bytes=88
	// EMF type=43 offset=88 bytes=24
	// EMF type=14 offset=112 bytes=20
	// 70.87 x 70.87 points
}
