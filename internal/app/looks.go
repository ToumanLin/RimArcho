package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/content"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/fsscan"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/looks"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Looks (21): which Mod's textures the game shows, chosen per look without changing the
// load order. The index is internal/looks; this file holds the choices and their states,
// looksbuild.go the generated Mod RimArcho Looks Preference.

const (
	// LooksPackage is the packageId of the generated Mod.
	LooksPackage = "rimarcho.looks"
	// LooksName is the generated Mod's name, as the game and RimArcho show it.
	LooksName = "RimArcho Looks Preference"
	// LooksLink is its link name in Mods/ (09 §3: RimArcho's links start with DeployPrefix).
	LooksLink = DeployPrefix + "Looks"
	// LooksLabel is the label of the looks directory's source.
	LooksLabel  = "looks"
	looksFolder = "Looks"
	// LooksAddOp names the plan edit of an apply in the undo stack (21 §4 "In the plan").
	LooksAddOp = "add " + LooksName + " last (looks)"
)

// Scopes of a choice as the interfaces name them (21 §3.1).
const (
	LooksPlan = "plan"
	LooksAll  = "all"
)

// States of a choice (21 §3.4).
const (
	LookApplied   = "applied"
	LookNotNeeded = "not needed"
	LookAsleep    = "asleep"
	LookBroken    = "broken"
)

// IsLooks reports whether m is the generated Mod RimArcho Looks Preference.
func IsLooks(m *store.Mod) bool {
	return m != nil && m.Kind == source.Extra && strings.EqualFold(m.PackageID, LooksPackage)
}

// LooksDir is the looks directory (21 §4 "Where"): the folder of the registered looks
// source, else config looks.dir; "" until the first apply chooses one.
func (a *App) LooksDir() string {
	if all, err := a.Store.AllSources(); err == nil {
		for _, s := range all {
			if s.Kind == source.Extra && s.Status != source.StatusRetired && s.Label == LooksLabel {
				return s.Root
			}
		}
	}
	return a.Store.Meta("cfg:looks.dir")
}

// LooksDirDefault is the folder the first apply chooses when looks.dir is unset: on the
// volume of the game installation, so that files can be hard links to its Mods.
func (a *App) LooksDirDefault(gameRoot string) string {
	local := filepath.Join(a.LocalDir(), LooksLabel)
	same := a.SameVolume
	if same == nil {
		same = source.SameVolume
	}
	if gameRoot == "" || same(gameRoot, local) {
		return local
	}
	root := a.VolumeRoot
	if root == nil {
		root = volumeRoot
	}
	return filepath.Join(root(gameRoot), "RimArcho", LooksLabel)
}

func volumeRoot(path string) string {
	return filepath.VolumeName(path) + string(filepath.Separator)
}

// looksModDir is the generated Mod's folder in the looks directory dir.
func looksModDir(dir string) string { return filepath.Join(dir, looksFolder) }

// looksDirReady checks a looks directory like the Monitor folder (19 §6): it must not
// contain or be contained by the data directory or another source root.
func (a *App) looksDirReady(dir string) error {
	if err := a.checkRoots(dir); err != nil {
		return err
	}
	o, err := a.overlapping(dir, func(s *source.Source) bool {
		return s.Kind == source.Extra && s.Label == LooksLabel && env.PathKey(s.Root) == env.PathKey(dir)
	})
	if err != nil {
		return err
	}
	if o != nil {
		return fmt.Errorf("the looks folder %s overlaps src:%s (%s); choose another folder with archo config set looks.dir <dir>", dir, o.Label, o.Root)
	}
	return nil
}

// ensureLooksDir returns the looks directory, choosing and storing it on the first apply
// (21 §4 "Where"), and creates it. chosen: this call chose it.
func (a *App) ensureLooksDir(gameRoot string) (dir string, chosen bool, err error) {
	if dir = a.LooksDir(); dir != "" {
		if err := a.looksDirReady(dir); err != nil {
			return "", false, err
		}
		return dir, false, os.MkdirAll(dir, 0o755)
	}
	local := filepath.Join(a.LocalDir(), LooksLabel)
	dir = a.LooksDirDefault(gameRoot)
	if env.PathKey(dir) != env.PathKey(local) {
		if a.looksDirReady(dir) != nil || os.MkdirAll(dir, 0o755) != nil {
			dir = local
		}
	}
	if err := a.looksDirReady(dir); err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, err
	}
	return dir, true, a.Store.SetMeta("cfg:looks.dir", dir)
}

