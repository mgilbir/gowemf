// Command corpusfetch downloads only the pinned manifest into .external.
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mgilbir/gowemf/internal/corpus"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	oracle := flag.Bool("oracle", false, "download pinned execution-only POI oracle jars")
	codepages := flag.Bool("codepages", false, "download pinned Windows code page data files")
	unicode := flag.Bool("unicode", false, "download pinned Unicode Character Database files")
	flag.Parse()
	client := &http.Client{
		Timeout: 45 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || req.URL.Scheme != "https" || (req.URL.Host != "raw.githubusercontent.com" && req.URL.Host != "repo.maven.apache.org" && req.URL.Host != "www.unicode.org") {
				return fmt.Errorf("refusing download redirect")
			}
			return nil
		},
	}
	if *oracle {
		for _, a := range corpus.OracleArtifacts {
			f := corpus.File{Path: filepath.Base(a.Path), SHA256: a.SHA256, Bytes: a.Bytes}
			if err := fetchURL(client, filepath.Join(".external", "oracle"), f, corpus.MavenURL+a.Path); err != nil {
				return err
			}
			fmt.Println("verified", f.Path)
		}
		return nil
	}
	if *codepages {
		for _, f := range append(append([]corpus.File(nil), corpus.CodePages...), corpus.DBCSCodePages...) {
			if err := fetchURL(client, filepath.Join(".external", "codepages"), f, corpus.CodePageURL+f.Path); err != nil {
				return fmt.Errorf("%s: %w", f.Path, err)
			}
			fmt.Println("verified", f.Path)
		}
		return nil
	}
	if *unicode {
		for _, f := range corpus.UnicodeFiles {
			if err := fetchURL(client, filepath.Join(".external", "unicode"), f, corpus.UnicodeURL+f.Path); err != nil {
				return fmt.Errorf("%s: %w", f.Path, err)
			}
			fmt.Println("verified", f.Path)
		}
		return nil
	}
	for _, f := range corpus.Files {
		if err := fetch(client, filepath.Join(".external", "poi"), f); err != nil {
			return fmt.Errorf("%s: %w", f.Path, err)
		}
		fmt.Println("verified", f.Path)
	}
	return nil
}

func fetch(client *http.Client, root string, f corpus.File) error {
	return fetchURL(client, root, f, corpus.BaseURL+f.Path)
}

func fetchURL(client *http.Client, root string, f corpus.File, url string) error {
	// All paths come from the source-controlled manifest, never an archive.
	path := filepath.Join(root, filepath.FromSlash(f.Path))
	if old, err := os.Open(path); err == nil {
		h := sha256.New()
		n, readErr := io.Copy(h, io.LimitReader(old, f.Bytes+1))
		closeErr := old.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if n == f.Bytes && fmt.Sprintf("%x", h.Sum(nil)) == f.SHA256 {
			return nil
		}
		return fmt.Errorf("existing file fails integrity check; remove it explicitly before retrying")
	} else if !os.IsNotExist(err) {
		return err
	}
	resp, err := get(client, url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP status %d", resp.StatusCode)
	}
	if resp.ContentLength > f.Bytes {
		return fmt.Errorf("download exceeds pinned size")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, f.Bytes+1))
	closeErr := tmp.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n != f.Bytes || fmt.Sprintf("%x", h.Sum(nil)) != f.SHA256 {
		return fmt.Errorf("download fails pinned size or SHA-256 check")
	}
	return os.Rename(tmp.Name(), path)
}

// Rate-limited downloads are retried: CI starts many jobs at once against
// the same hosts. Only 429 and 503, which carry Retry-After, are retried, at
// most maxAttempts times in all; a server asking for more than maxRetryWait
// fails the download instead of stalling the job.
const (
	maxAttempts  = 4
	maxRetryWait = 30 * time.Second
)

// sleep is replaced in tests.
var sleep = time.Sleep

func get(client *http.Client, url string) (*http.Response, error) {
	for attempt := 1; ; attempt++ {
		resp, err := client.Get(url)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != http.StatusServiceUnavailable {
			return resp, nil
		}
		wait, waitErr := retryDelay(resp.Header.Get("Retry-After"), attempt, time.Now())
		// Drain a bounded amount so the connection can be reused.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		if attempt == maxAttempts {
			return nil, fmt.Errorf("HTTP status %d after %d attempts", resp.StatusCode, attempt)
		}
		if waitErr != nil {
			return nil, fmt.Errorf("HTTP status %d: %w", resp.StatusCode, waitErr)
		}
		fmt.Fprintf(os.Stderr, "HTTP status %d; retrying in %v\n", resp.StatusCode, wait.Round(time.Millisecond))
		sleep(wait)
	}
}

// retryDelay honors Retry-After as delay-seconds or an HTTP-date (RFC 9110
// section 10.2.3). Without a usable value it backs off exponentially from one
// second, with jitter so simultaneous jobs spread out.
func retryDelay(value string, attempt int, now time.Time) (time.Duration, error) {
	var d time.Duration
	if secs, err := strconv.ParseInt(value, 10, 64); err == nil && secs >= 0 {
		if secs > int64(maxRetryWait/time.Second) {
			return 0, fmt.Errorf("server asked to retry after %d s, more than %v", secs, maxRetryWait)
		}
		d = time.Duration(secs) * time.Second
	} else if t, err := http.ParseTime(value); err == nil {
		d = max(t.Sub(now), 0)
		if d > maxRetryWait {
			return 0, fmt.Errorf("server asked to retry at %v, more than %v away", t, maxRetryWait)
		}
	} else {
		base := time.Second << (attempt - 1)
		d = base + rand.N(base/2)
	}
	return d, nil
}
