//go:build windows || cgo

package gui

import (
	"context"
	"errors"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// SteamCMD's prompts in the window (11 §3 "Password prompt"): when SteamCMD's own window
// asks for the password or a Steam Guard code, the window's corner card asks the player
// ("steamcmd-prompt") and SteamCMDAnswer hands the answer to the waiting run, which types it
// into SteamCMD's window. Nothing of it is kept: no task, no operation log, no setting.
type steamPrompts struct {
	s    *API
	mu   sync.Mutex
	kind string
	wait chan *app.SteamSecret
}

func (p *steamPrompts) Secret(ctx context.Context, kind string) *app.SteamSecret {
	ch := make(chan *app.SteamSecret, 1)
	p.mu.Lock()
	p.kind, p.wait = kind, ch
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		if p.wait == ch {
			p.kind, p.wait = "", nil
		}
		p.mu.Unlock()
	}()
	p.s.send("steamcmd-prompt", map[string]any{"kind": kind})
	select {
	case v := <-ch:
		return v
	case <-ctx.Done():
		p.s.send("steamcmd-prompt", map[string]any{"kind": ""})
		return nil
	}
}

func (p *steamPrompts) Phone() { p.s.send("steamcmd-prompt", map[string]any{"kind": "phone"}) }

// SteamCMDAnswer answers the prompt SteamCMD's window shows: kind "password" or "guard",
// value the text to type; an empty value leaves it to the player in that window.
func (s *API) SteamCMDAnswer(kind, value string) error {
	p := s.prompts
	p.mu.Lock()
	ch := p.wait
	if ch == nil || p.kind != kind {
		p.mu.Unlock()
		return errors.New(view.T("SteamCMD is not asking for this any more"))
	}
	p.kind, p.wait = "", nil
	p.mu.Unlock()
	ch <- app.NewSteamSecret(value)
	return nil
}
