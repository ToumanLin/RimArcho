package app

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Two folders in Mods/ with one packageId make the game log "Tried loading mod with the same
// packageId multiple times" and ignore all but the first (Verse.ModLister.TryAddMod, 1.6 and
// 1.7). Only a Workshop copy may share a packageId with a Mods/ copy (it becomes <pkg>_steam).
// RimArcho never makes such a pair (08 §3, §5.3) and reports any it finds (CodeModsDuplicate).

// CodeModsDuplicate is the finding for two copies of one packageId in Mods/.
const CodeModsDuplicate = "pkg.duplicate-mods"

// Who put a folder in Mods/.
const (
	CopyPlayer = ""      // a folder, or a link another tool made (RimCrow's _Link_…)
	CopyDeploy = "archo" // RimArcho's _Archo_ link (08 §3)
	CopyPack   = "pack"  // RimArcho's !ArchoPack_ link (08 §5.3)
)

// ModsCopy is one entry of Mods/ that holds a mod.
type ModsCopy struct {
	Name      string // the folder or link name in Mods/
	PackageID string // lower case
	By        string // CopyPlayer, CopyDeploy or CopyPack
	Link      bool
	Target    string     // a link's real folder
	Mod       *store.Mod // the instance it is, when RimArcho knows it ("" for a pack's copy)
}

// Ours: RimArcho made it and may remove it (only the link, never the target).
func (c ModsCopy) Ours() bool { return c.By != CopyPlayer }

type modsCopiesCache struct {
	key    string
	copies []ModsCopy
}

