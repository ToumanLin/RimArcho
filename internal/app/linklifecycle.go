package app

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// IncomingLinks uses the same fresh observations and ownership evidence as repair.
// Foreign links are included so a destructive target operation can refuse them.
func (a *App) IncomingLinks(target string) ([]*GameLink, error) {
	return a.affectedGameLinks(target, false)
}

func (a *App) IncomingInstanceLinks(id string) ([]*GameLink, error) {
	m, err := a.Store.Mod(id)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("unknown instance: %s", id)
	}
	return a.IncomingLinks(m.RealPath)
}

// incomingLinkCheck is a fresh observation for one operation, never retained across requests.
type incomingLinkCheck struct {
	roots   []gameMods
	links   []*GameLink
	targets map[*GameLink]string
	probe   func(string) *GameRun
}

func (a *App) affectedGameLinks(target string, locations bool) ([]*GameLink, error) {
	check, err := a.readIncomingLinks()
	if err != nil {
		return nil, err
	}
	return check.affected(target, locations), nil
}

func (a *App) readIncomingLinks() (*incomingLinkCheck, error) {
	managed, err := a.InspectGameLinks()
	if err != nil {
		return nil, err
	}
	byPath := map[string]*GameLink{}
	for _, l := range managed {
		if l.Code == CodeLinkRecovery {
			return nil, fmt.Errorf("unfinished link operation: %s; review archo links repair --all --dry-run first", l.Path)
		}
		byPath[env.PathKey(l.Path)] = l
	}
	roots, err := a.knownGameMods()
	if err != nil {
		return nil, err
	}
	var out []*GameLink
	for _, root := range roots {
		entries, err := os.ReadDir(root.mods)
		if os.IsNotExist(err) && missingTarget(root.mods) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("cannot check incoming links in %s: %w", root.mods, err)
		}
		for _, entry := range entries {
			path := filepath.Join(root.mods, entry.Name())
			l := byPath[env.PathKey(path)]
			if l == nil {
				info, err := source.ReadLink(path)
				if err != nil {
					return nil, err
				}
				if info.Kind == "plain" {
					continue
				}
				l = &GameLink{Game: root.game, Mods: root.mods, Path: path, Target: info.RawTarget, Kind: info.Kind, State: info.State, snapshot: info, Code: CodeLinkForeign, Reason: "link belongs to another tool or the player", Evidence: "no RimArcho prefix"}
			}
			if l.Kind == "plain" {
				continue
			}
			if l.snapshot == nil || l.Kind != "junction" && l.Kind != "symlink" || l.State == source.TargetUnknown {
				return nil, fmt.Errorf("cannot rule out an incoming link at %s: %s; leave it alone and resolve the check failure first", l.Path, l.Reason)
			}
			out = append(out, l)
		}
	}
	targets := make(map[*GameLink]string, len(out))
	for _, l := range out {
		targets[l] = physicalLinkPath(l.snapshot.Target)
	}
	return &incomingLinkCheck{roots: roots, links: out, targets: targets, probe: a.probe()}, nil
}

func (c *incomingLinkCheck) affected(target string, locations bool) []*GameLink {
	target = physicalLinkPath(target)
	var out []*GameLink
	for _, l := range c.links {
		if source.Within(c.targets[l], target) || locations && source.Within(l.Path, target) {
			out = append(out, l)
		}
	}
	return out
}

func missingTarget(path string) bool {
	state, _ := source.ProbeTarget(path)
	return state == source.TargetMissing
}

type LinkTransition struct {
	Game string `json:"game"`
	store.ManagedLink
	snapshot *source.LinkInfo
	detached bool
}

type LinkLifecyclePlan struct {
	Links     []*LinkTransition `json:"links"`
	From      string            `json:"from"`
	To        string            `json:"to"`
	Directory string            `json:"directory,omitempty"`
	Games     []string          `json:"games"`
	operation string
	unlocks   []func()
	parts     []*LinkLifecyclePlan
}

func joinLinkPlans(parts []*LinkLifecyclePlan) *LinkLifecyclePlan {
	p := &LinkLifecyclePlan{parts: parts}
	seen := map[string]bool{}
	for _, part := range parts {
		p.Games = append(p.Games, part.Games...)
		for _, l := range part.Links {
			key := env.PathKey(filepath.Join(l.Mods, l.Name))
			if seen[key] {
				continue
			}
			seen[key] = true
			l.From, l.To, l.Directory = part.From, part.To, part.Directory
			p.Links = append(p.Links, l)
		}
	}
	return p
}

func rebaseLinkPath(path, from, to string) string {
	if to != "" && source.Within(path, from) {
		rel, err := filepath.Rel(from, path)
		if err == nil {
			return filepath.Join(to, rel)
		}
	}
	return path
}

