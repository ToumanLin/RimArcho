package app_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestRemoveSteamCMDProtectsContent(t *testing.T) {
	for _, kind := range []string{"workshop", "game", "shared-source", "env-source", "lock"} {
		t.Run(kind, func(t *testing.T) {
			e := testkit.New(t)
			e.App.SteamCMDNoPath = true
			dir := filepath.Join(e.App.LocalDir(), deps.SteamCMD)
			exe := filepath.Join(dir, "steamcmd.exe")
			e.WriteFile(exe, "tool")
			content := filepath.Join(dir, "protected")
			switch kind {
			case "workshop":
				content = filepath.Join(dir, "steamapps", "workshop", "content", "294100", "123", "mod.txt")
			case "game":
				if err := e.App.Store.InsertEnv(&env.Environment{ID: "protected", Name: "protected", GameRoot: content}); err != nil {
					t.Fatal(err)
				}
			case "shared-source", "env-source":
				e.AddEnv(false)
				envID := store.Shared
				if kind == "env-source" {
					en, err := e.App.Env()
					if err != nil {
						t.Fatal(err)
					}
					envID = en.ID
				}
				if err := e.App.Store.InsertSource(&source.Source{ID: "protected", EnvID: envID, Kind: source.Extra, Label: "protected", Root: content, Status: source.StatusOK}); err != nil {
					t.Fatal(err)
				}
			case "lock":
				e.MustRun("config", "set", "lock.dir", content)
			}
			if kind != "workshop" {
				content = filepath.Join(content, "keep.txt")
			}
			e.WriteFile(content, "precious")
			if err := e.App.CheckDepRemoval(deps.SteamCMD); err == nil {
				t.Fatal("unsafe removal offered")
			}
			if err := e.App.RemoveDep(deps.SteamCMD); err == nil || !strings.Contains(err.Error(), "nothing removed") {
				t.Fatalf("removal: %v", err)
			}
			for _, p := range []string{exe, content} {
				if _, err := os.Stat(p); err != nil {
					t.Fatal("file removed", p, err)
				}
			}
			plan, err := e.App.PlanUninstall(app.UninstallOptions{})
			if err != nil {
				t.Fatal(err)
			}
			step := plan.Step(app.UnSteamCMD)
			if step == nil || step.Items[0].Do != app.DoLeave {
				t.Fatalf("uninstall did not protect SteamCMD: %+v", step)
			}
			for _, path := range plan.AfterExit.Dirs {
				if source.Within(dir, path) {
					t.Fatalf("SteamCMD scheduled for deletion via %s", path)
				}
			}
		})
	}
}

func TestRemoveDepStaysInsideManagedDirectory(t *testing.T) {
	for _, name := range []string{deps.Todds, deps.WezTerm} {
		t.Run(name, func(t *testing.T) {
			e := testkit.New(t)
			e.App.TextureEncoder = &texEnc{}
			t.Setenv("PATH", "")
			for _, key := range []string{"ProgramW6432", "ProgramFiles", "LOCALAPPDATA"} {
				t.Setenv(key, "")
			}
			i := e.App.Deps().Status(name)
			exe := "todds.exe"
			if name == deps.WezTerm {
				exe = "wezterm-gui.exe"
			}
			e.WriteFile(filepath.Join(i.Dir, exe), "tool")
			sibling := i.Dir + "-other"
			e.WriteFile(filepath.Join(sibling, "keep.txt"), "keep")
			e.WriteFile(filepath.Join(filepath.Dir(i.Dir), "keep.txt"), "keep")
			if err := e.App.RemoveDep(name); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(i.Dir); !os.IsNotExist(err) {
				t.Fatal("managed directory remains", err)
			}
			for _, p := range []string{filepath.Join(sibling, "keep.txt"), filepath.Join(filepath.Dir(i.Dir), "keep.txt")} {
				if _, err := os.Stat(p); err != nil {
					t.Fatal("removed outside own directory", err)
				}
			}
		})
	}
}

func TestInstallDepSteamCMDOwnDirectoryAndRemove(t *testing.T) {
	e := testkit.New(t)
	e.App.SteamCMDNoPath = true
	external := filepath.Join(e.Root, "external", "steamcmd.exe")
	e.WriteFile(external, "external program")
	e.MustRun("config", "set", "steamcmd.path", external)
	z := steamZip(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(z) }))
	defer srv.Close()
	e.App.SteamCMDZip = srv.URL
	var done int64
	p, err := e.App.InstallDep(deps.SteamCMD, func(n, total int64) { done = n })
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(e.App.LocalDir(), deps.SteamCMD, "steamcmd.exe")
	if p != want || done != int64(len(z)) {
		t.Fatalf("installed %s, progress %d", p, done)
	}
	if b, err := os.ReadFile(external); err != nil || string(b) != "external program" {
		t.Fatalf("external copy changed: %s %v", b, err)
	}
	if err := e.App.RemoveDep(deps.SteamCMD); err == nil {
		t.Fatal("removed configured external copy")
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatal("own copy removed when external selected", err)
	}
	e.MustRun("config", "set", "steamcmd.path", "")
	if err := e.App.RemoveDep(deps.SteamCMD); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(want)); !os.IsNotExist(err) {
		t.Fatalf("own directory remains: %v", err)
	}
	if _, err := os.Stat(external); err != nil {
		t.Fatal("external copy removed", err)
	}
	if v := e.App.Store.Meta("cfg:steamcmd.path"); v != "" {
		t.Fatalf("config changed: %q", v)
	}
}