// looksInstance is the environment's usable RimArcho Looks Preference instance (nil before the first apply).
func looksInstance(ix *Index) *store.Mod {
	for _, m := range ix.Mods {
		if IsLooks(m) && m.Status == identity.Live && !m.Residue && m.Unloadable == "" {
			return m
		}
	}
	return nil
}

// ensureLooksMod makes the generated Mod exist and known: the looks directory, the Mod's
// folder with its About.xml, the shared source src:looks, and a scan of that source alone
// when it has no Instance yet (as Archo Monitor's folder is read, 19 §6).
func (a *App) ensureLooksMod(gameRoot string) (m *store.Mod, dir string, chosen bool, err error) {
	if dir, chosen, err = a.ensureLooksDir(gameRoot); err != nil {
		return nil, "", false, err
	}
	if err := looksSkeleton(looksModDir(dir)); err != nil {
		return nil, dir, chosen, err
	}
	src, err := a.sharedSource(LooksLabel, dir)
	if err != nil {
		return nil, dir, chosen, err
	}
	ix, err := a.Index()
	if err != nil {
		return nil, dir, chosen, err
	}
	if m = looksInstance(ix); m == nil {
		if _, err := a.ScanWith(ScanOptions{Sources: []string{src.ID}}); err != nil {
			return nil, dir, chosen, err
		}
		if ix, err = a.Index(); err != nil {
			return nil, dir, chosen, err
		}
		if m = looksInstance(ix); m == nil {
			return nil, dir, chosen, fmt.Errorf("the scan did not find %s in %s", LooksName, dir)
		}
	}
	return m, dir, chosen, nil
}

// LooksIndex builds the looks index of the plan in st (21 §2): its entries in load order
// with their effective load folders. The generated Mod itself is no provider. With
// LooksCache set and no progress wanted, an index already built for the same key is reused.
func (a *App) LooksIndex(ctx context.Context, st *LoadoutState, progress func(done, total int)) (*looks.Index, error) {
	job, err := a.LooksIndexJob(st)
	if err != nil {
		return nil, err
	}
	if a.LooksCache != nil && progress == nil {
		return a.LooksCache(a.LooksKey(st), func() (*looks.Index, error) { return job(ctx, nil) })
	}
	return job(ctx, progress)
}

// LooksIndexJob prepares the build of st's looks index: it reads the declarations and
// resolves the load folders now, and returns the build itself, which reads only Mod folders
// and the header cache and so may run while the App serves other calls (16 §8.4).
func (a *App) LooksIndexJob(st *LoadoutState) (func(ctx context.Context, progress func(done, total int)) (*looks.Index, error), error) {
	mods, err := a.looksMods(st)
	if err != nil {
		return nil, err
	}
	o := looks.Options{CachePath: filepath.Join(a.Home, "cache", "looks-v1.json"), Workers: min(8, fsscan.Workers(st.Ix.Env.GameRoot))}
	return func(ctx context.Context, progress func(done, total int)) (*looks.Index, error) {
		o := o
		o.Progress = progress
		return looks.Build(ctx, mods, o)
	}, nil
}

// LooksKey names what the looks index of st's plan is built from (21 §2.5): the game
// version and every entry's position, Instance and snapshot. The RimArcho Looks Preference entry is no
// provider and is left out, so adding it last keeps the key. An interface keeps the index
// in memory under this key and drops it when the key changes.
func (a *App) LooksKey(st *LoadoutState) string {
	h := sha1.New()
	fmt.Fprintf(h, "%s|%s", st.Ix.Env.ID, st.GameMM())
	for i, e := range st.List {
		if m := st.Ix.ByID[e.Instance]; m != nil && !IsLooks(m) && m.TombstoneAt.IsZero() {
			fmt.Fprintf(h, "|%d:%s:%d", i, m.ID, m.SnapID)
		}
	}
	return hex.EncodeToString(h.Sum(nil)[:10])
}

