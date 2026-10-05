// Command gowemfdump prints deterministic JSON record dumps for diagnostics.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/mgilbir/gowemf"
)

type outputLimit struct {
	w         io.Writer
	remaining int64
}

func (w *outputLimit) Write(b []byte) (int, error) {
	if int64(len(b)) > w.remaining {
		return 0, gowemf.ErrLimit
	}
	n, err := w.w.Write(b)
	w.remaining -= int64(n)
	return n, err
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	offset := flag.Int("offset", -1, "dump only the record at this byte offset (-1: all)")
	summary := flag.Bool("summary", false, "omit decoded fields")
	strict := flag.Bool("strict", false, "fail on unsupported typed records")
	flag.Parse()
	if flag.NArg() != 1 || *offset < -1 {
		return fmt.Errorf("usage: gowemfdump [-summary] [-strict] [-offset n] file")
	}
	f, err := os.Open(flag.Arg(0))
	if err != nil {
		return err
	}
	defer f.Close()
	const max = 64 << 20
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return err
	}
	if len(b) > max {
		return gowemf.ErrLimit
	}
	enc := json.NewEncoder(&outputLimit{os.Stdout, 128 << 20})
	enc.SetEscapeHTML(false)
	_, err = gowemf.Walk(b, gowemf.Limits{}, func(r gowemf.Record) error {
		body, err := gowemf.Decode(r, gowemf.DecodeLimits{})
		status := "decoded"
		if errors.Is(err, gowemf.ErrUnsupported) && !*strict {
			status = "unsupported"
			body = nil
		} else if err != nil {
			return err
		}
		if *offset >= 0 && *offset != r.Offset {
			return nil
		}
		typeName := fmt.Sprintf("%T", body)
		if body == nil {
			typeName = ""
		}
		if *summary {
			body = nil
		}
		return enc.Encode(struct {
			Format   string `json:"format"`
			Type     uint32 `json:"type"`
			Flags    uint16 `json:"flags"`
			Offset   int    `json:"offset"`
			Parent   int    `json:"parent"`
			Bytes    int    `json:"bytes"`
			Status   string `json:"status"`
			BodyType string `json:"bodyType,omitempty"`
			Body     any    `json:"body,omitempty"`
		}{r.Format.String(), r.Type, r.Flags, r.Offset, r.ParentOffset, len(r.Raw), status, typeName, body})
	})
	return err
}
