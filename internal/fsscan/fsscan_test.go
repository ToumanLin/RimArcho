package fsscan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func generatedTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for i := 0; i < 500; i++ {
		rel := fmt.Sprintf("Group%02d/Sub%02d/File%03d.Dat", i%10, (i/10)%5, i)
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(strings.Repeat("x", i%103+1)), 0644); err != nil {
			t.Fatal(err)
		}
		mtime := time.Unix(1700000000+int64(i), 0)
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"Empty", "Zebra", "alpha", "Äpfel", "ångström", "Σ"} {
		if err := os.Mkdir(filepath.Join(root, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// Fix directory times after construction so deferred filesystem updates
	// cannot change the fixture between the reference walk and parallel walks.
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			mtime := time.Unix(1700000000, 0)
			return os.Chtimes(path, mtime, mtime)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func referenceOrder(a, b Entry) int {
	if c := strings.Compare(strings.ToLower(a.Rel), strings.ToLower(b.Rel)); c != 0 {
		return c
	}
	return strings.Compare(a.Rel, b.Rel)
}

func referenceWalk(t *testing.T, root string, o Options) *Tree {
	t.Helper()
	tree := &Tree{Root: root}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if d.Type() != 0 && d.Type() != fs.ModeDir {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		entry := Entry{rel, info.Size(), info.ModTime().UnixNano(), d.IsDir()}
		if d.IsDir() {
			if o.Dirs {
				tree.Entries = append(tree.Entries, entry)
			}
			if o.SkipDir != nil && o.SkipDir(rel, d.Name()) {
				return filepath.SkipDir
			}
		} else if o.Keep == nil || o.Keep(rel, d.Name()) {
			tree.Entries = append(tree.Entries, entry)
			tree.Files++
			tree.Bytes += info.Size()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(tree.Entries, referenceOrder)
	return tree
}

func assertTree(t *testing.T, got, want *Tree) {
	t.Helper()
	if got.Files != want.Files || got.Bytes != want.Bytes || got.Skipped != want.Skipped {
		t.Fatalf("totals = (%d, %d, %d), want (%d, %d, %d)", got.Files, got.Bytes, got.Skipped, want.Files, want.Bytes, want.Skipped)
	}
	if !reflect.DeepEqual(got.Entries, want.Entries) {
		for i := 0; i < min(len(got.Entries), len(want.Entries)); i++ {
			if got.Entries[i] != want.Entries[i] {
				t.Fatalf("entry %d = %+v, want %+v", i, got.Entries[i], want.Entries[i])
			}
		}
		t.Fatalf("entries differ from WalkDir: got %d entries, want %d", len(got.Entries), len(want.Entries))
	}
}

func TestWalk(t *testing.T) {
	root := generatedTree(t)
	want := referenceWalk(t, root, Options{Dirs: true})
	if want.Files != 500 {
		t.Fatalf("fixture contains %d files", want.Files)
	}
	var stamp [16]byte
	for _, workers := range []int{1, 4, 8, 16, 0, -1} {
		t.Run(fmt.Sprint(workers), func(t *testing.T) {
			got, err := Walk(t.Context(), root, Options{Workers: workers, Dirs: true, Sorted: true})
			if err != nil {
				t.Fatal(err)
			}
			assertTree(t, got, want)
			real, err := filepath.EvalSymlinks(root)
			if err != nil || got.Root != real {
				t.Fatalf("Root = %q, want %q (error %v)", got.Root, real, err)
			}
			if workers == 1 {
				stamp = got.Hash()
			} else if got.Hash() != stamp {
				t.Fatal("hash changed with worker count")
			}
		})
	}
	got, err := Walk(t.Context(), root, Options{Workers: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 500 {
		t.Fatalf("default entry count = %d, want 500 files", len(got.Entries))
	}
	for _, workers := range []int{1, 16, 0} {
		files, bytes, err := Size(t.Context(), root, workers)
		if err != nil || files != got.Files || bytes != got.Bytes {
			t.Fatalf("Size(%d) = (%d, %d, %v), Walk = (%d, %d)", workers, files, bytes, err, got.Files, got.Bytes)
		}
	}
	counted, err := walk(t.Context(), root, Options{Workers: 4}, false)
	if err != nil || counted.Entries != nil {
		t.Fatalf("count-only walk retained entries: %v", err)
	}
}

func TestWalkOptions(t *testing.T) {
	root := generatedTree(t)
	skip := func(rel, name string) bool { return name == "Sub02" }
	keep := func(rel, name string) bool { return strings.Contains(name, "1") }
	for _, dirs := range []bool{false, true} {
		t.Run(fmt.Sprintf("dirs=%t", dirs), func(t *testing.T) {
			var mu sync.Mutex
			seenDirs, seenFiles := map[string]int{}, map[string]int{}
			o := Options{Workers: 8, Dirs: dirs, Sorted: true,
				SkipDir: func(rel, name string) bool {
					mu.Lock()
					seenDirs[rel]++
					mu.Unlock()
					if name != filepath.Base(filepath.FromSlash(rel)) {
						t.Errorf("SkipDir name %q does not match %q", name, rel)
					}
					return skip(rel, name)
				},
				Keep: func(rel, name string) bool {
					mu.Lock()
					seenFiles[rel]++
					mu.Unlock()
					if name != filepath.Base(filepath.FromSlash(rel)) {
						t.Errorf("Keep name %q does not match %q", name, rel)
					}
					return keep(rel, name)
				},
			}
			got, err := Walk(t.Context(), root, o)
			if err != nil {
				t.Fatal(err)
			}
			want := referenceWalk(t, root, Options{Dirs: dirs, SkipDir: skip, Keep: keep})
			assertTree(t, got, want)
			all := referenceWalk(t, root, Options{Dirs: true, SkipDir: skip})
			for _, e := range all.Entries {
				calls := seenFiles[e.Rel]
				if e.Dir {
					calls = seenDirs[e.Rel]
				}
				if calls != 1 {
					t.Errorf("callback for %q called %d times", e.Rel, calls)
				}
			}
			if len(seenFiles) != 400 || len(seenDirs) != 66 {
				t.Fatalf("callback counts = (%d dirs, %d files), want (66, 400)", len(seenDirs), len(seenFiles))
			}
			if !slices.IsSortedFunc(got.Entries, referenceOrder) {
				t.Fatal("entries are not in canonical order")
			}
		})
	}
}

func TestCanonicalOrder(t *testing.T) {
	var entries []Entry
	for _, rel := range []string{"z", "A", "a", "Aa", "AA", "Ä", "ä", "İ", "K", "Σ", "σ", "ς", "\xff", "\xef\xbf\xbd"} {
		entries = append(entries, Entry{Rel: rel})
	}
	want := slices.Clone(entries)
	slices.SortFunc(want, referenceOrder)
	slices.SortFunc(entries, compareEntries)
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("canonical order = %v, want %v", entries, want)
	}
}

func TestHash(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Original.dat")
	write := func(contents string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	mtime := time.Unix(1700000000, 0)
	setTime := func(stamp time.Time) {
		t.Helper()
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	hash := func() [16]byte {
		t.Helper()
		tree, err := Walk(t.Context(), root, Options{Workers: 4})
		if err != nil {
			t.Fatal(err)
		}
		return tree.Hash()
	}
	write("a")
	setTime(mtime)
	original := hash()
	write("longer")
	setTime(mtime)
	if hash() == original {
		t.Fatal("size change did not change hash")
	}
	write("a")
	setTime(mtime.Add(time.Second))
	if hash() == original {
		t.Fatal("mtime change did not change hash")
	}
	setTime(mtime)
	if hash() != original {
		t.Fatal("restoring size and mtime did not restore hash")
	}
	if err := os.Rename(path, filepath.Join(root, "Renamed.dat")); err != nil {
		t.Fatal(err)
	}
	if hash() == original {
		t.Fatal("name change did not change hash")
	}

	entries := []Entry{{Rel: "z", Size: 4, MTime: 6}, {Rel: "Ä/İ", Size: 2, MTime: 3}, {Rel: "A", Dir: true}}
	tree := &Tree{Entries: slices.Clone(entries)}
	stamp := tree.Hash()
	if !reflect.DeepEqual(tree.Entries, entries) {
		t.Fatal("Hash reordered Entries")
	}
	slices.Reverse(tree.Entries)
	if tree.Hash() != stamp {
		t.Fatal("Hash depends on entry order")
	}
	for i := range tree.Entries {
		tree.Entries[i].Rel = strings.ToLower(tree.Entries[i].Rel)
	}
	if tree.Hash() != stamp {
		t.Fatal("case-only path changes changed Hash")
	}
	tree.Entries[0].Dir = !tree.Entries[0].Dir
	if tree.Hash() == stamp {
		t.Fatal("directory flag did not change Hash")
	}
	long := strings.Repeat("ÄbC/", 400)
	if (&Tree{Entries: []Entry{{Rel: long}}}).Hash() != (&Tree{Entries: []Entry{{Rel: strings.ToLower(long)}}}).Hash() {
		t.Fatal("long paths are not lower-cased consistently")
	}
}

func TestCancellation(t *testing.T) {
	root := generatedTree(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Walk(ctx, root, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled Walk error = %v", err)
	}
	if _, _, err := Size(ctx, root, 4); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled Size error = %v", err)
	}
	for _, workers := range []int{1, 16} {
		t.Run(fmt.Sprint(workers), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var calls atomic.Int32
			done := make(chan error, 1)
			go func() {
				_, err := Walk(ctx, root, Options{Workers: workers, Keep: func(string, string) bool {
					calls.Add(1)
					cancel()
					return true
				}})
				done <- err
			}()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("Walk error = %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("canceled workers did not stop")
			}
			if n := calls.Load(); n == 0 || n >= 500 {
				t.Fatalf("visited %d files before stopping", n)
			}
		})
	}
}

func TestRootErrors(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{file, filepath.Join(root, "missing")} {
		if _, err := Walk(t.Context(), path, Options{Workers: 4}); err == nil {
			t.Errorf("Walk(%q) succeeded", path)
		}
		if _, _, err := Size(t.Context(), path, 4); err == nil {
			t.Errorf("Size(%q) succeeded", path)
		}
	}
}

func TestUnreadableDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions need Windows ACLs")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	if err := os.Mkdir(locked, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0700) })
	if _, err := os.ReadDir(locked); err == nil {
		t.Skip("process can read directories without permission")
	}
	tree, err := Walk(t.Context(), root, Options{Workers: 4})
	if err != nil || tree.Skipped != 1 {
		t.Fatalf("Walk = (%v, %v), want one skipped directory", tree, err)
	}
	if _, err := Walk(t.Context(), locked, Options{Workers: 4}); err == nil {
		t.Fatal("unreadable root succeeded")
	}
}

func TestDisappearingDirectory(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "gone")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	tree, err := Walk(t.Context(), root, Options{Workers: 4, SkipDir: func(string, string) bool {
		if err := os.Remove(child); err != nil {
			t.Error(err)
		}
		return false
	}})
	if err != nil || tree.Skipped != 1 {
		t.Fatalf("Walk = (%v, %v), want one skipped directory", tree, err)
	}
}

func TestSymlinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "inside"), []byte("in"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "outside"), []byte("out"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "directory-link")); err != nil {
		t.Skipf("cannot create symlinks: %v", err)
	}
	for _, link := range []struct{ target, name string }{
		{root, "loop"},
		{filepath.Join(root, "inside"), "file-link"},
		{filepath.Join(root, "missing"), "broken-link"},
	} {
		if err := os.Symlink(link.target, filepath.Join(root, link.name)); err != nil {
			t.Fatal(err)
		}
	}
	tree, err := Walk(t.Context(), root, Options{Workers: 8, Dirs: true})
	if err != nil || tree.Files != 1 || tree.Bytes != 2 || len(tree.Entries) != 1 || tree.Skipped != 0 {
		t.Fatalf("walk followed or returned a link: (%v, %v)", tree, err)
	}
	linkRoot := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(root, linkRoot); err != nil {
		t.Fatal(err)
	}
	linked, err := Walk(t.Context(), linkRoot, Options{Workers: 8, Dirs: true})
	if err != nil {
		t.Fatal(err)
	}
	assertTree(t, linked, tree)
	if linked.Root != tree.Root {
		t.Fatalf("linked Root = %q, want %q", linked.Root, tree.Root)
	}
	files, bytes, err := Size(t.Context(), linkRoot, 8)
	if err != nil || files != 1 || bytes != 2 {
		t.Fatalf("linked Size = (%d, %d, %v)", files, bytes, err)
	}
}

func TestCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "cache.json")
	cache := OpenCache(path)
	stamp := [16]byte{1, 2, 3}
	var got map[string]int
	if cache.Get("mod", "size", stamp, &got) {
		t.Fatal("missing cache hit")
	}
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("unchanged empty cache created its directory: %v", err)
	}
	input := map[string]int{"bytes": 42}
	cache.Put("mod", "size", stamp, input)
	input["bytes"] = 100
	cache.Put("mod", "loadcost/v1", stamp, 7)
	cache.Put("old", "size", stamp, 9)
	if !cache.Get("mod", "size", stamp, &got) || got["bytes"] != 42 {
		t.Fatalf("Put did not marshal immediately: %v", got)
	}
	if cache.Get("mod", "size", [16]byte{9}, &got) {
		t.Fatal("stamp mismatch hit")
	}
	var incompatible []string
	if cache.Get("mod", "size", stamp, &incompatible) || cache.Get("mod", "size", stamp, nil) {
		t.Fatal("invalid destination hit")
	}
	cache.Put("mod", "size", stamp, func() {})
	if !cache.Get("mod", "size", stamp, &got) || got["bytes"] != 42 {
		t.Fatal("failed marshal replaced a value")
	}
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var disk struct {
		Version int `json:"v"`
		Entries map[string]map[string]struct {
			Stamp string          `json:"stamp"`
			Value json.RawMessage `json:"value"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(data, &disk); err != nil || disk.Version != 1 || disk.Entries["mod"]["size"].Stamp != "01020300000000000000000000000000" || string(disk.Entries["mod"]["size"].Value) != `{"bytes":42}` {
		t.Fatalf("unexpected cache format: %s (error %v)", data, err)
	}
	cache = OpenCache(path)
	if !cache.Get("mod", "size", stamp, &got) || got["bytes"] != 42 {
		t.Fatal("round trip lost value")
	}
	var number int
	if !cache.Get("mod", "loadcost/v1", stamp, &number) || number != 7 {
		t.Fatal("consumer names collided")
	}
	oldTime := time.Unix(1600000000, 0)
	if err := os.Chtimes(path, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	cache.Put("mod", "size", stamp, map[string]int{"bytes": 42})
	cache.Prune(func(string) bool { return true })
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.ModTime().Equal(oldTime) {
		t.Fatalf("unchanged Save rewrote the file: %v", err)
	}
	cache.Prune(func(key string) bool { return key == "mod" })
	if cache.Get("old", "size", stamp, &number) {
		t.Fatal("Prune kept an unwanted key")
	}
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}
	cache = OpenCache(path)
	if cache.Get("old", "size", stamp, &number) || !cache.Get("mod", "size", stamp, &got) {
		t.Fatal("Prune did not persist correctly")
	}
	cache.Put("mod", "size", [16]byte{4}, map[string]int{"bytes": 99})
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}
	if OpenCache(path).Get("mod", "size", stamp, &got) || !OpenCache(path).Get("mod", "size", [16]byte{4}, &got) || got["bytes"] != 99 {
		t.Fatal("updated stamp and value did not persist")
	}
}

func TestCacheDamaged(t *testing.T) {
	for _, data := range []string{
		"", "{", "null", `{"v":2,"entries":{}}`, `{"v":1,"entries":{}} garbage`,
		`{"v":1,"entries":{"key":{"name":{"stamp":"bad","value":1}}}}`,
		`{"v":1,"entries":{"key":{"name":{"stamp":"00000000000000000000000000000000"}}}}`,
		`{"v":1,"entries":[]}`,
	} {
		t.Run(data, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cache.json")
			if err := os.WriteFile(path, []byte(data), 0644); err != nil {
				t.Fatal(err)
			}
			cache := OpenCache(path)
			var value int
			if cache.Get("key", "name", [16]byte{}, &value) {
				t.Fatal("damaged cache hit")
			}
			cache.Put("key", "name", [16]byte{}, 123)
			if err := cache.Save(); err != nil {
				t.Fatal(err)
			}
			if !OpenCache(path).Get("key", "name", [16]byte{}, &value) || value != 123 {
				t.Fatal("damaged cache was not replaced")
			}
		})
	}
}

func TestCacheReadLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxCacheBytes + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	var value int
	if OpenCache(path).Get("key", "name", [16]byte{}, &value) {
		t.Fatal("oversized cache hit")
	}
}

func TestCacheSaveRetry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.json")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	cache := OpenCache(path)
	cache.Put("key", "name", [16]byte{}, 1)
	if err := cache.Save(); err == nil {
		t.Fatal("Save replaced a directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("failed Save left temporary files: %v, %v", entries, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}
	var value int
	if !OpenCache(path).Get("key", "name", [16]byte{}, &value) || value != 1 {
		t.Fatal("failed Save lost pending changes")
	}
}

func TestCacheConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	cache := OpenCache(path)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for worker := 0; worker < 8; worker++ {
		wg.Go(func() {
			<-start
			for i := 0; i < 32; i++ {
				key := fmt.Sprintf("%d/%d", worker, i)
				cache.Put(key, "name", [16]byte{}, i)
				var value int
				if !cache.Get(key, "name", [16]byte{}, &value) || value != i {
					t.Errorf("concurrent Get(%q) = %d", key, value)
				}
			}
		})
	}
	for saver := 0; saver < 2; saver++ {
		wg.Go(func() {
			<-start
			for i := 0; i < 8; i++ {
				cache.Prune(func(string) bool { return true })
				if err := cache.Save(); err != nil {
					t.Error(err)
				}
			}
		})
	}
	close(start)
	wg.Wait()
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}
	loaded := OpenCache(path)
	for worker := 0; worker < 8; worker++ {
		for i := 0; i < 32; i++ {
			key := fmt.Sprintf("%d/%d", worker, i)
			var value int
			if !loaded.Get(key, "name", [16]byte{}, &value) || value != i {
				t.Errorf("concurrent save lost %q", key)
			}
		}
	}
}

func BenchmarkWalk(b *testing.B) {
	root := os.Getenv("RIMARCHO_FSSCAN_BENCH_ROOT")
	if root == "" {
		b.Skip("set RIMARCHO_FSSCAN_BENCH_ROOT to a directory to benchmark")
	}
	for _, workers := range []int{1, 4, 8, 16} {
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := Walk(b.Context(), root, Options{Workers: workers}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
