package icon

import (
	"bytes"
	"flag"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite docs/assets/rimarcho.svg")

func TestRender(t *testing.T) {
	for _, s := range Sizes {
		img, err := png.Decode(bytes.NewReader(PNG(s)))
		if err != nil || img.Bounds().Dx() != s {
			t.Fatalf("size %d: %v", s, err)
		}
		// Corners are transparent, the keystone is drawn.
		if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
			t.Fatalf("size %d: corner not transparent", s)
		}
	}
	if !strings.Contains(SVG(256), "#E8A93A") {
		t.Fatal("svg has no keystone")
	}
	if dir := os.Getenv("ICON_PREVIEW"); dir != "" {
		for _, s := range Sizes {
			os.WriteFile(filepath.Join(dir, "icon"+strconv.Itoa(s)+".png"), PNG(s), 0o644)
		}
	}
}

// docs/assets/rimarcho.svg is generated from the geometry (08 §15.2):
// go test ./internal/icon -update
func TestSVGFile(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "assets", "rimarcho.svg")
	if *update {
		if err := os.WriteFile(path, []byte(SVG(256)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != SVG(256) {
		t.Fatalf("%s is out of date (go test ./internal/icon -update): %v", path, err)
	}
}
