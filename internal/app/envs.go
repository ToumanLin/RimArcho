package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// An environment is one game folder (02 §0): its Data/ and Mods/, version, launch, saves and
// plans. The mod folders every game shares (Workshop, SteamCMD, extra, monitor, lock
// library) are managed apart, with FolderAdd / FolderRemove (folders.go).

// Saves values (02 §0 item 5).
const (
	SavesShared = "shared" // the game's default user data folder
	SavesOwn    = "own"    // <data>/userdata/<env>, started with -savedatafolder
	SavesCustom = "custom" // a folder the player chose (--user-data)
)

// EnvSpec is what the player passes to `env add` / `env set`. Empty fields mean "keep" (set)
// or "derive" (add).
type EnvSpec struct {
	GameRoot     string
	UserDataRoot string
	Saves        string // shared | own ("" = keep, or shared when adding without --user-data)
	Launch       string
	// WorkshopRoot, SteamCMD and Extra are the older way to name shared mod folders: each is
	// FolderAdd (kept for scripts and tests).
	WorkshopRoot string
	SteamCMD     []string
	Extra        []string
	// RemoveSources lists shared folder labels to forget (env set only; = FolderRemove).
	RemoveSources []string
}

// SourceChange reports what happened to one source.
type SourceChange struct {
	Source  *source.Source
	Action  string // added | unchanged | rebound | rebind-pending | removed
	OldRoot string
	Rebind  *source.RebindResult
}

// Detected runs environment detection with the app's options.
func (a *App) Detected() env.Detected { return env.Detect(a.Detect) }

func absPath(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	return filepath.Abs(p)
}

func (s *EnvSpec) normalise() error {
	var err error
	if s.GameRoot, err = absPath(s.GameRoot); err != nil {
		return err
	}
	if s.UserDataRoot, err = absPath(s.UserDataRoot); err != nil {
		return err
	}
	if s.WorkshopRoot, err = absPath(s.WorkshopRoot); err != nil {
		return err
	}
	for _, l := range []*[]string{&s.SteamCMD, &s.Extra} {
		for i, p := range *l {
			if (*l)[i], err = absPath(p); err != nil {
				return err
			}
		}
	}
	switch s.Launch {
	case "", "direct", "steam":
	default:
		return fmt.Errorf("--launch must be direct or steam, not %q", s.Launch)
	}
	switch s.Saves {
	case "", SavesShared, SavesOwn:
	default:
		return fmt.Errorf("--saves must be shared or own, not %q", s.Saves)
	}
	if s.Saves != "" && s.UserDataRoot != "" {
		return fmt.Errorf("--saves and --user-data name the same thing; give one")
	}
	return nil
}

// checkRoots enforces the read-only boundary: RimArcho's own data directory must never sit
// inside a game, workshop or mod directory, since everything under those is read-only.
func (a *App) checkRoots(roots ...string) error {
	for _, r := range roots {
		if r != "" && (source.Within(a.Home, r) || source.Within(r, a.Home)) {
			return fmt.Errorf("data directory %s overlaps %s; RimArcho never writes into game or mod folders", a.Home, r)
		}
	}
	return nil
}

// OwnSavesRoot is where environments with their own saves keep them (02 §0 item 5).
func (a *App) OwnSavesRoot() string { return filepath.Join(a.Home, "userdata") }

// ownSaves is an environment's own saves folder.
func (a *App) ownSaves(name string) string { return filepath.Join(a.OwnSavesRoot(), name) }

// DefaultUserData is the game's default user data folder.
func (a *App) DefaultUserData() string { return env.DefaultUserDataFor(a.Detect) }

// Saves tells how an environment keeps its saves: shared, own or custom.
func (a *App) Saves(e *env.Environment) string {
	switch {
	case e.UserDataRoot == "" || env.PathKey(e.UserDataRoot) == env.PathKey(a.DefaultUserData()):
		return SavesShared
	case source.Within(e.UserDataRoot, a.OwnSavesRoot()):
		return SavesOwn
	}
	return SavesCustom
}

// checkUserData is checkRoots for a user data folder: RimArcho's own saves folders are
// inside its data directory on purpose.
func (a *App) checkUserData(p string) error {
	if p == "" || source.Within(p, a.OwnSavesRoot()) {
		return nil
	}
	return a.checkRoots(p)
}

