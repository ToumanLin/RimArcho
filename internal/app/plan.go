package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Plans (04 §1): named lists shared by every game; each game has one current list — the
// one it is launched with — and every editing, history and staging command acts on it, or
// on the list a session opened instead (OpenPlan, --open-plan). Switching writes no game
// file.

// PlanInfo is a plan with what `plan list` shows (05 §4.3).
type PlanInfo struct {
	*store.Plan
	Current    bool // the environment's current plan: the one launched
	Open       bool // the plan this session shows and edits (the current one unless OpenPlan)
	Entries    int  // list length, missing entries included
	Versions   int
	LastPlayed time.Time // zero: never launched
}

// CurrentPlan returns the environment's current plan, creating the first one from
// ModsConfig.xml on first use (04 §1.4).
func (a *App) CurrentPlan() (*store.Plan, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	return st.Plan, nil
}

// Plans lists every plan in creation order, marked against the current game.
func (a *App) Plans() ([]*PlanInfo, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	ps, err := a.Store.Plans()
	if err != nil {
		return nil, err
	}
	cur := a.Store.CurrentPlanID(st.Ix.Env.ID)
	var out []*PlanInfo
	for _, p := range ps {
		pi := &PlanInfo{Plan: p, Current: p.ID == cur, Open: p.ID == st.Plan.ID, Versions: a.Store.VersionCount(p.ID), LastPlayed: a.Store.LastPlanLaunch(p.ID)}
		if pi.Open {
			pi.Entries = len(st.List)
		} else if ws, err := a.Store.Workspace(p.ID); err != nil {
			return nil, err
		} else if ws != nil {
			pi.Entries = len(loadout.Decode(ws.Entries))
		}
		out = append(out, pi)
	}
	return out, nil
}

// PlansIn lists every plan with another game's current one marked, without loading that
// game: each list's length comes from its workspace (the home page lists every game, and
// lists are shared by all of them, 06 §2.3).
func (a *App) PlansIn(envID string) ([]*PlanInfo, error) {
	ps, err := a.Store.Plans()
	if err != nil {
		return nil, err
	}
	cur := a.Store.CurrentPlanID(envID)
	var out []*PlanInfo
	for _, p := range ps {
		pi := &PlanInfo{Plan: p, Current: p.ID == cur, Open: p.ID == cur, Versions: a.Store.VersionCount(p.ID), LastPlayed: a.Store.LastPlanLaunch(p.ID)}
		if ws, err := a.Store.Workspace(p.ID); err != nil {
			return nil, err
		} else if ws != nil {
			pi.Entries = len(loadout.Decode(ws.Entries))
		}
		out = append(out, pi)
	}
	return out, nil
}

// PlanRef is how output names a plan.
func PlanRef(p *store.Plan) string { return "plan:" + p.Name }

// FindPlan resolves a plan name (with or without "plan:").
func (a *App) FindPlan(name string) (*store.Plan, error) {
	if _, err := a.Loadout(); err != nil {
		return nil, err
	}
	return a.findPlan(name)
}

func (a *App) findPlan(name string) (*store.Plan, error) {
	n := strings.TrimSpace(name)
	if len(n) > 5 && strings.EqualFold(n[:5], "plan:") {
		n = n[5:]
	}
	p, err := a.Store.PlanByName(n)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, fmt.Errorf("no plan %q (archo plan list)", n)
	}
	return p, nil
}

// checkPlanName validates a new plan name and refuses one the environment already has.
func (a *App) checkPlanName(name string, self int64) (string, error) {
	n := strings.TrimSpace(name)
	if len(n) > 5 && strings.EqualFold(n[:5], "plan:") {
		n = strings.TrimSpace(n[5:])
	}
	switch {
	case n == "":
		return "", errors.New("a plan needs a name")
	case utf8.RuneCountInString(n) > 64:
		return "", errors.New("plan names are at most 64 characters")
	case strings.HasPrefix(n, "-"):
		return "", fmt.Errorf("plan name %q starts with -", n)
	case strings.IndexFunc(n, unicode.IsControl) >= 0:
		return "", errors.New("plan names cannot contain control characters")
	}
	ps, err := a.Store.Plans()
	if err != nil {
		return "", err
	}
	for _, p := range ps {
		if p.ID != self && strings.EqualFold(p.Name, n) {
			return "", fmt.Errorf("plan:%s already exists (archo plan list)", p.Name)
		}
	}
	return n, nil
}

