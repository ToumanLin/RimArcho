package gui

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The window's shell (06 §2.1–§2.3, §2.15): the sidebar, the home page and the first-run
// guide. Every method only calls view.

// Shell is the sidebar and the breadcrumb of every page in it.
type Shell struct {
	Nav    []view.NavGroup         `json:"nav"`
	Crumbs map[string][]view.Crumb `json:"crumbs"`
	Env    string                  `json:"env"` // the environment chip
	Envs   int                     `json:"envs"`
}

// Shell builds the sidebar from the needs count and the packs.
func (s *API) Shell() *Shell {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := &Shell{Crumbs: map[string][]view.Crumb{}}
	if envs, err := s.a.Store.Envs(); err == nil {
		out.Envs = len(envs)
	}
	count := 0
	if st := s.boardState(); st != nil {
		count = s.needsLocked(false).Count
		out.Env = view.EnvChip(s.board.EnvName, s.board.GameVersion)
	} else {
		out.Env = view.EnvChip("", "")
	}
	packs, _, _ := view.PackRows(s.a)
	out.Nav = view.Nav(count, packs)
	ids := []string{view.PageSettings}
	for _, g := range out.Nav {
		for _, it := range g.Items {
			ids = append(ids, it.ID)
			for _, k := range it.Kids {
				ids = append(ids, k.ID)
			}
		}
	}
	for _, id := range ids {
		out.Crumbs[id] = view.NavPath(out.Nav, id)
	}
	return out
}

// Home is the home page (06 §2.3).
func (s *API) Home() *view.Home {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.boardState() == nil {
		return &view.Home{}
	}
	packs, _, _ := view.PackRows(s.a)
	return view.BuildHome(s.a, s.board, s.needsLocked(false), packs)
}

// ---- the first-run guide (06 §2.15) ----

// GuideFind lists the RimWorld installs detection finds.
func (s *API) GuideFind() []view.GameFound {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.FindGames(s.a)
}

// GuideGame checks a game folder the player typed.
func (s *API) GuideGame(path string) (*view.GameFound, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.GameAt(s.a, path)
}

// GuideAdd creates the chosen environments; the first becomes current.
func (s *API) GuideAdd(games []view.GameFound) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	names, err := view.AddGames(s.a, games)
	s.board, s.needs = nil, nil
	if err != nil {
		return names, errors.New(view.Humanize(nil, err.Error()))
	}
	return names, nil
}

// GuideScan scans the environments one after another in child processes and returns at
// once; "guide" events carry which one is being read and the end (06 §8.4).
func (s *API) GuideScan(names []string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy["scan"] || len(names) == 0 {
		return false
	}
	s.busy["scan"] = true
	run := s.ScanRunner
	if run == nil {
		home, exe := s.a.Home, s.Exe
		run = func(env string) (string, error) {
			if exe != "" {
				return app.ChildScanExe(exe, home, env)
			}
			return app.ChildScan(home, env)
		}
	}
	go func() {
		ok := true
		for i, n := range names {
			s.send("guide", map[string]any{"i": i, "n": len(names), "env": n, "text": view.Tf("reading the mods of %s…", n)})
			out, err := run(n)
			ev := map[string]any{"i": i, "n": len(names), "env": n, "ok": err == nil, "step": true}
			if err != nil {
				ok = false
				ev["error"] = view.Humanize(nil, firstLine(out)+" "+err.Error())
			} else {
				ev["text"] = fmt.Sprintf("%s: %s", n, scanLine(out))
			}
			s.send("guide", ev)
		}
		s.mu.Lock()
		s.busy["scan"], s.board, s.needs = false, nil, nil
		s.mu.Unlock()
		s.send("guide", map[string]any{"done": true, "ok": ok})
	}()
	return true
}

var liveRe = regexp.MustCompile(`(\d+) live`)

// scanLine is the scan's "instances" line in words: how many mods it found.
func scanLine(out string) string {
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if m := liveRe.FindStringSubmatch(l); m != nil && strings.HasPrefix(l, "instances") {
			n, _ := strconv.Atoi(m[1])
			return view.Tf("%d mods found", n)
		}
	}
	return view.T("scan complete")
}

// PackImport reads a pack file (pack import, 08 §5.4): a full pack goes to the packs
// folder; a light pack's list becomes the current list (one undo step).
func (s *API) PackImport(path string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path = strings.Trim(strings.TrimSpace(path), `"'`)
	m, err := s.a.ImportPack(path, true)
	s.board, s.needs = nil, nil
	if err != nil {
		return "", errors.New(view.Humanize(nil, err.Error()))
	}
	return view.PackImportText(m), nil
}
