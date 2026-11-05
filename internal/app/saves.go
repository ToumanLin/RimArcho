package app

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/savefile"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Saves (12 §4): the folders games and packs save to, each save's head (12 §1.7 L0), the
// colony it belongs to (12 §4.1) and the list it matches (12 §4.2). Nothing here writes a save.

// How a save matched its list (12 §4.2).
const (
	MatchPack   = "pack"      // rule 1: the save is in a pack's folder
	MatchLaunch = "launch"    // rule 2: saved after a launch of that list
	MatchMods   = "same-mods" // rule 3: same Mods as a version of the list
)

// WritingWindow: a save modified this recently may still be written (12 §1.7).
const WritingWindow = 5 * time.Second

// SaveFolder is one Saves folder: a game's user data folder (shared by every environment
// using it) or a pack's own.
type SaveFolder struct {
	Dir  string
	Envs []*env.Environment // the games that save here (none for a pack of a removed game)
	Pack string             // the pack whose folder this is
}

// Save is one .rws file as read now.
type Save struct {
	Ref     string `json:"ref"`  // sv:<6 hex>, sha1 of the real path (13 §2)
	Path    string `json:"path"` // full path
	File    string `json:"file"` // base name
	Name    string `json:"name"` // shown: RimSaves folder and .rws removed
	Prefix  string `json:"prefix,omitempty"`
	Size    int64  `json:"size"`
	ModTime time.Time
	Auto    bool `json:"auto"` // Autosave-N
	Folder  *SaveFolder
	Head    *savefile.Head // nil when HeadErr is set
	HeadErr string
	// Writing: modified within WritingWindow, or changed while its head was read.
	Writing bool
	Colony  *Colony
	// Summary is the full read's summary (12 §1.7 L1) when the cache has a valid one;
	// nil: read from the head only ("meta only"). SaveFacts gives the whole read.
	Summary *SaveSummary

	// The list it matches (12 §4.2): a plan, a pack, or neither.
	Env   *env.Environment
	Plan  *store.Plan
	Pack  string
	Match string
}

// ListName names what the save matched: the plan's or pack's name ("" for none).
func (s *Save) ListName() string {
	switch {
	case s.Plan != nil:
		return s.Plan.Name
	case s.Pack != "":
		return s.Pack
	}
	return ""
}

// Colony is the group of saves of one game (12 §4.1).
type Colony struct {
	Key   string  `json:"key"`
	Name  string  `json:"name"`
	Saves []*Save // newest first
	// By: how the saves were grouped: "rimsaves" (folder prefix), "world" (the full
	// read's world identity) or "name" (file name).
	By string `json:"by"`
}

// Latest is the colony's newest save: the colony's facts come from it.
func (c *Colony) Latest() *Save { return c.Saves[0] }

// Orphan: the latest save matches no list (12 §4.1).
func (c *Colony) Orphan() bool { return c.Latest().Match == "" }

// SaveList is every save RimArcho can see.
type SaveList struct {
	Folders  []*SaveFolder
	Colonies []*Colony // by latest save, newest first
	Errors   []string  // folders that could not be read
}

// Saves lists every save in order of the colonies, newest first.
func (l *SaveList) Saves() []*Save {
	var out []*Save
	for _, c := range l.Colonies {
		out = append(out, c.Saves...)
	}
	return out
}

// SaveFolders lists the Saves folders of every environment (deduplicated by real path:
// environments may share the game's default user data, 03 §3.4) and every pack.
func (a *App) SaveFolders() ([]*SaveFolder, error) {
	envs, err := a.Store.Envs()
	if err != nil {
		return nil, err
	}
	var out []*SaveFolder
	byKey := map[string]*SaveFolder{}
	add := func(dir string, e *env.Environment, pack string) {
		key := env.PathKey(realOrAbs(dir))
		f := byKey[key]
		if f == nil {
			f = &SaveFolder{Dir: dir, Pack: pack}
			byKey[key] = f
			out = append(out, f)
		}
		if e != nil {
			f.Envs = append(f.Envs, e)
		}
	}
	for _, e := range envs {
		root := e.UserDataRoot
		if root == "" {
			root = a.DefaultUserData()
		}
		if root != "" {
			add(filepath.Join(root, "Saves"), e, "")
		}
	}
	packs, err := a.Packs()
	if err != nil {
		return out, nil // packs are optional here; the game folders still list
	}
	for _, p := range packs {
		var pe *env.Environment
		for _, e := range envs {
			if e.Name == p.Env {
				pe = e
			}
		}
		add(filepath.Join(p.Dir, "userdata", "Saves"), pe, p.Name)
	}
	return out, nil
}

