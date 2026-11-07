package mcp_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// subSide is one of the two identical fixtures: an environment and a local Git upstream.
type subSide struct {
	e *testkit.Env
	g *testkit.GitRepo
}

func subscriptionSide(t *testing.T) subSide {
	e := testkit.New(t)
	e.AddEnv(false)
	g := testkit.NewGitRepo(t, "")
	recycled := filepath.Join(e.Root, "recycled")
	e.App.TrashFunc = func(path string) error { return os.Rename(path, recycled) }
	return subSide{e, g}
}

func (s subSide) scrub(text string) string {
	return s.e.Scrub(strings.ReplaceAll(text, s.g.Bare, "$GIT"))
}

// 11 §1, 13 §1.2: manage_subscriptions runs every action through MCP on one fixture and
// the matching archo subscribe command on an identical one. The texts must be the same
// (I1), and the MCP transcript is compared with testdata/golden/mcp_subscriptions.txt.
func TestManageSubscriptions(t *testing.T) {
	sm, sc := subscriptionSide(t), subscriptionSide(t)
	cs := connect(t, sm.e)
	subRef := func(s subSide) string {
		ss, err := s.e.App.Subscriptions()
		if err != nil || len(ss) != 1 {
			t.Fatalf("subscriptions: %+v %v", ss, err)
		}
		return ss[0].Ref
	}
	type step struct {
		before func(s subSide) // done to both fixtures first
		args   func(s subSide) map[string]any
		cli    func(s subSide) []string
	}
	fixed := func(args map[string]any, cli ...string) step {
		return step{args: func(subSide) map[string]any { return args }, cli: func(subSide) []string { return cli }}
	}
	steps := []step{
		fixed(nil, "subscribe", "list"),
		{args: func(s subSide) map[string]any {
			return map[string]any{"action": "add", "kind": "git", "url": s.g.Bare, "dry_run": true}
		}, cli: func(s subSide) []string { return []string{"subscribe", "git", s.g.Bare, "--dry-run"} }},
		{args: func(s subSide) map[string]any {
			return map[string]any{"action": "add", "kind": "git", "url": s.g.Bare, "branch": "main"}
		}, cli: func(s subSide) []string { return []string{"subscribe", "git", s.g.Bare, "--branch", "main"} }},
		fixed(map[string]any{"action": "list"}, "subscribe", "list"),
		fixed(map[string]any{"action": "check"}, "subscribe", "check"),
		{before: func(s subSide) { s.g.Commit(t, "Defs/Example.xml", "<Defs/>") },
			args: func(s subSide) map[string]any { return map[string]any{"action": "check", "refs": []string{subRef(s)}} },
			cli:  func(s subSide) []string { return []string{"subscribe", "check", subRef(s)} }},
		{args: func(s subSide) map[string]any {
			return map[string]any{"action": "update", "refs": []string{subRef(s)}, "dry_run": true}
		}, cli: func(s subSide) []string { return []string{"subscribe", "update", subRef(s), "--dry-run"} }},
		{args: func(s subSide) map[string]any { return map[string]any{"action": "update", "refs": []string{subRef(s)}} },
			cli: func(s subSide) []string { return []string{"subscribe", "update", subRef(s)} }},
		fixed(map[string]any{"action": "update"}, "subscribe", "update"), // neither refs nor all: refused like the CLI
		fixed(map[string]any{"action": "list"}, "subscribe", "list"),
		{args: func(s subSide) map[string]any {
			return map[string]any{"action": "remove", "refs": []string{subRef(s)}, "dry_run": true}
		}, cli: func(s subSide) []string { return []string{"subscribe", "remove", subRef(s), "--dry-run"} }},
		{args: func(s subSide) map[string]any { return map[string]any{"action": "remove", "refs": []string{subRef(s)}} },
			cli: func(s subSide) []string { return []string{"subscribe", "remove", subRef(s)} }},
		fixed(map[string]any{"action": "list"}, "subscribe", "list"),
		fixed(map[string]any{"action": "add", "url": "not an address"}, mcp.SubscriptionsArgv(mcp.ManageSubscriptionsIn{Action: "add", URL: "not an address"})...),
	}
	var sb strings.Builder
	for _, st := range steps {
		if st.before != nil {
			st.before(sm)
			st.before(sc)
		}
		args := st.args(sm)
		got, isErr := call(t, cs, "manage_subscriptions", args)
		cli := st.cli(sc)
		want, code := sc.e.Run(cli...)
		if sm.scrub(got) != sc.scrub(want) {
			t.Errorf("%v: MCP text differs from `archo %s`:\n--- mcp\n%s\n--- cli\n%s", args, strings.Join(cli, " "), got, want)
		}
		if isErr != (code == cmds.ExitError || code == cmds.ExitUsage) {
			t.Errorf("%v: isError=%v, exit %d", args, isErr, code)
		}
		shown := map[string]any{}
		for k, v := range args {
			if s, ok := v.(string); ok {
				v = sm.scrub(s)
			}
			shown[k] = v
		}
		b, _ := json.Marshal(shown)
		fmt.Fprintf(&sb, "> manage_subscriptions %s\n%s", b, sm.scrub(got))
		if isErr {
			sb.WriteString("[isError]\n")
		}
		sb.WriteString("\n")
	}
	if ss, _ := sm.e.App.Subscriptions(); len(ss) != 0 {
		t.Fatalf("remove left %+v", ss)
	}
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "golden", "mcp_subscriptions.txt")
	if *update {
		if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/surface/mcp -update)", err)
	}
	if want := strings.ReplaceAll(string(b), "\r\n", "\n"); sb.String() != want {
		t.Errorf("mcp_subscriptions.txt differs from golden (go test ./internal/surface/mcp -update):\n%s", sb.String())
	}
}

