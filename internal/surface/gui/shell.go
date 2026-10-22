package gui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	// Plans are the environment's plans; Plan is the current one (the editor's), InGame
	// what the game's ModsConfig.xml holds (the status line, 04 §1.3).
	Plans  []view.PlanRow    `json:"plans"`
	Plan   *view.PlanRow     `json:"plan,omitempty"`
	InGame *view.InGameState `json:"inGame,omitempty"`
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
	if out.Envs > 0 {
		out.Plans, _ = view.PlanRows(s.a)
		out.InGame = view.InGame(s.a)
	}
	for i := range out.Plans {
		if out.Plans[i].Current {
			out.Plan = &out.Plans[i]
		}
	}
	out.Nav = view.Nav(count, out.Plans, packs)
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
	st := s.boardState()
	if st == nil {
		return &view.Home{}
	}
	// The last run's line reads Player.log when it is newer than the one read last (06
	// §2.3): the page loads, or the window comes back into focus, after a game.
	if _, newer := s.a.NewLog(st); newer {
		s.needsLocked(true)
	}
	packs, _, _ := view.PackRows(s.a)
	plans, _ := view.PlanRows(s.a)
	h := view.BuildHome(s.a, s.board, s.needsLocked(false), plans, packs)
	st = s.board.St
	view.AddOtherEnvs(s.a, h, st.Ix.Env.ID)
	view.PickFeatured(h, s.a.Store.Meta(view.FeaturedKey), st.Ix.Env.Name, st.Plan.ID)
	return h
}

// HomeApply is a home row's 应用 (06 §2.3): its environment becomes current if it is not,
// then a plan becomes the current plan, a pack the featured item. No game file is written;
// ▶ 启动游戏 on the featured card does that. The page reloads the board afterwards.
func (s *API) HomeApply(kind, envName string, plan int64, pack string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.boardState()
	if st == nil || st.Ix.Env.Name != envName {
		s.a.EnvName = ""
		if _, err := s.a.EnvUse(envName); err != nil {
			return "", err
		}
		s.needs, s.board = nil, nil
	}
	if kind == view.RowPlan {
		_, t, err := view.UsePlan(s.a, plan)
		if err != nil {
			return "", err
		}
		s.board = nil
		return t, s.a.Store.SetMeta(view.FeaturedKey, "")
	}
	st = s.boardState()
	if st == nil {
		return "", fmt.Errorf("no environment")
	}
	return view.Tf("%s is ready to launch", pack), s.a.Store.SetMeta(view.FeaturedKey, view.FeaturedValue(envName, pack, st.Plan.ID))
}

// ---- plans (04 §1.3, 06 §2.5) ----

// PlanFroms are the new-plan dialog's starting lists.
func (s *API) PlanFroms() ([]view.PlanFrom, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.PlanFroms(s.a)
}

// PlanName suggests a free name ("" base: "new plan") and checks one being typed: Problem
// says what is wrong with name ("" fine; self is the plan being renamed).
type PlanName struct {
	Free    string `json:"free"`
	Problem string `json:"problem"`
}

// CheckPlanName checks a name the player types.
func (s *API) CheckPlanName(base, name string, self int64) *PlanName {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &PlanName{Free: view.FreePlanName(s.a, base), Problem: view.CheckPlanName(s.a, name, self)}
}

// planDone is a managing verb's result: the text and the page to show next.
type PlanDone struct {
	Text string `json:"text"`
	Page string `json:"page"`
}

func (s *API) planDone(p int64, text string, err error) (*PlanDone, error) {
	s.board, s.needs = nil, nil
	if err != nil {
		return nil, err
	}
	d := &PlanDone{Text: text, Page: view.PagePlan}
	if p != 0 {
		d.Page = view.PlanPage(p)
	}
	return d, nil
}

// NewPlan creates a plan (from "" empty, "current", "v<n>") and switches to it.
func (s *API) NewPlan(name, from string) (*PlanDone, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, t, err := view.NewPlan(s.a, name, from)
	if p == nil {
		return s.planDone(0, t, err)
	}
	return s.planDone(p.ID, t, err)
}

// ImportAsPlan makes the import being previewed a new plan and switches to it.
func (s *API) ImportAsPlan(name string) (*PlanDone, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.x.imp == nil {
		return nil, errors.New(view.T("nothing to import"))
	}
	p, t, err := view.ImportAsPlan(s.a, s.x.imp, name)
	if err == nil {
		s.x.imp = nil
	}
	if p == nil {
		return s.planDone(0, t, err)
	}
	return s.planDone(p.ID, t, err)
}

// UsePlan switches the current plan; no game file is written.
func (s *API) UsePlan(id int64) (*PlanDone, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, t, err := view.UsePlan(s.a, id)
	return s.planDone(id, t, err)
}

// RenamePlan renames a plan.
func (s *API) RenamePlan(id int64, name string) (*PlanDone, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := view.RenamePlan(s.a, id, name)
	return s.planDone(id, t, err)
}

// RemovePlanWarn is the delete confirmation (an error: it cannot be deleted).
func (s *API) RemovePlanWarn(id int64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.RemovePlanWarn(s.a, id)
}

