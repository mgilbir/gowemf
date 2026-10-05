package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgilbir/gowemf/internal/corpus"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFetchIntegrity(t *testing.T) {
	const good = "generated fixture"
	f := corpus.File{Path: "sample.wmf", SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(good))), Bytes: int64(len(good))}
	for _, tc := range []struct {
		name, body string
		status     int
		ok         bool
	}{
		{"valid", good, 200, true},
		{"wrong digest", "GENERATED FIXTURE", 200, false},
		{"truncated", good[:5], 200, false},
		{"oversized", good + "x", 200, false},
		{"HTTP failure", good, 404, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			calls := 0
			client := &http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.URL.String() != corpus.BaseURL+f.Path {
					t.Fatal(req.URL)
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), ContentLength: -1, Header: make(http.Header)}, nil
			})}
			err := fetch(client, root, f)
			if (err == nil) != tc.ok {
				t.Fatalf("got %v; want success=%v", err, tc.ok)
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.ok {
				if len(entries) != 0 {
					t.Fatal("failed download left files behind", entries)
				}
				return
			}
			if len(entries) != 1 || entries[0].Name() != f.Path {
				t.Fatal(entries)
			}
			if err := fetch(client, root, f); err != nil || calls != 1 {
				t.Fatal("cache not verified", calls, err)
			}
			if err := os.WriteFile(filepath.Join(root, f.Path), []byte("corrupt"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := fetch(client, root, f); err == nil || calls != 1 {
				t.Fatal("corrupt cache reused or overwritten", calls, err)
			}
		})
	}
}
