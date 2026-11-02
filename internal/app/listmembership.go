package app

import "slices"

// ListMembership is the loaded list catalog and the exact copies enabled by each list.
type ListMembership struct {
	Names      []string
	Current    string
	Edited     string
	ByInstance map[string][]string
}

// ListsFor loads membership once for a board, without per-row database reads.
func (a *App) ListsFor(st *LoadoutState) (*ListMembership, error) {
	envs, err := a.libEnvs(st)
	if err != nil {
		return nil, err
	}
	_, lists, err := a.listUses(st, envs)
	return lists, err
}

func (a *App) listUses(st *LoadoutState, envs []libEnv) (map[string][]LibUse, *ListMembership, error) {
	uses := map[string][]LibUse{}
	lists := &ListMembership{ByInstance: map[string][]string{}}
	if st.Plan != nil {
		lists.Edited = st.Plan.Name
	}
	plans, err := a.Store.Plans()
	if err != nil {
		return nil, nil, err
	}
	here := libHere(envs)
	current := a.Store.CurrentPlanID(st.Ix.Env.ID)
	for _, p := range plans {
		if p.ID == current {
			lists.Current = p.Name
		}
		lists.Names = append(lists.Names, p.Name)
		entries := st.List
		if st.Plan == nil || p.ID != st.Plan.ID {
			if entries, err = a.planList(p); err != nil {
				return nil, nil, err
			}
		}
		u := LibUse{Kind: "plan", Name: p.Name, PlanID: p.ID, Current: st.Plan != nil && p.ID == st.Plan.ID}
		for _, e := range entries {
			id := here(e.Instance)
			if id == "" || slices.Contains(lists.ByInstance[id], p.Name) {
				continue
			}
			lists.ByInstance[id] = append(lists.ByInstance[id], p.Name)
			uses[id] = append(uses[id], u)
		}
	}
	return uses, lists, nil
}