// RemovePlan deletes a plan once the player confirmed.
func (s *API) RemovePlan(id int64) (*PlanDone, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := view.RemovePlan(s.a, id)
	return s.planDone(0, t, err)
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
		// A game added once the shared mod folders were read: only its own Data/ and Mods/
		// (02 §0 item 8). The first game reads the shared folders as well.
		var flags []string
		if !s.a.SharedUnread() {
			flags = append(flags, "--game-only")
		}
		home, exe := s.a.Home, s.Exe
		run = func(env string) (string, error) {
			if exe != "" {
				return app.ChildScanExe(exe, home, env, flags...)
			}
			return app.ChildScan(home, env, flags...)
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

var (
	liveRe   = regexp.MustCompile(`(\d+) live`)
	srcModRe = regexp.MustCompile(`^src:\S+\s+(\d+) mods?\b`)
)

// scanLine is the scan's "instances" line in words: how many mods it found. A scan of a
// game's own folders (--game-only) counts its Data/ and Mods/.
func scanLine(out string) string {
	if strings.Contains(out, "[game only]") {
		n := 0
		for _, l := range strings.Split(out, "\n") {
			if m := srcModRe.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
				k, _ := strconv.Atoi(m[1])
				n += k
			}
		}
		return view.Tf("%d mods in its Data and Mods", n)
	}
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

// ---- the guide's "download a version" (08 §11) ----

// DownloadInfo is what the download step shows: SteamCMD (or the offer to download it from
// Valve, with its size), RimWorld's branches, and a folder for the game.
type DownloadInfo struct {
	SteamCMD string             `json:"steamcmd"` // "" when it is not installed
	Offer    *app.SteamCMDOffer `json:"offer,omitempty"`
	Branches []app.GameBranch   `json:"branches,omitempty"`
	Games    string             `json:"games"` // a folder to put the new game in
	Error    string             `json:"error,omitempty"`
}

// GuideDownloadInfo asks SteamCMD (anonymously) which branches RimWorld has. SteamCMD
// runs without the API's lock: the first run updates itself and takes a while.
func (s *API) GuideDownloadInfo() *DownloadInfo {
	s.mu.Lock()
	a := s.a
	games := gamesFolder(a)
	s.mu.Unlock()
	d := &DownloadInfo{SteamCMD: a.SteamCMDPath(), Games: games}
	if d.SteamCMD == "" {
		d.Offer, _ = a.OfferSteamCMD()
		return d
	}
	bs, err := a.GameBranches()
	if err != nil {
		d.Error = view.Humanize(nil, err.Error())
	}
	d.Branches = bs
	return d
}

// gamesFolder suggests where a downloaded game goes: <drive>\Games next to the games there
// are (not inside a Steam library), else the system drive's.
func gamesFolder(a *app.App) string {
	envs, _ := a.Store.Envs()
	for _, e := range envs {
		if v := filepath.VolumeName(e.GameRoot); v != "" {
			return v + `\Games`
		}
	}
	if v := os.Getenv("SystemDrive"); v != "" {
		return v + `\Games`
	}
	return filepath.Join(os.Getenv("HOME"), "Games")
}

// GuideInstallSteamCMD downloads SteamCMD from Valve (the player said yes to the offer).
func (s *API) GuideInstallSteamCMD() (string, error) {
	p, err := s.a.InstallSteamCMD()
	if err != nil {
		return "", errors.New(view.Humanize(nil, err.Error()))
	}
	return p, nil
}

// GuideDownload starts SteamCMD for a version and returns at once: SteamCMD opens its own
// window for the login, and "download" events carry its progress and the end (with the
// environment added, as for a game the player has).
func (s *API) GuideDownload(branch, dir, account, name string) (bool, error) {
	s.mu.Lock()
	if s.busy["download"] {
		s.mu.Unlock()
		return false, nil
	}
	if _, err := s.a.CheckDownloadDir(dir); err != nil {
		s.mu.Unlock()
		return false, errors.New(view.Humanize(nil, err.Error()))
	}
	s.busy["download"] = true
	s.mu.Unlock()
	spec := app.DownloadSpec{Branch: branch, Dir: dir, Account: strings.TrimSpace(account), Name: strings.TrimSpace(name), Console: true}
	go func() {
		var r *app.SourceChangeEnv
		got, err := s.a.FetchGame(spec, func(p app.DownloadProgress) { s.send("download", p) })
		end := map[string]any{"end": true}
		s.mu.Lock()
		if err == nil {
			if r, err = s.a.AddDownloaded(spec, got); err == nil {
				s.a.EnvName = ""
				_, err = s.a.EnvUse(r.Env.Name)
			}
		}
		s.busy["download"], s.board, s.needs = false, nil, nil
		s.mu.Unlock()
		if err != nil {
			end["ok"], end["error"] = false, view.DownloadErrorText(err)
		} else {
			end["ok"], end["env"] = true, r.Env.Name
		}
		s.send("download", end)
	}()
	return true, nil
}
