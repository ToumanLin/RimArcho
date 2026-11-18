package gui

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// /preview/<id> serves the thumbnail, and making it (a full decode) does not hold the
// page lock (16 §8.4). Not parallel: it replaces previewOf.
func TestPreviewHandlerDoesNotHoldSharedLock(t *testing.T) {
	e, api, _ := fixture(t)
	img := image.NewNRGBA(image.Rect(0, 0, 1200, 600))
	for i := range img.Pix {
		img.Pix[i] = uint8(i)
	}
	var pic bytes.Buffer
	png.Encode(&pic, img)
	if err := os.WriteFile(filepath.Join(e.Workshop, "1001", "About", "Preview.png"), pic.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	m, err := ix.ResolveOne("ws:1001")
	if err != nil {
		t.Fatal(err)
	}
	api.Board()

	started, release := make(chan struct{}), make(chan struct{})
	var once, freed sync.Once
	free := func() { freed.Do(func() { close(release) }) }
	real := previewOf
	// Cleanups run last first: free the held request before the fixture closes the API.
	t.Cleanup(func() { free(); previewOf = real })
	previewOf = func(a *app.App, pt app.PreviewTarget) (string, int, int, error) {
		once.Do(func() { close(started) })
		<-release
		return real(a, pt)
	}
	h := handler(api, fstest.MapFS{})
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/preview/"+m.ID, nil))
		done <- rec
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the thumbnail was never made")
	}
	stampResponds(t, api)
	free()
	rec := <-done
	if rec.Code != 200 {
		t.Fatalf("preview: %d", rec.Code)
	}
	got, err := png.DecodeConfig(rec.Body)
	if err != nil || got.Width != app.PreviewWidth || got.Height != 180 {
		t.Fatalf("thumbnail: %+v %v", got, err)
	}
}

func TestPreviewThumbHandler(t *testing.T) {
	e, api, _ := fixture(t)
	var pic bytes.Buffer
	if err := png.Encode(&pic, image.NewNRGBA(image.Rect(0, 0, 1200, 600))); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(e.Workshop, "1001", "About", "Preview.png")
	if err := os.WriteFile(src, pic.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	m, err := ix.ResolveOne("ws:1001")
	if err != nil {
		t.Fatal(err)
	}
	api.Board()
	h := handler(api, fstest.MapFS{})
	get := func(id string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/preview/"+id+"?w=thumb", nil))
		return rec
	}
	first := get(m.ID)
	if first.Code != 200 || first.Header().Get("Cache-Control") != "max-age=86400" || first.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("response: %d %v", first.Code, first.Header())
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(first.Body.Bytes()))
	if err != nil || cfg.Width != 192 || cfg.Height != 96 {
		t.Fatalf("thumb: %+v %v", cfg, err)
	}
	// A repeat serves the same generated file, including Last-Modified.
	second := get(m.ID)
	if !bytes.Equal(first.Body.Bytes(), second.Body.Bytes()) || first.Header().Get("Last-Modified") != second.Header().Get("Last-Modified") {
		t.Fatal("thumbnail was not kept")
	}
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	if got := get(m.ID); got.Code != 404 {
		t.Fatalf("removed picture: %d", got.Code)
	}
	if got := get("gone"); got.Code != 404 {
		t.Fatalf("unknown Mod: %d", got.Code)
	}
}
