package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/loadout"
)

// Save actions (10 §4.4). Fixing a save edits its list (one undoable step); rename and copy
// change files in the save's folder when the player asks; backup copies into the data
// directory and recycling moves to the recycle bin, both only after confirmation (yes).

// ErrGameRunning: the action would change a save the running game may read or write.
var ErrGameRunning = errors.New("the game is running")

// SaveFixOptions chooses which of a save's fixes to apply. With none set, every Mod fix
// (enable installed Mods, add Workshop Mods to subscribe).
type SaveFixOptions struct {
	Mods         []string // packageIds of the Mods to enable or subscribe
	DisableAdded bool     // disable the list's Mods the save lacks
	Order        bool     // use the save's order for the Mods both have
}

// SaveFixResult is what a fix does or would do.
type SaveFixResult struct {
	Save       *Save
	Plan       string
	Enable     []SaveMod // installed Mods enabled
	Subscribe  []SaveMod // Workshop Mods added as missing entries; the caller subscribes (SubscribeWorkshop)
	Unresolved []SaveMod // not installed and without a Workshop ID: left out
	Disable    []SaveMod
	Reordered  int
	Edit       *EditResult // nil for a dry run or when nothing changed
}

// WorkshopIDs are the IDs to subscribe to.
func (r *SaveFixResult) WorkshopIDs() []string {
	var ids []string
	for _, m := range r.Subscribe {
		ids = append(ids, m.SteamID)
	}
	return ids
}

// FixSave applies a save's Mod fixes to the list it matched as one undoable edit (10 §4.4):
// missing Mods go in at their place in the save's order. Nothing is written to the game
// until save or launch. A dry run reports the same without editing.
func (a *App) FixSave(s *Save, opt SaveFixOptions, dryRun bool) (*SaveFixResult, error) {
	if s.Plan == nil {
		if s.Pack != "" {
			return nil, fmt.Errorf("%s is a save of pack %s; a pack's list does not change (loadout import %s --new makes a list from it)", s.Name, s.Pack, cmdQuote(s.Path))
		}
		return nil, fmt.Errorf("%s matches no list (loadout import %s --new makes one from it)", s.Name, cmdQuote(s.Path))
	}
	c, err := a.CheckSave(s)
	if err != nil {
		return nil, err
	}
	if c.Level == "writing" {
		return nil, fmt.Errorf("%s is being written; try again when the game has saved", s.Name)
	}
	all := len(opt.Mods) == 0 && !opt.DisableAdded && !opt.Order
	want := map[string]bool{}
	for _, m := range opt.Mods {
		want[normPkg(m)] = true
	}
	r := &SaveFixResult{Save: s, Plan: s.Plan.Name}
	var add []SaveMod
	for _, m := range c.Mods {
		if m.InList || (!all && !want[normPkg(m.PackageID)]) {
			continue
		}
		delete(want, normPkg(m.PackageID))
		switch {
		case m.Instance != "":
			r.Enable = append(r.Enable, m)
			add = append(add, m)
		case m.SteamID != "":
			r.Subscribe = append(r.Subscribe, m)
			add = append(add, m)
		default:
			r.Unresolved = append(r.Unresolved, m)
		}
	}
	for pkg := range want {
		return nil, fmt.Errorf("%s: no Mod of %s to enable (archo saves show %s)", pkg, s.Name, s.Ref)
	}
	if opt.DisableAdded {
		r.Disable = c.ListOnly
	}
	apply := func(st *LoadoutState, l loadout.List) (loadout.List, []string, error) {
		l = insertInSaveOrder(st, l, c.Mods, add)
		if opt.DisableAdded {
			drop := map[string]bool{}
			for _, m := range c.ListOnly {
				drop[normPkg(m.PackageID)] = true
			}
			kept := l[:0:0]
			for _, e := range l {
				if !drop[normPkg(entryPkg(st, e))] {
					kept = append(kept, e)
				}
			}
			l = kept
		}
		if opt.Order {
			l, r.Reordered = saveOrder(st, l, c.Mods)
		}
		return l, nil, nil
	}
	err = a.inSaveList(s, func() error {
		if dryRun {
			st, err := a.Loadout()
			if err != nil {
				return err
			}
			_, _, err = apply(st, st.List.Clone())
			return err
		}
		r.Edit, err = a.edit("fix from save "+s.Name, apply)
		return err
	})
	if err != nil {
		return nil, err
	}
	return r, nil
}

