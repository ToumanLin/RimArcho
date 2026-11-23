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
	// Partial: some sources were left out (GameOnly, Sources). The first scan that is not
	// partial is the one that finds the library already installed (NewSince).
	Partial bool `json:"partial,omitempty"`
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
	deferLoadout bool // environment creation supplies its own initial plan
	Full         bool // ignore cached hashes and read every semantic file again
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

// reads reports whether a scan with o reads source s of installation gameID.
func (o ScanOptions) reads(s *source.Source, gameID string) bool {
	if slices.Contains(o.Sources, s.ID) {
		return true
	}
	if o.GameOnly {
		return s.EnvID == gameID
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
	defer a.forgetCorpus() // a Workshop Monitor may have come or gone (05 §4.6)
	if o.Progress == nil {
		o.Progress = a.ScanProgress
	}
	full := o.Full
	started := a.Now()
	if err := a.adoptGameWorkshops(); err != nil {
		return nil, err
	}
	srcs, err := a.Store.Sources(e.GameID)
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
	set, err := a.scanSources(srcs, o, e.GameID)
	if err != nil {
		return nil, err
	}

	// What the folders look like as this scan starts (04 §1.6): recorded once it succeeds.
	prints := map[string]string{}
	for _, s := range set.listed {
		if s.Status != source.StatusRetired && s.Status != source.StatusRebindPending {
			prints[s.ID] = folderPrint(s)
		}
	}
	known, err := a.Store.Known(e.GameID)
	if err != nil {
		return nil, err
	}
	collect, err := a.scanCollectOptions(e.GameID, full, known, o)
	if err != nil {
		return nil, err
	}
	scanID, err := a.Store.BeginScan(e.GameID, started, full)
	if err != nil {
		return nil, err
	}
	obs, sstats := scan.Collect(set.active, collect)
	if o.Progress != nil {
		o.Progress(ScanProgress{Saving: true, Done: len(obs), Total: len(obs)})
	}
	if set.partial {
		sharedLinkObservations(obs, known, srcs, set.present, sstats)
	}
	collect = scan.Options{} // release the caches before reconciliation
	lineages, err := a.Store.LineageIDs(e.GameID)
	if err != nil {
		return nil, err
	}
	if err := a.renameOriginAliases(obs); err != nil {
		return nil, err
	}
	res := identity.Reconcile(identity.Input{
		Obs: obs, Known: known, Skip: set.skip, Present: set.present, Lineages: lineages, NewID: a.NewID,
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
	for _, s := range set.active {
		scanned = append(scanned, s.ID)
	}
	if err := a.Store.ApplyScan(e.GameID, scanID, now, res, scanned); err != nil {
		return nil, err
	}
	if v := env.ReadGameVersion(e.GameRoot); v != "" && v != e.GameVersion {
		e.GameVersion = v
		a.Store.UpdateGame(&store.Game{ID: e.GameID, Name: e.Name, GameRoot: e.GameRoot, GameVersion: e.GameVersion})
	}
	if err := a.indexGameTypes(e); err != nil {
		return nil, err
	}

	st := newScanStats(set, sstats, res, obs)
	mods, err := a.Store.Mods(e.GameID)
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
	a.recordFolderPrints(prints)
	if st.Snapshots > 0 {
		a.Store.TruncateWAL() // new snapshots wrote their file lists through the WAL (04 §4)
	}
	if err := a.tagOfficial(e.GameID); err != nil {
		return nil, err
	}
	rep := &ScanReport{Env: e, ScanID: scanID, Full: full, GameOnly: o.GameOnly, Stats: st}
	// 07 §4.3: every scan compares ModsConfig.xml with what RimArcho last wrote.
	if !o.deferLoadout {
		if ls, err := a.Loadout(); err == nil {
			rep.Notices = ls.Notices
		} else {
			rep.Notices = []string{"loadout: " + err.Error()}
		}
	}
	rep.GameLinks, err = a.InspectGameLinks()
	return rep, err
}

// scanSourceSet is how a scan treats the installation's sources.
type scanSourceSet struct {
	skip    map[string]bool
	present map[string]bool  // left out of this scan, not lost (identity.Input.Present)
	active  []*source.Source // the sources read
	listed  []*source.Source // the sources this scan reports on
	partial bool             // some sources are left out
}

// scanSources sorts the sources into read, skipped and left out, recording a source whose
// folder went missing or came back.
func (a *App) scanSources(srcs []*source.Source, o ScanOptions, gameID string) (*scanSourceSet, error) {
	set := &scanSourceSet{skip: map[string]bool{}, present: map[string]bool{}}
	for _, s := range srcs {
		if !o.reads(s, gameID) {
			set.partial = true
			if s.Status != source.StatusRetired {
				set.skip[s.ID] = true
				set.present[s.ID] = s.Status == source.StatusOK
			}
			continue
		}
		set.listed = append(set.listed, s)
		switch s.Status {
		case source.StatusRetired:
			continue
		case source.StatusRebindPending:
			set.skip[s.ID] = true
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
			set.skip[s.ID] = true
			continue
		}
		set.active = append(set.active, s)
	}
	return set, nil
}

// scanCollectOptions reads what scan.Collect reuses: the known locations, the previous
// file lists and hashes (unless full), and the DDS RimArcho wrote.
func (a *App) scanCollectOptions(gameID string, full bool, known []*identity.Known, o ScanOptions) (scan.Options, error) {
	knownLoc := map[string]bool{}
	for _, k := range known {
		knownLoc[scan.LocKey(k.SourceID, k.Rel)] = true
	}
	var prev map[string]map[string]scan.File
	var prevHash map[string]scan.Hash
	var err error
	if !full {
		if prev, err = a.Store.CurrentFiles(gameID); err != nil {
			return scan.Options{}, err
		}
		if prevHash, err = a.Store.CurrentHashes(gameID); err != nil {
			return scan.Options{}, err
		}
	}
	// The DDS RimArcho wrote next to mods' PNGs are not part of the mods (Texture optimisation).
	generated, err := a.Store.GeneratedFiles()
	if err != nil {
		return scan.Options{}, err
	}
	return scan.Options{
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
	}, nil
}

// renameOriginAliases renames the IDs merged away in origin markers written before
// migration v14 (03 §2.1).
func (a *App) renameOriginAliases(obs []*scan.Observation) error {
	aliases, err := a.Store.IDAliases()
	if err != nil {
		return err
	}
	if len(aliases) == 0 {
		return nil
	}
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
	return nil
}

// newScanStats sums up a scan: per source, the events, the files read and the snapshots.
func newScanStats(set *scanSourceSet, sstats map[string]*scan.SourceStats, res *identity.Result, obs []*scan.Observation) ScanStats {
	st := ScanStats{Events: map[string]int{}, Questions: len(res.Questions), Partial: set.partial}
	for _, s := range set.listed {
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
	return st
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

// LastScan returns the newest finished scan of the installation, or nil.
func (a *App) LastScan(gameID string) (*store.Scan, *ScanStats, error) {
	sc, err := a.Store.LastFinishedScan(gameID)
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