// ModsCopies lists the mods in the environment's Mods/ folder as the game sees them: every
// folder and link with an About.xml, with its packageId. The list is kept until Mods/ or the
// index changes (a folder added or removed changes the folder's time).
func (a *App) ModsCopies(ix *Index) []ModsCopy {
	mods := modsDir(ix)
	if mods == "" {
		return nil
	}
	fi, err := os.Stat(mods)
	if err != nil {
		return nil
	}
	key := fmt.Sprintf("%s|%d|%p", env.PathKey(mods), fi.ModTime().UnixNano(), ix)
	if c := a.modsCopies; c != nil && c.key == key {
		return c.copies
	}
	ents, err := os.ReadDir(mods)
	if err != nil {
		return nil
	}
	local := map[string]*store.Mod{} // Mods/ folder name (lower) → its instance
	byReal := map[string]*store.Mod{}
	for _, m := range ix.Mods {
		if m.Status != identity.Live {
			continue
		}
		if m.Kind == source.Local && !strings.ContainsAny(m.Rel, `/\`) {
			local[strings.ToLower(m.Rel)] = m
		}
		if m.RealPath != "" {
			byReal[env.PathKey(m.RealPath)] = m
		}
	}
	var out []ModsCopy
	for _, de := range ents {
		name := de.Name()
		p := filepath.Join(mods, name)
		c := ModsCopy{Name: name, Link: source.IsLink(p)}
		switch {
		case packLink(name):
			c.By = CopyPack
		case managedLink(name):
			c.By = CopyDeploy
		}
		if !c.Link && !de.IsDir() {
			continue
		}
		if c.Link {
			c.Target, _ = source.RealPath(p)
			if c.Target == "" {
				continue // a dangling link: the game sees nothing there
			}
			c.Mod = byReal[env.PathKey(c.Target)]
		} else {
			c.Mod = local[strings.ToLower(name)]
		}
		switch {
		case c.Mod != nil && c.By != CopyPack:
			if c.Mod.Unloadable != "" {
				continue
			}
			c.PackageID = c.Mod.PackageID
		default:
			d := scan.ReadDecl(p, false)
			if d.PackageID == "" {
				continue
			}
			c.PackageID = strings.ToLower(d.PackageID)
		}
		if c.PackageID == "" {
			continue
		}
		out = append(out, c)
	}
	a.modsCopies = &modsCopiesCache{key: key, copies: out}
	return out
}

// ModsDuplicate is one packageId held by several entries of Mods/.
type ModsDuplicate struct {
	PackageID string
	Copies    []ModsCopy // in the game's order (by folder name): the first one wins
}

// ModsDuplicates groups ModsCopies by packageId and keeps the groups with more than one.
func ModsDuplicates(copies []ModsCopy) []ModsDuplicate {
	by := map[string][]ModsCopy{}
	for _, c := range copies {
		by[c.PackageID] = append(by[c.PackageID], c)
	}
	var out []ModsDuplicate
	for p, cs := range by {
		if len(cs) < 2 {
			continue
		}
		sort.Slice(cs, func(i, j int) bool { return cs[i].Name < cs[j].Name })
		out = append(out, ModsDuplicate{PackageID: p, Copies: cs})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PackageID < out[j].PackageID })
	return out
}

// onlyPacks: the group is a duplicate only because of pack links (removing them leaves one).
func (d ModsDuplicate) onlyPacks() bool {
	n := 0
	for _, c := range d.Copies {
		if c.By != CopyPack {
			n++
		}
	}
	return n <= 1
}

// Names lists the group's entries: "Mods/_2888837973_, Mods/!ArchoPack_x__2888837973_".
func (d ModsDuplicate) Names() string {
	var ns []string
	for _, c := range d.Copies {
		ns = append(ns, "Mods/"+c.Name)
	}
	return strings.Join(ns, ", ")
}

// duplicateFindings are the Mods/ duplicates the pack links finding does not cover: each a
// pkg.duplicate-mods error (06 §2.4 会让游戏出错).
func (a *App) duplicateFindings(st *LoadoutState, dups []ModsDuplicate) []*finding.Finding {
	var out []*finding.Finding
	copies := a.ModsCopies(st.Ix)
	for _, d := range dups {
		if d.onlyPacks() {
			continue
		}
		f := &finding.Finding{Code: CodeModsDuplicate, Severity: finding.Error, Key: d.PackageID, Layer: "decl", Confidence: finding.Certain,
			Claim: fmt.Sprintf("Mods/ has %d folders with packageId %s (%s): the game reports an error and ignores all but %s", len(d.Copies), d.PackageID, d.Names(), d.Copies[0].Name)}
		var deployed, player *store.Mod
		for _, c := range d.Copies {
			if c.Mod != nil {
				f.Subjects = append(f.Subjects, finding.Inst(c.Mod.ID))
			}
			by := "a folder"
			switch {
			case c.By == CopyDeploy:
				by = "RimArcho's link for this plan"
				if c.Mod != nil {
					deployed = c.Mod
				}
			case c.By == CopyPack:
				by = "a pack's link"
			case c.Link:
				by = "a link another tool made"
				if player == nil {
					player = c.Mod
				}
			default:
				if player == nil {
					player = c.Mod
				}
			}
			f.Evidence = append(f.Evidence, finding.Evidence{Source: finding.Inferred, Text: "Mods/" + c.Name + " (" + by + ")"})
		}
		switch {
		case deployed != nil && player != nil && deployed.LineageID == player.LineageID && st.List.Index(deployed.ID) >= 0:
			f.Suggestions = []string{"archo swap " + finding.Inst(deployed.ID) + " " + finding.Inst(player.ID)}
		case deployed != nil && player != nil:
			f.Suggestions = []string{"archo disable " + finding.Inst(deployed.ID)}
		default:
			f.Suggestions = []string{"move all but one of them out of Mods/ (RimArcho does not move folders it did not make)"}
		}
		// First choice: move a plain folder into the lock library (收进锁定库), where plans
		// still choose it; the one beside RimArcho's link, else one the current plan does not
		// use, preferring one the game ignores (not the first).
		if mv := a.folderToMove(st, d, copies); mv != nil {
			f.Suggestions = append([]string{"archo library move-to-locked " + finding.Inst(mv.ID)}, f.Suggestions...)
		}
		out = append(out, f)
	}
	return out
}

// packSameContent: the instance m is what the pack entry carries (same semantic hash).
func packSameContent(pe PackEntry, m *store.Mod) bool {
	return m != nil && pe.Hash != "" && hex.EncodeToString(m.SnapHash) == pe.Hash
}

// folderToMove picks the plain folder of a Mods/ duplicate to move into the lock library
// (nil when none can move).
func (a *App) folderToMove(st *LoadoutState, d ModsDuplicate, copies []ModsCopy) *store.Mod {
	deploy := false
	var movable []int
	for i, c := range d.Copies {
		if c.By == CopyDeploy {
			deploy = true
		}
		if c.By == CopyPlayer && !c.Link && a.whyNotMovable(st, c, copies) == "" {
			movable = append(movable, i)
		}
	}
	if len(movable) == 0 {
		return nil
	}
	if deploy {
		return d.Copies[movable[0]].Mod
	}
	best, score := -1, -1
	for _, i := range movable {
		s := 0
		if st.List.Index(d.Copies[i].Mod.ID) < 0 {
			s += 2
		}
		if i > 0 {
			s++
		}
		if s > score {
			best, score = i, s
		}
	}
	return d.Copies[best].Mod
}
