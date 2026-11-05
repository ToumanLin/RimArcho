package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// A test run links the plan's Mods the game cannot see (pinned copies, SteamCMD and extra
// folders) into Mods/ for the run only, and removes those links when the game has exited
// (08 §12). The links are recorded before they are made, so the next test run removes
// what an interrupted one left.

const testLinksFile = "links.json"

type testLinks struct {
	Mods  string       `json:"mods"`
	Links []LinkChange `json:"links"`
}

// testRunLinks plans the temporary links: listed deployable Mods the game does not see.
// A Mod that cannot be linked without changing what is already in Mods/ stays missing,
// with the reason in blocked (by instance ID).
func (a *App) testRunLinks(st *LoadoutState) (string, []LinkChange, map[string]string) {
	seen := testRunSeen(st)
	blocked := map[string]string{}
	var want []*store.Mod
	for _, e := range st.List {
		if m := st.Ix.ByID[e.Instance]; m != nil && m.Kind != source.Official && seen[m.ID] == "" && st.Deployable(m) {
			want = append(want, m)
		}
	}
	if len(want) == 0 {
		return "", nil, blocked
	}
	mods := modsDir(st.Ix)
	if fi, err := os.Stat(mods); mods == "" || err != nil || !fi.IsDir() {
		for _, m := range want {
			blocked[m.ID] = "the game has no Mods/ folder to link it into"
		}
		return "", nil, blocked
	}
	inMods := map[string]string{} // packageId -> a Mods/ entry that has it
	for _, c := range a.ModsCopies(st.Ix) {
		if inMods[c.PackageID] == "" {
			inMods[c.PackageID] = c.Name
		}
	}
	var add []LinkChange
	for _, m := range want {
		name := DeployName(m)
		if _, err := os.Lstat(filepath.Join(mods, name)); err == nil {
			blocked[m.ID] = "Mods/" + name + " exists and is not this copy; save the plan to replace it"
		} else if other := inMods[strings.ToLower(m.PackageID)]; other != "" {
			// The game loads only one copy of a packageId from Mods/ (08 §3).
			blocked[m.ID] = "Mods/" + other + " has the same packageId; the game would load only one"
		} else {
			add = append(add, LinkChange{Op: "add", Name: name, Target: m.RealPath, Mod: m.ID})
		}
	}
	return mods, add, blocked
}

// addTestLinks records, then makes the links; the returned func removes them.
func (a *App) addTestLinks(base, mods string, add []LinkChange) (func() error, error) {
	undo := func() error { return a.removeTestLinks(base) }
	if len(add) == 0 {
		return undo, nil
	}
	b, err := json.MarshalIndent(testLinks{Mods: mods, Links: add}, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(base, testLinksFile), append(b, '\n'), 0o644); err != nil {
		return nil, err
	}
	if err := a.applyLinks(mods, add); err != nil {
		return nil, errors.Join(fmt.Errorf("test run: linking into %s: %w", mods, err), undo())
	}
	return undo, nil
}

// removeTestLinks removes the recorded links that are still links to the recorded folder,
// then the record. Nothing else in Mods/ is touched.
func (a *App) removeTestLinks(base string) error {
	path := filepath.Join(base, testLinksFile)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	var rec testLinks
	if err := json.Unmarshal(b, &rec); err != nil {
		return fmt.Errorf("test run: reading %s: %w", path, err)
	}
	var rm []LinkChange
	for _, l := range rec.Links {
		if p := filepath.Join(rec.Mods, l.Name); managedLink(l.Name) && source.IsLink(p) && sameDir(p, l.Target) {
			rm = append(rm, LinkChange{Op: "remove", Name: l.Name, Target: l.Target, Mod: l.Mod})
		}
	}
	if err := a.applyLinks(rec.Mods, rm); err != nil {
		return fmt.Errorf("test run: removing its links from %s: %w", rec.Mods, err)
	}
	return os.Remove(path)
}