// entryPkg is the lower-case packageId of a list entry.
func entryPkg(st *LoadoutState, e loadout.Entry) string {
	if m := st.Ix.ByID[e.Instance]; m != nil {
		return strings.ToLower(m.PackageID)
	}
	return strings.ToLower(e.Missing)
}

// insertInSaveOrder adds Mods to l, each right after the nearest Mod before it in the save
// that l has (at the start when none).
func insertInSaveOrder(st *LoadoutState, l loadout.List, saveMods, add []SaveMod) loadout.List {
	adding := map[string]SaveMod{}
	for _, m := range add {
		adding[normPkg(m.PackageID)] = m
	}
	for i, m := range saveMods {
		am, ok := adding[normPkg(m.PackageID)]
		if !ok {
			continue
		}
		at := 0
		for j := i - 1; j >= 0 && at == 0; j-- {
			for k, e := range l {
				if normPkg(entryPkg(st, e)) == normPkg(saveMods[j].PackageID) {
					at = k + 1
					break
				}
			}
		}
		e := loadout.Entry{Instance: am.Instance}
		if am.Instance == "" {
			e = loadout.Entry{Missing: am.PackageID, Name: am.Name, WorkshopID: am.SteamID}
		}
		l = append(l[:at], append(loadout.List{e}, l[at:]...)...)
	}
	return l
}

// saveOrder puts the entries the save also has into the save's order, in the places those
// entries take in l; the others stay where they are. It returns how many moved.
func saveOrder(st *LoadoutState, l loadout.List, saveMods []SaveMod) (loadout.List, int) {
	rank := map[string]int{}
	for i, m := range saveMods {
		rank[normPkg(m.PackageID)] = i
	}
	var slots []int
	var common loadout.List
	for i, e := range l {
		if _, ok := rank[normPkg(entryPkg(st, e))]; ok {
			slots = append(slots, i)
			common = append(common, e)
		}
	}
	sorted := common.Clone()
	slicesSortStable(sorted, func(x, y loadout.Entry) bool {
		return rank[normPkg(entryPkg(st, x))] < rank[normPkg(entryPkg(st, y))]
	})
	out := l.Clone()
	moved := 0
	for k, i := range slots {
		if out[i].Key() != sorted[k].Key() {
			moved++
		}
		out[i] = sorted[k]
	}
	return out, moved
}

func slicesSortStable(l loadout.List, less func(a, b loadout.Entry) bool) {
	for i := 1; i < len(l); i++ {
		for j := i; j > 0 && less(l[j], l[j-1]); j-- {
			l[j], l[j-1] = l[j-1], l[j]
		}
	}
}

// refuseWhileRunning stops an action that changes a save while a game runs.
func (a *App) refuseWhileRunning() error {
	g, err := a.RunningGame()
	if err != nil {
		return err
	}
	if g != nil {
		return fmt.Errorf("%w: %s", ErrGameRunning, g.Describe(a.Loc))
	}
	return nil
}

