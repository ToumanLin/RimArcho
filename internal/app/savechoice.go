package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
)

// Which save Can it run checks (12 §4.8): by default the save the Game Config's last run
// loaded (LastLoadedSave); the player may choose a save, or a colony, whose newest save is
// then checked. The choice is kept per Game Config, so switching configs and back keeps
// each one's choice, and it may name a save in another config's Saves folder (Can it run
// then says the game started with this config does not see it).

// SaveChoice is a Game Config's chosen save or colony.
type SaveChoice struct {
	Kind   string `json:"kind"`             // ChooseSaveFile or ChooseColony
	Path   string `json:"path,omitempty"`   // the save file (ChooseSaveFile)
	Colony string `json:"colony,omitempty"` // the colony's key (ChooseColony)
	Dir    string `json:"dir"`              // the Saves folder it is in
	Name   string `json:"name"`             // the save's or colony's name when chosen
	// Also are the colony's other keys (ChooseColony): the file-name keys of its saves. A
	// colony's saves group by file name until they are read in full, then by their world
	// (12 §4.1), so the key changes both ways: a colony chosen before the full read, or a
	// world colony saved again and not read yet.
	Also []string `json:"also,omitempty"`
}

// SaveChoice kinds.
const (
	ChooseSaveFile = "save"
	ChooseColony   = "colony"
)

func saveChoiceKey(envID string) string { return "saves.check." + envID }

// SaveChoiceOf is the Game Config's choice; nil: none (the last loaded save is checked).
func (a *App) SaveChoiceOf(envID string) *SaveChoice {
	v := a.Store.Meta(saveChoiceKey(envID))
	if v == "" {
		return nil
	}
	var c SaveChoice
	if json.Unmarshal([]byte(v), &c) != nil || (c.Kind != ChooseSaveFile && c.Kind != ChooseColony) {
		return nil
	}
	return &c
}

// ChooseSave makes s (colony: its colony) what Can it run checks for the Game Config envID.
func (a *App) ChooseSave(envID string, s *Save, colony bool) (*SaveChoice, error) {
	c := &SaveChoice{Kind: ChooseSaveFile, Path: s.Path, Dir: filepath.Dir(s.Path), Name: s.Name}
	if colony {
		if s.Colony == nil {
			return nil, fmt.Errorf("%s is in no colony", s.Name)
		}
		c = &SaveChoice{Kind: ChooseColony, Colony: s.Colony.Key, Also: colonyAliases(s.Colony), Dir: filepath.Dir(s.Path), Name: s.Colony.Name}
	}
	b, _ := json.Marshal(c)
	return c, a.Store.SetMeta(saveChoiceKey(envID), string(b))
}

// ClearSaveChoice goes back to checking the last loaded save.
func (a *App) ClearSaveChoice(envID string) error {
	return a.Store.SetMeta(saveChoiceKey(envID), "")
}

// ForgetSaveChoice removes a removed Game Config's choice from the store.
func (a *App) ForgetSaveChoice(envID string) error {
	_, err := a.Store.DB.Exec(`DELETE FROM meta WHERE key=?`, saveChoiceKey(envID))
	return err
}

// SaveChoiceStamp changes when what the Game Config's choice resolves to may have changed:
// the chosen file, or every save file in the chosen colony's folder (a save written over
// in place, such as an autosave, need not change the folder's own modification time).
// "" with no choice.
func (a *App) SaveChoiceStamp(envID string) string {
	ch := a.SaveChoiceOf(envID)
	if ch == nil {
		return ""
	}
	stamp := func(fi os.FileInfo) string {
		return fmt.Sprintf("%s:%d:%d;", fi.Name(), fi.Size(), fi.ModTime().UnixNano())
	}
	if ch.Kind == ChooseSaveFile {
		fi, err := os.Stat(ch.Path)
		if err != nil {
			return ch.Path + ":" + err.Error()
		}
		return ch.Path + "|" + stamp(fi)
	}
	ents, err := os.ReadDir(ch.Dir)
	if err != nil {
		return ch.Dir + ":" + err.Error()
	}
	var b strings.Builder
	b.WriteString(ch.Dir + "|" + ch.Colony + "|")
	for _, de := range ents {
		if de.IsDir() || !strings.EqualFold(filepath.Ext(de.Name()), ".rws") {
			continue
		}
		if fi, err := de.Info(); err == nil {
			b.WriteString(stamp(fi))
		}
	}
	return b.String()
}

