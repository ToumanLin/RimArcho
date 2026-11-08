package app_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// listLog: a missing texture first (E1), then an exception named by the [Mod C] prefix whose
// top mod frame is Mod A's and whose second mod frame is Mod B's (06 §3). The run loaded a
// save.
const listLog = "Loading game from file Colony1 with mods:\n- Core\n\n" +
	"Could not load Texture2D at 'Things/Gone' in any active mod or in base resources.\n" +
	"[Mod C] Exception ticking Pawn7: System.ArgumentOutOfRangeException: Index was out of range.\n" +
	"  at ModA.CompBroken.CompTick () [0x00000] in <00000000000000000000000000000000>:0\n" +
	"  at ModB.Pawn_Tick_B.Helper () [0x00000] in <00000000000000000000000000000000>:0\n" +
	"  at Verse.ThingWithComps.Tick () [0x00000] in <00000000000000000000000000000000>:0\n\n"

func listEnv(t *testing.T) *testkit.Env {
	t.Helper()
	e := testkit.New(t)
	e.HarmonyLogFixture()
	e.MustRun("analyze")
	writePlayerLog(e, listLog)
	return e
}

// The cluster list puts an exception before a missing texture although the texture came
// first; every missing texture says it is rendering only (MCP eval h4 B3, h8 B4).
func TestLogListOrderAndTextureNote(t *testing.T) {
	t.Parallel()
	e := listEnv(t)
	out := e.MustRun("log")
	exc, tex := strings.Index(out, "log:E2 exception"), strings.Index(out, "log:E1 texture-missing")
	if exc < 0 || tex < 0 || exc > tex {
		t.Errorf("exception must be listed before the missing texture:\n%s", out)
	}
	if !strings.Contains(out, "next:\n  archo log --cluster E2") {
		t.Errorf("next: points at the first listed cluster:\n%s", out)
	}
	if tx := e.MustRun("log", "--cluster", "E1"); !strings.Contains(tx, "detail: rendering only: the game draws a placeholder") {
		t.Errorf("texture cluster:\n%s", tx)
	}
	rep, err := e.App.AnalyzeLogText(listLog)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, lc := range app.ListOrder(rep.Clusters) {
		ids = append(ids, lc.ID)
	}
	if !slices.Equal(ids, []string{"E2", "E1"}) {
		t.Errorf("ListOrder: %v", ids)
	}
	if rep.Clusters[0].ID != "E1" {
		t.Errorf("cluster IDs keep the order of first appearance: %s", rep.Clusters[0].ID)
	}
}

// A mod only named by a lower frame (mentioned) does not get the cluster as its own problem:
// not in Mod details, not among a test run's mods (06 §3, h8 B4). Two sources: the report
// says to name both fix places.
func TestLogMentionedModIsNoSubjectAndTwoSourcesFix(t *testing.T) {
	t.Parallel()
	e := listEnv(t)
	e.MustRun("log")
	c := e.MustRun("log", "--cluster", "E2")
	for _, want := range []string{
		"#gpto Mod B  mentioned",
		"fixes: 2 sources, #xamx Mod C and #iqmu Mod A: one may have thrown and the other made it throw",
		"Report both and name both fix places: a guard or a different loop in the one that threw hides the error; undoing the action of the one that caused it removes the error",
	} {
		if !strings.Contains(c, want) {
			t.Errorf("cluster misses %q:\n%s", want, c)
		}
	}
	for ref, shown := range map[string]bool{"iqmu": true, "xamx": true, "gpto": false} {
		if got := strings.Contains(e.MustRun("mod", ref), "log.exception"); got != shown {
			t.Errorf("mod %s lists the log exception: %v, want %v", ref, got, shown)
		}
	}
	// A cluster with no or one source has no such line (the pattern's own fixes: is deeper).
	if one := e.MustRun("log", "--cluster", "E1"); strings.Contains(one, "\n  fixes: ") {
		t.Errorf("one source:\n%s", one)
	}
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	r := testRunWith(t, e, "menu", listLog)
	var mods []string
	for _, cl := range r.Clusters {
		if cl.Class == "exception" {
			mods = cl.Mods
		}
	}
	if !slices.Equal(mods, []string{"Mod C", "Mod A"}) {
		t.Errorf("test run mods of the exception: %v (a mentioned mod is not listed)", mods)
	}
}

