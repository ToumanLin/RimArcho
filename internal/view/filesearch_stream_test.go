package view_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/modsearch"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestFileSearchKinds(t *testing.T) {
	for _, tc := range []struct {
		text  string
		kinds modsearch.Kind
		code  bool
	}{
		{"", modsearch.DefaultKinds, false}, {"dEfS PATCHES About", modsearch.DefaultKinds, false},
		{"C#", 0, true}, {"translations textures sounds", modsearch.Languages | modsearch.Textures | modsearch.Sounds, false},
		{"-About", modsearch.DefaultKinds &^ modsearch.About, false}, {"C# Defs -C#", modsearch.Defs, false},
	} {
		k, c, err := view.ParseFileSearchKinds(tc.text)
		if err != nil || k != tc.kinds || c != tc.code {
			t.Fatalf("%q: %v %v %v", tc.text, k, c, err)
		}
	}
	if _, _, err := view.ParseFileSearchKinds("Defs typo"); err == nil {
		t.Fatal("unknown kind accepted")
	}
}

func TestFileSearchCopiesDefaultsAndDeltas(t *testing.T) {
	e := testkit.New(t)
	e.AddWorkshop("4000000001", testkit.Sample("sample.same", "Same"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Same"), testkit.Sample("sample.same", "Same"))
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "sample.same")
	e.AddEnv(false)
	e.MustRun("scan")
	lib, err := e.App.FileSearchLibrary()
	if err != nil {
		t.Fatal(err)
	}
	info, err := view.BuildFileSearchInfo(lib, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if info.Files != "Defs patches About" || !strings.HasPrefix(info.Mods, `lists:"`) {
		t.Fatalf("defaults %+v", info)
	}
	currentQuery := info.Mods
	other, err := e.App.Store.CreatePlan(lib.Env.ID, "Other edited list", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	e.App.OpenPlan = other.ID
	edited, err := e.App.FileSearchLibrary()
	if err != nil {
		t.Fatal(err)
	}
	info, err = view.BuildFileSearchInfo(edited, "", true)
	if err != nil || info.Mods != currentQuery || edited.St.Plan.ID != other.ID {
		t.Fatalf("current-list default while editing another list: %+v, %v", info, err)
	}
	e.App.OpenPlan = 0
	textureMod := lib.Lineages[0].Instances[0].Mod
	if err := e.App.Store.PutTextureFiles([]store.TextureFile{{Path: filepath.Join(e.Root, "stored.dds"), InstanceID: textureMod.ID, MadeAt: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	reader, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	cold, err := reader.FileSearchLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if reader.IndexCached() {
		t.Fatal("page snapshot triggered normal index loading and filesystem alias repair")
	}
	storedTexture := false
	for _, group := range cold.Lineages {
		for _, copy := range group.Instances {
			if copy.Mod.ID == textureMod.ID && copy.Textures != nil && copy.Textures.Files == 1 {
				storedTexture = true
			}
		}
	}
	if !storedTexture {
		t.Fatal("page snapshot lost stored texture metadata")
	}
	run := view.NewFileSearchRun()
	run.Search(context.Background(), e.App, view.FileSearchReq{Pattern: "Thing", ModsQuery: ""})
	u := run.Update(0)
	if u.Error != "" || !u.Done || u.Total != 3 || len(u.Deltas) < 2 {
		t.Fatalf("all copies %+v", u)
	}
	if len(run.Update(u.Next).Deltas) != 0 {
		t.Fatal("poll replayed history")
	}
	first := run.Update(1)
	if first.Since != 1 || len(first.Deltas) != len(u.Deltas)-1 {
		t.Fatal("since ignored")
	}
	run = view.NewFileSearchRun()
	run.Search(context.Background(), e.App, view.FileSearchReq{Pattern: "Thing", ModsQuery: "source:workshop"})
	u = run.Update(0)
	if u.Total != 1 || u.Mods != 1 {
		t.Fatalf("per-copy filter %+v", u)
	}
	for _, d := range u.Deltas {
		if !strings.Contains(strings.ToLower(d.Copy), "workshop") {
			t.Fatalf("wrong copy %s", d.Copy)
		}
	}
	// Opening from the stored inventory must still work when all synthetic folders are offline.
	if err := os.Rename(e.Workshop, e.Workshop+"-offline"); err != nil {
		t.Fatal(err)
	}
	defer os.Rename(e.Workshop+"-offline", e.Workshop)
	lib, err = e.App.FileSearchLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = view.BuildFileSearchInfo(lib, "", true); err != nil {
		t.Fatal(err)
	}
}

func TestFileSearchCapAndRead(t *testing.T) {
	e := testkit.New(t)
	dir := e.AddWorkshop("4000000001", testkit.Sample("sample.cap", "Cap"))
	path := filepath.Join(dir, "Defs", "Many.xml")
	e.WriteFile(path, strings.Repeat("<x>needle</x>\n", view.FileSearchLineCap+20))
	e.AddEnv(false)
	e.MustRun("scan")
	run := view.NewFileSearchRun()
	run.Search(context.Background(), e.App, view.FileSearchReq{Pattern: "needle"})
	u := run.Update(0)
	if u.Error != "" || !u.Capped || u.Matches != view.FileSearchLineCap {
		t.Fatalf("cap %+v", u)
	}
	ix, _ := e.App.Index()
	id := ""
	for _, m := range ix.Mods {
		if m.PackageID == "sample.cap" {
			id = m.ID
		}
	}
	text, err := view.ReadSearchFile(ix, e.App.SourceDir(), id, path, view.FileSearchReq{Pattern: "needle"})
	if err != nil || !text.Cut || len(text.Hits) != view.FileSearchLineCap {
		t.Fatalf("viewer cap %v %+v", err, text)
	}
	outside := filepath.Join(e.Root, "outside.xml")
	e.WriteFile(outside, "needle")
	if _, err = view.ReadSearchFile(ix, e.App.SourceDir(), id, outside, view.FileSearchReq{}); err == nil {
		t.Fatal("outside path accepted")
	}
	large := filepath.Join(dir, "About", "large.txt")
	e.WriteFile(large, strings.Repeat("x", view.FileSearchViewCap+1))
	text, err = view.ReadSearchFile(ix, e.App.SourceDir(), id, large, view.FileSearchReq{Pattern: "needle"})
	if err != nil || !text.Cut {
		t.Fatalf("byte cap %v", err)
	}
	binary := filepath.Join(dir, "Textures", "x.png")
	e.WriteFile(binary, "binary")
	text, err = view.ReadSearchFile(ix, e.App.SourceDir(), id, binary, view.FileSearchReq{})
	if err != nil || !text.Binary || len(text.Lines) != 0 {
		t.Fatal("binary text exposed", err)
	}
	e.WriteFile(filepath.Join(dir, "About", "oversized.xml"), "needle"+strings.Repeat("x", view.FileSearchReadCap))
	run = view.NewFileSearchRun()
	run.Search(context.Background(), e.App, view.FileSearchReq{Pattern: "needle", Files: "About"})
	u = run.Update(0)
	if u.Matches != 0 || !strings.Contains(strings.Join(u.Notes, " "), "2 MiB") {
		t.Fatalf("oversized input searched %+v", u)
	}
}

func TestFileSearchCoreAndStop(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	e.MustRun("scan")
	e.WriteFile(filepath.Join(e.Game, "RimWorldWin64_Data", "Managed", "Assembly-CSharp.dll"), "synthetic game assembly")
	started := make(chan struct{})
	release := make(chan struct{})
	e.App.Decompile = func(ctx context.Context, _, out, _ string, _ []string) error {
		close(started)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
		}
		return os.WriteFile(filepath.Join(out, "Game.cs"), []byte("class Needle {}"), 0600)
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := view.NewFileSearchRun()
	done := make(chan struct{})
	go func() {
		defer close(done)
		run.Search(ctx, e.App, view.FileSearchReq{Pattern: "Needle", Files: "C#", ModsQuery: "id:ludeon.rimworld"})
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("Core game decompile not reached")
	}
	cancel()
	<-done
	if u := run.Update(0); !u.Stopped || !u.Done {
		t.Fatalf("not stopped %+v", u)
	}
	e.App.Decompile = func(_ context.Context, _, out, _ string, _ []string) error {
		return os.WriteFile(filepath.Join(out, "Game.cs"), []byte("class Needle {}"), 0600)
	}
	run = view.NewFileSearchRun()
	run.Search(context.Background(), e.App, view.FileSearchReq{Pattern: "Needle", Files: "C#", ModsQuery: "id:ludeon.rimworld"})
	u := run.Update(0)
	if u.Error != "" || u.Matches != 1 || u.Deltas[0].Name != "Core" {
		t.Fatalf("Core source %+v", u)
	}
	if !strings.Contains(strings.Join(u.Notes, " "), "assemblies decompiled now: 1;") {
		t.Fatalf("decompile note %v", u.Notes)
	}
	run = view.NewFileSearchRun()
	run.Search(context.Background(), e.App, view.FileSearchReq{Pattern: "Needle", Files: "C#", ModsQuery: "id:ludeon.rimworld"})
	if u = run.Update(0); u.Matches != 1 || strings.Contains(strings.Join(u.Notes, " "), "assemblies decompiled now") {
		t.Fatalf("cache not reused %+v", u)
	}
}

func TestFileSearchStreamsBeforeDecompilingAndRetainsOrder(t *testing.T) {
	e := testkit.New(t)
	e.AddWorkshop("4000000001", testkit.Mod{PackageID: "sample.alpha", Name: "Alpha", Files: map[string]string{"Defs/a.xml": "<x>needle</x>", "Assemblies/A.dll": "synthetic assembly"}})
	e.AddWorkshop("4000000002", testkit.Mod{PackageID: "sample.beta", Name: "Beta", Files: map[string]string{"Defs/b.xml": "<x>needle</x>"}})
	e.AddEnv(false)
	e.MustRun("scan")
	started := make(chan struct{})
	release := make(chan struct{})
	e.App.Decompile = func(ctx context.Context, _, out, _ string, _ []string) error {
		close(started)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
		}
		return os.WriteFile(filepath.Join(out, "A.cs"), []byte("// needle"), 0600)
	}
	run := view.NewFileSearchRun()
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		defer close(done)
		run.Search(ctx, e.App, view.FileSearchReq{Pattern: "needle", Files: "Defs C#", ModsQuery: "-source:official"})
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("decompiler not reached")
	}
	first := run.Update(0)
	if first.Done || first.Matches != 2 || len(first.Deltas) != 2 || first.Deltas[0].Name != "Alpha" || first.Deltas[1].Name != "Beta" {
		t.Fatalf("text did not stream first: %+v", first)
	}
	close(release)
	<-done
	next := run.Update(first.Next)
	if !next.Done || next.Matches != 3 || len(next.Deltas) != 1 || next.Deltas[0].Name != "Alpha" {
		t.Fatalf("append-only continuation: %+v", next)
	}
	if run.Update(0).Deltas[1].Name != "Beta" {
		t.Fatal("earlier Mod reordered")
	}
}

func TestFileSearchCachedSourceAppearsBeforeMissingProject(t *testing.T) {
	e := testkit.New(t)
	e.AddWorkshop("4000000001", testkit.Mod{PackageID: "sample.missing", Name: "Alpha missing", Files: map[string]string{"Assemblies/A.dll": "missing code"}})
	e.AddWorkshop("4000000002", testkit.Mod{PackageID: "sample.cached", Name: "Zulu cached", Files: map[string]string{"Assemblies/Z.dll": "cached code"}})
	e.AddEnv(false)
	e.MustRun("scan")
	e.App.Decompile = func(_ context.Context, _, out, _ string, _ []string) error {
		return os.WriteFile(filepath.Join(out, "Z.cs"), []byte("// needle"), 0600)
	}
	st, _ := e.App.Loadout()
	for _, m := range st.Ix.Mods {
		if m.PackageID == "sample.cached" {
			ds, err := e.App.SourceDLLs(st, m)
			if err != nil {
				t.Fatal(err)
			}
			if err = e.App.DecompileDLL(context.Background(), st, ds[0]); err != nil {
				t.Fatal(err)
			}
		}
	}
	started := make(chan struct{})
	e.App.Decompile = func(ctx context.Context, _, _, _ string, _ []string) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := view.NewFileSearchRun()
	done := make(chan struct{})
	go func() {
		defer close(done)
		run.Search(ctx, e.App, view.FileSearchReq{Pattern: "needle", Files: "C#", ModsQuery: "-source:official"})
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("missing project not reached")
	}
	u := run.Update(0)
	if u.Done || u.Matches != 1 || u.Deltas[0].Name != "Zulu cached" {
		t.Fatalf("cached project delayed %+v", u)
	}
	cancel()
	<-done
	u = run.Update(u.Next)
	if !u.Stopped || u.Matches != 1 || len(u.Deltas) != 0 {
		t.Fatalf("stop lost cached hits %+v", u)
	}
}

func TestFileSearchLinkedEscape(t *testing.T) {
	e := testkit.New(t)
	dir := e.AddWorkshop("4000000001", testkit.Sample("sample.link", "Link"))
	e.AddEnv(false)
	e.MustRun("scan")
	outside := filepath.Join(e.Root, "outside.xml")
	e.WriteFile(outside, "needle")
	link := filepath.Join(dir, "Defs", "escape.xml")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("synthetic symlink unavailable: %v", err)
	}
	ix, _ := e.App.Index()
	id := ""
	for _, m := range ix.Mods {
		if m.PackageID == "sample.link" {
			id = m.ID
		}
	}
	if _, err := view.ReadSearchFile(ix, e.App.SourceDir(), id, link, view.FileSearchReq{Pattern: "needle"}); err == nil {
		t.Fatal("linked viewer escape accepted")
	}
	run := view.NewFileSearchRun()
	run.Search(context.Background(), e.App, view.FileSearchReq{Pattern: "needle"})
	if u := run.Update(0); u.Matches != 0 {
		t.Fatalf("linked search escape accepted %+v", u)
	}
}