// NewPlanResult describes a created plan.
type NewPlanResult struct {
	Plan    *store.Plan
	List    loadout.List
	Version *store.Version // its v1 (source import)
	State   *LoadoutState  // the current plan's state the list was resolved with
	// Imported is set when the list came from a file; Unresolved are its workshop IDs no
	// installed mod has (they are not in the list).
	Imported   *ImportedList
	Unresolved []string
	// Monitor says what the Archo Monitor default did to the new plan (04 §1.5): "" or
	// "added after Harmony", one undoable edit after v1.
	Monitor string
}

var versionArgRe = regexp.MustCompile(`^[vV]?\d+$`)

// NewPlan creates a plan in the current environment (04 §1.3) from an empty list (from ""),
// the current plan ("current"), one of its versions ("v<n>"), or a list file. Its first
// version has source import and a note saying where it came from. The current plan stays.
func (a *App) NewPlan(name, from string) (*NewPlanResult, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	e := st.Ix.Env
	if name, err = a.checkPlanName(name, 0); err != nil {
		return nil, err
	}
	v := &store.Version{Source: "import"}
	var list loadout.List
	var il *ImportedList
	var unresolved []string
	switch f := strings.TrimSpace(from); {
	case f == "":
		v.Note = "empty"
	case strings.EqualFold(f, "current"):
		list = st.List.Clone()
		if st.Latest != nil {
			v.GameVersion = st.Latest.GameVersion
		}
		if st.Latest != nil && len(st.Changes()) == 0 {
			v.Note = fmt.Sprintf("copied from %s v%d", st.Plan.Name, st.Latest.N)
		} else {
			v.Note = fmt.Sprintf("copied from %s (unsaved edits)", st.Plan.Name)
		}
	case versionArgRe.MatchString(f) && !fileExists(f):
		n, err := a.parsePlanVersion(st.Plan, f)
		if err != nil {
			return nil, err
		}
		if list, err = a.ListAt(st, n); err != nil {
			return nil, err
		}
		src, _ := a.Store.Version(st.Plan.ID, n)
		v.GameVersion = src.GameVersion
		v.Note = fmt.Sprintf("copied from %s v%d", st.Plan.Name, n)
	default:
		if il, err = readListFile(f); err != nil {
			return nil, err
		}
		list, unresolved = ImportedEntries(st, il)
		il.Unresolved = unresolved
		v.GameVersion, v.Note = il.Game, importLabel(f, il)
	}
	p, err := a.createPlan(e.ID, name, list, v)
	if err != nil {
		return nil, err
	}
	// A copy keeps the player's choice of game versions; a RimArcho list brings its own.
	switch {
	case il != nil:
		a.adoptVersions(p, il.Versions)
	case strings.TrimSpace(from) != "" && len(st.Plan.Versions) > 0:
		a.adoptVersions(p, st.Plan.Versions)
	}
	mon, err := a.MonitorForNewPlan(p)
	if err != nil {
		return nil, err
	}
	return &NewPlanResult{Plan: p, List: list, Version: v, State: st, Imported: il, Unresolved: unresolved, Monitor: mon}, nil
}

// ImportLoadoutAsPlan imports a list file (or share code) as a new plan (04 §9.1 "import
// as a new plan"); name "" takes the list's own name, else the file name. The current plan
// stays.
func (a *App) ImportLoadoutAsPlan(path, name string) (*NewPlanResult, error) {
	if strings.TrimSpace(name) != "" {
		return a.NewPlan(name, path)
	}
	il, err := readListFile(path)
	if err != nil {
		return nil, err
	}
	base := strings.TrimSpace(il.Title)
	if base == "" {
		base = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if il.Format == "rimcrow-share" {
			base = "share code"
		}
	}
	name = base
	for i := 2; ; i++ {
		if p, _ := a.Store.PlanByName(name); p == nil {
			break
		}
		name = fmt.Sprintf("%s (%d)", base, i)
	}
	return a.NewPlan(name, path)
}

