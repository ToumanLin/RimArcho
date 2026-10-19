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
)

// EnvSpec is what the player passes to `env add` / `env set`. Empty fields mean "keep" (set)
// or "derive" (add).
type EnvSpec struct {
	GameRoot     string
	UserDataRoot string
	WorkshopRoot string
	Launch       string
	SteamCMD     []string
	Extra        []string
	// RemoveSources lists steamcmd/extra source labels to retire (env set only).
	RemoveSources []string
}

// SourceChange reports what happened to one source.
type SourceChange struct {
	Source  *source.Source
	Action  string // added | unchanged | rebound | rebind-pending
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

// EnvAdd creates an environment and its sources. The first environment becomes current.
func (a *App) EnvAdd(name string, spec EnvSpec) (*env.Environment, []SourceChange, error) {
	if name == "" || strings.ContainsAny(name, " \t:#") {
		return nil, nil, fmt.Errorf("environment name %q must be non-empty without spaces, ':' or '#'", name)
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
	specs := source.Derive(spec.GameRoot, spec.WorkshopRoot, spec.SteamCMD, spec.Extra)
	roots := []string{spec.GameRoot, spec.UserDataRoot}
	for _, sp := range specs {
		roots = append(roots, sp.Root)
	}
	if err := a.checkRoots(roots...); err != nil {
		return nil, nil, err
	}
	e := &env.Environment{ID: a.NewID(), Name: name, GameRoot: spec.GameRoot, UserDataRoot: spec.UserDataRoot,
		GameVersion: env.ReadGameVersion(spec.GameRoot), Launch: spec.Launch, CreatedAt: a.Now().UTC()}
	if e.Launch == "" {
		e.Launch = "direct"
		for _, sp := range specs {
			if sp.Kind == source.Workshop {
				e.Launch = "steam"
			}
		}
	}
	if err := a.Store.InsertEnv(e); err != nil {
		return nil, nil, err
	}
	var srcs []*source.Source
	var changes []SourceChange
	for _, sp := range specs {
		s := &source.Source{ID: a.NewID(), EnvID: e.ID, Kind: sp.Kind, Label: nextLabel(srcs, sp.Kind), Root: sp.Root, Status: source.StatusOK}
		if st, err := os.Stat(sp.Root); err != nil || !st.IsDir() {
			s.Status = source.StatusMissing
		}
		if err := a.Store.InsertSource(s); err != nil {
			return nil, nil, err
		}
		srcs = append(srcs, s)
		changes = append(changes, SourceChange{Source: s, Action: "added"})
	}
	if cur, _ := a.Store.CurrentEnv(); cur == nil {
		if err := a.Store.SetCurrentEnv(e.ID); err != nil {
			return nil, nil, err
		}
	}
	return e, changes, nil
}

// EnvSet changes an environment's paths. A source whose root changes is re-bound when a
// sample of the new root matches its known instances (02 §3.2); otherwise the change waits
// for `env rebind`.
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
	if spec.GameRoot != "" {
		if !env.LooksLikeGameRoot(spec.GameRoot) {
			return nil, nil, fmt.Errorf("%s does not look like a RimWorld install (no executable or Data/Core)", spec.GameRoot)
		}
		e.GameRoot = spec.GameRoot
		e.GameVersion = env.ReadGameVersion(spec.GameRoot)
	}
	if spec.UserDataRoot != "" {
		e.UserDataRoot = spec.UserDataRoot
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
		if s.Status != source.StatusRetired && (s.Kind == source.Official || s.Kind == source.Local || s.Kind == source.Workshop) {
			byKind[s.Kind] = s
		}
	}
	want := map[source.Kind]string{}
	if gameChanged {
		want[source.Official] = filepath.Join(e.GameRoot, "Data")
		want[source.Local] = filepath.Join(e.GameRoot, "Mods")
		if ws := env.WorkshopRootFor(e.GameRoot); ws != "" && spec.WorkshopRoot == "" {
			if st, err := os.Stat(ws); err == nil && st.IsDir() {
				want[source.Workshop] = ws
			}
		}
	}
	if spec.WorkshopRoot != "" {
		want[source.Workshop] = spec.WorkshopRoot
	}
	roots := []string{e.GameRoot, e.UserDataRoot}
	for _, r := range want {
		roots = append(roots, r)
	}
	roots = append(append(roots, spec.SteamCMD...), spec.Extra...)
	if err := a.checkRoots(roots...); err != nil {
		return nil, nil, err
	}
	var changes []SourceChange
	for _, kind := range []source.Kind{source.Official, source.Local, source.Workshop} {
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
	for _, pair := range []struct {
		kind  source.Kind
		roots []string
	}{{source.SteamCMD, spec.SteamCMD}, {source.Extra, spec.Extra}} {
		for _, r := range pair.roots {
			dup := false
			for _, s := range srcs {
				if env.PathKey(s.Root) == env.PathKey(r) && s.Status != source.StatusRetired {
					dup = true
				}
			}
			if dup {
				continue
			}
			s := &source.Source{ID: a.NewID(), EnvID: e.ID, Kind: pair.kind, Label: nextLabel(srcs, pair.kind), Root: r, Status: source.StatusOK}
			if err := a.Store.InsertSource(s); err != nil {
				return nil, nil, err
			}
			srcs = append(srcs, s)
			changes = append(changes, SourceChange{Source: s, Action: "added"})
		}
	}
	for _, label := range spec.RemoveSources {
		label = strings.TrimPrefix(label, "src:")
		var s *source.Source
		for _, x := range srcs {
			if x.Label == label && x.Status != source.StatusRetired {
				s = x
			}
		}
		if s == nil {
			return nil, nil, fmt.Errorf("no active source src:%s in env:%s", label, e.Name)
		}
		if s.Kind != source.SteamCMD && s.Kind != source.Extra {
			return nil, nil, fmt.Errorf("src:%s is a %s source; only steamcmd and extra sources can be removed (change --game/--workshop instead)", label, s.Kind)
		}
		s.Status = source.StatusRetired
		s.Label = nextLabel(srcs, source.Kind(label+"-removed"))
		if err := a.Store.UpdateSource(s); err != nil {
			return nil, nil, err
		}
		changes = append(changes, SourceChange{Source: s, Action: "removed"})
	}
	if err := a.Store.UpdateEnv(e); err != nil {
		return nil, nil, err
	}
	return e, changes, nil
}

// EnvRemove deletes an environment from RimArcho's database (instances, history, rules).
// Game and mod files are not touched.
func (a *App) EnvRemove(name string) (*env.Environment, error) {
	e, err := a.Store.EnvByName(name)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("no environment named %q (archo env list)", name)
	}
	return e, a.Store.DeleteEnv(e.ID)
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
// created at the new root.
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
	newRoot := s.PendingRoot
	s.Status, s.PendingRoot = source.StatusRetired, ""
	oldLabel := s.Label
	s.Label = nextLabel(srcs, source.Kind(oldLabel+"-old"))
	if err := a.Store.UpdateSource(s); err != nil {
		return nil, nil, err
	}
	n := &source.Source{ID: a.NewID(), EnvID: e.ID, Kind: s.Kind, Label: oldLabel, Root: newRoot, Status: source.StatusOK}
	return s, n, a.Store.InsertSource(n)
}

// EnvUse selects the current environment.
func (a *App) EnvUse(name string) (*env.Environment, error) {
	e, err := a.Store.EnvByName(name)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("no environment named %q (archo env list)", name)
	}
	return e, a.Store.SetCurrentEnv(e.ID)
}