// PlanIncomingLinks is read-only. An empty destination means detach permanently.
// Directory moves also capture managed links located inside the moved tree.
func (a *App) PlanIncomingLinks(from, to string) (*LinkLifecyclePlan, error) {
	check, err := a.readIncomingLinks()
	if err != nil {
		return nil, err
	}
	return a.planIncomingLinks(from, to, check)
}

func (a *App) planIncomingLinks(from, to string, check *incomingLinkCheck) (*LinkLifecyclePlan, error) {
	from = physicalLinkPath(from)
	if to != "" {
		to = physicalLinkPath(to)
	}
	roots := check.roots
	var games []string
	for _, root := range roots {
		if source.Within(from, root.mods) || source.Within(root.mods, from) {
			games = append(games, root.game)
		}
		if (source.Within(from, root.mods) || source.Within(root.mods, from)) && check.probe(root.game) != nil {
			return nil, fmt.Errorf("affected game is running: %s", root.game)
		}
	}
	links := check.affected(from, to != "")
	p := &LinkLifecyclePlan{From: from, To: to, Games: games}
	unique := map[string]*LinkTransition{}
	for _, l := range links {
		if !l.Owned || l.Code == CodeLinkForeign || !managedLink(filepath.Base(l.Path)) && !packLink(filepath.Base(l.Path)) {
			return nil, fmt.Errorf("moving or removing %s would break foreign link %s -> %s (%s); detach it yourself first", from, l.Path, l.Target, l.Evidence)
		}
		if check.probe(l.Game) != nil {
			return nil, fmt.Errorf("affected game is running: %s", l.Game)
		}
		p.Games = append(p.Games, l.Game)
		nextPath, nextTarget := "", ""
		if to != "" {
			nextPath = rebaseLinkPath(l.Path, from, to)
			nextTarget = rebaseLinkPath(physicalLinkPath(l.snapshot.Target), from, to)
		}
		t := &LinkTransition{Game: l.Game, ManagedLink: store.ManagedLink{Mods: l.Mods, Name: filepath.Base(l.Path), Kind: l.Kind, Instance: l.Instance, Pack: l.Pack, Target: l.snapshot.Target, NextPath: nextPath, NextTarget: nextTarget}, snapshot: l.snapshot}
		// Multiple aliases of one instance in a game must become just one live entry.
		key := env.PathKey(l.Mods) + "|" + env.PathKey(physicalLinkPath(l.snapshot.Target))
		if prev := unique[key]; prev != nil && to != "" {
			if envLink(prev.Name) && !envLink(t.Name) {
				prev.NextPath, prev.NextTarget = "", ""
				unique[key] = t
			} else {
				t.NextPath, t.NextTarget = "", ""
			}
		} else {
			unique[key] = t
		}
		p.Links = append(p.Links, t)
	}
	return p, nil
}

func (p *LinkLifecyclePlan) ensureEntry(game, mods, name, target, instance string) {
	mods = realOrAbs(mods)
	for _, l := range p.Links {
		if env.PathKey(l.Mods) == env.PathKey(mods) && l.NextTarget != "" {
			return
		}
	}
	kind := "junction"
	if runtime.GOOS != "windows" {
		kind = "symlink"
	}
	p.Links = append(p.Links, &LinkTransition{Game: realOrAbs(game), ManagedLink: store.ManagedLink{Mods: mods, Name: name, Kind: kind, Instance: instance, NextPath: filepath.Join(mods, name), NextTarget: target}})
}

// Normalize surviving ancestors too: a dangling target can retain Windows 8.3
// components after its leaf has moved, when resolving the whole path cannot work.
func physicalLinkPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	for parent := abs; ; parent = filepath.Dir(parent) {
		if real, err := source.RealPath(parent); err == nil {
			rel, err := filepath.Rel(parent, abs)
			if err == nil {
				return filepath.Clean(filepath.Join(real, rel))
			}
		}
		if filepath.Dir(parent) == parent {
			return filepath.Clean(abs)
		}
	}
}

func (p *LinkLifecyclePlan) release() {
	for i := len(p.unlocks) - 1; i >= 0; i-- {
		p.unlocks[i]()
	}
	p.unlocks = nil
}

