package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Following a Steam Workshop collection (07 §7.6): a plan remembers a collection and a
// snapshot of its items. Changes are measured against the snapshot, not against the plan,
// so a collection mod the player disabled is not offered again. RimArcho reads the
// collection only when asked (the list management page, at most every CheckEvery, or
// `plan sync`), and changes the plan only when the player catches up with it.

// CheckEvery is how often the list management page reads a followed collection again.
const CheckEvery = 10 * time.Minute

// Follow modes.
const (
	FollowAdd    = "add"    // catching up enables the new installed mods
	FollowFollow = "follow" // and also disables the mods the collection dropped
)

// Followed is a followed collection with what changed since its snapshot.
type Followed struct {
	*store.PlanCollection
	Added, Removed []store.CollectionItem // mods (not nested collections)
	// Arrived are the items no installed copy had at the last catch-up (or the link) that
	// are installed now and that the plan does not hold: catching up enables them.
	Arrived []store.CollectionItem
	// NotInstalled are the collection's mods no installed copy has (Seen, else Snapshot).
	NotInstalled []store.CollectionItem
}

// Changed says whether the collection changed since the snapshot.
func (f *Followed) Changed() bool { return len(f.Added)+len(f.Removed)+len(f.Arrived) > 0 }

// Items is the newest list RimArcho has of the collection.
func (f *Followed) Items() []store.CollectionItem {
	if !f.Checked.IsZero() {
		return f.Seen
	}
	return f.Snapshot
}

func checkMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", FollowAdd:
		return FollowAdd, nil
	case FollowFollow:
		return FollowFollow, nil
	}
	return "", fmt.Errorf("mode %q: add (enable the new mods) or follow (also disable the ones it dropped)", mode)
}

func itemsOf(c *Collection) []store.CollectionItem {
	out := make([]store.CollectionItem, 0, len(c.Items))
	for _, it := range c.Items {
		out = append(out, store.CollectionItem{WorkshopID: it.WorkshopID, Title: it.Title, Sub: it.Sub})
	}
	return out
}

// installedByWorkshop maps workshop IDs to an installed copy (a loadable one first).
func installedByWorkshop(st *LoadoutState) map[string]*store.Mod {
	out := map[string]*store.Mod{}
	for _, m := range st.Ix.Mods {
		w := m.WorkshopID()
		if w == "" || m.Status != identity.Live {
			continue
		}
		if prev := out[w]; prev == nil || (whyNotLoadable(st, m) == "" && whyNotLoadable(st, prev) != "") {
			out[w] = m
		}
	}
	return out
}

// installedFor is the installed copy of each Workshop ID that the plan can enable.
func installedFor(st *LoadoutState) map[string]*store.Mod {
	out := installedByWorkshop(st)
	for w, m := range out {
		if whyNotLoadable(st, m) != "" {
			delete(out, w)
		}
	}
	return out
}

// snapshotOf is the collection's items as a snapshot: each records whether an installed
// copy the plan can enable was missing (07 §7.6).
func snapshotOf(st *LoadoutState, items []store.CollectionItem) []store.CollectionItem {
	inst := installedFor(st)
	out := make([]store.CollectionItem, len(items))
	for i, it := range items {
		it.Missing = nil
		if !it.Sub {
			missing := inst[it.WorkshopID] == nil
			it.Missing = &missing
		}
		out[i] = it
	}
	return out
}

// planPackages are the lower-cased packages a plan holds, in any copy or as a missing entry.
func planPackages(st *LoadoutState, l loadout.List) map[string]bool {
	out := make(map[string]bool, len(l))
	for _, e := range l {
		if m := st.Ix.ByID[e.Instance]; m != nil {
			out[strings.ToLower(m.PackageID)] = true
		}
		if e.Missing != "" {
			out[strings.ToLower(strings.TrimSuffix(e.Missing, "_steam"))] = true
		}
	}
	return out
}

// waited says whether a snapshot item was missing when the snapshot was taken, so following
// the collection never enabled it. A snapshot from before that was recorded counts the
// copies first seen after the collection was linked.
func waited(it store.CollectionItem, m *store.Mod, linked time.Time) bool {
	if it.Missing != nil {
		return *it.Missing
	}
	return m.FirstSeen.After(linked)
}

