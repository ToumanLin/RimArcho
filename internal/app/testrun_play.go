package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/savefile"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// Save runs, ticks, time limits and what the game loaded (20 §2).

// testRunSaveTicks: a save run plays this many ticks after the load when none are given
// (about an in-game hour and a half; about 70 s at normal speed).
const testRunSaveTicks = 4000

// testRunMinTimeout is the least time a map or save run gets when none is given.
const testRunMinTimeout = 10 * time.Minute

// TestRunSave is the save a save run loads (20 §2).
type TestRunSave struct {
	Name        string `json:"name"` // the file name without .rws, as the game's log names it
	Path        string `json:"path"`
	GameVersion string `json:"game_version,omitempty"`
	// Added: plan Mods the save was not saved with (allowed: Mods added to a colony).
	Added []string `json:"added,omitempty"`
	// OrderDiffers: the Mods both have are in another order in the plan.
	OrderDiffers bool `json:"order_differs,omitempty"`
}

// testRunSave resolves a save (a list_saves reference or name, or a .rws path) and checks
// that loading it with the plan tests the plan (20 §2): refused when the save is cut off,
// from another major.minor of the game, or uses Mods the plan does not have.
func (a *App) testRunSave(st *LoadoutState, ref string) (*TestRunSave, error) {
	path := ""
	if strings.EqualFold(filepath.Ext(ref), ".rws") {
		if fi, err := os.Stat(ref); err == nil && !fi.IsDir() {
			path = realOrAbs(ref)
		}
	}
	if path == "" {
		s, _, err := a.FindSave(ref)
		if err != nil {
			return nil, fmt.Errorf("test run: %w", err)
		}
		if s.Writing {
			return nil, fmt.Errorf("test run: save %s is being written; try again in a moment", s.File)
		}
		path = s.Path
	}
	h, _, changed, err := savefile.ReadHead(path)
	if err != nil {
		return nil, fmt.Errorf("test run: reading save %s: %w", path, err)
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if changed {
		return nil, fmt.Errorf("test run: save %s is being written; try again in a moment", name)
	}
	if !h.Complete {
		return nil, fmt.Errorf("test run: save %s is cut off (it does not end with </savegame>); the game cannot load it", name)
	}
	if sm, gm := env.MajorMinor(h.GameVersion), st.GameMM(); sm != "" && gm != "" && sm != gm {
		return nil, fmt.Errorf("test run: save %s is from RimWorld %s and the game is %s; its errors would be the conversion's, not the plan's", name, h.GameVersion, st.Ix.Env.GameVersion)
	}
	var plan []string
	names := map[string]string{}
	for _, e := range st.List {
		pkg, label := e.Missing, e.Missing
		if m := st.Ix.ByID[e.Instance]; m != nil {
			pkg, label = m.PackageID, m.DisplayName()
		}
		plan = append(plan, normPkg(pkg))
		names[normPkg(pkg)] = label
	}
	var saved, lacking []string
	for i, id := range h.ModIDs {
		saved = append(saved, normPkg(id))
		if !slices.Contains(plan, normPkg(id)) {
			lacking = append(lacking, fmt.Sprintf("%s (%s)", h.Name(i), id))
		}
	}
	if len(lacking) > 0 {
		return nil, fmt.Errorf("test run: save %s uses %d Mods the plan does not have: %s. Loading it without them logs their missing data, not the plan's errors; test a plan with the save's Mods (get_save compares them)",
			name, len(lacking), strings.Join(lacking, ", "))
	}
	r := &TestRunSave{Name: name, Path: path, GameVersion: h.GameVersion}
	var common []string
	for _, id := range plan {
		if !slices.Contains(saved, id) {
			r.Added = append(r.Added, names[id])
		} else {
			common = append(common, id)
		}
	}
	r.OrderDiffers = !slices.Equal(common, saved)
	return r, nil
}

func saveName(s *TestRunSave) string {
	if s == nil {
		return ""
	}
	return strings.ToLower(s.Name)
}

// testRunMode is how a run is compared and timed: menu, map (test maps, probes included) or save.
func testRunMode(mapRun bool, save *TestRunSave) string {
	switch {
	case save != nil:
		return "save"
	case mapRun:
		return "map"
	}
	return "menu"
}

// testRunTimeout is the time limit of a run given none: 10 minutes for a menu run; for a
// map or save run twice the load time of the newest earlier run of this environment in
// the same mode that arrived, plus twice the extra ticks at normal speed, at least 10
// minutes (20 §2).
func testRunTimeout(runs, current, mode string, ticks int) time.Duration {
	if mode == "menu" {
		return testRunMinTimeout
	}
	prevLoad, prevTicks := 0.0, 0
	entries, _ := os.ReadDir(runs)
	slices.Reverse(entries)
	for _, entry := range entries {
		dir := filepath.Join(runs, entry.Name())
		if !entry.IsDir() || dir == current || source.IsLink(dir) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, "report.json"))
		if err != nil {
			continue
		}
		var rep struct {
			Status      string       `json:"status"`
			Args        []string     `json:"args"`
			Save        *TestRunSave `json:"save"`
			LoadSeconds float64      `json:"load_seconds"`
			Ticks       int          `json:"ticks"`
		}
		if json.Unmarshal(b, &rep) != nil || testRunMode(slices.Contains(rep.Args, "quicktest"), rep.Save) != mode {
			continue
		}
		switch rep.Status {
		case "map", "save", "probe", "incomplete":
			prevLoad, prevTicks = rep.LoadSeconds, rep.Ticks
		default:
			continue
		}
		break
	}
	t := time.Duration(2*prevLoad*float64(time.Second)) + time.Duration(2*max(0, ticks-prevTicks))*time.Second/60
	return max(t, testRunMinTimeout)
}

