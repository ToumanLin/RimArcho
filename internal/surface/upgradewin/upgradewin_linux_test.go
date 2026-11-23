package upgradewin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeTools puts shell scripts named zenity (and so on) first on PATH; each appends its
// arguments, then its input, to <dir>/<name>.log.
func fakeTools(t *testing.T, names ...string) string {
	dir := t.TempDir()
	for _, n := range names {
		script := "#!/bin/sh\necho \"$@\" >> \"" + filepath.Join(dir, n+".log") + "\"\n/bin/cat >> \"" + filepath.Join(dir, n+".log") + "\"\n"
		if err := os.WriteFile(filepath.Join(dir, n), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	t.Setenv("DISPLAY", ":99")
	t.Setenv("WAYLAND_DISPLAY", "")
	return dir
}

func logOf(t *testing.T, dir, name string) string {
	b, _ := os.ReadFile(filepath.Join(dir, name+".log"))
	return string(b)
}

func TestLinuxErrorPrefersZenity(t *testing.T) {
	dir := fakeTools(t, "zenity", "kdialog", "notify-send")
	ShowError("RimArcho", "could not open: x")
	if got := logOf(t, dir, "zenity"); !strings.Contains(got, "--error --title=RimArcho --text=could not open: x --no-markup") {
		t.Fatalf("zenity: %q", got)
	}
	if logOf(t, dir, "kdialog")+logOf(t, dir, "notify-send") != "" {
		t.Fatal("only one dialog")
	}
}

func TestLinuxErrorFallsBack(t *testing.T) {
	dir := fakeTools(t, "notify-send")
	ShowError("RimArcho", "x")
	if got := logOf(t, dir, "notify-send"); !strings.Contains(got, "--urgency=critical --app-name=RimArcho RimArcho x") {
		t.Fatalf("notify-send: %q", got)
	}
	t.Setenv("DISPLAY", "")
	ShowError("RimArcho", "no desktop") // nothing to show it on
	if strings.Contains(logOf(t, dir, "notify-send"), "no desktop") {
		t.Fatal("shown without a desktop")
	}
}

func TestLinuxProgress(t *testing.T) {
	dir := fakeTools(t, "zenity")
	w := Open("RimArcho", "Upgrading")
	if w == nil {
		t.Fatal("no window with zenity on PATH")
	}
	w.Update("Upgrading 50%", 0.5)
	w.Update("Upgrading 100%", 1)
	w.Close()
	w.Close()
	w.Update("after", 0.1) // ignored
	got := logOf(t, dir, "zenity")
	for _, want := range []string{"--progress --title=RimArcho --text=Upgrading --no-cancel", "# Upgrading 50%\n50\n", "# Upgrading 100%\n99\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	t.Setenv("PATH", t.TempDir())
	if Open("RimArcho", "x") != nil {
		t.Fatal("a window without zenity")
	}
}
