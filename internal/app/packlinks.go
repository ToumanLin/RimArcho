package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// CodePackLinks is the finding for a pack's links left in Mods/ (08 §5.3).
const CodePackLinks = "pack.links-left"

// PackLinksLeft is what a pack left in the environment's Mods/ while that pack is not being
// played (08 §5.3): a game started from Steam, not from RimArcho, loads the pack's copies
// (the "!" sorts first) instead of the plan's mods.
type PackLinksLeft struct {
	Mods  string       // the Mods/ folder
	Packs []string     // the packs the links point into ("" parts: a link to nothing)
	Links []LinkChange // the links, as removals
	// Running: the game of this environment runs now, not as that pack (started from
	// Steam, or from RimArcho before the links were made by another tool).
	Running bool
	// Duplicates: packageIds a link shares with another copy in Mods/: the game reports
	// an error for each and ignores one of them.
	Duplicates []ModsDuplicate
}

// LeftPackLinks finds the !ArchoPack_ links in the current environment's Mods/ that no
// running pack uses; nil when there are none. It reads Mods/ only, and looks at the
// running game only when there are links.
func (a *App) LeftPackLinks(ix *Index) *PackLinksLeft {
	r := a.packLinksIn(modsDir(ix))
	if len(r.Links) == 0 {
		return nil
	}
	if g, err := a.RunningGame(); err == nil && g != nil && g.Env.ID == ix.Env.ID {
		if g.What.Pack != "" && r.has(g.What.Pack) {
			return nil // the pack is being played: its links are in use
		}
		r.Running = true
	}
	return r
}

// packLinksIn lists the !ArchoPack_ links in mods and the packs they point into.
func (a *App) packLinksIn(mods string) *PackLinksLeft {
	r := &PackLinksLeft{Mods: mods}
	if mods == "" {
		return r
	}
	ents, _ := os.ReadDir(mods)
	packs := map[string]bool{}
	for _, de := range ents {
		n := de.Name()
		p := filepath.Join(mods, n)
		if !packLink(n) || !source.IsLink(p) {
			continue
		}
		tgt, _ := source.RealPath(p)
		r.Links = append(r.Links, LinkChange{Op: "remove", Name: n, Target: tgt})
		packs[a.packOfLink(tgt)] = true
	}
	for p := range packs {
		r.Packs = append(r.Packs, p)
	}
	sort.Strings(r.Packs)
	return r
}

func (r *PackLinksLeft) has(pack string) bool {
	for _, p := range r.Packs {
		if strings.EqualFold(p, pack) {
			return true
		}
	}
	return false
}

// packOfLink names the pack a link's target belongs to: <pack folder>/<name>/mods/<folder>.
func (a *App) packOfLink(target string) string {
	if target == "" {
		return ""
	}
	dir := filepath.Dir(filepath.Dir(target))
	if !strings.EqualFold(filepath.Base(filepath.Dir(target)), "mods") {
		return ""
	}
	if !source.Within(dir, realOrAbs(a.PacksDir())) {
		return ""
	}
	return filepath.Base(dir)
}

// PackNames is the packs of the links for a sentence: "first", "first, second".
func (r *PackLinksLeft) PackNames() string {
	var out []string
	for _, p := range r.Packs {
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return "?"
	}
	return strings.Join(out, ", ")
}

// Finding is the left links as a problem of the current plan (warning; its fix is
// archo pack unlink).
func (r *PackLinksLeft) Finding() *finding.Finding {
	claim := fmt.Sprintf("Mods/ still has %d links of pack %s: a game started from Steam, not from RimArcho, loads the pack's copies instead of this plan's mods", len(r.Links), r.PackNames())
	if r.Running {
		claim = fmt.Sprintf("the game is running with %d links of pack %s in Mods/: it loaded the pack's copies instead of this plan's mods", len(r.Links), r.PackNames())
	}
	key, sev := "links", finding.Warning
	if r.Running {
		key = "running"
	}
	if n := len(r.Duplicates); n > 0 {
		var pkgs []string
		for _, d := range r.Duplicates {
			pkgs = append(pkgs, d.PackageID)
		}
		key, sev = "duplicate", finding.Error
		claim = fmt.Sprintf("Mods/ has %d links of pack %s next to other copies of the same mods (%s): the game reports an error for each and ignores one copy", len(r.Links), r.PackNames(), strings.Join(pkgs, ", "))
	}
	f := &finding.Finding{Code: CodePackLinks, Severity: sev, Claim: claim, Confidence: finding.Certain,
		Key: key, Layer: "decl", Suggestions: []string{"archo pack unlink"}}
	for _, p := range r.Packs {
		if p != "" {
			f.Subjects = append(f.Subjects, "pack:"+p)
		}
	}
	for _, d := range r.Duplicates {
		f.Evidence = append(f.Evidence, finding.Evidence{Source: finding.Inferred, Text: d.Names()})
	}
	for i, l := range r.Links {
		if len(r.Duplicates) > 0 {
			break
		}
		if i == 3 {
			f.Evidence = append(f.Evidence, finding.Evidence{Source: finding.Inferred, Text: fmt.Sprintf("and %d more", len(r.Links)-3)})
			break
		}
		f.Evidence = append(f.Evidence, finding.Evidence{Source: finding.Inferred, Text: "Mods/" + l.Name})
	}
	return f
}

// UnlinkPacks removes the !ArchoPack_ links from the current environment's Mods/ (08 §5.3):
// only the links, never what they point at. Refused while a game runs; launching a pack
// makes them again.
func (a *App) UnlinkPacks(dryRun bool) (*PackLinksLeft, error) {
	ix, err := a.Index()
	if err != nil {
		return nil, err
	}
	r := a.packLinksIn(modsDir(ix))
	mods := r.Mods
	if dryRun || len(r.Links) == 0 {
		return r, nil
	}
	if err := a.checkNoGame(); err != nil {
		return nil, err
	}
	if err := a.checkRoots(mods); err != nil {
		return nil, err
	}
	return r, a.applyLinks(mods, r.Links)
}
