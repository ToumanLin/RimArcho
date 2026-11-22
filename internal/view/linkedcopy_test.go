package view_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The player's case ("cannot delete a Mod"): a Workshop Mod also sits in the game's Mods/
// as RimCrow's `_Link_<id>` junction into RimCrow's own folder, outside every source. The
// scan reads it as a local copy that links out (04 §5 A11); RimArcho never deletes another
// tool's link (09 §3), so the menu says so and opens where the link is instead of offering
// no way out (16 §2.9). A real folder the player put in Mods/ still goes to the recycle bin,
// and the operation log records it (13 §1.3).
func TestLinkedLocalCopyOps(t *testing.T) {
	e := testkit.New(t)
	e.AddWorkshop("2880970348", testkit.Sample("cixwow.pump", "Pump"))
	outside := filepath.Join(e.Root, "RimCrow", "mods", "2880970348")
	old := testkit.Sample("cixwow.pump", "Pump")
	old.Files = map[string]string{"Defs/Old.xml": "<Defs/>"}
	e.WriteMod(outside, old)
	e.Junction(filepath.Join(e.Game, "Mods", "_Link_2880970348"), outside)
	e.WriteMod(filepath.Join(e.Game, "Mods", "Own Copy"), testkit.Sample("tester.own", "Own"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.own")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("pack", "build", "Multiplayer Pack")
	lib, err := e.App.Library()
	if err != nil {
		t.Fatal(err)
	}
	refs := map[string]string{}
	for _, l := range lib.Lineages {
		for _, i := range l.Instances {
			if i.Mod == nil || i.Source != "local" {
				continue
			}
			ops := view.CopyOps(lib, l, i, view.LibraryPage)
			refs[i.Mod.Rel] = lib.St.Ix.Ref(i.Mod.ID)
			switch i.Mod.Rel {
			case "_Link_2880970348":
				if slices.Contains(ops, view.OpDelete) || !slices.Contains(ops, view.OpLinked) || !slices.Contains(ops, view.OpFolder) {
					t.Errorf("linked copy: %v", ops)
				}
				if msg := view.LinkedText(lib.St.Ix, i.Mod); !strings.Contains(msg, "_Link_2880970348") || !strings.Contains(msg, "RimCrow") {
					t.Errorf("linked text: %q", msg)
				}
			case "Own Copy":
				if !slices.Contains(ops, view.OpDelete) || slices.Contains(ops, view.OpLinked) {
					t.Errorf("own copy: %v", ops)
				}
			}
		}
	}
	if len(refs) != 2 {
		t.Fatalf("local copies: %v", refs)
	}
	// archo trash refuses the link by its own name and says what to do instead.
	out, code := e.Run("trash", refs["_Link_2880970348"], "--dry-run")
	if code == 0 || !strings.Contains(out, "_Link_2880970348") || !strings.Contains(out, "RimCrow") {
		t.Fatalf("trash a link: exit %d\n%s", code, out)
	}
	// the confirm names the list and the pack that use the real folder, and it still goes
	pv, err := view.LibPreview(e.App, view.LibDelete, []string{refs["Own Copy"]})
	if err != nil || !pv.Apply {
		t.Fatalf("preview %+v %v", pv, err)
	}
	text := strings.Join(pv.Lines, "\n")
	if !strings.Contains(text, "is in list") || !strings.Contains(text, "“Multiplayer Pack”") {
		t.Fatalf("preview lines:\n%s", text)
	}
	var gone []string
	e.App.TrashFunc = func(p string) error { gone = append(gone, p); return os.RemoveAll(p) }
	e.MustRun("trash", refs["Own Copy"])
	if len(gone) != 1 || !strings.HasSuffix(gone[0], "Own Copy") {
		t.Fatalf("recycled %v", gone)
	}
	if log := e.MustRun("ops", "--op", "trash"); !strings.Contains(log, "Own Copy") {
		t.Fatalf("operation log:\n%s", log)
	}
}