// NewPlanFromImport creates a plan from a list an interface already read and previewed
// (a file, pasted text, a share code, a Workshop collection; 04 §9.1 "import as a new
// plan"). source is its v1's note. The current plan stays.
func (a *App) NewPlanFromImport(name string, il *ImportedList, source string) (*NewPlanResult, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	e := st.Ix.Env
	if name, err = a.checkPlanName(name, 0); err != nil {
		return nil, err
	}
	list, unresolved := ImportedEntries(st, il)
	v := &store.Version{Source: "import", GameVersion: il.Game, Note: source}
	if v.Note == "" {
		v.Note = "import"
	}
	p, err := a.createPlan(e.ID, name, list, v)
	if err != nil {
		return nil, err
	}
	a.adoptVersions(p, il.Versions)
	mon, err := a.MonitorForNewPlan(p)
	if err != nil {
		return nil, err
	}
	return &NewPlanResult{Plan: p, List: list, Version: v, State: st, Imported: il, Unresolved: unresolved, Monitor: mon}, nil
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func readListFile(path string) (*ImportedList, error) {
	il, err := ReadLoadoutFile(path)
	if err != nil {
		return nil, err
	}
	if len(il.Tokens) == 0 && len(il.WorkshopOnly) == 0 {
		return nil, fmt.Errorf("%s: no mods in the file", path)
	}
	return il, nil
}

func importLabel(path string, il *ImportedList) string {
	if il.Format == "rimcrow-share" {
		return "share code"
	}
	return filepath.Base(path)
}

// createPlan stores a plan with list as its v1 and its workspace.
func (a *App) createPlan(envID, name string, list loadout.List, v *store.Version) (*store.Plan, error) {
	now := a.Now().UTC()
	p, err := a.Store.CreatePlan(envID, name, now)
	if err != nil {
		return nil, err
	}
	v.Created, v.Entries = now, list.Encode()
	n, err := a.Store.AddVersion(p.ID, v)
	if err != nil {
		return nil, err
	}
	ws := &store.Workspace{PlanID: p.ID, Entries: v.Entries, OriginVersion: n, OriginKind: "import", Updated: now}
	return p, a.Store.SetWorkspace(ws)
}

// UsePlan makes a plan current: the one launched, and the one this session edits. It
// writes no game file (04 §1.3).
func (a *App) UsePlan(name string) (*store.Plan, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	p, err := a.findPlan(name)
	if err != nil {
		return nil, err
	}
	if err := a.Store.SetCurrentPlan(st.Ix.Env.ID, p.ID); err != nil {
		return nil, err
	}
	a.OpenPlan, a.PlanName = 0, ""
	return p, nil
}

// OpenPlanID makes this session show and edit a plan without making it current: the game
// keeps being launched with the current one until the player chooses this one, writes it
// to the game or launches it (04 §1.3). Opening the current plan clears the override.
func (a *App) OpenPlanID(id int64) (*store.Plan, error) {
	e, err := a.Env()
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, errors.New("no game")
	}
	p, err := a.Store.PlanByID(id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, errors.New("that list is gone")
	}
	a.PlanName = ""
	if id == a.Store.CurrentPlanID(e.ID) {
		a.OpenPlan = 0
	} else {
		a.OpenPlan = id
	}
	return p, nil
}

// OpenPlanName is the --open-plan value that opens this session's plan in a child process ("":
// the session edits the current plan).
func (a *App) OpenPlanName() string {
	if a.OpenPlan != 0 {
		if p, err := a.Store.PlanByID(a.OpenPlan); err == nil && p != nil {
			return p.Name
		}
		return ""
	}
	return a.PlanName
}

// planFor is the plan this session works on in an environment: the opened one when it
// belongs there, else the current one (first falling back to the first plan).
func (a *App) planFor(envID string) (*store.Plan, error) {
	if a.OpenPlan != 0 {
		if p, err := a.Store.PlanByID(a.OpenPlan); err != nil {
			return nil, err
		} else if p != nil {
			return p, nil
		}
	} else if a.PlanName != "" {
		return a.findPlan(a.PlanName)
	}
	return a.Store.CurrentPlan(envID)
}

// RenamePlan renames a plan; its history is unchanged.
func (a *App) RenamePlan(oldName, newName string) (*store.Plan, string, error) {
	if _, err := a.Loadout(); err != nil {
		return nil, "", err
	}
	p, err := a.findPlan(oldName)
	if err != nil {
		return nil, "", err
	}
	n, err := a.checkPlanName(newName, p.ID)
	if err != nil {
		return nil, "", err
	}
	old := p.Name
	if err := a.Store.RenamePlan(p.ID, n); err != nil {
		return nil, "", err
	}
	p.Name = n
	return p, old, nil
}

// RemovePlan deletes a plan with its workspace, undo stack, versions and staging list;
// the last plan of an environment cannot be removed. When the current plan goes, the first
// remaining one becomes current (returned as current).
func (a *App) RemovePlan(name string) (removed, current *store.Plan, err error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, nil, err
	}
	e := st.Ix.Env
	p, err := a.findPlan(name)
	if err != nil {
		return nil, nil, err
	}
	ps, err := a.Store.Plans()
	if err != nil {
		return nil, nil, err
	}
	if len(ps) <= 1 {
		return nil, nil, fmt.Errorf("plan:%s is the only plan; it cannot be removed", p.Name)
	}
	if err := a.Store.DeletePlan(p.ID); err != nil {
		return nil, nil, err
	}
	if a.OpenPlan == p.ID {
		a.OpenPlan = 0
	}
	current, err = a.Store.CurrentPlan(e.ID)
	return p, current, err
}
