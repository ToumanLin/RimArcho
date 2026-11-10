package main

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestPackLinuxRelease(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"archo", "README.md", "LICENSE"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(t.TempDir(), "release.tar.gz")
	if err := pack(dir, out); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	found := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		want := int64(0o644)
		if h.Name == "archo" {
			want = 0o755
		}
		b, err := io.ReadAll(tr)
		if err != nil || h.Mode != want || string(b) != h.Name {
			t.Fatal(h, string(b), err)
		}
		found++
	}
	if found != 3 {
		t.Fatal(found)
	}
}
