package app_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/machine"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 19 §2, 16 §2.17: Archo Monitor's memory records reach the comparison: the newest run's
// main-menu sample a minute in (not the one at entry), with its Mod count checked against
// the list, and Can it run names what the game used.
func TestMemoryLastRunFromMonitor(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.App.MemoryProbe = func() machine.Memory {
		return machine.Memory{TotalRAM: 16 << 30, AvailableRAM: 12 << 30, SampledAt: time.Now()}
	}
	if _, err := e.App.PlanLoadCost(context.Background(), 0, nil); err != nil {
		t.Fatal(err)
	}
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	line := func(at, phase string, after int, ws int64, mods int) string {
		return fmt.Sprintf(`{"timestamp":"2026-11-26 %s","level":"INFO","message":"[Archo Monitor] memory %s","details":"","count":1,"context":{"memory":{"phase":"%s","after":%d,"mods":%d,"workingSet":%d,"private":%d,"peakWorkingSet":%d,"vram":1}},"source":"archo-monitor","schema":1,"kind":"memory"}`+"\n", at, phase, phase, after, mods, ws, ws+1, ws+2)
	}
	n := len(st.List)
	e.WriteFile(filepath.Join(e.UserData, "ArchoMonitor.log"),
		`{"timestamp":"2026-11-26 10:00:00.000","level":"INFO","message":"[Archo Monitor] session 0123456789ab","details":"","count":1,"context":{"session":"0123456789ab","version":"0.13.0","game":"1.7.5021 rev1365"},"source":"archo-monitor","schema":1,"kind":"session"}`+"\n"+
			line("10:01:00.000", "menu", 0, 2<<30, n)+line("10:02:00.000", "menu", 60, 3<<30, n)+line("10:05:00.000", "play", 0, 4<<30, n))
	c := e.App.CachedMemoryAssessment(st)
	r := c.LastRun
	if r == nil || r.Session != "0123456789ab" || !r.SameCount || r.Menu == nil || r.Menu.After != 60 || r.Menu.WorkingSet != 3<<30 || r.Play == nil || r.Play.WorkingSet != 4<<30 {
		t.Fatalf("last run %+v", r)
	}
	if why := view.MemoryNeed(c, st.Plan.ID).Why; !strings.Contains(why, view.Tf("Last run: the game used %s at the main menu and %s in play (Archo Monitor).", view.SizeText(3<<30), view.SizeText(4<<30))) {
		t.Fatalf("why %q", why)
	}
	// a run of another list size is kept but not shown as this list's
	e.WriteFile(filepath.Join(e.UserData, "ArchoMonitor.log"),
		`{"timestamp":"2026-11-26 11:00:00.000","level":"INFO","message":"[Archo Monitor] session 0123456789cd","details":"","count":1,"context":{"session":"0123456789cd","version":"0.13.0","game":"1.7.5021 rev1365"},"source":"archo-monitor","schema":1,"kind":"session"}`+"\n"+
			line("11:01:00.000", "menu", 0, 5<<30, n+7))
	c = e.App.CachedMemoryAssessment(st)
	if c.LastRun == nil || c.LastRun.Session != "0123456789cd" || c.LastRun.SameCount || c.LastRun.Menu.After != 0 {
		t.Fatalf("other list %+v", c.LastRun)
	}
	if why := view.MemoryNeed(c, st.Plan.ID).Why; strings.Contains(why, "Last run") {
		t.Fatalf("another list's run shown: %q", why)
	}
}
