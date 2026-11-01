package app_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestSourceLibraryConcurrentRequestsSharePublication(t *testing.T) {
	e, a, b := sourceCopies(t)
	started, finish := make(chan struct{}), make(chan struct{})
	var mu sync.Mutex
	calls := 0
	e.App.Decompile = func(ctx context.Context, _, out, _ string, _ []string) error {
		mu.Lock()
		calls++
		if calls == 1 {
			close(started)
		}
		mu.Unlock()
		select {
		case <-finish:
		case <-ctx.Done():
			return ctx.Err()
		}
		return os.WriteFile(filepath.Join(out, "Shared.cs"), []byte("class Shared {}"), 0o644)
	}
	st, _ := e.App.Loadout()
	da, _ := e.App.SourceDLLs(st, a)
	db, _ := e.App.SourceDLLs(st, b)
	reader, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 2)
	go func() { done <- e.App.DecompileDLL(ctx, st, da[0]) }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	go func() { done <- reader.DecompileDLL(ctx, st, db[0]) }()
	close(finish)
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 || da[0].Dir != db[0].Dir {
		t.Fatalf("calls=%d folders=%s / %s", calls, da[0].Dir, db[0].Dir)
	}
	for _, m := range []*store.Mod{a, b} {
		rs, err := e.App.Store.DecompileRecords(m.ID)
		if err != nil || len(rs) != 1 {
			t.Fatalf("missing use: %v %v", rs, err)
		}
	}
}

func TestSourceLibraryGameBuildReuseAcrossEnvironments(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	e.WriteFile(filepath.Join(e.Game, "RimWorldWin64_Data", "Managed", "Assembly-CSharp.dll"), "synthetic game assembly")
	f := &fakeILSpy{}
	e.App.Decompile = f.run
	e.MustRun("source", "game")
	st, _ := e.App.Loadout()
	first, _ := e.App.GameSourceDLL(st)
	other := *st.Ix.Env
	other.ID, other.Name = "other-game", "Other game"
	if err := e.App.Store.InsertEnv(&other); err != nil {
		t.Fatal(err)
	}
	st.Ix.Env = &other
	shared, err := e.App.GameSourceDLL(st)
	if err != nil || !shared.Cached || shared.Dir != first.Dir {
		t.Fatalf("same build not shared: %+v %v", shared, err)
	}
	other.GameVersion = "1.6.9999"
	if err := e.App.Store.UpdateEnv(&other); err != nil {
		t.Fatal(err)
	}
	newBuild, err := e.App.GameSourceDLL(st)
	if err != nil || newBuild.Cached || newBuild.Dir == first.Dir || newBuild.Hash != first.Hash {
		t.Fatalf("build identity: %+v %v", newBuild, err)
	}
	if err := e.App.DecompileDLL(context.Background(), st, newBuild); err != nil {
		t.Fatal(err)
	}
	e.Open()
	e.App.Decompile = f.run
	e.MustRun("source", "game")
	if len(f.calls) != 2 {
		t.Fatalf("restart lost artifact: %v", f.calls)
	}
	p, err := e.App.CleanSourceLibrary(context.Background(), false)
	if err != nil || p.Folders != 0 {
		t.Fatalf("configured builds considered unused: %+v %v", p, err)
	}
}

func sourceCopies(t *testing.T) (*testkit.Env, *store.Mod, *store.Mod) {
	t.Helper()
	e := testkit.New(t)
	e.HarmonyLogFixture()
	b, err := os.ReadFile(filepath.Join(e.Game, "Mods", "ModA", "Assemblies", "ModA.dll"))
	if err != nil {
		t.Fatal(err)
	}
	e.AddWorkshop("4000000001", testkit.Mod{PackageID: "tester.moda", Name: "Mod A copy", Files: map[string]string{"Assemblies/ModA.dll": string(b)}})
	e.MustRun("scan")
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	a, err := ix.ResolveOne(ref(t, e, "Mod A"))
	if err != nil {
		t.Fatal(err)
	}
	bm, err := ix.ResolveOne("ws:4000000001")
	if err != nil {
		t.Fatal(err)
	}
	return e, a, bm
}