// TestRunLoaded is the folder the game took for a plan Mod (20 §2).
type TestRunLoaded struct {
	PackageID string `json:"package_id"`
	Name      string `json:"name"`
	Folder    string `json:"folder"` // Mods/<name>, Workshop/<id> or another source's folder
	// From: "log" when the run's Archo Monitor log names the folder, "disk" when it is the
	// only copy of the packageId the game could see during the run.
	From string `json:"from"`
}

// runningModsRe is a line of the game's running Mods list after its new-game or load line.
var runningModsRe = regexp.MustCompile(`^\s+- (\S+)\s*$`)

// runningMods reads the packageIds (normPkg) the game listed after its last new-game or
// save-load line; nil when the log has neither.
func runningMods(logPath string) map[string]bool {
	f, err := os.Open(logPath)
	if err != nil {
		return nil
	}
	defer f.Close()
	var running map[string]bool
	in := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		l := sc.Text()
		if m := logs.GameMark(l); m != nil {
			running, in = map[string]bool{}, true
			continue
		}
		if !in {
			continue
		}
		if m := runningModsRe.FindStringSubmatch(l); m != nil {
			running[normPkg(m[1])] = true
		} else {
			in = false
		}
	}
	return running
}

// testRunFolders maps each listed Mod outside Data/ the run's game sees to its folder as
// the report names it: Mods/<name> (a folder or link there) or Workshop/<id>. Called while
// the run's links are there.
func testRunFolders(st *LoadoutState) map[string]string {
	mods := modsDir(st.Ix)
	out := map[string]string{}
	for id, name := range testRunSeen(st) {
		if mods != "" && lexists(filepath.Join(mods, name)) {
			out[id] = "Mods/" + name
		} else {
			out[id] = "Workshop/" + name
		}
	}
	return out
}

// testRunLoaded fills Loaded with the folder the game took for each plan Mod outside Data/,
// and moves to Missing the Mods the log shows the game did not load: one its running Mods
// list lacks, or one Archo Monitor saw in another folder than this run's copy.
func (a *App) testRunLoaded(st *LoadoutState, r *TestRunReport, seen map[string]string, data string) {
	running := runningMods(r.LogPath)
	folders := map[string]string{} // packageId (normPkg) -> folder, from the Monitor's settings record
	if path := CompanionForRun(r.LogPath, data); path != "" {
		if c, err := logs.ReadCompanion(path); err == nil {
			for _, s := range c.Settings {
				if s.PackageID != "" && s.Folder != "" {
					folders[normPkg(s.PackageID)] = s.Folder
				}
			}
		}
	}
	var loaded, linked []TestRunLoaded
	for _, e := range st.List {
		m := st.Ix.ByID[e.Instance]
		if m == nil || m.Kind == source.Official || seen[m.ID] == "" {
			continue
		}
		pkg := normPkg(m.PackageID)
		folder := seen[m.ID]
		_, name, _ := strings.Cut(folder, "/")
		l := TestRunLoaded{PackageID: m.PackageID, Name: m.DisplayName(), Folder: folder, From: "disk"}
		if f := folders[pkg]; f != "" {
			if !strings.EqualFold(f, name) {
				r.Missing = append(r.Missing, TestRunMissing{PackageID: m.PackageID, Name: m.DisplayName(),
					Reason: fmt.Sprintf("Archo Monitor saw the game load it from the folder %s, not this run's %s", f, folder)})
				continue
			}
			l.From = "log"
		}
		if running != nil && !running[pkg] {
			r.Missing = append(r.Missing, TestRunMissing{PackageID: m.PackageID, Name: m.DisplayName(),
				Reason: "not in the running Mods the game listed when the map started; the log says why (a missing dependency, a wrong game version)"})
			continue
		}
		r.LoadedTotal++
		if slices.Contains(r.Linked, name) {
			linked = append(linked, l)
		} else {
			loaded = append(loaded, l)
		}
	}
	slices.SortStableFunc(loaded, func(x, y TestRunLoaded) int {
		return strings.Compare(y.From, x.From) // "log" before "disk"
	})
	r.Loaded = append(linked, loaded...)
	if len(r.Loaded) > testRunListLimit {
		r.Loaded = r.Loaded[:testRunListLimit]
	}
}