// userDataFor resolves the spec's saves into a user data folder ("" keeps the current one).
func (a *App) userDataFor(name string, spec EnvSpec, adding bool) string {
	switch {
	case spec.UserDataRoot != "":
		return spec.UserDataRoot
	case spec.Saves == SavesOwn:
		return a.ownSaves(name)
	case spec.Saves == SavesShared || adding:
		return a.DefaultUserData()
	}
	return ""
}

func nextLabel(existing []*source.Source, kind source.Kind) string {
	used := map[string]bool{}
	for _, s := range existing {
		used[s.Label] = true
	}
	base := string(kind)
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		if l := base + strconv.Itoa(i); !used[l] {
			return l
		}
	}
}

// ValidEnvName checks an environment name.
func ValidEnvName(name string) error {
	if name == "" || strings.ContainsAny(name, " \t:#/\\") {
		return fmt.Errorf("environment name %q must be non-empty without spaces, ':', '#' or slashes", name)
	}
	return nil
}

// EnvAdd creates an environment for one game folder: its Data/ and Mods/ sources. The
// Workshop folder of the Steam library holding the game becomes a shared folder when there
// is none yet. The first environment becomes current.
func (a *App) EnvAdd(name string, spec EnvSpec) (*env.Environment, []SourceChange, error) {
	if err := ValidEnvName(name); err != nil {
		return nil, nil, err
	}
	if e, err := a.Store.EnvByName(name); err != nil {
		return nil, nil, err
	} else if e != nil {
		return nil, nil, fmt.Errorf("environment %q already exists (archo env set %s ...)", name, name)
	}
	if err := spec.normalise(); err != nil {
		return nil, nil, err
	}
	if spec.GameRoot == "" {
		return nil, nil, fmt.Errorf("--game is required (or --detected)")
	}
	if !env.LooksLikeGameRoot(spec.GameRoot) {
		return nil, nil, fmt.Errorf("%s does not look like a RimWorld install (no executable or Data/Core)", spec.GameRoot)
	}
	if o, err := a.envOfGame(spec.GameRoot); err != nil {
		return nil, nil, err
	} else if o != nil {
		return nil, nil, fmt.Errorf("environment %s already uses %s; one environment per game folder (several lists of one game are plans: archo plan new <name>)", o.Name, spec.GameRoot)
	}
	user := a.userDataFor(name, spec, true)
	if err := a.checkRoots(spec.GameRoot, filepath.Join(spec.GameRoot, "Data"), filepath.Join(spec.GameRoot, "Mods")); err != nil {
		return nil, nil, err
	}
	if err := a.checkUserData(user); err != nil {
		return nil, nil, err
	}
	if err := a.checkFolderRoots(spec); err != nil {
		return nil, nil, err
	}
	e := &env.Environment{ID: a.NewID(), Name: name, GameRoot: spec.GameRoot, UserDataRoot: user,
		GameVersion: env.ReadGameVersion(spec.GameRoot), Launch: spec.Launch, CreatedAt: a.Now().UTC()}
	if e.Launch == "" {
		e.Launch = "direct"
		if env.WorkshopRootFor(spec.GameRoot) != "" {
			e.Launch = "steam"
		}
	}
	if spec.Saves == SavesOwn {
		if err := os.MkdirAll(user, 0o755); err != nil {
			return nil, nil, err
		}
	}
	if err := a.Store.InsertEnv(e); err != nil {
		return nil, nil, err
	}
	srcs, err := a.Store.Sources(e.ID)
	if err != nil {
		return nil, nil, err
	}
	var changes []SourceChange
	for _, sp := range []source.Spec{{Kind: source.Official, Root: filepath.Join(e.GameRoot, "Data")}, {Kind: source.Local, Root: filepath.Join(e.GameRoot, "Mods")}} {
		s := &source.Source{ID: a.NewID(), EnvID: e.ID, Kind: sp.Kind, Label: nextLabel(srcs, sp.Kind), Root: sp.Root, Status: source.StatusOK}
		if !dirExists(sp.Root) {
			s.Status = source.StatusMissing
		}
		if err := a.Store.InsertSource(s); err != nil {
			return nil, nil, err
		}
		srcs = append(srcs, s)
		changes = append(changes, SourceChange{Source: s, Action: "added"})
	}
	more, err := a.foldersFromSpec(e, spec, true, true)
	changes = append(changes, more...)
	if err != nil {
		return e, changes, err
	}
	if cur, _ := a.Store.CurrentEnv(); cur == nil {
		if err := a.Store.SetCurrentEnv(e.ID); err != nil {
			return nil, nil, err
		}
	}
	return e, changes, nil
}

