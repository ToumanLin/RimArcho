//go:build windows || cgo

package gui

import (
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Slow checks (16 §2.14 "Slow checks"): after a can-it-run check over view.SlowCheckAfter,
// the "slow-check" event carries one prompt (view.SlowCheck) with the exact timings it would
// send. The page answers with SlowCheckSend or SlowCheckDismiss; what goes is what the
// prompt previewed, kept here, never text from the page.

// offerSlowCheck sends the prompt for the check clock timed, if it gets one.
func (s *API) offerSlowCheck(clock *app.CheckClock) {
	p := view.SlowCheckOf(s.a, clock, time.Since(clock.Began))
	if p == nil {
		return
	}
	s.mu.Lock()
	s.slowCheck = p.Preview
	s.mu.Unlock()
	s.send("slow-check", p)
}

// SlowCheckSend sends the previewed timings; always also sets feedback.timings to always.
// It answers "" when they went (or the relay has no timings route), else the reason.
func (s *API) SlowCheckSend(always bool) string {
	s.mu.Lock()
	body := s.slowCheck
	s.slowCheck = ""
	s.mu.Unlock()
	if always {
		if err := view.AlwaysSendSlowChecks(s.a); err != nil {
			return err.Error()
		}
	}
	if body == "" {
		return ""
	}
	if err := view.SendSlowCheck(s.a, body); err != nil {
		return view.T("could not send the timings")
	}
	return ""
}

// SlowCheckDismiss drops the prompt; it counted toward view.SlowCheckEvery when it showed.
func (s *API) SlowCheckDismiss() {
	s.mu.Lock()
	s.slowCheck = ""
	s.mu.Unlock()
}
