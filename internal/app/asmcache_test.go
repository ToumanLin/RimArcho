package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/harmony"
)

// The Harmony fact cache (05 §5.3): what Put stores Get returns; a check's prune keeps what
// it used and what was used lately, and removes the rest and stale temporary files.
func TestAssemblyFileCache(t *testing.T) {
	a := &App{Home: t.TempDir()}
	c := a.assemblyCache()
	asm := &harmony.Assembly{Types: []string{"A.B", "A.B+C"}, Patches: []harmony.Patch{{PatchType: "P", PatchMethod: "Pre", Kind: harmony.Prefix,
		TargetType: "Verse.Pawn", TargetMethod: "Tick", Skips: true}}, Manual: true, UsesHarm: true, IDs: []string{"x.y"},
		ManualCalls: 2, ManualResolved: 1, Unresolved: map[string]int{"field": 1}}
	if _, ok := c.Get("k1"); ok {
		t.Fatal("empty cache hit")
	}
	c.Put("k1", asm)
	got, ok := c.Get("k1")
	if !ok || !reflect.DeepEqual(got, asm) {
		t.Fatalf("Get = %+v, %v; want %+v", got, ok, asm)
	}
	old := time.Now().Add(-asmCacheUnused - time.Hour)
	for _, k := range []string{"k1", "used-lately"} {
		c.Put(k, asm)
	}
	c.Put("unused", asm)
	os.Chtimes(c.file("unused"), old, old)
	os.Chtimes(c.file("k1"), old, old)
	tmp := filepath.Join(c.dir, "123.tmp")
	os.WriteFile(tmp, nil, 0o644)
	os.Chtimes(tmp, old, old)

	next := a.assemblyCache() // the next check uses only k1, whose old time Get refreshes
	if _, ok := next.Get("k1"); !ok {
		t.Fatal("k1 gone")
	}
	next.prune()
	for name, want := range map[string]bool{"k1.json": true, "used-lately.json": true, "unused.json": false, "123.tmp": false} {
		if _, err := os.Stat(filepath.Join(c.dir, name)); (err == nil) != want {
			t.Errorf("%s kept = %v, want %v", name, err == nil, want)
		}
	}
	if fi, _ := os.Stat(c.file("k1")); time.Since(fi.ModTime()) > time.Minute {
		t.Error("a used entry's time was not refreshed")
	}
	var none *asmCache
	none.prune() // no data directory
	if (&App{}).assemblyCache() != nil {
		t.Error("a cache without a data directory")
	}
}
