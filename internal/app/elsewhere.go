package app

import (
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// withElsewhere adds the other games' own Mods/ folders to an index (07 §1.7, 16 §2.9):
// the mod library is the whole machine's, and a list can enable those mods in any game —
// saving links them into this game's Mods/ as _Archo_env_<ID>, as switching games does
// (03 §2 item 14). Their copies carry kind Elsewhere, so nothing takes them for this
// game's Mods/. A mod this game already sees at the same folder (an earlier link, read as
// this game's own) is not added twice.
func (a *App) withElsewhere(e *env.Environment, mods []*store.Mod, srcs []*source.Source) ([]*store.Mod, []*source.Source, error) {
	other, err := a.Store.ElsewhereMods(e.ID)
	if err != nil || len(other) == 0 {
		return mods, srcs, err
	}
	seen := map[string]bool{}
	for _, m := range mods {
		if m.Status == identity.Live && m.RealPath != "" {
			seen[env.PathKey(m.RealPath)] = true
		}
	}
	all, err := a.Store.AllSources()
	if err != nil {
		return nil, nil, err
	}
	envs, err := a.Store.Envs()
	if err != nil {
		return nil, nil, err
	}
	names := map[string]string{}
	for _, x := range envs {
		names[x.ID] = x.Name
	}
	bySource := map[string]*source.Source{}
	for _, s := range all {
		if s.Kind == source.Local && s.EnvID != e.ID && s.EnvID != store.Shared && s.Status == source.StatusOK && names[s.EnvID] != "" {
			c := *s
			c.Kind, c.Label = source.Elsewhere, "local-"+names[s.EnvID]
			bySource[s.ID] = &c
		}
	}
	out := mods
	used := map[string]bool{}
	for _, m := range other {
		s := bySource[m.SourceID]
		if s == nil || m.RealPath == "" || m.Status == identity.Live && seen[env.PathKey(m.RealPath)] {
			continue
		}
		if m.Status == identity.Live {
			seen[env.PathKey(m.RealPath)] = true
		}
		m.Kind, m.SourceLabel = source.Elsewhere, s.Label
		out = append(out[:len(out):len(out)], m)
		used[s.ID] = true
	}
	for _, s := range all {
		if used[s.ID] {
			srcs = append(srcs[:len(srcs):len(srcs)], bySource[s.ID])
		}
	}
	return out, srcs, nil
}