// followedOf adds the changes and the uninstalled items to a stored collection; list is
// the plan's (for what arrived).
func followedOf(st *LoadoutState, list loadout.List, c *store.PlanCollection) *Followed {
	f := &Followed{PlanCollection: c}
	if !c.Checked.IsZero() {
		had, now := map[string]bool{}, map[string]bool{}
		for _, it := range c.Snapshot {
			had[it.WorkshopID] = true
		}
		for _, it := range c.Seen {
			now[it.WorkshopID] = true
			if !it.Sub && !had[it.WorkshopID] {
				f.Added = append(f.Added, it)
			}
		}
		for _, it := range c.Snapshot {
			if !it.Sub && !now[it.WorkshopID] {
				f.Removed = append(f.Removed, it)
			}
		}
	}
	inst := installedByWorkshop(st)
	for _, it := range f.Items() {
		if !it.Sub && inst[it.WorkshopID] == nil {
			f.NotInstalled = append(f.NotInstalled, it)
		}
	}
	snap := make(map[string]store.CollectionItem, len(c.Snapshot))
	for _, it := range c.Snapshot {
		snap[it.WorkshopID] = it
	}
	ready, have := installedFor(st), planPackages(st, list)
	for _, it := range f.Items() {
		s, ok := snap[it.WorkshopID]
		m := ready[it.WorkshopID]
		if it.Sub || !ok || m == nil || have[strings.ToLower(m.PackageID)] || !waited(s, m, c.Linked) {
			continue
		}
		f.Arrived = append(f.Arrived, it)
	}
	return f
}

// followList is the list of plan p (st's own when p is st's plan).
func (a *App) followList(st *LoadoutState, p *store.Plan) loadout.List {
	ps, err := a.PlanState(st, p)
	if err != nil {
		return nil
	}
	return ps.List
}

// PlanFollowed lists what a plan follows, from what RimArcho stored (no network).
func (a *App) PlanFollowed(st *LoadoutState, p *store.Plan) ([]*Followed, error) {
	cs, err := a.Store.PlanCollections(p.ID)
	if err != nil {
		return nil, err
	}
	out := make([]*Followed, 0, len(cs))
	var list loadout.List
	if len(cs) > 0 {
		list = a.followList(st, p)
	}
	for _, c := range cs {
		out = append(out, followedOf(st, list, c))
	}
	return out, nil
}

func (a *App) planFollow(st *LoadoutState, p *store.Plan, ref string) (*store.PlanCollection, error) {
	id, err := CollectionID(ref)
	if err != nil {
		return nil, err
	}
	cs, err := a.Store.PlanCollections(p.ID)
	if err != nil {
		return nil, err
	}
	for _, c := range cs {
		if c.WorkshopID == id {
			return c, nil
		}
	}
	return nil, fmt.Errorf("%s does not follow collection ws:%s (archo plan follow %s %s)", PlanRef(p), id, quoteName(p.Name), id)
}

func quoteName(s string) string {
	if strings.ContainsAny(s, " \t\"'") {
		return `"` + s + `"`
	}
	return s
}

// FollowCollection links a collection to a plan (uses the network once): its items now
// are the snapshot, so nothing has changed yet. Linking it again changes the mode only.
func (a *App) FollowCollection(planName, ref, mode string) (*Followed, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	p, err := a.findPlan(planName)
	if err != nil {
		return nil, err
	}
	if mode, err = checkMode(mode); err != nil {
		return nil, err
	}
	if c, err := a.planFollow(st, p, ref); err == nil {
		c.Mode = mode
		if err := a.Store.SetPlanCollection(c); err != nil {
			return nil, err
		}
		return followedOf(st, a.followList(st, p), c), nil
	}
	col, err := a.ReadCollection(ref)
	if err != nil {
		return nil, err
	}
	return a.followRead(st, p, col, mode)
}

// FollowReadCollection links a collection already read (an import that ticked "follow
// this collection", 07 §7.1) to a plan, without reading it again.
func (a *App) FollowReadCollection(p *store.Plan, col *Collection, mode string) (*Followed, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	if mode, err = checkMode(mode); err != nil {
		return nil, err
	}
	return a.followRead(st, p, col, mode)
}

func (a *App) followRead(st *LoadoutState, p *store.Plan, col *Collection, mode string) (*Followed, error) {
	now := a.Now().UTC()
	title := col.Title
	if title == "" {
		title = "ws:" + col.ID
	}
	items := itemsOf(col)
	c := &store.PlanCollection{PlanID: p.ID, WorkshopID: col.ID, Title: title, Mode: mode, Linked: now, Checked: now, Snapshot: snapshotOf(st, items), Seen: items}
	if err := a.Store.SetPlanCollection(c); err != nil {
		return nil, err
	}
	return followedOf(st, a.followList(st, p), c), nil
}

