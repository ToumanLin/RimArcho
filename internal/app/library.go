package app

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
)

// Mod 库存 (06 §2.9): every mod on this machine for the environment, one Lineage with each
// of its Instances, where it lives, how big it is, which plans and packs use it and what is
// wrong with it. The interfaces (CLI `library`, MCP list_library, the GUI page, the TUI
// list) all filter the same Library with the same LibQuery.

// Library states, most important first: a row shows the first one it has (06 §2.9).
const (
	LibUnloadable   = "unloadable" // no readable About.xml (mod.unloadable)
	LibMissing      = "missing"    // subscribed in the Workshop ACF, no folder on disk
	LibIgnored      = "ignored"    // another copy with the same packageId wins (pkg.duplicate-installed)
	LibResidue      = "residue"    // a Workshop folder that is no longer subscribed
	LibHidden       = "hidden"     // About.xml renamed to About.xml.disabled by another manager (08 §6)
	LibUpdated      = "updated"    // a locked version whose upstream changed (04 §7.3)
	LibSourceUpdate = "source-update"
)

// LibStates is the state order.
var LibStates = []string{LibUnloadable, LibMissing, LibIgnored, LibResidue, LibHidden, LibUpdated, LibSourceUpdate}

// Library source values (the source: filter).
const (
	LibWorkshop = "workshop"
	LibLocal    = "local"  // the game's Mods/
	LibLocked   = "locked" // the lock library (04 §7)
	LibSteamCMD = "steamcmd"
	LibExtra    = "extra"
	LibMonitor  = "monitor" // Archo Monitor's folder (08 §9)
	LibGit      = "git"
	LibPage     = "page"
)

// LibSources is the source order (rows and filters).
var LibSources = []string{LibWorkshop, LibLocal, LibLocked, LibSteamCMD, LibGit, LibPage, LibExtra, LibMonitor}

// Library filter keys (LibQuery Include / Exclude are "key:value").
var LibKeys = []string{"tag", "source", "location", "version", "state", "used", "updated", "content", "has", "name", "author", "id"}

// LibAges are the updated: values (06 §2.9): within a week, a month, half a year, and
// older. They nest: a copy updated three days ago is in week, month and half-year.
var LibAges = []string{"week", "month", "half-year", "older"}

func libAges(t, now time.Time) []string {
	if t.IsZero() {
		return nil
	}
	switch d := now.Sub(t); {
	case d <= 7*24*time.Hour:
		return []string{"week", "month", "half-year"}
	case d <= 31*24*time.Hour:
		return []string{"month", "half-year"}
	case d <= 183*24*time.Hour:
		return []string{"half-year"}
	}
	return []string{"older"}
}

// LibUse is a plan or a pack that uses an instance.
type LibUse struct {
	Kind    string // "plan" or "pack"
	Name    string // a plan of another environment of the same game: "<env>/<plan>"
	PlanID  int64  // plans of this environment (0 otherwise)
	Current bool   // the environment's current plan
}

// Value is the used: filter value ("plan:main", "pack:Colony").
func (u LibUse) Value() string { return u.Kind + ":" + u.Name }

// LibInstance is one copy of a mod: an Instance, or a subscribed Workshop item with no folder.
type LibInstance struct {
	Subscription *subscribe.Record
	// Mod is the instance (a tombstone for a missing item RimArcho saw before; nil for one it
	// never saw).
	Mod             *store.Mod
	Ref             string // "#…", or "ws:<id>" for an item never seen
	DirectoryValues []string
	DirectoryName   string
	Content         int
	Source          string // LibSources
	WorkshopID      string
	Path            string
	Location        string // registered source root; retained for subscribed items not on disk
	// Created: when RimArcho first saw the copy; a lock library copy's copy time when
	// origin.json has it.
	Created  time.Time
	Bytes    int64
	Sized    bool // Bytes comes from a scan (else "—")
	Versions []string
	States   []string // LibStates order
	UsedIn   []LibUse
	InPlan   bool // the current plan uses it
	Staged   bool // in the current plan's staging (06 §2.5)
	// Played: the newest launch whose list held it (launch records, 08 §1).
	Played  time.Time
	Changed time.Time // the newest scan that recorded a change
	Locked  *Locked   // a locked version RimArcho made, with its upstream
	Spare   bool      // a copy in Mods/ beside another copy of the lineage: it can move to the lock library
	// Updated: the Workshop's update time when known, else the folder's modification time.
	Updated time.Time
	Authors string
	// Note is this copy's own note and what RimArcho recorded when it made the copy (nil:
	// none); UpstreamUpdates counts the upstream's updates since the copy was made: a locked
	// version's upstream, or for another copy beside a Workshop copy of the same mod, that
	// Workshop copy (UpstreamWorkshop).
	Note             *store.CopyNote
	UpstreamUpdates  int
	UpstreamWorkshop bool
	// Textures: the DDS RimArcho wrote next to this copy's PNGs (贴图优化; nil: none).
	Textures *TextureSummary
	// PNG: the copy has PNGs under a Textures/ folder (its last scan), so 优化贴图 applies.
	PNG bool
}

// Missing reports a subscribed item with no folder.
func (i *LibInstance) Missing() bool { return slices.Contains(i.States, LibMissing) }

// State is the most important state ("" when nothing is wrong).
func (i *LibInstance) State() string {
	if len(i.States) == 0 {
		return ""
	}
	return i.States[0]
}