// ListSaves reads every save's head (cached by path, size and mtime), groups them into
// colonies and matches each to a list. The heads of new or changed files are read in
// parallel; an unchanged library costs one directory listing and a stat per save.
func (a *App) ListSaves() (*SaveList, error) {
	folders, err := a.SaveFolders()
	if err != nil {
		return nil, err
	}
	l := &SaveList{Folders: folders}
	var saves []*Save
	for _, f := range folders {
		ents, err := os.ReadDir(f.Dir)
		if err != nil {
			if !os.IsNotExist(err) {
				l.Errors = append(l.Errors, err.Error())
			}
			continue
		}
		for _, de := range ents {
			if de.IsDir() || !strings.EqualFold(filepath.Ext(de.Name()), ".rws") {
				continue
			}
			s := &Save{Path: filepath.Join(f.Dir, de.Name()), File: de.Name(), Folder: f}
			s.Ref = SaveRef(s.Path)
			s.Prefix, s.Name = SplitSaveName(de.Name())
			s.Auto = autosaveRe.MatchString(s.Name)
			saves = append(saves, s)
		}
	}
	a.readHeads(saves)
	if err := a.attachSummaries(saves); err != nil {
		return nil, err
	}
	m, err := a.newSaveMatcher()
	if err != nil {
		return nil, err
	}
	for _, s := range saves {
		m.match(s)
	}
	l.Colonies = groupColonies(saves)
	return l, nil
}

// FindSave resolves a save reference (13 §2): sv:<hex>, a full path, a file name, or
// <colony>/<save>. Several matches are an error that lists them.
func (a *App) FindSave(ref string) (*Save, *SaveList, error) {
	l, err := a.ListSaves()
	if err != nil {
		return nil, nil, err
	}
	s, err := l.Find(ref)
	return s, l, err
}

// ErrNoSave: a save reference matches nothing.
var ErrNoSave = errors.New("no such save")

// Find resolves a save reference within the list.
func (l *SaveList) Find(ref string) (*Save, error) {
	ref = strings.TrimSpace(ref)
	var hits []*Save
	for _, s := range l.Saves() {
		switch {
		case strings.EqualFold(ref, s.Ref), env.PathKey(ref) == env.PathKey(s.Path),
			strings.EqualFold(ref, s.File), strings.EqualFold(ref, s.Name) && s.Prefix == "",
			strings.EqualFold(ref, s.Colony.Name+"/"+s.Name):
			hits = append(hits, s)
		}
	}
	switch len(hits) {
	case 0:
		return nil, fmt.Errorf("%w: %s (archo saves list)", ErrNoSave, ref)
	case 1:
		return hits[0], nil
	}
	return nil, &AmbiguousSave{Ref: ref, Saves: hits}
}

// AmbiguousSave: a save reference matches several saves (13 §3.3), in listing order.
type AmbiguousSave struct {
	Ref   string
	Saves []*Save
}

func (e *AmbiguousSave) Error() string {
	var names []string
	for _, s := range e.Saves {
		names = append(names, s.Ref+" "+s.Colony.Name+"/"+s.Name)
	}
	return fmt.Sprintf("%s matches %d saves: %s", e.Ref, len(e.Saves), strings.Join(names, ", "))
}

// SaveRef is a save's reference: the first 3 bytes of sha1 of its real path, which stays
// the same while the file stays where it is (13 §2).
func SaveRef(path string) string {
	sum := sha1.Sum([]byte(env.PathKey(realOrAbs(path))))
	return "sv:" + hex.EncodeToString(sum[:3])
}

// rimSavesSep separates RimSaves' folder from the save's name: <folder>#§#<save>.rws (12 §4.1).
const rimSavesSep = "#§#"

// SplitSaveName splits a file name into a RimSaves folder ("" for none) and the name shown.
func SplitSaveName(file string) (prefix, name string) {
	name = strings.TrimSuffix(file, filepath.Ext(file))
	if i := strings.LastIndex(name, rimSavesSep); i >= 0 {
		return name[:i], name[i+len(rimSavesSep):]
	}
	return "", name
}

var (
	autosaveRe = regexp.MustCompile(`(?i)^autosave-\d+$`)
	// trailing numbers and separators: "Tundra 2", "Tundra-03", "Tundra_1"
	trailingNumRe = regexp.MustCompile(`[\s_\-.]*\d+$`)
)

// colonyBaseName is a manual save's name with trailing numbers removed (12 §4.1 rule 3).
func colonyBaseName(name string) string {
	base := strings.TrimSpace(trailingNumRe.ReplaceAllString(name, ""))
	if base == "" {
		return name
	}
	return base
}

// ---- heads ----

type saveHeadCache struct {
	mu sync.Mutex
	m  map[string]cachedHead
}

type cachedHead struct {
	file savefile.File
	head *savefile.Head
	err  string
}

