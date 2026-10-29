package app

import (
	"path/filepath"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// RunSource is what wrote a log or plays a running game (03 §6.4, 08 §4): the newest launch
// record that started no later than the moment, else the ModsConfig.xml write record
// before it (a game started from Steam plays what was last written), else unknown.
type RunSource struct {
	Launch  *store.LaunchRecord // the launch it maps to (nil: none)
	ByWrite bool                // no launch: attributed through the write record (04 §4.3)
	At      time.Time           // the launch's start or the write's time
	EnvID   string              // the launch's environment ("" when attributed by the write)
	PlanID  int64               // the plan played or written (0: a pack, or unknown)
	Plan    string              // its name ("" when the plan is gone)
	Pack    string              // the pack played
	// Fingerprint is the played list's ListFingerprint ("" when not known).
	Fingerprint string
	// VersionN is the plan version played or written (0: not known).
	VersionN int
	Unknown  bool // neither a launch nor a write matches
}

// sharedEnvs are the environments whose game writes into e's user data folder: e and
// every environment sharing that folder (02 §2.4).
func (a *App) sharedEnvs(e *env.Environment) []string {
	ids := []string{e.ID}
	if e.UserDataRoot == "" {
		return ids
	}
	envs, err := a.Store.Envs()
	if err != nil {
		return ids
	}
	key := store.UserDataKey(e.UserDataRoot)
	for _, o := range envs {
		if o.ID != e.ID && store.UserDataKey(o.UserDataRoot) == key {
			ids = append(ids, o.ID)
		}
	}
	return ids
}

// sourceAt says which launch or write of e's user data folder a moment t belongs to.
// plansOnly leaves pack launches out: on 1.7 a pack's game writes its own Logs/, never the
// environment's (08 §5.3). reg reads a written version's list for its fingerprint (nil:
// not needed).
func (a *App) sourceAt(e *env.Environment, reg *loadout.Registry, t time.Time, plansOnly bool) *RunSource {
	src := &RunSource{}
	ids := a.sharedEnvs(e)
	var l *store.LaunchRecord
	for at := t; ; {
		r, err := a.Store.LaunchRecordBefore(ids, at)
		if err != nil || r == nil {
			break
		}
		if !plansOnly || r.Pack() == "" {
			l = r
			break
		}
		at = r.Started.Add(-time.Second)
	}
	if l != nil {
		src.Launch, src.At, src.EnvID, src.PlanID, src.Pack, src.Fingerprint = l, l.Started, l.EnvID, l.PlanID, l.Pack(), l.Fingerprint
		src.VersionN = l.VersionN
	} else if w, err := a.Store.ModsConfigStateAt(e.UserDataRoot); err == nil && w.WriterPlan != 0 &&
		!w.WrittenAt.IsZero() && !w.WrittenAt.After(t) {
		src.ByWrite, src.At, src.PlanID = true, w.WrittenAt, w.WriterPlan
		if p, _ := a.Store.PlanByID(w.WriterPlan); p != nil {
			src.EnvID = w.WriterEnv
			if reg != nil && (w.WriterEnv == "" || w.WriterEnv == e.ID) && w.WriterVersion > 0 {
				src.VersionN = w.WriterVersion
				// The written version's list, read as this environment reads it.
				if v, _ := a.Store.Version(p.ID, w.WriterVersion); v != nil {
					src.Fingerprint = ListFingerprint(reg.Heal(loadout.Decode(v.Entries)))
				}
			}
		}
	} else {
		src.Unknown = true
		return src
	}
	if src.PlanID != 0 {
		if p, _ := a.Store.PlanByID(src.PlanID); p != nil {
			src.Plan = p.Name
		}
	}
	return src
}

// logSource is sourceAt for a log file: a log under a Logs/ folder of the user data
// folder (1.7 and later) cannot come from a pack's game.
func (a *App) logSource(st *LoadoutState, path string, mod time.Time) *RunSource {
	root := st.Ix.Env.UserDataRoot
	plansOnly := root != "" && path != "" && env.PathKey(filepath.Dir(path)) == env.PathKey(filepath.Join(root, "Logs"))
	return a.sourceAt(st.Ix.Env, st.Reg, mod, plansOnly)
}

// judge sets r's attribution and staleness against st's plan (03 §6.4 new rule): the log
// is stale when it was written by another game version, when its run is not st's plan
// (another plan, another environment or a pack), when the plan's enabled list changed
// since that run, or when no launch or write matches it.
func (a *App) judge(st *LoadoutState, r *LogRun) {
	r.OtherGame = r.MM != "" && r.EnvMM != "" && r.MM != r.EnvMM
	src := a.logSource(st, r.Path, r.ModTime)
	r.Source = src
	switch {
	case src.Unknown:
		r.Unknown = true
	case src.Pack != "":
		r.OtherRun = true
	case src.EnvID != "" && src.EnvID != st.Ix.Env.ID:
		r.OtherRun = true
		if envs, err := a.Store.Envs(); err == nil {
			for _, o := range envs {
				if o.ID == src.EnvID {
					r.OtherEnv = o.Name
				}
			}
		}
	case st.Plan == nil || src.PlanID != st.Plan.ID:
		r.OtherRun = true
	case src.Fingerprint == "" || src.Fingerprint != ListFingerprint(st.List) && !a.monitorSwapOnly(st, src):
		// A launch from before plans had fingerprints cannot be compared: changed.
		r.Changed = true
	}
}

// monitorSwapOnly reports whether the plan's list differs from the played one only in which
// Archo Monitor version sits at the same place (08 §9 "更新"): the monitor update does not
// change what the run's errors describe, so the log stays counted (03 §6.4). The played
// list is the version the launch or write names, checked against its fingerprint.
func (a *App) monitorSwapOnly(st *LoadoutState, src *RunSource) bool {
	if src.PlanID == 0 || src.VersionN <= 0 {
		return false
	}
	v, err := a.Store.Version(src.PlanID, src.VersionN)
	if err != nil || v == nil {
		return false
	}
	played := loadout.Decode(v.Entries)
	if ListFingerprint(played) != src.Fingerprint {
		if played = st.Reg.Heal(played); ListFingerprint(played) != src.Fingerprint {
			return false
		}
	}
	return monitorBlind(st.Ix, played) == monitorBlind(st.Ix, st.List)
}

// monitorBlind is ListFingerprint with every Archo Monitor entry counted as one key.
func monitorBlind(ix *Index, l loadout.List) string {
	keys := make([]string, 0, len(l))
	for _, e := range l {
		if IsMonitor(ix.ByID[e.Instance]) || e.Instance == "" && packageBase(e.Missing) == MonitorPackage {
			keys = append(keys, "monitor:"+MonitorPackage)
		} else {
			keys = append(keys, e.Key())
		}
	}
	return fingerprintOf(keys)
}
