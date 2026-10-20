package app_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// G8 (08 §12): two RimArcho processes on one data directory (the TUI and the window). Each
// sees the other's edit at its next look, and an edit made on a stale view does not
// overwrite the other's: both edits are in the workspace and in the undo history.
func TestTwoProcessesShareTheWorkspace(t *testing.T) {
	e := loadoutEnv(t)
	scanNow(t, e)
	other, err := app.Open(e.Home)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	other.Now, other.Loc = e.App.Now, e.App.Loc

	// Both have looked once (caches warm).
	if _, err := e.App.Loadout(); err != nil {
		t.Fatal(err)
	}
	stamp := e.App.Store.ChangeKey()
	if _, err := other.Loadout(); err != nil {
		t.Fatal(err)
	}

	// The other process enables B; this one sees it next time it looks.
	if _, err := other.Enable([]string{"pkg:tester.b"}, app.Placement{}); err != nil {
		t.Fatal(err)
	}
	if e.App.Store.ChangeKey() == stamp {
		t.Fatal("the change key did not move after the other process wrote (the window would not reload)")
	}
	if got := joined(active(t, e)); !strings.Contains(got, "B") {
		t.Fatalf("this process does not see the other's edit: %s", got)
	}

	// This process disables A without looking first: the edit starts from the current
	// workspace, so B stays.
	if _, err := e.App.Disable([]string{"pkg:tester.a"}); err != nil {
		t.Fatal(err)
	}
	st, err := other.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, en := range st.List {
		if m := st.Ix.ByID[en.Instance]; m != nil {
			names = append(names, m.DisplayName())
		}
	}
	if got := strings.Join(names, ", "); !strings.Contains(got, "B") || strings.Contains(got, ", A") {
		t.Fatalf("the other process sees %s; want B kept and A gone", got)
	}
	h, err := other.History()
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Undo) < 2 || h.Undo[0].Op != "disable" || h.Undo[1].Op != "enable" {
		t.Fatalf("history: %+v", h.Undo)
	}
}
