package app_test

import (
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The run's saves from its phase marks (06 §7): which it loaded, which it played last.
func TestLogRunSaves(t *testing.T) {
	t.Parallel()
	p := func(name, save string) *logs.Phase { return &logs.Phase{Name: name, Save: save} }
	r := &app.LogRun{Marks: []*logs.Phase{p("menu", ""), p("save", "Ashfall.rws"), p("play", ""), p("menu", ""), p("save", "Tundra"), p("play", ""), p("menu", "")}}
	if got := r.LoadedSaves(); len(got) != 2 || got[0] != "Ashfall.rws" || got[1] != "Tundra" {
		t.Fatalf("loaded %q", got)
	}
	if r.PlayedSave() != "Tundra" || r.LastSave() != "Tundra" {
		t.Fatalf("played %q last %q", r.PlayedSave(), r.LastSave())
	}
	r.Marks = append(r.Marks, p("new", ""), p("play", ""))
	if r.PlayedSave() != "" || r.LastSave() != "Tundra" {
		t.Fatalf("new colony: played %q last %q", r.PlayedSave(), r.LastSave())
	}
	if (*app.LogRun)(nil).LastSave() != "" || app.SaveKey("Colony1.RWS") != app.SaveKey("colony1") {
		t.Fatal("nil run or save key")
	}
}

// Can it load? has a row for errors the last load of the save logged (12 §4.3), from the
// log's stored report only; a run that loaded two saves attributes nothing.
func TestCheckSaveLastLoad(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.WriteSave("Colony1.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "brrainz.harmony", "tester.a"}})
	e.WriteSave("Other.rws", testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "brrainz.harmony", "tester.a"}})
	pl, am := filepath.Join(e.UserData, "Player.log"), filepath.Join(e.UserData, "ArchoMonitor.log")
	msg := "Could not resolve cross-reference to Verse.ThingDef named Gone_Thing"
	entry := func(n, total int) string { return monitorLine(t, "", "ERROR", msg, "", n, total, "xref", nil) }
	// A texture warning begins while the game loads and repeats during the save's load:
	// it counts, but the load's first error is the one that began there.
	tex := "Could not load Texture2D at 'Things/Example/Empty' for def 'a_Thing'"
	texEntry := func(n, total int) string {
		return monitorLine(t, "", "WARNING", tex, "", n, total, "texture", map[string]any{"relatedModIds": []string{"tester.a"}, "inferredType": "MissingTexture"})
	}
	write := func(saves ...string) {
		comp := sessionLine(t, "bbbbbbbbbbbb") + texEntry(1, 1) + phaseLine(t, "menu", "")
		log := unityHead + marker("bbbbbbbbbbbb", am) + tex + "\n\n[Archo Monitor] phase menu\n"
		for i, s := range saves {
			comp += phaseLine(t, "save", s) + texEntry(1, i+2) + entry(1, i+1) + phaseLine(t, "play", "")
			log += "[Archo Monitor] phase save\n" + tex + "\n\n" + msg + "\n\n[Archo Monitor] phase play\n"
		}
		e.WriteFile(am, comp)
		e.WriteFile(pl, log)
	}
	check := func(name string) *app.SaveCheck {
		t.Helper()
		s, _, err := e.App.FindSave(name)
		if err != nil {
			t.Fatal(err)
		}
		c, err := e.App.CheckSave(s)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	lastLoad := func(c *app.SaveCheck) *app.SaveIssue {
		for i := range c.Issues {
			if c.Issues[i].Kind == app.IssueLastLoad {
				return &c.Issues[i]
			}
		}
		return nil
	}

	write("Colony1")
	// Not analysed yet: the stored report is all CheckSave reads, so nothing is said.
	if is := lastLoad(check("Colony1")); is != nil {
		t.Fatalf("before analysis: %+v", is)
	}
	if _, err := e.App.AnalyzeLog(pl); err != nil {
		t.Fatal(err)
	}
	is := lastLoad(check("Colony1"))
	if is == nil || is.Level != app.LevelError || is.Load.Errors != 2 || is.Load.Kinds != 2 || is.Load.Cluster != "E2" || is.Load.First != msg {
		t.Fatalf("last load: %+v", is)
	}
	if len(is.Fixes) != 1 || is.Fixes[0].Action != app.FixOpenLog || is.Fixes[0].Target != pl {
		t.Fatalf("fix %+v", is.Fixes)
	}
	if c := check("Other"); lastLoad(c) != nil {
		t.Fatal("another save got the row")
	}
	// Can it run's last save: the one the run loaded, checked against the current list.
	ps, err := e.App.Problems(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	c, run, err := e.App.LastLoadedSave(ps.State, ps.Logs)
	if err != nil || run == nil || c == nil || c.Save.Name != "Colony1" || c.Save.Plan == nil || c.Save.Plan.ID != ps.State.Plan.ID || lastLoad(c) == nil {
		t.Fatalf("last loaded save: %+v run %+v err %v", c, run, err)
	}
	if v := view.BuildNeeds(e.App, ps.State, ps).LastSave; v == nil || v.Name != "Colony1" || v.Level != app.LevelError || len(v.Lines) == 0 {
		t.Fatalf("needs last save: %+v", v)
	}

	// Two saves loaded in one run: the save phase's counts are not attributed.
	write("Colony1.rws", "Other.rws")
	if _, err := e.App.AnalyzeLog(pl); err != nil {
		t.Fatal(err)
	}
	if is := lastLoad(check("Colony1")); is != nil {
		t.Fatalf("ambiguous run: %+v", is)
	}
}
