//go:build linux

package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestLinuxUpdateNoticeNeverInstalls(t *testing.T) {
	e := testkit.New(t)
	f := newSelfRelease(t)
	f.use(e.App)
	e.App.SteamAPI = f.srv.URL + "/workshop"
	view.SetLang("en")
	if err := os.MkdirAll(f.program, 0o755); err != nil {
		t.Fatal(err)
	}
	before := f.files(t)
	stale := filepath.Join(f.program, app.UpdateDirName, "old-test", "archo.exe")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("keep stale state"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"deb", "rpm", "aur", "", "unknown", "missing"} {
		if err := os.WriteFile(filepath.Join(f.program, app.InstallKindFile), []byte(kind+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if kind == "missing" {
			if err := os.Remove(filepath.Join(f.program, app.InstallKindFile)); err != nil {
				t.Fatal(err)
			}
		}
		out := e.MustRun("update")
		r := e.App.SelfUpdatePending()
		if r == nil || e.App.SelfUpdateInfo().Source != "GitHub" {
			t.Fatal(r)
		}
		instructions := e.App.SelfUpdateInstructions()
		if kind == "" || kind == "unknown" || kind == "missing" {
			if !strings.Contains(instructions, "tarball") {
				t.Fatal(instructions)
			}
		} else if !strings.Contains(instructions, "package manager") {
			t.Fatal(instructions)
		}
		if !strings.Contains(out, r.Version) || !strings.Contains(out, r.Page) || !strings.Contains(out, instructions) {
			t.Fatal(out)
		}
		it := view.SelfUpdateItem(e.App)
		if !strings.Contains(it.ValueNote, instructions) || it.Details.Link != r.Page {
			t.Fatal(it)
		}
		for _, action := range it.Actions {
			if action.Task == view.TaskSelfUpdate {
				t.Fatal("install action on Linux")
			}
		}
		notice := view.AutomaticNotice(e.App, app.AutomaticStatus{}, e.App.AutomaticStatus("self"))
		if !strings.Contains(notice, r.Version) || !strings.Contains(notice, r.Page) || !strings.Contains(notice, instructions) {
			t.Fatal(notice)
		}
		if _, err := e.App.InstallSelfUpdate(context.Background(), nil); err == nil {
			t.Fatal("self-install allowed")
		}
		e.App.CleanSelfUpdate()
		if data, err := os.ReadFile(stale); err != nil || string(data) != "keep stale state" || f.files(t) != before {
			t.Fatal("program folder modified", err)
		}
	}
	if f.zipHits.Load() != 0 || f.workshopHits.Load() != 0 {
		t.Fatal("downloaded or used Workshop")
	}
}
