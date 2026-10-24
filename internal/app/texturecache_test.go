package app_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/textures"
)

func TestTextureSummaryCacheHitAndInvalidation(t *testing.T) {
	e, _, dir := texSetup(t)
	targets, err := e.App.TextureTargets([]string{"pkg:tester.tex"}, app.TexRefs)
	if err != nil {
		t.Fatal(err)
	}
	read := func(scale int) *app.TextureReport {
		t.Helper()
		r, err := e.App.TextureSummaryTargets(context.Background(), targets, textures.Options{Scale: scale})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	first := read(100)
	realDir, err := source.RealPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(e.App.Home, "cache", "textures", fmt.Sprintf("%x.json", sha256.Sum256([]byte(store.PathKey(realDir)))))
	if err := os.Remove(cache); err != nil {
		t.Fatal(err)
	}
	// A summary hit needs neither per-PNG headers nor a rewritten cache, including
	// across reader sessions. Removing the header cache makes a fallback observable.
	reader, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	hit, err := reader.TextureSummaryTargets(context.Background(), targets, textures.Options{})
	if err != nil || hit.Est != first.Est {
		t.Fatalf("persisted hit differs: %v", err)
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatal("summary hit collected PNG headers again")
	}
	equalFull := func(scale int) {
		t.Helper()
		summary := read(scale)
		full, err := e.App.TextureStatus([]string{"pkg:tester.tex"}, app.TexRefs, textures.Options{Scale: scale})
		if err != nil {
			t.Fatal(err)
		}
		a, b := summary.Mods[0], full.Mods[0]
		if summary.Est != full.Est || a.PNG != b.PNG || a.Foreign != b.Foreign || a.Ours != b.Ours || a.Stale != b.Stale || a.Orphans != b.Orphans || !reflect.DeepEqual(a.Plan.Skipped, b.Plan.Skipped) {
			t.Fatal("cached summary differs from fresh collection")
		}
	}
	texPNG(t, filepath.Join(dir, "Textures", "Things", "A.png"), 256, 128)
	equalFull(100) // overwrite without a scan
	equalFull(50)  // scale is part of the cache key
	added := filepath.Join(dir, "1.7", "Textures", "new.png")
	texPNG(t, added, 128, 128)
	equalFull(50)
	if err := os.Remove(added); err != nil {
		t.Fatal(err)
	}
	equalFull(50)
	if _, err := e.App.OptimizeTextures(app.TextureRun{Refs: []string{"pkg:tester.tex"}}); err != nil {
		t.Fatal(err)
	}
	equalFull(100) // owned DDS records invalidate the summary
	if _, err := e.App.CleanTextures([]string{"pkg:tester.tex"}, app.TexRefs, false); err != nil {
		t.Fatal(err)
	}
	equalFull(100)
	summaries := filepath.Join(e.App.Home, "cache", "textures", "summaries-v1.json")
	if err := os.WriteFile(summaries, []byte("{damaged"), 0644); err != nil {
		t.Fatal(err)
	}
	equalFull(100)
}
