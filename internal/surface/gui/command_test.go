package gui

import (
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/cmds"
)

func waitCommand(t *testing.T, s *API) CommandResult {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r := s.CommandStatus()
		if !r.Running {
			return r
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("command did not finish")
	return CommandResult{}
}
func TestCommandRawOutputHistoryAndGuard(t *testing.T) {
	e, s, _ := fixture(t)
	s.emit = nil
	for _, line := range []string{"log --follow", "plan new", "no-such-command", "list --env"} {
		if _, err := s.RunCLI(line); err == nil {
			t.Errorf("accepted %q", line)
		}
	}
	want, code := cmds.Execute(e.App, []string{"list", "--active"})
	if r, err := s.RunCLI("archo list --active"); err != nil || !r.Running {
		t.Fatalf("start: %+v %v", r, err)
	}
	r := waitCommand(t, s)
	if r.Output != want || r.Code != code {
		t.Fatalf("raw output: %+v want %q exit %d", r, want, code)
	}
	h, err := s.CommandHistory(-1, -1, "draft", "")
	if err != nil || h.Text != "archo list --active" {
		t.Fatalf("history: %+v %v", h, err)
	}
	h, err = s.CommandHistory(h.At, 1, h.Text, h.Draft)
	if err != nil || h.Text != "draft" {
		t.Fatalf("draft: %+v %v", h, err)
	}
	if _, err = s.RunCLI(`plan new 'Two words'`); err != nil {
		t.Fatal(err)
	}
	r = waitCommand(t, s)
	if r.Code != 0 {
		t.Fatal(r)
	}
	if _, err = s.RunCLI(`plan remove 'Two words'`); err != nil {
		t.Fatal(err)
	}
	r = waitCommand(t, s)
	if r.Code == 0 || !strings.Contains(r.Output, "--yes") {
		t.Fatalf("destructive command lacked CLI confirmation: %+v", r)
	}
}
func TestCommandDoesNotHoldPageLock(t *testing.T) {
	_, s, _ := fixture(t)
	s.emit = nil
	entered, release := make(chan struct{}), make(chan struct{})
	s.ScanRunner = func(string) (string, error) { close(entered); <-release; return "raw child output\n", nil }
	if _, err := s.RunCLI("scan"); err != nil {
		t.Fatal(err)
	}
	<-entered
	if s.Scan() {
		t.Error("scan page started a duplicate command")
	}
	if _, err := s.RunCLI("version"); err == nil {
		t.Error("concurrent command accepted")
	}
	read := make(chan struct{})
	go func() { s.Setting("ui.gui.test"); close(read) }()
	select {
	case <-read:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("command held page lock")
	}
	close(release)
	r := waitCommand(t, s)
	if r.Output != "raw child output\n" || r.Code != 0 {
		t.Fatal(r)
	}
	s.closeCommand()
	if _, err := s.RunCLI("version"); err == nil {
		t.Fatal("command after close")
	}
}
func TestCommandSharedCompletions(t *testing.T) {
	_, s, _ := fixture(t)
	s.emit = nil
	for _, tc := range []struct{ line, want string }{{"sort --dr", "--dry-run"}, {"config set ui.icons ", "compat"}, {"why harm", "Harmony"}} {
		r, err := s.CommandComplete(tc.line, len([]rune(tc.line)), "", nil)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, c := range r.Candidates {
			if c.Insert == tc.want || c.Label == tc.want {
				found = true
			}
		}
		if !found {
			t.Errorf("%q: %+v", tc.line, r)
		}
	}
}