// The argv of each add form: the source recognised from url as the paste box does (16 §2.9),
// or the kind given.
func TestManageSubscriptionsArgv(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		in   mcp.ManageSubscriptionsIn
		want []string
	}{
		{mcp.ManageSubscriptionsIn{}, []string{"subscribe", "list"}},
		{mcp.ManageSubscriptionsIn{Action: "add", URL: "https://github.com/Example/colony", Tag: "v1", DryRun: true}, []string{"subscribe", "git", "https://github.com/Example/colony", "--tag", "v1", "--dry-run"}},
		{mcp.ManageSubscriptionsIn{Action: "add", URL: "https://www.loverslab.com/files/file/20511-gene-tweaks/", From: "C:/Downloads/gene.zip"}, []string{"subscribe", "page", "https://www.loverslab.com/files/file/20511-gene-tweaks/", "--from", "C:/Downloads/gene.zip"}},
		{mcp.ManageSubscriptionsIn{Action: "add", URL: "https://example.invalid/mods/colony", Git: true}, []string{"subscribe", "page", "https://example.invalid/mods/colony", "--git"}},
		{mcp.ManageSubscriptionsIn{Action: "add", URL: "2009463077 818773962", SteamCMD: true, DryRun: true}, []string{"subscribe", "workshop", "2009463077", "818773962", "--steamcmd", "--dry-run"}},
		{mcp.ManageSubscriptionsIn{Action: "add", Kind: "git", URL: "D:/repos/mod.git", Commit: "abc"}, []string{"subscribe", "git", "D:/repos/mod.git", "--commit", "abc"}},
		{mcp.ManageSubscriptionsIn{Action: "check", Refs: []string{"#a1b2"}}, []string{"subscribe", "check", "#a1b2"}},
		{mcp.ManageSubscriptionsIn{Action: "update", All: true, DryRun: true}, []string{"subscribe", "update", "--all", "--dry-run"}},
		{mcp.ManageSubscriptionsIn{Action: "update", Refs: []string{"#a1b2"}, From: "new.zip"}, []string{"subscribe", "update", "#a1b2", "--from", "new.zip"}},
		{mcp.ManageSubscriptionsIn{Action: "remove", Refs: []string{"#a1b2", "#c3d4"}, DryRun: true}, []string{"subscribe", "remove", "#a1b2", "#c3d4", "--dry-run"}},
	} {
		if got := mcp.SubscriptionsArgv(c.in); !slices.Equal(got, c.want) {
			t.Errorf("%+v: %q, want %q", c.in, got, c.want)
		}
	}
	if got := mcp.SubscriptionsArgv(mcp.ManageSubscriptionsIn{Action: "add", URL: "not an address"}); len(got) != 1 || !strings.Contains(got[0], "not recognised") {
		t.Errorf("unrecognised url: %q", got)
	}
}
