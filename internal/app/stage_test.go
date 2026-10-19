package app_test

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// stageEnv: Core plus 30 enabled local mods M01…M30 and 3 more that are not enabled.
func stageEnv(t *testing.T) *testkit.Env {
	e := testkit.New(t)
	active := []string{"ludeon.rimworld"}
	for i := 1; i <= 33; i++ {
		pkg := fmt.Sprintf("tester.m%02d", i)
		e.WriteMod(filepath.Join(e.Game, "Mods", fmt.Sprintf("M%02d", i)), testkit.Sample(pkg, fmt.Sprintf("M%02d", i)))
		if i <= 30 {
			active = append(active, pkg)
		}
	}
	e.WriteModsConfig("1.6.4633 rev1261", active...)
	e.AddEnv(false)
	e.MustRun("scan")
	return e
}

func names(t *testing.T, e *testkit.Env) string {
	t.Helper()
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, en := range st.List {
		out = append(out, st.Ix.ByID[en.Instance].Name)
	}
	return strings.Join(out, " ")
}

func stagedNames(t *testing.T, e *testkit.Env) string {
	t.Helper()
	items, st, err := e.App.Staged()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, it := range items {
		out = append(out, st.Ix.ByID[it.Instance].Name)
	}
	return strings.Join(out, " ")
}

// 06 §11 T9: 20 mods into staging, then all back: every mod returns to its position.
func TestStageRestoreAllKeepsPositions(t *testing.T) {
	e := stageEnv(t)
	before := names(t, e)
	// 20 mods, in an order unrelated to their positions, with runs of neighbours.
	pick := []int{30, 2, 3, 4, 17, 9, 10, 11, 25, 26, 1, 5, 14, 22, 23, 24, 7, 28, 19, 12}
	var refs []string
	for _, i := range pick {
		refs = append(refs, ref(t, e, fmt.Sprintf("M%02d", i)))
	}
	e.MustRun(append([]string{"stage", "add"}, refs[:7]...)...) // in two batches
	e.MustRun(append([]string{"stage", "add"}, refs[7:]...)...)
	if got := len(strings.Fields(names(t, e))); got != 31-20 {
		t.Fatalf("after staging %d enabled: %s", got, names(t, e))
	}
	if got := len(strings.Fields(stagedNames(t, e))); got != 20 {
		t.Fatalf("staged %d", got)
	}
	out := e.MustRun("stage", "list")
	if !strings.Contains(out, "staging: 20 mods") || !strings.Contains(out, "was 31") {
		t.Fatalf("stage list:\n%s", out)
	}
	e.MustRun("stage", "restore")
	if got := names(t, e); got != before {
		t.Fatalf("restore all:\n got %s\nwant %s", got, before)
	}
	if s := stagedNames(t, e); s != "" {
		t.Fatalf("still staged: %s", s)
	}
}

// One mod back at a time follows its anchor, also when the list changed meanwhile.
func TestStageRestoreOneFollowsAnchor(t *testing.T) {
	e := stageEnv(t)
	e.MustRun("stage", "add", ref(t, e, "M10"), ref(t, e, "M11")) // M11's anchor is M10, also staged
	e.MustRun("move", ref(t, e, "M09"), "--to", "2")              // M10's anchor moves to the top
	e.MustRun("stage", "restore", ref(t, e, "M11"))               // M10 still staged: M11 follows M09
	got := strings.Fields(names(t, e))
	if got[1] != "M09" || got[2] != "M11" {
		t.Fatalf("M11 after its anchor chain: %v", got[:5])
	}
	e.MustRun("stage", "restore", ref(t, e, "M10"))
	got = strings.Fields(names(t, e))
	if got[2] != "M10" || got[3] != "M11" {
		t.Fatalf("M10 after M09: %v", got[:5])
	}
	// A mod staged from outside the Loadout goes to the end; drop leaves it disabled.
	e.MustRun("stage", "add", ref(t, e, "M31"), ref(t, e, "M32"))
	e.MustRun("stage", "drop", ref(t, e, "M32"))
	e.MustRun("stage", "restore")
	got = strings.Fields(names(t, e))
	if got[len(got)-1] != "M31" || strings.Contains(names(t, e), "M32") || stagedNames(t, e) != "" {
		t.Fatalf("prep-area mods: %v / staged %q", got[len(got)-3:], stagedNames(t, e))
	}
	// An explicit position wins; undo re-enables and so leaves staging.
	e.MustRun("stage", "add", ref(t, e, "M05"))
	e.MustRun("stage", "restore", ref(t, e, "M05"), "--at", "2")
	if got = strings.Fields(names(t, e)); got[1] != "M05" {
		t.Fatalf("--at 2: %v", got[:3])
	}
	e.MustRun("stage", "add", ref(t, e, "M06"))
	e.MustRun("undo")
	if s := stagedNames(t, e); s != "" || !strings.Contains(names(t, e), "M06") {
		t.Fatalf("after undo: staged %q", s)
	}
	if _, code := e.Run("stage", "restore", ref(t, e, "M07")); code == 0 {
		t.Fatal("restoring a mod that is not staged succeeded")
	}
	var _ app.StageItem
}

// Random batches in random order, then restore all: the list is exactly as before.
func TestStageRestoreAllRandomBatches(t *testing.T) {
	e := stageEnv(t)
	before := names(t, e)
	for seed := int64(1); seed <= 12; seed++ {
		r := rand.New(rand.NewSource(seed))
		perm := r.Perm(30)
		n := 5 + r.Intn(21) // 5…25 mods
		for k := 0; k < n; {
			b := 1 + r.Intn(6)
			var refs []string
			for ; b > 0 && k < n; b, k = b-1, k+1 {
				refs = append(refs, ref(t, e, fmt.Sprintf("M%02d", perm[k]+1)))
			}
			e.MustRun(append([]string{"stage", "add"}, refs...)...)
		}
		e.MustRun("stage", "restore")
		if got := names(t, e); got != before {
			t.Fatalf("seed %d (%d mods):\n got %s\nwant %s", seed, n, got, before)
		}
	}
}