// UnfollowCollection unlinks a collection; the plan is unchanged.
func (a *App) UnfollowCollection(planName, ref string) (*store.PlanCollection, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	p, err := a.findPlan(planName)
	if err != nil {
		return nil, err
	}
	c, err := a.planFollow(st, p, ref)
	if err != nil {
		return nil, err
	}
	_, err = a.Store.DeletePlanCollection(p.ID, c.WorkshopID)
	return c, err
}

// CheckFollowed reads a plan's followed collections again (the network) and records what
// they hold now; ref "" checks every one. Without force, one checked within CheckEvery is
// not read again. A collection that cannot be read keeps what was stored; its error is
// returned with the others.
func (a *App) CheckFollowed(planName, ref string, force bool) ([]*Followed, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	p, err := a.findPlan(planName)
	if err != nil {
		return nil, err
	}
	return a.checkFollowed(st, p, ref, force)
}

func (a *App) checkFollowed(st *LoadoutState, p *store.Plan, ref string, force bool) ([]*Followed, error) {
	var cs []*store.PlanCollection
	if ref != "" {
		c, err := a.planFollow(st, p, ref)
		if err != nil {
			return nil, err
		}
		cs = []*store.PlanCollection{c}
	} else {
		var err error
		if cs, err = a.Store.PlanCollections(p.ID); err != nil {
			return nil, err
		}
	}
	now := a.Now().UTC()
	var errs []error
	out := make([]*Followed, 0, len(cs))
	list := a.followList(st, p)
	for _, c := range cs {
		if force || c.Checked.IsZero() || now.Sub(c.Checked) >= CheckEvery {
			col, err := a.readCollection(c.WorkshopID, knownTitles(c))
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", c.Title, err))
			} else {
				c.Seen, c.Checked = itemsOf(col), now
				if col.Title != "" {
					c.Title = col.Title
				}
				if err := a.Store.SetPlanCollection(c); err != nil {
					return nil, err
				}
			}
		}
		out = append(out, followedOf(st, list, c))
	}
	return out, errors.Join(errs...)
}

// knownTitles are the item titles a followed collection already has.
func knownTitles(c *store.PlanCollection) map[string]string {
	out := make(map[string]string, len(c.Seen)+len(c.Snapshot))
	for _, l := range [][]store.CollectionItem{c.Snapshot, c.Seen} {
		for _, it := range l {
			if it.Title != "" {
				out[it.WorkshopID] = it.Title
			}
		}
	}
	return out
}

// SyncResult is what catching up with a collection did (or would do).
type SyncResult struct {
	Plan       *store.Plan
	Collection *Followed // as it was before catching up
	Enabled    []*store.Mod
	Disabled   []*store.Mod
	// NotInstalled are new items no installed copy has (listed, not added).
	NotInstalled []store.CollectionItem
	Before       loadout.List
	After        loadout.List
	DryRun       bool
}

// SyncCollection catches a plan up with a followed collection (07 §7.6): it reads the
// collection when the last check is older than CheckEvery, enables the new installed
// mods where the sort constraints suggest, and in follow mode disables the mods the
// collection dropped; one undoable edit of that plan, named "sync collection <title>".
// The snapshot becomes what the collection holds now. ref "" needs exactly one followed
// collection. dryRun changes nothing.
func (a *App) SyncCollection(planName, ref string, dryRun bool) (*SyncResult, error) {
	return a.syncCollection(planName, ref, dryRun, true)
}

// SyncCollectionShown catches up with the collection as RimArcho last read it, without the
// network: the changes the player saw on the card (16 §2.17) are the ones made, however
// long ago it was read.
func (a *App) SyncCollectionShown(planName, ref string) (*SyncResult, error) {
	return a.syncCollection(planName, ref, false, false)
}

