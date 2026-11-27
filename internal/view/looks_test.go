package view_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func looksPNG(t *testing.T, shade uint8) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for i := 0; i < 4; i++ {
		img.SetNRGBA(i%2, i/2, color.NRGBA{shade, uint8(i), 7, 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

const (
	looksGun   = "things/item/gun"
	looksHat   = "things/item/hat"
	looksShirt = "things/apparel/shirt/shirt"
	looksRocks = "things/rocks"
	looksIcon  = "ui/icon"
)

// looksHome: Core, Alpha, Bravo, Charlie in that load order (positions 0 to 3).
//
//	gun (plain, a Def):  Alpha south, east, north and the mask of south; Bravo south, north; Charlie south
//	hat (plain, no Def): Alpha and Charlie, the same file
//	shirt (worn):        Alpha on the ground, male south and east, female south; Bravo male south
//	rocks (folder):      Alpha RockA, RockB; Bravo RockA
//	icon (no Def):       Alpha
func looksHome(t *testing.T) *testkit.Env {
	t.Helper()
	view.SetLang("en")
	e := testkit.New(t)
	e.AddEnv(false)
	mod := func(pkg, name string, shade uint8, files ...string) testkit.Mod {
		m := testkit.Mod{PackageID: pkg, Name: name, Files: map[string]string{}}
		for i, f := range files {
			m.Files["Textures/"+f] = looksPNG(t, shade+uint8(i))
		}
		return m
	}
	alpha := mod("looks.a", "Alpha", 10, "Things/Item/Gun_south.png", "Things/Item/Gun_east.png", "Things/Item/Gun_north.png", "Things/Item/Gun_southm.png",
		"Things/Apparel/Shirt/Shirt.png", "Things/Apparel/Shirt/Shirt_Male_south.png", "Things/Apparel/Shirt/Shirt_Male_east.png", "Things/Apparel/Shirt/Shirt_Female_south.png",
		"Things/Rocks/RockA.png", "Things/Rocks/RockB.png", "UI/Icon.png")
	alpha.Files["Textures/Things/Item/Hat.png"] = looksPNG(t, 200)
	alpha.Files["Defs/Things.xml"] = `<?xml version="1.0" encoding="utf-8"?><Defs>
<ThingDef><defName>Gun_Revolver</defName><label>revolver</label><graphicData><texPath>Things/Item/Gun</texPath><graphicClass>Graphic_Multi</graphicClass></graphicData></ThingDef>
<ThingDef><defName>Apparel_Shirt</defName><label>shirt</label><apparel><wornGraphicPath>Things/Apparel/Shirt/Shirt</wornGraphicPath></apparel></ThingDef>
<ThingDef><defName>Rocks</defName><label>rocks</label><graphicData><texPath>Things/Rocks</texPath><graphicClass>Graphic_Random</graphicClass></graphicData></ThingDef>
</Defs>`
	e.AddWorkshop("1001", alpha)
	e.AddWorkshop("1002", mod("looks.b", "Bravo", 60, "Things/Item/Gun_south.png", "Things/Item/Gun_north.png", "Things/Apparel/Shirt/Shirt_Male_south.png", "Things/Rocks/RockA.png"))
	charlie := mod("looks.c", "Charlie", 120, "Things/Item/Gun_south.png")
	charlie.Files["Textures/Things/Item/Hat.png"] = looksPNG(t, 200)
	e.WriteMod(filepath.Join(e.Game, "Mods", "C"), charlie)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "looks.a", "looks.b", "looks.c")
	e.MustRun("scan")
	return e
}

func looksState(t *testing.T, e *testkit.Env) *app.LooksState {
	t.Helper()
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	ls, err := e.App.LooksState(context.Background(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ls
}

func galleryIDs(g *view.LooksGallery) string {
	var out []string
	for _, grp := range g.Groups {
		for _, r := range grp.Rows {
			out = append(out, r.ID[strings.LastIndexByte(r.ID, '/')+1:])
		}
	}
	return strings.Join(out, " ")
}

func TestLooksOverviewAndGallery(t *testing.T) {
	e := looksHome(t)
	ls := looksState(t, e)
	o, err := view.BuildLooksOverview(e.App, ls)
	if err != nil {
		t.Fatal(err)
	}
	if o.Looks != 5 || o.Overridden != 3 || o.Mixed != 2 || o.NoDef != 2 || o.Chosen != 0 || o.Scope != "plan" || o.GameRunning || o.Confirmed || o.Stamp == "" {
		t.Fatalf("totals %+v", o)
	}
	var mods []string
	for _, m := range o.Mods {
		b, _ := json.Marshal([]any{m.Pos, m.No, m.Name, m.Kind, m.Looks, m.Overridden, m.Overriding, m.Generated})
		mods = append(mods, string(b))
		if m.Ref == "" || m.ID == "" {
			t.Fatalf("mod %+v", m)
		}
	}
	want := `[1,2,"Alpha","workshop",5,3,0,false] [2,3,"Bravo","workshop",3,1,3,false] [3,4,"Charlie","local",2,0,1,false]`
	if got := strings.Join(mods, " "); got != want {
		t.Fatalf("mods\n got %s\nwant %s", got, want)
	}
	if g := o.Generated; g.Name != "RimArcho Looks Preference" || g.Entry != 0 || g.Files != 0 || !g.DirChosen || g.Dir == "" {
		t.Fatalf("generated %+v", g)
	}

	gallery := func(mod, filter, search string) *view.LooksGallery {
		t.Helper()
		g, err := view.BuildLooksGallery(e.App, ls, mod, filter, search)
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	g := gallery("1", "", "")
	if got := galleryIDs(g); got != "shirt gun hat rocks icon" || g.Rows != 5 || g.Mod != 1 || g.Stamp != o.Stamp {
		t.Fatalf("Alpha's looks: %q %+v", got, g)
	}
	if len(g.Groups) != 4 || g.Groups[0].Category != "things/apparel/shirt" || g.Groups[1].Category != "things/item" || len(g.Groups[1].Rows) != 2 {
		t.Fatalf("groups %+v", g.Groups)
	}
	if c := g.Counts; c["all"] != 5 || c["overridden"] != 3 || c["overriding"] != 0 || c["mixed"] != 2 || c["chosen"] != 0 || c["nodef"] != 2 {
		t.Fatalf("counts %v", c)
	}
	gun := g.Groups[1].Rows[0]
	if gun.Title != "revolver" || gun.Kind != "plain" || gun.Providers != 3 || gun.InGame || gun.OverriddenBy != "Charlie" || !gun.Overridden || gun.Overriding || !gun.Mixed || gun.NoDef || gun.Choice != "" {
		t.Fatalf("gun row %+v", gun)
	}
	if b, _ := json.Marshal(gun.Thumbs); string(b) != `[{"p":3,"s":"south"},{"p":1,"s":"south","own":true}]` {
		t.Fatalf("gun thumbs %s", b)
	}
	// a worn look shows a worn texture, not the one on the ground
	if b, _ := json.Marshal(g.Groups[0].Rows[0].Thumbs); string(b) != `[{"p":2,"s":"male_south"},{"p":1,"s":"male_south","own":true}]` {
		t.Fatalf("shirt thumbs %s", b)
	}
	hat := g.Groups[1].Rows[1]
	if hat.Title != "hat" || !hat.InGame || hat.Overridden || !hat.NoDef || len(hat.Thumbs) != 1 {
		t.Fatalf("hat row (an identical copy is no override) %+v", hat)
	}
	rocks := g.Groups[2].Rows[0]
	if b, _ := json.Marshal(rocks.Thumbs); rocks.Kind != "folder" || rocks.Mixed || string(b) != `[{"p":2,"s":"rocka"},{"p":1,"s":"rockb"}]` {
		t.Fatalf("rocks row %+v %s", rocks, b)
	}
	for _, c := range []struct{ mod, filter, search, want string }{
		{"1", "overridden", "", "shirt gun rocks"},
		{"1", "mixed", "", "shirt gun"},
		{"1", "nodef", "", "hat icon"},
		{"1", "overriding", "", ""},
		{"2", "overriding", "", "shirt gun rocks"},
		{"3", "", "", "gun hat"},
		{"1", "", "GUN", "gun"},
		{"1", "", "revolver", "gun"},        // a Def label
		{"1", "", "apparel_shirt", "shirt"}, // a Def name
		{"1", "", "things item", "gun hat"},
		{"1", "", "bravo", ""},               // a Mod's name is not searched inside one Mod
		{"", "", "bravo", "shirt gun rocks"}, // … and is across the plan
		{"", "", "", "shirt gun hat rocks icon"},
		{"", "mixed", "charlie", "gun"},
		{"chosen", "", "", ""},
	} {
		if got := galleryIDs(gallery(c.mod, c.filter, c.search)); got != c.want {
			t.Errorf("gallery(%q, %q, %q) = %q, want %q", c.mod, c.filter, c.search, got, c.want)
		}
	}
	if _, err := view.BuildLooksGallery(e.App, ls, "../x", "", ""); err == nil {
		t.Fatal("a Mod that is no position was accepted")
	}
}

func TestLookDetailAxesAndPreview(t *testing.T) {
	e := looksHome(t)
	ls := looksState(t, e)
	detail := func(id string, mod int) *view.LookDetail {
		t.Helper()
		d, err := view.BuildLookDetail(e.App, ls, id, mod, "plan")
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	axes := func(d *view.LookDetail) string {
		b, _ := json.Marshal(d.Axes)
		return string(b)
	}
	targets := func(d *view.LookDetail) string {
		var out []string
		for _, tg := range d.Targets {
			s := tg.Kind
			if tg.Body+tg.Facing != "" {
				s += ":" + strings.Trim(tg.Body+"_"+tg.Facing, "_")
			}
			if tg.Kind == "slot" && tg.Facing == "" {
				s += ":" + tg.Slots[0]
			}
			if tg.Preview != nil {
				s += "=" + tg.Preview.Outcome
			}
			out = append(out, s)
		}
		return strings.Join(out, " ")
	}

	// plain: three facings, and west drawn as east mirrored
	d := detail(looksGun, 1)
	if d.Title != "revolver" || d.Kind != "plain" || d.Category != "things/item" || !d.Mixed || d.Selected != 1 || len(d.Providers) != 3 || d.CanUndo {
		t.Fatalf("gun %+v", d)
	}
	if want := `{"bodies":[{"body":"","facings":[{"facing":"south","slot":"south","contested":true},{"facing":"east","slot":"east"},{"facing":"north","slot":"north","contested":true},{"facing":"west","slot":"east","mirrored":true}],"contested":true}],"open":"south"}`; axes(d) != want {
		t.Fatalf("gun axes\n got %s\nwant %s", axes(d), want)
	}
	if def := d.Defs[0]; len(d.Defs) != 1 || def.Type != "ThingDef" || def.Name != "Gun_Revolver" || def.Label != "revolver" || !strings.Contains(def.Field, "texPath") || def.Class != "Graphic_Multi" || def.Mod != "Alpha" || def.ModPos != 1 {
		t.Fatalf("gun defs %+v", d.Defs)
	}
	south := d.Slots[0]
	if south.Slot != "south" || south.Facing != "south" || south.InGame != 3 || south.ByChoice || south.MaskInGame != 1 || len(south.Chain) != 3 || !south.Contested {
		t.Fatalf("south %+v", south)
	}
	if c := south.Chain[0]; c.Pos != 1 || c.Mask == nil || c.File.Rel != "Textures/Things/Item/Gun_south.png" || c.File.Format != "png" || c.File.W != 2 || c.File.H != 2 || c.File.Bytes == 0 || c.File.SameAs != -1 || !c.File.Preview {
		t.Fatalf("Alpha's south %+v", c)
	}
	if p := d.Providers; p[0].Name != "Alpha" || strings.Join(p[0].Slots, ",") != "south,east,north" || !p[0].InGame || p[1].Ref == "" || strings.Join(p[2].Slots, ",") != "south" {
		t.Fatalf("providers %+v", p)
	}
	// Alpha has every facing: the entire look is not mixed; one facing alone is
	if got := targets(d); got != "all=apply slot:south=mixed slot:east=inGame slot:north=mixed" {
		t.Fatalf("gun targets for Alpha: %s", got)
	}
	if p := d.Targets[1].Preview; p.Still != 1 || strings.Join(p.StillFrom, ",") != "Bravo" || p.Text != "mixed result: 1 textures still from Bravo" {
		t.Fatalf("south from Alpha: %+v", p)
	}
	// Bravo lacks east: the entire look stays mixed. Charlie's only texture is in the game already.
	if got := targets(detail(looksGun, 2)); got != "all=mixed slot:south=mixed slot:east=none slot:north=inGame" {
		t.Fatalf("gun targets for Bravo: %s", got)
	}
	if got := targets(detail(looksGun, 3)); got != "all=inGame slot:south=inGame slot:east=none slot:north=none" {
		t.Fatalf("gun targets for Charlie: %s", got)
	}
	if got := targets(detail(looksGun, -1)); got != "all slot:south slot:east slot:north" {
		t.Fatalf("gun targets without a Mod: %s", got)
	}

	// worn: on the ground, then the body types; it opens on the first worn texture
	d = detail(looksShirt, 2)
	if want := `{"ground":{"facing":"single","slot":"single"},"bodies":[{"body":"male","facings":[{"facing":"south","slot":"male_south","contested":true},{"facing":"east","slot":"male_east"},{"facing":"west","slot":"male_east","mirrored":true}],"contested":true},{"body":"female","facings":[{"facing":"south","slot":"female_south"}]}],"open":"male_south"}`; d.Kind != "worn" || axes(d) != want {
		t.Fatalf("shirt axes\n got %s\nwant %s", axes(d), want)
	}
	if got := targets(d); got != "all=inGame worn=inGame ground=none body:male=inGame body:female=none slot:male_south=inGame slot:male_east=none slot:female_south=none" {
		t.Fatalf("shirt targets for Bravo: %s", got)
	}
	if tg := d.Targets[1]; strings.Join(tg.Slots, ",") != "male_south,male_east,female_south" || strings.Join(d.Targets[3].Slots, ",") != "male_south,male_east" {
		t.Fatalf("shirt target slots %+v", d.Targets)
	}

	// folder: its files
	d = detail(looksRocks, 1)
	if want := `{"files":[{"slot":"rocka","contested":true},{"slot":"rockb"}],"open":"rocka"}`; d.Kind != "folder" || axes(d) != want || d.Mixed {
		t.Fatalf("rocks axes %s", axes(d))
	}
	if got := targets(d); got != "all=apply slot:rocka=apply slot:rockb=inGame" {
		t.Fatalf("rocks targets for Alpha: %s", got)
	}

	// the cheap prediction alone
	for _, c := range []struct {
		look  string
		mod   int
		slots []string
		want  string
	}{
		{looksHat, 2, nil, "none"},   // Bravo has no hat
		{looksHat, 3, nil, "inGame"}, // Charlie's is in the game
		{looksHat, 1, nil, "inGame"}, // Alpha's is the same file
		{looksGun, 1, nil, "apply"},  // every facing from Alpha
		{looksGun, 2, []string{"South"}, "mixed"},
		{looksGun, 2, []string{"east"}, "none"},
		{looksGun, 2, []string{"south_mask"}, "none"},
		{looksGun, 0, nil, "none"}, // Core supplies no texture
		{looksGun, 99, nil, "none"},
	} {
		p, err := view.BuildLookPreview(ls, c.look, c.mod, c.slots, "plan")
		if err != nil || p.Outcome != c.want {
			t.Errorf("preview(%s, %d, %v) = %+v %v, want %s", c.look, c.mod, c.slots, p, err, c.want)
		}
	}
	if _, err := view.BuildLookPreview(ls, "no/such", 1, nil, "plan"); err == nil {
		t.Fatal("a look that is not there was predicted")
	}
	if _, err := view.BuildLookDetail(e.App, ls, "no/such", 1, "plan"); err == nil {
		t.Fatal("a look that is not there was described")
	}
}

func looksPlan(t *testing.T, e *testkit.Env) string {
	t.Helper()
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, en := range st.List {
		if m := st.Ix.ByID[en.Instance]; m != nil {
			out = append(out, m.DisplayName())
		}
	}
	return strings.Join(out, ", ")
}

func TestLooksApplyManyIsOneUndoStep(t *testing.T) {
	e := looksHome(t)
	ctx := context.Background()
	o, err := view.BuildLooksOverview(e.App, looksState(t, e))
	if err != nil {
		t.Fatal(err)
	}
	alpha, bravo := o.Mods[0].Ref, o.Mods[1].Ref

	// the confirmation's dry run writes nothing and says what would be written
	dry, err := view.LooksApply(ctx, e.App, []string{looksGun, looksHat, looksRocks}, alpha, nil, "plan", true)
	if err != nil {
		t.Fatal(err)
	}
	if !dry.DryRun || dry.Done != 3 || !dry.Added || !dry.DirChosen || dry.Dir == "" || dry.Files != 4 || dry.Linked+dry.Copied != 4 || dry.Bytes == 0 || dry.Overview != nil || dry.Mod != "Alpha" {
		t.Fatalf("dry run %+v", dry)
	}
	if got := looksPlan(t, e); got != "Core, Alpha, Bravo, Charlie" || len(looksState(t, e).Looks) != 0 {
		t.Fatalf("the dry run wrote: %s", got)
	}

	// Bravo has no hat: that look is left out, the others are applied in one plan edit
	d, err := view.LooksApply(ctx, e.App, []string{looksGun, looksHat, looksShirt}, bravo, nil, "plan", false)
	if err != nil {
		t.Fatal(err)
	}
	if d.Done != 2 || len(d.Items) != 3 || d.Items[1].Error == "" || d.Items[1].Title != "hat" || !d.Items[0].Mixed || d.Mixed != 2 || !d.Added || d.Mod != "Bravo" || d.BuildError != "" {
		t.Fatalf("apply many %+v", d)
	}
	if want := "2 looks now come from Bravo  1 left out: it has no texture of them  2 looks have a mixed result"; d.Text != want {
		t.Fatalf("text %q", d.Text)
	}
	if got := looksPlan(t, e); got != "Core, Alpha, Bravo, Charlie, RimArcho Looks Preference" {
		t.Fatalf("plan after the apply: %s", got)
	}
	// gun south from Bravo is the one file to write: north is Bravo's by load order, and so is the shirt
	if ov := d.Overview; ov == nil || ov.Chosen != 2 || ov.Generated.Entry != 5 || ov.Generated.Applied != 1 || ov.Generated.NotNeeded != 1 || ov.Generated.Files != 1 || ov.Generated.DirChosen || d.Files != 1 {
		t.Fatalf("overview after the apply %+v, files %d", d.Overview, d.Files)
	}
	if last := d.Overview.Mods[len(d.Overview.Mods)-1]; !last.Generated || last.Looks != 2 || last.Pos != 4 {
		t.Fatalf("the generated Mod's row %+v", last)
	}
	ls := looksState(t, e)
	g, err := view.BuildLooksGallery(e.App, ls, "chosen", "", "")
	if err != nil || galleryIDs(g) != "shirt gun" || !g.Chosen || g.Groups[1].Rows[0].Choice != "applied" || g.Groups[1].Rows[0].From != "Bravo" || g.Groups[0].Rows[0].Choice != "not needed" {
		t.Fatalf("chosen %q %+v %v", galleryIDs(g), g, err)
	}
	if byRow, _ := view.BuildLooksGallery(e.App, ls, "4", "chosen", ""); galleryIDs(byRow) != "shirt gun" {
		t.Fatalf("the generated Mod's row shows %q", galleryIDs(byRow))
	}
	if none, err := view.BuildLooksGallery(e.App, ls, "styles", "", ""); err != nil || !none.Styles || none.Mod != -1 || none.Rows != 0 {
		t.Fatalf("styles without an entry %+v %v", none, err)
	}
	det, err := view.BuildLookDetail(e.App, ls, looksGun, 2, "plan")
	if err != nil || !det.CanUndo || len(det.Choices) != 1 || det.Choices[0].Scope != "plan" || det.Choices[0].Slot != "all" || det.Choices[0].Mod != "Bravo" || det.Choices[0].State != "applied" ||
		det.Slots[0].InGame != 2 || !det.Slots[0].ByChoice || det.Slots[0].State != "applied" {
		t.Fatalf("gun after the apply %+v %v", det, err)
	}
	// a choice for all plans would not show while this plan has its own
	if p, _ := view.BuildLookPreview(ls, looksGun, 1, nil, "all"); p.Outcome != "overruled" {
		t.Fatalf("for all plans, under the plan's choice: %+v", p)
	}
	if other, _ := view.BuildLookDetail(e.App, ls, looksGun, 2, "all"); other.CanUndo {
		t.Fatal("undo offered in a scope without a choice")
	}

	// one undo step takes the entry out again; the choices are not plan history
	if _, op, err := e.App.UndoRedo(false); err != nil || op != app.LooksAddOp {
		t.Fatalf("undo: %q %v", op, err)
	}
	if got := looksPlan(t, e); got != "Core, Alpha, Bravo, Charlie" {
		t.Fatalf("plan after one undo: %s", got)
	}
	if _, op, _ := e.App.UndoRedo(false); op == app.LooksAddOp {
		t.Fatal("the apply made two plan edits")
	}

	// undo of several looks: the one without a choice is reported
	u, err := view.LooksUndo(ctx, e.App, []string{looksGun, looksShirt, looksIcon}, "plan", false)
	if err != nil || u.Done != 2 || u.Items[2].Error == "" || u.Text != "the choices for 2 looks were removed" || u.Overview.Chosen != 0 {
		t.Fatalf("undo many %+v %v", u, err)
	}
	if _, err := view.LooksUndo(ctx, e.App, []string{looksGun}, "plan", false); err == nil {
		t.Fatal("undo without a choice succeeded")
	}
	if _, err := view.LooksApply(ctx, e.App, []string{looksHat, looksIcon}, bravo, nil, "plan", false); err == nil {
		t.Fatal("an apply none of whose looks the Mod has succeeded")
	}
	one, err := view.LooksApply(ctx, e.App, []string{looksGun}, bravo, []string{"south"}, "all", false)
	if err != nil || one.Scope != "all" || one.Text != "the look revolver now comes from Bravo  mixed result: some of its textures still come from another Mod" {
		t.Fatalf("apply one %+v %v", one, err)
	}
	r, err := view.LooksRebuild(ctx, e.App, false)
	if err != nil || r.Text != "RimArcho Looks Preference is up to date" {
		t.Fatalf("rebuild %+v %v", r, err)
	}
}

// 21 §5, §6: the add row of a look (what the selected Mod's version can be kept as, or
// why not), its entries, and the RimArcho Styles Preference card.
func TestLookAddRowAndStylesCard(t *testing.T) {
	e := looksHome(t)
	e.WriteMod(filepath.Join(e.Game, "Mods", "D"), testkit.Mod{PackageID: "looks.d", Name: "Delta", Files: map[string]string{
		"Defs/Styles.xml": `<Defs><StyleCategoryDef><defName>Rustic</defName></StyleCategoryDef>
<ThingDef><defName>Hat</defName><graphicData><texPath>Things/Item/Hat</texPath><graphicClass>Graphic_Single</graphicClass></graphicData></ThingDef></Defs>`,
	}})
	e.MustRun("scan")
	e.MustRun("enable", "pkg:looks.d")
	ctx := context.Background()
	detail := func(id string, mod int) *view.LookAdd {
		t.Helper()
		d, err := view.BuildLookDetail(e.App, looksState(t, e), id, mod, "plan")
		if err != nil || d.Add == nil {
			t.Fatalf("detail %s: %+v %v", id, d, err)
		}
		return d.Add
	}
	if a := detail(looksHat, -1); a.StylesNone == "" || a.VariantsNone != a.StylesNone || len(a.Styles) != 0 || a.RandomLabel == "" {
		t.Fatalf("no Mod selected: %+v", a)
	}
	a := detail(looksHat, 1)
	if len(a.Styles) != 1 || !a.Styles[0].OK || a.Styles[0].Target != "Rustic" || a.Styles[0].Label != "style in Rustic" || a.Styles[0].Note == "" || a.Styles[0].Shared || a.StylesNone != "" {
		t.Fatalf("styles: %+v", a)
	}
	// Alpha's and Charlie's hat are the same file: the game already shows it.
	if len(a.Variants) != 1 || a.Variants[0].OK || a.Variants[0].Code != app.StyleWhyShown || a.VariantsNone != "the game already shows that Mod's version" {
		t.Fatalf("variants: %+v", a)
	}
	if a := detail(looksIcon, 1); a.StylesNone != "no thing Def in the list uses this look as its graphic" || a.VariantsNone != a.StylesNone {
		t.Fatalf("a look no Def uses: %+v", a)
	}
	if a := detail(looksGun, 1); a.Variants[0].Code != app.StyleWhyFacings || !a.Styles[0].OK {
		t.Fatalf("the gun on 1.6: %+v", a)
	}
	if a := detail(looksShirt, 1); a.Variants[0].Code != app.StyleWhyKind || !a.Styles[0].OK {
		t.Fatalf("a worn look: %+v", a)
	}
	if a := detail(looksRocks, 1); a.Variants[0].Code != app.StyleWhyKind || !a.Styles[0].OK {
		t.Fatalf("a folder look: %+v", a)
	}

	dry, err := view.LookStyleAdd(ctx, e.App, looksHat, "pkg:looks.a", app.StyleKindStyle, "Rustic", true, true)
	if err != nil || !dry.DryRun || !dry.Added || dry.Files != 1 || dry.DefFiles != 2 || dry.Linked != 1 || !dry.DirChosen || dry.Overview != nil || dry.Entry.State != app.StyleActive {
		t.Fatalf("dry run: %+v %v", dry, err)
	}
	if rows, _ := e.App.Store.LookStyles(); len(rows) != 0 {
		t.Fatal("the dry run wrote")
	}
	done, err := view.LookStyleAdd(ctx, e.App, looksHat, "pkg:looks.a", app.StyleKindStyle, "Rustic", true, false)
	if err != nil || done.Text != "style in Rustic added for all lists" || !done.Added || done.Note == "" || done.Entry.ID == 0 || !done.Entry.Random || done.Entry.Mod != "Alpha" {
		t.Fatalf("add: %+v %v", done, err)
	}
	if styled, err := view.BuildLooksGallery(e.App, looksState(t, e), "styles", "", ""); err != nil || !styled.Styles || styled.Rows != 1 || styled.Groups[0].Rows[0].ID != looksHat {
		t.Fatalf("the looks with an entry: %+v %v", styled, err)
	}
	g := done.Overview.Styles
	if g.Name != "RimArcho Styles Preference" || g.Active != 1 || g.Skipped+g.Broken+g.Asleep != 0 || g.Files != 1 || g.Linked != 1 || g.Bytes == 0 || g.Dir == "" || g.DirChosen || g.Entry != 6 || g.Confirmed {
		t.Fatalf("the card: %+v", g)
	}
	for _, m := range done.Overview.Mods {
		if m.Name == app.StylesName {
			t.Fatal("RimArcho Styles Preference is listed as a Mod with looks")
		}
	}
	a = detail(looksHat, 1)
	if len(a.Entries) != 1 || a.Entries[0].State != app.StyleActive || a.Entries[0].ID != done.Entry.ID || a.Styles[0].OK || a.Styles[0].Code != app.StyleWhyExists || a.StylesNone != "" {
		t.Fatalf("after the add: %+v", a)
	}
	if b, _ := json.Marshal(a); !strings.Contains(string(b), `"styles":[{"kind":"style","target":"Rustic"`) || !strings.Contains(string(b), `"entries":[{"id":1,`) {
		t.Fatalf("JSON: %s", b)
	}
	if _, err := view.LookStyleAdd(ctx, e.App, looksHat, "pkg:looks.a", app.StyleKindVariant, "default", false, false); err == nil || err.Error() != "the game already shows that Mod's version" {
		t.Fatalf("a refused add: %v", err)
	}
	e.MustRun("disable", "pkg:looks.a")
	if a := detail(looksHat, -1); a.Entries[0].State != app.StyleAsleep || a.Entries[0].Why == "" || a.Entries[0].Mod != "Alpha" {
		t.Fatalf("asleep: %+v", a.Entries)
	}
	rm, err := view.LookStyleRemove(ctx, e.App, done.Entry.ID, false)
	if err != nil || !rm.Removed || rm.Text != "style in Rustic removed" || rm.Overview.Styles.Asleep != 0 || rm.Overview.Styles.Files != 0 {
		t.Fatalf("remove: %+v %v", rm, err)
	}
}
