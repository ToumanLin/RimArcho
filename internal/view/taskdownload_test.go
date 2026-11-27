package view_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestSteamTaskDownloadView(t *testing.T) {
	defer view.SetLang("en")
	task := app.Task{ID: "t1", Kind: view.LibResub, Lane: app.LaneSteam, State: app.TaskRunning, Total: 14, Done: 2, Frac: 2.45 / 14, Phase: "Steam is downloading 3 of 14",
		Items: []app.TaskItem{{Ref: "mod", Name: "mod", State: app.ItemRun, Got: 45, Size: 100, Bytes: 200}, {Ref: "waiting", State: app.ItemRun}}}
	for _, c := range []struct{ lang, phase, down, wait string }{
		{"en", "Steam is downloading 3 of 14", "downloading 45%", "waiting for Steam"},
		{"zh", "Steam 正在下载 3 / 14", "下载中 45%", "等 Steam 开始下载"},
	} {
		view.SetLang(c.lang)
		row := view.BuildTasks([]app.Task{task}, time.Now()).Rows[0]
		if row.Line != c.phase || row.Items[0].StateText != c.down || row.Items[1].StateText != c.wait || row.Pct != 17 {
			t.Fatalf("%+v", row)
		}
		data, err := json.Marshal(row.Items[0])
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`"got":45`, `"downloadSize":100`, `"bytes":200`, `"stateText":`, `"size":"`} {
			if !strings.Contains(string(data), want) {
				t.Fatalf("missing %s: %s", want, data)
			}
		}
	}
}

// A subscription that follows Steam's download (11 §2.2) shows the same stage and item text
// as a resubscribe, from SetWorkshopProgress; a stalled item says why.
func TestSubscribeTaskFollowsSteamDownload(t *testing.T) {
	defer view.SetLang("en")
	task := app.Task{ID: "t2", Kind: view.TaskSubscribe, Lane: app.LaneSelf, State: app.TaskRunning}
	task.SetWorkshopProgress([]app.TaskItem{
		{Ref: "ws:1", Name: "One", State: app.ItemOK, Size: 100, Got: 100},
		{Ref: "ws:2", Name: "Two", State: app.ItemRun, Steam: true, Size: 100, Got: 40},
		{Ref: "ws:3", Name: "Three", State: app.ItemRun, Steam: true, Size: 100},
		{Ref: "ws:4", Name: "Four", State: app.ItemFail, Steam: true, FailureKind: "stalled", Why: "Steam did not download it; check Steam (no download progress for 2 minutes)"},
	})
	if task.Phase != "Steam is downloading 2 of 4" || task.Frac < 0.34 || task.Frac > 0.36 {
		t.Fatalf("phase %q frac %v", task.Phase, task.Frac)
	}
	for _, c := range []struct{ lang, phase, down, wait string }{
		{"en", "Steam is downloading 2 of 4", "downloading 40%", "waiting for Steam"},
		{"zh", "Steam 正在下载 2 / 4", "下载中 40%", "等 Steam 开始下载"},
	} {
		view.SetLang(c.lang)
		row := view.BuildTasks([]app.Task{task}, time.Now()).Rows[0]
		byRef := map[string]view.TaskItemRow{}
		for _, it := range row.Items {
			byRef[it.Ref] = it
		}
		if row.Line != c.phase || byRef["ws:2"].StateText != c.down || byRef["ws:3"].StateText != c.wait {
			t.Fatalf("%s: %+v", c.lang, row)
		}
		if c.lang == "en" && byRef["ws:4"].Why != "Steam stopped downloading it (no progress for 2 minutes)" {
			t.Fatalf("stalled reason %q", byRef["ws:4"].Why)
		}
	}
}
