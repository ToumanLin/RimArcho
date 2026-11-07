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
	// NotInstalled are the collection's mods no installed copy has (Seen, else Snapshot).
	NotInstalled []store.CollectionItem
}

// Changed says whether the collection changed since the snapshot.
func (f *Followed) Changed() bool { return len(f.Added) > 0 || len(f.Removed) > 0 }

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

// followedOf adds the changes and the uninstalled items to a stored collection.
func followedOf(st *LoadoutState, c *store.PlanCollection) *Followed {
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
	return f
}

// PlanFollowed lists what a plan follows, from what RimArcho stored (no network).
func (a *App) PlanFollowed(st *LoadoutState, p *store.Plan) ([]*Followed, error) {
	cs, err := a.Store.PlanCollections(p.ID)
	if err != nil {
		return nil, err
	}
	out := make([]*Followed, 0, len(cs))
	for _, c := range cs {
		out = append(out, followedOf(st, c))
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
		return followedOf(st, c), nil
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
	c := &store.PlanCollection{PlanID: p.ID, WorkshopID: col.ID, Title: title, Mode: mode, Linked: now, Checked: now, Snapshot: items, Seen: items}
	if err := a.Store.SetPlanCollection(c); err != nil {
		return nil, err
	}
	return followedOf(st, c), nil
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
	for _, c := range cs {
		if force || c.Checked.IsZero() || now.Sub(c.Checked) >= CheckEvery {
			col, err := a.ReadCollection(c.WorkshopID)
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
		out = append(out, followedOf(st, c))
	}
	return out, errors.Join(errs...)
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
	fs, err := a.checkFollowed(st, p, ref, false)
	if err != nil {
		return nil, err
	}
	f := fs[0]
	ps, err := a.PlanState(st, p)
	if err != nil {
		return nil, err
	}
	r := &SyncResult{Plan: p, Collection: f, Before: ps.List.Clone(), DryRun: dryRun}
	added, removed := map[string]bool{}, map[string]bool{}
	for _, it := range f.Added {
		added[it.WorkshopID] = true
	}
	for _, it := range f.Removed {
		removed[it.WorkshopID] = true
	}
	// The collection in its order, so a new mod goes next to its neighbours there.
	il := newList("collection")
	for _, it := range f.Seen {
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
	after := mergeEntries(ps, il, func(e loadout.Entry) bool { return added[wsOf(e)] })
	for _, e := range after {
		if added[wsOf(e)] && r.Before.Index(e.Instance) < 0 {
			r.Enabled = append(r.Enabled, st.Ix.ByID[e.Instance])
		}
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
		return nil, err
	}
	if ps == st {
		st.List = after
	}
	f.Snapshot = f.Seen
	if err := a.Store.SetPlanCollection(f.PlanCollection); err != nil {
		return nil, err
	}
	return r, nil
}
