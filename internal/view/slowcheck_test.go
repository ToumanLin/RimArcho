package view_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/feedback"
	"github.com/ToumanLin/RimArcho/internal/oplog"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// a stub relay: what /timings received
type timingsRelay struct {
	mu     sync.Mutex
	bodies []string
}

func stubRelay(t *testing.T, route bool) *timingsRelay {
	r := &timingsRelay{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		if q.URL.Path != "/timings" || !route {
			w.WriteHeader(404)
			return
		}
		b, _ := io.ReadAll(q.Body)
		r.mu.Lock()
		r.bodies = append(r.bodies, string(b))
		r.mu.Unlock()
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("RIMARCHO_FEEDBACK_RELAY", srv.URL+"/report")
	return r
}

func (r *timingsRelay) sent() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.bodies...)
}

// slowEnv is a home whose last check took 40 s, from origin.
func slowEnv(t *testing.T, origin string) (*testkit.Env, time.Time) {
	e := testkit.New(t)
	e.AddEnv(false)
	e.App.Ops.SetOrigin(origin)
	began := e.App.Now()
	p := app.CheckPhases{Game: "1.6", Active: 300, Total: 1300}
	p.Add("read", 900)
	p.Add("defs", 20000)
	p.Add("patches", 19000)
	h := e.App.Ops.Begin(oplog.Row{Op: oplog.Check, Subject: "My secret plan"})
	h.End(oplog.OK, p.Detail())
	return e, began
}

// The prompt: only over the threshold, only for the window and the terminal, once a day,
// and its preview is exactly what goes, with no names (16 §2.14 "Slow checks").
func TestSlowCheckPrompt(t *testing.T) {
	relay := stubRelay(t, true)
	e, began := slowEnv(t, oplog.GUI)
	if p := view.SlowCheckOf(e.App, &app.CheckClock{Began: began}, 10*time.Second); p != nil {
		t.Fatal("prompt for a 10 s check")
	}
	p := view.SlowCheckOf(e.App, &app.CheckClock{Began: began}, 48*time.Second)
	if p == nil || p.Took != "48 s" || !strings.Contains(p.Text, "48 s") {
		t.Fatalf("prompt %+v", p)
	}
	var tm map[string]any
	if err := json.Unmarshal([]byte(p.Preview), &tm); err != nil {
		t.Fatal(err)
	}
	if tm["active_mods"] != 300.0 || tm["total_mods"] != 1300.0 || tm["game"] != "1.6" || tm["wall_ms"] != 48000.0 {
		t.Fatalf("timings %s", p.Preview)
	}
	phases := tm["phases"].(map[string]any)
	if len(phases) != 3 && len(phases) != 4 || phases["defs"] != 20000.0 { // read, defs, patches, and other when the row's own time is known
		t.Fatalf("phases %v", phases)
	}
	if !strings.Contains(p.Preview, `"phases": {"read":900,"defs":20000,"patches":19000`) {
		t.Fatalf("phases not one compact object in order: %s", p.Preview)
	}
	for _, s := range []string{"My secret plan", e.Home, e.Game, "Users"} {
		if strings.Contains(p.Preview, s) {
			t.Fatalf("%q in the preview", s)
		}
	}
	if again := view.SlowCheckOf(e.App, &app.CheckClock{Began: began}, 48*time.Second); again != nil {
		t.Fatal("a second prompt within a day")
	}
	if err := view.SendSlowCheck(e.App, p.Preview); err != nil {
		t.Fatal(err)
	}
	if got := relay.sent(); len(got) != 1 || got[0] != p.Preview {
		t.Fatalf("sent %q", got)
	}
}

