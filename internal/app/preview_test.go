package app_test

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/testkit"
)

func pngBytes(w, h int) string {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 128
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.String()
}

// 06 §6.2: Preview.png (any case) scaled to 360 px wide, cached by snapshot hash.
func TestPreview(t *testing.T) {
	e := testkit.New(t)
	withP := testkit.Sample("tester.pic", "With Picture")
	withP.Files = map[string]string{"About/preview.PNG": pngBytes(1600, 900)}
	small := testkit.Sample("tester.small", "Small Picture")
	small.Files = map[string]string{"About/Preview.png": pngBytes(200, 300)}
	e.AddWorkshop("3001", withP)
	e.AddWorkshop("3002", small)
	e.AddWorkshop("3003", testkit.Sample("tester.none", "No Picture"))
	e.AddEnv(false)
	e.MustRun("scan")
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	get := func(ref string) (string, int, int, error) {
		m, err := ix.ResolveOne(ref)
		if err != nil {
			t.Fatal(err)
		}
		return e.App.Preview(ix, m)
	}
	path, w, h, err := get("ws:3001")
	if err != nil || w != app.PreviewWidth || h != 202 || filepath.Base(filepath.Dir(path)) != "previews" {
		t.Fatalf("preview: %s %dx%d %v", path, w, h, err)
	}
	if _, w, h, _ := get("ws:3002"); w != 200 || h != 300 {
		t.Fatalf("small: not scaled up: %dx%d", w, h)
	}
	if _, _, _, err := get("ws:3003"); !errors.Is(err, app.ErrNoPreview) {
		t.Fatalf("none: %v", err)
	}
	// from the cache: the mod's own file is not read again
	os.Remove(filepath.Join(e.Workshop, "3001", "About", "preview.PNG"))
	if p2, _, _, err := get("ws:3001"); err != nil || p2 != path {
		t.Fatalf("cached: %s %v", p2, err)
	}
}
