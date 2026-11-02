package app

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// attributeExternal handles a ModsConfig.xml whose hash is neither the one RimArcho last
// saw nor the one it last wrote (04 §4.3). The change belongs to the plan that last wrote
// the file — possibly another plan of this environment, or a plan of another environment
// sharing the user data folder — with entries resolved by the game that wrote it (§4.2).
// Without a usable record it belongs to the current plan. A file that only differs in
// format from the version the file is known to hold is no change.
//
// The record then names that plan and the new version as what the file holds, so the next
// check compares against it. mcs is updated in place; the caller stores it.
func (a *App) attributeExternal(st *LoadoutState, mcs *store.ModsConfigState, h []byte, now time.Time) error {
	target, tenv, reg, err := a.writerPlan(st, mcs)
	if err != nil {
		return err
	}
	list, _ := reg.FromTokens(st.File.Active)
	read := func(entries string) loadout.List {
		return reg.Heal(a.localizer(tenv.ID, reg, nil)(loadout.Decode(entries)))
	}
	// The version the file was known to hold: the writer's, else (no record) the newest.
	var ref *store.Version
	if target.ID == mcs.WriterPlan && mcs.WriterVersion > 0 {
		if ref, err = a.Store.Version(target.ID, mcs.WriterVersion); err != nil {
			return err
		}
	}
	if ref == nil {
		if ref, err = a.Store.LatestVersion(target.ID); err != nil {
			return err
		}
	}
	mcs.Seen = h
	if ref != nil && ref.GameVersion == st.File.Version && read(ref.Entries).Equal(list) {
		// Rewritten by the game or another tool without a real change (formatting).
		return nil
	}
	ws, err := a.planWorkspace(target)
	if err != nil {
		return err
	}
	cur := read(ws.Entries)
	clean := ref == nil || cur.Equal(read(ref.Entries))
	n, err := a.Store.AddVersion(target.ID, &store.Version{Created: now, Source: "external", Entries: list.Encode(),
		GameVersion: st.File.Version, Note: "ModsConfig.xml changed outside RimArcho"})
	if err != nil {
		return err
	}
	c := &ExternalConflict{Plan: target, Env: tenv, Version: n, Current: target.ID == st.Plan.ID}
	if clean {
		ws.Entries, ws.OriginVersion, ws.OriginKind, ws.Conflict = list.Encode(), n, "external", 0
		note := fmt.Sprintf("ModsConfig.xml changed outside RimArcho: loaded as %s", c.Name(st.Ix.Env))
		// Mods the change left out are said, here and on the can it run page, until the
		// player keeps the change or puts the list back (VM run B8: the game resets its
		// list when mods fail to load).
		if dropped := removedEntries(cur, list, st.LineageOf); ref != nil && dropped > 0 {
			if err := a.Store.SetMeta(droppedKey(target.ID), fmt.Sprintf("%d %d", ref.N, n)); err != nil {
				return err
			}
			note += fmt.Sprintf("; %d mods left the list (v%d has them)", dropped, ref.N)
		}
		st.Notices = append(st.Notices, note)
	} else {
		ws.Conflict = n
		st.Notices = append(st.Notices, fmt.Sprintf("ModsConfig.xml changed outside RimArcho: kept as %s, not loaded because the plan changed since it was saved (%s)",
			c.Name(st.Ix.Env), strings.Join(c.Commands(st.Ix.Env), "; ")))
	}
	ws.Updated = now
	if clean {
		err = a.Store.EditWorkspace(ws, cur.Encode(), "external", nil)
	} else {
		err = a.Store.SetWorkspace(ws)
	}
	if err != nil {
		return err
	}
	mcs.WriterPlan, mcs.WriterVersion, mcs.WriterEnv = target.ID, n, tenv.ID
	return nil
}

