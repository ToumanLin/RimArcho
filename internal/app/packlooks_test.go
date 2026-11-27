package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/looks"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/textures"
)

const packLooksDefs = `<Defs>
  <ThingDef>
    <defName>Cap</defName>
    <graphicData>
      <texPath>Things/Item/CapItem</texPath>
    </graphicData>
    <apparel>
      <wornGraphicPath>Things/Worn/Cap/Cap</wornGraphicPath>
    </apparel>
  </ThingDef>
  <ThingDef>
    <defName>Rock</defName>
    <graphicData>
      <texPath>Things/Rock</texPath>
      <graphicClass>Graphic_Random</graphicClass>
    </graphicData>
  </ThingDef>
</Defs>`

// packLooksEnv: Core and the Mods A, B (Workshop) and C (Mods/), in that load order, with
// a plain look with facings, a mask and a second format (gun), a plain look whose Mod B
// ships it in two load folders and a folder left out (hat), a worn look (cap) and a folder
// look (rock). B's and C's Gun_north are identical copies.
func packLooksEnv(t *testing.T) *testkit.Env {
	t.Helper()
	e := setup(t, true)
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	mod := func(pkg, name string, shade uint8, files ...string) testkit.Mod {
		m := testkit.Mod{PackageID: pkg, Name: name, Files: map[string]string{}}
		for i, f := range files {
			switch {
			case strings.HasSuffix(f, ".dds"):
				m.Files[f] = "DDS " + name + f
			case strings.HasSuffix(f, "Gun_north.png") && name != "A":
				m.Files[f] = lookPNG(t, 200) // B and C: identical copies
			default:
				m.Files[f] = lookPNG(t, shade+uint8(i))
			}
		}
		return m
	}
	a := mod("looks.a", "A", 10,
		"Textures/Things/Item/Gun_south.png", "Textures/Things/Item/Gun_south.dds", "Textures/Things/Item/Gun_north.png", "Textures/Things/Item/Gun_southm.png",
		"Textures/Things/Item/Hat.png", "Textures/Things/Item/Only.png",
		"Textures/Things/Worn/Cap/Cap_Male_south.png", "Textures/Things/Worn/Cap/Cap_Female_south.png",
		"Textures/Things/Rock/RockA.png", "Textures/Things/Rock/RockB.png")
	a.Files["Defs/Things.xml"] = packLooksDefs
	e.AddWorkshop("1001", a)
	b := mod("looks.b", "B", 60,
		"Textures/Things/Item/Gun_south.png", "Textures/Things/Item/Gun_south.dds", "Textures/Things/Item/Gun_north.png",
		"1.6/Textures/Things/Item/Hat.png", "Textures/Things/Item/Hat.png", "1.5/Textures/Things/Item/Hat.png",
		"Textures/Things/Worn/Cap/Cap_Male_south.png", "Textures/Things/Rock/RockA.png")
	b.Supported = []string{"1.5", "1.6"}
	e.AddWorkshop("1002", b)
	e.WriteMod(filepath.Join(e.Game, "Mods", "C"), mod("looks.c", "C", 120,
		"Textures/Things/Item/Gun_south.png", "Textures/Things/Item/Gun_north.png",
		"Textures/Things/Item/Hat.png", "Textures/Things/Item/Hat_m.png",
		"Textures/Things/Rock/RockA.png", "Textures/Things/Rock/RockC.png"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "looks.a", "looks.b", "looks.c")
	scanNow(t, e)
	if got := joined(active(t, e)); got != "Core, A, B, C" {
		t.Fatalf("plan: %s", got)
	}
	return e
}

// packLooksSources are the folders of the Mods A, B and C.
func packLooksSources(e *testkit.Env) []string {
	return []string{filepath.Join(e.Workshop, "1001"), filepath.Join(e.Workshop, "1002"), filepath.Join(e.Game, "Mods", "C")}
}

func readPackJSON(t *testing.T, e *testkit.Env, name string) *app.Pack {
	t.Helper()
	var p app.Pack
	b, err := os.ReadFile(filepath.Join(e.App.PacksDir(), name, "pack.json"))
	if err != nil || json.Unmarshal(b, &p) != nil {
		t.Fatalf("pack.json of %s: %v", name, err)
	}
	return &p
}

// packTextureFiles lists the texture files of the pack copy of the Mod pkg, relative to
// the copy, and its folders below a Textures folder ("dir/").
func packTextureFiles(t *testing.T, e *testkit.Env, p *app.Pack, pkg string) string {
	t.Helper()
	for _, pe := range p.Entries {
		if pe.PackageID != pkg {
			continue
		}
		root := filepath.Join(e.App.PacksDir(), p.Name, "mods", pe.Folder)
		var out []string
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			i := strings.Index(rel+"/", "Textures/")
			if i < 0 || rel == "Textures" || strings.HasSuffix(rel, "/Textures") {
				return nil
			}
			if d.IsDir() {
				out = append(out, rel+"/")
			} else {
				out = append(out, rel)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(out)
		return strings.Join(out, "\n")
	}
	t.Fatalf("no entry %s in the pack", pkg)
	return ""
}

func lines(s ...string) string { return strings.Join(s, "\n") }

// 10 §2 step 6, 21 §4 "In a pack": the copies keep only the file the game shows for each
// texture: by load order, and by the plan's choices (an earlier Mod then wins inside the
// pack). Masks, second formats, lower-priority load folders, identical copies; a dry run
// reports the same and writes nothing; the Mods are not touched; the digests agree; a new
// revision goes through the same step.
func TestPackLooks(t *testing.T) {
	t.Parallel()
	e := packLooksEnv(t)
	sources := packLooksSources(e)
	before := modsDigest(t, sources...)

	// load order alone: C wins gun, hat and RockA, B the cap's male body
	var steps []string
	r, err := e.App.BuildPackWith("order", app.BuildOptions{Progress: func(p app.BuildProgress) {
		if len(steps) == 0 || steps[len(steps)-1] != p.Step {
			steps = append(steps, p.Step)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(steps, ","); got != "check,measure,copy,looks,write,done" {
		t.Fatalf("steps %s", got)
	}
	if r.Pack.Looks == nil || r.Pack.Looks.Choices != 0 || r.Pack.Looks.Removed != 12 {
		t.Fatalf("load order: %+v", r.Pack.Looks)
	}
	p := readPackJSON(t, e, "order")
	if p.Looks == nil || *p.Looks != *r.Pack.Looks {
		t.Fatalf("pack.json looks: %+v, result %+v", p.Looks, r.Pack.Looks)
	}
	if got, want := packTextureFiles(t, e, p, "looks.a"), lines(
		"Textures/Things/",
		"Textures/Things/Item/",
		"Textures/Things/Item/Gun_southm.png", // the mask of the last Mod that has one
		"Textures/Things/Item/Only.png",
		"Textures/Things/Rock/",
		"Textures/Things/Rock/RockB.png",
		"Textures/Things/Worn/",
		"Textures/Things/Worn/Cap/",
		"Textures/Things/Worn/Cap/Cap_Female_south.png"); got != want {
		t.Fatalf("A by load order:\n%s", got)
	}
	if got, want := packTextureFiles(t, e, p, "looks.b"), lines(
		"Textures/Things/",
		"Textures/Things/Worn/",
		"Textures/Things/Worn/Cap/",
		"Textures/Things/Worn/Cap/Cap_Male_south.png"); got != want {
		t.Fatalf("B by load order (both load folders lose the hat, empty folders go):\n%s", got)
	}
	if got, want := packTextureFiles(t, e, p, "looks.c"), lines(
		"Textures/Things/",
		"Textures/Things/Item/",
		"Textures/Things/Item/Gun_north.png",
		"Textures/Things/Item/Gun_south.png",
		"Textures/Things/Item/Hat.png",
		"Textures/Things/Item/Hat_m.png",
		"Textures/Things/Rock/",
		"Textures/Things/Rock/RockA.png",
		"Textures/Things/Rock/RockC.png"); got != want {
		t.Fatalf("C by load order:\n%s", got)
	}
	if problems, err := e.App.VerifyPack("order"); err != nil || len(problems) != 0 {
		t.Fatalf("the digests must cover the copies after the removal: %+v %v", problems, err)
	}
	var sum int64
	for _, pe := range p.Entries {
		if pe.Folder != "" {
			if n := dirBytes(t, filepath.Join(e.App.PacksDir(), "order", "mods", pe.Folder)); n != pe.Bytes {
				t.Errorf("%s: bytes %d recorded, %d on disk", pe.Name, pe.Bytes, n)
			}
			sum += pe.Bytes
		}
	}
	if sum == 0 || modsDigest(t, sources...) != before {
		t.Fatal("the build changed a Mod's folder")
	}

	// choices: the gun from A for this plan, the hat from A for all plans
	applyLook(t, e, gunLook, "pkg:looks.a", app.LooksPlan)
	applyLook(t, e, hatLook, "pkg:looks.a", app.LooksAll)
	if got := joined(active(t, e)); got != "Core, A, B, C, "+app.LooksName {
		t.Fatalf("plan: %s", got)
	}
	before = modsDigest(t, sources...)
	dry, err := e.App.BuildPackWith("chosen", app.BuildOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if dry.Pack.Looks == nil || dry.Pack.Looks.Removed != 11 || dry.Pack.Looks.Choices != 2 || dry.Pack.Looks.Bytes == 0 {
		t.Fatalf("dry run: %+v", dry.Pack.Looks)
	}
	for _, n := range []string{"chosen", "chosen" + app.TmpSuffix} {
		if _, err := os.Lstat(filepath.Join(e.App.PacksDir(), n)); err == nil {
			t.Fatalf("the dry run made %s", n)
		}
	}
	if out := e.MustRun("pack", "build", "chosen", "--dry-run"); !strings.Contains(out, "would remove 11 textures") || !strings.Contains(out, "choice of 2 looks") {
		t.Fatalf("dry run output:\n%s", out)
	}
	out := e.MustRun("pack", "build", "chosen")
	if !strings.Contains(out, "removed 11 textures") || strings.Contains(out, "would remove") {
		t.Fatalf("build output:\n%s", out)
	}
	p = readPackJSON(t, e, "chosen")
	if p.Looks == nil || *p.Looks != *dry.Pack.Looks {
		t.Fatalf("the build removed %+v, the dry run said %+v", p.Looks, dry.Pack.Looks)
	}
	for i, pe := range p.Entries {
		if pe.PackageID == app.LooksPackage {
			t.Fatalf("%s is in the pack's list", app.LooksName)
		}
		if pe.Bytes != dry.Pack.Entries[i].Bytes {
			t.Errorf("%s: %d bytes, the dry run said %d", pe.Name, pe.Bytes, dry.Pack.Entries[i].Bytes)
		}
	}
	if ents, _ := os.ReadDir(filepath.Join(e.App.PacksDir(), "chosen", "mods")); len(ents) != 3 {
		t.Fatalf("the pack carries %d folders, want A, B and C", len(ents))
	}
	if got, want := packTextureFiles(t, e, p, "looks.a"), lines(
		"Textures/Things/",
		"Textures/Things/Item/",
		"Textures/Things/Item/Gun_north.png",
		"Textures/Things/Item/Gun_south.dds",
		"Textures/Things/Item/Gun_south.png",
		"Textures/Things/Item/Gun_southm.png",
		"Textures/Things/Item/Hat.png",
		"Textures/Things/Item/Only.png",
		"Textures/Things/Rock/",
		"Textures/Things/Rock/RockB.png",
		"Textures/Things/Worn/",
		"Textures/Things/Worn/Cap/",
		"Textures/Things/Worn/Cap/Cap_Female_south.png"); got != want {
		t.Fatalf("A chosen:\n%s", got)
	}
	if got, want := packTextureFiles(t, e, p, "looks.b"), lines(
		"Textures/Things/",
		"Textures/Things/Worn/",
		"Textures/Things/Worn/Cap/",
		"Textures/Things/Worn/Cap/Cap_Male_south.png"); got != want {
		t.Fatalf("B, a later Mod than the chosen one, keeps a file of the gun or the hat:\n%s", got)
	}
	if got, want := packTextureFiles(t, e, p, "looks.c"), lines(
		"Textures/Things/",
		"Textures/Things/Item/",
		"Textures/Things/Item/Hat_m.png", // A has no mask of the hat: the last Mod's stays
		"Textures/Things/Rock/",
		"Textures/Things/Rock/RockA.png",
		"Textures/Things/Rock/RockC.png"); got != want {
		t.Fatalf("C, a later Mod than the chosen one:\n%s", got)
	}
	// inside the pack the load order alone now gives what the plan's choices gave
	if got := packInGame(t, e, p, gunLook) + " " + packInGame(t, e, p, hatLook); got != "south=A north=A single=A" {
		t.Fatalf("in the pack: %s", got)
	}
	if problems, err := e.App.VerifyPack("chosen"); err != nil || len(problems) != 0 {
		t.Fatalf("verify: %+v %v", problems, err)
	}
	if out := e.MustRun("pack", "check", "chosen"); strings.Contains(out, "could not be verified") {
		t.Fatalf("pack check:\n%s", out)
	}
	if modsDigest(t, sources...) != before {
		t.Fatal("the build changed a Mod's folder")
	}

	// a new revision goes through the same step: without the gun's choice C wins it again
	if _, err := e.App.LooksUndo(context.Background(), gunLook, app.LooksPlan, false); err != nil {
		t.Fatal(err)
	}
	up, err := e.App.BuildPackWith("chosen", app.BuildOptions{Update: true})
	if err != nil {
		t.Fatal(err)
	}
	p = readPackJSON(t, e, "chosen")
	if p.Revision != 2 || p.Looks == nil || p.Looks.Choices != 1 || *p.Looks != *up.Pack.Looks {
		t.Fatalf("revision %d, looks %+v", p.Revision, p.Looks)
	}
	if got := packInGame(t, e, p, gunLook) + " " + packInGame(t, e, p, hatLook); got != "south=C north=C single=A" {
		t.Fatalf("in revision 2: %s", got)
	}
	if got := packTextureFiles(t, e, p, "looks.a"); strings.Contains(got, "Gun_south.") || strings.Contains(got, "Gun_north") || !strings.Contains(got, "Gun_southm.png") {
		t.Fatalf("A in revision 2:\n%s", got)
	}
	if problems, err := e.App.VerifyPack("chosen"); err != nil || len(problems) != 0 {
		t.Fatalf("verify revision 2: %+v %v", problems, err)
	}
	if modsDigest(t, sources...) != before {
		t.Fatal("the update changed a Mod's folder")
	}
}

func dirBytes(t *testing.T, dir string) int64 {
	t.Helper()
	var n int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && d.Name() == ".rimarcho" {
			return filepath.SkipDir // the origin marker is not part of the copy's size
		}
		if err != nil || d.IsDir() {
			return err
		}
		fi, err := d.Info()
		n += fi.Size()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// packInGame indexes the pack's copies as the game loads them and names the Mod whose
// file is the only one left for each texture slot of a look ("south=A north=A").
func packInGame(t *testing.T, e *testkit.Env, p *app.Pack, look string) string {
	t.Helper()
	var mods []looks.Mod
	names := map[int]string{}
	for i, pe := range p.Entries {
		if pe.Folder == "" {
			continue
		}
		root := filepath.Join(e.App.PacksDir(), p.Name, "mods", pe.Folder)
		folders := []string{root}
		if _, err := os.Stat(filepath.Join(root, "1.6")); err == nil {
			folders = []string{filepath.Join(root, "1.6"), root}
		}
		mods = append(mods, looks.Mod{Pos: i, ID: pe.Instance, Name: pe.Name, Root: root, Folders: folders})
		names[i] = pe.Name
	}
	idx, err := looks.Build(context.Background(), mods, looks.Options{})
	if err != nil {
		t.Fatal(err)
	}
	l := idx.ByID[look]
	if l == nil {
		t.Fatalf("no look %s in the pack", look)
	}
	var out []string
	for _, s := range l.SlotNames() {
		if looks.IsMask(s) {
			continue
		}
		n := 0
		for _, prov := range l.Providers {
			if prov.Slots[s] != nil {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("%s %s: %d Mods of the pack still ship it", look, s, n)
		}
		out = append(out, s+"="+names[l.OrderWinner(s).Pos])
	}
	return strings.Join(out, " ")
}

// 10 §2 step 6: when the index cannot be built, a plan with a choice for a Mod of the pack
// is refused (the pack would lose it); without one the step is skipped with a warning.
func TestPackLooksIndexFailure(t *testing.T) {
	t.Parallel()
	e := packLooksEnv(t)
	boom := errors.New("index unreadable")
	fail := func(string, func() (*looks.Index, error)) (*looks.Index, error) { return nil, boom }

	e.App.LooksCache = fail
	r, err := e.App.BuildPackWith("plain", app.BuildOptions{})
	if err != nil {
		t.Fatalf("no choice: the build goes on: %v", err)
	}
	if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "index unreadable") || r.Pack.Looks != nil {
		t.Fatalf("warnings %v, looks %+v", r.Warnings, r.Pack.Looks)
	}
	p := readPackJSON(t, e, "plain")
	if got := packTextureFiles(t, e, p, "looks.a"); !strings.Contains(got, "Gun_south.png") || p.Looks != nil {
		t.Fatalf("the step was skipped, every texture stays:\n%s", got)
	}
	if out := e.MustRun("pack", "build", "plain2", "--dry-run"); !strings.Contains(out, "warning: looks:") {
		t.Fatalf("dry run output:\n%s", out)
	}

	e.App.LooksCache = nil
	applyLook(t, e, gunLook, "pkg:looks.a", app.LooksPlan)
	e.App.LooksCache = fail
	for _, dry := range []bool{true, false} {
		_, err = e.App.BuildPackWith("chosen", app.BuildOptions{DryRun: dry})
		if err == nil || !strings.Contains(err.Error(), "index unreadable") || !strings.Contains(err.Error(), "looks choices") {
			t.Fatalf("dry %v: a plan with a choice must be refused: %v", dry, err)
		}
		for _, n := range []string{"chosen", "chosen" + app.TmpSuffix} {
			if _, err := os.Lstat(filepath.Join(e.App.PacksDir(), n)); err == nil {
				t.Fatalf("the refused build left %s", n)
			}
		}
	}
}

// 10 §2: RimArcho Looks Preference is never in a pack; RimArcho Styles Preference is
// copied like any Mod, its hard links as files of the pack's own, and launches like any
// pack copy. Texture conversion still works on what the looks step left.
func TestPackGeneratedMods(t *testing.T) {
	t.Parallel()
	e := stylesEnv(t, "")
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	enc := &texEnc{}
	e.App.TextureEncoder = enc
	addStyle(t, e, app.StyleAddInput{Look: hatLook, Mod: "pkg:looks.a", Kind: app.StyleKindStyle, Target: "Rustic"})
	applyLook(t, e, gunLook, "pkg:looks.a", app.LooksPlan)
	if got := joined(active(t, e)); got != "Core, A, B, C, S, "+app.StylesName+", "+app.LooksName {
		t.Fatalf("plan: %s", got)
	}
	styleFile := filepath.Join("Textures", "RimArchoStyles", "Style", "1", "Hat.png")
	src := filepath.Join(e.Workshop, "1001", "Textures", "Things", "Item", "Hat.png")
	if !sameFile(t, filepath.Join(stylesModDir(e), styleFile), src) {
		t.Fatal("fixture: the styles Mod's texture is no hard link")
	}
	r, err := e.App.BuildPackWith("gen", app.BuildOptions{Textures: &textures.Options{}})
	if err != nil {
		t.Fatal(err)
	}
	p := readPackJSON(t, e, "gen")
	var styles *app.PackEntry
	for i := range p.Entries {
		switch p.Entries[i].PackageID {
		case app.LooksPackage:
			t.Fatalf("%s is in the pack's list", app.LooksName)
		case app.StylesPackage:
			styles = &p.Entries[i]
		}
	}
	if styles == nil || styles.Folder == "" || styles.Files == 0 || styles.SHA256 == "" {
		t.Fatalf("%s is not carried: %+v", app.StylesName, styles)
	}
	mods := filepath.Join(e.App.PacksDir(), "gen", "mods")
	ents, _ := os.ReadDir(mods)
	for _, de := range ents {
		if b, _ := os.ReadFile(filepath.Join(mods, de.Name(), "About", "About.xml")); strings.Contains(string(b), app.LooksPackage) {
			t.Fatalf("the pack carries %s in %s", app.LooksName, de.Name())
		}
	}
	if len(ents) != 5 {
		t.Fatalf("%d folders in the pack, want A, B, C, S and the styles Mod", len(ents))
	}
	copyDir := filepath.Join(mods, styles.Folder)
	for _, rel := range []string{"About/About.xml", "Defs/RimArchoStyles.xml", "Patches/RimArchoStyles.xml"} {
		if _, err := os.Stat(filepath.Join(copyDir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("the styles copy lacks %s", rel)
		}
	}
	// the texture was converted in the pack: had the copy been a hard link, the Mod's PNG
	// would be gone or changed
	dds := filepath.Join(copyDir, strings.TrimSuffix(styleFile, ".png")+".dds")
	if _, err := os.Stat(dds); err != nil {
		t.Fatalf("the styles copy's texture was not converted: %v", err)
	}
	if _, err := os.Stat(src); err != nil || !sameFile(t, filepath.Join(stylesModDir(e), styleFile), src) {
		t.Fatalf("the Mod's file or the generated Mod's link changed: %v", err)
	}
	if r.Pack.Looks == nil || r.Pack.Looks.Choices != 1 || r.Pack.Looks.Removed == 0 || r.Pack.Textures == nil || r.Pack.Textures.Converted == 0 {
		t.Fatalf("looks %+v textures %+v", r.Pack.Looks, r.Pack.Textures)
	}
	// B lost the gun to the chosen A: nothing of it was converted either
	if got := packTextureFiles(t, e, p, "looks.b"); strings.Contains(got, "Gun_") {
		t.Fatalf("B keeps a gun texture:\n%s", got)
	}
	if problems, err := e.App.VerifyPack("gen"); err != nil || len(problems) != 0 {
		t.Fatalf("verify: %+v %v", problems, err)
	}
	e.MustRun("env", "set", "main", "--launch", "direct")
	if out := e.MustRun("pack", "launch", "gen", "--dry-run"); !strings.Contains(out, "+ !ArchoPack_gen_"+styles.Folder) {
		t.Fatalf("launch dry run:\n%s", out)
	}
	e.MustRun("pack", "launch", "gen")
	cfg, err := os.ReadFile(filepath.Join(e.App.PacksDir(), "gen", "userdata", "Config", "ModsConfig.xml"))
	if err != nil || !strings.Contains(string(cfg), "<li>"+app.StylesPackage+"</li>") || strings.Contains(string(cfg), app.LooksPackage) {
		t.Fatalf("the pack's list: %v\n%s", err, cfg)
	}
}
