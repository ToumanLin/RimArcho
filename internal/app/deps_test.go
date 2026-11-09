package app_test

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
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
	if runtime.GOOS == "linux" {
		// Linux plan §3 step 3: toolIdle still refuses tool replacement.
		t.Skip("Linux tool-process detection is not implemented (plan step 3)")
	}
	t.Parallel()
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
	if runtime.GOOS == "linux" {
		// Linux plan §3 step 3: toolIdle still refuses tool replacement.
		t.Skip("Linux tool-process detection is not implemented (plan step 3)")
	}
	for _, name := range []string{deps.Todds} {
		t.Run(name, func(t *testing.T) {
			e := testkit.New(t)
			e.App.TextureEncoder = &texEnc{}
			t.Setenv("PATH", "")
			for _, key := range []string{"ProgramW6432", "ProgramFiles", "LOCALAPPDATA"} {
				t.Setenv(key, "")
			}
			i := e.App.Deps().Status(name)
			exe := "todds.exe"
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
	if runtime.GOOS == "linux" {
		// Linux plan §3 steps 3 and 6: process safety and the SteamCMD installer.
		t.Skip("Linux tool-process detection and SteamCMD installation are not implemented (plan steps 3, 6)")
	}
	t.Parallel()
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
	if b, err := os.ReadFile(external); err != nil || string(b) != "external program" {
		t.Fatalf("refused removal changed the external copy: %s %v", b, err)
	}
	if v := e.App.Store.Meta("cfg:steamcmd.path"); v != external {
		t.Fatalf("refused removal cleared the setting: %q", v)
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

// The SteamCMD case is in TestInstallDepSteamCMDOwnDirectoryAndRemove.
func TestRemoveDepRefusesExternalCopies(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.App.SteamCMDNoPath = true
	i := e.App.Deps().Status(deps.Todds)
	external := filepath.Join(e.Root, "external", deps.Todds+".exe")
	e.WriteFile(external, "external")
	e.MustRun("config", "set", i.ConfigKey, external)
	if err := e.App.RemoveDep(deps.Todds); err == nil {
		t.Fatal("removed configured copy")
	}
	if b, err := os.ReadFile(external); err != nil || string(b) != "external" {
		t.Fatalf("external copy changed: %s %v", b, err)
	}
	if e.App.Store.Meta("cfg:"+i.ConfigKey) != external {
		t.Fatal("setting cleared")
	}
}

func TestDepUnknownAndMissingRefusals(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	if _, err := e.App.InstallDep("unknown", nil); err == nil || !strings.Contains(err.Error(), "unknown dependency") {
		t.Fatalf("unknown install: %v", err)
	}
	for _, name := range []string{"unknown", deps.Todds} {
		if err := e.App.RemoveDep(name); err == nil {
			t.Fatalf("removed %s", name)
		}
	}
}

func TestInstallDepToddsAndRemove(t *testing.T) {
	if runtime.GOOS == "linux" {
		// Linux plan §3 step 3: toolIdle still refuses tool replacement.
		t.Skip("Linux tool-process detection is not implemented (plan step 3)")
	}
	t.Parallel()
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
