package view_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestVersionReasonsAndMuteActions(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.Game, "Version.txt"), "1.7.5000 rev1")
	e.AddWorkshop("1001", testkit.Mod{PackageID: "tester.a", Name: "Alpha", Supported: []string{"1.6"}})
	e.AddWorkshop("1002", testkit.Mod{PackageID: "tester.b", Name: "Beta", Supported: []string{"1.5", "1.6"}})
	e.WriteModsConfig("1.7.5000 rev1", "ludeon.rimworld", "tester.a", "tester.b")
	e.AddEnv(false)
	e.MustRun("scan")
	build := func() (*view.Needs, *app.ProblemSet) {
		t.Helper()
		ps, err := e.App.Problems(false, nil)
		if err != nil {
			t.Fatal(err)
		}
		return view.BuildNeeds(e.App, ps.State, ps), ps
	}
	n, ps := build()
	var merged *view.Need
	for i := range n.Items {
		if strings.Contains(n.Items[i].Text, "mods do not list 1.7") {
			merged = &n.Items[i]
		}
	}
	if merged == nil || !merged.Counted || len(merged.Mods) != 2 {
		t.Fatalf("merged: %+v", merged)
	}
	batch := merged.Actions[1]
	if batch.Kind != view.ActVersionMute || batch.Confirm == "" || batch.Warn == "" || len(batch.Refs) != 2 {
		t.Fatalf("batch: %+v", batch)
	}
	if merged.Actions[0].Kind != view.ActVersionList || merged.Actions[0].GameVersion != "1.7" {
		t.Fatalf("list: %+v", merged.Actions)
	}
	panel, err := view.ReadVersionWarnings(e.App, "1.7", false)
	if err != nil || len(panel.Groups) != 1 || len(panel.Groups[0].Rows) != 2 {
		t.Fatalf("lazy list: %+v %v", panel, err)
	}
	var alpha string
	for id, rs := range n.Reasons {
		m := ps.State.Ix.ByID[id]
		if len(rs) != 1 {
			t.Fatalf("%s reasons: %+v", m.Name, rs)
		}
		r := rs[0]
		if strings.Contains(r.Text, "mods") || !strings.Contains(r.Text, "does not list 1.7") || len(r.Actions) != 1 || r.Actions[0].Kind != view.ActVersionMute || r.Actions[0].Confirm != "" {
			t.Fatalf("row reason: %+v", r)
		}
		if m.Name == "Alpha" {
			alpha = id
			if !strings.HasSuffix(r.Text, "(lists 1.6)") {
				t.Fatal(r.Text)
			}
		}
		if m.Name == "Beta" && !strings.HasSuffix(r.Text, "(lists 1.5, 1.6)") {
			t.Fatal(r.Text)
		}
	}
	if alpha == "" {
		t.Fatal("missing Alpha reason")
	}
	// Event payload carries these individual reasons, so JS never recreates merged ones.
	b, err := json.Marshal(view.ProblemsOf(n))
	if err != nil {
		t.Fatal(err)
	}
	var event struct {
		Reasons map[string][]view.Reason `json:"reasons"`
	}
	if err := json.Unmarshal(b, &event); err != nil || event.Reasons[alpha][0].Text != n.Reasons[alpha][0].Text {
		t.Fatalf("event reasons: %s, %v", b, err)
	}
	if _, err := view.Do(e.App, n.Reasons[alpha][0].Actions[0], 0); err != nil {
		t.Fatal(err)
	}
	n, ps = build()
	if len(ps.Muted) != 1 || n.Status[alpha] != 0 || len(n.Reasons[alpha]) != 0 {
		t.Fatal("muted row still warned")
	}
	r, err := view.VersionReason(e.App, ps.State, ps.State.Ix.ByID[alpha])
	if err != nil || r == nil || r.Status != view.StatusNone || len(r.Actions) != 1 || r.Actions[0].Kind != view.ActVersionUnmute {
		t.Fatalf("muted detail: %+v %v", r, err)
	}
	if _, err := view.Do(e.App, batch, 0); err != nil {
		t.Fatal(err)
	}
	n, ps = build()
	if n.Count != 0 || len(n.Status) != 0 || len(ps.Muted) != 2 {
		t.Fatalf("all muted: %+v", n)
	}
	var muted *view.Need
	for i := range n.Items {
		if n.Items[i].Text == "2 mods' 1.7 warnings are ignored" {
			muted = &n.Items[i]
		}
	}
	if muted == nil || !muted.Dim || muted.Counted || !muted.Actions[0].VersionMuted {
		t.Fatalf("muted line: %+v", muted)
	}
	panel, err = view.ReadVersionWarnings(e.App, "1.7", true)
	if err != nil || len(panel.Groups) != 1 || len(panel.Groups[0].Rows) != 2 {
		t.Fatalf("ignored list: %+v %v", panel, err)
	}
	if _, err := view.Do(e.App, panel.Groups[0].Rows[0].Action, 0); err != nil {
		t.Fatal(err)
	}
	n, ps = build()
	if len(ps.Muted) != 1 || n.Count != 1 {
		t.Fatalf("restored counts: muted=%d count=%d", len(ps.Muted), n.Count)
	}
	// A later update can declare support; the stored choice still has an undo in detail.
	e.AddWorkshop("1001", testkit.Mod{PackageID: "tester.a", Name: "Alpha", Supported: []string{"1.7"}})
	e.MustRun("scan")
	e.MustRun("problems", "mute-version", "pkg:tester.a")
	_, ps = build()
	for _, m := range ps.State.Ix.Mods {
		if m.Name != "Alpha" {
			continue
		}
		r, err := view.VersionReason(e.App, ps.State, m)
		if err != nil || r == nil || r.Actions[0].Kind != view.ActVersionUnmute {
			t.Fatalf("updated detail: %+v %v", r, err)
		}
	}
}

