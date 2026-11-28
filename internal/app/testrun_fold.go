package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// The --json and text reports of a test run fold repeated clusters and cut the probe's
// lines (20 §2 "Report size"); report.json keeps every listed cluster, since a later run's
// gone list reads its attribution from there.

const (
	foldMin           = 3 // clusters with one class and note that fold into one entry
	foldExamples      = 3 // messages a folded entry keeps
	probeBriefHead    = 3 // probe lines in the --json report: the first 3 and the last 5
	probeBriefTail    = 5
	probeBriefLineMax = 160
	probeLinesFile    = "probe-lines.txt"
)

// FoldClusters folds clusters that share their class, note, delta, before and confidence
// (and, without a note, their Mods) into one entry once there are foldMin of them: at the
// first one's place, with the summed count, every candidate Mod (the most frequent first,
// ModClusters counting them), Folded clusters and the first foldExamples messages.
func FoldClusters(list []TestRunCluster) []TestRunCluster {
	type group struct {
		idx  []int
		mods map[string]int
	}
	key := func(c TestRunCluster) string {
		k := c.Class + "\x00" + c.Note + "\x00" + c.Delta + "\x00" + c.Before + "\x00" + c.Confidence
		if c.Note == "" { // without a note, only clusters of the same Mods fold
			k += "\x00" + strings.Join(c.Mods, "\x00")
		}
		return k
	}
	// Without a note and a Mod, clusters say nothing in common.
	alone := func(c TestRunCluster) bool { return c.Note == "" && len(c.Mods) == 0 }
	groups := map[string]*group{}
	for i, c := range list {
		if alone(c) {
			continue
		}
		g := groups[key(c)]
		if g == nil {
			g = &group{mods: map[string]int{}}
			groups[key(c)] = g
		}
		g.idx = append(g.idx, i)
		for _, m := range c.Mods {
			g.mods[m]++
		}
	}
	out := make([]TestRunCluster, 0, len(list))
	for i, c := range list {
		g := groups[key(c)]
		if alone(c) || len(g.idx) < foldMin {
			out = append(out, c)
			continue
		}
		if g.idx[0] != i {
			continue
		}
		f := c
		f.Message, f.Count, f.Consequences, f.Folded, f.Examples = "", 0, 0, len(g.idx), nil
		for _, j := range g.idx {
			f.Count += list[j].Count
			f.Consequences += list[j].Consequences
			if len(f.Examples) < foldExamples {
				f.Examples = append(f.Examples, shortMessage(list[j].Message))
			}
		}
		f.Mods = make([]string, 0, len(g.mods))
		for m := range g.mods {
			f.Mods = append(f.Mods, m)
		}
		slices.SortFunc(f.Mods, func(a, b string) int {
			if d := g.mods[b] - g.mods[a]; d != 0 {
				return d
			}
			return strings.Compare(a, b)
		})
		f.ModClusters = make([]int, len(f.Mods))
		for k, m := range f.Mods {
			f.ModClusters[k] = g.mods[m]
		}
		out = append(out, f)
	}
	return out
}

// Brief is the probe part of the --json report: the first 3 and the last 5 lines cut to
// 160 characters (lines_file has every line), and the named screenshots' file names.
func (p *TestRunProbe) Brief() *TestRunProbe {
	if p == nil {
		return nil
	}
	b := *p
	lines := p.Lines
	if len(lines) > probeBriefHead+probeBriefTail {
		lines = append(slices.Clone(lines[:probeBriefHead]), lines[len(lines)-probeBriefTail:]...)
	}
	b.Lines = make([]string, len(lines))
	for i, l := range lines {
		b.Lines[i] = cutRunes(l, probeBriefLineMax)
	}
	// The named screenshots are all in the run folder (run_dir).
	b.Screenshots = nil
	for _, shot := range p.Screenshots {
		b.Screenshots = append(b.Screenshots, filepath.Base(shot))
	}
	return &b
}

// Verdict is the report's first line: whether the run tested the plan, whether the probe's
// RESULT arrived (the --json report's probe, right after it, has the text), and the error
// clusters.
func (r *TestRunReport) Verdict() string {
	if r.Status == "links-blocked" {
		return "not run: Mods/ has links that need repair first (links)"
	}
	var parts []string
	if r.NotTested != "" {
		parts = append(parts, "NOT TESTED: "+r.NotTested)
	}
	if p := r.Probe; p != nil {
		switch p.Outcome {
		case "result":
			parts = append(parts, "probe RESULT arrived (probe.result)")
		case "exception":
			parts = append(parts, "probe EXCEPTION (probe.exception)")
		default:
			parts = append(parts, "probe: no RESULT line")
		}
	}
	switch errs := r.ClustersTotal; {
	case errs == 0 && r.reached():
		parts = append(parts, "reached "+r.Status+" with no error clusters")
	case errs == 0:
		parts = append(parts, "no error clusters")
	default:
		line := fmt.Sprintf("%d error clusters", errs)
		if b := r.Baseline; b != nil {
			line += fmt.Sprintf(" (%d new since the previous run)", b.New)
		} else if p := r.PlayerLog; p != nil && p.Error == "" {
			line += fmt.Sprintf(" (%d not in the player's log)", p.Absent)
		}
		parts = append(parts, line)
	}
	return strings.Join(parts, "; ")
}

// writeProbeLines saves every [Probe] line of the run in its folder (20 §3).
func writeProbeLines(p *TestRunProbe, runDir string, all []string) error {
	if len(all) == 0 || runDir == "" {
		return nil
	}
	path := filepath.Join(runDir, probeLinesFile)
	if err := os.WriteFile(path, []byte(strings.Join(all, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	p.LinesFile = path
	return nil
}
