package gui

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestSettingCommandsAreTasks(t *testing.T) {
	for _, tc := range []struct {
		argv        []string
		kind, state string
	}{
		{[]string{"cache", "clear", "previews"}, view.TaskCache, app.TaskDone},
		{[]string{"cache", "clear", "invalid"}, view.TaskCache, app.TaskFailed},
		{[]string{"db", "backup"}, view.TaskBackup, app.TaskDone},
		{[]string{"db", "vacuum"}, view.TaskVacuum, app.TaskDone},
		{[]string{"gc"}, view.TaskGC, app.TaskDone},
		{[]string{"data", "update"}, view.TaskData, app.TaskFailed},
	} {
		t.Run(strings.Join(tc.argv, "-"), func(t *testing.T) {
			e, api, events := fixture(t)
			for _, d := range app.Datasets {
				e.MustRun("config", "set", "data."+d.Key+".url", filepath.Join(t.TempDir(), "missing.json"))
			}
			out, err := api.ApplySetting(tc.argv, "")
			if err != nil || len(out) != 1 || !strings.Contains(out[0], "RimArcho") {
				t.Fatalf("submission %v %v", out, err)
			}
			r := endedTask(t, api, tc.kind)
			if r.State != tc.state || r.Line == "" || r.Target != "settings" || r.Lane != app.LaneSelf {
				t.Fatalf("task %+v", r)
			}
			waitSettingEnd(t, events, r.ID)
			var record []app.Task
			b, err := os.ReadFile(filepath.Join(e.App.Home, "tasks.json"))
			if err != nil || json.Unmarshal(b, &record) != nil || len(record) != 1 || record[0].Result == "" {
				t.Fatalf("result not saved: %s %v", b, err)
			}
			if tc.kind == view.TaskBackup {
				status, _ := e.App.DBInfo()
				if len(status.Backups) != 1 {
					t.Fatalf("backup not made: %+v", status)
				}
			}
		})
	}
}

func waitSettingEnd(t *testing.T, events <-chan string, id string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case event := <-events:
			if strings.HasPrefix(event, "tasks ") && strings.Contains(event, `"ended"`) && strings.Contains(event, id) {
				return
			}
		case <-deadline:
			t.Fatal("no completion for the status line")
		}
	}
}

// The guide returns while downloading, progress appears in the record, and settings
// can still be read. SteamCMD and todds share the same queue entry points (06 §2.16).
func TestToolDownloadsAreTasks(t *testing.T) {
	e, api, events := fixture(t)
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, _ := w.Create("steamcmd.exe")
	f.Write([]byte("stand-in"))
	w.Close()
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/todds" {
			http.Error(w, "offline", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(b.Bytes()[:8])
		w.(http.Flusher).Flush()
		close(started)
		<-release
		w.Write(b.Bytes()[8:])
	}))
	t.Cleanup(srv.Close)
	e.App.SteamCMDZip, e.App.ToddsZip = srv.URL+"/steam", srv.URL+"/todds"
	id, err := api.GuideInstallSteamCMD()
	if err != nil || id == "" {
		t.Fatalf("submit %q %v", id, err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("SteamCMD did not start")
	}
	read := make(chan struct{})
	go func() { api.Settings(); close(read) }()
	select {
	case <-read:
	case <-time.After(5 * time.Second):
		t.Fatal("download held the GUI lock")
	}
	if _, err := api.TexturesInstall(); err != nil {
		t.Fatal(err)
	}
	todds := endedTask(t, api, view.TaskTodds)
	if todds.State != app.TaskFailed || !strings.Contains(todds.Line, "503") {
		t.Fatalf("todds %+v", todds)
	}
	waitSettingEnd(t, events, todds.ID)
	for _, task := range api.tasks.List() {
		if task.ID == id && (task.State != app.TaskRunning || task.Phase == "") {
			t.Fatalf("progress %+v", task)
		}
	}
	once.Do(func() { close(release) })
	steam := endedTask(t, api, view.TaskSteamCMD)
	if steam.State != app.TaskDone || e.App.SteamCMDPath() == "" {
		t.Fatalf("SteamCMD %+v", steam)
	}
	waitSettingEnd(t, events, steam.ID)
}

