package cmds

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// probeRunReport is a synthetic probe run shaped like the one an agent noted (2026-11-27):
// 30 listed of 337 missing-texture clusters with one note, a 40-line probe and its RESULT.
func probeRunReport() *app.TestRunReport {
	note := "rendering only: the game draws a placeholder (or nothing) where this texture belongs; nothing stops loading or playing because of it, and it causes none of the other errors in this log"
	r := &app.TestRunReport{Env: "1.6", Plan: "plan:probe-run", GameVersion: "1.6.4871 rev590", Status: "probe",
		LoadSeconds: 139.5, TotalSeconds: 140.1, PeakMemoryMB: 1720, TimeoutSeconds: 900, Phase: "new", MapSeconds: 124.1,
		TicksEstimated: 7446, Requested: 12, Missing: []app.TestRunMissing{}, RunDir: `C:\Users\Player\AppData\Roaming\RimArcho\tests\envid\runs\20261127-212400`,
		LogPath: `C:\Users\Player\AppData\Roaming\RimArcho\tests\envid\runs\20261127-212400\Player.log`, Attribution: "tested-plan",
		Counts: map[string]int{"texture-missing": 337}, ClustersTotal: 337,
		PlayerLog: &app.TestRunPlayerLog{Path: `D:\Game\Player.log`, Present: 0, Absent: 337},
		Baseline:  &app.TestRunBaseline{RunDir: `C:\runs\20261127-211000`, Status: "probe", Present: 337}}
	for i := range 4 {
		r.Info = append(r.Info, app.TestRunCluster{Class: "mod-report", Count: 1, Before: "absent", Message: fmt.Sprintf("[Mod %d] loaded settings for version 1.6, all features enabled; see the settings window for more", i)})
	}
	for i := range 6 {
		r.Loaded = append(r.Loaded, app.TestRunLoaded{PackageID: fmt.Sprintf("author.mod%d", i), Name: fmt.Sprintf("Mod %d", i), Folder: fmt.Sprintf("Mods/_Archo_workshop_35239167%02d", i), From: "log"})
	}
	for i := range 30 {
		mod := "Beds Alpha"
		if i%3 == 0 {
			mod = "Light Industries"
		}
		r.Clusters = append(r.Clusters, app.TestRunCluster{Class: "texture-missing", Count: 4 + i%3,
			Message: fmt.Sprintf("Failed to find any textures at Things/Bed%d/Bed%d while constructing Multi(initPath=Things/Bed%d/Bed%d, color=RGBA(1.000, 1.000, 1.000, 1.000), colorTwo=RGBA(1.000, 1.000, 1.000, 1.000))", i, i, i, i),
			Mods:    []string{mod}, Confidence: "likely", Delta: "present", Before: "absent", Note: note})
	}
	for i := range 11 {
		r.Fixed = append(r.Fixed, app.TestRunCluster{Class: "texture-missing", Count: 2, Message: fmt.Sprintf("Failed to find any textures at Old/Path%d while constructing Multi(initPath=Old/Path%d...", i, i),
			Mods: []string{"Beds Alpha"}, Confidence: "likely", Note: note})
	}
	r.Probe = &app.TestRunProbe{Folder: `D:\work\probe`, PackageID: "author.probe", Position: 13, Outcome: "result",
		Result: strings.Repeat("Bed:PASS maxAsleep=3/3 allAsleepTicks=24750 | ", 12), LinesTotal: 40,
		LinesFile: `C:\Users\Player\AppData\Roaming\RimArcho\tests\envid\runs\20261127-212400\probe-lines.txt`}
	for i := range 40 {
		r.Probe.Lines = append(r.Probe.Lines, fmt.Sprintf("t=%d %s", 34499+250*i, strings.Repeat("DoubleBed=0/3[GotoWander,Wait_Wander] ", 7)))
	}
	return r
}

// TestTestRunJSONSize: the --json report of the noted probe run starts with its verdict and
// the probe RESULT, folds the 30 clusters into one entry, and stays under 6 KB (20 §2).
func TestTestRunJSONSize(t *testing.T) {
	r := probeRunReport()
	before, _ := json.MarshalIndent(r, "", "  ")
	after, err := lineJSON(testRunJSONReport(r))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("--json report: %d bytes before folding, %d after", len(before), len(after))
	if len(after) > 6<<10 {
		t.Fatalf("--json report is %d bytes, over 6 KB", len(after))
	}
	s := string(after)
	if !strings.HasPrefix(s, "{\n  \"verdict\":\"probe RESULT arrived (probe.result); 337 error clusters (0 new since the previous run)\",\n  \"probe\":{\"outcome\":\"result\",\"result\":\"Bed:PASS") {
		t.Fatalf("the verdict and the probe do not come first:\n%s", s[:min(len(s), 600)])
	}
	var out struct {
		Clusters []app.TestRunCluster `json:"clusters"`
		Fixed    []app.TestRunCluster `json:"fixed"`
		Probe    app.TestRunProbe     `json:"probe"`
	}
	if err := json.Unmarshal(after, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Clusters) != 1 || out.Clusters[0].Folded != 30 || out.Clusters[0].Count != 150 || len(out.Clusters[0].Examples) != 3 ||
		strings.Join(out.Clusters[0].Mods, ",") != "Beds Alpha,Light Industries" || fmt.Sprint(out.Clusters[0].ModClusters) != "[20 10]" {
		t.Fatalf("clusters not folded into one entry: %+v", out.Clusters)
	}
	if len(out.Fixed) != 1 || out.Fixed[0].Folded != 11 {
		t.Fatalf("fixed not folded: %+v", out.Fixed)
	}
	if len(out.Probe.Lines) != 8 || out.Probe.LinesFile == "" || len([]rune(out.Probe.Lines[7])) > 160 {
		t.Fatalf("probe lines not cut to 8 with the file named: %d lines, file %q", len(out.Probe.Lines), out.Probe.LinesFile)
	}
	// Two clusters with one note stay apart; so do clusters without a note of other Mods.
	two := app.FoldClusters(r.Clusters[:2])
	plain := app.FoldClusters([]app.TestRunCluster{{Class: "exception", Message: "a", Mods: []string{"A"}}, {Class: "exception", Message: "b"}, {Class: "exception", Message: "c", Mods: []string{"C"}}})
	if len(two) != 2 || len(plain) != 3 {
		t.Fatalf("folded too eagerly: %d, %d", len(two), len(plain))
	}
	text := testRunDoc(r).String()
	if !strings.Contains(text, "texture-missing x150: 30 clusters, e.g. ") || !strings.Contains(text, "Beds Alpha (20), Light Industries (10)") ||
		!strings.Contains(text, "all lines: ") {
		t.Fatalf("text report does not fold:\n%s", text)
	}
}

