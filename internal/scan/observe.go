// Package scan enumerates source roots and records one observation per mod folder:
// location, declarations, external state and layered content fingerprint (02 §4).
package scan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/rimarcho/rimarcho/internal/env"
	"github.com/rimarcho/rimarcho/internal/source"
)

// Origin is the content of .rimarcho/origin.json, written by copy-local (00 §6.6).
type Origin struct {
	InstanceID   string `json:"instance_id"`
	LineageID    string `json:"lineage_id,omitempty"`
	WorkshopID   string `json:"workshop_id,omitempty"`
	SnapshotHash string `json:"snapshot_hash,omitempty"`
	CopiedAt     string `json:"copied_at,omitempty"`
}

// OriginFile is the marker's path relative to a mod folder.
const OriginFile = ".rimarcho/origin.json"

// Location is where a mod folder was seen: a source plus a path relative to its root.
type Location struct {
	SourceID string
	Kind     source.Kind
	Label    string
	Rel      string
	Path     string
	IsLink   bool
}

// Key identifies a location; relative paths compare case-insensitively (Windows).
func (l Location) Key() string { return LocKey(l.SourceID, l.Rel) }

// LocKey builds a location key.
func LocKey(sourceID, rel string) string { return sourceID + "\x00" + strings.ToLower(rel) }

// Observation is everything one scan learned about one mod folder (02 §4.2).
type Observation struct {
	Loc        Location
	Aliases    []Location // other locations resolving to the same real directory
	RealPath   string
	Decl       Decl
	WorkshopID string // folder name for workshop-like sources, else PublishedFileId.txt
	FP         Fingerprint
	Origin     *Origin
	ACF        *ACFItem
	GitHead    string
	Residue    bool // workshop folder that Steam no longer lists as subscribed
	Err        string
}

// SourceStats counts what a scan saw under one source.
type SourceStats struct {
	Folders        int
	Mods           int
	Residue        int
	NoAbout        int // folders without About/About.xml (not mods)
	ResidueNoAbout int // unsubscribed workshop folders without About.xml: counted only
	Aliases        int
	HasACF         bool
	Err            string
}

// Options tune a collection run.
type Options struct {
	// PrevFiles returns the previous manifest of a location (lower-cased Rel → File) for
	// incremental hashing; nil or a nil result means hash everything.
	PrevFiles func(loc Location) map[string]File
	// Known reports whether a location already belongs to an Instance; alias groups keep
	// a known location as primary so R1 keeps matching.
	Known   func(loc Location) bool
	Workers int // per-source concurrency; default runtime.NumCPU()
}

