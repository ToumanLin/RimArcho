package gui_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/surface/gui"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Both command execution and the old GUI inspect path must keep --subject.
// Unrelated mods must not enter the heading or the log rows.
func TestLastRunSubjectAndNavigation(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	for _, m := range []struct{ pkg, name, def string }{{"tester.alpha", "Alpha Props", "AlphaDef"}, {"tester.beta", "Beta", "BetaDef"}} {
		e.AddWorkshop(m.pkg, testkit.Mod{PackageID: m.pkg, Name: m.name, Files: map[string]string{"Defs/Test.xml": "<Defs><ThingDef><defName>" + m.def + "</defName></ThingDef></Defs>"}})
	}
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "tester.alpha", "tester.beta")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("analyze")
	e.RecordLaunch(e.Now().Add(-time.Minute))
	log := "RimWorld 1.6.4633 rev1\n\n" + strings.Repeat("Could not load Texture2D at 'Things/Alpha' for def 'AlphaDef'\n\n", 8) + "Could not load Texture2D at 'Things/Beta' for def 'BetaDef'\n\n"
	path := filepath.Join(e.UserData, "Player.log")
	e.WriteFile(path, log)
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	alpha, err := st.Ix.ResolveOne("pkg:tester.alpha")
	if err != nil {
		t.Fatal(err)
	}
	api := gui.NewAPI(e.App, nil)
	defer api.Close()
	for _, subject := range []string{"pkg:tester.alpha", st.Ix.Ref(alpha.ID)} {
		argv := []string{"problems", "--code", "log", "--subject", subject}
		out, code := cmds.Execute(e.App, argv)
		if code != 0 {
			t.Fatal(out)
		}
		inspected, err := api.Inspect(argv)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range []string{out, strings.Join(inspected, "\n")} {
			if !strings.Contains(s, "problems: 0 error, 1 warning") || !strings.Contains(s, "x8:") || strings.Contains(s, "Things/Beta") {
				t.Fatalf("subject %s: %s", subject, s)
			}
		}
	}
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	ns := view.BuildNeeds(e.App, ps.State, ps)
	found := false
	for _, n := range ns.Items {
		if n.Group != view.GroupLastRun || !strings.Contains(n.Title, "Alpha Props") {
			continue
		}
		found = true
		a := n.Actions[0]
		if a.Kind != view.ActGameLog || a.LogPath != path || a.Cluster == "" {
			t.Fatalf("action %+v", a)
		}
		if len(a.Argv) != 5 || a.Argv[3] != "--subject" {
			t.Fatalf("TUI: %+v", a)
		}
		page, err := view.BuildLogPage(e.App, ps.State, a.LogPath, nil)
		if err != nil {
			t.Fatal(err)
		}
		selected := false
		for _, r := range page.Rows {
			if r.Cluster == a.Cluster {
				selected = strings.Contains(r.Mods, "Alpha Props")
			}
		}
		if !selected {
			t.Fatalf("target cluster absent: %+v", page.Rows)
		}
	}
	if !found {
		t.Fatalf("no last-run Alpha item: %+v", ns.Items)
	}
}
