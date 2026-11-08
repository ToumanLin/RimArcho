package cmds_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// `archo scan --progress` writes its progress to Progress (stderr), never to its output (16 §8.4).
func TestScanWritesProgress(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(true)
	var lines []string
	old := cmds.Progress
	cmds.Progress = func(l string) { lines = append(lines, l) } // scanProgress serialises calls
	defer func() { cmds.Progress = old }()
	e.MustRun("scan")
	if len(lines) != 0 {
		t.Fatalf("progress without --progress: %q", lines)
	}
	out := e.MustRun("scan", "--progress", "--full")
	if strings.Contains(out, "progress:") {
		t.Fatalf("progress in the output:\n%s", out)
	}
	if len(lines) < 2 {
		t.Fatalf("progress = %q", lines)
	}
	first, ok := app.ParseScanProgress(lines[0])
	if !ok || first.Saving || first.Done != 0 || first.Total == 0 {
		t.Fatalf("first = %q", lines[0])
	}
	last, ok := app.ParseScanProgress(lines[len(lines)-1])
	if !ok || !last.Saving || last.Total != first.Total {
		t.Fatalf("last = %q", lines[len(lines)-1])
	}
	if done, _ := app.ParseScanProgress(lines[len(lines)-2]); done.Done != first.Total {
		t.Fatalf("the last folder is not reported: %q", lines)
	}
}