func (a *App) looksMods(st *LoadoutState) ([]looks.Mod, error) {
	var mods []*store.Mod
	active := map[string]bool{}
	for _, e := range st.List {
		if m := st.Ix.ByID[e.Instance]; m != nil {
			active[packageBase(strings.ToLower(m.PackageID))] = true
			mods = append(mods, m)
		}
	}
	if err := a.Store.LoadDecls(mods); err != nil {
		return nil, err
	}
	mm := st.GameMM()
	var out []looks.Mod
	for i, e := range st.List {
		m := st.Ix.ByID[e.Instance]
		if m == nil || IsLooks(m) || !m.TombstoneAt.IsZero() {
			continue
		}
		dir := modDir(st.Ix, m)
		res := content.Resolve(dir, m.Decl().LoadFolders, mm, func(pkg string) bool {
			return active[packageBase(strings.ToLower(pkg))]
		})
		out = append(out, looks.Mod{Pos: i, ID: m.ID, Name: m.DisplayName(), Root: dir, Folders: res.Folders, Official: m.Kind == source.Official})
	}
	return out, nil
}

// LookChoiceState is one stored choice with its state in this plan (21 §3.4).
type LookChoiceState struct {
	store.LookChoice
	Plan bool       // the choice is this plan's (false: for all plans)
	Mod  *store.Mod // the chosen Lineage's Instance in the plan; nil: asleep
	Pos  int        // its 0-based position in the plan (-1: asleep)
	// State is applied, not needed, asleep or broken; "" when Overruled.
	State string
	// Overruled: another choice wins every slot this one names (the plan's choice over the
	// one for all plans, a slot's over the whole look's, 21 §3.2).
	Overruled bool
}

// LookState is one look that has choices.
type LookState struct {
	ID      string
	Look    *looks.Look       // nil: no Mod of the plan supplies it now
	Choice  map[string]int    // texture slot → provider position, for Look.InGame
	Slots   map[string]string // texture slot → state, for the slots a choice decides
	Choices []LookChoiceState
	Mixed   bool // the in-game result takes slots from more than one Mod
}

// LooksState is a plan's looks with the choices that apply to it.
type LooksState struct {
	St    *LoadoutState
	Index *looks.Index
	Mods  map[int]*store.Mod    // the plan's Mods by position (looks.Provider.Pos)
	Looks map[string]*LookState // the looks that have a choice, by id
	// Entry is the 1-based position of RimArcho Looks Preference in the plan; 0: not in it, so applied
	// choices do not reach the game.
	Entry int
	// Counts of choices by state (overruled ones are not counted).
	Applied, NotNeeded, Asleep, Broken int
}

// ChoiceOf returns the choice map of a look for looks.Look.InGame (nil: no choice).
func (ls *LooksState) ChoiceOf(look string) map[string]int {
	if s := ls.Looks[look]; s != nil {
		return s.Choice
	}
	return nil
}

// LooksState builds the index of st's plan and resolves the stored choices against it.
func (a *App) LooksState(ctx context.Context, st *LoadoutState, progress func(done, total int)) (*LooksState, error) {
	idx, err := a.LooksIndex(ctx, st, progress)
	if err != nil {
		return nil, err
	}
	return a.LooksStateOf(st, idx)
}

// LooksStateOf resolves the stored choices against idx, an index built for st's plan
// (LooksKey). It reads the choices only.
func (a *App) LooksStateOf(st *LoadoutState, idx *looks.Index) (*LooksState, error) {
	rows, err := a.Store.LookChoices(store.LookScope(st.Plan.ID), store.LookScopeAll)
	if err != nil {
		return nil, err
	}
	return looksState(st, idx, rows), nil
}

