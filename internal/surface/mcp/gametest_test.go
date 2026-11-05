package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestTestRunToolReport(t *testing.T) {
	t.Parallel()
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
	// report.json also keeps the launch arguments, for a later run's baseline.
	saved, err := os.ReadFile(filepath.Join(r.RunDir, "report.json"))
	var stored app.TestRunReport
	if err == nil {
		err = json.Unmarshal(saved, &stored)
	}
	stored.Args = nil
	if b, _ := json.MarshalIndent(&stored, "", "  "); err != nil || string(b)+"\n" != out {
		t.Fatalf("tool result differs from report.json: %v", err)
	}
	out, isErr = call(t, cs, "test_run", map[string]any{"env": "main", "timeout_seconds": 5})
	var repeated app.TestRunReport
	if !isErr || json.Unmarshal([]byte(out), &repeated) != nil || repeated.Baseline == nil ||
		repeated.Baseline.RunDir != r.RunDir || repeated.Baseline.New != 0 || repeated.Baseline.Present != 35 || repeated.Baseline.Gone != 0 {
		t.Fatalf("unchanged faulty run must retain isError and count every cluster: %s", out)
	}
	for _, c := range repeated.Clusters {
		if c.Delta != "present" {
			t.Fatalf("repeated cluster: %+v", c)
		}
	}
	_, isErr = call(t, cs, "test_run", map[string]any{"timeout_seconds": -1})
	if !isErr {
		t.Fatal("negative timeout accepted")
	}
}

// TestTestRunToolProgress: a client that asks for progress gets a notification every
// ProgressEvery while the game runs, naming the step, and none after the result; this is
// what keeps Pi from cancelling a test run after its 60 s request timeout (05 §5.1).
func TestTestRunToolProgress(t *testing.T) {
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
		cmd.Env = append(os.Environ(), "ARCHO_MCP_TEST_LOG="+path, "ARCHO_MCP_TEST_SLEEP=600ms")
		cmd.Path, cmd.Args = exe, []string{exe, "-test.run=^TestMCPGameHelper$"}
		return cmd.Start()
	}
	old := mcp.ProgressEvery
	mcp.ProgressEvery = 50 * time.Millisecond
	defer func() { mcp.ProgressEvery = old }()

	var mu sync.Mutex
	var got []*sdk.ProgressNotificationParams
	ctx := context.Background()
	ct, st := sdk.NewInMemoryTransports()
	if _, err := mcp.New(e.App, mcp.Core).SDK().Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	c := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, &sdk.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *sdk.ProgressNotificationClientRequest) {
			mu.Lock()
			got = append(got, req.Params)
			mu.Unlock()
		}})
	cs, err := c.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	count := func() int { mu.Lock(); defer mu.Unlock(); return len(got) }

	params := &sdk.CallToolParams{Name: "test_run", Arguments: map[string]any{"env": "main", "timeout_seconds": 5}}
	params.SetProgressToken("run-1")
	res, err := cs.CallTool(ctx, params)
	if err != nil || len(res.Content) == 0 {
		t.Fatalf("test_run: %v", err)
	}
	var r app.TestRunReport
	if text := res.Content[0].(*sdk.TextContent).Text; json.Unmarshal([]byte(text), &r) != nil || r.Status != "exited" {
		t.Fatalf("report: %s", text)
	}
	after := count()
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	all := append([]*sdk.ProgressNotificationParams(nil), got...)
	mu.Unlock()
	if len(all) < 3 || len(all) != after {
		t.Fatalf("%d notifications during the run, %d in all", after, len(all))
	}
	waiting := false
	for i, p := range all {
		if p.ProgressToken != "run-1" || p.Progress != float64(i+1) || !strings.HasSuffix(p.Message, "s)") {
			t.Fatalf("notification %d: %+v", i, p)
		}
		waiting = waiting || strings.HasPrefix(p.Message, "starting test run; waiting for the game to load (")
	}
	if !waiting {
		t.Fatalf("no notification names the step: %+v", all[0])
	}

	// Without a progress token the call sends nothing.
	if _, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "test_run", Arguments: map[string]any{"env": "main", "timeout_seconds": 5}}); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != after {
		t.Fatalf("a call without a progress token sent %d notifications", n-after)
	}
}

func TestMCPGameHelper(t *testing.T) {
	path := os.Getenv("ARCHO_MCP_TEST_LOG")
	if path == "" {
		return
	}
	if d, err := time.ParseDuration(os.Getenv("ARCHO_MCP_TEST_SLEEP")); err == nil {
		time.Sleep(d)
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
