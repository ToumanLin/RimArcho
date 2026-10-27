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
	// Plans are the environment's plans, the current one (launched) first; Plan is the open
	// one (the editor's); Current the current one when another is open; InGame what the
	// game's ModsConfig.xml holds (04 §1.3).
	Plans   []view.PlanRow    `json:"plans"`
	Plan    *view.PlanRow     `json:"plan,omitempty"`
	Current *view.PlanRow     `json:"current,omitempty"`
	InGame  *view.InGameState `json:"inGame,omitempty"`
}

// Shell builds the sidebar from the needs count and the packs.
func (s *API) Shell() *Shell {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shellLocked(nil)
}

// shellLocked builds the sidebar; n is needs just built for the current board, else nil.
func (s *API) shellLocked(n *view.Needs) *Shell {
	out := &Shell{Crumbs: map[string][]view.Crumb{}}
	if envs, err := s.a.Store.Envs(); err == nil {
		out.Envs = len(envs)
	}
	count := 0
	if st := s.boardState(); st != nil {
		if n == nil {
			n = s.needsLocked()
		}
		count = n.Count
		out.Env = view.EnvChip(st.Ix.Env.Name, st.Ix.Env.GameVersion)
	} else {
		out.Env = view.EnvChip("", "")
	}
	packs, _, _ := view.PackRows(s.a)
	if out.Envs > 0 {
		out.Plans, _ = view.PlanRows(s.a)
		out.InGame = view.InGame(s.a)
	}
	for i := range out.Plans {
		if out.Plans[i].Open {
			out.Plan = &out.Plans[i]
		} else if out.Plans[i].Current {
			out.Current = &out.Plans[i]
		}
	}
	out.Nav = view.Nav(count, out.Plans, packs)
	ids := []string{view.PageSettings, view.PageTasks}
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
	s.logRead.mu.Lock()
	defer s.logRead.mu.Unlock()
	defer s.free.soon()
	r, err := s.logSession()
	if err != nil {
		return &view.Home{Featured: -1}
	}
	// The launch page is about the current list, the one launched, whichever list the
	// editor has open (04 §1.3); the reader takes the window's options again next time.
	r.OpenPlan, r.PlanName = 0, ""
	b, err := view.BuildBoard(r)
	if err != nil || b.St == nil {
		return &view.Home{}
	}
	st := b.St
	// The last run's line reads Player.log when it is newer than the one read last (06
	// §2.3): the page loads, or the window comes back into focus, after a game.
	_, newer := r.NewLog(st)
	n, _ := readNeeds(r, newer)
	packs, _, _ := view.PackRows(r)
	plans, _ := view.PlanRows(r)
	h := view.BuildHome(r, b, n, plans, packs)
	view.AddOtherEnvs(r, h, st.Ix.Env.ID)
	view.PickFeatured(h, r.Store.Meta(view.FeaturedKey), st.Ix.Env.Name, st.Plan.ID)
	return h
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
	return s.importAsPlan(name)
}

// importAsPlan requires s.mu (both legacy import and the checked new-list preview).
func (s *API) importAsPlan(name string) (*PlanDone, error) {
	if s.x.imp == nil {
		return nil, errors.New(view.T("nothing to import"))
	}
	imp := s.x.imp
	p, t, err := view.ImportAsPlan(s.a, imp, name)
	if err == nil {
		s.x.imp = nil
		if s.x.follow && p != nil {
			if e := view.FollowImported(s.a, imp, p.ID); e != nil {
				t += "  " + e.Error()
			}
		}
	}
	if p == nil {
		return s.planDone(0, t, err)
	}
	return s.planDone(p.ID, t, err)
}

// UsePlan makes a plan current (the one launched) and opens it; no game file is written.
func (s *API) UsePlan(id int64) (*PlanDone, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, t, err := view.UsePlan(s.a, id)
	return s.planDone(id, t, err)
}

// OpenPlan shows a plan in the editor without making it current (06 §2.5): the sidebar's
// lists are places to go, not a choice of what to play.
func (s *API) OpenPlan(id int64) (*PlanDone, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := view.OpenPlan(s.a, id)
	return s.planDone(id, "", err)
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
	r, err := s.detachedReader()
	if err != nil {
		return nil
	}
	defer r.Close()
	return view.FindGames(r)
}

// GuideGame checks a game folder the player typed.
func (s *API) GuideGame(path string) (*view.GameFound, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.GameAt(s.a, path)
}

