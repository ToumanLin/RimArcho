package view_test

import (
	"encoding/json"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

var agentCommandRe = regexp.MustCompile(`\bagent (status|install|key|run|remove)\b`)

// agentVisible lists where Archo Agent shows to a player or an agent (15 §7): the settings
// page (every group, item, note, action and confirmation, en and zh), the GUI's settings
// commands and tasks, the command table and help that the CLI, TUI, command palette and MCP
// share, and the settings keys that `config get`, completion and MCP list.
func agentVisible(t *testing.T, e *testkit.Env) []string {
	t.Helper()
	var out []string
	for _, lang := range []string{"en", "zh"} {
		view.SetLang(lang)
		for _, g := range view.SettingSections(view.SettingsPage(e.App, nil)) {
			b, err := json.Marshal(g)
			if err != nil {
				t.Fatal(err)
			}
			if s := string(b); strings.Contains(s, "Archo Agent") || strings.Contains(s, `"agent"`) || strings.Contains(s, "agent.") {
				out = append(out, lang+" settings group "+g.Title)
			}
		}
	}
	view.SetLang("en")
	for _, argv := range [][]string{{"agent", "install"}, {"agent", "remove"}, {"agent", "run", "--window"}} {
		if view.SettingCommand(argv) == nil {
			out = append(out, "settings command "+strings.Join(argv, " "))
		}
	}
	if view.SettingTask([]string{"agent", "install"}) != "" {
		out = append(out, "settings task agent install")
	}
	for _, c := range cmds.Commands() {
		if strings.HasPrefix(c.Name, "agent") || strings.Contains(c.Summary, "agent.") {
			out = append(out, "command "+c.Name)
		}
	}
	if c, _ := cmds.Find([]string{"agent", "status"}); c != nil {
		out = append(out, "cmds.Find agent status")
	}
	if _, code := e.Run("agent", "status"); code == cmds.ExitOK {
		out = append(out, "archo agent status runs")
	}
	if _, code := e.Run("config", "set", "agent.model", "deepseek-chat"); code == cmds.ExitOK {
		out = append(out, "config set agent.model")
	}
	for _, k := range cmds.SettingKeys() {
		if strings.HasPrefix(k, "agent.") {
			out = append(out, "settings key "+k)
		}
	}
	// "agent" alone also names outside MCP clients (guide, mcp); the Agent's own commands are these
	if s := e.MustRun("help"); agentCommandRe.MatchString(s) || strings.Contains(s, "agent.") {
		out = append(out, "archo help")
	}
	if s := e.MustRun("config", "get"); strings.Contains(s, "agent.") {
		out = append(out, "config get")
	}
	return out
}

// 15 §7: a build without Archo Agent (buildinfo.Agent unset, as 1.1 ships) shows it nowhere;
// with the switch on, each of those places shows it again.
func TestSpecAgentOffBuild(t *testing.T) {
	t.Setenv("ARCHO_LANG", "")
	t.Cleanup(func() { view.SetLang("en") })
	if buildinfo.AgentOn() {
		t.Fatal("this build offers Archo Agent; 1.1 ships without it (buildinfo.Agent, 15 §7)")
	}
	e := testkit.New(t)
	// what an earlier development build with the Agent left: its settings stay, unused and unshown
	for k, v := range map[string]string{"agent.model": "deepseek-chat", "agent.base_url": "https://api.deepseek.com/v1"} {
		if err := e.App.Store.SetMeta("cfg:"+k, v); err != nil {
			t.Fatal(err)
		}
	}
	if got := agentVisible(t, e); len(got) > 0 {
		t.Fatalf("Archo Agent is visible in a build without it (15 §7, buildinfo.Agent):\n%s", strings.Join(got, "\n"))
	}

	// Not parallel: the switch is a package variable.
	buildinfo.Agent = "1"
	t.Cleanup(func() { buildinfo.Agent = "" })
	got := agentVisible(t, e)
	want := []string{"settings command agent install", "settings command agent remove", "settings command agent run --window",
		"settings task agent install", "command agent status", "command agent install", "command agent key", "command agent run",
		"command agent remove", "cmds.Find agent status", "archo agent status runs", "config set agent.model",
		"settings key agent.api", "settings key agent.base_url", "settings key agent.model", "settings key agent.thinking", "archo help", "config get"}
	if runtime.GOOS == "windows" {
		want = append([]string{"en settings group Archo Agent", "zh settings group Archo Agent"}, want...)
	}
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("with buildinfo.Agent=1, %q is missing; got %q", w, got)
		}
	}
}
