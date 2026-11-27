package analyze

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/harmony"
)

// memCache is an AssemblyCache in memory that stores what the app's file cache stores: the
// facts as JSON.
type memCache struct {
	mu         sync.Mutex
	m          map[string][]byte
	gets, puts int
}

func (c *memCache) Get(key string) (*harmony.Assembly, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gets++
	b, ok := c.m[key]
	if !ok {
		return nil, false
	}
	var a harmony.Assembly
	if json.Unmarshal(b, &a) != nil {
		return nil, false
	}
	return &a, true
}

func (c *memCache) Put(key string, a *harmony.Assembly) {
	b, _ := json.Marshal(a)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.puts++
	c.m[key] = b
}

// A check with the Harmony fact cache gives the same assemblies and findings as one that
// reads every DLL, and a repeated check reads only the DLLs that cannot be read (05 §5.3,
// 18 §2). Mods sharing a DLL share its entry; the skip flag is part of the key.
func TestAssemblyCacheSameResults(t *testing.T) {
	bin := filepath.Join("..", "..", "testdata", "dotnet", "bin")
	root := t.TempDir()
	cp := func(dir, name string) {
		b, err := os.ReadFile(filepath.Join(bin, name))
		if err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(dir, "Assemblies", name), string(b))
	}
	var mods []*Mod
	for i, d := range []struct{ name, pkg string }{{"a", "test.a"}, {"b", "test.b"}, {"c", "test.c"}, {"h", "brrainz.harmony"}} {
		m := &Mod{Instance: d.name, Pkg: d.pkg, Name: d.name, Pos: i + 1, Dir: filepath.Join(root, d.name), SnapHash: []byte(d.name)}
		mods = append(mods, m)
	}
	cp(mods[0].Dir, "ModA.dll")
	cp(mods[0].Dir, "ModManual.dll")
	cp(mods[1].Dir, "ModB.dll")
	cp(mods[1].Dir, "ModA.dll") // the same DLL in another Mod
	cp(mods[2].Dir, "ModC.dll")
	cp(mods[2].Dir, "0Harmony.dll") // skipped here
	cp(mods[3].Dir, "0Harmony.dll")
	write(t, filepath.Join(mods[2].Dir, "Assemblies", "Broken.dll"), "not a PE file")

	reads := atomic.Int32{}
	old := readDLL
	readDLL = func(p string, skip bool) (*harmony.Assembly, error) { reads.Add(1); return old(p, skip) }
	t.Cleanup(func() { readDLL = old })

	run := func(c AssemblyCache) string {
		t.Helper()
		reads.Store(0)
		out, err := Run(Input{Mods: mods, MM: "1.6", Assemblies: c})
		if err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(struct {
			A []*AssemblyFacts
			F []*Finding
		}{out.Assemblies, out.Findings})
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	want := run(nil)
	if reads.Load() != 8 {
		t.Fatalf("without a cache %d DLLs read, want 8", reads.Load())
	}
	c := &memCache{m: map[string][]byte{}}
	if got := run(c); got != want {
		t.Errorf("first check with the cache differs:\n%s\nwant\n%s", got, want)
	}
	// ModA.dll and 0Harmony.dll twice (skipped in both Mods): one entry each; the broken
	// DLL is never stored. Two workers may read a shared DLL at the same time and both store it.
	if len(c.m) != 5 {
		t.Errorf("stored %d entries, want 5", len(c.m))
	}
	if got := run(c); got != want {
		t.Errorf("repeated check differs:\n%s\nwant\n%s", got, want)
	}
	if reads.Load() != 1 {
		t.Errorf("repeated check read %d DLLs, want only the broken one", reads.Load())
	}
}

// readOnce reads each file once per run, and keeps no more than readOnceCap.
func TestReadOnce(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.xml"), filepath.Join(dir, "b.xml")
	write(t, a, "<Defs/>")
	write(t, b, "<Defs><x/></Defs>")
	n := 0
	old := readFile
	readFile = func(p string) ([]byte, error) { n++; return old(p) }
	t.Cleanup(func() { readFile = old })
	s := &sim{}
	for range 3 {
		for _, p := range []string{a, b} {
			if got, err := s.readOnce(p); err != nil || string(got) != map[string]string{a: "<Defs/>", b: "<Defs><x/></Defs>"}[p] {
				t.Fatalf("readOnce(%s) = %q, %v", p, got, err)
			}
		}
	}
	if n != 2 {
		t.Errorf("%d reads, want 2", n)
	}
	s = &sim{readBytes: readOnceCap - 8}
	s.readOnce(a)
	s.readOnce(b) // over the cap: read, not kept
	s.readOnce(b)
	if n != 5 || len(s.read) != 1 {
		t.Errorf("at the cap: %d reads, %d kept; want 5 and 1", n, len(s.read))
	}
	if _, err := s.readOnce(filepath.Join(dir, "missing.xml")); err == nil {
		t.Error("a missing file reads")
	}
}