func TestVersionReasonChinese(t *testing.T) {
	view.SetLang("zh")
	t.Cleanup(func() { view.SetLang("en") })
	if got := view.Tf("does not list %s as supported (lists %s)", "1.7", "1.6"); got != "没有标明支持 1.7（标明了 1.6）" {
		t.Fatal(got)
	}
	for _, c := range []struct {
		key  string
		args []any
		want string
	}{
		{"ignore the %s warning for this mod", []any{"1.7"}, "忽略此 Mod 的 1.7 提醒"},
		{"the %s warning for this mod is ignored", []any{"1.7"}, "已忽略此 Mod 的 1.7 提醒"},
		{"restore the %s warning", []any{"1.7"}, "恢复 1.7 提醒"},
		{"the %s warning for this mod is restored", []any{"1.7"}, "已恢复此 Mod 的 1.7 提醒"},
		{"ignore these %d mods' %s warnings", []any{365, "1.7"}, "忽略这 365 个 Mod 的 1.7 提醒"},
		{"%d mods' %s warnings are ignored", []any{1, "1.7"}, "1 个 Mod 的 1.7 提醒已忽略"},
	} {
		if got := view.Tf(c.key, c.args...); got != c.want {
			t.Errorf("%s: %s", c.key, got)
		}
	}
}

func TestVersionPanelGroupsAndActions(t *testing.T) {
	view.SetLang("en")
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.Game, "Version.txt"), "1.7.5000 rev1")
	packages := []string{"ludeon.rimworld"}
	for i, versions := range [][]string{{"1.6"}, {"1.5", "1.6"}, {"1.4", "1.5"}, {"1.4"}, {"1.8"}} {
		pkg := fmt.Sprintf("tester.mod%d", i)
		e.AddWorkshop(fmt.Sprint(1001+i), testkit.Mod{PackageID: pkg, Name: fmt.Sprintf("Mod %d", i), Supported: versions})
		packages = append(packages, pkg)
	}
	e.WriteModsConfig("1.7.5000 rev1", packages...)
	e.AddEnv(false)
	e.MustRun("scan")
	panel, err := view.ReadVersionWarnings(e.App, "1.7", false)
	if err != nil {
		t.Fatal(err)
	}
	if panel.Title != "does not list 1.7 as supported" || len(panel.Groups) != 3 || panel.All.Confirm == "" || len(panel.All.Refs) != 5 {
		t.Fatalf("panel: %+v", panel)
	}
	if panel.Groups[0].Title != "lists 1.6  2" || panel.Groups[1].Title != "only lists 1.5 or earlier  2" {
		t.Fatalf("groups: %+v", panel.Groups)
	}
	for _, group := range panel.Groups {
		if group.Action.Confirm == "" || len(group.Action.Refs) != len(group.Rows) {
			t.Fatalf("group action: %+v", group)
		}
		for _, row := range group.Rows {
			if row.ID == "" || strings.Contains(row.Supported, "does not list") || row.Action.Label != "ignore" || len(row.Action.Refs) != 1 || row.Action.Confirm != "" {
				t.Fatalf("row: %+v", row)
			}
		}
	}
	if _, err := view.Do(e.App, panel.Groups[0].Action, 0); err != nil {
		t.Fatal(err)
	}
	panel, err = view.ReadVersionWarnings(e.App, "1.7", false)
	if err != nil || len(panel.All.Refs) != 3 || len(panel.Groups) != 2 {
		t.Fatalf("after group: %+v %v", panel, err)
	}
	ignored, err := view.ReadVersionWarnings(e.App, "1.7", true)
	if err != nil || len(ignored.All.Refs) != 2 || ignored.All.Kind != view.ActVersionUnmute || ignored.All.Confirm != "" {
		t.Fatalf("restore all: %+v %v", ignored, err)
	}
	if _, err := view.Do(e.App, ignored.All, 0); err != nil {
		t.Fatal(err)
	}
	panel, err = view.ReadVersionWarnings(e.App, "1.7", false)
	if err != nil || len(panel.All.Refs) != 5 {
		t.Fatalf("restored: %+v %v", panel, err)
	}
	if _, err := view.ReadVersionWarnings(e.App, "1.8", false); err == nil {
		t.Fatal("stale version accepted")
	}
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(view.BuildNeeds(e.App, ps.State, ps))
	if err != nil || strings.Contains(string(data), `"entries":`) || strings.Contains(string(data), `"rows":`) {
		t.Fatalf("eager panel payload: %s %v", data, err)
	}
}