// looksState resolves rows (this plan's and those for all plans) against idx.
func looksState(st *LoadoutState, idx *looks.Index, rows []store.LookChoice) *LooksState {
	ls := &LooksState{St: st, Index: idx, Mods: map[int]*store.Mod{}, Looks: map[string]*LookState{}}
	byLineage := map[string]int{}
	for i, e := range st.List {
		m := st.Ix.ByID[e.Instance]
		switch {
		case m == nil:
		case IsLooks(m):
			ls.Entry = i + 1
		default:
			ls.Mods[i] = m
			if _, ok := byLineage[m.LineageID]; !ok {
				byLineage[m.LineageID] = i
			}
		}
	}
	plan := store.LookScope(st.Plan.ID)
	for _, r := range rows {
		s := ls.Looks[r.Look]
		if s == nil {
			s = &LookState{ID: r.Look, Look: idx.ByID[r.Look], Choice: map[string]int{}, Slots: map[string]string{}}
			ls.Looks[r.Look] = s
		}
		c := LookChoiceState{LookChoice: r, Plan: r.Scope == plan, Pos: -1}
		if pos, ok := byLineage[r.Lineage]; ok {
			c.Pos, c.Mod = pos, ls.Mods[pos]
		}
		s.Choices = append(s.Choices, c)
	}
	for _, s := range ls.Looks {
		s.resolve()
		for _, c := range s.Choices {
			switch c.State {
			case LookApplied:
				ls.Applied++
			case LookNotNeeded:
				ls.NotNeeded++
			case LookAsleep:
				ls.Asleep++
			case LookBroken:
				ls.Broken++
			}
		}
	}
	return ls
}

// pick is the index of the choice that decides slot (21 §3.2): the plan's choice for the
// slot, else the plan's for the whole look, else the same two for all plans; -1 if none.
func (s *LookState) pick(slot string) int {
	best, rank := -1, 4
	for i, c := range s.Choices {
		r := 4
		switch {
		case c.Plan && c.Slot == slot && slot != looks.All:
			r = 0
		case c.Plan && c.Slot == looks.All:
			r = 1
		case !c.Plan && c.Slot == slot && slot != looks.All:
			r = 2
		case !c.Plan && c.Slot == looks.All:
			r = 3
		}
		if r < rank {
			best, rank = i, r
		}
	}
	return best
}

// textureSlots are the look's slots a choice can name (masks follow their texture).
func textureSlots(l *looks.Look) []string {
	if l == nil {
		return nil
	}
	var out []string
	for _, n := range l.SlotNames() {
		if !looks.IsMask(n) {
			out = append(out, n)
		}
	}
	return out
}

// resolve fills Choice, Slots, Mixed and every choice's State.
func (s *LookState) resolve() {
	slots := textureSlots(s.Look)
	decides := make([][]string, len(s.Choices)) // choice → the slots it decides
	for _, slot := range slots {
		i := s.pick(slot)
		if i < 0 {
			continue
		}
		decides[i] = append(decides[i], slot)
		if c := s.Choices[i]; c.Mod != nil && supplies(s.Look, c.Pos, slot) {
			s.Choice[slot] = c.Pos
		}
	}
	var wins, base map[string]looks.Win
	if s.Look != nil {
		wins, base = s.Look.InGame(s.Choice), s.Look.InGame(nil)
		s.Mixed = s.Look.Mixed(s.Choice)
	}
	for i := range s.Choices {
		c := &s.Choices[i]
		switch {
		case c.Slot != looks.All || len(slots) == 0:
			c.Overruled = s.pick(c.Slot) != i
		default:
			c.Overruled = len(decides[i]) == 0
		}
		if c.Overruled {
			continue
		}
		if c.Mod == nil {
			c.State = LookAsleep
			for _, slot := range decides[i] {
				s.Slots[slot] = LookAsleep
			}
			continue
		}
		c.State = LookBroken
		for _, slot := range decides[i] {
			st := slotState(s.Look, c.Pos, slot, wins, base)
			if st == "" {
				continue // the whole look was chosen and this Mod has no such slot
			}
			s.Slots[slot] = st
			if st == LookApplied || c.State == LookBroken {
				c.State = st
			}
		}
	}
}

func supplies(l *looks.Look, pos int, slot string) bool {
	for _, p := range l.Providers {
		if p.Pos == pos {
			return p.Slots[slot] != nil
		}
	}
	return false
}

