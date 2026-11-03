package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/gametest"
	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// TestRunBaseline compares error clusters with an earlier run of the same plan (08 §12).
type TestRunBaseline struct {
	RunDir  string `json:"run_dir"`
	Status  string `json:"status"`
	New     int    `json:"new"`
	Present int    `json:"present"`
	Gone    int    `json:"gone"`
}

func compareTestRun(r *TestRunReport) {
	baseline, clusters := previousTestRun(r)
	if baseline == nil {
		return
	}
	r.Baseline = baseline
	previous := make(map[string]bool, len(clusters))
	for _, c := range clusters {
		class := c.Class
		if gametest.SettingsError(c.First) {
			class = gametest.ClassSettings
		}
		if logs.Informational(class) {
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
	for _, c := range clusters {
		if !previous[c.Key] || current[c.Key] {
			continue
		}
		baseline.Gone++
		if len(r.Gone) == 30 {
			continue
		}
		class := c.Class
		if gametest.SettingsError(c.First) {
			class = gametest.ClassSettings
		}
		message, _, _ := strings.Cut(c.First.Message, "\n")
		r.Gone = append(r.Gone, TestRunCluster{
			Class: class, Message: strings.TrimSuffix(message, "\r"), Count: c.Count,
			Mods: []string{},
		})
	}
}

func previousTestRun(r *TestRunReport) (*TestRunBaseline, []*logs.Cluster) {
	root := filepath.Dir(r.RunDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil
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
			Plan   string   `json:"plan"`
			Status string   `json:"status"`
			Args   []string `json:"args"`
		}
		if json.Unmarshal(b, &report) != nil || report.Plan != r.Plan || report.Status == "links-blocked" ||
			slices.Contains(report.Args, "quicktest") != slices.Contains(r.Args, "quicktest") {
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
		return &TestRunBaseline{RunDir: dir, Status: report.Status}, logs.Clusters(log)
	}
	return nil, nil
}