// LibLineage is one mod: its instances in source order.
type LibLineage struct {
	ID, Ref, Name string
	Alias, Note   string
	Tags          []string
	Instances     []*LibInstance
	search        string // lower-cased names, package ids, authors, ids, folders, player data
}

// InPlan reports whether the current plan uses one of its copies.
func (l *LibLineage) InPlan() bool {
	return slices.ContainsFunc(l.Instances, func(i *LibInstance) bool { return i.InPlan })
}

// Bytes is the size of every instance with a known size.
func (l *LibLineage) Bytes() (n int64, sized bool) {
	for _, i := range l.Instances {
		if i.Sized {
			n, sized = n+i.Bytes, true
		}
	}
	return n, sized
}

// State is the most important state of any instance.
func (l *LibLineage) State() string {
	best := len(LibStates)
	for _, i := range l.Instances {
		if len(i.States) > 0 {
			best = min(best, slices.Index(LibStates, i.States[0]))
		}
	}
	if best == len(LibStates) {
		return ""
	}
	return LibStates[best]
}

// UsedIn is every plan and pack that uses an instance, without repeats.
func (l *LibLineage) UsedIn() []LibUse {
	var out []LibUse
	for _, i := range l.Instances {
		for _, u := range i.UsedIn {
			if !slices.ContainsFunc(out, func(x LibUse) bool { return x.Value() == u.Value() }) {
				out = append(out, u)
			}
		}
	}
	sortUses(out)
	return out
}

// Played is the newest launch of any instance.
func (l *LibLineage) Played() time.Time {
	var t time.Time
	for _, i := range l.Instances {
		if i.Played.After(t) {
			t = i.Played
		}
	}
	return t
}

// Changed is the newest change of any instance.
func (l *LibLineage) Changed() time.Time {
	var t time.Time
	for _, i := range l.Instances {
		if i.Changed.After(t) {
			t = i.Changed
		}
	}
	return t
}

// Library is an environment's inventory.
type Library struct {
	Env      *env.Environment
	GameMM   string
	Lineages []*LibLineage // by name
	St       *LoadoutState
	Now      time.Time // what updated: is counted from
}

// Library builds the current environment's inventory.
func (a *App) Library() (*Library, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	return a.library(st, "")
}

// LibraryFor builds only the lineage containing an exact copy ID/ref (or a Workshop
// ghost). It retains the full loadout so copy actions use the same rules as Library.
func (a *App) LibraryFor(ref string) (*Library, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	lineage := "\x00" + ref
	if strings.HasPrefix(ref, "ws:") {
		lineage = ref
	}
	for _, m := range st.Ix.Mods {
		if m.ID == ref || st.Ix.Ref(m.ID) == ref {
			lineage = m.LineageID
			break
		}
	}
	return a.library(st, lineage)
}