func (a *App) syncCollection(planName, ref string, dryRun, read bool) (*SyncResult, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	p, err := a.findPlan(planName)
	if err != nil {
		return nil, err
	}
	if ref == "" {
		cs, err := a.Store.PlanCollections(p.ID)
		if err != nil {
			return nil, err
		}
		switch len(cs) {
		case 0:
			return nil, fmt.Errorf("%s follows no collection (archo plan follow %s <link>)", PlanRef(p), quoteName(p.Name))
		case 1:
			ref = cs[0].WorkshopID
		default:
			return nil, fmt.Errorf("%s follows %d collections: name one (archo plan sync %s <ID>)", PlanRef(p), len(cs), quoteName(p.Name))
		}
	}
	ps, err := a.PlanState(st, p)
	if err != nil {
		return nil, err
	}
	var f *Followed
	if read {
		fs, err := a.checkFollowed(st, p, ref, false)
		if err != nil {
			return nil, err
		}
		f = fs[0]
	} else {
		c, err := a.planFollow(st, p, ref)
		if err != nil {
			return nil, err
		}
		f = followedOf(st, ps.List, c)
	}
	r := &SyncResult{Plan: p, Collection: f, Before: ps.List.Clone(), DryRun: dryRun}
	enable, removed := map[string]bool{}, map[string]bool{}
	for _, it := range f.Added {
		enable[it.WorkshopID] = true
	}
	for _, it := range f.Arrived {
		enable[it.WorkshopID] = true
	}
	for _, it := range f.Removed {
		removed[it.WorkshopID] = true
	}
	after := collectionMerge(st, ps, f, enable, r)
	wsOf := func(e loadout.Entry) string {
		if m := st.Ix.ByID[e.Instance]; m != nil {
			return m.WorkshopID()
		}
		return ""
	}
	if f.Mode == FollowFollow && len(removed) > 0 {
		kept := after[:0:0]
		for _, e := range after {
			if removed[wsOf(e)] {
				r.Disabled = append(r.Disabled, st.Ix.ByID[e.Instance])
				continue
			}
			kept = append(kept, e)
		}
		after = kept
	}
	inst := installedByWorkshop(st)
	for _, it := range f.Added {
		if inst[it.WorkshopID] == nil {
			r.NotInstalled = append(r.NotInstalled, it)
		}
	}
	r.After = after
	if dryRun {
		return r, nil
	}
	if err := a.editPlan(p, ps.WS, r.Before, after, "sync collection "+f.Title); err != nil {
		a.logSync(r, err)
		return nil, err
	}
	a.logSync(r, nil)
	if ps == st {
		st.List = after
	}
	f.Snapshot = snapshotOf(st, f.Seen)
	if err := a.Store.SetPlanCollection(f.PlanCollection); err != nil {
		return nil, err
	}
	return r, nil
}

// collectionMerge enables the installed items of enable in the collection's order, each
// next to its neighbours there, and records them in r.Enabled.
func collectionMerge(st, ps *LoadoutState, f *Followed, enable map[string]bool, r *SyncResult) loadout.List {
	il := newList("collection")
	for _, it := range f.Items() {
		if !it.Sub {
			il.WorkshopOnly = append(il.WorkshopOnly, it.WorkshopID)
		}
	}
	wsOf := func(e loadout.Entry) string {
		if m := st.Ix.ByID[e.Instance]; m != nil {
			return m.WorkshopID()
		}
		return ""
	}
	after := mergeEntries(ps, il, func(e loadout.Entry) bool { return enable[wsOf(e)] })
	for _, e := range after {
		if enable[wsOf(e)] && r.Before.Index(e.Instance) < 0 {
			r.Enabled = append(r.Enabled, st.Ix.ByID[e.Instance])
		}
	}
	return after
}

// CatchUpDownloaded enables, in plan planName, the items of a followed collection that a
// finished download brought (ids): those that arrived since the last catch-up (07 §7.6),
// in the collection's order. Nothing else changes: the other changes still wait for the
// player's catch-up, and only these items' snapshot records say they are no longer missing.
// The player's own "subscribe all" on the collection's card asks for it (16 §2.17).
func (a *App) CatchUpDownloaded(planName, ref string, ids []string) (*SyncResult, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	p, err := a.findPlan(planName)
	if err != nil {
		return nil, err
	}
	c, err := a.planFollow(st, p, ref)
	if err != nil {
		return nil, err
	}
	ps, err := a.PlanState(st, p)
	if err != nil {
		return nil, err
	}
	f := followedOf(st, ps.List, c)
	r := &SyncResult{Plan: p, Collection: f, Before: ps.List.Clone()}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	enable := map[string]bool{}
	for _, it := range f.Arrived {
		if want[it.WorkshopID] {
			enable[it.WorkshopID] = true
		}
	}
	if len(enable) == 0 {
		return r, nil
	}
	after := collectionMerge(st, ps, f, enable, r)
	r.After = after
	if err := a.editPlan(p, ps.WS, r.Before, after, "add downloaded from "+f.Title); err != nil {
		return nil, err
	}
	if ps == st {
		st.List = after
	}
	f2 := snapshotOf(st, c.Snapshot)
	for i := range c.Snapshot {
		if enable[c.Snapshot[i].WorkshopID] {
			c.Snapshot[i].Missing = f2[i].Missing
		}
	}
	if err := a.Store.SetPlanCollection(c); err != nil {
		return nil, err
	}
	return r, nil
}
