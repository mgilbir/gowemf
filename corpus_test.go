package gowemf

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/gowemf/internal/corpus"
)

func TestExternalCorpus(t *testing.T) {
	if os.Getenv("GOWEMF_EXTERNAL") != "1" {
		t.Skip("run make test-external to verify downloaded fixtures")
	}
	for _, f := range corpus.Files {
		t.Run(f.Path, func(t *testing.T) {
			file, err := os.Open(filepath.Join(".external", "poi", filepath.FromSlash(f.Path)))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			b, err := io.ReadAll(io.LimitReader(file, f.Bytes+1))
			if err != nil {
				t.Fatal(err)
			}
			if int64(len(b)) != f.Bytes || fmt.Sprintf("%x", sha256.Sum256(b)) != f.SHA256 {
				t.Fatal("fixture integrity failure")
			}
			outer, plus := 0, 0
			unsupported := make(map[string]int)
			h, err := Walk(b, Limits{}, func(r Record) error {
				if r.Format == EMFPlus {
					plus++
				} else {
					outer++
				}
				_, decodeErr := Decode(r, DecodeLimits{})
				if errors.Is(decodeErr, ErrUnsupported) {
					unsupported[fmt.Sprintf("%d:%04x", r.Format, r.Type)]++
					return nil
				}
				if decodeErr != nil {
					return fmt.Errorf("record type %x: %w", r.Type, decodeErr)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if outer != f.OuterRecords || plus != f.PlusRecords {
				t.Fatalf("got %d outer, %d plus; want %d, %d", outer, plus, f.OuterRecords, f.PlusRecords)
			}
			if (h.EMFPlus != nil) != (plus != 0) {
				t.Fatal("EMF+ metadata does not match records")
			}
			t.Logf("unsupported typed records: %v", unsupported)
			if len(unsupported) != 0 {
				t.Fatal("typed coverage regressed")
			}
			if _, err := Stream(b, StreamOptions{}, func(c Command) error {
				switch v := c.Body.(type) {
				case RasterTransfer:
					if len(v.Info) == 0 {
						return nil
					}
					d, err := ParseDIB(v.Info, v.Bits, v.Usage, nil, ImageLimits{})
					if err != nil {
						return err
					}
					if c.Source.Type == 114 && v.Operation>>24 == 1 {
						_, err = d.AlphaImage()
					} else {
						_, err = d.Image()
					}
					return err
				case BitmapTransfer:
					if len(v.Info) == 0 {
						return nil
					}
					d, err := ParseDIB(v.Info, v.Bits, v.Usage, nil, ImageLimits{})
					if err != nil {
						return err
					}
					_, err = d.Image()
					return err
				case PlusImage:
					if v.Type == 2 {
						data, err := v.MetafileBytes(Limits{})
						if err != nil {
							return err
						}
						_, err = Walk(data, Limits{}, nil)
						if err != nil {
							return err
						}
						_, err = Stream(data, StreamOptions{}, nil)
						return err
					}
					_, err := v.Image(ImageLimits{})
					return err
				}
				return nil
			}); err != nil {
				t.Fatal("native stream:", err)
			}
		})
	}
}
