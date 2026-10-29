package app

// IncrementalProblems recomputes declarations after an edit, retaining previous content
// findings until background analysis completes. It reads no Def/Patch facts or log
// entries; log ownership is rechecked from startup headers.
// The caller must match the environment, plan, scan snapshot and game version.
func (a *App) IncrementalProblems(previous *ProblemSet) (*ProblemSet, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	analysis, err := a.Analysis(st)
	if err != nil {
		return nil, err
	}
	ps := &ProblemSet{State: st, Analysis: analysis, GameLinks: previous.GameLinks}
	all, err := a.DeclFindings(st)
	if err != nil {
		return nil, err
	}
	if err = a.finishProblems(ps, all, nil, nil); err != nil {
		return nil, err
	}
	runs := map[*LogRun]*LogRun{}
	judge := func(old *LogRun) *LogRun {
		if a.CheckLogOwner(st, old.Path) != nil {
			return nil
		}
		if r := runs[old]; r != nil {
			return r
		}
		r := *old
		r.Changed = false
		r.Unknown = false
		r.OtherRun = false
		r.OtherEnv = ""
		a.judge(st, &r)
		runs[old] = &r
		return &r
	}
	for _, old := range previous.All() {
		if old.Layer == "decl" && old.Log == nil && old.Code != CodeModsDuplicate && old.Code != CodePackLinks {
			continue
		}
		if id := old.SubjectInstance(); id != "" && st.Position(id) == 0 && old.Log == nil {
			continue
		}
		p := *old
		if old.Log != nil {
			run := judge(old.Log)
			if run == nil {
				continue
			}
			p.Log = run
			if run.Stale() {
				ps.Stale = append(ps.Stale, &p)
				continue
			}
		}
		if p.Ignored {
			ps.Ignored = append(ps.Ignored, &p)
		} else {
			ps.List = append(ps.List, &p)
		}
	}
	for _, old := range previous.Logs {
		if run := judge(old); run != nil {
			ps.Logs = append(ps.Logs, run)
		}
	}
	return ps, nil
}
