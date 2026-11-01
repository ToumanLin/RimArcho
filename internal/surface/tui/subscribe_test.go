package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// drain runs a command the model returned, feeds its message back and follows the commands
// that come of it, until nothing is left (no program in tests). A waitTask blocks until
// the background task ends, so a drained queue means the tasks are done.
func drain(m *model, c tea.Cmd) {
	for c != nil {
		switch msg := c().(type) {
		case nil:
			return
		case tea.BatchMsg:
			for _, x := range msg {
				drain(m, x)
			}
			return
		default:
			_, c = m.update(msg)
		}
	}
}

// key sends one key and drains what it starts.
func key(m *model, k tea.KeyMsg) {
	_, c := m.update(k)
	drain(m, c)
}

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func subscriptionEnv(t *testing.T) (*testkit.Env, *testkit.GitRepo) {
	e := testkit.New(t)
	e.AddEnv(false)
	e.MustRun("scan")
	g := testkit.NewGitRepo(t, "")
	recycled := filepath.Join(e.Root, "recycled")
	e.App.TrashFunc = func(path string) error { return os.Rename(path, recycled) }
	return e, g
}

// addText is what add a subscription shows.
func addText(m *model) string { return strings.Join(m.addLines(40), "\n") }

// tasksOnRecord reads <data>/tasks.json, where the window's Background tasks page finds them.
func tasksOnRecord(t *testing.T, e *testkit.Env) []app.Task {
	b, err := os.ReadFile(filepath.Join(e.Home, "tasks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var list []app.Task
	if err := json.Unmarshal(b, &list); err != nil {
		t.Fatal(err)
	}
	return list
}

// 06 §2.9, §9.1: add a subscription recognises a pasted address as the window does, asks the one
// decision that source needs, shows the dry run, and enter queues subscribe as a background task
// (06 §2.16) that installs the mod.
func TestAddSubscriptionPage(t *testing.T) {
	e, g := subscriptionEnv(t)
	const url = "https://github.com/Example/colony"
	m := direct(t, e, 120, 30)
	m.tasks.runSub = func(a *app.App, argv []string) (string, int) {
		for i, w := range argv {
			if w == url {
				argv[i] = g.Bare // the same repository, without the network
			}
		}
		return cmds.Execute(a, argv)
	}
	press(m, "c")
	key(m, runes("+"))
	if m.tab != tabSubscribe || !strings.Contains(addText(m), "Git repository") {
		t.Fatalf("+ did not open Add a subscription: tab %d\n%s", m.tab, addText(m))
	}
	key(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("not an address"), Paste: true})
	if !strings.Contains(addText(m), "Not recognised") {
		t.Fatalf("bad text:\n%s", addText(m))
	}
	key(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.tab != tabSubscribe || m.add.text != "" {
		t.Fatalf("esc did not clear the box first: tab %d %q", m.tab, m.add.text)
	}
	// typing goes into the box: q and ? are letters here
	key(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(url), Paste: true})
	if m.add.kind != "git" || len(m.add.options()) != 4 || !m.add.apply || !strings.Contains(strings.Join(m.add.lines, "\n"), "dry run, nothing changed") {
		t.Fatalf("git card: %+v\n%s", m.add, addText(m))
	}
	if _, err := os.Stat(e.App.SubscribedDir()); !os.IsNotExist(err) {
		t.Fatal("the dry run wrote files")
	}
	// a branch needs its name: enter asks for it, the name field takes the letters
	key(m, tea.KeyMsg{Type: tea.KeyDown})
	if !m.add.needsName() || m.add.field != 1 || m.add.apply {
		t.Fatalf("branch: %+v", m.add)
	}
	key(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.tasks.ts != nil && len(m.tasks.ts.List()) > 0 {
		t.Fatal("enter queued an incomplete subscription")
	}
	key(m, runes("m"))
	key(m, runes("a"))
	key(m, runes("i"))
	key(m, runes("n"))
	if m.add.ref != "main" || !m.add.apply || !strings.Contains(addText(m), "branch name: main") {
		t.Fatalf("branch name: %+v\n%s", m.add, addText(m))
	}
	key(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.add.text != "" {
		t.Fatal("the box was not cleared after subscribing")
	}
	ss, err := e.App.Subscriptions()
	if err != nil || len(ss) != 1 || ss[0].Record.RefType != "branch" || ss[0].Record.Ref != "main" || ss[0].Record.Installed != g.First {
		t.Fatalf("installed: %+v %v", ss, err)
	}
	if !strings.Contains(m.status, "colony") || m.sayKind != sayOK {
		t.Fatalf("status after the task: %d %q", m.sayKind, m.status)
	}
	if list := tasksOnRecord(t, e); len(list) != 1 || list[0].Kind != view.TaskSubscribe || list[0].State != app.TaskDone {
		t.Fatalf("record: %+v", list)
	}
	// esc on the empty box goes back to subscriptions and updates
	key(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.tab != tabChanges {
		t.Fatalf("esc on an empty box: tab %d", m.tab)
	}
	// a paste on subscriptions and updates opens add a subscription with it: Workshop IDs ask Steam or SteamCMD
	key(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2009463077 818773962"), Paste: true})
	if m.tab != tabSubscribe || m.add.kind != "workshop" || !strings.Contains(addText(m), "download with SteamCMD") {
		t.Fatalf("paste on Subscriptions and updates: tab %d %+v", m.tab, m.add)
	}
	// a download page without a single repository: the files are chosen in the window
	m.add = addState{text: "https://example.invalid/mods/colony", shown: "https://example.invalid/mods/colony", kind: "page", info: "Colony 1.2"}
	if r, ok := m.add.request(); ok || !strings.Contains(addText(m), "--from <zip|folder>") {
		t.Fatalf("page without a repository: %+v\n%s", r, addText(m))
	}
	m.add.main = "https://gitgud.io/Example/colony"
	if r, ok := m.add.request(); !ok || r.Kind != "page" || !r.Git {
		t.Fatalf("page with a repository: %+v", r)
	}
}

// 06 §2.9: on subscriptions and updates, K checks every source with mods, u updates a waiting one (its dry
// run first; refused while a game runs), Del removes a subscription — each a background task.
func TestSubscriptionsPageActions(t *testing.T) {
	e, g := subscriptionEnv(t)
	e.MustRun("subscribe", "git", g.Bare, "--branch", "main")
	running := map[string]*app.GameRun{}
	e.App.GameProbe = func(root string) *app.GameRun { return running[env.PathKey(root)] }
	m := direct(t, e, 120, 30)
	press(m, "c")
	if m.tab != tabChanges {
		t.Fatalf("c: tab %d", m.tab)
	}
	next := g.Commit(t, "Defs/Example.xml", "<Defs/>")
	key(m, runes("K"))
	pending := func() int {
		for i, r := range m.chg.rows {
			if r.kind == chPending {
				return i
			}
		}
		return -1
	}
	at := pending()
	if at < 0 || !strings.Contains(m.chg.rows[at].text, next[:7]) {
		t.Fatalf("K found no update: %s (status %q)", chText(m), m.status)
	}
	if !strings.Contains(m.status, "check for updates") {
		t.Fatalf("status after the check: %q", m.status)
	}
	m.chg.cursor = at
	hints := m.hintLine()
	for _, want := range []string{"u ", "Del ", "K ", "+ "} {
		if !strings.Contains(hints, want) {
			t.Fatalf("hints miss %q: %s", want, hints)
		}
	}

	// refused while a game runs: the dry run says so and enter does nothing
	running[env.PathKey(e.Game)] = &app.GameRun{PID: 7, Exe: filepath.Join(e.Game, "RimWorldWin64.exe"), Started: e.Now().Add(-time.Minute), ArgsKnown: true}
	key(m, runes("u"))
	if m.panel == nil || m.panel.onEnter != nil || !strings.Contains(strings.Join(m.panel.lines, "\n"), "the game is running") {
		t.Fatalf("update while the game runs: %+v", m.panel)
	}
	key(m, tea.KeyMsg{Type: tea.KeyEsc})
	delete(running, env.PathKey(e.Game))

	key(m, runes("u"))
	if m.panel == nil || m.panel.onEnter == nil || !strings.Contains(strings.Join(m.panel.lines, "\n"), next) {
		t.Fatalf("update preview: %+v", m.panel)
	}
	ss, _ := e.App.Subscriptions()
	if ss[0].Record.Installed != g.First {
		t.Fatal("the dry run updated")
	}
	key(m, tea.KeyMsg{Type: tea.KeyEnter})
	ss, err := e.App.Subscriptions()
	if err != nil || len(ss) != 1 || ss[0].Record.Installed != next {
		t.Fatalf("updated: %+v %v (status %q)", ss, err, m.status)
	}
	if pending() >= 0 {
		t.Fatalf("still waiting after the update: %s", chText(m))
	}

	// Del on a waiting update removes the subscription (the dry run first)
	g.Commit(t, "Defs/More.xml", "<Defs/>")
	key(m, runes("K"))
	if at = pending(); at < 0 {
		t.Fatalf("second check found nothing: %s", chText(m))
	}
	m.chg.cursor = at
	key(m, tea.KeyMsg{Type: tea.KeyDelete})
	if m.panel == nil || m.panel.onEnter == nil || !strings.Contains(strings.Join(m.panel.lines, "\n"), "dry run, nothing changed") {
		t.Fatalf("remove preview: %+v", m.panel)
	}
	if ss, _ := e.App.Subscriptions(); len(ss) != 1 {
		t.Fatal("the dry run removed")
	}
	key(m, tea.KeyMsg{Type: tea.KeyEnter})
	if ss, err := e.App.Subscriptions(); err != nil || len(ss) != 0 {
		t.Fatalf("removed: %+v %v (status %q)", ss, err, m.status)
	}
	if _, err := os.Stat(filepath.Join(e.Root, "recycled")); err != nil {
		t.Fatal("the folder did not go to the recycle bin")
	}
	kinds := map[string]string{}
	for _, task := range tasksOnRecord(t, e) {
		kinds[task.Kind] = task.State
	}
	for _, k := range []string{view.TaskSourceCheck, view.OpSubUpdate, view.OpSubRemove} {
		if kinds[k] != app.TaskDone {
			t.Errorf("task %s on the record: %q (%v)", k, kinds[k], kinds)
		}
	}
}

func chText(m *model) string {
	var out []string
	for _, r := range m.chg.rows {
		out = append(out, r.text)
	}
	return strings.Join(out, "\n")
}