// RenameSave renames a save in its folder, keeping a RimSaves folder prefix (10 §4.4). It
// refuses an existing name, a name with path characters, and while a game runs.
func (a *App) RenameSave(s *Save, name string) (string, error) {
	name = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(name), ".rws"))
	switch {
	case name == "":
		return "", errors.New("the new name is empty")
	case strings.ContainsAny(name, `\/:*?"<>|`) || strings.Contains(name, rimSavesSep):
		return "", fmt.Errorf("%q: a save name cannot contain \\ / : * ? \" < > | or %s", name, rimSavesSep)
	case s.Writing:
		return "", fmt.Errorf("%s is being written; try again when the game has saved", s.Name)
	}
	if err := a.refuseWhileRunning(); err != nil {
		return "", err
	}
	file := name + ".rws"
	if s.Prefix != "" {
		file = s.Prefix + rimSavesSep + file
	}
	to := filepath.Join(filepath.Dir(s.Path), file)
	if strings.EqualFold(to, s.Path) && file == s.File {
		return to, nil
	}
	if _, err := os.Stat(to); err == nil && !strings.EqualFold(to, s.Path) {
		return "", fmt.Errorf("a save named %s already exists", name)
	}
	if err := os.Rename(s.Path, to); err != nil {
		return "", err
	}
	return to, nil
}

// CopySave copies a save next to itself as "<name> (copy)", never overwriting, and keeps
// its modification time so it sorts beside the original (10 §4.4).
func (a *App) CopySave(s *Save) (string, error) {
	if s.Writing {
		return "", fmt.Errorf("%s is being written; try again when the game has saved", s.Name)
	}
	base := s.Name + " (copy)"
	for n := 1; ; n++ {
		name := base
		if n > 1 {
			name = fmt.Sprintf("%s (copy %d)", s.Name, n)
		}
		file := name + ".rws"
		if s.Prefix != "" {
			file = s.Prefix + rimSavesSep + file
		}
		to := filepath.Join(filepath.Dir(s.Path), file)
		err := copyFileExcl(s.Path, to)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		os.Chtimes(to, s.ModTime, s.ModTime)
		return to, nil
	}
}

// copyFileExcl copies src to a new file dst; an existing dst is os.ErrExist.
func copyFileExcl(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

// SaveFilesResult is the preview or result of backing up or recycling saves.
type SaveFilesResult struct {
	Saves []*Save
	Bytes int64
	Dir   string   // backup: the time folder
	Done  bool     // false: preview only
	Errs  []string // per save, when some failed
}

// BackupSaves copies saves to <data>/backups/saves/<time>/ (07 §3, 10 §4.4). Without yes it
// only previews the files and their size.
func (a *App) BackupSaves(saves []*Save, yes bool) (*SaveFilesResult, error) {
	r := &SaveFilesResult{Saves: saves, Dir: filepath.Join(a.Home, "backups", "saves", a.Now().In(a.Loc).Format("20060102-150405"))}
	for _, s := range saves {
		r.Bytes += s.Size
	}
	if !yes {
		return r, nil
	}
	if err := os.MkdirAll(r.Dir, 0o755); err != nil {
		return nil, err
	}
	for _, s := range saves {
		to := filepath.Join(r.Dir, s.File)
		if err := copyFileExcl(s.Path, to); err != nil {
			r.Errs = append(r.Errs, s.Name+": "+err.Error())
			continue
		}
		os.Chtimes(to, s.ModTime, s.ModTime)
	}
	r.Done = true
	return r, nil
}

// TrashSaves moves saves to the recycle bin (08 §7, 10 §4.4), refusing while a game runs.
// Without yes it only previews the files and their size.
func (a *App) TrashSaves(saves []*Save, yes bool) (*SaveFilesResult, error) {
	r := &SaveFilesResult{Saves: saves}
	for _, s := range saves {
		r.Bytes += s.Size
	}
	if !yes {
		return r, nil
	}
	if err := a.refuseWhileRunning(); err != nil {
		return nil, err
	}
	for _, s := range saves {
		if s.Writing {
			r.Errs = append(r.Errs, s.Name+": being written")
			continue
		}
		if err := a.ToTrash(s.Path); err != nil {
			r.Errs = append(r.Errs, s.Name+": "+err.Error())
		}
	}
	r.Done = true
	return r, nil
}
