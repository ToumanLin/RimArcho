package app

import (
	"os"
	"path/filepath"
	"slices"
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
	GameLinks []*GameLink
	Env       *env.Environment
	ScanID    int64
	Full      bool
	GameOnly  bool // only the game's own Data/ and Mods/ were read
	Stats     ScanStats
	Notices   []string // Loadout sync messages
}

// ScanOptions choose what a scan reads.
type ScanOptions struct {
	Full bool // ignore cached hashes and read every semantic file again
	// GameOnly reads only the environment's own Data/ and Mods/, not the shared mod folders
	// (a game just added, 03 §2 item 8).
	GameOnly bool
	// Progress, when set, hears how far the scan is: the mod folders read, then saving.
	Progress func(ScanProgress)
	// Sources, when set, limits the scan to these source IDs (with GameOnly: the game's own
	// folders and these). Like GameOnly, the sources left out keep their instances: Archo
	// Monitor's folder just installed is read alone (19 §6).
	Sources []string
}

// ScanProgress is how far a scan is.
type ScanProgress struct {
	Saving      bool // every folder is read; reconciling and writing the index
	Done, Total int  // mod folders read of all of them
}

// reads reports whether a scan with o reads source s of environment envID.
func (o ScanOptions) reads(s *source.Source, envID string) bool {
	if slices.Contains(o.Sources, s.ID) {
		return true
	}
	if o.GameOnly {
		return s.EnvID == envID
	}
	return len(o.Sources) == 0
}

// Scan runs 04 §1.1 for the current environment: the shared mod folders and the game's own
// Data/ and Mods/, each read once. full ignores cached hashes.
func (a *App) Scan(full bool) (*ScanReport, error) { return a.ScanWith(ScanOptions{Full: full}) }

// ScanWith is Scan with options: enumerate → observe → reconcile → persist.
func (a *App) ScanWith(o ScanOptions) (*ScanReport, error) {
	e, err := a.Env()
	if err != nil {
		return nil, err
	}
	return a.scanEnvironment(e, o)
}

