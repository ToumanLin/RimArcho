package gui

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/view"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestUninstallGuardsNativeCloseAndQuit(t *testing.T) {
	for _, commandLine := range []bool{false, true} {
		name := "page"
		if commandLine {
			name = "command-line"
		}
		t.Run(name, func(t *testing.T) { testUninstallCloseGuard(t, commandLine) })
	}
}

func testUninstallCloseGuard(t *testing.T, commandLine bool) {
	e, s, _ := fixture(t)
	tool := filepath.Join(e.Root, "LocalAppData", "RimArcho", "steamcmd", "steamcmd.exe")
	e.WriteFile(tool, "synthetic executable")
	reached, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	e.App.AfterExitFunc = func(after app.AfterExit) error {
		for _, group := range [][]string{after.Dirs, after.Files, after.Empty} {
			for _, p := range group {
				if !source.Within(p, e.Root) {
					return errors.New("cleanup target escaped synthetic fixture")
				}
			}
		}
		close(reached)
		<-release
		return nil // never start the real cleanup script
	}
	blocked := 0
	s.emit = func(name string, data any) {
		if name == "uninstall-close-blocked" {
			blocked++
		}
		if name == "uninstall" && data.(UninstallStatus).Done {
			close(done)
		}
	}
	quit := make(chan struct{}, 1)
	s.quit = func() { quit <- struct{}{} }
	if commandLine {
		out, err := s.RunCLI("archo uninstall --yes")
		if err != nil || !out.Uninstall {
			t.Fatalf("execution did not enter guarded worker: %+v, %v", out, err)
		}
	} else if err := s.UninstallRun(view.UninstallChoice{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reached:
	case <-time.After(10 * time.Second):
		t.Fatal("synthetic uninstall did not reach cleanup hook")
	}
	if _, err := os.Stat(tool); !os.IsNotExist(err) {
		t.Fatalf("synthetic tool was not removed: %v", err)
	}
	st := s.UninstallStatus()
	if len(st.Steps) == 0 || st.Steps[0].Key != app.UnAccounts || st.Steps[0].Title == "" || st.StepKey == "" {
		t.Fatalf("reload lost progress steps: %+v", st)
	}
	if !st.Running {
		t.Fatal("missing backend running state")
	}
	for i := 0; i < 4; i++ { // every native/custom close uses the same Wails hook
		ev := application.NewWindowEvent()
		s.guardWindowClose(ev)
		if !ev.IsCancelled() {
			t.Fatal("native close was not cancelled")
		}
	}
	if s.Quit() {
		t.Fatal("API quit bypassed guard")
	}
	if blocked != 5 {
		t.Fatalf("close notices = %d", blocked)
	}
	if err := s.UninstallRun(view.UninstallChoice{}); err == nil {
		t.Fatal("duplicate uninstall accepted")
	}
	unblock()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("completion was not delivered")
	}
	status := s.UninstallStatus()
	if status.Running || !status.Done || status.Page == nil || !status.Page.Ran || status.Page.Failed != 0 {
		t.Fatalf("lost report: %+v", status)
	}
	ev := application.NewWindowEvent()
	s.guardWindowClose(ev)
	if ev.IsCancelled() || !s.Quit() {
		t.Fatal("close still blocked after completion")
	}
	select {
	case <-quit:
	case <-time.After(time.Second):
		t.Fatal("quit not called")
	}
}

func TestUninstallFailureReleasesCloseGuard(t *testing.T) {
	e, s, _ := fixture(t)
	done := make(chan struct{})
	e.App.AfterExitFunc = func(app.AfterExit) error { return errors.New("synthetic cleanup failure") }
	s.emit = func(name string, data any) {
		if name == "uninstall" && data.(UninstallStatus).Done {
			close(done)
		}
	}
	if err := s.UninstallRun(view.UninstallChoice{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("uninstall did not finish")
	}
	if st := s.UninstallStatus(); st.Running || st.Page == nil || st.Page.Failed == 0 {
		t.Fatalf("missing failed report: %+v", st)
	}
	if !s.allowClose() {
		t.Fatal("failure retained close guard")
	}
}

func TestCloseBeforeUninstallPreventsStartup(t *testing.T) {
	_, s, _ := fixture(t)
	if !s.allowClose() {
		t.Fatal("normal close changed")
	}
	if err := s.UninstallRun(view.UninstallChoice{}); err == nil {
		t.Fatal("uninstall started after close was accepted")
	}
}