// readHeads fills each save's head, size and mtime, reading changed files in parallel.
func (a *App) readHeads(saves []*Save) {
	now := a.Now()
	c := &a.saveHeads
	c.mu.Lock()
	if c.m == nil {
		c.m = map[string]cachedHead{}
	}
	var todo []*Save
	for _, s := range saves {
		fi, err := os.Stat(s.Path)
		if err != nil {
			s.HeadErr = err.Error()
			continue
		}
		s.Size, s.ModTime = fi.Size(), fi.ModTime()
		if h, ok := c.m[s.Path]; ok && h.file.Size == s.Size && h.file.ModTime.Equal(s.ModTime) {
			s.Head, s.HeadErr = h.head, h.err
		} else {
			todo = append(todo, s)
		}
		s.Writing = now.Sub(s.ModTime) < WritingWindow && now.Sub(s.ModTime) > -WritingWindow
	}
	c.mu.Unlock()

	work := make(chan *Save)
	var wg sync.WaitGroup
	for range min(8, len(todo)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := range work {
				h, f, changed, err := savefile.ReadHead(s.Path)
				if err != nil {
					s.HeadErr = err.Error()
				}
				s.Head = h
				if changed {
					s.Writing = true
					continue // read again next time
				}
				if f.Size != 0 || err == nil {
					s.Size, s.ModTime = f.Size, f.ModTime
				}
				c.mu.Lock()
				c.m[s.Path] = cachedHead{file: f, head: h, err: s.HeadErr}
				c.mu.Unlock()
			}
		}()
	}
	for _, s := range todo {
		work <- s
	}
	close(work)
	wg.Wait()
}

// ---- grouping (12 §4.1) ----

// groupColonies groups saves (12 §4.1): a RimSaves folder first; else the world identity
// of the full read (12 §1.7 L1), named after the player faction, else the world; else the
// file name without trailing numbers, where a save not read in full yet joins the world
// colony whose saves have that name. Autosaves without a full read carry no colony in
// their name, so each joins the colony in the same folder whose save has the same Mods
// and is nearest in time; the rest form "Autosaves".
func groupColonies(saves []*Save) []*Colony {
	byKey := map[string]*Colony{}
	var cols []*Colony
	add := func(s *Save, key, name, by string) {
		c := byKey[key]
		if c == nil {
			c = &Colony{Key: key, Name: name, By: by}
			byKey[key] = c
			cols = append(cols, c)
		}
		c.Saves = append(c.Saves, s)
		s.Colony = c
	}
	folderKey := func(s *Save) string { return env.PathKey(s.Folder.Dir) }
	nameKey := func(s *Save) string { return folderKey(s) + "|name:" + strings.ToLower(colonyBaseName(s.Name)) }
	var autos, named []*Save
	worldByName := map[string]*Colony{} // file-name key → the one world colony with saves of that name
	for _, s := range saves {
		switch {
		case s.Prefix != "":
			add(s, folderKey(s)+"|rimsaves:"+strings.ToLower(s.Prefix), s.Prefix, "rimsaves")
		case s.Summary != nil && s.Summary.Identity != "":
			name := s.Summary.Faction
			if name == "" {
				name = s.Summary.World
			}
			add(s, folderKey(s)+"|world:"+s.Summary.Identity, name, "world")
			if !s.Auto {
				k := nameKey(s)
				if c, ok := worldByName[k]; !ok {
					worldByName[k] = s.Colony
				} else if c != s.Colony {
					worldByName[k] = nil // two worlds share the name: do not guess
				}
			}
		case s.Auto:
			autos = append(autos, s)
		default:
			named = append(named, s)
		}
	}
	for _, s := range named {
		if c := worldByName[nameKey(s)]; c != nil {
			add(s, c.Key, c.Name, c.By)
			continue
		}
		add(s, nameKey(s), colonyBaseName(s.Name), "name")
	}
	for _, s := range autos {
		var best *Save
		for _, o := range saves {
			if o.Auto || o.Colony == nil || o.Folder != s.Folder || !sameMods(o, s) {
				continue
			}
			if best == nil || absDur(o.ModTime.Sub(s.ModTime)) < absDur(best.ModTime.Sub(s.ModTime)) {
				best = o
			}
		}
		if best != nil {
			add(s, best.Colony.Key, best.Colony.Name, best.Colony.By)
		} else {
			add(s, folderKey(s)+"|autosaves", "Autosaves", "name")
		}
	}
	for _, c := range cols {
		sort.SliceStable(c.Saves, func(i, j int) bool { return c.Saves[i].ModTime.After(c.Saves[j].ModTime) })
		if c.By == "world" { // named as of its newest read save: the player may rename the faction
			for _, s := range c.Saves {
				if s.Summary != nil && s.Summary.Identity != "" {
					if c.Name = s.Summary.Faction; c.Name == "" {
						c.Name = s.Summary.World
					}
					break
				}
			}
		}
	}
	sort.SliceStable(cols, func(i, j int) bool { return cols[i].Latest().ModTime.After(cols[j].Latest().ModTime) })
	return cols
}

