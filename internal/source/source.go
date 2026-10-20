// Package source models mod source roots (02 §3): their kinds, link resolution, directory
// enumeration and re-binding after the player moves a root.
package source

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
)

// Kind is a source type (02 §3.1).
type Kind string

const (
	Official Kind = "official"
	Local    Kind = "local"
	Workshop Kind = "workshop"
	SteamCMD Kind = "steamcmd"
	Extra    Kind = "extra"
)

// WorkshopLike reports whether sub-directory names of this kind are workshop IDs.
func (k Kind) WorkshopLike() bool { return k == Workshop || k == SteamCMD }

// ParseKind validates a kind name.
func ParseKind(s string) (Kind, error) {
	switch k := Kind(strings.ToLower(s)); k {
	case Official, Local, Workshop, SteamCMD, Extra:
		return k, nil
	}
	return "", fmt.Errorf("unknown source kind %q (official, local, workshop, steamcmd, extra)", s)
}

// ReconcileRank orders sources for identity reconciliation: originals (workshop) are seen
// before likely copies (Mods/), so first-scan copies attach to the original's lineage.
func (k Kind) ReconcileRank() int {
	switch k {
	case Workshop:
		return 0
	case SteamCMD:
		return 1
	case Official:
		return 2
	case Local:
		return 3
	default:
		return 4
	}
}

// Status values for a source.
const (
	StatusOK            = "ok"
	StatusMissing       = "missing"        // root does not exist; not scanned, instances kept
	StatusRebindPending = "rebind-pending" // new root proposed but sample match < threshold
	StatusRetired       = "retired"        // replaced by another source; instances tombstone
)

// Source is one root directory of mods within an environment.
type Source struct {
	ID          string
	EnvID       string
	Kind        Kind
	Label       string // unique per environment, used in refs: src:<label>
	Root        string
	Status      string
	PendingRoot string
}

// Ref is the source's reference in outline output.
func (s *Source) Ref() string { return "src:" + s.Label }

// Spec is a desired source derived from environment settings.
type Spec struct {
	Kind Kind
	Root string
}

// Derive returns the standard sources of an environment (02 §3.1). Explicit roots for
// workshop/steamcmd/extra are passed in; workshop defaults to the library holding the game.
func Derive(gameRoot, workshopRoot string, steamcmd, extra []string) []Spec {
	var out []Spec
	if gameRoot != "" {
		out = append(out, Spec{Official, filepath.Join(gameRoot, "Data")}, Spec{Local, filepath.Join(gameRoot, "Mods")})
	}
	if workshopRoot == "" && gameRoot != "" {
		if ws := env.WorkshopRootFor(gameRoot); ws != "" {
			if st, err := os.Stat(ws); err == nil && st.IsDir() {
				workshopRoot = ws
			}
		}
	}
	if workshopRoot != "" {
		out = append(out, Spec{Workshop, workshopRoot})
	}
	for _, r := range steamcmd {
		out = append(out, Spec{SteamCMD, r})
	}
	for _, r := range extra {
		out = append(out, Spec{Extra, r})
	}
	return out
}

// Entry is one immediate sub-directory of a source root.
type Entry struct {
	Name     string // directory name = relative path within the source
	Path     string // path through the source root (may be a link)
	RealPath string // resolved target, with links and 8.3 names expanded
	IsLink   bool
}

// PackLinkPrefix names the Mods/ links of a pack being played (10 §8.3). "!" sorts before
// every folder name, so the game registers the pack's pinned copy before another Mods/ copy
// with the same packageId. The links are not part of
// the environment and List skips them.
const PackLinkPrefix = "!ArchoPack_"

// List enumerates the sub-directories of root, following junctions and symlinks the way the
// game's DirectoryInfo.GetDirectories does. realRoot is root with links resolved.
func List(root string) (entries []Entry, realRoot string, err error) {
	realRoot, err = RealPath(root)
	if err != nil {
		return nil, "", err
	}
	des, err := os.ReadDir(root)
	if err != nil {
		return nil, realRoot, err
	}
	for _, de := range des {
		p := filepath.Join(root, de.Name())
		e := Entry{Name: de.Name(), Path: p, RealPath: filepath.Join(realRoot, de.Name())}
		if IsLink(p) {
			if strings.HasPrefix(strings.ToLower(de.Name()), strings.ToLower(PackLinkPrefix)) {
				continue // a pack's copy, linked only while the pack is played (10 §8.3)
			}
			rp, err := RealPath(p)
			if err != nil {
				continue // dangling link
			}
			e.IsLink, e.RealPath = true, rp
		}
		st, err := os.Stat(p)
		if err != nil || !st.IsDir() {
			continue
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, realRoot, nil
}

// About file states of a folder.
const (
	AboutNone     = 0 // not a mod folder
	AboutPresent  = 1 // About/About.xml
	AboutDisabled = 2 // only About/About.xml.disabled (how RimCrow disables a mod, 02 §4.1)
)

// AboutState reports whether dir is a mod folder; names compare case-insensitively.
func AboutState(dir string) int {
	if _, err := os.Stat(filepath.Join(dir, "About", "About.xml")); err == nil {
		return AboutPresent
	}
	state := AboutNone
	des, err := os.ReadDir(dir)
	if err != nil {
		return AboutNone
	}
	for _, d := range des {
		if strings.EqualFold(d.Name(), "About") {
			fs, _ := os.ReadDir(filepath.Join(dir, d.Name()))
			for _, f := range fs {
				switch {
				case strings.EqualFold(f.Name(), "About.xml"):
					return AboutPresent
				case strings.EqualFold(f.Name(), "About.xml.disabled"):
					state = AboutDisabled
				}
			}
		}
	}
	return state
}

// HasAbout reports whether dir has an About/About.xml (any case).
func HasAbout(dir string) bool { return AboutState(dir) == AboutPresent }

// Within reports whether p is inside (or equal to) root, comparing resolved keys.
func Within(p, root string) bool {
	pk, rk := env.PathKey(p), env.PathKey(root)
	return pk == rk || strings.HasPrefix(pk, rk+string(filepath.Separator))
}
