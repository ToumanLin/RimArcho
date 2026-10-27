package app

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
)

func TestUninstallSizeCache(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(nested, "data")
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(s), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("abc")
	var c UninstallSizeCache
	if got := c.sizeOf(root); got != 3 {
		t.Fatalf("initial bytes=%d", got)
	}
	write("abcdefg")
	// A nested file edit need not change the top directory's mtime.
	if got := c.sizeOf(root); got != 3 {
		t.Fatalf("cached bytes=%d", got)
	}
	if got := c.sizeOf(nested); got != 7 {
		t.Fatalf("different path bytes=%d", got)
	}
	key := env.PathKey(root)
	c.entries[key] = uninstallSize{bytes: 3, until: time.Now().Add(-time.Second)}
	if got := c.sizeOf(root); got != 7 {
		t.Fatalf("expired bytes=%d", got)
	}
	if _, ok := c.entries[env.PathKey(nested)]; !ok {
		t.Fatal("fresh entry discarded")
	}
	// Path aliases must not bypass the cache.
	write("abcdefghij")
	if got := c.sizeOf(root + string(filepath.Separator) + "."); got != 7 {
		t.Fatalf("normalized path bytes=%d", got)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			if got := c.sizeOf(root); got != 7 {
				t.Errorf("concurrent bytes=%d", got)
			}
		})
	}
	wg.Wait()
}
