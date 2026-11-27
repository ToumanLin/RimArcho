package view_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
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
	if p := view.SlowCheckOf(e.App, began, 10*time.Second); p != nil {
		t.Fatal("prompt for a 10 s check")
	}
	p := view.SlowCheckOf(e.App, began, 48*time.Second)
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
	phases := tm["phases"].([]any)
	if len(phases) != 3 && len(phases) != 4 { // read, defs, patches, and other when the row's own time is known
		t.Fatalf("phases %v", phases)
	}
	for _, s := range []string{"My secret plan", e.Home, e.Game, "Users"} {
		if strings.Contains(p.Preview, s) {
			t.Fatalf("%q in the preview", s)
		}
	}
	if again := view.SlowCheckOf(e.App, began, 48*time.Second); again != nil {
		t.Fatal("a second prompt within a day")
	}
	if err := view.SendSlowCheck(e.App, p.Preview); err != nil {
		t.Fatal(err)
	}
	if got := relay.sent(); len(got) != 1 || got[0] != p.Preview {
		t.Fatalf("sent %q", got)
	}
}

func TestSlowCheckNeverForAgents(t *testing.T) {
	stubRelay(t, true)
	for _, o := range []string{oplog.MCP, oplog.Agent, oplog.CLI} {
		e, began := slowEnv(t, o)
		if p := view.SlowCheckOf(e.App, began, time.Minute); p != nil {
			t.Fatalf("%s: prompt", o)
		}
	}
	e, began := slowEnv(t, oplog.TUI)
	if p := view.SlowCheckOf(e.App, began, time.Minute); p == nil {
		t.Fatal("tui: no prompt")
	}
}

// off asks nothing; always sends without a prompt; a relay without the route is quiet.
func TestSlowCheckSettings(t *testing.T) {
	relay := stubRelay(t, true)
	e, began := slowEnv(t, oplog.GUI)
	e.MustRun("config", "set", "feedback.timings", "off")
	if p := view.SlowCheckOf(e.App, began, time.Minute); p != nil {
		t.Fatal("prompt with off")
	}
	if err := view.AlwaysSendSlowChecks(e.App); err != nil || cmds.Setting(e.App, "feedback.timings") != "always" {
		t.Fatal(err, cmds.Setting(e.App, "feedback.timings"))
	}
	if p := view.SlowCheckOf(e.App, began, time.Minute); p != nil {
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
