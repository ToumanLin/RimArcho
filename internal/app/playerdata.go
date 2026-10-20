package app

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/store"
)

// Player data on lineages (04 §7.1): tags, alias and note are shared by every instance of
// a lineage. These are the typed calls the interfaces use (08 §1.2); the CLI's tag, alias
// and note commands write the same store rows.

// SetTags adds tags to the mods (or removes them); copies of one lineage are written once.
// It returns how many tag rows changed.
func (a *App) SetTags(refs, tags []string, remove bool) (int, error) {
	ix, err := a.Index()
	if err != nil {
		return 0, err
	}
	var mods []*store.Mod
	for _, r := range refs {
		m, err := ix.ResolveOne(r)
		if err != nil {
			return 0, err
		}
		mods = append(mods, m)
	}
	n := 0
	seen := map[string]bool{}
	for _, m := range mods {
		if seen[m.LineageID] {
			continue
		}
		seen[m.LineageID] = true
		for _, t := range tags {
			if t = strings.TrimSpace(t); t == "" {
				continue
			}
			var ok bool
			if remove {
				ok, err = a.Store.RemoveTag(ix.Env.ID, m.LineageID, t)
			} else {
				ok, err = a.Store.AddTag(ix.Env.ID, m.LineageID, t)
			}
			if err != nil {
				return n, err
			}
			if ok {
				n++
			}
		}
	}
	return n, nil
}

// SetAlias sets the name RimArcho shows for a mod ("" clears it).
func (a *App) SetAlias(ref, alias string) error { return a.setText(ref, alias, a.Store.SetAlias) }

// SetNote attaches a plain-text note to a mod ("" clears it).
func (a *App) SetNote(ref, note string) error { return a.setText(ref, note, a.Store.SetNote) }

func (a *App) setText(ref, v string, set func(env, lin, v string) error) error {
	ix, err := a.Index()
	if err != nil {
		return err
	}
	m, err := ix.ResolveOne(ref)
	if err != nil {
		return err
	}
	return set(ix.Env.ID, m.LineageID, strings.TrimSpace(v))
}
