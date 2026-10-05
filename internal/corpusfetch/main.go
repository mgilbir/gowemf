// Command corpusfetch downloads only the pinned manifest into .external.
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	flag.Parse()
	client := &http.Client{
		Timeout: 45 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || req.URL.Scheme != "https" || (req.URL.Host != "raw.githubusercontent.com" && req.URL.Host != "repo.maven.apache.org") {
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
	resp, err := client.Get(url)
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
