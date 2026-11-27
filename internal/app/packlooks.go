package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/looks"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// A pack's looks (10 §2 step 6, 21 §4 "In a pack"). A pack never carries RimArcho Looks
// Preference: it carries the result. After the copy, every texture the game would not show
// with the pack's list and the plan's choices is removed from the copies, so that the file
// the game shows is the only one of its key and wins by load order inside the pack. Only
// the pack's build folder is written; the Mods are read.

// PackLooks is what a build removed from the copies for the looks of the pack's list.
type PackLooks struct {
	Removed int   `json:"removed"`           // texture files removed from the copies
	Bytes   int64 `json:"removed_bytes"`     // their size
	Choices int   `json:"choices,omitempty"` // looks whose applied choice the pack carries
}

// packLoser is one texture file a copy loses: the game shows another Mod's file of its key.
type packLoser struct {
	entry int    // index into Pack.Entries
	rel   string // relative to the Mod root, forward slashes, as the Mod names it
	size  int64  // as the index read it
}

// packLooksPlan is the looks step of a build before anything is copied.
type packLooksPlan struct {
	losers  []packLoser
	choices int
}

// packLooksState builds the looks state of the list being packed: the pack's entries in
// their order (the sorted list without RimArcho Looks Preference), with the plan's choices
// resolved as for the plan (its own, then those for all plans, 21 §3.2).
func (a *App) packLooksState(pl *PackPlan) (*LooksState, []store.LookChoice, error) {
	st := *pl.St
	st.List = make(loadout.List, len(pl.mods))
	for i, m := range pl.mods {
		st.List[i] = loadout.Entry{Instance: m.ID}
	}
	rows, err := a.Store.LookChoices(store.LookScope(st.Plan.ID), store.LookScopeAll)
	if err != nil {
		return nil, nil, err
	}
	idx, err := a.LooksIndex(context.Background(), &st, nil)
	if err != nil {
		return nil, rows, err
	}
	return looksState(&st, idx, rows), rows, nil
}

// planPackLooks finds the textures the pack's copies lose. When the index cannot be built
// it refuses if a choice names a Mod of the pack (the pack would silently lose it);
// otherwise the step is skipped with a warning in the result.
func (a *App) planPackLooks(pl *PackPlan) (*packLooksPlan, error) {
	r := pl.R
	ls, rows, err := a.packLooksState(pl)
	if err != nil {
		in := map[string]bool{}
		for _, m := range pl.mods {
			in[m.LineageID] = true
		}
		for _, c := range rows {
			if in[c.Lineage] {
				return nil, fmt.Errorf("the looks of plan:%s cannot be read (%v), and the plan has looks choices the pack would lose; build it again, or undo the choices (archo looks --filter chosen)", pl.St.Plan.Name, err)
			}
		}
		r.Warnings = append(r.Warnings, fmt.Sprintf("looks: the textures of the list could not be compared (%v); every Mod keeps all its textures", err))
		return nil, nil
	}
	p := &packLooksPlan{}
	for _, s := range ls.Looks {
		for _, c := range s.Choices {
			if c.State == LookApplied {
				p.choices++
				break
			}
		}
	}
	trimmed := make([]map[string]bool, len(r.Pack.Entries))
	lose := func(entry int, f *looks.File) {
		pe := &r.Pack.Entries[entry]
		if trimmed[entry] == nil {
			trimmed[entry] = map[string]bool{}
			for _, t := range pe.TrimmedDir {
				trimmed[entry][t] = true
			}
		}
		// a version folder left out and RimArcho's own DDS are not copied (steps 3 and 5)
		if trimmed[entry][strings.Split(f.Rel, "/")[0]] || pl.gen[entry][strings.ToLower(f.Rel)] {
			return
		}
		p.losers = append(p.losers, packLoser{entry: entry, rel: f.Rel, size: f.Size})
	}
	for _, l := range ls.Index.Looks {
		wins := l.InGame(ls.ChoiceOf(l.ID))
		for _, prov := range l.Providers {
			if prov.Pos < 0 || prov.Pos >= len(r.Pack.Entries) || r.Pack.Entries[prov.Pos].Folder == "" {
				continue
			}
			for slot, f := range prov.Slots {
				if wins[slot].File == f {
					continue
				}
				lose(prov.Pos, f)
				if f.Alt != nil {
					lose(prov.Pos, f.Alt)
				}
				// its lower-priority load folders would supply the key in its place
				for _, u := range f.Under {
					lose(prov.Pos, u)
				}
			}
		}
	}
	return p, nil
}

