package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/order"
	"github.com/ToumanLin/RimArcho/internal/rules"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// LoadoutState is the current plan's workspace with everything needed to interpret it.
type LoadoutState struct {
	Ix       *Index
	Plan     *store.Plan // the environment's current plan (07 §1.1)
	Reg      *loadout.Registry
	WS       *store.Workspace
	List     loadout.List
	Latest   *store.Version // newest version: what ModsConfig.xml holds as far as RimArcho knows
	File     *loadout.ModsConfig
	FilePath string
	FileErr  error
	Notices  []string // what Sync did (import, external change, conflict)
	Aliases  map[string][]store.Alias
	// MM overrides the game's major.minor for analysis (--game-version, 05 §3).
	MM string

	cands  []loadout.Candidate // what Reg was built from, deployed instances included
	deploy map[string]bool     // instances linked into Mods/ by RimArcho (09 §3)
	// localize reads another game's own entries by packageId (nil: as stored).
	localize func(loadout.List) loadout.List
}

// readList is a stored list as this game reads it: another game's Core, DLCs and Mods/
// folders become their packageIds (lists are shared by every game, 07 §1.1), then every
// packageId this game has resolves to its instance.
func (st *LoadoutState) readList(entries string) loadout.List {
	l := loadout.Decode(entries)
	if st.localize != nil {
		l = st.localize(l)
	}
	return st.Reg.Heal(l)
}

// localizer reads the entries of a list that are another game's own instances as this
// game sees them: this game's own Mods/ copy of the package when it has one; else the
// instance at the same folder (a link it made, or the other game's Mods/ copy in ix,
// linked in on save); else a missing entry naming the packageId, so Heal can find this
// game's copy (Core and DLCs always go this way). ix may be nil.
func (a *App) localizer(envID string, reg *loadout.Registry, ix *Index) func(loadout.List) loadout.List {
	return func(l loadout.List) loadout.List {
		var ids []string
		own := false
		for _, e := range l {
			if e.Instance != "" && !reg.Known(e.Instance) && (ix == nil || ix.ByID[e.Instance] == nil) {
				ids = append(ids, e.Instance)
			}
			if ix != nil && e.Instance != "" {
				if m := ix.ByID[e.Instance]; m != nil && ix.Elsewhere(m) != m {
					own = true
				}
			}
		}
		if own { // another game's Mods/ copy of a mod this game has in its own Mods/
			l = l.Clone()
			for i, e := range l {
				if m := ix.ByID[e.Instance]; m != nil {
					if o := ix.Elsewhere(m); o != m && l.Index(o.ID) < 0 {
						l[i] = loadout.Entry{Instance: o.ID}
					}
				}
			}
		}
		if len(ids) == 0 {
			return l
		}
		db := a.Store
		if ix != nil && ix.store != nil {
			db = ix.store
		}
		fs, err := db.ForeignInstances(envID, ids)
		if err != nil || len(fs) == 0 {
			return l
		}
		var byReal map[string]string
		if ix != nil {
			byReal = map[string]string{}
			for _, m := range ix.Mods {
				if m.Status == identity.Live && m.RealPath != "" && m.Kind != source.Official {
					byReal[env.PathKey(m.RealPath)] = m.ID
				}
			}
		}
		out := l.Clone()
		for i, e := range out {
			f, ok := fs[e.Instance]
			if !ok {
				continue
			}
			if id := byReal[env.PathKey(f.RealPath)]; id != "" && !f.Official && f.RealPath != "" {
				if o := ix.Elsewhere(ix.ByID[id]); o != nil && out.Index(o.ID) < 0 {
					out[i] = loadout.Entry{Instance: o.ID}
				}
			} else if f.PackageID != "" {
				out[i] = loadout.Entry{Missing: f.PackageID}
			}
		}
		return out
	}
}

// GameMM is the environment's game major.minor version.
func (st *LoadoutState) GameMM() string {
	if st.MM != "" {
		return st.MM
	}
	return env.MajorMinor(st.Ix.Env.GameVersion)
}

// Baseline is the list of the newest version (empty if none).
func (st *LoadoutState) Baseline() loadout.List {
	if st.Latest == nil {
		return nil
	}
	return st.readList(st.Latest.Entries)
}

// Changes is the diff from the newest version to the workspace.
func (st *LoadoutState) Changes() []loadout.Change {
	return loadout.Diff(st.Baseline(), st.List, st.LineageOf)
}

// LineageOf maps an instance to its lineage.
func (st *LoadoutState) LineageOf(id string) string {
	if m := st.Ix.ByID[id]; m != nil {
		return m.LineageID
	}
	return ""
}

// Position returns the 1-based position of an instance in the workspace (0 = disabled).
func (st *LoadoutState) Position(id string) int { return st.List.Index(id) + 1 }

// FileVersionMismatch reports the ModsConfig.xml version when its major.minor differs from
// the game's: the game resets such a list on load (07 §4.1 rule 6).
func (st *LoadoutState) FileVersionMismatch() string {
	if st.File == nil {
		return ""
	}
	fv, gv := env.MajorMinor(st.File.Version), st.GameMM()
	if fv != "" && gv != "" && fv != gv {
		return st.File.Version
	}
	return ""
}

// Candidates derives the game-loadable instances (04 §1.2, 07 §4.2): live, subscribed,
// not unloadable, and located in Data/, Mods/ or the workshop — directly or via an alias.
// RimArcho's own Mods/ links do not count here; deployed instances are added separately.
func Candidates(ix *Index, aliases map[string][]store.Alias) []loadout.Candidate {
	visible := gameVisible
	kindOf := map[string]source.Kind{}
	for _, s := range ix.Sources {
		kindOf[s.ID] = s.Kind
	}
	var out []loadout.Candidate
	for _, m := range ix.Mods {
		if m.Status != identity.Live || m.Residue || m.Unloadable != "" {
			continue
		}
		c := loadout.Candidate{ID: m.ID, PackageID: m.PackageID, Lineage: m.LineageID}
		switch {
		case visible(m.Kind):
			c.Kind, c.Folder = m.Kind, m.Rel
		default:
			for _, al := range aliases[m.ID] {
				if k := kindOf[al.SourceID]; visible(k) && !managedLink(al.Rel) {
					c.Kind, c.Folder = k, al.Rel
					break
				}
			}
		}
		if c.Kind != "" {
			out = append(out, c)
		}
	}
	return out
}