func TestSettingsConfirmationsAndRefresh(t *testing.T) {
	e, api, events := fixture(t)
	p := filepath.Join(e.App.Home, "cache", "previews", "one.png")
	e.WriteFile(p, "cached image")
	check := func(cleared bool) {
		t.Helper()
		confirmed := map[string]bool{}
		for _, group := range api.Settings() {
			for _, item := range group.Items {
				for _, action := range item.Actions {
					if action.Task == "" {
						continue
					}
					if action.Pending == "" {
						t.Fatalf("no pending label: %+v", action)
					}
					confirmed[action.Task] = action.Confirm != ""
					if action.Task == view.TaskCache && action.Argv[2] == "previews" && cleared && item.Value != view.Tf("%s  %d items", view.SizeText(0), 0) {
						t.Fatalf("size not refreshed: %+v", item)
					}
				}
			}
		}
		if !confirmed[view.TaskData] || !confirmed[view.TaskGC] || confirmed[view.TaskBackup] || confirmed[view.TaskVacuum] {
			t.Fatalf("confirmations %v", confirmed)
		}
	}
	check(false)
	if len(api.tasks.List()) != 0 {
		t.Fatal("reading confirmation choices submitted a task")
	}
	api.ApplySetting([]string{"cache", "clear", "previews"}, "")
	r := endedTask(t, api, view.TaskCache)
	waitSettingEnd(t, events, r.ID)
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("cache still exists: %v", err)
	}
	check(true)
}

func TestCommunityUpdateTaskRefresh(t *testing.T) {
	e, api, events := fixture(t)
	files := []string{
		`{"timestamp":1789239946,"rules":{"tester.b":{"loadAfter":{"tester.a":{"name":["A"]}}}}}`,
		`{"version":1790337009,"database":{"1001":{"packageId":"tester.a","name":"Alpha"}}}`,
		`{"version":"2026-10-19T06:29:54Z","rules":[{"oldWorkshopId":"1001","oldPackageId":"tester.a","newWorkshopId":"9999","newPackageId":"tester.a2"}]}`,
	}
	for i, d := range app.Datasets {
		path := filepath.Join(t.TempDir(), d.Key+".json")
		e.WriteFile(path, files[i])
		e.MustRun("config", "set", "data."+d.Key+".url", path)
	}
	if _, err := api.ApplySetting([]string{"data", "update"}, ""); err != nil {
		t.Fatal(err)
	}
	r := endedTask(t, api, view.TaskData)
	if r.State != app.TaskDone || r.Pct != 100 || r.Count != "3 / 3" {
		t.Fatalf("community update %+v", r)
	}
	waitSettingEnd(t, events, r.ID)
	status := e.App.DataStatus()
	for _, d := range app.Datasets {
		if status[d.Key] == nil || status[d.Key].Entries != 1 {
			t.Fatalf("status %s: %+v", d.Key, status[d.Key])
		}
	}
	for _, group := range api.Settings() {
		for _, item := range group.Items {
			if len(item.Argv) > 2 && strings.HasPrefix(item.Argv[2], "data.") && !strings.Contains(item.Note, "1") {
				t.Fatalf("entries not refreshed: %+v", item)
			}
		}
	}
}

func TestSettingTaskRetryAfterReopen(t *testing.T) {
	_, api, events := fixture(t)
	argv := []string{"cache", "clear", "invalid"}
	if _, err := api.ApplySetting(argv, ""); err != nil {
		t.Fatal(err)
	}
	first := endedTask(t, api, view.TaskCache)
	waitSettingEnd(t, events, first.ID)
	if !hasAct(first, "retry") {
		t.Fatal("failed maintenance has no retry action")
	}
	api.tasks.Close()
	api.startTasks()
	if _, err := api.TaskAct("retry", []string{first.ID}, ""); err != nil {
		t.Fatal(err)
	}
	var next app.Task
	for _, task := range api.tasks.List() {
		if task.ID != first.ID {
			next = task
		}
	}
	if strings.Join(next.Args, " ") != strings.Join(argv, " ") || next.Env == "" {
		t.Fatalf("retry lost its request: %+v", next)
	}
	waitSettingEnd(t, events, next.ID)
	for _, task := range api.tasks.List() {
		if task.ID == next.ID && (task.State != app.TaskFailed || task.Result == "") {
			t.Fatalf("retry result %+v", task)
		}
	}
}
