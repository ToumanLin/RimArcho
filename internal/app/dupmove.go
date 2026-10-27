package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// Another version in the way (08 §3, §5.3): RimArcho must link a copy into Mods/ (a pack's,
// or a plan's _Archo_ deploy), and Mods/ already holds a plain folder with the same
// packageId and other content. Two of them would make the game report an error, so instead
// of refusing outright RimArcho offers to move that folder into the lock library (the
// existing 收进锁定库: renamed on one volume, else copied, checked and the original recycled;
// its instance and every plan that uses it stay the same) and then to link the copy needed.

// DuplicateMove is one folder in the way.
type DuplicateMove struct {
	Copy  ModsCopy
	For   string // what needs the packageId: "pack X's copy of Name" or "#ref Name"
	Pack  string // for a pack's copy: the pack and the mod's name (to say For in the UI language)
	Mod   string
	Link  string // the link made in Mods/ once it has moved
	To    string // where it would go in the lock library
	Why   string // "" when it can move; else why RimArcho leaves it
	Ref   string // the folder's instance ref, for archo library move-to-locked
	Links *LinkLifecyclePlan
}

// DuplicatesError is a launch or a write refused because Mods/ holds other versions.
type DuplicatesError struct {
	Moves []DuplicateMove
	Fix   string // the same command with --move-duplicates ("" when something cannot move)
}

// ErrDuplicates is what DuplicatesError wraps.
var ErrDuplicates = errors.New("Mods/ holds another version")

func (e *DuplicatesError) Unwrap() error { return ErrDuplicates }

// Movable: every folder in the way can move into the lock library.
func (e *DuplicatesError) Movable() bool {
	for _, m := range e.Moves {
		if m.Why != "" {
			return false
		}
	}
	return len(e.Moves) > 0
}

func (e *DuplicatesError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%v: Mods/ already holds %d of these mods in another version; the game would load both and report an error:", ErrLaunchBlocked, len(e.Moves))
	for _, m := range e.Moves {
		fmt.Fprintf(&b, "\n  Mods/%s is in the way of %s", m.Copy.Name, m.For)
		if m.Why != "" {
			b.WriteString(": " + m.Why)
		}
	}
	if e.Movable() {
		var refs []string
		for _, m := range e.Moves {
			refs = append(refs, "archo library move-to-locked "+m.ref())
		}
		fmt.Fprintf(&b, "\n(move them into the lock library and go on: %s; or one by one: %s)", e.Fix, strings.Join(refs, "; "))
	}
	return b.String()
}

func (m DuplicateMove) ref() string {
	if m.Ref == "" {
		return "Mods/" + m.Copy.Name
	}
	return m.Ref
}

// Lines say what moving them does, for the one confirmation (and the dry runs).
func (e *DuplicatesError) Lines() []string {
	var out []string
	for _, m := range e.Moves {
		out = append(out, fmt.Sprintf("Mods/%s → %s (the lock library); then Mods/%s for %s", m.Copy.Name, m.To, m.Link, m.For))
		if m.Links != nil {
			out = append(out, m.Links.Lines()...)
		}
	}
	return out
}

// planMoves says, for each folder in the way, where it would go and whether it can move.
func (a *App) planMoves(st *LoadoutState, copies []ModsCopy, moves []DuplicateMove) []DuplicateMove {
	root := a.LockDir()
	used := map[string]bool{}
	for i := range moves {
		m := &moves[i]
		m.Why = a.whyNotMovable(st, m.Copy, copies)
		if m.Copy.Mod != nil {
			m.Ref = st.Ix.Ref(m.Copy.Mod.ID)
		}
		name := freeNameUsed(root, m.Copy.Name, used)
		used[strings.ToLower(name)] = true
		m.To = filepath.Join(root, name)
		if m.Why == "" {
			var err error
			m.Links, err = a.PlanIncomingLinks(m.Copy.Mod.RealPath, m.To)
			if err != nil {
				m.Why = err.Error()
			}
		}
	}
	sort.Slice(moves, func(i, j int) bool { return moves[i].Copy.Name < moves[j].Copy.Name })
	return moves
}

// whyNotMovable: "" when the Mods/ entry c can move into the lock library. Links never move
// (RimArcho never moves what it did not make a folder of), nor folders another tool's link
// points at (RimCrow's _Link_…): moving them would break that link.
func (a *App) whyNotMovable(st *LoadoutState, c ModsCopy, copies []ModsCopy) string {
	switch {
	case c.By != CopyPlayer:
		return "it is RimArcho's own link"
	case c.Link:
		return "it is a link another tool made, and RimArcho does not move links; remove it in that tool, or move its target out of the way yourself"
	case c.Mod == nil || c.Mod.Kind != source.Local:
		return "RimArcho does not know this folder yet (archo scan)"
	}
	if by := linkedBy(st, c.Mod.RealPath, copies); by != "" {
		return by
	}
	if out := linksOutside(st.Ix.ModDir(c.Mod)); len(out) > 0 {
		return fmt.Sprintf("it has links that point outside it (%s); move it yourself", strings.Join(out, ", "))
	}
	return ""
}

// linkedBy names another tool's link in Mods/ that points at the folder real ("" if none).
func linkedBy(st *LoadoutState, real string, copies []ModsCopy) string {
	if real == "" {
		return ""
	}
	for _, o := range copies {
		if o.Link && o.By == CopyPlayer && o.Target != "" && env.PathKey(o.Target) == env.PathKey(real) {
			return fmt.Sprintf("Mods/%s (another tool's link) points at it; moving it would break that link", o.Name)
		}
	}
	return ""
}

// doMoves moves the folders into the lock library, one by one (stopping at the first that
// fails: what moved stays moved, each is a whole step).
func (a *App) doMoves(moves []DuplicateMove) ([]*MoveResult, error) {
	var out []*MoveResult
	for _, m := range moves {
		st, err := a.Loadout()
		if err != nil {
			return out, err
		}
		cur := st.Ix.ByID[m.Copy.Mod.ID]
		if cur == nil {
			return out, fmt.Errorf("%s is no longer known; rescan and try again", m.Copy.Name)
		}
		r, err := a.moveToLibrary(st, cur, false)
		if err != nil {
			return out, fmt.Errorf("moving Mods/%s into the lock library: %w", m.Copy.Name, err)
		}
		out = append(out, r)
	}
	return out, nil
}

// inModsDuplicate: the instance's folder in Mods/ shares its packageId with another entry
// of Mods/ (pkg.duplicate-mods).
func (a *App) inModsDuplicate(st *LoadoutState, id string) bool {
	return a.modsDuplicateIDs(st.Ix, modsRootOf(st.Ix, id))[id]
}

// modsRootOf is the Mods/ folder an instance sits in (the current game's when it is not in
// a game's Mods/).
func modsRootOf(ix *Index, id string) string {
	if m := ix.ByID[id]; m != nil && m.Kind.GameMods() {
		if s := ix.Source(m.SourceID); s != nil {
			return s.Root
		}
	}
	return modsDir(ix)
}

// modsDuplicateIDs: the player's own folders (not links) in the Mods/ folder root that share
// their packageId with another entry there; one pass over the folder.
func (a *App) modsDuplicateIDs(ix *Index, root string) map[string]bool {
	out := map[string]bool{}
	for _, d := range ModsDuplicates(a.modsCopiesIn(ix, root)) {
		for _, c := range d.Copies {
			if c.Mod != nil && c.By == CopyPlayer && !c.Link {
				out[c.Mod.ID] = true
			}
		}
	}
	return out
}
