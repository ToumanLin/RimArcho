package icon

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

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
	if !strings.Contains(SVG(), "#E8A93A") {
		t.Fatal("svg has no keystone")
	}
	if dir := os.Getenv("ICON_PREVIEW"); dir != "" {
		for _, s := range Sizes {
			os.WriteFile(filepath.Join(dir, "icon"+strconv.Itoa(s)+".png"), PNG(s), 0o644)
		}
	}
}