// A missing texture in a test run says it is rendering only, in the remaining list and in the
// fixed list (06 §3, 20 §2).
func TestTextureRenderingOnlyInTestRun(t *testing.T) {
	t.Parallel()
	e := listEnv(t)
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	r := testRunWith(t, e, "menu", listLog)
	found := false
	for _, cl := range r.Clusters {
		if cl.Class == "texture-missing" {
			found = true
			if !strings.HasPrefix(cl.Note, "rendering only: ") {
				t.Errorf("remaining: %+v", cl)
			}
		}
	}
	if !found {
		t.Fatalf("no texture cluster: %+v", r.Clusters)
	}
	r = testRunWith(t, e, "menu", "")
	found = false
	for _, cl := range r.Fixed {
		if cl.Class == "texture-missing" {
			found = true
			if !strings.HasPrefix(cl.Note, "rendering only: ") {
				t.Errorf("fixed: %+v", cl)
			}
		}
	}
	if !found {
		t.Fatalf("no fixed texture cluster: %+v", r.Fixed)
	}
}

// A log that loaded a save which is not in Saves/ says so; the line is gone once the save is
// there (06 §3). Pasted text has no Saves/ to compare with.
func TestLogSaveNotInSaves(t *testing.T) {
	t.Parallel()
	e := listEnv(t)
	const line = `saves: this run loaded save "Colony1", which is not in Saves/; a test run cannot reproduce errors that need that save`
	if out := e.MustRun("log"); !strings.Contains(out, line) {
		t.Errorf("missing %q:\n%s", line, out)
	}
	e.WriteFile(filepath.Join(e.UserData, "Saves", "Colony1.rws"), "<savegame/>")
	if out := e.MustRun("log"); strings.Contains(out, "saves:") {
		t.Errorf("the save is there:\n%s", out)
	}
	if out := e.MustRun("log", "--text", listLog); strings.Contains(out, "saves:") {
		t.Errorf("pasted text:\n%s", out)
	}
}

// Mod details say whether a copy's files are the Workshop copy's, from the stored scan (04 §1.4).
func TestModDetailsProvenance(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	s := testkit.Sample("tester.prov", "Prov")
	e.AddWorkshop("1001", s)
	dir := filepath.Join(e.Game, "Mods", "ProvLocal")
	e.WriteMod(dir, s)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.prov")
	e.AddEnv(false)
	e.MustRun("scan")
	local, ws := "nyh3", "6olp"
	if out := e.MustRun("find", "Prov"); !strings.Contains(out, "#"+local+"  Prov  [active") || !strings.Contains(out, "#"+ws+"  Prov  [workshop") {
		t.Fatalf("refs changed:\n%s", out)
	}
	if out := e.MustRun("mod", local); !strings.Contains(out, "provenance: files identical to Workshop 1001 (#6olp), as of the last scan") {
		t.Errorf("identical:\n%s", out)
	}
	if out := e.MustRun("mod", ws); strings.Contains(out, "provenance:") {
		t.Errorf("the Workshop copy has no counterpart to compare with:\n%s", out)
	}
	s.Files["Defs/Extra.xml"] = "<Defs/>\n"
	s.Files["Defs/ThingDefs/prov.xml"] = "<Defs><ThingDef><defName>prov_Thing2</defName></ThingDef></Defs>\n"
	e.WriteMod(dir, s)
	e.MustRun("scan")
	if out := e.MustRun("mod", local); !strings.Contains(out, "provenance: 2 files differ from Workshop 1001 (#6olp), as of the last scan: Defs/Extra.xml added, Defs/ThingDefs/prov.xml changed") {
		t.Errorf("differs:\n%s", out)
	}
	// A mod with no Workshop copy has no line.
	e2 := testkit.New(t)
	e2.WriteMod(filepath.Join(e2.Game, "Mods", "Solo"), testkit.Sample("tester.solo", "Solo"))
	e2.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.solo")
	e2.AddEnv(false)
	e2.MustRun("scan")
	if out := e2.MustRun("mod", "pkg:tester.solo"); strings.Contains(out, "provenance:") {
		t.Errorf("solo mod:\n%s", out)
	}
}