// slotState is the state of a slot decided by a choice of the Mod at pos, which is in the
// plan: "" when that Mod does not supply the slot, not needed when the load order already
// gives the chosen file (or an identical one) and its mask, else applied.
func slotState(l *looks.Look, pos int, slot string, wins, base map[string]looks.Win) string {
	if !supplies(l, pos, slot) {
		return ""
	}
	w, b := wins[slot], base[slot]
	if w.Pos == b.Pos {
		return LookNotNeeded
	}
	if !looks.Identical(w.File, b.File) {
		return LookApplied
	}
	mw, mb := wins[looks.MaskOf(slot)], base[looks.MaskOf(slot)]
	if mw.File != mb.File && !looks.Identical(mw.File, mb.File) {
		return LookApplied
	}
	return LookNotNeeded
}

// takeLooks takes the RimArcho Looks Preference entry out of l: a sort orders the rest and puts it back last.
func takeLooks(st *LoadoutState, l loadout.List) (rest, entry loadout.List) {
	for i, e := range l {
		if IsLooks(st.Ix.ByID[e.Instance]) {
			rest = append(l[:i:i].Clone(), l[i+1:]...)
			return rest, loadout.List{e}
		}
	}
	return l, nil
}

// LooksResult is what an apply, an undo or a rebuild did, or with DryRun would do.
type LooksResult struct {
	DryRun  bool
	Look    string
	Scope   string             // LooksPlan or LooksAll
	Plan    string             // the current plan's name
	From    *store.Mod         // apply: the chosen Mod
	Set     []store.LookChoice // apply: the choices written
	Removed int                // undo: the choices removed
	Mixed   bool               // apply: the in-game result takes slots from more than one Mod
	// Added: RimArcho Looks Preference was added to the plan, or moved to its end, as one undoable edit.
	Added bool
	// Dir is the looks directory; DirChosen: this call chose it (the first apply).
	Dir       string
	DirChosen bool
	Build     *LooksBuild // nil when the rebuild did not run
	// BuildErr says why the rebuild did not run or failed (a game is running); the choices
	// are kept and the next write to the game rebuilds.
	BuildErr string
	State    *LooksState // the plan's looks after the call
}

func looksScope(st *LoadoutState, scope string) (string, error) {
	switch scope {
	case "", LooksPlan:
		return store.LookScope(st.Plan.ID), nil
	case LooksAll:
		return store.LookScopeAll, nil
	}
	return "", fmt.Errorf("scope %q: plan or all", scope)
}

func scopeName(scope string) string {
	if scope == LooksAll {
		return LooksAll
	}
	return LooksPlan
}

// looksApplyRows validates an apply against the plan's index and returns the choices to
// write: one for the whole look, or one per slot.
func (a *App) looksApplyRows(ls *LooksState, lookID, modRef string, slots []string, scope string) (*store.Mod, []store.LookChoice, error) {
	st := ls.St
	l := ls.Index.ByID[strings.ToLower(lookID)]
	if l == nil {
		return nil, nil, fmt.Errorf("no look %q in plan:%s (archo looks)", lookID, st.Plan.Name)
	}
	m, err := st.Ix.ResolveOne(modRef)
	if err != nil {
		return nil, nil, err
	}
	pos := st.List.Index(m.ID)
	if pos < 0 {
		return nil, nil, fmt.Errorf("%s %s is not in plan:%s", st.Ix.Ref(m.ID), m.DisplayName(), st.Plan.Name)
	}
	var prov *looks.Provider
	for _, p := range l.Providers {
		if p.Pos == pos {
			prov = p
		}
	}
	if prov == nil {
		return nil, nil, fmt.Errorf("%s %s has no texture of the look %s", st.Ix.Ref(m.ID), m.DisplayName(), l.ID)
	}
	now := a.Now().UTC()
	if len(slots) == 0 {
		return m, []store.LookChoice{{Scope: scope, Look: l.ID, Slot: looks.All, Lineage: m.LineageID, At: now}}, nil
	}
	var rows []store.LookChoice
	seen := map[string]bool{}
	for _, s := range slots {
		s = strings.ToLower(strings.TrimSpace(s))
		switch {
		case seen[s]:
			continue
		case looks.IsMask(s):
			return nil, nil, fmt.Errorf("%s is a mask: it follows its texture and is not chosen on its own", s)
		case prov.Slots[s] == nil:
			return nil, nil, fmt.Errorf("%s %s has no slot %s of the look %s (slots: %s)", st.Ix.Ref(m.ID), m.DisplayName(), s, l.ID, strings.Join(textureSlotsOf(prov), ", "))
		}
		seen[s] = true
		rows = append(rows, store.LookChoice{Scope: scope, Look: l.ID, Slot: s, Lineage: m.LineageID, At: now})
	}
	return m, rows, nil
}

