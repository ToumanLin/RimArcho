package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rimarcho/rimarcho/internal/env"
	"github.com/rimarcho/rimarcho/internal/identity"
	"github.com/rimarcho/rimarcho/internal/loadout"
	"github.com/rimarcho/rimarcho/internal/order"
	"github.com/rimarcho/rimarcho/internal/rules"
	"github.com/rimarcho/rimarcho/internal/source"
	"github.com/rimarcho/rimarcho/internal/store"
)

// LoadoutState is the workspace with everything needed to interpret it.
type LoadoutState struct {
	Ix       *Index
	Reg      *loadout.Registry
	WS       *store.Workspace
	List     loadout.List
	Latest   *store.Version // newest version: what ModsConfig.xml holds as far as RimArcho knows
	File     *loadout.ModsConfig
	FilePath string
	FileErr  error
	Notices  []string // what Sync did (import, external change, conflict)
	Aliases  map[string][]store.Alias
	// MM overrides the game's major.minor for analysis (--game-version, decisions-m3 §12.7).
	MM string
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
	return st.Reg.Heal(loadout.Decode(st.Latest.Entries))
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
// the game's: the game resets such a list on load (04 §2.2 rule 5).
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

// Candidates derives the game-loadable instances (02 §4.1, 04 §2.4): live, subscribed,
// not unloadable, and located in Data/, Mods/ or the workshop — directly or via an alias.
func Candidates(ix *Index, aliases map[string][]store.Alias) []loadout.Candidate {
	visible := func(k source.Kind) bool { return k == source.Official || k == source.Local || k == source.Workshop }
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
				if k := kindOf[al.SourceID]; visible(k) {
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

// Loadout loads the workspace, first syncing it with ModsConfig.xml (04 §2.3). Like Index,
// the result is cached until the database or ModsConfig.xml (size, mtime) changes; a cached
// state carries no notices (they were reported when the sync happened).
func (a *App) Loadout() (*LoadoutState, error) {
	ix, err := a.Index()
	if err != nil {
		return nil, err
	}
	key := a.Store.ChangeKey() + "|" + ix.Env.ID
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
		return &c, nil
	}
	st, err := a.loadLoadout(ix)
	if err != nil {
		return nil, err
	}
	st.MM = a.GameVersion
	// Syncing may have written; key the cache on the state after it.
	key = a.Store.ChangeKey() + key[strings.Index(key, "|"):]
	if ix2, err := a.Index(); err == nil && ix2 == ix {
		a.stCache, a.stKey = st, key
	} else {
		a.stCache = nil
	}
	return st, nil
}

func (a *App) loadLoadout(ix *Index) (*LoadoutState, error) {
	aliases, err := a.Store.AllAliases(ix.Env.ID)
	if err != nil {
		return nil, err
	}
	st := &LoadoutState{Ix: ix, Reg: loadout.NewRegistry(Candidates(ix, aliases)), Aliases: aliases}
	if err := a.sync(st); err != nil {
		return nil, err
	}
	return st, nil
}

func (a *App) sync(st *LoadoutState) error {
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
	ws, err := a.Store.Workspace(e.ID)
	if err != nil {
		return err
	}
	mcs, err := a.Store.ModsConfigState(e.ID)
	if err != nil {
		return err
	}
	now := a.Now().UTC()
	if ws == nil {
		v := &store.Version{Created: now, Source: "import", Entries: fileList.Encode()}
		if st.File != nil {
			v.GameVersion = st.File.Version
			v.Note = "ModsConfig.xml"
		} else {
			v.Note = "no ModsConfig.xml"
		}
		n, err := a.Store.AddVersion(e.ID, v)
		if err != nil {
			return err
		}
		ws = &store.Workspace{EnvID: e.ID, Entries: fileList.Encode(), OriginVersion: n, OriginKind: "import", Updated: now}
		if err := a.Store.SetWorkspace(ws); err != nil {
			return err
		}
		mcs.Seen = h
		if err := a.Store.SetModsConfigState(e.ID, mcs); err != nil {
			return err
		}
		st.Notices = append(st.Notices, fmt.Sprintf("imported ModsConfig.xml as v%d: %d entries", n, len(fileList)))
	} else if st.File != nil && !bytes.Equal(h, mcs.Seen) {
		if bytes.Equal(h, mcs.Written) {
			mcs.Seen = h
		} else {
			latest, err := a.latest(e.ID)
			if err != nil {
				return err
			}
			if latest != nil && latest.GameVersion == st.File.Version &&
				st.Reg.Heal(loadout.Decode(latest.Entries)).Equal(fileList) {
				// Rewritten by the game or another tool without a real change (formatting).
				mcs.Seen = h
				if err := a.Store.SetModsConfigState(e.ID, mcs); err != nil {
					return err
				}
				goto done
			}
			clean := latest == nil || st.Reg.Heal(loadout.Decode(ws.Entries)).Equal(st.Reg.Heal(loadout.Decode(latest.Entries)))
			n, err := a.Store.AddVersion(e.ID, &store.Version{Created: now, Source: "external", Entries: fileList.Encode(),
				GameVersion: st.File.Version, Note: "ModsConfig.xml changed outside RimArcho"})
			if err != nil {
				return err
			}
			if clean {
				ws.Entries, ws.OriginVersion, ws.OriginKind, ws.Conflict = fileList.Encode(), n, "external", 0
				st.Notices = append(st.Notices, fmt.Sprintf("ModsConfig.xml changed outside RimArcho: loaded as v%d", n))
			} else {
				ws.Conflict = n
				st.Notices = append(st.Notices, fmt.Sprintf("ModsConfig.xml changed outside RimArcho: kept as v%d, not loaded because the workspace has unsaved edits (archo loadout diff v%d; archo loadout checkout v%d)", n, n, n))
			}
			ws.Updated = now
			if err := a.Store.SetWorkspace(ws); err != nil {
				return err
			}
			mcs.Seen = h
		}
		if err := a.Store.SetModsConfigState(e.ID, mcs); err != nil {
			return err
		}
	}
done:
	st.WS = ws
	st.List = st.Reg.Heal(loadout.Decode(ws.Entries))
	st.Latest, err = a.latest(e.ID)
	return err
}

func (a *App) latest(envID string) (*store.Version, error) {
	vs, err := a.Store.Versions(envID)
	if err != nil || len(vs) == 0 {
		return nil, err
	}
	return vs[0], nil
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
	if !after.Equal(before) {
		e := st.Ix.Env
		if err := a.Store.PushStep(e.ID, "undo", before.Encode(), op); err != nil {
			return nil, err
		}
		if err := a.Store.ClearSteps(e.ID, "redo"); err != nil {
			return nil, err
		}
		st.WS.Entries, st.WS.Updated = after.Encode(), a.Now().UTC()
		if st.WS.OriginKind == "checkout" {
			st.WS.OriginKind = "edited"
		}
		if err := a.Store.SetWorkspace(st.WS); err != nil {
			return nil, err
		}
		st.List = after
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
	case !st.Reg.Known(m.ID):
		return fmt.Sprintf("the game does not load mods from src:%s; link or copy it into Mods/", m.SourceLabel)
	case !st.Reg.Loadable(m.ID):
		return "pkg.duplicate-installed: another copy with packageId " + m.PackageID + " in the same place wins (the game ignores this one)"
	}
	return ""
}

// Enable adds instances to the workspace (04 §2.1).
func (a *App) Enable(refs []string, p Placement) (*EditResult, error) {
	return a.edit("enable", func(st *LoadoutState, l loadout.List) (loadout.List, []string, error) {
		var notes []string
		at := -1
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
			for _, o := range st.Reg.SamePackage(m.ID) {
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
			if at < 0 {
				if at, err = p.target(st, l, len(l)); err != nil {
					return nil, nil, err
				}
			}
			l = insert(l, at, loadout.Entry{Instance: m.ID})
			at++
		}
		return l, notes, nil
	})
}

// Disable removes instances (or missing entries named by pkg:) from the workspace.
func (a *App) Disable(refs []string) (*EditResult, error) {
	return a.edit("disable", func(st *LoadoutState, l loadout.List) (loadout.List, []string, error) {
		var notes []string
		for _, ref := range refs {
			ms, err := st.Ix.Resolve(ref)
			if err != nil && strings.HasPrefix(strings.ToLower(ref), "pkg:") {
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
			if pkg, ok := strings.CutPrefix(strings.ToLower(ref), "pkg:"); ok {
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

// Swap replaces an enabled instance by another instance of the same lineage (04 §2.1).
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
		if why := whyNotLoadable(st, other); why != "" {
			return nil, nil, fmt.Errorf("cannot swap in %s: %s", st.Ix.Ref(other.ID), why)
		}
		l[i] = loadout.Entry{Instance: other.ID}
		return l, nil, nil
	})
}

// SortResult is the outcome of Sort.
type SortResult struct {
	State   *LoadoutState
	Input   order.Input
	Result  order.Result
	Sorted  loadout.List
	Changes []loadout.Change
	Applied bool
}

// Community loads community rules from <home>/cache/communityRules.json (04 §5.2).
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
	rules, err := a.Store.Rules(st.Ix.Env.ID)
	if err != nil {
		return in, err
	}
	for _, r := range rules {
		in.Rules = append(in.Rules, order.Rule{ID: r.ID, Kind: r.Kind, A: r.A, B: r.B, Target: r.Target})
	}
	return in, nil
}

// Sort orders the workspace (04 §4); dryRun only computes.
func (a *App) Sort(dryRun bool) (*SortResult, error) {
	var out *SortResult
	compute := func(st *LoadoutState, l loadout.List) (loadout.List, error) {
		in, err := a.SortInput(st, l)
		if err != nil {
			return nil, err
		}
		r := order.Sort(in)
		sorted := make(loadout.List, 0, len(l))
		for _, i := range r.Order {
			sorted = append(sorted, l[i])
		}
		out = &SortResult{Input: in, Result: r, Sorted: sorted, Changes: loadout.Diff(l, sorted, st.LineageOf)}
		return sorted, nil
	}
	if dryRun {
		st, err := a.Loadout()
		if err != nil {
			return nil, err
		}
		if _, err := compute(st, st.List); err != nil {
			return nil, err
		}
		out.State = st
		return out, nil
	}
	r, err := a.edit("sort", func(st *LoadoutState, l loadout.List) (loadout.List, []string, error) {
		sorted, err := compute(st, l)
		return sorted, nil, err
	})
	if err != nil {
		return nil, err
	}
	out.State, out.Applied = r.State, true
	return out, nil
}

// UndoRedo steps the workspace back (undo) or forward (redo).
func (a *App) UndoRedo(redo bool) (*EditResult, string, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, "", err
	}
	e := st.Ix.Env
	from, to := "undo", "redo"
	if redo {
		from, to = "redo", "undo"
	}
	entries, op, ok, err := a.Store.PopStep(e.ID, from)
	if err != nil {
		return nil, "", err
	}
	if !ok {
		return nil, "", fmt.Errorf("nothing to %s", from)
	}
	if err := a.Store.PushStep(e.ID, to, st.List.Encode(), op); err != nil {
		return nil, "", err
	}
	before := st.List
	st.WS.Entries, st.WS.Updated = entries, a.Now().UTC()
	if err := a.Store.SetWorkspace(st.WS); err != nil {
		return nil, "", err
	}
	st.List = loadout.Decode(entries)
	return &EditResult{State: st, Before: before, Changes: loadout.Diff(before, st.List, st.LineageOf)}, op, nil
}

// SaveResult describes a save (04 §2.2).
type SaveResult struct {
	State      *LoadoutState
	Path       string
	Backup     string
	Version    int // new version number (0 on dry run)
	Source     string
	OldVersion string // ModsConfig.xml <version> before
	NewVersion string
	Tokens     []string
	Changes    []loadout.Change // newest version → saved list
	DryRun     bool
	Restores   int // source "undo": the version the workspace was checked out from
	Missing    int
	Unchanged  bool
}

// Save writes the workspace to ModsConfig.xml: backup, atomic replace, version (04 §2.2).
func (a *App) Save(dryRun bool) (*SaveResult, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	e := st.Ix.Env
	if st.FilePath == "" {
		return nil, errors.New("the environment has no user data directory; set one with archo env set --user-data <dir>")
	}
	if st.FileErr != nil {
		return nil, fmt.Errorf("cannot read %s: %v (fix or remove the file first)", st.FilePath, st.FileErr)
	}
	r := &SaveResult{State: st, Path: st.FilePath, DryRun: dryRun, Changes: st.Changes()}
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
	r.Source = "user"
	if st.WS.OriginKind == "checkout" {
		r.Source, r.Restores = "undo", st.WS.OriginVersion
	}
	if dryRun {
		return r, nil
	}
	if err := a.checkRoots(filepath.Dir(st.FilePath)); err != nil {
		return nil, err
	}
	if r.Backup, err = loadout.WriteFile(st.FilePath, data); err != nil {
		return nil, err
	}
	now := a.Now().UTC()
	note := ""
	if r.Source == "undo" {
		note = fmt.Sprintf("restored v%d", st.WS.OriginVersion)
	}
	n, err := a.Store.AddVersion(e.ID, &store.Version{Created: now, Source: r.Source, Entries: st.List.Encode(), GameVersion: r.NewVersion, Note: note})
	if err != nil {
		return nil, err
	}
	h := loadout.Hash(data)
	if err := a.Store.SetModsConfigState(e.ID, &store.ModsConfigState{Written: h, Seen: h, WrittenAt: now}); err != nil {
		return nil, err
	}
	st.WS.OriginVersion, st.WS.OriginKind, st.WS.Conflict, st.WS.Updated = n, "save", 0, now
	if err := a.Store.SetWorkspace(st.WS); err != nil {
		return nil, err
	}
	r.Version = n
	return r, nil
}

// ParseVersionRef resolves "v3", "3", a tag name, or "workspace" (0).
func (a *App) ParseVersionRef(envID, ref string) (int, error) {
	r := strings.ToLower(strings.TrimSpace(ref))
	if r == "workspace" || r == "ws" || r == "current" {
		return 0, nil
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(r, "v")); err == nil && n > 0 {
		if v, _ := a.Store.Version(envID, n); v != nil {
			return n, nil
		}
		return 0, fmt.Errorf("no version v%d (archo loadout log)", n)
	}
	if n := a.Store.TagVersionN(envID, ref); n > 0 {
		return n, nil
	}
	return 0, fmt.Errorf("%q is not a version (v<n>, a tag, or workspace; archo loadout log)", ref)
}

// ListAt returns the list of a version (0 = workspace).
func (a *App) ListAt(st *LoadoutState, n int) (loadout.List, error) {
	if n == 0 {
		return st.List, nil
	}
	v, err := a.Store.Version(st.Ix.Env.ID, n)
	if err != nil || v == nil {
		return nil, fmt.Errorf("no version v%d", n)
	}
	return st.Reg.Heal(loadout.Decode(v.Entries)), nil
}

// Checkout loads a version into the workspace (it still has to be saved).
func (a *App) Checkout(ref string) (*EditResult, int, error) {
	var n int
	r, err := a.edit("checkout", func(st *LoadoutState, l loadout.List) (loadout.List, []string, error) {
		var err error
		if n, err = a.ParseVersionRef(st.Ix.Env.ID, ref); err != nil {
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
	st.WS.OriginVersion, st.WS.OriginKind = n, "checkout"
	if st.WS.Conflict == n {
		st.WS.Conflict = 0
	}
	if err := a.Store.SetWorkspace(st.WS); err != nil {
		return nil, 0, err
	}
	return r, n, nil
}

// RuleSpec is a user rule as typed (04 §5.1).
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
	r := &store.UserRule{Kind: spec.Kind, Note: spec.Note, Created: a.Now().UTC()}
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
	case order.RuleIgnore:
		if r.Target, err = ix.StoredConstraintRef(spec.Target); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown rule kind %q (after, before, incompatible, attach, ignore)", spec.Kind)
	}
	if _, err := a.Store.AddRule(ix.Env.ID, r); err != nil {
		return nil, err
	}
	return r, nil
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
