package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/gametest"
	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// TestRunBaseline compares error clusters with an earlier run of the same plan (20 §2).
type TestRunBaseline struct {
	RunDir  string `json:"run_dir"`
	Status  string `json:"status"`
	New     int    `json:"new"`
	Present int    `json:"present"`
	Gone    int    `json:"gone"`
}

// TestRunPlayerLog compares a test run with the player's problem log, the log
// `archo log` reads by default (20 §2).
type TestRunPlayerLog struct {
	Path    string `json:"path"`
	Present int    `json:"present"` // this run's clusters already in the player's log
	Absent  int    `json:"absent"`  // this run's clusters the player's log does not have
	Fixed   int    `json:"fixed"`   // the player's clusters this run no longer has
	// Unreached: the player's clusters this run could not have shown: it stopped before
	// its target, or they occurred only after loading a save or in play.
	Unreached int    `json:"unreached,omitempty"`
	Error     string `json:"error,omitempty"`
}

// testRunListLimit is how many clusters each list of the report names.
const testRunListLimit = 30

// comparePlayerLog marks each cluster present or absent in the player's log and lists the
// player's clusters that are gone, with their attribution (20 §2). The player's log is
// attributed as `archo log` attributes it (AnalyzeLog, cached).
func (a *App) comparePlayerLog(st *LoadoutState, r *TestRunReport) {
	cur, prev := a.LogPaths(st)
	path := cur
	if path == "" {
		path = prev
	}
	if path == "" {
		return
	}
	pl := &TestRunPlayerLog{Path: path}
	r.PlayerLog = pl
	rep, err := a.AnalyzeLog(path)
	if err != nil {
		pl.Error = err.Error()
		return
	}
	before := make(map[string]bool, len(rep.Clusters))
	for _, lc := range rep.Clusters {
		before[lc.Key] = true
	}
	current := make(map[string]bool, len(r.clusterKeys))
	for _, key := range r.clusterKeys {
		current[key] = true
		if before[key] {
			pl.Present++
		} else {
			pl.Absent++
		}
	}
	for _, list := range [][]TestRunCluster{r.Clusters, r.Info} {
		for i := range list {
			c := &list[i]
			c.Before = "absent"
			if before[c.key] {
				c.Before = "present"
			}
		}
	}
	reached := r.reached()
	// A probe run starts a new game like a map run; its minutes of play do not count (20 §3).
	target := testRunTarget{mapRun: slices.Contains(r.Args, "quicktest"), ticks: r.TicksReached > 0}
	if r.Save != nil {
		target.save = r.Save.Name
	}
	r.playerClusters = map[string]TestRunCluster{}
	for _, lc := range rep.Clusters {
		// Preference errors are not compared: test runs use their own preferences.
		if logs.Informational(lc.Class) || gametest.SettingsError(lc.First) {
			continue
		}
		c := TestRunCluster{Class: lc.Class, Message: shortMessage(lc.First.Message), Count: lc.Count,
			Mods: testRunMods(st, lc), Confidence: lc.Confidence, Consequences: lc.Consequences}
		if slices.Contains(lc.Details, ownArtNote) {
			// Gone because files were added, not because the fault was fixed (MCP eval h2).
			c.Note = ownArtNote
		} else if lc.Class == logs.ClassTexture {
			c.Note = renderingOnlyNote
		}
		r.playerClusters[lc.Key] = c
		if current[lc.Key] {
			continue
		}
		if !reached || !testRunReaches(lc, rep.Phases, rep.GameMarks, target) {
			pl.Unreached++
			continue
		}
		pl.Fixed++
		// A consequence of a failed def file is listed through its cause (06 §3).
		if len(r.Fixed) < testRunListLimit && lc.ConsequenceOf == "" {
			r.Fixed = append(r.Fixed, c)
		}
	}
}