// envOfGame returns the environment that uses a game folder, or nil.
func (a *App) envOfGame(game string) (*env.Environment, error) {
	envs, err := a.Store.Envs()
	if err != nil {
		return nil, err
	}
	for _, e := range envs {
		if env.PathKey(e.GameRoot) == env.PathKey(game) {
			return e, nil
		}
	}
	return nil, nil
}

func (a *App) checkFolderRoots(spec EnvSpec) error {
	roots := append(append([]string{spec.WorkshopRoot}, spec.SteamCMD...), spec.Extra...)
	return a.checkRoots(roots...)
}

// foldersFromSpec applies the spec's shared folders (the older --workshop / --steamcmd /
// --extra flags) and, when adding a game from a Steam library while no Workshop folder is
// known, that library's Workshop folder. A game folder that is new (adding) or moved
// brings its own Workshop folder as a SteamCMD folder (adoptGameWorkshop).
func (a *App) foldersFromSpec(e *env.Environment, spec EnvSpec, adding, moved bool) ([]SourceChange, error) {
	var out []SourceChange
	add := func(kind source.Kind, root string) error {
		ch, err := a.FolderAdd(kind, root)
		if err != nil {
			return err
		}
		out = append(out, ch)
		return nil
	}
	ws := spec.WorkshopRoot
	if ws == "" && adding {
		if w := env.WorkshopRootFor(e.GameRoot); w != "" && dirExists(w) {
			if cur, err := a.workshopFolder(); err != nil {
				return out, err
			} else if cur == nil {
				ws = w
			}
		}
	}
	// A game's own Workshop folder (a SteamCMD download) is a SteamCMD folder, never the
	// Workshop folder: naming it must not move the Steam client's one.
	if ws != "" && env.PathKey(ws) == env.PathKey(env.GameWorkshopFor(e.GameRoot)) {
		spec.SteamCMD, ws = append([]string{ws}, spec.SteamCMD...), ""
	}
	if ws != "" {
		if err := add(source.Workshop, ws); err != nil {
			return out, err
		}
	}
	for _, r := range spec.SteamCMD {
		if err := add(source.SteamCMD, r); err != nil {
			return out, err
		}
	}
	for _, r := range spec.Extra {
		if err := add(source.Extra, r); err != nil {
			return out, err
		}
	}
	if adding || moved {
		ch, err := a.adoptGameWorkshop(e)
		if err != nil {
			return out, err
		}
		if ch != nil {
			out = append(out, *ch)
		}
	}
	return out, nil
}

