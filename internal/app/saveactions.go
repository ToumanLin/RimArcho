package app

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/loadout"
)

// Save actions (12 §4.4). Fixing a save edits its list (one undoable step); rename and copy
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
	Waiting    []SaveMod // Workshop Mods the list already has as missing entries: only subscribed again
	Unresolved []SaveMod // not installed and without a Workshop ID: left out
	// Stuck: Mods the list already has as missing entries that no download installs: no
	// Workshop ID, or their Workshop item is installed under another packageId (Why). They
	// are final: --launch does not wait for them.
	Stuck []StuckMod
	// Already: Mods asked for by name (Mods) that the list already loads: nothing to do.
	Already   []SaveMod
	Disable   []SaveMod
	Reordered int
	Edit      *EditResult // nil for a dry run or when the list did not change (no undo step)
}

// StuckMod is a missing entry of the list that cannot be downloaded, and why.
type StuckMod struct {
	SaveMod
	Why string
}

// Stuck reasons.
const (
	StuckNoWorkshop = "no Workshop ID"
)

// StuckWhy is why a missing entry of the list can never be installed by a download ("" it
// can): no Workshop ID, or its Workshop item is installed with another packageId. ix is
// the save's game's index (nil: only the first is checked).
func StuckWhy(ix *Index, m SaveMod) string {
	if m.Instance != "" {
		return ""
	}
	if m.SteamID == "" {
		return StuckNoWorkshop
	}
	if ix != nil {
		for _, im := range ix.Mods {
			if im.WorkshopID() == m.SteamID && !strings.EqualFold(normPkg(im.PackageID), normPkg(m.PackageID)) {
				return "its Workshop item ws:" + m.SteamID + " is installed as " + im.PackageID
			}
		}
	}
	return ""
}

// Changed reports whether the fix changes (or, for a dry run, would change) the list.
func (r *SaveFixResult) Changed() bool {
	return len(r.Enable)+len(r.Subscribe)+len(r.Disable)+r.Reordered > 0
}

// WorkshopIDs are the IDs to subscribe to.
func (r *SaveFixResult) WorkshopIDs() []string {
	var ids []string
	for _, m := range append(append([]SaveMod{}, r.Subscribe...), r.Waiting...) {
		ids = append(ids, m.SteamID)
	}
	return ids
}

