package source

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveTreeDoesNotFollowLinks(t *testing.T) {
	root, target := filepath.Join(t.TempDir(), "delete"), t.TempDir()
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(target, "keep.txt")
	if err := os.WriteFile(keep, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := CreateLink(filepath.Join(root, "nested-link"), target); err != nil {
		t.Fatal(err)
	}
	if err := RemoveTree(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("deleted root: %v", err)
	}
	if b, err := os.ReadFile(keep); err != nil || string(b) != "keep" {
		t.Fatalf("link target touched: %q %v", b, err)
	}
	link := filepath.Join(t.TempDir(), "root-link")
	if err := CreateLink(link, target); err != nil {
		t.Fatal(err)
	}
	if err := RemoveTree(link); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal(err)
	}
}