// testRunNotTested turns a run that reached its target with Mods missing into
// "incomplete", and says why a run is no test of the plan; "" when it is one (20 §2).
func testRunNotTested(r *TestRunReport, mode string) string {
	target := map[string]string{"menu": "the main menu", "map": "the map", "save": "the loaded save"}[mode]
	if r.Ticks > 0 {
		target = fmt.Sprintf("%d ticks on %s", r.Ticks, target)
	}
	last := ""
	if r.LastLogLine != "" {
		last = "; last log line: " + r.LastLogLine
	}
	where := ""
	if r.Phase != "" && r.Phase != "loading" {
		where = fmt.Sprintf(" (it got to %s)", map[string]string{"menu": "the main menu", "new": "a new game", "save": "the save load"}[r.Phase])
	}
	switch r.Status {
	case "timeout":
		return fmt.Sprintf("the game did not reach %s within %.0f s%s%s", target, r.TimeoutSeconds, where, last)
	case "exited":
		return fmt.Sprintf("the game exited before it reached %s%s%s", target, where, last)
	case "start-failed":
		return "the game did not start"
	}
	if r.reached() && len(r.Missing) > 0 {
		r.Status = "incomplete"
		var names []string
		for _, m := range r.Missing {
			names = append(names, m.Name+" ("+m.PackageID+")")
		}
		return "these Mods were not loaded: " + strings.Join(names, ", ")
	}
	return ""
}

// testRunTarget is what a run can show of the player's errors (testRunReaches).
type testRunTarget struct {
	mapRun bool   // a test map: a new game (probe runs too)
	ticks  bool   // it played the asked ticks
	save   string // a save run: the save's name
}

// testRunReaches reports whether a run could show a cluster of the player's log (20 §2).
// With Archo Monitor phases (19 §4): every run loads and reaches the main menu; a map run
// also starts a new game, and with ticks plays it; a save run loads that save and plays.
// Without them, the game's own lines place the cluster's first occurrence (marks): before
// any, loading or the menu; after "Initializing new game", a new game (play included);
// after "Loading game from file X", the save X. A cluster with neither counts as reached.
func testRunReaches(lc *LogCluster, phases, marks []*logs.Phase, t testRunTarget) bool {
	if len(lc.Phases) > 0 {
		for _, pc := range lc.Phases {
			p := pc.Phase
			if logs.PhaseName(p) == "play" {
				// Play after a new game or after a save load: the phase before it says which.
				if p == nil || p.Index < 2 || p.Index > len(phases) {
					continue
				}
				switch before := phases[p.Index-2]; before.Name {
				case "new":
					if t.mapRun && t.ticks {
						return true
					}
				case "save":
					if t.reaches(before) {
						return true
					}
				}
				continue
			}
			if t.reaches(p) {
				return true
			}
		}
		return false
	}
	if lc.OnlyCompanion || lc.First == nil {
		return true
	}
	return t.reaches(logs.GamePhaseAt(marks, lc.First.Line))
}

func (t testRunTarget) reaches(p *logs.Phase) bool {
	switch logs.PhaseName(p) {
	case "loading", "menu", "unknown":
		return true
	case "new":
		return t.mapRun
	case "save":
		return t.save != "" && (p.Save == "" || strings.EqualFold(p.Save, t.save))
	}
	return false
}

// deepSavedNote marks the save-load cluster dev mode reports while this run's autosave is
// written (06 §1: it stays an error).
const deepSavedNote = "written while this run's autosave was saved: the game checks saved references only in dev mode, which a save run turns on. A Mod that saves a reference to a pawn or thing it does not save causes it; vanilla dev mode reports some pawns too (MCP eval h7), so compare with the player's log (before)"

// testRunNote is the note for a cluster this run's own setup explains ("" for none).
func testRunNote(r *TestRunReport, lc *LogCluster) string {
	if r.Ticks > 0 && lc.Class == logs.ClassSave && strings.Contains(lc.First.Message, "but is not deep-saved") {
		return deepSavedNote
	}
	return ""
}