// EnvSet changes an environment's game folder, saves and launch. Data/ and Mods/ are
// re-bound when a sample of the new folder matches their known instances (02 §3.2);
// otherwise the change waits for `env rebind`. The older --workshop / --steamcmd / --extra /
// --remove-source flags change the shared folders.
func (a *App) EnvSet(name string, spec EnvSpec) (*env.Environment, []SourceChange, error) {
	e, err := a.Store.EnvByName(name)
	if err != nil {
		return nil, nil, err
	}
	if e == nil {
		return nil, nil, fmt.Errorf("no environment named %q (archo env list)", name)
	}
	if err := spec.normalise(); err != nil {
		return nil, nil, err
	}
	gameChanged := spec.GameRoot != "" && env.PathKey(spec.GameRoot) != env.PathKey(e.GameRoot)
	if gameChanged {
		if !env.LooksLikeGameRoot(spec.GameRoot) {
			return nil, nil, fmt.Errorf("%s does not look like a RimWorld install (no executable or Data/Core)", spec.GameRoot)
		}
		if o, err := a.envOfGame(spec.GameRoot); err != nil {
			return nil, nil, err
		} else if o != nil && o.ID != e.ID {
			return nil, nil, fmt.Errorf("environment %s already uses %s; one environment per game folder", o.Name, spec.GameRoot)
		}
		e.GameRoot = spec.GameRoot
		e.GameVersion = env.ReadGameVersion(spec.GameRoot)
	}
	if u := a.userDataFor(e.Name, spec, false); u != "" {
		if err := a.checkUserData(u); err != nil {
			return nil, nil, err
		}
		if spec.Saves == SavesOwn {
			if err := os.MkdirAll(u, 0o755); err != nil {
				return nil, nil, err
			}
		}
		e.UserDataRoot = u
	}
	if spec.Launch != "" {
		e.Launch = spec.Launch
	}
	srcs, err := a.Store.Sources(e.ID)
	if err != nil {
		return nil, nil, err
	}
	byKind := map[source.Kind]*source.Source{}
	for _, s := range srcs {
		if s.EnvID == e.ID && s.Status != source.StatusRetired && (s.Kind == source.Official || s.Kind == source.Local) {
			byKind[s.Kind] = s
		}
	}
	want := map[source.Kind]string{}
	if gameChanged {
		want[source.Official] = filepath.Join(e.GameRoot, "Data")
		want[source.Local] = filepath.Join(e.GameRoot, "Mods")
		if err := a.checkRoots(e.GameRoot, want[source.Official], want[source.Local]); err != nil {
			return nil, nil, err
		}
		// The game moved to another Steam library: its Workshop folder moved with it.
		if ws := env.WorkshopRootFor(e.GameRoot); ws != "" && spec.WorkshopRoot == "" && dirExists(ws) {
			spec.WorkshopRoot = ws
		}
	}
	if err := a.checkFolderRoots(spec); err != nil {
		return nil, nil, err
	}
	var changes []SourceChange
	for _, kind := range []source.Kind{source.Official, source.Local} {
		root, ok := want[kind]
		if !ok {
			continue
		}
		s := byKind[kind]
		if s == nil {
			s = &source.Source{ID: a.NewID(), EnvID: e.ID, Kind: kind, Label: nextLabel(srcs, kind), Root: root, Status: source.StatusOK}
			if err := a.Store.InsertSource(s); err != nil {
				return nil, nil, err
			}
			srcs = append(srcs, s)
			changes = append(changes, SourceChange{Source: s, Action: "added"})
			continue
		}
		if env.PathKey(s.Root) == env.PathKey(root) {
			changes = append(changes, SourceChange{Source: s, Action: "unchanged"})
			continue
		}
		ch, err := a.rebind(s, root)
		if err != nil {
			return nil, nil, err
		}
		changes = append(changes, ch)
	}
	more, err := a.foldersFromSpec(e, spec, false, gameChanged)
	changes = append(changes, more...)
	if err != nil {
		return nil, nil, err
	}
	for _, label := range spec.RemoveSources {
		ch, err := a.FolderRemove(label)
		if err != nil {
			return nil, nil, err
		}
		changes = append(changes, ch)
	}
	if err := a.Store.UpdateEnv(e); err != nil {
		return nil, nil, err
	}
	return e, changes, nil
}

// EnvRemoval is what removing an environment forgets (06 §2.14), for the dry run and the
// result. Only RimArcho's records go: game files, mods and saves stay, and so do the shared
// mod folders with their mods and what the player noted on them.
type EnvRemoval struct {
	Env      *env.Environment
	Plans    int    // its plans (lists, versions, undo steps)
	Launches int    // its launch records
	Mods     int    // instances RimArcho read from its Data/ and Mods/
	Current  bool   // it is the current environment
	Next     string // the environment that becomes current ("" none left: the guide)
}

