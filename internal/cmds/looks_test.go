package cmds_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func lookTex(t *testing.T, shade uint8) string {
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

// looksFixture: Core, then A and B (Workshop), which both ship Things/Item/Gun (south and
// north) with different content; A's ThingDef uses it. B loads last and wins.
func looksFixture(t *testing.T) *testkit.Env {
	t.Helper()
	e := testkit.New(t)
	e.AddEnv(false)
	e.AddWorkshop("1001", testkit.Mod{PackageID: "looks.a", Name: "A", Files: map[string]string{
		"Textures/Things/Item/Gun_south.png": lookTex(t, 10),
		"Textures/Things/Item/Gun_north.png": lookTex(t, 11),
		"Textures/UI/Icon.png":               lookTex(t, 12),
		"Defs/Gun.xml":                       `<Defs><ThingDef><defName>A_Gun</defName><label>gun</label><graphicData><texPath>Things/Item/Gun</texPath><graphicClass>Graphic_Multi</graphicClass></graphicData></ThingDef></Defs>`,
	}})
	e.AddWorkshop("1002", testkit.Mod{PackageID: "looks.b", Name: "B", Files: map[string]string{
		"Textures/Things/Item/Gun_south.png": lookTex(t, 60),
		"Textures/Things/Item/Gun_north.png": lookTex(t, 61),
	}})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "looks.a", "looks.b")
	e.MustRun("scan")
	return e
}

func wantAll(t *testing.T, out string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(out, s) {
			t.Fatalf("missing %q in:\n%s", s, out)
		}
	}
}

// 13 §4.2 `looks`: the list with its filters, one look, and apply, undo and rebuild with
// their dry runs (21 §3.3, §4).
func TestLooksCommands(t *testing.T) {
	t.Parallel()
	e := looksFixture(t)
	out := e.MustRun("looks")
	wantAll(t, out, "looks of plan:", "2 looks, 1 overridden, 0 mixed", "things/item/gun  plain  2 Mods  overridden", "ui/icon", "no Def")
	if out := e.MustRun("looks", "--filter", "overridden"); strings.Contains(out, "ui/icon") || !strings.Contains(out, "things/item/gun") {
		t.Fatalf("--filter overridden:\n%s", out)
	}
	if out := e.MustRun("looks", "--filter", "overriding", "--mod", "pkg:looks.a"); strings.Contains(out, "things/item/gun") {
		t.Fatalf("A overrides nothing:\n%s", out)
	}
	wantAll(t, e.MustRun("looks", "--filter", "overriding", "--mod", "pkg:looks.b"), "things/item/gun")
	if out := e.MustRun("looks", "--filter", "nodef"); strings.Contains(out, "things/item/gun") || !strings.Contains(out, "ui/icon") {
		t.Fatalf("--filter nodef:\n%s", out)
	}
	wantAll(t, e.MustRun("looks", "--search", "a_gun"), "1 look,", "things/item/gun")
	if out := e.MustRun("looks", "--filter", "chosen"); !strings.Contains(out, "0 looks") {
		t.Fatalf("--filter chosen before any choice:\n%s", out)
	}
	if out, code := e.Run("looks", "--filter", "nope"); code != 2 {
		t.Fatalf("bad filter: %d\n%s", code, out)
	}

	show := e.MustRun("looks", "show", "Things/Item/Gun")
	wantAll(t, show, "look things/item/gun  plain  overridden", "ThingDef/A_Gun \"gun\"  graphicData/texPath", "south: in game ", " B (load order)", "Textures/Things/Item/Gun_south.png  png 2×2")

	// The dry run says what would be written and writes nothing.
	dir := filepath.Join(e.Root, "LocalAppData", "RimArcho", "looks")
	dry := e.MustRun("looks", "apply", "things/item/gun", "--from", "pkg:looks.a", "--dry-run")
	wantAll(t, dry, "would apply look things/item/gun from ", " A (plan:", "entire look", "would add RimArcho Looks Preference as the last entry", "folder: ", "(would be chosen: looks.dir)", "would write 2 files in ", "2 hard links, 0 copies", "Textures/Things/Item/Gun_south.png  hard link to ")
	if out := e.MustRun("looks", "--filter", "chosen"); !strings.Contains(out, "0 looks") {
		t.Fatalf("the dry run wrote a choice:\n%s", out)
	}
	if out := e.MustRun("list", "--active"); strings.Contains(out, "RimArcho Looks Preference") {
		t.Fatalf("the dry run changed the plan:\n%s", out)
	}

	applied := e.MustRun("looks", "apply", "things/item/gun", "--from", "pkg:looks.a")
	wantAll(t, applied, "applied look things/item/gun from ", "added RimArcho Looks Preference as the last entry", "(chosen now: looks.dir)", "wrote 2 files in ", "2 hard links, 0 copies")
	wantAll(t, e.MustRun("looks"), "choices: 1 applied, 0 not needed, 0 asleep, 0 broken", "RimArcho Looks Preference: position 4, the plan's last entry", "chosen: applied")
	wantAll(t, e.MustRun("looks", "show", "things/item/gun"), "south: in game ", " A (by choice)", "choices:", "entire look from ", "applied")
	wantAll(t, e.MustRun("looks", "rebuild"), "unchanged: nothing written")
	wantAll(t, e.MustRun("ops", "--op", "looks"), "looks.choice", "looks.build", "apply from A")
	wantAll(t, e.MustRun("config", "get", "looks.dir"), dir)

	// A slot for all plans; the plan's choice still wins.
	wantAll(t, e.MustRun("looks", "apply", "things/item/gun", "--from", "pkg:looks.b", "--slot", "south", "--scope", "all"), "(all plans)", "slot south")
	wantAll(t, e.MustRun("looks", "show", "things/item/gun"), "all plans  slot south from ", "overruled by a choice that comes first")
	if out, code := e.Run("looks", "apply", "things/item/gun", "--from", "pkg:looks.b", "--slot", "west"); code == 0 || !strings.Contains(out, "has no slot west") {
		t.Fatalf("a slot the Mod does not have: %d\n%s", code, out)
	}
	if out, code := e.Run("looks", "apply", "things/item/gun"); code != 2 {
		t.Fatalf("apply without --from: %d\n%s", code, out)
	}

	wantAll(t, e.MustRun("looks", "undo", "things/item/gun", "--dry-run"), "would remove 1 choice of look things/item/gun (plan:")
	wantAll(t, e.MustRun("looks", "show", "things/item/gun"), " A (by choice)")
	wantAll(t, e.MustRun("looks", "undo", "things/item/gun"), "removed 1 choice of look things/item/gun (plan:")
	// The choice for all plans now decides south: B, which the load order gives anyway.
	wantAll(t, e.MustRun("looks", "show", "things/item/gun"), "all plans  slot south from ", "not needed: the load order already shows it")
	if out, code := e.Run("looks", "undo", "things/item/gun"); code == 0 {
		t.Fatalf("nothing to undo in the plan's scope:\n%s", out)
	}
	wantAll(t, e.MustRun("looks", "undo", "things/item/gun", "--scope", "all"), "removed 1 choice", "(all plans)")
	wantAll(t, e.MustRun("looks", "--filter", "chosen"), "0 looks")
}
