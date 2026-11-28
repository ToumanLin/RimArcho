package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// An analysis reads Mod folders and writes only RimArcho's store; the launch lock guards the
// game folders' links, ModsConfig.xml and the stored lists (14 §1 "Busy"). A test run holds
// that lock for minutes while it changes nothing an analysis reads except the Mods/ entries
// it moves aside, so an analysis of a list that is already stored runs beside it.

// analysisLoadout is the list AnalyzeContext analyses. Normally it is Loadout, which imports
// an outside ModsConfig.xml change under the lock. While another process's test run holds the
// lock (its busy.json), it reads the stored list instead; the import waits for the lock, and
// so does a list never stored (a first use) and an analysis that would read a Mods/ entry the
// run moved aside.
func (a *App) analysisLoadout() (*LoadoutState, error) {
	h, ok := liveBusy(a.Home)
	if !ok || h.Kind != busyTestRun {
		return a.Loadout()
	}
	prev := a.tryWorkspaceSync
	a.tryWorkspaceSync = true // one attempt: the holder keeps it for minutes
	st, err := a.Loadout()
	a.tryWorkspaceSync = prev
	var busy *ErrBusy
	if err == nil || !errors.As(err, &busy) {
		return st, err
	}
	stored, serr := a.LoadoutSnapshot()
	if serr != nil || stored == nil || stored.Plan == nil {
		return nil, err // nothing stored yet: the first import needs the lock
	}
	if name := testRunMovedAside(a.Home, stored); name != "" {
		return nil, fmt.Errorf("%w; the analysis reads %s, which the test run moved aside until it ends", err, name)
	}
	return stored, nil
}

// testRunMovedAside names ("Mods/<name>") the first entry a test run journal moves aside
// that holds a Mod the analysis of st reads; "" when none. The journal lists the entries
// before they move and until they are back.
func testRunMovedAside(home string, st *LoadoutState) string {
	bases, _ := os.ReadDir(filepath.Join(home, "tests"))
	mods, _ := analysisMods(st)
	for _, de := range bases {
		b, err := os.ReadFile(filepath.Join(home, "tests", de.Name(), testLinksFile))
		if err != nil {
			continue
		}
		var rec testLinks
		if json.Unmarshal(b, &rec) != nil {
			return "a Mods/ entry (unreadable test run journal)"
		}
		for _, h := range rec.Hidden {
			entry := filepath.Join(rec.Mods, h.Name)
			key := canonicalRoot(entry)
			for _, m := range mods {
				if dir := st.Ix.ModDir(m); within(entry, dir) || within(key, dir) {
					return "Mods/" + h.Name
				}
			}
		}
	}
	return ""
}
