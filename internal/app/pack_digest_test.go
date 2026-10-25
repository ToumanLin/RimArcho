package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

func TestCopyDigestRejectsLinks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "content"), []byte("mod"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, sum, err := copyDigest(dir)
	if err != nil || before != 1 {
		t.Fatalf("initial digest: %d %s %v", before, sum, err)
	}
	metadata := filepath.Join(dir, ".rimarcho")
	if err := os.Mkdir(metadata, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadata, "origin.json"), []byte("metadata"), 0o644); err != nil {
		t.Fatal(err)
	}
	if n, got, err := copyDigest(dir); err != nil || n != before || got != sum {
		t.Fatalf("metadata changed digest: %d %s %v", n, got, err)
	}
	for _, link := range []string{filepath.Join(dir, "linked"), filepath.Join(metadata, "linked")} {
		if err := source.CreateLink(link, t.TempDir()); err != nil {
			t.Fatal(err)
		}
		if _, _, err := copyDigest(dir); err == nil || !strings.Contains(err.Error(), "links") {
			t.Fatalf("link accepted: %v", err)
		}
		if err := source.RemoveLink(link); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPackEntryProvenance(t *testing.T) {
	t.Parallel()
	m := &store.Mod{PackageID: "test.provenance", ACFTimeUpdated: 123456, GitHead: "abcdef"}
	e := packEntryOf(m)
	if e.WorkshopUpdated != m.ACFTimeUpdated || e.GitHead != m.GitHead {
		t.Fatalf("missing provenance: %+v", e)
	}
}