// Chose: s, or its colony (colony), is the Game Config's choice.
func (c *SaveChoice) Chose(s *Save, colony bool) bool {
	if c == nil || s == nil {
		return false
	}
	if colony {
		return c.Kind == ChooseColony && s.Colony != nil && c.names(s.Colony)
	}
	return c.Kind == ChooseSaveFile && env.PathKey(c.Path) == env.PathKey(s.Path)
}

// ChosenSave checks the Game Config's chosen save, or its chosen colony's newest save,
// against st's list as it is now (Can it run's Before you start, 16 §2.4). nil check with
// a choice: the file or colony is gone. Only the chosen file, or the chosen colony's
// folder, is read.
func (a *App) ChosenSave(st *LoadoutState) (*SaveCheck, *SaveChoice, error) {
	if st == nil || st.Plan == nil || st.Ix == nil || st.Ix.Env == nil {
		return nil, nil, nil
	}
	ch := a.SaveChoiceOf(st.Ix.Env.ID)
	if ch == nil {
		return nil, nil, nil
	}
	f, err := a.saveFolderOf(ch.Dir)
	if err != nil {
		return nil, ch, err
	}
	if ch.Kind == ChooseSaveFile {
		fi, err := os.Stat(ch.Path)
		if err != nil || fi.IsDir() {
			return nil, ch, nil
		}
		sv := &Save{Path: ch.Path, File: filepath.Base(ch.Path), Folder: f}
		c, err := a.checkOneSave(st, sv, nil)
		return c, ch, err
	}
	ents, err := os.ReadDir(f.Dir)
	if err != nil {
		return nil, ch, nil
	}
	var saves []*Save
	for _, de := range ents {
		if de.IsDir() || !strings.EqualFold(filepath.Ext(de.Name()), ".rws") {
			continue
		}
		s := &Save{Path: filepath.Join(f.Dir, de.Name()), File: de.Name(), Folder: f}
		s.Prefix, s.Name = SplitSaveName(s.File)
		s.Auto = autosaveRe.MatchString(s.Name)
		saves = append(saves, s)
	}
	if len(saves) == 0 {
		return nil, ch, nil
	}
	// Group the folder as ListSaves does to find the colony's newest save.
	probe := make([]*Save, len(saves))
	for i, s := range saves {
		c := *s
		c.Ref = SaveRef(c.Path)
		probe[i] = &c
	}
	a.readHeads(probe)
	if err := a.attachSummaries(probe); err != nil {
		return nil, ch, err
	}
	m, err := a.newSaveMatcher()
	if err != nil {
		return nil, ch, err
	}
	for _, s := range probe {
		m.match(s)
	}
	col := ch.colonyIn(groupColonies(probe))
	if col == nil {
		return nil, ch, nil
	}
	if col.By == "world" && col.Key != ch.Colony {
		// chosen while its saves were grouped by file name: they have been read in full
		// since and form their world's colony, whose key stays; keep that one
		moved := *ch
		moved.Colony, moved.Also = col.Key, colonyAliases(col)
		if !slices.Contains(moved.Also, ch.Colony) {
			moved.Also = append(moved.Also, ch.Colony)
		}
		if b, err := json.Marshal(&moved); err == nil && a.Store.SetMeta(saveChoiceKey(st.Ix.Env.ID), string(b)) == nil {
			ch = &moved
		}
	}
	latest := col.Latest()
	var rest []*Save
	var sv *Save
	for _, s := range saves {
		if s.Path == latest.Path {
			sv = s
		} else if latest.Colony != nil && inColony(latest.Colony, s.Path) {
			rest = append(rest, s)
		}
	}
	c, err := a.checkOneSave(st, sv, rest)
	return c, ch, err
}

