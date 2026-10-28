package cmds_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestCompactAutomaticStatus(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(true)
	e.MustRun("scan")
	seed := func(kind string, pending int) {
		s := app.AutomaticStatus{Kind: kind, Pending: pending, Checked: e.Now(), Completed: e.Now(), State: "check complete"}
		b, _ := json.Marshal(s)
		if err := e.App.Store.SetMeta("automatic:"+kind, string(b)); err != nil {
			t.Fatal(err)
		}
	}
	seed("session", 1) // an old forgotten account must not appear
	seed("mods", 2)
	seed("tools", 1)
	for _, argv := range [][]string{{"status"}, {"data", "status"}} {
		out := e.MustRun(argv...)
		for _, want := range []string{"automatic checks: on;", "2 mod updates waiting", "1 tool update waiting"} {
			if !strings.Contains(out, want) {
				t.Fatal(out)
			}
		}
		if strings.Count(out, "automatic checks:") != 1 || strings.Contains(out, "LoversLab") || strings.Contains(out, "0 pending") || strings.Contains(out, "not checked yet") {
			t.Fatal(out)
		}
	}
	if err := e.App.Store.SetMeta("account:loverslab:enabled", "true"); err != nil {
		t.Fatal(err)
	}
	if out := e.MustRun("status"); !strings.Contains(out, "LoversLab needs login") {
		t.Fatal(out)
	}
	seed("mods", 0)
	seed("tools", 0)
	seed("session", 0)
	e.Advance(time.Hour)
	seed("data", 0)
	if out := e.MustRun("status"); !strings.Contains(out, "automatic checks: on; nothing waiting (last 10-01 11:00)") || strings.Contains(out, "LoversLab") {
		t.Fatal(out)
	}
	e.MustRun("config", "set", "updates.auto", "off")
	if out := e.MustRun("status"); !strings.Contains(out, "automatic checks: off; nothing waiting") {
		t.Fatal(out)
	}
}
