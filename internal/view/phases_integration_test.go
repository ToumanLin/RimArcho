package view_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestPhaseGroupsAndLogSeparators(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.WriteMod(filepath.Join(e.Game, "Mods", "Alpha"), testkit.Sample("phase.alpha", "Alpha Props"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "phase.alpha")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("save")
	player, monitor := filepath.Join(e.UserData, "Player.log"), filepath.Join(e.UserData, "ArchoMonitor.log")
	var p, m strings.Builder
	p.WriteString("RimWorld 1.6.4633 rev1261\n[Archo Monitor] 0.9.1 session aaaaaaaaaaaa: " + monitor + "\n")
	line := func(kind, phase, message string, n int) {
		ctx := map[string]any{"phase": phase, "relatedModIds": []string{"phase.alpha"}, "roles": map[string]string{"phase.alpha": "xml"}}
		if phase == "save" {
			ctx["save"] = "Colony1.rws"
		}
		b, _ := json.Marshal(map[string]any{"timestamp": "2026-10-28 09:48:00.000", "source": "archo-monitor", "schema": 1, "kind": kind, "context": ctx, "level": "ERROR", "message": message, "count": n, "id": message})
		m.Write(b)
		m.WriteByte('\n')
	}
	b, _ := json.Marshal(map[string]any{"kind": "session", "message": "session", "context": map[string]any{"session": "aaaaaaaaaaaa"}})
	m.Write(b)
	m.WriteByte('\n')
	for i, phase := range []string{"loading", "new", "play", "save", "menu"} {
		if phase != "loading" {
			p.WriteString("[Archo Monitor] phase " + phase + "\n")
			line("phase", phase, "[Archo Monitor] phase "+phase, 1)
		}
		msg := fmt.Sprintf("Could not load Texture2D at 'Things/%s/Empty' for def 'phase_Thing'", phase)
		p.WriteString(msg + "\n\n")
		line("", "", msg, i+1)
	}
	e.WriteFile(player, p.String())
	e.WriteFile(monitor, m.String())
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	page, err := view.BuildLogPage(e.App, st, player, nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.Monitor == nil || len(page.Rows) != 5 || len(page.Monitor.Entries) != 5 {
		t.Fatalf("rows %d monitor %+v", len(page.Rows), page.Monitor)
	}
	want := []string{"loading the game", "starting a new colony", "playing", "loading Colony1.rws", "main menu"}
	for i, row := range page.Rows {
		if !strings.HasPrefix(row.PhaseStart, want[i]) || !strings.HasPrefix(page.Monitor.Entries[i].PhaseStart, want[i]) {
			t.Fatalf("row %d: %+v %+v", i, row, page.Monitor.Entries[i])
		}
	}
	ps, err := e.App.Problems(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	ns := view.BuildNeeds(e.App, ps.State, ps)
	var phases []string
	for _, n := range ns.Items {
		if n.Phase != "" {
			phases = append(phases, n.Phase)
			if n.PhaseHeading == "" || n.Actions[0].Kind != view.ActGameLog {
				t.Fatalf("item: %+v", n)
			}
		}
	}
	// The last run's three blocks (16 §2.4): loading and the menu are starting the game; a
	// new colony is playing.
	if strings.Join(phases, ",") != "start,load,play" {
		t.Fatalf("needs phases %v", phases)
	}
}