func TestSourceLibrarySharedIdentityAndTypes(t *testing.T) {
	t.Parallel()
	e, a, b := sourceCopies(t)
	f := &fakeILSpy{}
	e.App.Decompile = f.run
	st, _ := e.App.Loadout()
	da, _ := e.App.SourceDLLs(st, a)
	db, _ := e.App.SourceDLLs(st, b)
	_, p, err := e.App.DecompileType(context.Background(), st, da, "ModA.CompBroken")
	if err != nil || !strings.Contains(p, filepath.Join("mods", "tester.moda", "ModA ")) || !strings.Contains(p, string(filepath.Separator)+".types"+string(filepath.Separator)) {
		t.Fatalf("type path %s %v", p, err)
	}
	if dir, err := view.SearchFolder(st.Ix, e.App.SourceDir(), a.ID, p); err != nil || dir != filepath.Dir(p) {
		t.Fatalf("open library folder %s %v", dir, err)
	}
	if _, err := view.SearchFolder(st.Ix, e.App.SourceDir(), a.ID, filepath.Join(e.Home, "archo.db")); err == nil {
		t.Fatal("open folder accepted a path outside source/mod roots")
	}
	_, p2, err := e.App.DecompileType(context.Background(), st, db, "ModA.CompBroken")
	if err != nil || p != p2 || len(f.calls) != 1 {
		t.Fatalf("shared type %s %s %v %v", p, p2, f.calls, err)
	}
	for _, m := range []*store.Mod{a, b} {
		rs, err := e.App.Store.DecompileRecords(m.ID)
		if err != nil || len(rs) != 1 {
			t.Fatalf("type use not recorded for %s: %v %v", m.ID, rs, err)
		}
	}
	r, err := e.App.SourceSearchContext(context.Background(), st, []*store.Mod{a, b}, "AllComps.Remove(", nil, 0, 0, nil)
	if err != nil || r.Total != 1 || r.Assemblies != 1 || len(r.Hits) != 1 || len(r.Hits[0].Shared) != 2 || len(f.calls) != 2 {
		t.Fatalf("dedup: %+v calls=%v err=%v", r, f.calls, err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("type-only output survived whole-project publication", err)
	}
	_, whole, err := e.App.DecompileType(context.Background(), st, da, "ModA.CompBroken")
	if err != nil || strings.Contains(whole, ".types") || len(f.calls) != 2 {
		t.Fatalf("whole-project type %s %v", whole, err)
	}
	rows, err := e.App.Store.SourceArtifacts()
	if err != nil || len(rows) != 1 || rows[0].PackageID != "tester.moda" || rows[0].DLLName != "ModA.dll" || rows[0].ILSpyVersion != "custom" || rows[0].Bytes == 0 || filepath.IsAbs(rows[0].Folder) {
		t.Fatalf("identity %+v %v", rows, err)
	}
	if out, code := e.Run("cache", "clear", "source"); code == 0 || !strings.Contains(out, "unknown cache") {
		t.Fatalf("library clearable: %s", out)
	}
	e.MustRun("cache", "clear", "previews")
	if _, err := os.Stat(whole); err != nil {
		t.Fatal("clear cache removed library", err)
	}
	out := e.MustRun("source", "search", "AllComps.Remove(", "--in", st.Ix.Ref(a.ID), "--in", st.Ix.Ref(b.ID))
	if strings.Count(out, "same code in:") != 1 || !strings.Contains(out, whole) {
		t.Fatalf("CLI attribution/path:\n%s", out)
	}
	page, err := view.FileSearch(context.Background(), e.App, view.FileSearchReq{Pattern: "AllComps.Remove(", Code: true, Mods: []string{a.ID, b.ID}}, nil)
	if err != nil || len(page.Mods) != 1 || !strings.Contains(page.Mods[0].Files[0].Shared, "same code in:") {
		t.Fatalf("view attribution %+v %v", page, err)
	}
}

func TestSourceLibraryRelocation(t *testing.T) {
	for _, cross := range []bool{false, true} {
		t.Run(map[bool]string{false: "rename", true: "cross-volume"}[cross], func(t *testing.T) {
			e := testkit.New(t)
			e.HarmonyLogFixture()
			f := &fakeILSpy{}
			e.App.Decompile = f.run
			r := ref(t, e, "Mod A")
			e.MustRun("source", r)
			st, _ := e.App.Loadout()
			m, _ := st.Ix.ResolveOne(r)
			ds, _ := e.App.SourceDLLs(st, m)
			rel, _ := filepath.Rel(e.App.SourceDir(), ds[0].Dir)
			to := filepath.Join(e.Root, "Readable Source")
			e.App.SameVolume = func(string, string) bool { return !cross }
			e.App.TrashFunc = func(p string) error { return os.RemoveAll(p) }
			copied, err := e.App.MoveDirectory("source", to)
			if err != nil || copied != cross {
				t.Fatalf("move copied=%v err=%v", copied, err)
			}
			e.Open()
			e.App.Decompile = f.run
			if e.App.SourceDir() != to {
				t.Fatal("location did not survive restart")
			}
			out := e.MustRun("source", r)
			if len(f.calls) != 1 || !strings.Contains(out, filepath.Join(to, rel)) {
				t.Fatalf("moved lookup:\n%s", out)
			}
			manual := filepath.Join(e.Root, "Manual Source")
			if err := os.Rename(to, manual); err != nil {
				t.Fatal(err)
			}
			if err := e.App.SetDirectory("source", manual); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(e.MustRun("source", r), manual) {
				t.Fatal("relocate did not rebind")
			}
			if err := e.App.SetDirectory("source", filepath.Join(e.Home, "cache")); err == nil {
				t.Fatal("overlapping location accepted")
			}
		})
	}
}

func TestSourceLibraryGameBuildAndCleanup(t *testing.T) {
	t.Parallel()
	e, a, b := sourceCopies(t)
	f := &fakeILSpy{}
	e.App.Decompile = f.run
	managed := filepath.Join(e.Game, "RimWorldWin64_Data", "Managed")
	e.WriteFile(filepath.Join(managed, "Assembly-CSharp.dll"), "synthetic game assembly")
	st, _ := e.App.Loadout()
	game, err := e.App.GameSourceDLL(st)
	if err != nil || !strings.HasSuffix(game.Dir, filepath.Join("game", "1.6.4633", "Assembly-CSharp")) {
		t.Fatalf("game %+v %v", game, err)
	}
	e.MustRun("source", "game", "--type", "Verse.Pawn")
	e.MustRun("source", "game")
	gameFile := filepath.Join(game.Dir, "ModA", "CompBroken.cs")
	if dir, err := view.SearchFolder(st.Ix, e.App.SourceDir(), game.Instance, gameFile); err != nil || dir != filepath.Dir(gameFile) {
		t.Fatalf("open game source folder %s %v", dir, err)
	}
	out := e.MustRun("source", "search", "AllComps.Remove(", "--game")
	if !strings.Contains(out, "game  ") || !strings.Contains(out, game.Dir) {
		t.Fatalf("game search:\n%s", out)
	}
	e.MustRun("source", st.Ix.Ref(a.ID))
	old, _ := e.App.SourceDLLs(st, a)
	// Inactive installed copies and locked copies retain shared code.
	e.MustRun("swap", st.Ix.Ref(a.ID), "ws:4000000001")
	e.MustRun("pin", "ws:4000000001")
	e.WriteFile(filepath.Join(e.Game, "Mods", "ModA", "Assemblies", "ModA.dll"), "new DLL revision")
	if err := os.RemoveAll(filepath.Join(e.Workshop, "4000000001")); err != nil {
		t.Fatal(err)
	}
	e.MustRun("scan")
	p, err := e.App.CleanSourceLibrary(context.Background(), false)
	if err != nil || p.Folders != 0 {
		t.Fatalf("locked copy not retained: %+v %v (copy %s)", p, err, b.ID)
	}
	if err := os.RemoveAll(e.App.LockDir()); err != nil {
		t.Fatal(err)
	}
	e.MkdirAll(e.App.LockDir())
	e.MustRun("scan")
	// A changed environment build makes only the previous game build unused.
	env, _ := e.App.Env()
	env.GameVersion = "1.6.9999"
	if err := e.App.Store.UpdateEnv(env); err != nil {
		t.Fatal(err)
	}
	p, err = e.App.CleanSourceLibrary(context.Background(), false)
	if err != nil || p.Folders != 2 || p.Bytes == 0 {
		t.Fatalf("unused preview %+v %v", p, err)
	}
	if _, err := os.Stat(old[0].Dir); err != nil {
		t.Fatal("preview removed code", err)
	}
	if out := e.MustRun("source", "clean", "--dry-run"); !strings.Contains(out, "2 folders unused") {
		t.Fatal(out)
	}
	e.MustRun("source", "clean", "--yes")
	if _, err := os.Stat(old[0].Dir); !os.IsNotExist(err) {
		t.Fatal("unused mod code remains", err)
	}
	if _, err := os.Stat(game.Dir); !os.IsNotExist(err) {
		t.Fatal("unused game code remains", err)
	}
}

func TestSourceLibraryLegacyDiscardOnceAndCancellation(t *testing.T) {
	e := testkit.New(t)
	legacy := filepath.Join(e.Home, "cache", "source", "old", "Old.cs")
	e.WriteFile(legacy, "old output")
	if _, err := e.App.Store.DB.Exec(`DELETE FROM meta WHERE key='source-library:legacy-cleared'`); err != nil {
		t.Fatal(err)
	}
	e.Open()
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("legacy cache retained", err)
	}
	e.WriteFile(legacy, "created after migration")
	e.Open()
	if _, err := os.Stat(legacy); err != nil {
		t.Fatal("migration repeated", err)
	}
	e.HarmonyLogFixture()
	started := make(chan struct{})
	var once sync.Once
	e.App.Decompile = func(ctx context.Context, _, out, typ string, _ []string) error {
		e.MkdirAll(out)
		e.WriteFile(filepath.Join(out, "Partial.cs"), "partial")
		once.Do(func() { close(started) })
		<-ctx.Done()
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- e.App.DecompileActive(ctx, nil) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("decompile did not start")
	}
	reader, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.MoveDirectory("source", filepath.Join(e.Root, "Moved")); err == nil {
		t.Fatal("move raced decompiler")
	}
	reader.Close()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	filepath.WalkDir(e.App.SourceDir(), func(p string, d os.DirEntry, err error) error {
		if err == nil && (strings.HasPrefix(d.Name(), ".decompile-") || d.Name() == ".done") {
			t.Errorf("partial publication %s", p)
		}
		return nil
	})
}