// EnvRemovePreview is the dry run of EnvRemove: nothing changes.
func (a *App) EnvRemovePreview(name string) (*EnvRemoval, error) {
	e, err := a.Store.EnvByName(name)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("no environment named %q (archo env list)", name)
	}
	r := &EnvRemoval{Env: e}
	db := a.Store.DB
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM plan WHERE environment_id=?), (SELECT count(*) FROM launch WHERE environment_id=?),
		(SELECT count(*) FROM instance WHERE environment_id=? AND status='live')`, e.ID, e.ID, e.ID).Scan(&r.Plans, &r.Launches, &r.Mods); err != nil {
		return nil, err
	}
	if cur, _ := a.Store.CurrentEnv(); cur != nil && cur.ID == e.ID {
		r.Current = true
		envs, err := a.Store.Envs()
		if err != nil {
			return nil, err
		}
		for _, o := range envs {
			if o.ID != e.ID {
				r.Next = o.Name
				break
			}
		}
	}
	return r, nil
}

// EnvRemove deletes an environment from RimArcho's database: its Data/ and Mods/ instances,
// plans, launches and history. The shared mod folders and the player's data on mods stay.
// Game and mod files are not touched. It is refused while that game runs. Removing the
// current environment makes another one current (EnvRemoval.Next), or none.
func (a *App) EnvRemove(name string) (*EnvRemoval, error) {
	r, err := a.EnvRemovePreview(name)
	if err != nil {
		return nil, err
	}
	if g := a.probe()(r.Env.GameRoot); g != nil {
		return nil, fmt.Errorf("env:%s's game is running (pid %d); close it before removing the environment", r.Env.Name, g.PID)
	}
	if err := a.Store.DeleteEnv(r.Env.ID); err != nil {
		return nil, err
	}
	if r.Next != "" {
		if n, err := a.Store.EnvByName(r.Next); err == nil && n != nil {
			if err := a.Store.SetCurrentEnv(n.ID); err != nil {
				return nil, err
			}
		}
	}
	return r, nil
}

// rebind evaluates moving source s to newRoot and applies it when the sample matches.
func (a *App) rebind(s *source.Source, newRoot string) (SourceChange, error) {
	old, err := a.Store.SourceSemantics(s.ID)
	if err != nil {
		return SourceChange{}, err
	}
	r := source.EvaluateRebind(newRoot, old, scan.SemanticOnly, scan.Similarity)
	ch := SourceChange{Source: s, OldRoot: s.Root, Rebind: &r}
	if r.Accept {
		s.Root, s.Status, s.PendingRoot = newRoot, source.StatusOK, ""
		ch.Action = "rebound"
	} else {
		s.Status, s.PendingRoot = source.StatusRebindPending, newRoot
		ch.Action = "rebind-pending"
	}
	return ch, a.Store.UpdateSource(s)
}

// EnvRebind answers a pending re-binding. yes: the new root is the same source. no: the old
// source is retired (its instances become tombstones on the next scan) and a new source is
// created at the new root. The source may be the environment's or a shared folder.
func (a *App) EnvRebind(e *env.Environment, label string, yes bool) (*source.Source, *source.Source, error) {
	srcs, err := a.Store.Sources(e.ID)
	if err != nil {
		return nil, nil, err
	}
	label = strings.TrimPrefix(label, "src:")
	var s *source.Source
	for _, x := range srcs {
		if x.Label == label {
			s = x
		}
	}
	if s == nil {
		return nil, nil, fmt.Errorf("no source src:%s in env:%s", label, e.Name)
	}
	if s.Status != source.StatusRebindPending {
		return nil, nil, fmt.Errorf("src:%s has no pending re-binding (status %s)", label, s.Status)
	}
	if yes {
		s.Root, s.Status, s.PendingRoot = s.PendingRoot, source.StatusOK, ""
		return s, nil, a.Store.UpdateSource(s)
	}
	if s.EnvID == store.Shared {
		if srcs, err = a.Store.AllSources(); err != nil {
			return nil, nil, err
		}
	}
	newRoot := s.PendingRoot
	s.Status, s.PendingRoot = source.StatusRetired, ""
	oldLabel := s.Label
	s.Label = nextLabel(srcs, source.Kind(oldLabel+"-old"))
	if err := a.Store.UpdateSource(s); err != nil {
		return nil, nil, err
	}
	n := &source.Source{ID: a.NewID(), EnvID: s.EnvID, Kind: s.Kind, Label: oldLabel, Root: newRoot, Status: source.StatusOK}
	return s, n, a.Store.InsertSource(n)
}

// EnvUse prepares missing Mods/ links before selecting the current environment.
func (a *App) EnvUse(name string) (*env.Environment, error) {
	e, err := a.Store.EnvByName(name)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("no environment named %q (archo env list)", name)
	}
	if err := a.prepareEnvLinks(e); err != nil {
		return nil, err
	}
	return e, a.Store.SetCurrentEnv(e.ID)
}