func (a *App) beginLinkLifecycle(p *LinkLifecyclePlan) error {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	p.operation = hex.EncodeToString(id[:])
	sort.Slice(p.Links, func(i, j int) bool { return p.Links[i].Mods+p.Links[i].Name < p.Links[j].Mods+p.Links[j].Name })
	locked := map[string]bool{}
	for _, l := range p.Links {
		if !locked[l.Mods] {
			unlock, err := lockGameLinks(l.Mods)
			if err != nil {
				p.release()
				return err
			}
			p.unlocks = append(p.unlocks, unlock)
			locked[l.Mods] = true
		}
	}
	for _, game := range p.Games {
		if a.probe()(game) != nil {
			p.release()
			return fmt.Errorf("affected game is running: %s", game)
		}
	}
	// Freeze the whole incoming set under the game locks, including additions
	// since the preview. A new foreign entry cancels before any detach.
	parts := p.parts
	if parts == nil {
		parts = []*LinkLifecyclePlan{p}
	}
	observed := map[string]*source.LinkInfo{}
	for _, part := range parts {
		fresh, err := a.PlanIncomingLinks(part.From, part.To)
		if err != nil {
			p.release()
			return err
		}
		for _, l := range fresh.Links {
			observed[env.PathKey(l.snapshot.Path)] = l.snapshot
		}
	}
	expected := map[string]*source.LinkInfo{}
	for _, l := range p.Links {
		if l.snapshot != nil {
			expected[env.PathKey(l.snapshot.Path)] = l.snapshot
		}
	}
	if len(expected) != len(observed) {
		p.release()
		return fmt.Errorf("incoming links changed since preview")
	}
	for path, snapshot := range observed {
		if !source.SameLink(expected[path], snapshot) {
			p.release()
			return fmt.Errorf("incoming link changed since preview: %s", path)
		}
	}
	for _, l := range p.Links {
		if a.probe()(l.Game) != nil {
			p.release()
			return fmt.Errorf("affected game is running: %s", l.Game)
		}
		if l.snapshot != nil {
			if err := source.VerifyLink(l.snapshot); err != nil {
				p.release()
				return err
			}
		} else if _, err := os.Lstat(l.NextPath); !os.IsNotExist(err) {
			p.release()
			return fmt.Errorf("new link path is occupied or inaccessible: %s", l.NextPath)
		}
	}
	rows := make([]store.ManagedLink, 0, len(p.Links))
	for _, l := range p.Links {
		l.Operation = p.operation
		if l.From == "" {
			l.From, l.To, l.Directory = p.From, p.To, p.Directory
		}
		rows = append(rows, l.ManagedLink)
	}
	if err := a.Store.RecordLinkIntent(rows); err != nil {
		p.release()
		return err
	}
	for _, l := range p.Links {
		if l.snapshot != nil {
			if a.probe()(l.Game) != nil {
				return a.abortLinkLifecycle(p, fmt.Errorf("affected game is running: %s", l.Game))
			}
			if err := source.RemoveLinkChecked(l.snapshot); err != nil {
				return a.abortLinkLifecycle(p, err)
			}
			l.detached = true
		}
	}
	if err := a.Store.LinkPhase(p.operation, "detached"); err != nil {
		return a.abortLinkLifecycle(p, err)
	}
	a.modsCopies = nil
	return nil
}

// abort never re-creates a link into a missing/unknown target. Unresolved intent
// stays in the ledger for a read-only recovery finding on the next startup.
func (a *App) abortLinkLifecycle(p *LinkLifecyclePlan, cause error) error {
	defer p.release()
	for _, l := range p.Links {
		if !l.detached {
			if l.snapshot == nil && l.From != "" && missingTarget(l.From) {
				continue
			}
			cause = errors.Join(cause, a.Store.LinkEntryPhase(p.operation, l.Mods, l.Name, "rolled-back"))
			continue
		}
		state, _ := source.ProbeTarget(l.Target)
		if state != source.TargetPresent || a.probe()(l.Game) != nil {
			cause = errors.Join(cause, fmt.Errorf("link recovery remains pending: %s", filepath.Join(l.Mods, l.Name)))
			continue
		}
		if err := CreateLink(filepath.Join(l.Mods, l.Name), l.Target); err != nil {
			cause = errors.Join(cause, err)
			continue
		}
		cause = errors.Join(cause, a.Store.LinkEntryPhase(p.operation, l.Mods, l.Name, "rolled-back"))
	}
	return cause
}

func (a *App) finishLinkLifecycle(p *LinkLifecyclePlan) error {
	defer p.release()
	if err := a.Store.LinkPhase(p.operation, "records-updated"); err != nil {
		return err
	}
	for _, l := range p.Links {
		if l.NextTarget == "" {
			continue
		}
		game := filepath.Dir(filepath.Dir(l.NextPath))
		if a.probe()(game) != nil {
			return fmt.Errorf("affected game is running: %s", game)
		}
		state, reason := source.ProbeTarget(l.NextTarget)
		if state != source.TargetPresent {
			return fmt.Errorf("cannot reattach %s: %s", l.NextPath, reason)
		}
		if err := CreateLink(l.NextPath, l.NextTarget); err != nil {
			return err
		}
	}
	a.modsCopies = nil
	return a.Store.LinkPhase(p.operation, "complete")
}

// withIncomingLinks brackets a target replacement/removal; update must include any
// associated database change and runs only after all incoming links are detached.
func (a *App) withIncomingLinks(from, to string, update func() error) error {
	p, err := a.PlanIncomingLinks(from, to)
	if err != nil {
		return err
	}
	if err = a.beginLinkLifecycle(p); err != nil {
		return err
	}
	if err = update(); err != nil {
		return a.abortLinkLifecycle(p, err)
	}
	return a.finishLinkLifecycle(p)
}