// GuideScan scans the environments one after another in child processes and returns at
// once; "guide" events carry which one is being read and the end (06 §8.4).
func (s *API) GuideScan(names []string) bool {
	s.mu.Lock()
	if s.busy["scan"] || len(names) == 0 {
		s.mu.Unlock()
		return false
	}
	s.busy["scan"] = true
	run, home, exe := s.ScanRunner, s.a.Home, s.Exe
	s.mu.Unlock()
	names = append([]string(nil), names...)
	go func() {
		ok := true
		r, setupErr := s.detachedReader()
		if r != nil {
			defer r.Close()
		}
		var flags []string
		if setupErr == nil && !r.SharedUnread() {
			flags = append(flags, "--game-only")
		}
		for i, n := range names {
			task := s.beginSelf(view.TaskScan, n, nil)
			s.send("guide", map[string]any{"i": i, "n": len(names), "env": n, "text": view.Tf("reading the mods of %s…", n)})
			out, err := "", setupErr
			if err == nil {
				// The first link preparation can scan too: it belongs here, never in
				// GuideAdd or the GUI call (06 §8.4).
				s.environment.gate.Lock()
				_, err = r.PrepareEnv(n)
				s.environment.gate.Unlock()
			}
			if err == nil {
				switch {
				case run != nil:
					out, err = run(n)
				case exe != "":
					out, err = app.ChildScanExe(exe, home, n, flags...)
				default:
					out, err = app.ChildScan(home, n, flags...)
				}
			}
			if err == nil {
				r.EnvName, r.AnalyzeWait = n, 0
				_, err = r.EnsureAnalysis(nil)
			}
			ev := map[string]any{"i": i, "n": len(names), "env": n, "ok": err == nil, "step": true}
			if err != nil {
				ok = false
				ev["error"] = view.Humanize(nil, firstLine(out)+" "+err.Error())
			} else {
				ev["text"] = fmt.Sprintf("%s: %s", n, scanLine(out))
			}
			s.endSelf(task, err, scanLine(out))
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

// PackImport unpacks a pack file (.archopack) into the packs folder (pack import, 08
// §5.4). A RimArcho list goes through the import preview like any other list.
func (s *API) PackImport(path string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path = strings.Trim(strings.TrimSpace(path), `"'`)
	if !app.IsPackZip(path) {
		return "", errors.New(view.Tf("%s is not a pack (.archopack)", path))
	}
	m, err := s.a.ImportPack(path, false)
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
	Refreshing bool               `json:"refreshing,omitempty"`
	Account    string             `json:"account"`  // effective default; no password
	SteamCMD   string             `json:"steamcmd"` // "" when it is not installed
	Offer      *app.SteamCMDOffer `json:"offer,omitempty"`
	Branches   []app.GameBranch   `json:"branches,omitempty"`
	Games      string             `json:"games"` // a folder to put the new game in
	Error      string             `json:"error,omitempty"`
}

// GuideDownloadInfo asks SteamCMD (anonymously) which branches RimWorld has. SteamCMD
// runs without the API's lock: the first run updates itself and takes a while.
func (s *API) GuideDownloadInfo() *DownloadInfo {
	s.mu.Lock()
	a := s.a
	games := gamesFolder(a)
	account, _ := a.SteamCMDAccount("")
	d := &DownloadInfo{SteamCMD: a.SteamCMDPath(), Games: games, Account: account}
	s.mu.Unlock()
	if d.SteamCMD == "" {
		d.Offer, _ = a.OfferSteamCMD()
		return d
	}
	bs, refreshing, err := a.GameBranchesCached(d.SteamCMD)
	d.Refreshing = refreshing
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

// GuideInstallSteamCMD queues the confirmed download and returns its task ID (06 §2.15).
func (s *API) GuideInstallSteamCMD() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.queueSettingTaskLocked(view.TaskSteamCMD, nil, "")
	return t.ID, nil
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
	account, err := s.a.SteamCMDAccount(account)
	if err != nil {
		s.mu.Unlock()
		return false, errors.New(view.Humanize(nil, err.Error()))
	}
	spec := app.DownloadSpec{Branch: branch, Dir: dir, Account: account, Name: strings.TrimSpace(name), Console: true}
	s.busy["download"] = true
	s.mu.Unlock()
	task := s.beginSelf(view.TaskDownload, branch, nil)
	go func() {
		var r *app.SourceChangeEnv
		got, err := s.a.FetchGame(spec, func(p app.DownloadProgress) { s.send("download", p) })
		end := map[string]any{"end": true}
		if err == nil {
			reader, openErr := s.detachedReader()
			if openErr != nil {
				err = openErr
			} else {
				r, err = reader.AddDownloaded(spec, got)
				reader.Close()
			}
		}
		s.mu.Lock()
		if err == nil {
			err = s.a.Store.SetCurrentEnv(r.Env.ID)
			s.a.EnvName = r.Env.Name
		}
		// GuideScan owns link preparation and the first scan (06 §8.4).
		s.busy["download"], s.board, s.needs = false, nil, nil
		s.mu.Unlock()
		if err != nil {
			end["ok"], end["error"] = false, view.DownloadErrorText(err)
			s.tasks.Finish(task, app.TaskFailed, view.DownloadErrorText(err))
		} else {
			end["ok"], end["env"] = true, r.Env.Name
			s.endSelf(task, nil, r.Env.Name)
		}
		s.send("download", end)
	}()
	return true, nil
}