// Loadout loads the workspace, first syncing it with ModsConfig.xml (07 §4.3). Like Index,
// the result is cached until the database or ModsConfig.xml (size, mtime) changes; a cached
// state carries no notices (they were reported when the sync happened).
func (a *App) Loadout() (*LoadoutState, error) {
	ix, err := a.Index()
	if err != nil {
		return nil, err
	}
	key := a.Store.ChangeKey() + "|" + ix.Env.ID + fmt.Sprintf("|open=%d|%s", a.OpenPlan, a.PlanName)
	if ix.Env.UserDataRoot != "" {
		if fi, err := os.Stat(env.ModsConfigPath(ix.Env.UserDataRoot)); err == nil {
			key += fmt.Sprintf("|%d|%d", fi.Size(), fi.ModTime().UnixNano())
		}
	}
	if a.stCache != nil && a.stKey == key && a.stCache.Ix == ix {
		c := *a.stCache
		c.MM = a.GameVersion
		c.Notices = nil
		c.List = c.List.Clone()
		ws := *c.WS
		c.WS = &ws
		a.Store.MM = c.GameMM()
		return &c, nil
	}
	st, err := a.loadLoadout()
	if err != nil {
		return nil, err
	}
	st.MM = a.GameVersion
	a.Store.MM = st.GameMM() // fact queries read this game version's analysis
	// Syncing may have written; key the cache on the state after it.
	key = a.Store.ChangeKey() + key[strings.Index(key, "|"):]
	if ix2, err := a.Index(); err == nil && ix2 == ix {
		a.stCache, a.stKey = st, key
	} else {
		a.stCache = nil
	}
	return st, nil
}

// LoadoutKey validates the cached state, including ModsConfig.xml changes. Views
// can retain derived rows without hiding an external edit to the game file.
func (a *App) LoadoutKey() (string, error) {
	if _, err := a.Loadout(); err != nil {
		return "", err
	}
	return a.stKey + "|" + a.GameVersion, nil
}

func (a *App) loadLoadout() (*LoadoutState, error) {
	if a.workspaceSnapshot {
		st, err := a.storedLoadout()
		if err != nil {
			return nil, err
		}
		if st != nil {
			mcs, err := a.Store.ModsConfigStateAt(st.Ix.Env.UserDataRoot)
			if err != nil {
				return nil, err
			}
			if st.File == nil || bytes.Equal(loadout.Hash(st.File.Raw), mcs.Seen) {
				return st, nil
			}
			// External changes still import through guarded sync. Ordinary page
			// reads of persisted state never take a writer or launch lock.
		}
		// First use has no persisted list yet; initialise it through normal sync.
	}
	st := &LoadoutState{}
	if err := a.sync(st); err != nil {
		return nil, err
	}
	return st, nil
}

func (a *App) storedLoadout() (*LoadoutState, error) {
	now := a.Now().UTC()
	ix, err := a.Index()
	if err != nil {
		return nil, err
	}
	plan, err := a.planFor(ix.Env.ID)
	if err != nil || plan == nil {
		return nil, err
	}
	reg, aliases, all, deploy, err := a.registry(ix)
	if err != nil {
		return nil, err
	}
	ws, err := a.Store.Workspace(plan.ID)
	if err != nil {
		return nil, err
	}
	latest, err := a.Store.LatestVersion(plan.ID)
	if err != nil {
		return nil, err
	}
	if ws == nil {
		ws = &store.Workspace{PlanID: plan.ID, Entries: loadout.List(nil).Encode(), Updated: now}
		if latest != nil {
			ws.Entries, ws.OriginVersion, ws.OriginKind = latest.Entries, latest.N, latest.Source
		}
	}
	st := &LoadoutState{Ix: ix, Plan: plan, Reg: reg, Aliases: aliases, cands: all, deploy: deploy, WS: ws, Latest: latest, localize: a.localizer(ix.Env.ID, reg, ix)}
	st.List = st.readList(ws.Entries)
	if ix.Env.UserDataRoot != "" {
		st.FilePath = env.ModsConfigPath(ix.Env.UserDataRoot)
		st.File, st.FileErr = loadout.ReadModsConfig(st.FilePath)
		if errors.Is(st.FileErr, os.ErrNotExist) {
			st.FileErr = nil
		}
	}
	return st, nil
}

