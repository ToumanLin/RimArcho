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