// Collect scans every source and returns the observations in reconciliation order
// (workshop, steamcmd, official, local, extra; then by relative path).
func Collect(sources []*source.Source, o Options) ([]*Observation, map[string]*SourceStats) {
	if o.Workers <= 0 {
		o.Workers = runtime.NumCPU()
	}
	stats := map[string]*SourceStats{}
	type cand struct {
		loc      Location
		real     string
		residue  bool
		acf      *ACFItem
		srcOrder int
	}
	groups := map[string][]cand{}
	var order []string
	for si, s := range sources {
		st := &SourceStats{}
		stats[s.ID] = st
		entries, _, err := source.List(s.Root)
		if err != nil {
			st.Err = err.Error()
			continue
		}
		var acf *ACF
		if s.Kind.WorkshopLike() {
			acf = ReadACF(s.Root)
			st.HasACF = acf != nil
		}
		for _, e := range entries {
			st.Folders++
			var item *ACFItem
			residue := false
			if acf != nil {
				item = acf.Items[e.Name]
				// Only the Steam client's own library decides what the game loads; SteamCMD
				// folders are never loaded directly, so residue applies to workshop only.
				residue = s.Kind == source.Workshop && (item == nil || !item.Subscribed)
			}
			if !source.HasAbout(e.Path) {
				if residue {
					st.ResidueNoAbout++
				} else {
					st.NoAbout++
				}
				continue
			}
			c := cand{
				loc:      Location{SourceID: s.ID, Kind: s.Kind, Label: s.Label, Rel: e.Name, Path: e.Path, IsLink: e.IsLink},
				real:     e.RealPath,
				residue:  residue,
				acf:      item,
				srcOrder: si,
			}
			k := env.PathKey(e.RealPath)
			if _, ok := groups[k]; !ok {
				order = append(order, k)
			}
			groups[k] = append(groups[k], c)
		}
	}

	obs := make([]*Observation, 0, len(order))
	for _, k := range order {
		g := groups[k]
		sort.SliceStable(g, func(i, j int) bool {
			a, b := g[i], g[j]
			if o.Known != nil {
				if ka, kb := o.Known(a.loc), o.Known(b.loc); ka != kb {
					return ka
				}
			}
			if a.loc.IsLink != b.loc.IsLink {
				return !a.loc.IsLink
			}
			if ra, rb := a.loc.Kind.ReconcileRank(), b.loc.Kind.ReconcileRank(); ra != rb {
				return ra < rb
			}
			if a.srcOrder != b.srcOrder {
				return a.srcOrder < b.srcOrder
			}
			return a.loc.Rel < b.loc.Rel
		})
		p := g[0]
		ob := &Observation{Loc: p.loc, RealPath: p.real, Residue: p.residue, ACF: p.acf}
		for _, a := range g[1:] {
			ob.Aliases = append(ob.Aliases, a.loc)
			stats[a.loc.SourceID].Aliases++
		}
		obs = append(obs, ob)
		st := stats[p.loc.SourceID]
		st.Mods++
		if p.residue {
			st.Residue++
		}
	}

	// Read observations: sources in parallel, each with at most Workers goroutines.
	bySource := map[string][]*Observation{}
	for _, ob := range obs {
		bySource[ob.Loc.SourceID] = append(bySource[ob.Loc.SourceID], ob)
	}
	var wg sync.WaitGroup
	for _, list := range bySource {
		wg.Add(1)
		go func(list []*Observation) {
			defer wg.Done()
			jobs := make(chan *Observation)
			var inner sync.WaitGroup
			for w := 0; w < o.Workers; w++ {
				inner.Add(1)
				go func() {
					defer inner.Done()
					for ob := range jobs {
						observe(ob, o)
					}
				}()
			}
			for _, ob := range list {
				jobs <- ob
			}
			close(jobs)
			inner.Wait()
		}(list)
	}
	wg.Wait()

	sort.SliceStable(obs, func(i, j int) bool {
		a, b := obs[i].Loc, obs[j].Loc
		if ra, rb := a.Kind.ReconcileRank(), b.Kind.ReconcileRank(); ra != rb {
			return ra < rb
		}
		if a.SourceID != b.SourceID {
			return sourceIndex(sources, a.SourceID) < sourceIndex(sources, b.SourceID)
		}
		return strings.ToLower(a.Rel) < strings.ToLower(b.Rel)
	})
	return obs, stats
}

func sourceIndex(sources []*source.Source, id string) int {
	for i, s := range sources {
		if s.ID == id {
			return i
		}
	}
	return len(sources)
}

func observe(ob *Observation, o Options) {
	dir := ob.Loc.Path
	ob.Decl = ReadDecl(dir, ob.Loc.Kind == source.Official)
	if ob.Loc.Kind.WorkshopLike() {
		ob.WorkshopID = ob.Loc.Rel
	} else {
		ob.WorkshopID = ob.Decl.PublishedFileID
	}
	var prev map[string]File
	if o.PrevFiles != nil {
		prev = o.PrevFiles(ob.Loc)
	}
	fp, err := Compute(dir, prev)
	if err != nil {
		ob.Err = err.Error()
	}
	ob.FP = fp
	if b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(OriginFile))); err == nil {
		var og Origin
		if json.Unmarshal(b, &og) == nil && og.InstanceID != "" {
			ob.Origin = &og
		}
	}
	ob.GitHead = gitHead(dir)
}

// gitHead returns the commit at HEAD if dir is a git work tree, else "".
func gitHead(dir string) string {
	gd := filepath.Join(dir, ".git")
	b, err := os.ReadFile(filepath.Join(gd, "HEAD"))
	if err != nil {
		return ""
	}
	head := strings.TrimSpace(string(b))
	ref, ok := strings.CutPrefix(head, "ref: ")
	if !ok {
		return head
	}
	if b, err := os.ReadFile(filepath.Join(gd, filepath.FromSlash(ref))); err == nil {
		return strings.TrimSpace(string(b))
	}
	if b, err := os.ReadFile(filepath.Join(gd, "packed-refs")); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if f := strings.Fields(line); len(f) == 2 && f[1] == ref {
				return f[0]
			}
		}
	}
	return ""
}