// colonyAliases are a colony's file-name keys (SaveChoice.Also): those it claims
// (nameKeys, set by ListSaves) and those of its saves saved by hand.
func colonyAliases(c *Colony) []string {
	var out []string
	add := func(k string) {
		if k != c.Key && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	for _, k := range c.nameKeys {
		add(k)
	}
	if c.By != "rimsaves" {
		for _, s := range c.Saves {
			if !s.Auto && s.Folder != nil {
				add(colonyNameKey(s))
			}
		}
	}
	return out
}

// names: c is the colony the choice names, by its key or another key it had (Also).
func (ch *SaveChoice) names(c *Colony) bool {
	if c.Key == ch.Colony || slices.Contains(ch.Also, c.Key) {
		return true
	}
	return slices.Contains(c.nameKeys, ch.Colony)
}

// colonyIn is the colony of cols the choice names: of those with its key or another key
// it had, or the world colony claiming its key as a file-name key (12 §4.1), the one with
// the newest save. A world colony saved again groups its new saves by file name until
// they are read in full, and those saves are its newest.
func (ch *SaveChoice) colonyIn(cols []*Colony) *Colony {
	claims := nameClaims(cols)
	var best *Colony
	for _, c := range cols {
		if !ch.names(c) && claims[ch.Colony] != c {
			continue
		}
		if best == nil || c.Latest().ModTime.After(best.Latest().ModTime) {
			best = c
		}
	}
	return best
}

func inColony(c *Colony, path string) bool {
	for _, s := range c.Saves {
		if s.Path == path {
			return true
		}
	}
	return false
}

// saveFolderOf is the Saves folder dir as SaveFolders lists it (with the Game Configs and
// pack it belongs to), or a folder no config uses any more.
func (a *App) saveFolderOf(dir string) (*SaveFolder, error) {
	folders, err := a.SaveFolders()
	if err != nil {
		return nil, err
	}
	key := env.PathKey(realOrAbs(dir))
	for _, f := range folders {
		if env.PathKey(realOrAbs(f.Dir)) == key {
			return f, nil
		}
	}
	return &SaveFolder{Dir: dir}, nil
}

// SeenBy: the game started with Game Config e (with no pack) lists this save: it is in
// e's user data Saves folder, not a pack's.
func (s *Save) SeenBy(e *env.Environment) bool {
	return e != nil && s.Folder != nil && s.Folder.Pack == "" && folderHasEnv(s.Folder, e.ID)
}

// SavesDir is the Saves folder of Game Config e's user data.
func (a *App) SavesDir(e *env.Environment) string {
	root := e.UserDataRoot
	if root == "" {
		root = a.DefaultUserData()
	}
	if root == "" {
		return ""
	}
	return filepath.Join(root, "Saves")
}

// ErrSameFolder: the save is in that Saves folder already.
var ErrSameFolder = errors.New("the save is in this Game Config's Saves folder already")

// CopySaveTo copies s into Game Config e's Saves folder (12 §4.4): under its own file name,
// else "<name> (copy)" and so on, never overwriting, keeping its modification time. Only
// the save file is copied: no Mod, list or setting.
func (a *App) CopySaveTo(s *Save, e *env.Environment) (string, error) {
	if s.Writing {
		return "", fmt.Errorf("%s is being written; try again when the game has saved", s.Name)
	}
	dir := a.SavesDir(e)
	if dir == "" {
		return "", fmt.Errorf("%s has no user data folder", e.Name)
	}
	if env.PathKey(realOrAbs(dir)) == env.PathKey(realOrAbs(filepath.Dir(s.Path))) {
		return "", ErrSameFolder
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return copySaveInto(s, dir, true)
}