// registry builds the environment's instance registry (07 §4.2): the game-loadable
// instances plus the ones RimArcho deploys into Mods/.
func (a *App) registry(ix *Index) (*loadout.Registry, map[string][]store.Alias, []loadout.Candidate, map[string]bool, error) {
	aliases, err := a.Store.AllAliases(ix.Env.ID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	deploy, err := a.deploySet(ix, aliases)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	all := Candidates(ix, aliases)
	for _, m := range ix.Mods { // index order keeps the registry deterministic
		if deploy[m.ID] {
			all = append(all[:len(all):len(all)], deployCandidate(m))
		}
	}
	reg := sharedRegistry(all)
	return reg, aliases, reg.Given(), deploy, nil
}

func (a *App) sync(st *LoadoutState) error {
	// Readers may spend time measuring before they need a write. Do not hold the
	// handoff lock across that work. Rebuild the registry and reread ModsConfig
	// under the guard, so a read overtaken by replacement cannot sync old IDs back.
	now := a.Now().UTC()
	unlock, err := a.lockLaunch()
	if err != nil {
		return err
	}
	defer unlock()
	ix, err := a.Index()
	if err != nil {
		return err
	}
	reg, aliases, all, deploy, err := a.registry(ix)
	if err != nil {
		return err
	}
	*st = LoadoutState{Ix: ix, Reg: reg, Aliases: aliases, cands: all, deploy: deploy, localize: a.localizer(ix.Env.ID, reg, ix)}
	return a.syncLocked(st, now)
}

func (a *App) syncLocked(st *LoadoutState, now time.Time) error {
	e := st.Ix.Env
	if e.UserDataRoot != "" {
		st.FilePath = env.ModsConfigPath(e.UserDataRoot)
		st.File, st.FileErr = loadout.ReadModsConfig(st.FilePath)
		if errors.Is(st.FileErr, os.ErrNotExist) {
			st.FileErr = nil
		}
	}
	var h []byte
	var fileList loadout.List
	if st.File != nil {
		h = loadout.Hash(st.File.Raw)
		fileList, _ = st.Reg.FromTokens(st.File.Active)
	}
	mcs, err := a.Store.ModsConfigStateAt(e.UserDataRoot)
	if err != nil {
		return err
	}
	plan, err := a.planFor(e.ID)
	if err != nil {
		return err
	}
	if plan == nil {
		if plan, err = a.adoptPlan(st, mcs, h); err != nil {
			return err
		}
	}
	if plan == nil {
		// First use (07 §1.4): ModsConfig.xml becomes a new list, current in this game.
		name := a.firstPlanName(e)
		p, err := a.Store.CreatePlan(e.ID, name, now)
		if err != nil {
			// Another process may have just made it.
			if p, _ = a.Store.PlanByName(name); p == nil {
				return err
			}
			plan = p
		} else {
			plan = p
			if err := a.Store.SetCurrentPlan(e.ID, plan.ID); err != nil {
				return err
			}
			v := &store.Version{Created: now, Source: "import", Entries: fileList.Encode()}
			if st.File != nil {
				v.GameVersion = st.File.Version
				v.Note = "ModsConfig.xml"
			} else {
				v.Note = "no ModsConfig.xml"
			}
			n, err := a.Store.AddVersion(plan.ID, v)
			if err != nil {
				return err
			}
			ws := &store.Workspace{PlanID: plan.ID, Entries: fileList.Encode(), OriginVersion: n, OriginKind: "import", Updated: now}
			if err := a.Store.SetWorkspace(ws); err != nil {
				return err
			}
			// The file is not marked as seen here: the check below finds it equal to this
			// import, or — when another plan sharing the folder wrote it and it changed
			// since — an external change of that plan (07 §4.3).
			st.Notices = append(st.Notices, fmt.Sprintf("imported ModsConfig.xml as v%d: %d entries", n, len(fileList)))
		}
	}
	st.Plan = plan
	if st.File != nil && !bytes.Equal(h, mcs.Seen) {
		if bytes.Equal(h, mcs.Written) {
			mcs.Seen = h
		} else if err := a.attributeExternal(st, mcs, h, now); err != nil {
			return err
		}
		if err := a.Store.SetModsConfigStateAt(e.UserDataRoot, mcs); err != nil {
			return err
		}
	}
	ws, err := a.planWorkspace(plan)
	if err != nil {
		return err
	}
	st.WS = ws
	st.List = st.readList(ws.Entries)
	st.Latest, err = a.Store.LatestVersion(plan.ID)
	return err
}

// adoptPlan picks the current list of a game that has none yet, from the lists other
// games already have (they are shared, 07 §1.4): the list that last wrote this game's
// ModsConfig.xml when the file is still what it wrote, else — with no ModsConfig.xml —
// the most recently changed list. nil: import the file as a new list.
func (a *App) adoptPlan(st *LoadoutState, mcs *store.ModsConfigState, h []byte) (*store.Plan, error) {
	e := st.Ix.Env
	var p *store.Plan
	switch {
	case st.File != nil:
		if mcs.WriterPlan == 0 || !bytes.Equal(h, mcs.Written) {
			return nil, nil
		}
		var err error
		if p, err = a.Store.PlanByID(mcs.WriterPlan); err != nil || p == nil {
			return nil, err
		}
	default:
		ps, err := a.Store.Plans()
		if err != nil {
			return nil, err
		}
		for _, q := range ps {
			if p == nil || q.Updated.After(p.Updated) {
				p = q
			}
		}
		if p == nil {
			return nil, nil
		}
	}
	return p, a.Store.SetCurrentPlan(e.ID, p.ID)
}

// firstPlanName names the list imported on a game's first use: "default", else the
// game's name, else that name numbered.
func (a *App) firstPlanName(e *env.Environment) string {
	taken := func(n string) bool { p, _ := a.Store.PlanByName(n); return p != nil && strings.EqualFold(p.Name, n) }
	if !taken(store.DefaultPlanName) {
		return store.DefaultPlanName
	}
	return store.UniquePlanName(e.Name, taken)
}

// planWorkspace returns a plan's workspace; a plan without one (only versions) starts
// from its newest version.
func (a *App) planWorkspace(p *store.Plan) (*store.Workspace, error) {
	ws, err := a.Store.Workspace(p.ID)
	if err != nil || ws != nil {
		return ws, err
	}
	ws = &store.Workspace{PlanID: p.ID, Updated: a.Now().UTC()}
	latest, err := a.Store.LatestVersion(p.ID)
	if err != nil {
		return nil, err
	}
	if latest != nil {
		ws.Entries, ws.OriginVersion, ws.OriginKind = latest.Entries, latest.N, latest.Source
	} else {
		ws.Entries = loadout.List(nil).Encode()
	}
	return ws, a.Store.SetWorkspace(ws)
}

// EditResult describes a workspace edit.
type EditResult struct {
	State   *LoadoutState // after the edit
	Before  loadout.List
	Changes []loadout.Change // before → after
	Notes   []string
}

// edit applies f to the workspace, recording an undo step.
func (a *App) edit(op string, f func(st *LoadoutState, l loadout.List) (loadout.List, []string, error)) (*EditResult, error) {
	return a.editStaged(op, nil, f)
}

func (a *App) editStaged(op string, staged *string, f func(st *LoadoutState, l loadout.List) (loadout.List, []string, error)) (*EditResult, error) {
	return a.editOrigin(op, staged, nil, f)
}

// editOrigin is editStaged with origin, when set, adjusting the workspace's origin in the
// same transaction as the edit.
func (a *App) editOrigin(op string, staged *string, origin func(ws *store.Workspace), f func(st *LoadoutState, l loadout.List) (loadout.List, []string, error)) (*EditResult, error) {
	unlock, err := a.lockLaunch()
	if err != nil {
		return nil, err
	}
	defer unlock()

	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	before := st.List.Clone()
	after, notes, err := f(st, st.List.Clone())
	if err != nil {
		return nil, err
	}
	r := &EditResult{Before: before, Notes: notes}
	if !after.Equal(before) || staged != nil {
		ws := *st.WS
		ws.Entries, ws.Updated = after.Encode(), a.Now().UTC()
		if ws.OriginKind == "checkout" {
			ws.OriginKind = "edited"
		}
		if origin != nil {
			origin(&ws)
		}
		// History keeps stored identities; before is localized to the current game.
		if err := a.Store.EditWorkspace(&ws, st.WS.Entries, op, staged); err != nil {
			return nil, err
		}
		st.WS, st.List = &ws, after
	}

	r.State = st
	r.Changes = loadout.Diff(before, st.List, st.LineageOf)
	return r, nil
}

// Placement says where enable/move put an entry.
type Placement struct {
	At     int    // 1-based position; 0 = unset
	Before string // reference
	After  string
	Delta  int // move only: -n up, +n down
}

func (p Placement) set() bool { return p.At > 0 || p.Before != "" || p.After != "" || p.Delta != 0 }

// target computes the insertion index (0-based, in l without the moving entry).
func (p Placement) target(st *LoadoutState, l loadout.List, cur int) (int, error) {
	anchor := func(ref string) (int, error) {
		m, err := st.Ix.ResolveOne(ref)
		if err != nil {
			return 0, err
		}
		i := l.Index(m.ID)
		if i < 0 {
			return 0, fmt.Errorf("%s is not enabled", st.Ix.Ref(m.ID))
		}
		return i, nil
	}
	switch {
	case p.At > 0:
		return min(p.At-1, len(l)), nil
	case p.Before != "":
		return anchor(p.Before)
	case p.After != "":
		i, err := anchor(p.After)
		return i + 1, err
	case p.Delta != 0:
		return max(0, min(cur+p.Delta, len(l))), nil
	}
	return len(l), nil
}

func remove(l loadout.List, i int) loadout.List { return append(l[:i:i], l[i+1:]...) }

func insert(l loadout.List, i int, e loadout.Entry) loadout.List {
	l = append(l, loadout.Entry{})
	copy(l[i+1:], l[i:])
	l[i] = e
	return l
}

// whyNotLoadable explains why an instance cannot go into a Loadout ("" if it can).
func whyNotLoadable(st *LoadoutState, m *store.Mod) string {
	switch {
	case m.Status != identity.Live:
		return "it is a tombstone (no longer on disk)"
	case m.Unloadable != "":
		return "unloadable: " + m.Unloadable + " (About.xml was renamed; the game cannot load it)"
	case m.Residue:
		return "residue: the workshop folder is no longer subscribed, the game ignores it"
	case !st.Reg.Known(m.ID) && !st.Deployable(m):
		return fmt.Sprintf("the game does not load mods from src:%s; link or copy it into Mods/", m.SourceLabel)
	case !st.regWith(m.ID).Loadable(m.ID):
		return "pkg.duplicate-installed: another copy with packageId " + m.PackageID + " in the same place wins (the game ignores this one)"
	}
	return ""
}

// Enable adds instances to the workspace (07 §2.1).
func (a *App) Enable(refs []string, p Placement) (*EditResult, error) {
	return a.edit("enable", func(st *LoadoutState, l loadout.List) (loadout.List, []string, error) {
		return a.enableList(st, l, refs, p)
	})
}

func (a *App) enableList(st *LoadoutState, l loadout.List, refs []string, p Placement) (loadout.List, []string, error) {
	var notes []string
	at := -1
	placed := 0 // without a position: appended here, then placed by the constraints
	for _, ref := range refs {
		m, err := st.Ix.ResolveOne(ref)
		if err != nil {
			return nil, nil, err
		}
		if why := whyNotLoadable(st, m); why != "" {
			return nil, nil, fmt.Errorf("cannot enable %s: %s", st.Ix.Ref(m.ID), why)
		}
		if l.Index(m.ID) >= 0 {
			notes = append(notes, st.Ix.Ref(m.ID)+" is already enabled")
			continue
		}
		for _, o := range st.regWith(m.ID).SamePackage(m.ID) {
			if l.Index(o.ID) >= 0 {
				return nil, nil, fmt.Errorf("cannot enable %s: %s with the same packageId %s is enabled; the game keeps only one (archo swap %s %s)",
					st.Ix.Ref(m.ID), st.Ix.Ref(o.ID), m.PackageID, st.Ix.Ref(o.ID), st.Ix.Ref(m.ID))
			}
		}
		// A missing entry for this package is replaced in place.
		idx := -1
		for i, e := range l {
			if e.Missing != "" && st.Reg.Resolve(e.Missing).Instance == "" && strings.TrimSuffix(e.Missing, "_steam") == m.PackageID {
				idx = i
			}
		}
		if idx >= 0 && !p.set() {
			l[idx] = loadout.Entry{Instance: m.ID}
			continue
		}
		if !p.set() {
			l = append(l, loadout.Entry{Instance: m.ID})
			placed++
			continue
		}
		if at < 0 {
			if at, err = p.target(st, l, len(l)); err != nil {
				return nil, nil, err
			}
		}
		l = insert(l, at, loadout.Entry{Instance: m.ID})
		at++
	}
	if placed > 0 {
		// The latest position the sort constraints allow (07 §2.1, order.Place).
		in, err := a.SortInput(st, l)
		if err != nil {
			return nil, nil, err
		}
		out := make(loadout.List, 0, len(l))
		for _, i := range order.Place(in, len(l)-placed) {
			out = append(out, l[i])
		}
		l = out
	}
	return l, notes, nil
}

// Disable removes instances (or missing entries named by a packageId) from the workspace.
func (a *App) Disable(refs []string) (*EditResult, error) {
	return a.edit("disable", func(st *LoadoutState, l loadout.List) (loadout.List, []string, error) {
		var notes []string
		for _, ref := range refs {
			ms, err := st.Ix.Resolve(ref)
			pkg, isPkg := PackageRef(ref)
			if err != nil && isPkg {
				ms, err = nil, nil
			}
			if err != nil {
				return nil, nil, err
			}
			found := false
			for _, m := range ms {
				if i := l.Index(m.ID); i >= 0 {
					l, found = remove(l, i), true
				}
			}
			if isPkg && pkg != "" {
				for i := len(l) - 1; i >= 0; i-- {
					if l[i].Missing == pkg || l[i].Missing == pkg+"_steam" {
						l, found = remove(l, i), true
					}
				}
			}
			if !found {
				notes = append(notes, ref+" was not enabled")
			}
		}
		return l, notes, nil
	})
}

// Move repositions one enabled instance.
func (a *App) Move(ref string, p Placement) (*EditResult, error) {
	if !p.set() {
		return nil, errors.New("say where: --to <n>, --before <ref>, --after <ref>, --up or --down")
	}
	return a.edit("move", func(st *LoadoutState, l loadout.List) (loadout.List, []string, error) {
		m, err := st.Ix.ResolveOne(ref)
		if err != nil {
			return nil, nil, err
		}
		cur := l.Index(m.ID)
		if cur < 0 {
			return nil, nil, fmt.Errorf("%s is not enabled (archo enable %s)", st.Ix.Ref(m.ID), st.Ix.Ref(m.ID))
		}
		rest := remove(l.Clone(), cur)
		at, err := p.target(st, rest, cur)
		if err != nil {
			return nil, nil, err
		}
		return insert(rest, at, l[cur]), nil, nil
	})
}

// Swap replaces an enabled instance by another instance of the same lineage (07 §2.1).
func (a *App) Swap(ref, with string) (*EditResult, error) {
	return a.edit("swap", func(st *LoadoutState, l loadout.List) (loadout.List, []string, error) {
		m, err := st.Ix.ResolveOne(ref)
		if err != nil {
			return nil, nil, err
		}
		i := l.Index(m.ID)
		var other *store.Mod
		if with != "" {
			if other, err = st.Ix.ResolveOne(with); err != nil {
				return nil, nil, err
			}
		}
		if i < 0 && other != nil && l.Index(other.ID) >= 0 {
			m, other, i = other, m, l.Index(other.ID) // either order works
		}
		if i < 0 {
			return nil, nil, fmt.Errorf("%s is not enabled", st.Ix.Ref(m.ID))
		}
		if other == nil {
			var cands []*store.Mod
			for _, o := range st.Ix.ByLineage[m.LineageID] {
				if o.ID != m.ID && whyNotLoadable(st, o) == "" {
					cands = append(cands, o)
				}
			}
			switch len(cands) {
			case 0:
				return nil, nil, fmt.Errorf("%s has no other loadable instance in %s", st.Ix.Ref(m.ID), st.Ix.LRef(m.LineageID))
			case 1:
				other = cands[0]
			default:
				return nil, nil, &Ambiguous{Ref: st.Ix.LRef(m.LineageID), Mods: cands}
			}
		}
		if other.LineageID != m.LineageID {
			return nil, nil, fmt.Errorf("%s and %s are not in the same lineage (archo lineage merge, or disable + enable)", st.Ix.Ref(m.ID), st.Ix.Ref(other.ID))
		}
		if why := swapBlocker(st, other, m); why != "" {
			return nil, nil, fmt.Errorf("cannot swap in %s: %s", st.Ix.Ref(other.ID), why)
		}
		l[i] = loadout.Entry{Instance: other.ID}
		return l, nil, nil
	})
}

// SortResult is the outcome of Sort.
type SortResult struct {
	SortNotices
	State   *LoadoutState
	Input   order.Input
	Result  order.Result
	Sorted  loadout.List
	Changes []loadout.Change
	Applied bool
	// MonitorSkipped says why the sort did not add Archo Monitor although monitor.default
	// is on and it is installed (no Harmony, Mods/ cannot hold links); "" otherwise.
	MonitorSkipped string `json:"monitorSkipped,omitempty"`
}

// sortMonitor puts the newest installed Archo Monitor right after Harmony when
// monitor.default is on and the list has none (07 §1.5, 08 §1.1): a sort applies the setting
// every time, also after the player took the monitor out. why says why it could not go in.
func (a *App) sortMonitor(st *LoadoutState, l loadout.List) (loadout.List, string) {
	if !a.MonitorDefault() {
		return l, ""
	}
	for _, e := range l {
		if IsMonitor(st.Ix.ByID[e.Instance]) {
			return l, ""
		}
	}
	ms := monitorInstances(st.Ix)
	if len(ms) == 0 {
		return l, ""
	}
	at, why := a.monitorSlot(st.Ix, l, ms[0].Mod)
	switch {
	case why == "":
		return insert(l.Clone(), at, loadout.Entry{Instance: ms[0].Mod.ID}), ""
	case at < 0 && strings.HasPrefix(why, "no Harmony"):
		return l, MonitorNoHarmony
	default:
		return l, MonitorNoLinks
	}
}

// Why a sort could not add Archo Monitor (SortResult.MonitorSkipped; view translates them).
const (
	MonitorNoHarmony = "the list has no Harmony, which it needs"
	MonitorNoLinks   = "Mods/ is on a FAT or exFAT drive, which cannot hold the link it needs"
)

// Community loads community rules from <home>/cache/communityRules.json (08 §2.3).
func (a *App) Community() order.Community {
	c, err := loadCommunity(filepath.Join(a.Home, "cache", "communityRules.json"))
	if err != nil {
		return nil
	}
	return c
}

// SortInput builds the constraint input for a list.
func (a *App) SortInput(st *LoadoutState, l loadout.List) (order.Input, error) {
	in := order.Input{GameMM: st.GameMM(), Community: a.Community()}
	var enabled []*store.Mod
	for _, e := range l {
		if m := st.Ix.ByID[e.Instance]; m != nil {
			enabled = append(enabled, m)
		}
	}
	if err := a.Store.LoadDecls(enabled); err != nil {
		return in, err
	}
	for _, e := range l {
		if e.Instance == "" {
			in.Nodes = append(in.Nodes, order.Node{PackageID: strings.TrimSuffix(e.Missing, "_steam")})
			continue
		}
		m := st.Ix.ByID[e.Instance]
		if m == nil {
			in.Nodes = append(in.Nodes, order.Node{Instance: e.Instance})
			continue
		}
		d := m.Decl()
		in.Nodes = append(in.Nodes, order.Node{Instance: m.ID, PackageID: m.PackageID, Lineage: m.LineageID, Decl: &d})
	}
	in.Replacements = a.sortReplacements(st, in.Nodes, enabled)
	rules, err := a.Store.Rules(st.Ix.Env.ID)
	if err != nil {
		return in, err
	}
	for _, r := range rules {
		in.Rules = append(in.Rules, order.Rule{ID: r.ID, Kind: r.Kind, A: r.A, B: r.B, Target: r.Target})
	}
	return in, nil
}

// sortReplacements: for each required package no list entry has, the enabled mods the
// player's replaces rules or Use This Instead name as its replacement (order.Input.Replacements,
// 08 §1.1).
func (a *App) sortReplacements(st *LoadoutState, nodes []order.Node, enabled []*store.Mod) map[string][]string {
	inList := map[string]bool{}
	for _, nd := range nodes {
		inList[dependencyPackage(nd.PackageID)] = true
	}
	activeByPkg := map[string][]*store.Mod{}
	for _, m := range enabled {
		p := dependencyPackage(m.PackageID)
		activeByPkg[p] = append(activeByPkg[p], m)
	}
	ix := a.newDependencyIndex(st, activeByPkg)
	var out map[string][]string
	for _, nd := range nodes {
		if nd.Decl == nil {
			continue
		}
		for _, dep := range append(toDeps(nd.Decl.Dependencies), toDeps(nd.Decl.DependenciesByVersion["v"+st.GameMM()])...) {
			p := dependencyPackage(dep.pkg)
			if p == "" || inList[p] || out[p] != nil {
				continue
			}
			dep.alts = nil // the sort reads alternatives from the declaration itself
			if ms, how := ix.standIns(dep); how == StandInReplacement || how == StandInRule {
				if out == nil {
					out = map[string][]string{}
				}
				for _, m := range ms {
					out[p] = append(out[p], m.PackageID)
				}
			}
		}
	}
	return out
}

// Sort orders the workspace (08 §1); dryRun only computes.
func (a *App) Sort(dryRun bool) (*SortResult, error) {
	var out *SortResult

	if dryRun {
		st, err := a.Loadout()
		if err != nil {
			return nil, err
		}
		out, err = a.sortList(st, st.List)
		if err != nil {
			return nil, err
		}
		out.State = st
		return out, nil
	}
	r, err := a.edit("sort", func(st *LoadoutState, l loadout.List) (loadout.List, []string, error) {
		var err error
		out, err = a.sortList(st, l)
		if err != nil {
			return nil, nil, err
		}
		return out.Sorted, nil, nil
	})
	if err != nil {
		return nil, err
	}
	out.State, out.Applied = r.State, true
	return out, nil
}

func (a *App) sortList(st *LoadoutState, l loadout.List) (*SortResult, error) {
	before := l.Clone()
	// Sorting also restores the installed, loadable base game. DLCs remain
	// the player's choice; missing Core on disk remains a missing entry.
	core := st.Reg.Resolve("ludeon.rimworld").Instance
	if core != "" && st.Reg.Loadable(core) && l.Index(core) < 0 {
		l = append(l.Clone(), loadout.Entry{Instance: core})
	}
	l, monitorSkipped := a.sortMonitor(st, l)
	in, err := a.SortInput(st, l)
	if err != nil {
		return nil, err
	}
	r := order.Sort(in)
	sorted := make(loadout.List, 0, len(l))
	for _, i := range r.Order {
		sorted = append(sorted, l[i])
	}
	out := &SortResult{Input: in, Result: r, Sorted: sorted, Changes: loadout.Diff(before, sorted, st.LineageOf), MonitorSkipped: monitorSkipped}
	state := *st
	state.List = sorted
	out.MissingDependencies, err = a.MissingDependencies(&state)
	if err != nil {
		return nil, err
	}
	out.MissingMods = a.MissingMods(&state)
	out.Replacements = a.ReplacementSuggestions(&state)
	out.Cycles = sortCycles(&state, in, r)
	return out, nil
}

// UndoRedo steps the workspace back (undo) or forward (redo).
func (a *App) UndoRedo(redo bool) (*EditResult, string, error) {
	unlock, err := a.lockLaunch()
	if err != nil {
		return nil, "", err
	}
	defer unlock()

	st, err := a.Loadout()
	if err != nil {
		return nil, "", err
	}
	from, to := "undo", "redo"
	if redo {
		from, to = "redo", "undo"
	}
	step, err := a.Store.PeekStep(st.Plan.ID, from)
	if err != nil {
		return nil, "", err
	}
	if step == nil {
		return nil, "", fmt.Errorf("nothing to %s", from)
	}
	before := st.List
	ws := *st.WS
	ws.Entries, ws.Updated = step.Entries, a.Now().UTC()
	if ws.OriginKind == "checkout" {
		// The list is no longer the checked-out version: saving it is not a restore.
		ws.OriginKind = "edited"
	}
	if err := a.Store.RestoreStep(&ws, st.WS.Entries, from, to, *step); err != nil {
		return nil, "", err
	}
	st.WS, st.List = &ws, st.readList(step.Entries)
	return &EditResult{State: st, Before: before, Changes: loadout.Diff(before, st.List, st.LineageOf)}, step.Op, nil
}

// SaveResult describes a write to the game: archo save, or the writing part of a launch
// (07 §4.1, 09 §2).
type SaveResult struct {
	// Moved: plain Mods/ folders moved into the lock library first (--move-duplicates); in a
	// dry run, the ones that would move.
	Moved      []DuplicateMove
	State      *LoadoutState
	Path       string
	Backup     string
	Version    int // new version number (0 on dry run, or when Unchanged)
	Source     string
	OldVersion string // ModsConfig.xml <version> before
	NewVersion string
	Tokens     []string
	Changes    []loadout.Change // newest version → saved list
	DryRun     bool
	Restores   int // source "undo": the version the workspace was checked out from
	Missing    int
	// Unchanged: a launch whose list equals the newest version records no new version
	// (09 §2 step 7).
	Unchanged bool
	Links     []LinkChange // Mods/ links created or removed first (09 §3)
	// Settings: Mod settings copied to the file names the Mods' folders now give them
	// (09 §3); in a dry run, the ones that would be.
	Settings []SettingsCarry
	// Launch is the game command (launch only).
	Launch *LaunchPlan
	// Running is a game that reads this ModsConfig.xml and is running now (save only): it
	// reads the new list on its next start.
	Running *RunningGame
}

// Save writes the workspace to ModsConfig.xml without starting the game (09 §2 "writegame":
// steps 1–5 and 7). It is allowed while the game runs; the game reads the file on its next
// start.
func (a *App) Save(dryRun bool) (*SaveResult, error) { return a.SaveWith(dryRun, false) }

// SaveWith is Save that, with move, first moves plain Mods/ folders in the way of a deployed
// copy into the lock library (09 §3; --move-duplicates). The dry run lists those moves.
func (a *App) SaveWith(dryRun, move bool) (*SaveResult, error) {
	unlock, err := a.lockLaunch()
	if err != nil {
		return nil, err
	}
	defer unlock()

	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	r, err := a.writeGame(st, dryRun, false, move)
	if err != nil {
		return nil, err
	}
	if g, _ := a.RunningGame(); g != nil && g.What.UserData != "" &&
		env.PathKey(g.What.UserData) == env.PathKey(st.Ix.Env.UserDataRoot) {
		r.Running = g
	}
	return r, nil
}

// writeGame carries out 09 §2 steps 1 (the file checks), 3–5 and 7 for the synced state st
// (step 2 happened in Loadout): Mods/ links, ModsConfig.xml by the 07 §4.1 rules, and a
// version — source "launch" when launch is set (none when the list equals the newest
// version), else "user" or "undo". dryRun computes the same result and writes nothing.
// move: plain Mods/ folders in the way move into the lock library first (09 §3).
func (a *App) writeGame(st *LoadoutState, dryRun, launch, move bool, creditFrom ...*store.Mod) (*SaveResult, error) {
	e := st.Ix.Env
	if st.FilePath == "" {
		return nil, errors.New("the environment has no user data directory; set one with archo env set --user-data <dir>")
	}
	if st.FileErr != nil {
		return nil, fmt.Errorf("cannot read %s: %v (fix or remove the file first)", st.FilePath, st.FileErr)
	}
	// Never over an external change the player has not seen (05 I8, 07 §4.3).
	if c, err := a.writeBlocker(st); err != nil {
		return nil, err
	} else if c != nil {
		return nil, conflictError(st, c)
	}
	copies := a.ModsCopies(st.Ix)
	mods, links, dups, err := planLinks(st, copies)
	if err != nil {
		return nil, err
	}
	var moved []DuplicateMove
	if len(dups) > 0 {
		de := &DuplicatesError{Moves: a.planMoves(st, copies, dups), Fix: "archo save --move-duplicates"}
		if launch {
			de.Fix = "archo launch --move-duplicates"
		}
		if !move || !de.Movable() {
			if !de.Movable() {
				de.Fix = ""
			}
			return nil, de
		}
		if !dryRun {
			// Move them, then write from the state after the move (the instances moved).
			if _, err := a.doMoves(de.Moves); err != nil {
				return nil, err
			}
			st2, err := a.Loadout()
			if err != nil {
				return nil, err
			}
			r, err := a.writeGame(st2, false, launch, false)
			if r != nil {
				r.Moved = de.Moves
			}
			return r, err
		}
		// The dry run: what would be linked once they have moved.
		moved = de.Moves
		var rest []ModsCopy
		for _, c := range copies {
			gone := false
			for _, m := range de.Moves {
				gone = gone || m.Copy.Name == c.Name
			}
			if !gone {
				rest = append(rest, c)
			}
		}
		if mods, links, _, err = planLinks(st, rest); err != nil {
			return nil, err
		}
	}
	// A credit handoff projects the file already in the game, keeping unrelated
	// pending edits, links and the selected plan intact. Only the old copy's managed
	// deployment is detached. Registry tokens must describe the links that will remain.
	if len(creditFrom) > 0 {
		old := creditFrom[0]
		filtered := []LinkChange{}
		for _, c := range links {
			if c.Op == "remove" && c.Target != "" && samePath(c.Target, old.RealPath) {
				filtered = append(filtered, c)
			}
		}
		links = filtered
		base := Candidates(st.Ix, st.Aliases)
		seen := map[string]bool{}
		for _, c := range base {
			seen[c.ID] = true
		}
		for _, c := range st.cands {
			if c.ID != old.ID && !seen[c.ID] {
				base = append(base, c)
				seen[c.ID] = true
			}
		}
		st.Reg = loadout.NewRegistry(base)
	}
	r := &SaveResult{State: st, Path: st.FilePath, DryRun: dryRun, Changes: st.Changes(), Moved: moved}
	r.Links = links
	r.Tokens = st.Reg.Tokens(st.List, func(id string) string {
		if m := st.Ix.ByID[id]; m != nil {
			return m.PackageID
		}
		return ""
	})
	for _, en := range st.List {
		if en.Instance == "" {
			r.Missing++
		}
	}
	r.NewVersion = e.GameVersion
	var orig []byte
	if st.File != nil {
		orig, r.OldVersion = st.File.Raw, st.File.Version
		if r.NewVersion == "" {
			r.NewVersion = r.OldVersion
		}
	}
	var known []string
	for _, m := range st.Ix.Mods {
		if m.Kind == source.Official && m.PackageID != "ludeon.rimworld" && m.Status == identity.Live {
			known = append(known, m.PackageID)
		}
	}
	data, err := loadout.Render(orig, r.NewVersion, r.Tokens, known)
	if err != nil {
		return nil, err
	}
	switch {
	case launch:
		r.Source = "launch"
		r.Unchanged = st.Latest != nil && st.Latest.GameVersion == r.NewVersion && st.Baseline().Equal(st.List)
	case st.WS.OriginKind == "checkout":
		r.Source, r.Restores = "undo", st.WS.OriginVersion
	default:
		r.Source = "user"
	}
	settings := a.listSettings(st, nil, filepath.Dir(st.FilePath))
	if dryRun {
		r.Settings = planModSettings(settings)
		return r, nil
	}
	if err := a.checkUserData(filepath.Dir(st.FilePath)); err != nil { // own saves live in the data directory (03 §2)
		return nil, err
	}
	if len(links) > 0 {
		if err := a.checkRoots(mods); err != nil {
			return nil, err
		}
		if err := a.applyLinks(mods, links); err != nil {
			return nil, err
		}
	}
	if r.Backup, err = loadout.WriteFile(st.FilePath, data); err != nil {
		return nil, err
	}
	// Settings follow the Mod (09 §3): a Mod seen under a new folder gets its settings.
	r.Settings = carryModSettings(settings, nil)
	now := a.Now().UTC()
	n := 0
	if r.Unchanged {
		n = st.Latest.N
	} else {
		note := ""
		if r.Source == "undo" {
			note = fmt.Sprintf("restored v%d", st.WS.OriginVersion)
		}
		if n, err = a.Store.AddVersion(st.Plan.ID, &store.Version{Created: now, Source: r.Source, Entries: st.List.Encode(), GameVersion: r.NewVersion, Note: note}); err != nil {
			return nil, err
		}
		r.Version = n
	}
	// The write record is per user data folder and names the writer (07 §4.3).
	h := loadout.Hash(data)
	if err := a.Store.SetModsConfigStateAt(e.UserDataRoot, &store.ModsConfigState{Written: h, Seen: h, WrittenAt: now,
		WriterPlan: st.Plan.ID, WriterVersion: n, WriterEnv: e.ID}); err != nil {
		return nil, err
	}
	if len(creditFrom) > 0 {
		return r, nil
	}
	st.WS.OriginVersion, st.WS.OriginKind, st.WS.Conflict, st.WS.Updated = n, "save", 0, now
	if err := a.Store.SetWorkspace(st.WS); err != nil {
		return nil, err
	}
	// Writing an opened list to the game chooses it: it becomes the current one (07 §1.3).
	if a.Store.CurrentPlanID(e.ID) != st.Plan.ID {
		if err := a.Store.SetCurrentPlan(e.ID, st.Plan.ID); err != nil {
			return nil, err
		}
	}
	a.OpenPlan, a.PlanName = 0, ""
	return r, nil
}

// ParseVersionRef resolves "v3", "3" or "workspace" (0) in the environment's current plan.
func (a *App) ParseVersionRef(envID, ref string) (int, error) {
	p, err := a.planFor(envID)
	if err != nil {
		return 0, err
	}
	if p == nil {
		return 0, errors.New("the environment has no plan yet")
	}
	return a.parsePlanVersion(p, ref)
}

func (a *App) parsePlanVersion(p *store.Plan, ref string) (int, error) {
	r := strings.ToLower(strings.TrimSpace(ref))
	if r == "workspace" || r == "ws" || r == "current" {
		return 0, nil
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(r, "v")); err == nil && n > 0 {
		if v, _ := a.Store.Version(p.ID, n); v != nil {
			return n, nil
		}
		return 0, fmt.Errorf("no version v%d in plan:%s (archo loadout log)", n, p.Name)
	}
	return 0, fmt.Errorf("%q is not a version (v<n> or workspace; archo loadout log)", ref)
}

// ListAt returns the list of a version (0 = workspace).
func (a *App) ListAt(st *LoadoutState, n int) (loadout.List, error) {
	if n == 0 {
		return st.List, nil
	}
	v, err := a.Store.Version(st.Plan.ID, n)
	if err != nil || v == nil {
		return nil, fmt.Errorf("no version v%d", n)
	}
	return st.readList(v.Entries), nil
}

// Checkout loads a version into the workspace (it still has to be saved).
func (a *App) Checkout(ref string) (*EditResult, int, error) {
	unlock, err := a.lockLaunch()
	if err != nil {
		return nil, 0, err
	}
	defer unlock()

	var n int
	// The origin is written with the list, in one transaction.
	origin := func(ws *store.Workspace) {
		ws.OriginVersion, ws.OriginKind = n, "checkout"
		if ws.Conflict == n {
			ws.Conflict = 0
		}
	}
	r, err := a.editOrigin("checkout", nil, origin, func(st *LoadoutState, l loadout.List) (loadout.List, []string, error) {
		var err error
		if n, err = a.parsePlanVersion(st.Plan, ref); err != nil {
			return nil, nil, err
		}
		if n == 0 {
			return nil, nil, errors.New("checkout needs a saved version")
		}
		target, err := a.ListAt(st, n)
		return target, nil, err
	})
	if err != nil {
		return nil, 0, err
	}
	st := r.State
	if r.Before.Equal(st.List) {
		// Nothing to undo; the origin stays. A conflict about this version is resolved,
		// though: the list already is that version.
		if st.WS.Conflict == n {
			st.WS.Conflict = 0
			if err := a.Store.SetWorkspace(st.WS); err != nil {
				return nil, 0, err
			}
		}
		return r, n, nil
	}
	return r, n, nil
}

// RuleSpec is a user rule as typed (08 §2.1).
type RuleSpec struct {
	Kind   string
	A, B   string // references
	Target string // ignore: constraint reference as printed by sort
	Note   string
}

// AddRule stores a user rule, resolving references to lineages.
func (a *App) AddRule(spec RuleSpec) (*store.UserRule, error) {
	ix, err := a.Index()
	if err != nil {
		return nil, err
	}
	r, err := ix.resolveRule(spec)
	if err != nil {
		return nil, err
	}
	r.Created = a.Now().UTC()
	if _, err := a.Store.AddRule(ix.Env.ID, r); err != nil {
		return nil, err
	}
	return r, nil
}

// UpdateRule replaces rule r:<id> with spec; the ID and creation time stay (08 §2.1).
func (a *App) UpdateRule(id int64, spec RuleSpec) (*store.UserRule, error) {
	ix, err := a.Index()
	if err != nil {
		return nil, err
	}
	rs, err := a.Store.Rules(ix.Env.ID)
	if err != nil {
		return nil, err
	}
	var old *store.UserRule
	for _, r := range rs {
		if r.ID == id {
			old = r
		}
	}
	if old == nil {
		return nil, fmt.Errorf("no rule r:%d (archo rule list)", id)
	}
	r, err := ix.resolveRule(spec)
	if err != nil {
		return nil, err
	}
	r.ID, r.Created = id, old.Created
	if _, err := a.Store.UpdateRule(r); err != nil {
		return nil, err
	}
	return r, nil
}

// resolveRule checks a typed rule and resolves its references to lineages.
func (ix *Index) resolveRule(spec RuleSpec) (*store.UserRule, error) {
	var err error
	r := &store.UserRule{Kind: spec.Kind, Note: spec.Note}
	switch spec.Kind {
	case order.RuleAfter, order.RuleBefore, order.RuleIncompatible, order.RuleAttach:
		if r.A, err = ix.ResolveLineage(spec.A); err != nil {
			return nil, err
		}
		if r.B, err = ix.ResolveLineage(spec.B); err != nil {
			return nil, err
		}
		if r.A == r.B {
			return nil, errors.New("both references are in the same lineage")
		}
	case order.RuleTop, order.RuleBottom:
		if spec.B != "" {
			return nil, fmt.Errorf("%s takes one reference", spec.Kind)
		}
		if r.A, err = ix.ResolveLineage(spec.A); err != nil {
			return nil, err
		}
	case order.RuleIgnore:
		if r.Target, err = ix.StoredConstraintRef(spec.Target); err != nil {
			return nil, err
		}
	case order.RuleReplaces:
		if r.A, err = ix.ResolveLineage(spec.A); err != nil {
			return nil, err
		}
		if r.Target, err = ix.replacedPackage(spec.B); err != nil {
			return nil, err
		}
		for _, m := range ix.ByLineage[r.A] {
			if dependencyPackage(m.PackageID) == r.Target {
				return nil, fmt.Errorf("%s is %s itself", spec.A, spec.B)
			}
		}
	default:
		return nil, fmt.Errorf("unknown rule kind %q (after, before, incompatible, attach, top, bottom, replaces, ignore)", spec.Kind)
	}
	return r, nil
}

// replacedPackage is the package a replaces rule stands in for: pkg:<id>, which need not be
// installed, or a mod reference, whose packageId it takes.
func (ix *Index) replacedPackage(ref string) (string, error) {
	if p, ok := strings.CutPrefix(strings.TrimSpace(ref), "pkg:"); ok {
		if p = dependencyPackage(p); p != "" {
			return p, nil
		}
		return "", fmt.Errorf("%q names no package", ref)
	}
	m, err := ix.ResolveOne(ref)
	if err != nil {
		return "", fmt.Errorf("%w (a package that is not installed: pkg:<packageId>)", err)
	}
	return dependencyPackage(m.PackageID), nil
}

// StoredConstraintRef converts a printed constraint reference (with an instance or lineage
// reference) to its stored, lineage-keyed form:
//
//	decl:#a3f9:loadafter:brrainz.harmony   official:#a3f9   community:<pkg>:loadafter:<pkg>
func (ix *Index) StoredConstraintRef(ref string) (string, error) {
	parts := strings.Split(ref, ":")
	bad := fmt.Errorf("%q is not a constraint reference (decl:<ref>:<field>:<pkg>, official:<ref> or community:<pkg>:<field>:<pkg>, as printed by archo sort)", ref)
	switch strings.ToLower(parts[0]) {
	case "decl":
		if len(parts) != 4 {
			return "", bad
		}
		lin, err := ix.ResolveLineage(parts[1])
		if err != nil {
			return "", err
		}
		return order.DeclRef(lin, parts[2], parts[3]), nil
	case "official":
		if len(parts) != 2 {
			return "", bad
		}
		lin, err := ix.ResolveLineage(parts[1])
		if err != nil {
			return "", err
		}
		return order.OfficialRef(lin), nil
	case "community":
		if len(parts) == 3 && (strings.EqualFold(parts[2], "loadtop") || strings.EqualFold(parts[2], "loadbottom")) {
			parts = append(parts, "") // community:<pkg>:loadtop (08 §1.5)
		}
		if len(parts) != 4 {
			return "", bad
		}
		return order.CommunityRef(strings.ToLower(parts[1]), parts[2], parts[3]), nil
	}
	return "", bad
}

// DisplayConstraintRef is the inverse of StoredConstraintRef for output.
func (ix *Index) DisplayConstraintRef(stored string, primaryOf func(lineage string) string) string {
	parts := strings.Split(stored, ":")
	switch {
	case parts[0] == "decl" && len(parts) == 4:
		return "decl:" + primaryOf(parts[1]) + ":" + parts[2] + ":" + parts[3]
	case parts[0] == "official" && len(parts) == 2:
		return "official:" + primaryOf(parts[1])
	}
	return stored
}

func loadCommunity(path string) (order.Community, error) {
	c, err := rules.Load(path)
	if err != nil {
		return nil, err
	}
	return c, nil
}