// writerPlan is the plan an external change of the folder's ModsConfig.xml belongs to, its
// environment and that environment's registry: the record's writer when it still exists,
// else the current plan.
func (a *App) writerPlan(st *LoadoutState, mcs *store.ModsConfigState) (*store.Plan, *env.Environment, *loadout.Registry, error) {
	if mcs.WriterPlan == 0 || mcs.WriterPlan == st.Plan.ID && (mcs.WriterEnv == "" || mcs.WriterEnv == st.Ix.Env.ID) {
		return st.Plan, st.Ix.Env, st.Reg, nil
	}
	p, err := a.Store.PlanByID(mcs.WriterPlan)
	if err != nil {
		return nil, nil, nil, err
	}
	if p == nil { // the writer was deleted
		return st.Plan, st.Ix.Env, st.Reg, nil
	}
	// Lists are shared by every game: the entries are read as the game that wrote them.
	if mcs.WriterEnv == "" || mcs.WriterEnv == st.Ix.Env.ID {
		return p, st.Ix.Env, st.Reg, nil
	}
	e, err := a.Store.EnvByID(mcs.WriterEnv)
	if err != nil {
		return nil, nil, nil, err
	}
	if e == nil {
		return p, st.Ix.Env, st.Reg, nil
	}
	ix, err := a.loadIndex(e)
	if err != nil {
		return nil, nil, nil, err
	}
	reg, _, _, _, err := a.registry(ix)
	if err != nil {
		return nil, nil, nil, err
	}
	return p, e, reg, nil
}

// ExternalConflict is a plan's external ModsConfig.xml version that was not loaded because
// the plan had changed since it was written (04 §4.3: "an outside version has not been loaded").
type ExternalConflict struct {
	Plan    *store.Plan
	Env     *env.Environment // the plan's environment
	Version int
	// Current: the plan is the current plan of the invocation's environment.
	Current bool
}

// Name is "plan:<name> v<n>", with the environment first when it is not from.
func (c *ExternalConflict) Name(from *env.Environment) string {
	s := fmt.Sprintf("%s v%d", PlanRef(c.Plan), c.Version)
	if from == nil || c.Env.ID != from.ID {
		s = "env:" + c.Env.Name + " " + s
	}
	return s
}

// Commands are the three ways out — see the difference, load it, ignore it — runnable from
// the environment from: a plan that is not the current one is selected first.
func (c *ExternalConflict) Commands(from *env.Environment) []string {
	pre := "archo "
	var out []string
	if from == nil || c.Env.ID != from.ID {
		pre = "archo --env " + quoteArg(c.Env.Name) + " "
	}
	if !c.Current || from == nil || c.Env.ID != from.ID {
		out = append(out, pre+"plan use "+quoteArg(c.Plan.Name))
	}
	return append(out,
		fmt.Sprintf("%sloadout diff v%d workspace", pre, c.Version),
		fmt.Sprintf("%sloadout checkout v%d", pre, c.Version),
		pre+"loadout ignore-external")
}

// Conflicts lists the plans with an unloaded external version, the current plan first.
func (a *App) Conflicts(st *LoadoutState) ([]*ExternalConflict, error) {
	var out []*ExternalConflict
	if st.WS.Conflict != 0 {
		out = append(out, &ExternalConflict{Plan: st.Plan, Env: st.Ix.Env, Version: st.WS.Conflict, Current: true})
	}
	ps, err := a.Store.Plans()
	if err != nil {
		return nil, err
	}
	for _, p := range ps {
		if p.ID == st.Plan.ID {
			continue
		}
		ws, err := a.Store.Workspace(p.ID)
		if err != nil {
			return nil, err
		}
		if ws != nil && ws.Conflict != 0 {
			out = append(out, &ExternalConflict{Plan: p, Env: st.Ix.Env, Version: ws.Conflict})
		}
	}
	return out, nil
}

// ErrExternalConflict is wrapped by writes refused over an unloaded external version.
var ErrExternalConflict = errors.New("unloaded external change")

