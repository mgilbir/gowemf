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
	"time"

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

func TestFetchRetriesRateLimits(t *testing.T) {
	const good = "generated fixture"
	f := corpus.File{Path: "sample.wmf", SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(good))), Bytes: int64(len(good))}
	when := time.Now().Add(20 * time.Second).UTC().Format(http.TimeFormat)
	for _, tc := range []struct {
		name       string
		statuses   []int
		retryAfter string
		ok         bool
		calls      int
		minWait    time.Duration // total slept, lower bound
		maxWait    time.Duration // total slept, upper bound
	}{
		{"429 then success", []int{429, 200}, "2", true, 2, 2 * time.Second, 2 * time.Second},
		{"503 then success", []int{503, 200}, "0", true, 2, 0, 0},
		{"HTTP-date", []int{429, 200}, when, true, 2, 15 * time.Second, 20 * time.Second},
		{"backoff without Retry-After", []int{429, 429, 200}, "", true, 3, 3 * time.Second, 4500 * time.Millisecond},
		{"gives up", []int{429, 429, 429, 429, 200}, "1", false, maxAttempts, 3 * time.Second, 3 * time.Second},
		{"too long a wait", []int{429, 200}, "3600", false, 1, 0, 0},
		{"too distant a date", []int{429, 200}, time.Now().Add(time.Hour).UTC().Format(http.TimeFormat), false, 1, 0, 0},
		{"other statuses are not retried", []int{500, 200}, "1", false, 1, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var slept time.Duration
			sleep = func(d time.Duration) { slept += d }
			defer func() { sleep = time.Sleep }()
			root := t.TempDir()
			calls := 0
			client := &http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
				status := tc.statuses[calls]
				calls++
				h := make(http.Header)
				if status != 200 && tc.retryAfter != "" {
					h.Set("Retry-After", tc.retryAfter)
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(good)), ContentLength: -1, Header: h}, nil
			})}
			err := fetch(client, root, f)
			if (err == nil) != tc.ok || calls != tc.calls {
				t.Fatalf("got %v after %d calls; want success=%v after %d", err, calls, tc.ok, tc.calls)
			}
			if slept < tc.minWait || slept > tc.maxWait {
				t.Fatalf("slept %v; want %v to %v", slept, tc.minWait, tc.maxWait)
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			if want := map[bool]int{true: 1, false: 0}[tc.ok]; len(entries) != want {
				t.Fatal("unexpected files", entries)
			}
		})
	}
}