func compareTestRun(r *TestRunReport) {
	baseline, clusters, listed := previousTestRun(r)
	if baseline == nil {
		return
	}
	r.Baseline = baseline
	previous := make(map[string]bool, len(clusters))
	for _, c := range clusters {
		if logs.Informational(testRunClass(c)) {
			continue
		}
		previous[c.Key] = true
	}
	current := make(map[string]bool, len(r.clusterKeys))
	for _, key := range r.clusterKeys {
		current[key] = true
		if previous[key] {
			baseline.Present++
		} else {
			baseline.New++
		}
	}
	for i := range r.Clusters {
		c := &r.Clusters[i]
		c.Delta = "new"
		if previous[c.key] {
			c.Delta = "present"
		}
	}
	// Gone clusters keep the attribution the earlier report listed, else the player's log's.
	attributed := make(map[string]TestRunCluster, len(listed))
	for _, c := range listed {
		attributed[goneKey(c.Class, c.Message, c.Count)] = c
	}
	for _, c := range clusters {
		if !previous[c.Key] || current[c.Key] {
			continue
		}
		baseline.Gone++
		if len(r.Gone) == testRunListLimit {
			continue
		}
		g := TestRunCluster{Class: testRunClass(c), Message: shortMessage(c.First.Message), Count: c.Count, Mods: []string{}}
		// Earlier reports list the whole first line.
		if p, ok := attributed[goneKey(g.Class, firstLine(c.First.Message), g.Count)]; ok && p.Mods != nil {
			g.Mods, g.Confidence = p.Mods, p.Confidence
		}
		if p, ok := r.playerClusters[c.Key]; ok && len(g.Mods) == 0 && (len(p.Mods) > 0 || g.Confidence == "") {
			g.Mods, g.Confidence = p.Mods, p.Confidence
		}
		r.Gone = append(r.Gone, g)
	}
}

func testRunClass(c *logs.Cluster) string {
	if gametest.SettingsError(c.First) {
		return gametest.ClassSettings
	}
	return c.Class
}

func goneKey(class, message string, count int) string {
	return class + "\x00" + message + "\x00" + strconv.Itoa(count)
}

func firstLine(message string) string {
	line, _, _ := strings.Cut(message, "\n")
	return strings.TrimSuffix(line, "\r")
}

// previousTestRun finds the baseline run, its clusters and the clusters its report listed.
func previousTestRun(r *TestRunReport) (*TestRunBaseline, []*logs.Cluster, []TestRunCluster) {
	root := filepath.Dir(r.RunDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil, nil
	}
	// ReadDir sorts names; timestamps sort in chronological order.
	slices.Reverse(entries)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() >= filepath.Base(r.RunDir) {
			continue
		}
		if _, err := time.Parse("20060102-150405", entry.Name()); err != nil {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		if source.IsLink(dir) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, "report.json"))
		if err != nil {
			continue
		}
		var report struct {
			Plan     string           `json:"plan"`
			Status   string           `json:"status"`
			Args     []string         `json:"args"`
			Clusters []TestRunCluster `json:"clusters"`
			Probe    *TestRunProbe    `json:"probe"`
			Save     *TestRunSave     `json:"save"`
		}
		// A probe run compares only with runs of the same probe (20 §3), a save run only
		// with runs of the same save.
		if json.Unmarshal(b, &report) != nil || report.Plan != r.Plan || report.Status == "links-blocked" ||
			slices.Contains(report.Args, "quicktest") != slices.Contains(r.Args, "quicktest") ||
			probePackage(report.Probe) != probePackage(r.Probe) || saveName(report.Save) != saveName(r.Save) {
			continue
		}
		f, err := os.Open(filepath.Join(dir, "Player.log"))
		if err != nil {
			continue
		}
		log, parseErr := logs.Parse(f)
		f.Close()
		if parseErr != nil {
			continue
		}
		return &TestRunBaseline{RunDir: dir, Status: report.Status}, logs.Clusters(log), report.Clusters
	}
	return nil, nil, nil
}