func sameMods(a, b *Save) bool {
	return a.Head != nil && b.Head != nil && slices.Equal(a.Head.ModIDs, b.Head.ModIDs)
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// ---- matching (12 §4.2) ----

type saveMatcher struct {
	a       *App
	cur     *env.Environment
	byFP    map[string]*store.Plan // rule 3: version fingerprint → plan (newest version wins)
	byFPAt  map[string]time.Time
	plans   map[int64]*store.Plan
	envByID map[string]*env.Environment
}

func (a *App) newSaveMatcher() (*saveMatcher, error) {
	m := &saveMatcher{a: a, byFP: map[string]*store.Plan{}, byFPAt: map[string]time.Time{},
		plans: map[int64]*store.Plan{}, envByID: map[string]*env.Environment{}}
	m.cur, _ = a.Env()
	envs, err := a.Store.Envs()
	if err != nil {
		return nil, err
	}
	for _, e := range envs {
		m.envByID[e.ID] = e
	}
	plans, err := a.Store.Plans()
	if err != nil {
		return nil, err
	}
	pkgOf := map[string]string{}
	pkg := func(id string) string {
		if p, ok := pkgOf[id]; ok {
			return p
		}
		p := ""
		if mod, err := a.Store.Mod(id); err == nil && mod != nil {
			p = mod.PackageID
		}
		pkgOf[id] = p
		return p
	}
	for _, p := range plans {
		m.plans[p.ID] = p
		vs, err := a.Store.Versions(p.ID)
		if err != nil {
			return nil, err
		}
		for _, v := range vs {
			var tokens []string
			for _, e := range loadout.Decode(v.Entries) {
				t := e.Missing
				if e.Instance != "" {
					t = pkg(e.Instance)
				}
				tokens = append(tokens, t)
			}
			fp := modsFingerprint(tokens)
			if at, ok := m.byFPAt[fp]; !ok || v.Created.After(at) {
				m.byFP[fp], m.byFPAt[fp] = p, v.Created
			}
		}
	}
	return m, nil
}

// modsFingerprint compares Mod lists the way the game does: packageIds without case and
// without the Workshop copy's _steam suffix (07 §4.1), in order.
func modsFingerprint(tokens []string) string {
	norm := make([]string, 0, len(tokens))
	for _, t := range tokens {
		norm = append(norm, normPkg(t))
	}
	return TokenFingerprint(norm)
}

func normPkg(t string) string { return strings.TrimSuffix(strings.ToLower(t), "_steam") }

func (m *saveMatcher) match(s *Save) {
	f := s.Folder
	s.Env = m.envFor(f, "")
	if f.Pack != "" {
		s.Pack, s.Match = f.Pack, MatchPack
		return
	}
	if len(f.Envs) > 0 && !s.ModTime.IsZero() {
		ids := make([]string, 0, len(f.Envs))
		for _, e := range f.Envs {
			ids = append(ids, e.ID)
		}
		if r, err := m.a.Store.LaunchRecordBefore(ids, s.ModTime); err == nil && r != nil && r.Pack() == "" && m.plans[r.PlanID] != nil {
			s.Plan, s.Match, s.Env = m.plans[r.PlanID], MatchLaunch, m.envFor(f, r.EnvID)
			return
		}
	}
	if s.Head != nil {
		if p := m.byFP[modsFingerprint(s.Head.ModIDs)]; p != nil {
			s.Plan, s.Match = p, MatchMods
		}
	}
}

// envFor is the game a save in f belongs to: the launch's, else the current environment
// when it saves here, else the first that does.
func (m *saveMatcher) envFor(f *SaveFolder, launchEnv string) *env.Environment {
	if e := m.envByID[launchEnv]; e != nil {
		return e
	}
	for _, e := range f.Envs {
		if m.cur != nil && e.ID == m.cur.ID {
			return e
		}
	}
	if len(f.Envs) > 0 {
		return f.Envs[0]
	}
	return nil
}

// inSaveList runs fn with the save's environment and list as the App's current ones, then
// restores them (as envlinks does for one environment).
func (a *App) inSaveList(s *Save, fn func() error) error {
	oldEnv, oldPlan, oldName := a.EnvName, a.OpenPlan, a.PlanName
	defer func() { a.EnvName, a.OpenPlan, a.PlanName = oldEnv, oldPlan, oldName }()
	if s.Env != nil {
		a.EnvName = s.Env.Name
	}
	if s.Plan != nil {
		a.OpenPlan, a.PlanName = s.Plan.ID, ""
	}
	return fn()
}
