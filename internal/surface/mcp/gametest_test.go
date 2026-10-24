package mcp_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestTestRunToolReport(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	e.MustRun("scan")
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	e.App.TestGameStart = func(cmd *exec.Cmd) error {
		var path string
		for i, arg := range cmd.Args {
			if arg == "-logfile" {
				path = cmd.Args[i+1]
			}
		}
		cmd.Env = append(os.Environ(), "ARCHO_MCP_TEST_LOG="+path)
		cmd.Path, cmd.Args = exe, []string{exe, "-test.run=^TestMCPGameHelper$"}
		return cmd.Start()
	}
	cs := connect(t, e)
	out, isErr := call(t, cs, "test_run", map[string]any{"env": "main", "timeout_seconds": 5})
	var r app.TestRunReport
	if !isErr || json.Unmarshal([]byte(out), &r) != nil || r.Status != "exited" || r.Requested != 1 {
		t.Fatalf("isError=%v, output: %s", isErr, out)
	}
	if len(r.Clusters) != 30 || r.ClustersTotal != 35 || len(out) < mcp.MaxOutput {
		t.Fatalf("cluster limit or JSON output: %d/%d, %d bytes", len(r.Clusters), r.ClustersTotal, len(out))
	}
	saved, err := os.ReadFile(filepath.Join(r.RunDir, "report.json"))
	if err != nil || string(saved) != out {
		t.Fatalf("tool result differs from report.json: %v", err)
	}
	_, isErr = call(t, cs, "test_run", map[string]any{"timeout_seconds": -1})
	if !isErr {
		t.Fatal("negative timeout accepted")
	}
}

func TestMCPGameHelper(t *testing.T) {
	path := os.Getenv("ARCHO_MCP_TEST_LOG")
	if path == "" {
		return
	}
	var text strings.Builder
	text.WriteString("RimWorld 1.6.4633 rev1261\n")
	for i := range 35 {
		word := fmt.Sprintf("%c%c", 'A'+i/26, 'A'+i%26)
		fmt.Fprintf(&text, "Exception: %s\n", strings.Repeat(word, 1000))
	}
	if err := os.WriteFile(path, []byte(text.String()), 0o644); err != nil {
		os.Exit(98)
	}
	os.Exit(7)
}
