package main

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/selfupdate"
)

// RimArcho's own release client reads the served release as it reads GitHub's: the
// rehearsal exercises the player's path with only the API root changed.
func TestSelfUpdateClientReadsRelease(t *testing.T) {
	dir := t.TempDir()
	zip := "archo-1.1.3-windows-amd64.zip"
	os.WriteFile(filepath.Join(dir, zip), []byte("zip"), 0o644)
	os.WriteFile(filepath.Join(dir, zip+".sha256"), []byte("x  "+zip+"\n"), 0o644)
	h, err := newHandler(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	h.root = srv.URL
	c := &selfupdate.Client{API: srv.URL, GOOS: "windows", GOARCH: "amd64"}
	for name, get := range map[string]func() (*selfupdate.Release, error){
		"latest":  func() (*selfupdate.Release, error) { return c.Latest(context.Background(), false) },
		"version": func() (*selfupdate.Release, error) { return c.Version(context.Background(), "1.1.3") },
	} {
		r, err := get()
		if err != nil {
			t.Fatal(name, err)
		}
		if r.Version != "1.1.3" || r.Installable() != nil || r.Zip.Size != 3 || r.Zip.URL != srv.URL+"/download/"+zip {
			t.Fatalf("%s: %+v", name, r)
		}
	}
}

func TestOneZipOnly(t *testing.T) {
	dir := t.TempDir()
	if _, err := newHandler(dir, ""); err == nil {
		t.Fatal("an empty folder served")
	}
	for _, v := range []string{"1.1.3", "1.1.4"} {
		z := filepath.Join(dir, "archo-"+v+"-windows-amd64.zip")
		os.WriteFile(z, nil, 0o644)
		os.WriteFile(z+".sha256", nil, 0o644)
	}
	if _, err := newHandler(dir, ""); err == nil {
		t.Fatal("two zips served")
	}
}