// writeBlocker is the unloaded external version a write of st would overwrite unseen
// (05 I8): the plan's own, or the one ModsConfig.xml still holds for another plan.
func (a *App) writeBlocker(st *LoadoutState) (*ExternalConflict, error) {
	if st.WS.Conflict != 0 {
		return &ExternalConflict{Plan: st.Plan, Env: st.Ix.Env, Version: st.WS.Conflict, Current: true}, nil
	}
	if st.File == nil {
		return nil, nil
	}
	mcs, err := a.Store.ModsConfigStateAt(st.Ix.Env.UserDataRoot)
	if err != nil || mcs.WriterPlan == 0 || mcs.WriterPlan == st.Plan.ID || !bytes.Equal(mcs.Seen, loadout.Hash(st.File.Raw)) {
		return nil, err
	}
	ws, err := a.Store.Workspace(mcs.WriterPlan)
	if err != nil || ws == nil || ws.Conflict == 0 || ws.Conflict != mcs.WriterVersion {
		return nil, err
	}
	p, err := a.Store.PlanByID(mcs.WriterPlan)
	if err != nil || p == nil {
		return nil, err
	}
	e := st.Ix.Env
	if mcs.WriterEnv != "" && mcs.WriterEnv != e.ID {
		if o, err := a.Store.EnvByID(mcs.WriterEnv); err != nil {
			return nil, err
		} else if o != nil {
			e = o
		}
	}
	return &ExternalConflict{Plan: p, Env: e, Version: ws.Conflict}, nil
}

// conflictError is the refusal of a write over an unloaded external version.
func conflictError(st *LoadoutState, c *ExternalConflict) error {
	cmds := c.Commands(st.Ix.Env)
	n := len(cmds)
	how := fmt.Sprintf("see the difference: %s; load it: %s; ignore it: %s", cmds[n-3], cmds[n-2], cmds[n-1])
	if n == 4 {
		how = "first " + cmds[0] + ", then " + how
	}
	return fmt.Errorf("%w: ModsConfig.xml changed outside RimArcho and %s was not loaded; nothing was written (%s)",
		ErrExternalConflict, c.Name(st.Ix.Env), how)
}

// IgnoreExternal clears the current plan's "unloaded external version" mark (04 §4.3,
// `loadout ignore-external`); the version stays in the history. It returns the version
// that was ignored, 0 when there was none.
func (a *App) IgnoreExternal() (int, *LoadoutState, error) {
	unlock, err := a.lockLaunch()
	if err != nil {
		return 0, nil, err
	}
	defer unlock()

	st, err := a.Loadout()
	if err != nil {
		return 0, nil, err
	}
	n := st.WS.Conflict
	if n == 0 {
		return 0, st, nil
	}
	st.WS.Conflict = 0
	return n, st, a.Store.SetWorkspace(st.WS)
}

// DismissConflict is IgnoreExternal for the UI's "ignore it" (06 §2.5).
func (a *App) DismissConflict() error {
	_, _, err := a.IgnoreExternal()
	return err
}

// removedEntries counts the entries of before that after does not have.
func removedEntries(before, after loadout.List, lineage func(string) string) int {
	n := 0
	for _, c := range loadout.Diff(before, after, lineage) {
		if c.Kind == loadout.Removed {
			n++
		}
	}
	return n
}

func droppedKey(plan int64) string { return fmt.Sprintf("external-dropped:%d", plan) }

// DroppedByOutsideChange: the list was loaded from an outside change (to) that left out mods
// of the version before it (from), and the player has not answered yet. 0, 0 otherwise: no
// such change, or the list was edited, checked out or loaded again since.
func (a *App) DroppedByOutsideChange(st *LoadoutState) (from, to int) {
	if st.WS == nil || st.WS.OriginKind != "external" || st.WS.Conflict != 0 {
		return 0, 0
	}
	if _, err := fmt.Sscanf(a.Store.Meta(droppedKey(st.Plan.ID)), "%d %d", &from, &to); err != nil || to != st.WS.OriginVersion {
		return 0, 0
	}
	if v, err := a.Store.Version(st.Plan.ID, to); err != nil || v == nil || v.Entries != st.WS.Entries {
		return 0, 0
	}
	return from, to
}

// KeepOutsideChange: the player keeps the list an outside change left mods out of; the
// question is not asked again.
func (a *App) KeepOutsideChange() error {
	st, err := a.Loadout()
	if err != nil {
		return err
	}
	return a.Store.SetMeta(droppedKey(st.Plan.ID), "")
}