// FixSave applies a save's Mod fixes to the list it matched as one undoable edit (12 §4.4):
// missing Mods go in at their place in the save's order. Nothing is written to the game
// until save or launch. A dry run reports the same without editing.
func (a *App) FixSave(s *Save, opt SaveFixOptions, dryRun bool) (*SaveFixResult, error) {
	if s.Plan == nil {
		if s.Pack != "" {
			return nil, fmt.Errorf("%s is a save of pack %s; a pack's list does not change (loadout import %s --new makes a list from it)", s.Name, s.Pack, cmdQuote(s.Path))
		}
		return nil, fmt.Errorf("%s matches no list (loadout import %s --new makes one from it)", s.Name, cmdQuote(s.Path))
	}
	// The check and the edit see the same list: no other session's edit comes between them
	// (edit takes the same lock again, nested).
	unlock, err := a.lockLaunch()
	if err != nil {
		return nil, err
	}
	defer unlock()
	c, err := a.CheckSave(s)
	if err != nil {
		return nil, err
	}
	if c.Level == "writing" {
		return nil, fmt.Errorf("%s is being written; try again when the game has saved", s.Name)
	}
	ix, err := a.SaveIndex(s)
	if err != nil {
		return nil, err
	}
	all := len(opt.Mods) == 0 && !opt.DisableAdded && !opt.Order
	want := map[string]bool{}
	for _, m := range opt.Mods {
		want[normPkg(m)] = true
	}
	r := &SaveFixResult{Save: s, Plan: s.Plan.Name}
	var add []SaveMod
	for _, m := range c.Mods {
		if !all && !want[normPkg(m.PackageID)] {
			continue
		}
		delete(want, normPkg(m.PackageID))
		switch {
		case m.InList && m.Instance != "":
			if !all { // asked for by name and done already (a fix run again after the download)
				r.Already = append(r.Already, m)
			}
		case m.InList:
			if why := StuckWhy(ix, m); why != "" {
				r.Stuck = append(r.Stuck, StuckMod{SaveMod: m, Why: why})
			} else {
				r.Waiting = append(r.Waiting, m)
			}
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
	if r.Edit != nil && len(r.Edit.Changes) == 0 {
		r.Edit = nil // edit records no step for an unchanged list
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

// RenameSave renames a save in its folder, keeping a RimSaves folder prefix (12 §4.4). It
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
	if to == s.Path && file == s.File {
		return to, nil
	}
	if target, err := os.Lstat(to); err == nil {
		original, err := os.Lstat(s.Path)
		if err != nil {
			return "", err
		}
		if !os.SameFile(original, target) {
			return "", fmt.Errorf("a save named %s already exists", name)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	// never over a file that appeared since the check (os.Rename replaces on Windows)
	if err := renameNoReplace(s.Path, to); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("a save named %s already exists", name)
		}
		return "", err
	}
	return to, nil
}

// CopySave copies a save next to itself as "<name> (copy)", never overwriting, and keeps
// its modification time so it sorts beside the original (12 §4.4).
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
	// Token binds a confirmation to this preview (SaveFilesToken): the saves as listed, by
	// reference, size and modification time, and a backup's time folder.
	Token string
}

// ErrPreviewChanged: a confirmation names saves that changed since its preview (an
// autosave written again under the same name, 12 §4.4).
var ErrPreviewChanged = errors.New("the saves changed since the preview; preview again")

// backupStamp is a backup's time folder name, YYYYMMDD-HHMMSS in local time (17 §3).
const backupStamp = "20060102-150405"

// SaveFilesToken is the token of a backup or recycle preview of saves: a short hash of each
// save's reference, size and modification time, after stamp (a backup's time folder; ""
// for recycling) and a dot.
func SaveFilesToken(saves []*Save, stamp string) string {
	keys := make([]string, len(saves))
	for i, s := range saves {
		keys[i] = s.Ref + "|" + strconv.FormatInt(s.Size, 10) + "|" + strconv.FormatInt(s.ModTime.UnixNano(), 10)
	}
	sort.Strings(keys)
	sum := sha1.Sum([]byte(strings.Join(keys, "\n")))
	t := hex.EncodeToString(sum[:5])
	if stamp != "" {
		t = stamp + "." + t
	}
	return t
}

// statNow is saves as their files are now: copies with the size and modification time
// read again, since a listing (the GUI's) may be older than an autosave written over one.
// A file that cannot be stat'ed keeps the listing's (its action then reports the error).
func statNow(saves []*Save) []*Save {
	out := make([]*Save, len(saves))
	for i, s := range saves {
		c := *s
		if fi, err := os.Stat(s.Path); err == nil {
			c.Size, c.ModTime = fi.Size(), fi.ModTime()
		}
		out[i] = &c
	}
	return out
}

// checkToken compares a confirmation's token with the saves as they are now; "" checks
// nothing (a player confirming on the command line). It returns the token's time folder.
func checkToken(saves []*Save, token string, backup bool) (string, error) {
	if token == "" {
		return "", nil
	}
	stamp := ""
	if backup {
		var ok bool
		if stamp, _, ok = strings.Cut(token, "."); !ok {
			return "", ErrPreviewChanged
		}
		if _, err := time.Parse(backupStamp, stamp); err != nil {
			return "", ErrPreviewChanged
		}
	}
	if SaveFilesToken(saves, stamp) != token {
		return "", ErrPreviewChanged
	}
	return stamp, nil
}

// BackupSaves copies saves to <data>/backups/saves/<time>/ (17 §3, 12 §4.4). Without yes it
// only previews the files and their size, with a Token; a confirmation with that token
// copies into the preview's time folder (another suffixed -2, -3… when a backup took it),
// and refuses when the saves changed since. A file whose name another save in this backup
// has goes into a subfolder named after its pack or game.
func (a *App) BackupSaves(saves []*Save, yes bool, token string) (*SaveFilesResult, error) {
	saves = statNow(saves)
	stamp := a.Now().In(a.Loc).Format(backupStamp)
	if yes {
		st, err := checkToken(saves, token, true)
		if err != nil {
			return nil, err
		}
		if st != "" {
			stamp = st
		}
	}
	root := filepath.Join(a.Home, "backups", "saves")
	r := &SaveFilesResult{Saves: saves, Dir: filepath.Join(root, stamp), Token: SaveFilesToken(saves, stamp)}
	for _, s := range saves {
		r.Bytes += s.Size
	}
	if !yes {
		return r, nil
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	for n := 2; ; n++ { // a folder of its own: two backups in one second do not mix
		err := os.Mkdir(r.Dir, 0o755)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		r.Dir = filepath.Join(root, fmt.Sprintf("%s-%d", stamp, n))
	}
	used := map[string]bool{}
	for _, s := range saves {
		rel := backupName(s, used)
		to := filepath.Join(r.Dir, rel)
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			r.Errs = append(r.Errs, s.Name+": "+err.Error())
			continue
		}
		if err := copyFileExcl(s.Path, to); err != nil {
			r.Errs = append(r.Errs, s.Name+": "+err.Error())
			continue
		}
		os.Chtimes(to, s.ModTime, s.ModTime)
	}
	r.Done = true
	return r, nil
}

// backupName is where a save goes in its backup folder, relative to it: its file name, or,
// when another save of the backup has that name (saves of two folders), the same name in a
// subfolder named after its pack or game ("2", "3"… when that is taken too).
func backupName(s *Save, used map[string]bool) string {
	key := func(p string) string { return strings.ToLower(filepath.ToSlash(p)) }
	if !used[key(s.File)] {
		used[key(s.File)] = true
		return s.File
	}
	sub := ""
	if s.Folder != nil {
		sub = s.Folder.Pack
		if sub == "" && len(s.Folder.Envs) > 0 {
			sub = s.Folder.Envs[0].Name
		}
	}
	sub = strings.TrimSpace(strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) || r < ' ' {
			return '_'
		}
		return r
	}, sub))
	sub = strings.Trim(sub, ". ")
	for n := 1; ; n++ {
		dir := sub
		switch {
		case dir == "":
			dir = strconv.Itoa(n + 1)
		case n > 1:
			dir = fmt.Sprintf("%s %d", sub, n)
		}
		rel := filepath.Join(dir, s.File)
		if !used[key(rel)] {
			used[key(rel)] = true
			return rel
		}
	}
}

// TrashSaves moves saves to the recycle bin (02 §3, 12 §4.4), refusing while a game runs,
// the preview too. Without yes it only previews the files and their size, with a Token; a
// confirmation with that token refuses when the saves changed since.
func (a *App) TrashSaves(saves []*Save, yes bool, token string) (*SaveFilesResult, error) {
	saves = statNow(saves)
	r := &SaveFilesResult{Saves: saves, Token: SaveFilesToken(saves, "")}
	for _, s := range saves {
		r.Bytes += s.Size
	}
	if err := a.refuseWhileRunning(); err != nil {
		return nil, err
	}
	if !yes {
		return r, nil
	}
	if _, err := checkToken(saves, token, false); err != nil {
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
