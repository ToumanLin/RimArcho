package cli

import (
	"bytes"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func stubMachine(t *testing.T, webview, wine, vt, own bool) {
	t.Helper()
	w, u, d, o := webView2Installed, underWine, terminalDrawsTUI, ownConsole
	t.Cleanup(func() { webView2Installed, underWine, terminalDrawsTUI, ownConsole = w, u, d, o })
	webView2Installed = func() bool { return webview }
	underWine = func() bool { return wine }
	terminalDrawsTUI = func() bool { return vt }
	ownConsole = func() bool { return own }
}

// Without WebView2 the window would never appear (Wails only logs), so RimArcho says why
// before starting it; under Wine it points to archo.exe in a Linux terminal (16 §8.1).
func TestGUIPreflight(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("WebView2 is Windows only")
	}
	view.SetLang("en")
	stubMachine(t, true, true, true, false)
	if err := guiPreflight(); err != nil {
		t.Fatalf("installed WebView2 (even under Wine) gets its chance: %v", err)
	}
	stubMachine(t, false, false, true, false)
	err := guiPreflight()
	if err == nil || !strings.Contains(err.Error(), "WebView2 Runtime, which is not installed") || !strings.Contains(windowFailureText(err), "https://developer.microsoft.com/microsoft-edge/webview2/") {
		t.Fatalf("missing runtime: %v", err)
	}
	stubMachine(t, false, true, true, false)
	err = guiPreflight()
	if err == nil || !strings.Contains(windowFailureText(err), "Wine or Proton") || !strings.Contains(windowFailureText(err), "wine archo.exe") {
		t.Fatalf("under Wine: %v", err)
	}
	if got := windowFailureText(errors.New("boom")); got != "RimArcho could not open its window:\nboom" {
		t.Fatalf("other failure: %q", got)
	}
}

// A console that would print the TUI's escape sequences as text gets plain lines and an
// error code instead of the TUI (16 §9.2).
func TestNoTUIConsole(t *testing.T) {
	t.Setenv("ARCHO_LANG", "en")
	defer view.SetLang("en")
	ap, err := app.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer ap.Close()
	for _, c := range []struct {
		wine bool
		want string
	}{{true, "wine archo.exe"}, {false, "open RimArcho.exe"}} {
		stubMachine(t, true, c.wine, false, false)
		var out bytes.Buffer
		if code := noTUI(ap, &out); code != cmds.ExitError {
			t.Fatalf("code %d", code)
		}
		s := out.String()
		if strings.Contains(s, "\x1b") || !strings.Contains(s, "cannot draw RimArcho's text interface") || !strings.Contains(s, c.want) || strings.Contains(s, "Press Enter") {
			t.Fatalf("wine=%v:\n%s", c.wine, s)
		}
	}
}
