package gui

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// SelfUpdateLine is the release waiting to be installed, for the navigation's update
// button (16 §2.1), or nil. It reads only the last check's record (17 §5.1).
func (s *API) SelfUpdateLine() *view.SelfUpdateLine {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.SelfUpdateHome(s.a)
}

// runSelfCheck is settings' "check for updates" (17 §5.1): recorded like the automatic check.
func (s *API) runSelfCheck(a *app.App) (string, error) {
	ran, err := a.RunAutomaticNow(s.branchCtx, "self")
	if !ran && err == nil {
		_, _, err = a.CheckSelfUpdate(s.branchCtx)
	}
	if err != nil {
		return "", err
	}
	return view.SelfCheckResult(a), nil
}

// runSelfUpdate installs the pending release as a background task the player can stop
// while it downloads or waits for the other RimArcho to close; then the window restarts.
func (s *API) runSelfUpdate(a *app.App, id string) (string, error) {
	ctx, cancel := context.WithCancel(s.branchCtx)
	defer cancel()
	s.tasks.SetStop(id, func() error { cancel(); return nil })
	res, err := a.InstallSelfUpdate(ctx, func(st app.SelfUpdateStep) {
		s.tasks.Update(id, func(t *app.Task) {
			t.Phase = view.SelfUpdatePhase(st)
			if st.Phase == "download" && st.Total > 0 {
				t.Frac = min(0.9, 0.9*float64(st.Done)/float64(st.Total))
			}
		})
	})
	if err != nil {
		return "", err
	}
	if !s.restartUpdated(res) {
		return view.Tf("updated to RimArcho %s; open RimArcho again", res.To), nil
	}
	return view.Tf("updated to RimArcho %s; restarting", res.To), nil
}

// restartUpdated starts the new program and closes this window a moment later, once the
// task's result is written. The new one waits for this process to exit before it removes
// the old files. The development harness has no window: the player reopens it.
func (s *API) restartUpdated(res *app.SelfUpdateResult) bool {
	if s.quit == nil {
		return false
	}
	c := exec.Command(app.SelfRestartProgram(res.Program), os.Args[1:]...)
	c.Env = append(os.Environ(), app.UpdatedFromEnv+"="+strconv.Itoa(os.Getpid()))
	if err := c.Start(); err != nil {
		return false
	}
	c.Process.Release()
	time.AfterFunc(1500*time.Millisecond, s.quit)
	return true
}