func TestRemoveDepRefusesExternalCopies(t *testing.T) {
	for _, name := range []string{deps.Todds, deps.SteamCMD, deps.WezTerm} {
		t.Run(name, func(t *testing.T) {
			e := testkit.New(t)
			e.App.SteamCMDNoPath = true
			i := e.App.Deps().Status(name)
			external := filepath.Join(e.Root, "external", name+".exe")
			e.WriteFile(external, "external")
			e.MustRun("config", "set", i.ConfigKey, external)
			if err := e.App.RemoveDep(name); err == nil {
				t.Fatal("removed configured copy")
			}
			if b, err := os.ReadFile(external); err != nil || string(b) != "external" {
				t.Fatalf("external copy changed: %s %v", b, err)
			}
			if e.App.Store.Meta("cfg:"+i.ConfigKey) != external {
				t.Fatal("setting cleared")
			}
		})
	}
}

func TestRemoveDepRefusesPATHAndProgramFiles(t *testing.T) {
	for _, src := range []string{"path", "program-files"} {
		t.Run(src, func(t *testing.T) {
			e := testkit.New(t)
			dir := filepath.Join(e.Root, "external")
			p := filepath.Join(dir, "wezterm-gui.exe")
			t.Setenv("PATH", dir)
			for _, key := range []string{"ProgramW6432", "ProgramFiles", "LOCALAPPDATA"} {
				t.Setenv(key, "")
			}
			if src == "program-files" {
				p = filepath.Join(dir, "WezTerm", "wezterm-gui.exe")
				t.Setenv("PATH", "")
				t.Setenv("ProgramFiles", dir)
			}
			e.WriteFile(p, "external")
			i := e.App.Deps().Status(deps.WezTerm)
			if i.Source != src || i.Own {
				t.Fatalf("status %+v", i)
			}
			if err := e.App.RemoveDep(deps.WezTerm); err == nil {
				t.Fatal("removed external copy")
			}
			if _, err := os.Stat(p); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInstallDepWezTermAndRemove(t *testing.T) {
	e := testkit.New(t)
	const name = "WezTerm-windows-20990101-a"
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create(name + "/wezterm-gui.exe")
	w.Write([]byte("portable"))
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
	var done int64
	p, err := e.App.InstallDep(deps.WezTerm, func(n, total int64) { done = n })
	if err != nil || done != int64(len(z)) {
		t.Fatalf("install %s, progress %d: %v", p, done, err)
	}
	if p != filepath.Join(e.App.Home, deps.WezTerm, name, "wezterm-gui.exe") {
		t.Fatal(p)
	}
	// Only the portable copy is visible for removal, regardless of the test machine.
	t.Setenv("PATH", "")
	for _, key := range []string{"ProgramW6432", "ProgramFiles", "LOCALAPPDATA"} {
		t.Setenv(key, "")
	}
	i := e.App.Deps().Status(deps.WezTerm)
	if !i.OK || !i.Own || i.Version != "20990101-a" {
		t.Fatalf("portable status %+v", i)
	}
	if err := e.App.RemoveDep(deps.WezTerm); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(i.Dir); !os.IsNotExist(err) {
		t.Fatalf("portable directory remains: %v", err)
	}
}

func TestDepFontUnknownAndMissingRefusals(t *testing.T) {
	e := testkit.New(t)
	if _, err := e.App.InstallDep(deps.Font, nil); err == nil || err.Error() != "the font ships with RimArcho; reinstall the release zip" {
		t.Fatalf("font install: %v", err)
	}
	if _, err := e.App.InstallDep("unknown", nil); err == nil || !strings.Contains(err.Error(), "unknown dependency") {
		t.Fatalf("unknown install: %v", err)
	}
	for _, name := range []string{deps.Font, "unknown", deps.Todds} {
		if err := e.App.RemoveDep(name); err == nil {
			t.Fatalf("removed %s", name)
		}
	}
}

func TestInstallDepToddsAndRemove(t *testing.T) {
	e := testkit.New(t)
	e.App.TextureEncoder = &texEnc{}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"todds.exe", "LICENSE"} {
		w, err := zw.Create("release/" + name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(name))
	}
	zw.Close()
	z := buf.Bytes()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(z) }))
	defer srv.Close()
	e.App.ToddsZip = srv.URL
	var done int64
	p, err := e.App.InstallDep(deps.Todds, func(n, total int64) { done = n })
	if err != nil || p != filepath.Join(e.App.ToddsDir(), "todds.exe") || done != int64(len(z)) {
		t.Fatalf("install %s, progress %d: %v", p, done, err)
	}
	if i := e.App.Deps().Status(deps.Todds); !i.Own || !i.OK {
		t.Fatalf("todds status %+v", i)
	}
	if err := e.App.RemoveDep(deps.Todds); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(p)); !os.IsNotExist(err) {
		t.Fatalf("todds remains: %v", err)
	}
}
