package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// A test run links the plan's Mods the game cannot see (pinned copies, SteamCMD and extra
// folders) into Mods/ for the run only, and removes those links when the game has exited
// (20 §2). A Mods/ entry with the packageId of a Mod it links (the original of a pinned
// copy, say) would be loaded instead of it, so that entry is moved aside into the game
// folder's _ArchoTestHidden for the run and moved back after it. Links and moves are
// recorded before they are made, so the next test run undoes what an interrupted one left.

const testLinksFile = "links.json"

// testHiddenDir is the folder next to Mods/ that holds the entries a run moved aside.
const testHiddenDir = "_ArchoTestHidden"

type testLinks struct {
	Mods   string       `json:"mods"`
	Links  []LinkChange `json:"links"`
	Hidden []testHidden `json:"hidden,omitempty"`
}

// testHidden is a Mods/ entry moved aside for a run.
type testHidden struct {
	Name string `json:"name"` // its name in Mods/
	To   string `json:"to"`   // where it is during the run
	Mod  string `json:"mod"`  // the instance whose link needs it out of the way
}

// testRunLinks plans the temporary links: listed deployable Mods the game does not see,
// and the Mods/ entries with their packageId to move aside (hide, by name). A Mod that
// cannot be linked without changing another Mod in Mods/ stays missing, with the reason in
// blocked (by instance ID).
func (a *App) testRunLinks(st *LoadoutState) (string, []LinkChange, []testHidden, map[string]string) {
	seen := testRunSeen(st)
	blocked := map[string]string{}
	var want []*store.Mod
	for _, e := range st.List {
		if m := st.Ix.ByID[e.Instance]; m != nil && m.Kind != source.Official && seen[m.ID] == "" && st.Deployable(m) {
			want = append(want, m)
		}
	}
	if len(want) == 0 {
		return "", nil, nil, blocked
	}
	mods := modsDir(st.Ix)
	if fi, err := os.Stat(mods); mods == "" || err != nil || !fi.IsDir() {
		for _, m := range want {
			blocked[m.ID] = "the game has no Mods/ folder to link it into"
		}
		return "", nil, nil, blocked
	}
	byPkg := map[string][]string{} // packageId -> the Mods/ entries that have it
	for _, c := range a.ModsCopies(st.Ix) {
		byPkg[c.PackageID] = append(byPkg[c.PackageID], c.Name)
	}
	var add []LinkChange
	var hide []testHidden
	for _, m := range want {
		name := DeployName(m)
		same := byPkg[strings.ToLower(m.PackageID)]
		if _, err := os.Lstat(filepath.Join(mods, name)); err == nil && !slices.ContainsFunc(same, func(s string) bool { return strings.EqualFold(s, name) }) {
			blocked[m.ID] = "Mods/" + name + " exists and is not this copy; save the plan to replace it"
			continue
		}
		// The game loads only one copy of a packageId from Mods/ (09 §3): the others go
		// aside for the run.
		for _, other := range same {
			hide = append(hide, testHidden{Name: other, Mod: m.ID})
		}
		add = append(add, LinkChange{Op: "add", Name: name, Target: m.RealPath, Mod: m.ID})
	}
	return mods, add, hide, blocked
}

// addTestLinks records, then moves the hidden entries aside and makes the links; the
// returned func undoes both. An entry that cannot be moved leaves its Mod unlinked, with
// the reason in blocked; done lists the names moved aside.
func (a *App) addTestLinks(base, mods string, add []LinkChange, hide []testHidden) (undo func() error, done []string, blocked map[string]string, err error) {
	undo = func() error { return a.removeTestLinks(base) }
	blocked = map[string]string{}
	if len(add) == 0 && len(hide) == 0 {
		return undo, nil, blocked, nil
	}
	aside := filepath.Join(filepath.Dir(mods), testHiddenDir)
	taken := map[string]bool{}
	for i := range hide {
		to := filepath.Join(aside, hide[i].Name)
		for n := 2; taken[strings.ToLower(to)] || lexists(to); n++ {
			to = filepath.Join(aside, hide[i].Name+"-"+strconv.Itoa(n))
		}
		taken[strings.ToLower(to)] = true
		hide[i].To = to
	}
	record := func(add []LinkChange, hide []testHidden) error {
		b, err := json.MarshalIndent(testLinks{Mods: mods, Links: add, Hidden: hide}, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(base, testLinksFile), append(b, '\n'), 0o644)
	}
	if err := record(add, hide); err != nil {
		return nil, nil, nil, err
	}
	var moved []testHidden
	for _, h := range hide {
		if blocked[h.Mod] != "" {
			continue
		}
		err := os.MkdirAll(aside, 0o755)
		if err == nil {
			err = os.Rename(filepath.Join(mods, h.Name), h.To)
		}
		if err != nil {
			blocked[h.Mod] = fmt.Sprintf("Mods/%s has the same packageId and could not be moved aside for the run: %v", h.Name, err)
			continue
		}
		moved = append(moved, h)
		done = append(done, h.Name)
	}
	add = slices.DeleteFunc(add, func(c LinkChange) bool { return blocked[c.Mod] != "" })
	if err := record(add, moved); err != nil {
		return nil, nil, nil, errors.Join(err, undo())
	}
	if err := a.applyLinks(mods, add); err != nil {
		return nil, nil, nil, errors.Join(fmt.Errorf("test run: linking into %s: %w", mods, err), undo())
	}
	slices.Sort(done)
	return undo, done, blocked, nil
}

func lexists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// removeTestLinks removes the recorded links that are still links to the recorded folder,
// moves the hidden entries back, then deletes the record. Nothing else in Mods/ is
// touched. An entry that cannot go back (a new one has its name) keeps the record, so the
// next test run tries again, and is an error naming where it is.
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
	var left []testHidden
	var errs []error
	for _, h := range rec.Hidden {
		if !lexists(h.To) {
			continue // moved back already, or by the player
		}
		back := filepath.Join(rec.Mods, h.Name)
		if lexists(back) {
			left = append(left, h)
			errs = append(errs, fmt.Errorf("test run: Mods/%s exists again, so the copy moved aside for the run stays in %s", h.Name, h.To))
			continue
		}
		if err := os.Rename(h.To, back); err != nil {
			left = append(left, h)
			errs = append(errs, fmt.Errorf("test run: moving %s back to Mods/: %w", h.To, err))
		}
	}
	if len(rec.Hidden) > 0 {
		os.Remove(filepath.Join(filepath.Dir(rec.Mods), testHiddenDir)) // only when empty
	}
	if len(left) > 0 {
		rec.Links, rec.Hidden = nil, left
		b, err := json.MarshalIndent(rec, "", "  ")
		if err == nil {
			err = os.WriteFile(path, append(b, '\n'), 0o644)
		}
		return errors.Join(append(errs, err)...)
	}
	return os.Remove(path)
}
