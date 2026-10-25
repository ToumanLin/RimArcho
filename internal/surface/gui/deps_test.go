package gui

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestDependencySettingsRenderer(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is needed for the dependency settings tests")
	}
	if out, err := exec.Command(node, "--experimental-vm-modules", "--test", "testdata/deps.mjs").CombinedOutput(); err != nil {
		t.Fatalf("dependency settings: %v\n%s", err, out)
	}
}

func TestDependencySettingTasksInstallRemoveAndRetry(t *testing.T) {
	t.Parallel()
	e, api, events := fixture(t)
	e.App.SteamCMDNoPath = true
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("steamcmd.exe")
	w.Write([]byte("fake steamcmd"))
	zw.Close()
	z := buf.Bytes()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(z) }))
	defer srv.Close()
	e.App.SteamCMDZip = srv.URL
	if _, err := api.ApplySetting([]string{"deps", "install", deps.SteamCMD}, ""); err != nil {
		t.Fatal(err)
	}
	installed := endedTask(t, api, view.TaskSteamCMD)
	waitSettingEnd(t, events, installed.ID)
	if installed.State != app.TaskDone || installed.Obj != deps.SteamCMD || installed.Target != view.PageSettings {
		t.Fatalf("install task %+v", installed)
	}
	p := e.App.SteamCMDPath()
	if p == "" {
		t.Fatal("SteamCMD not installed")
	}
	if _, err := api.ApplySetting([]string{"deps", "remove", deps.SteamCMD}, ""); err != nil {
		t.Fatal(err)
	}
	removed := endedTask(t, api, view.TaskDepRemove)
	waitSettingEnd(t, events, removed.ID)
	if removed.State != app.TaskDone || removed.Obj != deps.SteamCMD {
		t.Fatalf("remove task %+v", removed)
	}
	if _, err := os.Stat(filepath.Dir(p)); !os.IsNotExist(err) {
		t.Fatalf("SteamCMD remains: %v", err)
	}
	// Retry preserves the dependency name and install request after reopening the queue.
	api.tasks.Close()
	api.startTasks()
	if _, err := api.TaskAct("retry", []string{installed.ID}, ""); err != nil {
		t.Fatal(err)
	}
	var retry app.Task
	for _, task := range api.tasks.List() {
		if task.ID != installed.ID && task.Kind == view.TaskSteamCMD {
			retry = task
		}
	}
	if strings.Join(retry.Args, " ") != "deps install steamcmd" {
		t.Fatalf("retry %+v", retry)
	}
	waitSettingEnd(t, events, retry.ID)
	if e.App.SteamCMDPath() == "" {
		t.Fatal("retry did not reinstall SteamCMD")
	}
}

func TestWezTermSettingsTask(t *testing.T) {
	t.Parallel()
	e, api, events := fixture(t)
	const name = "WezTerm-windows-20990101-a"
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create(name + "/wezterm-gui.exe")
	w.Write([]byte("fake"))
	zw.Close()
	z := buf.Bytes()
	sum := sha256.Sum256(z)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/wezterm/wezterm/releases/latest":
			fmt.Fprintf(w, `{"tag_name":"test","assets":[{"name":"%[1]s.zip","browser_download_url":"%[2]s/zip","size":%[3]d},{"name":"%[1]s.zip.sha256","browser_download_url":"%[2]s/sha"}]}`, name, srv.URL, len(z))
		case "/zip":
			w.Write(z)
		case "/sha":
			fmt.Fprintf(w, "%x", sum)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	e.App.WezTermAPI = srv.URL
	if _, err := api.ApplySetting([]string{"deps", "install", deps.WezTerm}, ""); err != nil {
		t.Fatal(err)
	}
	r := endedTask(t, api, view.TaskWezTerm)
	waitSettingEnd(t, events, r.ID)
	if r.State != app.TaskDone || r.Obj != deps.WezTerm || r.Target != view.PageSettings || r.Pct != 100 {
		t.Fatalf("WezTerm task %+v", r)
	}
	if _, err := os.Stat(filepath.Join(e.App.Home, deps.WezTerm, name, "wezterm-gui.exe")); err != nil {
		t.Fatal(err)
	}
}

func TestToolInstallOfferUsesManagedDestinationAndServerSize(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodHead {
			t.Error("offer downloaded the tool")
		}
		w.Header().Set("Content-Length", "12345")
	}))
	defer srv.Close()
	e.App.SteamCMDZip = srv.URL
	external := filepath.Join(e.Root, "external", "steamcmd.exe")
	e.WriteFile(external, "offline fixture")
	e.MustRun("config", "set", "steamcmd.path", external)
	offer, err := api.ToolInstallOffer(deps.SteamCMD)
	if err != nil || offer.URL != srv.URL || offer.Bytes != 12345 || offer.Dir != filepath.Join(e.App.LocalDir(), deps.SteamCMD) {
		t.Fatal(offer, err)
	}
	if requests != 1 {
		t.Fatal(requests)
	}
	todds, err := api.ToolInstallOffer(deps.Todds)
	if err != nil || todds.URL == "" || todds.Bytes <= 0 || todds.Dir != e.App.ToddsDir() {
		t.Fatal(todds, err)
	}
	if _, err := api.ToolInstallOffer("not-a-tool"); err == nil {
		t.Fatal("unknown offer accepted")
	}
}