func textureSlotsOf(p *looks.Provider) []string {
	var out []string
	for s := range p.Slots {
		if !looks.IsMask(s) {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// withRows is the stored choices after set is written (whole: the look's other choices
// in that scope go first) or, with set nil, after look's choices in scope are removed.
func withRows(rows, set []store.LookChoice, scope, look string, whole bool) []store.LookChoice {
	var out []store.LookChoice
	for _, r := range rows {
		drop := r.Scope == scope && r.Look == look && (whole || set == nil)
		for _, n := range set {
			drop = drop || r.Scope == n.Scope && r.Look == n.Look && r.Slot == n.Slot
		}
		if !drop {
			out = append(out, r)
		}
	}
	return append(out, set...)
}

// LooksApply chooses that look, or the given slots of it, comes from the Mod modRef names
// (21 §3.3): it writes choices only and never changes the plan's order, except that
// RimArcho Looks Preference becomes the current plan's last entry as one undoable edit (21 §4). The
// generated Mod is then rebuilt. scope is LooksPlan or LooksAll. dryRun writes nothing.
func (a *App) LooksApply(ctx context.Context, lookID, modRef string, slots []string, scope string, dryRun bool) (*LooksResult, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	sc, err := looksScope(st, scope)
	if err != nil {
		return nil, err
	}
	ls, err := a.LooksState(ctx, st, nil)
	if err != nil {
		return nil, err
	}
	m, set, err := a.looksApplyRows(ls, lookID, modRef, slots, sc)
	if err != nil {
		return nil, err
	}
	if fs := a.modsVolumeFS(st.Ix); noLinks(fs) {
		return nil, fmt.Errorf("Mods/ is on %s, which cannot hold the link %s needs (09 §3)", fs, LooksName)
	}
	r := &LooksResult{DryRun: dryRun, Look: set[0].Look, Scope: scopeName(scope), Plan: st.Plan.Name, From: m, Set: set}
	whole := len(slots) == 0
	if dryRun {
		return a.looksPreview(r, ls, withRowsOf(a, st, set, sc, r.Look, whole))
	}
	if _, r.Dir, r.DirChosen, err = a.ensureLooksMod(st.Ix.Env.GameRoot); err != nil {
		return nil, err
	}
	if err := a.Store.SetLookChoices(set, whole); err != nil {
		return nil, err
	}
	detail := "apply from " + m.DisplayName()
	if !whole {
		detail += ": " + strings.Join(slots, ", ")
	}
	a.logLooksChoice(st, r, detail)
	if r.Added, err = a.looksLast(); err != nil {
		return r, err
	}
	return r, a.looksAfter(ctx, r)
}

// withRowsOf reads the stored choices of st's plan and applies withRows to them.
func withRowsOf(a *App, st *LoadoutState, set []store.LookChoice, scope, look string, whole bool) []store.LookChoice {
	rows, _ := a.Store.LookChoices(store.LookScope(st.Plan.ID), store.LookScopeAll)
	return withRows(rows, set, scope, look, whole)
}

// looksPreview fills a dry run's result: the state and the rebuild as they would be with rows.
func (a *App) looksPreview(r *LooksResult, ls *LooksState, rows []store.LookChoice) (*LooksResult, error) {
	st := ls.St
	r.State = looksState(st, ls.Index, rows)
	if s := r.State.Looks[r.Look]; s != nil {
		r.Mixed = s.Mixed
	}
	r.Added = r.Set != nil && r.State.Entry != len(st.List)
	if r.Dir = a.LooksDir(); r.Dir == "" {
		r.Dir, r.DirChosen = a.LooksDirDefault(st.Ix.Env.GameRoot), true
	}
	var err error
	r.Build, err = a.looksRebuild(r.State, r.Dir, true, nil)
	if err != nil {
		r.BuildErr = err.Error()
	}
	return r, nil
}

// looksLast makes RimArcho Looks Preference the current plan's last entry as one undoable edit; added
// reports a change.
func (a *App) looksLast() (added bool, err error) {
	r, err := a.edit(LooksAddOp, func(st *LoadoutState, l loadout.List) (loadout.List, []string, error) {
		m := looksInstance(st.Ix)
		if m == nil {
			return nil, nil, errors.New(LooksName + " is not installed")
		}
		out := make(loadout.List, 0, len(l)+1)
		for _, e := range l {
			if !IsLooks(st.Ix.ByID[e.Instance]) {
				out = append(out, e)
			}
		}
		return append(out, loadout.Entry{Instance: m.ID}), nil, nil
	})
	if err != nil {
		return false, err
	}
	return len(r.Changes) > 0, nil
}

// looksAfter rebuilds the generated Mod for the current plan after an apply or an undo and
// fills the result's state. A rebuild that fails leaves the choices as they are.
func (a *App) looksAfter(ctx context.Context, r *LooksResult) error {
	st, err := a.Loadout()
	if err != nil {
		return err
	}
	if r.State, err = a.LooksState(ctx, st, nil); err != nil {
		return err
	}
	if s := r.State.Looks[r.Look]; s != nil {
		r.Mixed = s.Mixed
	}
	if r.Dir = a.LooksDir(); r.Dir == "" {
		return nil // nothing was ever applied: there is no generated Mod to rebuild
	}
	if r.Build, err = a.looksRebuild(r.State, r.Dir, false, nil); err != nil {
		r.BuildErr = err.Error()
	}
	return nil
}

// LooksUndo removes look's choices in one scope only (21 §3.3): never another look's, never
// the other scope's. The plan keeps its RimArcho Looks Preference entry. dryRun writes nothing.
func (a *App) LooksUndo(ctx context.Context, lookID, scope string, dryRun bool) (*LooksResult, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	sc, err := looksScope(st, scope)
	if err != nil {
		return nil, err
	}
	lookID = strings.ToLower(strings.TrimSpace(lookID))
	r := &LooksResult{DryRun: dryRun, Look: lookID, Scope: scopeName(scope), Plan: st.Plan.Name}
	rows, err := a.Store.LookChoices(sc)
	if err != nil {
		return nil, err
	}
	for _, c := range rows {
		if c.Look == lookID {
			r.Removed++
		}
	}
	if r.Removed == 0 {
		return nil, fmt.Errorf("no choice for the look %s in scope %s (archo looks --filter chosen)", lookID, r.Scope)
	}
	if dryRun {
		ls, err := a.LooksState(ctx, st, nil)
		if err != nil {
			return nil, err
		}
		return a.looksPreview(r, ls, withRowsOf(a, st, nil, sc, lookID, true))
	}
	if _, err := a.Store.DeleteLookChoices(sc, lookID); err != nil {
		return nil, err
	}
	a.logLooksChoice(st, r, fmt.Sprintf("undo: %s removed", plural(r.Removed, "choice")))
	return r, a.looksAfter(ctx, r)
}

// LooksRebuild rebuilds the generated Mod for the current plan (21 §4 "Rebuild"). When the
// plan has applied choices and no RimArcho Looks Preference entry, it adds the entry like an apply.
func (a *App) LooksRebuild(ctx context.Context, dryRun bool) (*LooksResult, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	ls, err := a.LooksState(ctx, st, nil)
	if err != nil {
		return nil, err
	}
	r := &LooksResult{DryRun: dryRun, Scope: LooksPlan, Plan: st.Plan.Name}
	if dryRun {
		if _, err := a.looksPreview(r, ls, withRowsOf(a, st, nil, "", "", false)); err != nil {
			return nil, err
		}
		r.Added = ls.Applied > 0 && ls.Entry != len(st.List)
		return r, nil
	}
	if ls.Applied > 0 {
		if _, r.Dir, r.DirChosen, err = a.ensureLooksMod(st.Ix.Env.GameRoot); err != nil {
			return nil, err
		}
		if r.Added, err = a.looksLast(); err != nil {
			return r, err
		}
	}
	return r, a.looksAfter(ctx, r)
}