func (a *App) library(st *LoadoutState, only string) (*Library, error) {
	ix := st.Ix
	selected := func(m *store.Mod) bool { return only == "" || m.LineageID == only }
	lib := &Library{Env: ix.Env, GameMM: st.GameMM(), St: st}
	used, err := a.libUses(st)
	if err != nil {
		return nil, err
	}
	played, err := a.libPlayed(st)
	if err != nil {
		return nil, err
	}
	var instances []string
	if only != "" {
		for _, m := range ix.ByLineage[only] {
			instances = append(instances, m.ID)
		}
		if len(instances) == 0 { // an unseen ghost has no history or note
			instances = []string{""}
		}
	}
	changed, err := a.Store.LastChanged(ix.Env.ID, instances...)
	if err != nil {
		return nil, err
	}
	notes, err := a.Store.CopyNotes(ix.Env.ID, instances...)
	texs := a.TextureSummaries() // 贴图优化: the DDS RimArcho wrote into each copy
	if err != nil {
		return nil, err
	}
	lib.Now = a.Now()
	locks := map[string]*Locked{}
	for _, l := range st.Locked() {
		locks[l.Copy.ID] = l
	}
	pinUpdated := map[string]bool{} // RimCrow-style copies whose hidden upstream changed
	for _, h := range st.Hidden() {
		if h.Kind == HiddenPinned && h.Upstream {
			pinUpdated[h.Copy.ID] = true
		}
	}
	snaps := map[string]int64{}
	for _, m := range ix.Mods {
		if selected(m) {
			snaps[m.ID] = m.SnapID
		}
	}
	contents, err := a.Store.ContentKinds(snaps)
	if err != nil {
		return nil, err
	}
	srcKind := map[string]string{}
	for _, s := range ix.Sources {
		switch {
		case s.Kind == source.Workshop:
			srcKind[s.ID] = LibWorkshop
		case s.Kind == source.SteamCMD:
			srcKind[s.ID] = LibSteamCMD
		case s.Kind == source.Local, s.Kind == source.Elsewhere: // every game's Mods/ (04 §1.7)
			srcKind[s.ID] = LibLocal
		case a.IsLockSource(s):
			srcKind[s.ID] = LibLocked
		case s.Kind == source.Extra && s.Label == MonitorLabel:
			srcKind[s.ID] = LibMonitor
		case s.Kind == source.Extra:
			srcKind[s.ID] = LibExtra
		}
	}
	byLin := map[string]*LibLineage{}
	lineage := func(id string) *LibLineage {
		l := byLin[id]
		if l == nil {
			l = &LibLineage{ID: id, Ref: ix.LRef(id)}
			byLin[id] = l
		}
		return l
	}
	live := map[string]int{} // live instances per lineage
	onDisk := map[string]bool{}
	for _, m := range ix.Mods {
		if m.Status == identity.Live && m.Kind != source.Official {
			live[m.LineageID]++
			if m.Kind == source.Workshop {
				onDisk[m.Rel] = true
			}
		}
	}
	add := func(m *store.Mod, i *LibInstance) {
		l := lineage(m.LineageID)
		l.Instances = append(l.Instances, i)
		l.Alias, l.Note, l.Tags = m.Alias, m.Note, m.Tags
		l.search += " " + strings.ToLower(strings.Join([]string{m.Name, m.Alias, m.PackageID, m.Authors, m.WorkshopID(), m.Rel, m.Note, strings.Join(m.Tags, " ")}, " "))
		if i.Note != nil {
			l.search += " " + strings.ToLower(i.Note.Text)
		}
	}
	staged := map[string]bool{}
	if items, _, err := a.Staged(); err == nil {
		for _, it := range items {
			staged[it.Instance] = true
		}
	}
	var png map[int64]bool
	if only == "" {
		png = a.pngSnapshots()
	} else {
		png, err = a.Store.SnapshotsWithPNGFor(snaps)
		if err != nil {
			return nil, err
		}
	}
	for _, m := range ix.Mods {
		if !selected(m) || m.Status != identity.Live || m.Kind == source.Official || srcKind[m.SourceID] == "" {
			continue
		}
		i := &LibInstance{Mod: m, Ref: ix.Ref(m.ID), Source: srcKind[m.SourceID], WorkshopID: m.WorkshopID(), Path: ix.ModDir(m),
			Bytes: m.TotalSize, Sized: m.SnapID != 0 && (m.FileCount > 0 || m.TotalSize > 0),
			UsedIn: used[m.ID], InPlan: st.List.Index(m.ID) >= 0, Staged: staged[m.ID], Played: played[m.ID], Changed: changed[m.ID], Locked: locks[m.ID],
			Authors: strings.TrimSpace(m.Authors), Note: notes[m.ID]}
		i.Subscription = a.SubscriptionOf(m)
		if r := i.Subscription; r != nil {
			if r.Kind == "git" || r.Kind == "page" {
				i.Source = r.Kind
			}
			if r.WorkshopID != "" {
				i.WorkshopID = r.WorkshopID
			}
		}
		if s := ix.Source(m.SourceID); s != nil {
			i.Location = s.Root
			i.DirectoryValues, i.DirectoryName = a.DirectoryValues(s), DirectoryName(s)
			i.Content = contents[m.ID]
		}
		i.PNG = m.SnapID != 0 && png[m.SnapID]
		if t, ok := texs[m.ID]; ok {
			i.Textures = &t
		}
		if m.ACFTimeUpdated > 0 {
			i.Updated = time.Unix(m.ACFTimeUpdated, 0).UTC()
		} else if fi, err := os.Stat(i.Path); err == nil {
			i.Updated = fi.ModTime().UTC()
		}
		for _, v := range strings.Fields(strings.ReplaceAll(m.SupportedVersions, ",", " ")) {
			if !slices.Contains(i.Versions, v) {
				i.Versions = append(i.Versions, v)
			}
		}
		i.Created = m.FirstSeen
		if m.Origin != "" {
			var og scan.Origin
			if json.Unmarshal([]byte(m.Origin), &og) == nil && og.CopiedAt != "" {
				if t, err := time.Parse(time.RFC3339, og.CopiedAt); err == nil {
					i.Created = t
				}
			}
		}
		i.Spare = m.Kind.GameMods() && (live[m.LineageID] > 1 || m.Origin != "" || a.inModsDuplicate(st, m.ID)) && !ix.ModIsLink(m)
		if i.Locked != nil {
			since := i.Created
			if i.Note != nil && i.Note.Made != nil {
				since = i.Note.Made.At
			}
			if !since.IsZero() {
				i.UpstreamUpdates = a.Store.UpdatesSince(i.Locked.Upstream.ID, since)
			}
		}
		i.States = libStates(st, m, i.Locked != nil && i.Locked.Updated || pinUpdated[m.ID])
		if i.Subscription != nil && i.Subscription.HasUpdate() {
			i.States = append(i.States, LibSourceUpdate)
		}
		add(m, i)
	}
	// Subscribed Workshop items with no folder (06 §2.9: "幽灵"). A tombstone of the item
	// keeps its name and lineage.
	for _, s := range ix.Sources {
		if s.Kind != source.Workshop || s.Status == source.StatusRetired {
			continue
		}
		acf := scan.ReadACF(s.Root)
		if acf == nil {
			continue
		}
		ids := make([]string, 0, len(acf.Items))
		for id, it := range acf.Items {
			if it.Subscribed && !onDisk[id] {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		for _, id := range ids {
			var ghost *store.Mod
			for _, m := range ix.Mods {
				if m.SourceID == s.ID && m.Rel == id && (ghost == nil || m.LastSeen.After(ghost.LastSeen)) {
					ghost = m
				}
			}
			i := &LibInstance{Source: LibWorkshop, Location: s.Root, WorkshopID: id, Ref: "ws:" + id, States: []string{LibMissing}}
			if ghost == nil {
				if only != "" && only != "ws:"+id {
					continue
				}
				byLin["ws:"+id] = &LibLineage{ID: "ws:" + id, Ref: "ws:" + id, Name: "Workshop item " + id, Instances: []*LibInstance{i}, search: " " + id}
				continue
			}
			if !selected(ghost) {
				continue
			}
			i.Mod, i.Ref, i.UsedIn, i.InPlan, i.Note = ghost, ix.Ref(ghost.ID), used[ghost.ID], st.List.Index(ghost.ID) >= 0, notes[ghost.ID]
			add(ghost, i)
		}
	}
	for id, l := range byLin {
		if l.Name == "" {
			l.Name = libName(ix, id, l)
		}
		l.search = strings.ToLower(l.Name) + l.search
		// a copy beside a Workshop copy of the same mod: how often the Workshop updated since
		var ws *LibInstance
		for _, i := range l.Instances {
			if i.Source == LibWorkshop && i.Mod != nil && !i.Missing() {
				ws = i
			}
		}
		for _, i := range l.Instances {
			if ws == nil || i == ws || i.Locked != nil || i.Source == LibWorkshop || i.Created.IsZero() {
				continue
			}
			i.UpstreamUpdates, i.UpstreamWorkshop = a.Store.UpdatesSince(ws.Mod.ID, i.Created), true
		}
		sort.SliceStable(l.Instances, func(x, y int) bool {
			a, b := l.Instances[x], l.Instances[y]
			if ra, rb := slices.Index(LibSources, a.Source), slices.Index(LibSources, b.Source); ra != rb {
				return ra < rb
			}
			if !a.Created.Equal(b.Created) {
				return a.Created.After(b.Created) // the newest locked version first
			}
			return a.Ref < b.Ref
		})
		lib.Lineages = append(lib.Lineages, l)
	}
	sortLineages(lib.Lineages, "name")
	return lib, nil
}

// libName is the lineage's name: its primary instance's, else its first instance's.
func libName(ix *Index, id string, l *LibLineage) string {
	if lin := ix.Lineages[id]; lin != nil {
		if p := ix.ByID[lin.PrimaryID]; p != nil && p.Status == identity.Live {
			return p.DisplayName()
		}
	}
	for _, i := range l.Instances {
		if i.Mod != nil {
			return i.Mod.DisplayName()
		}
	}
	return id
}

func libStates(st *LoadoutState, m *store.Mod, updated bool) []string {
	var out []string
	switch {
	case m.Residue:
		out = append(out, LibResidue)
	case m.Unloadable == scan.UnloadableAboutDisabled:
		out = append(out, LibHidden)
	case m.Unloadable != "":
		out = append(out, LibUnloadable)
	case st.Reg.Known(m.ID) && !st.Reg.Loadable(m.ID):
		out = append(out, LibIgnored)
	}
	if updated {
		out = append(out, LibUpdated)
	}
	slices.SortFunc(out, func(a, b string) int { return slices.Index(LibStates, a) - slices.Index(LibStates, b) })
	return out
}

func sortUses(us []LibUse) {
	sort.SliceStable(us, func(i, j int) bool {
		a, b := us[i], us[j]
		if a.Current != b.Current {
			return a.Current
		}
		if a.Kind != b.Kind {
			return a.Kind == "plan"
		}
		if (a.PlanID != 0) != (b.PlanID != 0) {
			return a.PlanID != 0 // this environment's plans before another's
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
}

// libEnv is an environment whose plans count: every one (02 §0 item 11). here maps an
// instance of its plans to the current environment's ("" none): a shared folder's instance
// is the same in every environment; another environment's own Data/ and Mods/ are not this
// game's, except for an environment made before migration v14 on the same game folder,
// whose instances are matched by real path.
type libEnv struct {
	e    *env.Environment
	here func(id string) string
	cur  bool
}

func (a *App) libEnvs(st *LoadoutState) ([]libEnv, error) {
	ix := st.Ix
	out := []libEnv{{e: ix.Env, here: func(id string) string { return id }, cur: true}}
	envs, err := a.Store.Envs()
	if err != nil {
		return nil, err
	}
	var byPath map[string]string
	for _, e := range envs {
		if e.ID == ix.Env.ID {
			continue
		}
		path := map[string]string{}
		if env.PathKey(e.GameRoot) == env.PathKey(ix.Env.GameRoot) {
			if byPath == nil {
				byPath = map[string]string{}
				for _, m := range ix.Mods {
					if m.Status == identity.Live && m.RealPath != "" {
						byPath[env.PathKey(m.RealPath)] = m.ID
					}
				}
			}
			mods, err := a.Store.Mods(e.ID)
			if err != nil {
				return nil, err
			}
			for _, m := range mods {
				if m.RealPath != "" && ix.ByID[m.ID] == nil {
					path[m.ID] = byPath[env.PathKey(m.RealPath)]
				}
			}
		}
		out = append(out, libEnv{e: e, here: func(id string) string {
			if ix.ByID[id] != nil {
				return id
			}
			return path[id]
		}})
	}
	return out, nil
}

// planList is a plan's list without loading its environment: the workspace, else its
// newest version.
func (a *App) planList(p *store.Plan) (loadout.List, error) {
	ws, err := a.Store.Workspace(p.ID)
	if err != nil {
		return nil, err
	}
	if ws != nil {
		return loadout.Decode(ws.Entries), nil
	}
	v, err := a.Store.LatestVersion(p.ID)
	if err != nil || v == nil {
		return nil, err
	}
	return loadout.Decode(v.Entries), nil
}

// libHere maps an instance ID of any list to the instance it is in this game: itself when
// visible here, else the copy at the same path seen by another game sharing this game
// folder ("" when none).
func libHere(envs []libEnv) func(id string) string {
	return func(id string) string {
		for _, le := range envs {
			if h := le.here(id); h != "" {
				return h
			}
		}
		return ""
	}
}

// libUses maps each instance to the plans (lists are shared by every game) and packs
// that use it.
func (a *App) libUses(st *LoadoutState) (map[string][]LibUse, error) {
	out := map[string][]LibUse{}
	envs, err := a.libEnvs(st)
	if err != nil {
		return nil, err
	}
	names := map[string]libEnv{}
	for _, le := range envs {
		names[le.e.Name] = le
	}
	here := libHere(envs)
	ps, err := a.Store.Plans()
	if err != nil {
		return nil, err
	}
	for _, p := range ps {
		l := st.List
		if st.Plan == nil || p.ID != st.Plan.ID {
			if l, err = a.planList(p); err != nil {
				return nil, err
			}
		}
		u := LibUse{Kind: "plan", Name: p.Name, PlanID: p.ID, Current: st.Plan != nil && p.ID == st.Plan.ID}
		for _, e := range l {
			if id := here(e.Instance); id != "" && !slices.ContainsFunc(out[id], func(x LibUse) bool { return x.Value() == u.Value() }) {
				out[id] = append(out[id], u)
			}
		}
	}
	packs, err := a.Packs()
	if err != nil {
		return nil, err
	}
	for _, p := range packs {
		le, ok := names[p.Env]
		if !ok {
			continue
		}
		u := LibUse{Kind: "pack", Name: p.Name}
		for _, e := range p.Entries {
			if id := le.here(e.Instance); id != "" && e.Instance != "" && !slices.ContainsFunc(out[id], func(x LibUse) bool { return x.Value() == u.Value() }) {
				out[id] = append(out[id], u)
			}
		}
	}
	for _, us := range out {
		sortUses(us)
	}
	return out, nil
}

// libPlayed maps each instance to the newest launch whose list held it: a plan's launch by
// the version it recorded (checked against the list fingerprint, else the plan's version
// with that fingerprint), a pack's by its pack.json.
func (a *App) libPlayed(st *LoadoutState) (map[string]time.Time, error) {
	out := map[string]time.Time{}
	envs, err := a.libEnvs(st)
	if err != nil {
		return nil, err
	}
	var packs map[string]*PackInfo
	versions := map[int64][]*store.Version{}
	for _, le := range envs {
		rs, err := a.Store.Launches(le.e.ID, 0)
		if err != nil {
			return nil, err
		}
		for _, r := range rs {
			var ids []string
			if name := r.Pack(); name != "" {
				if packs == nil {
					packs = map[string]*PackInfo{}
					ps, err := a.Packs()
					if err != nil {
						return nil, err
					}
					for _, p := range ps {
						packs[p.Name] = p
					}
				}
				if p := packs[name]; p != nil {
					for _, e := range p.Entries {
						ids = append(ids, e.Instance)
					}
				}
			} else if r.PlanID != 0 {
				vs, ok := versions[r.PlanID]
				if !ok {
					if vs, err = a.Store.Versions(r.PlanID); err != nil {
						return nil, err
					}
					versions[r.PlanID] = vs
				}
				var hit loadout.List
				for _, v := range vs {
					if v.N == r.VersionN {
						if l := loadout.Decode(v.Entries); r.Fingerprint == "" || ListFingerprint(l) == r.Fingerprint {
							hit = l
						}
						break
					}
				}
				if hit == nil && r.Fingerprint != "" {
					for _, v := range vs {
						if l := loadout.Decode(v.Entries); ListFingerprint(l) == r.Fingerprint {
							hit = l
							break
						}
					}
				}
				for _, e := range hit {
					ids = append(ids, e.Instance)
				}
			}
			for _, id := range ids {
				if h := le.here(id); h != "" && r.Started.After(out[h]) {
					out[h] = r.Started
				}
			}
		}
	}
	return out, nil
}

// TagMatch: v names tag when, ignoring case, it starts at the beginning of one of the tag's
// words (06 §3.7): "facial" finds "Facial Hair" and "Facial Animation", "anim" finds
// "Facial Animation"; "air" does not. The plan's search (view.TagMatch) and the library's
// tag: filter share it.
func TagMatch(tag, v string) bool {
	if v == "" {
		return false
	}
	t := strings.ToLower(tag)
	v = strings.ToLower(v)
	for i := 0; i < len(t); i++ {
		if (i == 0 || IsTagSep(t[i-1])) && strings.HasPrefix(t[i:], v) {
			return true
		}
	}
	return false
}

// IsTagSep separates the words of a tag.
func IsTagSep(c byte) bool { return c == ' ' || c == '-' || c == '_' || c == '/' || c == '.' }

// ---- filtering (06 §2.9) ----

// LibQuery selects and orders the library: search words (all must match: names, package
// id, authors, alias, tags, note, Workshop ID, folder), filters "key:value" (one key's
// values are alternatives, different keys must all hold; Exclude drops what has any of
// its values) and a sort: name, size, changed, played.
type LibQuery struct {
	Search           string
	Include, Exclude []string
	Sort             string
}

// LibSorts are the sort orders, by column (06 §2.9); a leading "-" reverses one. Each has
// its natural direction: names, sources, authors and tags A to Z; sizes biggest first;
// dates and versions newest first; states most important first; used-in most used first.
var LibSorts = []string{"name", "size", "changed", "played", "state", "source", "used", "updated", "version", "author", "tags"}

// LibMatch is a lineage that matches, with the instances that match.
type LibMatch struct {
	*LibLineage
	Shown []*LibInstance
}

// LibFacetValue is one filter value with how many lineages it would show.
type LibFacetValue struct {
	Value string
	Count int
	On    int // 1 included, -1 excluded
}

// LibFacet is one filter key's values.
type LibFacet struct {
	Key    string
	Values []LibFacetValue
}

// LibResult is a filtered library.
type LibResult struct {
	Matches []*LibMatch
	Facets  []LibFacet
	Total   int // lineages in the library
}

// ParseLibFilter splits "key:value" and checks the key.
func ParseLibFilter(f string) (key, value string, err error) {
	k, v, ok := strings.Cut(f, ":")
	k = strings.ToLower(strings.TrimSpace(k))
	if k == "tags" {
		k = "tag"
	}
	if !ok || v == "" || !slices.Contains(LibKeys, k) {
		return "", "", fmt.Errorf("bad filter %q: want key:value with key %s", f, strings.Join(LibKeys, ", "))
	}
	if k != "used" && k != "tag" && k != "location" {
		v = strings.ToLower(v)
	}
	return k, strings.TrimSpace(v), nil
}

// values are an instance's values for a filter key (tag: its lineage's tags).
func libValues(l *LibLineage, i *LibInstance, key string, now time.Time) []string {
	switch key {
	case "updated":
		return libAges(i.Updated, now)
	case "tag":
		return l.Tags
	case "source":
		if len(i.DirectoryValues) > 0 {
			return i.DirectoryValues
		}
		return []string{i.Source}
	case "content":
		if i.Mod == nil {
			return nil
		}
		if i.Content&store.HasDLL != 0 {
			return []string{"assembly"}
		}
		if i.Content&store.HasDefsPatch == 0 && i.Content&store.HasLanguages != 0 {
			return []string{"xml", "language"}
		}
		return []string{"xml"}
	case "has":
		var vs []string
		if len(i.States) > 0 {
			vs = append(vs, "problem")
		}
		if i.State() == LibUnloadable || i.Missing() {
			vs = append(vs, "error")
		}
		if l.Note != "" {
			vs = append(vs, "note")
		}
		if len(l.Tags) > 0 {
			vs = append(vs, "tag")
		}
		if l.Alias != "" {
			vs = append(vs, "alias")
		}
		return vs
	case "name":
		return []string{l.Name}
	case "author":
		return []string{i.Authors}
	case "id":
		if i.Mod != nil {
			return []string{i.Mod.PackageID, i.Ref, i.WorkshopID}
		}
		return []string{i.Ref, i.WorkshopID}
	case "location":
		if i.Location != "" {
			return []string{i.Location}
		}
		return nil
	case "version":
		return i.Versions
	case "state":
		return i.States
	case "used":
		if len(i.UsedIn) == 0 {
			return []string{"none"}
		}
		out := make([]string, len(i.UsedIn))
		for j, u := range i.UsedIn {
			out[j] = u.Value()
		}
		return out
	}
	return nil
}

func anyIn(vals []string, want map[string]bool, fold bool) bool {
	for _, v := range vals {
		if fold {
			v = strings.ToLower(v)
		}
		if want[v] {
			return true
		}
	}
	return false
}

type libCond struct {
	inc, exc map[string]map[string]bool // key → values (tag and used compared case-insensitively)
	words    []string
	// tags are the library's tag names (lower case): tag:<v> naming one of them is that tag
	// (a click on the filter column); any other v matches tags by word (TagMatch).
	tags      map[string]bool
	exactTags map[string]bool
	now       time.Time
}

func (q LibQuery) cond() (*libCond, error) {
	c := &libCond{inc: map[string]map[string]bool{}, exc: map[string]map[string]bool{}, exactTags: map[string]bool{}, words: strings.Fields(strings.ToLower(q.Search))}
	for _, set := range []struct {
		in []string
		to map[string]map[string]bool
	}{{q.Include, c.inc}, {q.Exclude, c.exc}} {
		for _, f := range set.in {
			k, v, err := ParseLibFilter(f)
			if err != nil {
				return nil, err
			}
			if set.to[k] == nil {
				set.to[k] = map[string]bool{}
			}
			set.to[k][strings.ToLower(v)] = true
			if strings.HasPrefix(strings.ToLower(f), "tags:") {
				c.exactTags[strings.ToLower(v)] = true
			}
		}
	}
	return c, nil
}

// passes: the instance holds every key's included values but skip's, and none excluded.
func (c *libCond) passes(l *LibLineage, i *LibInstance, skip string) bool {
	for k, want := range c.inc {
		if k != skip && !c.has(l, i, k, want) {
			return false
		}
	}
	for k, drop := range c.exc {
		if c.has(l, i, k, drop) {
			return false
		}
	}
	return true
}

func (c *libCond) has(l *LibLineage, i *LibInstance, k string, want map[string]bool) bool {
	if k == "source" {
		for v := range want {
			switch v {
			case "steamworkshop":
				v = LibWorkshop
			case "localmods":
				v = LibLocal
			case "archolocked", "copy":
				v = LibLocked
			case "extra":
				v = "other"
			}
			if anyIn(libValues(l, i, k, c.now), map[string]bool{v: true}, true) || v == i.Source {
				return true
			}
		}
		return false
	}
	if k == "name" || k == "author" || k == "id" {
		for _, value := range libValues(l, i, k, c.now) {
			for v := range want {
				if strings.Contains(strings.ToLower(value), v) {
					return true
				}
			}
		}
		return false
	}
	if anyIn(libValues(l, i, k, c.now), want, true) {
		return true
	}
	if k != "tag" {
		return false
	}
	for v := range want {
		if c.tags[v] || c.exactTags[v] {
			continue
		}
		for _, t := range l.Tags {
			if TagMatch(t, v) {
				return true
			}
		}
	}
	return false
}

func (c *libCond) found(l *LibLineage) bool {
	for _, w := range c.words {
		if !strings.Contains(l.search, w) {
			return false
		}
	}
	return true
}

// Filter applies a query.
func (lib *Library) Filter(q LibQuery) (*LibResult, error) {
	c, err := q.cond()
	if err != nil {
		return nil, err
	}
	if q.Sort != "" && !slices.Contains(LibSorts, strings.TrimPrefix(q.Sort, "-")) {
		return nil, fmt.Errorf("bad sort %q: want %s (a leading - reverses)", q.Sort, strings.Join(LibSorts, ", "))
	}
	c.tags, c.now = map[string]bool{}, lib.Now
	for _, l := range lib.Lineages {
		for _, t := range l.Tags {
			c.tags[strings.ToLower(t)] = true
		}
	}
	res := &LibResult{Total: len(lib.Lineages)}
	counts := map[string]map[string]int{}
	for _, k := range LibKeys {
		counts[k] = map[string]int{}
	}
	all := map[string]map[string]string{} // key → lower value → as shown
	for _, l := range lib.Lineages {
		for _, i := range l.Instances {
			for _, k := range LibKeys {
				if all[k] == nil {
					all[k] = map[string]string{}
				}
				for _, v := range libValues(l, i, k, c.now) {
					if strings.TrimSpace(v) == "" {
						continue
					}
					if _, ok := all[k][strings.ToLower(v)]; !ok {
						all[k][strings.ToLower(v)] = v
					}
				}
			}
		}
		if !c.found(l) {
			continue
		}
		m := &LibMatch{LibLineage: l}
		for _, i := range l.Instances {
			if c.passes(l, i, "") {
				m.Shown = append(m.Shown, i)
			}
		}
		if len(m.Shown) > 0 {
			res.Matches = append(res.Matches, m)
		}
		// Each key's counts ignore that key's own choices, so its other values show what
		// adding them would bring.
		for _, k := range LibKeys {
			seen := map[string]bool{}
			for _, i := range l.Instances {
				if !c.passes(l, i, k) {
					continue
				}
				for _, v := range libValues(l, i, k, c.now) {
					seen[strings.ToLower(v)] = true
				}
			}
			for v := range seen {
				counts[k][v]++
			}
		}
	}
	for _, k := range LibKeys {
		f := LibFacet{Key: k}
		for lv, v := range all[k] {
			fv := LibFacetValue{Value: v, Count: counts[k][lv]}
			if c.inc[k][lv] {
				fv.On = 1
			} else if c.exc[k][lv] {
				fv.On = -1
			}
			f.Values = append(f.Values, fv)
		}
		sortFacet(k, f.Values, lib)
		res.Facets = append(res.Facets, f)
	}
	ls := make([]*LibLineage, len(res.Matches))
	for j, m := range res.Matches {
		ls[j] = m.LibLineage
	}
	order := map[*LibLineage]int{}
	sortLineages(ls, q.Sort)
	for j, l := range ls {
		order[l] = j
	}
	sort.SliceStable(res.Matches, func(x, y int) bool { return order[res.Matches[x].LibLineage] < order[res.Matches[y].LibLineage] })
	return res, nil
}

func sortFacet(k string, vs []LibFacetValue, lib *Library) {
	rank := func(v string) int {
		switch k {
		case "source":
			if strings.HasPrefix(v, LibLocal+"-") { // a game's Mods/ (local-main) sorts as local
				v = LibLocal
			}
			return slices.Index(LibSources, v)
		case "state":
			return slices.Index(LibStates, v)
		case "updated":
			return slices.Index(LibAges, v)
		case "used":
			if v == "none" {
				return -1
			}
			if strings.HasPrefix(v, "pack:") {
				return 2
			}
			if lib.St.Plan != nil && v == "plan:"+lib.St.Plan.Name {
				return 0
			}
			if strings.Contains(v, "/") {
				return 2 // another environment's plan
			}
			return 1
		}
		return 0
	}
	sort.SliceStable(vs, func(i, j int) bool {
		a, b := vs[i], vs[j]
		if ra, rb := rank(a.Value), rank(b.Value); ra != rb {
			return ra < rb
		}
		switch k {
		case "version": // newest first
			return versionLess(b.Value, a.Value)
		case "tag":
			if a.Count != b.Count {
				return a.Count > b.Count
			}
		}
		return strings.ToLower(a.Value) < strings.ToLower(b.Value)
	})
}

// versionLess compares "1.5" < "1.10".
func versionLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		x, e1 := strconv.Atoi(pa[i])
		y, e2 := strconv.Atoi(pb[i])
		if e1 != nil || e2 != nil {
			if pa[i] != pb[i] {
				return pa[i] < pb[i]
			}
			continue
		}
		if x != y {
			return x < y
		}
	}
	return len(pa) < len(pb)
}

func sortLineages(ls []*LibLineage, by string) {
	name := func(l *LibLineage) string { return strings.ToLower(l.Name) }
	rev := strings.HasPrefix(by, "-")
	by = strings.TrimPrefix(by, "-")
	// cmp: <0 a first, >0 b first, 0 tie (then by name, A to Z, either way)
	cmp := func(a, b *LibLineage) int {
		str := func(x, y string) int {
			switch {
			case x == y:
				return 0
			case x == "":
				return 1 // empty last
			case y == "":
				return -1
			}
			return strings.Compare(x, y)
		}
		switch by {
		case "size":
			x, _ := a.Bytes()
			y, _ := b.Bytes()
			return compareDesc(x, y)
		case "changed":
			return timeDesc(a.Changed(), b.Changed())
		case "played":
			return timeDesc(a.Played(), b.Played())
		case "updated":
			return timeDesc(a.updated(), b.updated())
		case "state":
			return compareDesc(-stateRank(a.State()), -stateRank(b.State()))
		case "source":
			return str(a.sources(), b.sources())
		case "used":
			return compareDesc(int64(len(a.UsedIn())), int64(len(b.UsedIn())))
		case "version":
			x, y := a.newestVersion(), b.newestVersion()
			switch {
			case x == y:
				return 0
			case x == "":
				return 1
			case y == "":
				return -1
			case versionLess(y, x):
				return -1
			}
			return 1
		case "author":
			return str(strings.ToLower(a.authors()), strings.ToLower(b.authors()))
		case "tags":
			return str(strings.ToLower(strings.Join(a.Tags, " ")), strings.ToLower(strings.Join(b.Tags, " ")))
		}
		return 0
	}
	sort.SliceStable(ls, func(i, j int) bool {
		a, b := ls[i], ls[j]
		if c := cmp(a, b); c != 0 {
			if rev {
				return c > 0
			}
			return c < 0
		}
		if by == "name" && rev {
			if name(a) != name(b) {
				return name(a) > name(b)
			}
			return a.ID > b.ID
		}
		if name(a) != name(b) {
			return name(a) < name(b)
		}
		return a.ID < b.ID
	})
}

func compareDesc(x, y int64) int {
	switch {
	case x > y:
		return -1
	case x < y:
		return 1
	}
	return 0
}

func timeDesc(x, y time.Time) int {
	switch {
	case x.Equal(y):
		return 0
	case x.After(y):
		return -1
	}
	return 1
}

// stateRank: the most important state is the highest; no state is the lowest.
func stateRank(s string) int64 {
	if s == "" {
		return -1
	}
	return int64(len(LibStates) - slices.Index(LibStates, s))
}

func (l *LibLineage) updated() time.Time {
	var t time.Time
	for _, i := range l.Instances {
		if i.Updated.After(t) {
			t = i.Updated
		}
	}
	return t
}

// sources are the lineage's copies' sources in order, each once ("workshop local").
func (l *LibLineage) sources() string {
	var out []string
	for _, i := range l.Instances {
		if !slices.Contains(out, i.Source) {
			out = append(out, i.Source)
		}
	}
	return strings.Join(out, " ")
}

func (l *LibLineage) newestVersion() string {
	best := ""
	for _, i := range l.Instances {
		for _, v := range i.Versions {
			if best == "" || versionLess(best, v) {
				best = v
			}
		}
	}
	return best
}

func (l *LibLineage) authors() string {
	for _, i := range l.Instances {
		if i.Authors != "" {
			return i.Authors
		}
	}
	return ""
}

// Lineage finds a lineage by reference: L#…, ws:… for a missing item, or any instance
// reference.
func (lib *Library) Lineage(ref string) (*LibLineage, error) {
	for _, l := range lib.Lineages {
		if l.Ref == ref {
			return l, nil
		}
	}
	ix := lib.St.Ix
	var id string
	if strings.HasPrefix(ref, "L#") {
		var err error
		if id, err = ix.ResolveLineage(ref); err != nil {
			return nil, err
		}
	} else {
		m, err := ix.ResolveOne(ref)
		if err != nil {
			return nil, err
		}
		id = m.LineageID
	}
	for _, l := range lib.Lineages {
		if l.ID == id {
			return l, nil
		}
	}
	return nil, fmt.Errorf("%s is not in the library (official content and folders RimArcho no longer sees are left out)", ref)
}

// Instance finds an instance of the library by its reference.
func (lib *Library) Instance(ref string) *LibInstance {
	for _, l := range lib.Lineages {
		for _, i := range l.Instances {
			if i.Ref == ref || i.Mod != nil && i.Mod.ID == ref {
				return i
			}
		}
	}
	return nil
}