func (a *App) scanEnvironment(e *env.Environment, o ScanOptions) (*ScanReport, error) {
	if o.Progress == nil {
		o.Progress = a.ScanProgress
	}
	full := o.Full
	started := a.Now()
	if err := a.adoptGameWorkshops(); err != nil {
		return nil, err
	}
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
	present := map[string]bool{} // left out of this scan, not lost (identity.Input.Present)
	var active []*source.Source
	var listed []*source.Source // the sources this scan reports on
	partial := false            // some sources are left out
	for _, s := range srcs {
		if !o.reads(s, e.ID) {
			partial = true
			if s.Status != source.StatusRetired {
				skip[s.ID] = true
				present[s.ID] = s.Status == source.StatusOK
			}
			continue
		}
		listed = append(listed, s)
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
	// The DDS RimArcho wrote next to mods' PNGs are not part of the mods (Texture optimisation).
	generated, err := a.Store.GeneratedFiles()
	if err != nil {
		return nil, err
	}
	scanID, err := a.Store.BeginScan(e.ID, started, full)
	if err != nil {
		return nil, err
	}
	obs, sstats := scan.Collect(active, scan.Options{
		Generated: func(dir string) map[string]bool {
			if len(generated) == 0 {
				return nil
			}
			if g := generated[store.PathKey(dir)]; g != nil {
				return g
			}
			if real, err := source.RealPath(dir); err == nil {
				return generated[store.PathKey(real)]
			}
			return nil
		},
		PrevFiles: func(l scan.Location) map[string]scan.File { return prev[l.Key()] },
		PrevHash: func(l scan.Location) (scan.Hash, bool) {
			h, ok := prevHash[l.Key()]
			return h, ok
		},
		Known:    func(l scan.Location) bool { return knownLoc[l.Key()] },
		Progress: o.readProgress(),
	})
	if o.Progress != nil {
		o.Progress(ScanProgress{Saving: true, Done: len(obs), Total: len(obs)})
	}
	if partial {
		sharedLinkObservations(obs, known, srcs, present, sstats)
	}
	prev, prevHash = nil, nil // release the caches before reconciliation
	lineages, err := a.Store.LineageIDs(e.ID)
	if err != nil {
		return nil, err
	}
	// Origin markers written before migration v14 name IDs merged away (03 §2.1).
	if aliases, err := a.Store.IDAliases(); err != nil {
		return nil, err
	} else if len(aliases) > 0 {
		for _, ob := range obs {
			if og := ob.Origin; og != nil {
				if n := aliases[og.InstanceID]; n != "" {
					og.InstanceID = n
				}
				if n := aliases[og.LineageID]; n != "" {
					og.LineageID = n
				}
			}
		}
	}
	res := identity.Reconcile(identity.Input{
		Obs: obs, Known: known, Skip: skip, Present: present, Lineages: lineages, NewID: a.NewID,
		Semantic: func(k *identity.Known) scan.SemanticSet {
			if k.Snap == nil {
				return nil
			}
			s, _ := a.Store.SemanticSet(k.Snap.ID)
			return s
		},
	})
	now := a.Now()
	var scanned []string
	for _, s := range active {
		scanned = append(scanned, s.ID)
	}
	if err := a.Store.ApplyScan(e.ID, scanID, now, res, scanned); err != nil {
		return nil, err
	}
	if v := env.ReadGameVersion(e.GameRoot); v != "" && v != e.GameVersion {
		e.GameVersion = v
		a.Store.UpdateEnv(e)
	}
	if err := a.indexGameTypes(e); err != nil {
		return nil, err
	}

	st := ScanStats{Events: map[string]int{}, Questions: len(res.Questions)}
	for _, s := range listed {
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
	if st.Snapshots > 0 {
		a.Store.TruncateWAL() // new snapshots wrote their file lists through the WAL (04 §4)
	}
	rep := &ScanReport{Env: e, ScanID: scanID, Full: full, GameOnly: o.GameOnly, Stats: st}
	// 07 §4.3: every scan compares ModsConfig.xml with what RimArcho last wrote.
	if ls, err := a.Loadout(); err == nil {
		rep.Notices = ls.Notices
	} else {
		rep.Notices = []string{"loadout: " + err.Error()}
	}
	rep.GameLinks, err = a.InspectGameLinks()
	return rep, err
}

func (o ScanOptions) readProgress() func(done, total int) {
	if o.Progress == nil {
		return nil
	}
	return func(done, total int) { o.Progress(ScanProgress{Done: done, Total: total}) }
}

// A game-only scan still sees links into shared folders. They are aliases of the
// shared instance, even though that source was not enumerated during this scan.
func sharedLinkObservations(obs []*scan.Observation, known []*identity.Known, sources []*source.Source, present map[string]bool, stats map[string]*scan.SourceStats) {
	byReal := map[string]*identity.Known{}
	bySource := map[string]*source.Source{}
	for _, s := range sources {
		bySource[s.ID] = s
	}
	for _, k := range known {
		if present[k.SourceID] && k.Status == identity.Live && k.RealPath != "" {
			byReal[env.PathKey(k.RealPath)] = k
		}
	}
	acfs := map[string]*scan.ACF{}
	for _, ob := range obs {
		if !ob.Loc.IsLink {
			continue
		}
		k := byReal[env.PathKey(ob.RealPath)]
		if k == nil {
			continue
		}
		s := bySource[k.SourceID]
		if s == nil {
			continue
		}
		path := filepath.Join(s.Root, k.Rel)
		if !sameDir(path, ob.RealPath) {
			continue // a moved or missing shared source does not own this link
		}
		if st := stats[ob.Loc.SourceID]; st != nil {
			st.Mods--
			st.Aliases++
		}
		ob.Aliases = append(ob.Aliases, ob.Loc)
		ob.Loc = scan.Location{SourceID: s.ID, Kind: s.Kind, Label: s.Label, Rel: k.Rel, Path: path}
		if s.Kind.WorkshopLike() {
			ob.WorkshopID = k.Rel
			acf, loaded := acfs[s.ID]
			if !loaded {
				acf = scan.ReadACF(s.Root)
				acfs[s.ID] = acf
			}
			if acf != nil {
				ob.ACF = acf.Items[k.Rel]
				ob.Residue = s.Kind == source.Workshop && (ob.ACF == nil || !ob.ACF.Subscribed)
			}
		}
	}
}

// LastScan returns the newest finished scan of the environment, or nil.
func (a *App) LastScan(envID string) (*store.Scan, *ScanStats, error) {
	sc, err := a.Store.LastFinishedScan(envID)
	if err != nil || sc == nil {
		return nil, nil, err
	}
	var st ScanStats
	sc.DecodeStats(&st)
	return sc, &st, nil
}

// Elapsed is a duration rounded for display.
func Elapsed(ms int64) string {
	return (time.Duration(ms) * time.Millisecond).Round(10 * time.Millisecond).String()
}