// expect records the plan in the result as a dry run shows it ("would remove"): the files
// and bytes come off each copy's size.
func (p *packLooksPlan) expect(r *PackBuild) {
	if p == nil {
		return
	}
	pk := &PackLooks{Choices: p.choices}
	for _, x := range p.losers {
		pk.Removed++
		pk.Bytes += x.size
		r.Pack.Entries[x.entry].Bytes -= x.size
	}
	r.Final -= pk.Bytes
	if pk.Removed > 0 || pk.Choices > 0 {
		r.Pack.Looks = pk
	}
}

// packCopyFile resolves rel (a file's path relative to its Mod root, forward slashes)
// inside the pack copy dir. ok is false when rel leaves the copy, names no file below a
// Textures folder, or a folder on the way is a link: such a file is never touched.
func packCopyFile(dir, rel string) (path string, ok bool) {
	local := filepath.FromSlash(rel)
	if rel == "" || !filepath.IsLocal(local) || local != filepath.Clean(local) {
		return "", false
	}
	parts := strings.Split(local, string(filepath.Separator))
	tex := false
	for _, s := range parts[:len(parts)-1] {
		tex = tex || strings.EqualFold(s, "Textures")
	}
	if !tex {
		return "", false
	}
	path = dir
	for _, s := range parts[:len(parts)-1] {
		path = filepath.Join(path, s)
		fi, err := os.Lstat(path)
		if err != nil {
			return filepath.Join(dir, local), true // absent: nothing to remove
		}
		if !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 || source.IsLink(path) {
			return "", false
		}
	}
	path = filepath.Join(dir, local)
	if !within(dir, path) {
		return "", false
	}
	return path, true
}

// packLooks removes the planned files from the copies in tmp (the pack's build folder) and
// the folders that leaves empty below Textures/. A file the copy step did not copy is
// simply absent. It records what it removed in the pack.
func (a *App) packLooks(r *PackBuild, p *packLooksPlan, tmp string, bs *buildState) error {
	if p == nil {
		return nil
	}
	pk := &PackLooks{Choices: p.choices}
	mods := filepath.Join(tmp, "mods")
	if len(p.losers) > 0 { // a step of the build only when it has something to remove
		bs.update(func(b *BuildProgress) { b.Step, b.Mod, b.Files, b.FilesTotal = StepLooks, "", 0, len(p.losers) })
	}
	for i, x := range p.losers {
		if i%64 == 0 {
			if err := bs.stopRequested(); err != nil {
				return err
			}
		}
		pe := &r.Pack.Entries[x.entry]
		dir := filepath.Join(mods, pe.Folder)
		if pe.Folder == "" || !within(mods, dir) {
			continue
		}
		path, ok := packCopyFile(dir, x.rel)
		if !ok {
			return fmt.Errorf("looks: %s is not a texture inside the pack copy of %s; nothing more was removed", x.rel, pe.Name)
		}
		fi, err := os.Lstat(path)
		if err != nil {
			continue // not copied
		}
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("looks: %s in the pack copy of %s is not a file", x.rel, pe.Name)
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		pk.Removed++
		pk.Bytes += fi.Size()
		pe.Bytes -= fi.Size()
		removeEmptyTextureDirs(dir, filepath.Dir(path))
		bs.update(func(b *BuildProgress) { b.Files = i + 1 })
	}
	r.Final -= pk.Bytes
	r.Pack.Looks = nil
	if pk.Removed > 0 || pk.Choices > 0 {
		r.Pack.Looks = pk
	}
	return nil
}

// removeEmptyTextureDirs removes folder and its parents while they are empty and lie below
// a Textures folder of the copy dir (the Textures folder itself stays).
func removeEmptyTextureDirs(dir, folder string) {
	for within(dir, folder) && folder != dir {
		rel, err := filepath.Rel(dir, folder)
		if err != nil {
			return
		}
		parts := strings.Split(rel, string(filepath.Separator))
		below := false
		for _, s := range parts[:len(parts)-1] {
			below = below || strings.EqualFold(s, "Textures")
		}
		if !below || os.Remove(folder) != nil { // os.Remove fails on a folder that is not empty
			return
		}
		folder = filepath.Dir(folder)
	}
}
