package app_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
)

func readyPackEnv(t *testing.T) *app.App {
	e := packEnv(t)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("enable", "pkg:tester.versions")
	e.MustRun("save")
	return e.App
}

// The build reports its steps in order, the copy's bytes add up to what was measured, and
// nothing of the build folder is left.
func TestPackBuildProgress(t *testing.T) {
	a := readyPackEnv(t)
	dry, err := a.BuildPack("first", false, true)
	if err != nil || dry.Need == 0 || dry.Free == 0 {
		t.Fatalf("dry run: %+v %v", dry, err)
	}
	var steps []string
	var last app.BuildProgress
	sawFile := false
	r, err := a.BuildPackWith("first", app.BuildOptions{Progress: func(p app.BuildProgress) {
		if len(steps) == 0 || steps[len(steps)-1] != p.Step {
			steps = append(steps, p.Step)
		}
		if p.Step == app.StepCopy {
			// The state file is there while copying, and says the build runs.
			b := app.ReadBuildProgress(filepath.Join(a.PacksDir(), "first"+app.TmpSuffix))
			sawFile = sawFile || (b != nil && b.Running() && b.Name == "first")
		}
		last = p
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(steps, ","); got != "check,measure,copy,write,done" {
		t.Fatalf("steps %s", got)
	}
	if !sawFile {
		t.Fatal("no running build.json while copying")
	}
	if last.Bytes != r.Need || last.Mods != last.ModsTotal || last.ModsTotal == 0 {
		t.Fatalf("progress at the end: %+v, need %d", last, r.Need)
	}
	if _, err := os.Stat(filepath.Join(a.PacksDir(), "first"+app.TmpSuffix)); err == nil {
		t.Fatal("the build folder is left")
	}
	if _, err := os.Stat(filepath.Join(r.Dir, app.BuildFile)); err == nil {
		t.Fatal("build.json is in the pack")
	}
	if un, _ := a.UnfinishedPacks(); len(un) != 0 {
		t.Fatalf("unfinished after a build: %+v", un)
	}
	ps, _ := a.Packs()
	if len(ps) != 1 || ps[0].Bytes != r.Need {
		t.Fatalf("packs: %+v", ps)
	}
}

// Asked to stop, a build removes what it copied; with too little space it does not start.
func TestPackBuildStopAndSpace(t *testing.T) {
	a := readyPackEnv(t)
	tmp := filepath.Join(a.PacksDir(), "first"+app.TmpSuffix)
	_, err := a.BuildPackWith("first", app.BuildOptions{Progress: func(p app.BuildProgress) {
		if p.Step == app.StepCopy {
			if err := a.StopBuild("first"); err != nil {
				t.Errorf("stop: %v", err)
			}
		}
	}})
	if err == nil || !strings.Contains(err.Error(), "stopped") {
		t.Fatalf("stopped build: %v", err)
	}
	if _, err := os.Stat(tmp); err == nil {
		t.Fatal("a stopped build left its folder")
	}

	a.FreeSpace = func(string) int64 { return 1 << 20 }
	if _, err := a.BuildPack("first", false, false); !errors.Is(err, app.ErrNoSpace) {
		t.Fatalf("no space: %v", err)
	}
	if _, err := os.Stat(tmp); err == nil {
		t.Fatal("a refused build left its folder")
	}
	if r, err := a.BuildPack("first", false, true); err != nil || r.Free != 1<<20 {
		t.Fatalf("dry run with no space: %+v %v", r, err)
	}
}

func writeBuild(t *testing.T, tmp string, p app.BuildProgress) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(tmp, "mods", "A"), 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(p)
	if err := os.WriteFile(filepath.Join(tmp, app.BuildFile), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A build cut short (its process ended) is reported, can be removed or built again; a
// running one (fresh heartbeat) blocks another build of the name and its removal.
func TestPackUnfinished(t *testing.T) {
	a := readyPackEnv(t)
	dir := a.PacksDir()
	stale := filepath.Join(dir, "old"+app.TmpSuffix)
	writeBuild(t, stale, app.BuildProgress{Name: "old", Step: app.StepCopy, Mods: 3, ModsTotal: 9, Beat: time.Now().Add(-time.Hour)})
	bare := filepath.Join(dir, "bare"+app.TmpSuffix)
	os.MkdirAll(bare, 0o755)
	live := filepath.Join(dir, "live"+app.TmpSuffix)
	writeBuild(t, live, app.BuildProgress{Name: "live", Step: app.StepCopy, Beat: time.Now()})

	un, err := a.UnfinishedPacks()
	if err != nil || len(un) != 3 {
		t.Fatalf("unfinished: %+v %v", un, err)
	}
	got := map[string]*app.UnfinishedPack{}
	for _, u := range un {
		got[u.Name] = u
	}
	if got["old"].Running || got["old"].Progress == nil || got["old"].Progress.Mods != 3 {
		t.Fatalf("old: %+v", got["old"])
	}
	if got["bare"].Running || got["bare"].Progress != nil {
		t.Fatalf("bare: %+v", got["bare"])
	}
	if !got["live"].Running {
		t.Fatalf("live: %+v", got["live"])
	}
	if ps, _ := a.Packs(); len(ps) != 0 {
		t.Fatalf("build folders listed as packs: %+v", ps)
	}

	if _, err := a.BuildPack("live", false, false); !errors.Is(err, app.ErrBuildRunning) {
		t.Fatalf("build over a running build: %v", err)
	}
	if err := a.RemoveUnfinished("live"); !errors.Is(err, app.ErrBuildRunning) {
		t.Fatalf("remove a running build: %v", err)
	}
	if err := a.RemoveUnfinished("bare"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bare); err == nil {
		t.Fatal("bare is left")
	}
	if err := a.RemoveUnfinished("../x"); err == nil {
		t.Fatal("a name with a path was accepted")
	}
	// Built again: the leftover is replaced by the pack.
	if _, err := a.BuildPack("old", false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Fatal("the leftover is still there after building again")
	}
	if _, err := a.ReadPack("old"); err != nil {
		t.Fatal(err)
	}
	if err := a.StopBuild("old"); err == nil {
		t.Fatal("stopping a build that does not run")
	}
}
