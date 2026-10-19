package app

import (
	"os"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// SourceStat is the per-source part of a scan report.
type SourceStat struct {
	Label          string `json:"label"`
	Kind           string `json:"kind"`
	Status         string `json:"status"`
	Folders        int    `json:"folders"`
	Mods           int    `json:"mods"`
	Residue        int    `json:"residue,omitempty"`
	Disabled       int    `json:"disabled,omitempty"`
	NoAbout        int    `json:"no_about,omitempty"`
	ResidueNoAbout int    `json:"residue_no_about,omitempty"`
	Aliases        int    `json:"aliases,omitempty"`
	HasACF         bool   `json:"acf,omitempty"`
	Err            string `json:"error,omitempty"`
}

// ScanStats is stored with each scan and printed by `scan`.
type ScanStats struct {
	Sources    []SourceStat   `json:"sources"`
	Events     map[string]int `json:"events"`
	Questions  int            `json:"questions"`
	Live       int            `json:"live"`
	Tombstones int            `json:"tombstones"`
	Files      int            `json:"files"`
	Bytes      int64          `json:"bytes"`
	Hashed     int            `json:"hashed"`
	Reused     int            `json:"reused"`
	Snapshots  int            `json:"snapshots"`
	DurationMS int64          `json:"duration_ms"`
}

// ScanReport is the result of Scan.
type ScanReport struct {
	Env     *env.Environment
	ScanID  int64
	Full    bool
	Stats   ScanStats
	Notices []string // Loadout sync messages
}

// Scan runs 02 §4.1 for the current environment: enumerate → observe → reconcile → persist.
// full ignores cached hashes and reads every semantic file again.
func (a *App) Scan(full bool) (*ScanReport, error) {
	e, err := a.Env()
	if err != nil {
		return nil, err
	}
	started := a.Now()
	srcs, err := a.Store.Sources(e.ID)
	if err != nil {
		return nil, err
	}
	var roots []string
	for _, s := range srcs {
		roots = append(roots, s.Root)
	}
	if err := a.checkRoots(roots...); err != nil {
		return nil, err
	}
	skip := map[string]bool{}
	var active []*source.Source
	for _, s := range srcs {
		switch s.Status {
		case source.StatusRetired:
			continue
		case source.StatusRebindPending:
			skip[s.ID] = true
			continue
		}
		status := source.StatusOK
		if st, err := os.Stat(s.Root); err != nil || !st.IsDir() {
			status = source.StatusMissing
		}
		if status != s.Status {
			s.Status = status
			if err := a.Store.UpdateSource(s); err != nil {
				return nil, err
			}
		}
		if status == source.StatusMissing {
			skip[s.ID] = true
			continue
		}
		active = append(active, s)
	}

	known, err := a.Store.Known(e.ID)
	if err != nil {
		return nil, err
	}
	knownLoc := map[string]bool{}
	for _, k := range known {
		knownLoc[scan.LocKey(k.SourceID, k.Rel)] = true
	}
	var prev map[string]map[string]scan.File
	var prevHash map[string]scan.Hash
	if !full {
		if prev, err = a.Store.CurrentFiles(e.ID); err != nil {
			return nil, err
		}
		if prevHash, err = a.Store.CurrentHashes(e.ID); err != nil {
			return nil, err
		}
	}
	scanID, err := a.Store.BeginScan(e.ID, started, full)
	if err != nil {
		return nil, err
	}
	obs, sstats := scan.Collect(active, scan.Options{
		PrevFiles: func(l scan.Location) map[string]scan.File { return prev[l.Key()] },
		PrevHash: func(l scan.Location) (scan.Hash, bool) {
			h, ok := prevHash[l.Key()]
			return h, ok
		},
		Known: func(l scan.Location) bool { return knownLoc[l.Key()] },
	})
	prev, prevHash = nil, nil // release the caches before reconciliation
	lineages, err := a.Store.LineageIDs(e.ID)
	if err != nil {
		return nil, err
	}
	res := identity.Reconcile(identity.Input{
		Obs: obs, Known: known, Skip: skip, Lineages: lineages, NewID: a.NewID,
		Semantic: func(k *identity.Known) scan.SemanticSet {
			if k.Snap == nil {
				return nil
			}
			s, _ := a.Store.SemanticSet(k.Snap.ID)
			return s
		},
	})
	now := a.Now()
	if err := a.Store.ApplyScan(e.ID, scanID, now, res); err != nil {
		return nil, err
	}
	if v := env.ReadGameVersion(e.GameRoot); v != "" && v != e.GameVersion {
		e.GameVersion = v
		a.Store.UpdateEnv(e)
	}

	st := ScanStats{Events: map[string]int{}, Questions: len(res.Questions)}
	for _, s := range srcs {
		ss := SourceStat{Label: s.Label, Kind: string(s.Kind), Status: s.Status}
		if x := sstats[s.ID]; x != nil {
			ss.Folders, ss.Mods, ss.Residue, ss.Disabled, ss.NoAbout, ss.ResidueNoAbout, ss.Aliases, ss.HasACF, ss.Err =
				x.Folders, x.Mods, x.Residue, x.Disabled, x.NoAbout, x.ResidueNoAbout, x.Aliases, x.HasACF, x.Err
		}
		st.Sources = append(st.Sources, ss)
	}
	for _, ev := range res.Events {
		st.Events[ev.Kind]++
	}
	for _, ob := range obs {
		st.Files += ob.FP.Count
		st.Bytes += ob.FP.TotalSize
		st.Hashed += ob.FP.Hashed
		st.Reused += ob.FP.Reused
	}
	for _, as := range res.Assignments {
		if as.NeedSnapshot {
			st.Snapshots++
		}
	}
	mods, err := a.Store.Mods(e.ID)
	if err != nil {
		return nil, err
	}
	for _, m := range mods {
		if m.Status == identity.Live {
			st.Live++
		} else {
			st.Tombstones++
		}
	}
	st.DurationMS = a.Now().Sub(started).Milliseconds()
	if err := a.Store.FinishScan(scanID, a.Now(), st); err != nil {
		return nil, err
	}
	rep := &ScanReport{Env: e, ScanID: scanID, Full: full, Stats: st}
	// 04 §2.3: every scan compares ModsConfig.xml with what RimArcho last wrote.
	if ls, err := a.Loadout(); err == nil {
		rep.Notices = ls.Notices
	} else {
		rep.Notices = []string{"loadout: " + err.Error()}
	}
	return rep, nil
}

// LastScan returns the newest finished scan of the environment, or nil.
func (a *App) LastScan(envID string) (*store.Scan, *ScanStats, error) {
	scans, err := a.Store.Scans(envID, 1)
	if err != nil || len(scans) == 0 {
		return nil, nil, err
	}
	var st ScanStats
	scans[0].DecodeStats(&st)
	return scans[0], &st, nil
}

// Elapsed is a duration rounded for display.
func Elapsed(ms int64) string {
	return (time.Duration(ms) * time.Millisecond).Round(10 * time.Millisecond).String()
}
