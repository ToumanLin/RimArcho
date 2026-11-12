package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// archo.exe opened before any game was added: the screen says so in the interface's
// language and does not repeat its hints as a raw English error (VM round 2026-11-10).
func TestNoGameScreen(t *testing.T) {
	SetLang("zh")
	defer SetLang("en")
	m := &model{loadErr: app.ErrNoEnv, w: 100, h: 20}
	out := m.View()
	if strings.Contains(out, "no Game Config configured") || !strings.Contains(out, "\u8fd8\u6ca1\u6709\u6dfb\u52a0\u6e38\u620f\u3002") || !strings.Contains(out, "\u8bf7\u5148\u6dfb\u52a0\u6e38\u620f") {
		t.Fatalf("no-game screen:\n%s", out)
	}
	m = &model{loadErr: errors.New("database is broken"), w: 100, h: 20}
	if out := m.View(); !strings.Contains(out, "database is broken") {
		t.Fatalf("other errors are shown:\n%s", out)
	}
}