// A real check's report: the slowest operations name their Mod, Workshop id, class, XPath
// and file relative to the Mod; a repeat says so. No local path reaches the preview, even
// when one gets into the report (16 §2.14 "Slow checks").
func TestSlowCheckReportDetail(t *testing.T) {
	stubRelay(t, true)
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Mod{PackageID: "test.child", Name: "Child", Files: map[string]string{
		"Patches/Deep/A.xml": `<Patch><Operation Class="PatchOperationSequence"><operations><li Class="PatchOperationAdd"><xpath>Defs/ThingDef[defName="Target"]</xpath><value><label>x</label></value></li></operations></Operation></Patch>`,
	}})
	e.AddWorkshop("1002", testkit.Mod{PackageID: "test.target", Name: "Target Mod", Files: map[string]string{"Defs/Target.xml": `<Defs><ThingDef><defName>Target</defName></ThingDef></Defs>`}})
	e.WriteModsConfig("1.6", "ludeon.rimworld", "test.target", "test.child")
	e.AddEnv(false)
	e.MustRun("scan")
	e.App.Ops.SetOrigin(oplog.GUI)
	clock := &app.CheckClock{Began: e.App.Now().Add(-time.Second), First: true}
	e.MustRun("analyze")
	rows, _ := e.App.Ops.Recent(oplog.Query{Op: oplog.Check, Limit: 1})
	rep, ok := e.App.CheckReportOf(rows[0].ID)
	if !ok || rep.List != "new" || rep.Ops != 1 || rep.Nodes == 0 || len(rep.SlowestOps) != 1 {
		t.Fatalf("report %+v", rep)
	}
	op := rep.SlowestOps[0]
	if op.Pkg != "test.child" || op.WS != "1001" || op.Class != "PatchOperationSequence" ||
		op.XPath != `Defs/ThingDef[defName="Target"]` || op.File != "Patches/Deep/A.xml" {
		t.Fatalf("slowest %+v", op)
	}
	e.MustRun("analyze")
	rows, _ = e.App.Ops.Recent(oplog.Query{Op: oplog.Check, Limit: 1})
	if rep, ok = e.App.CheckReportOf(rows[0].ID); !ok || rep.List != "same" {
		t.Fatalf("repeat %+v", rep)
	}

	// paths on this computer in every field a Mod can fill
	home, _ := os.UserHomeDir()
	abs := filepath.Join(e.Workshop, "1001", "Patches", "A.xml")
	rep.SlowestOps = []app.CheckOp{
		{Us: 12_100_000, Pkg: "test.child", WS: "1001", Class: "PatchOperationReplace", XPath: `Defs/ThingDef[defName="` + e.Home + `"]`, File: abs},
		{Us: 900_000, Pkg: "test.child", Class: filepath.Join(home, "x"), XPath: "Defs/" + strings.Repeat("a", 400), File: "../../" + filepath.ToSlash(home) + "/B.xml"},
	}
	rep.SlowestDLLs = []app.CheckDLL{{Us: 800_000, Pkg: "test.child", File: filepath.Join(e.Game, "Mods", "x.dll")}}
	b, _ := json.Marshal(rep)
	if err := e.App.Store.SetMeta("check:report", string(b)); err != nil {
		t.Fatal(err)
	}
	p := view.SlowCheckOf(e.App, clock, 40*time.Second)
	if p == nil {
		t.Fatal("no prompt")
	}
	var tm feedback.Timings
	if err := json.Unmarshal([]byte(p.Preview), &tm); err != nil {
		t.Fatal(err, p.Preview)
	}
	assertNoLocalPath(t, p.Preview, e.Home, e.Workshop, e.Game, home)
	if tm.Run == nil || !tm.Run.Cold || tm.Run.List != "same" || tm.Patches == nil || len(tm.Patches.Slowest) != 2 || tm.Harmony == nil {
		t.Fatalf("timings %s", p.Preview)
	}
	if s := tm.Patches.Slowest[0]; s.Ms != 12100 || s.File != "A.xml" || s.XPath != "" || s.Pkg != "test.child" {
		t.Fatalf("first %+v", s)
	}
	if s := tm.Patches.Slowest[1]; len([]rune(s.XPath)) != feedback.XPathMax || s.File != "B.xml" || s.Class != "" {
		t.Fatalf("second %+v", s)
	}
}

// assertNoLocalPath fails when body names a drive, a user folder or any of dirs.
func assertNoLocalPath(t *testing.T, body string, dirs ...string) {
	t.Helper()
	if m := regexp.MustCompile(`(?i)(^|[^a-z])[a-z]:(\\\\|\\|/)|\\\\|[\\/](users|home)[\\/]`).FindString(body); m != "" {
		t.Fatalf("a local path (%q) in %s", m, body)
	}
	low := strings.ToLower(strings.ReplaceAll(body, `\\`, `\`))
	for _, d := range dirs {
		for _, form := range []string{d, filepath.ToSlash(d), filepath.Base(d)} {
			if len(form) > 3 && strings.Contains(low, strings.ToLower(form)) && form != "Mods" {
				t.Fatalf("%q in %s", form, body)
			}
		}
	}
	if u := os.Getenv("USERNAME") + os.Getenv("USER"); len(u) > 3 && strings.Contains(low, strings.ToLower(u)) {
		t.Fatalf("the user name in %s", body)
	}
}

func TestSlowCheckNeverForAgents(t *testing.T) {
	stubRelay(t, true)
	for _, o := range []string{oplog.MCP, oplog.Agent, oplog.CLI} {
		e, began := slowEnv(t, o)
		if p := view.SlowCheckOf(e.App, &app.CheckClock{Began: began}, time.Minute); p != nil {
			t.Fatalf("%s: prompt", o)
		}
	}
	e, began := slowEnv(t, oplog.TUI)
	if p := view.SlowCheckOf(e.App, &app.CheckClock{Began: began}, time.Minute); p == nil {
		t.Fatal("tui: no prompt")
	}
}

// off asks nothing; always sends without a prompt; a relay without the route is quiet.
func TestSlowCheckSettings(t *testing.T) {
	relay := stubRelay(t, true)
	e, began := slowEnv(t, oplog.GUI)
	e.MustRun("config", "set", "feedback.timings", "off")
	if p := view.SlowCheckOf(e.App, &app.CheckClock{Began: began}, time.Minute); p != nil {
		t.Fatal("prompt with off")
	}
	if err := view.AlwaysSendSlowChecks(e.App); err != nil || cmds.Setting(e.App, "feedback.timings") != "always" {
		t.Fatal(err, cmds.Setting(e.App, "feedback.timings"))
	}
	if p := view.SlowCheckOf(e.App, &app.CheckClock{Began: began}, time.Minute); p != nil {
		t.Fatal("prompt with always")
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(relay.sent()) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if len(relay.sent()) != 1 {
		t.Fatal("always did not send")
	}

	stubRelay(t, false)
	e2, _ := slowEnv(t, oplog.GUI)
	if err := view.SendSlowCheck(e2.App, "{}"); err != nil {
		t.Fatalf("404 is not an error: %v", err)
	}
}
