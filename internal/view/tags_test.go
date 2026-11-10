package view_test

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Tag candidates (16 §3.8): the selection's own tags first (all, then some), then pinned,
// then by use.
func TestTagCandidatesOwnFirst(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("6100", testkit.Sample("tester.a", "Alpha"))
	e.AddWorkshop("6200", testkit.Sample("tester.b", "Beta"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("tag", "remove", "pkg:ludeon.rimworld", "RimWorld") // the default tag (16 §3.8): not under test here
	e.MustRun("config", "set", "tags.pinned", "framework,visuals")
	e.MustRun("tag", "add", "pkg:tester.a", "popular", "races")
	e.MustRun("tag", "add", "pkg:tester.b", "popular", "weapons")

	got := func(sel ...[]string) []string {
		var out []string
		for _, c := range view.TagCandidates(e.App, "main", sel) {
			out = append(out, c.Tag)
		}
		return out
	}
	want := func(name string, have, exp []string) {
		t.Helper()
		if len(have) != len(exp) {
			t.Fatalf("%s: %v, want %v", name, have, exp)
		}
		for i := range exp {
			if have[i] != exp[i] {
				t.Fatalf("%s: %v, want %v", name, have, exp)
			}
		}
	}
	want("no tags", got([]string{}), []string{"framework", "visuals", "popular", "races", "weapons"})
	want("one mod", got([]string{"weapons", "visuals"}), []string{"visuals", "weapons", "framework", "popular", "races"})
	want("two mods", got([]string{"popular", "races"}, []string{"popular", "weapons"}), []string{"popular", "races", "weapons", "framework", "visuals"})
}
