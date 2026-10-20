package host

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGitHub serves a latest release with a portable zip and its .sha256, like
// github.com/wezterm/wezterm. Nothing in these tests reaches the internet.
type fakeGitHub struct {
	*httptest.Server
	zip       []byte
	sha       string
	downloads int
}

func newFakeGitHub(t *testing.T, entries map[string]string) *fakeGitHub {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for name, body := range entries {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	sum := sha256.Sum256(b.Bytes())
	f := &fakeGitHub{zip: b.Bytes(), sha: hex.EncodeToString(sum[:])}
	const name = "WezTerm-windows-20990101-000000-abcdef12.zip"
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/wezterm/wezterm/releases/latest":
			fmt.Fprintf(w, `{"tag_name":"20990101-000000-abcdef12","assets":[
				{"name":"WezTerm-20990101-000000-abcdef12-setup.exe","browser_download_url":"%[1]s/setup.exe","size":1},
				{"name":%[2]q,"browser_download_url":"%[1]s/zip","size":%[3]d},
				{"name":%[4]q,"browser_download_url":"%[1]s/sha","size":90}]}`, f.URL, name, len(f.zip), name+".sha256")
		case "/zip":
			f.downloads++
			w.Write(f.zip)
		case "/sha":
			fmt.Fprintf(w, "%s  %s\n", f.sha, name)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

var portable = map[string]string{
	"WezTerm-windows-20990101-000000-abcdef12/wezterm-gui.exe": "gui",
	"WezTerm-windows-20990101-000000-abcdef12/wezterm.exe":     "cli",
}

type started struct {
	name string
	args []string
}

func testHost(t *testing.T, api, answer string) (*Host, *[]started, *string, *bytes.Buffer) {
	home := t.TempDir()
	pref := ""
	var starts []started
	var out bytes.Buffer
	h := &Host{
		Home: home, ProgramDir: filepath.Join(home, "program"), Exe: `C:\RimArcho\archo.exe`, Args: []string{"--env", "main"},
		API: api, Out: &out, In: strings.NewReader(answer),
		Getenv:       func(string) string { return "" },
		LookPath:     func(string) (string, error) { return "", errors.New("not found") },
		ProgramFiles: []string{filepath.Join(home, "pf")},
		Start:        func(name string, args ...string) error { starts = append(starts, started{name, args}); return nil },
		Pref:         func() string { return pref },
		SetPref:      func(v string) error { pref = v; return nil },
	}
	return h, &starts, &pref, &out
}

// T10 (06 §11): first run without WezTerm. Yes downloads from the release, verifies the
// sha256, extracts into <data dir>/wezterm/, writes wezterm.lua and starts archo tui in it.
func TestT10FirstRunYes(t *testing.T) {
	gh := newFakeGitHub(t, portable)
	h, starts, pref, out := testHost(t, gh.URL, "\n") // enter = yes
	launched, err := h.Run()
	if err != nil || !launched {
		t.Fatalf("launched %v, err %v\n%s", launched, err, out)
	}
	if !strings.Contains(out.String(), "[Y/n]") || !strings.Contains(out.String(), "sha256 verified") {
		t.Fatalf("console:\n%s", out)
	}
	gui := filepath.Join(h.Home, "wezterm", "WezTerm-windows-20990101-000000-abcdef12", "wezterm-gui.exe")
	if len(*starts) != 1 || (*starts)[0].name != gui {
		t.Fatalf("started %+v", *starts)
	}
	want := []string{"start", "--config-file", filepath.Join(h.Home, "wezterm.lua"), "--", h.Exe, "tui", "--env", "main"}
	if got := (*starts)[0].args; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("args %q", got)
	}
	lua, err := os.ReadFile(filepath.Join(h.Home, "wezterm.lua"))
	if err != nil || !strings.Contains(string(lua), "CloseOnCleanExit") || !strings.Contains(string(lua), filepath.Join(h.ProgramDir, "fonts")) {
		t.Fatalf("wezterm.lua: %v\n%s", err, lua)
	}
	if *pref != "wezterm" {
		t.Fatalf("pref %q", *pref)
	}
	if ps, _ := filepath.Glob(filepath.Join(h.Home, "wezterm", "*.part")); len(ps) != 0 {
		t.Fatalf("partial download left: %v", ps)
	}
	// Second start: found in <data dir>/wezterm/, no question, no download.
	h.In = strings.NewReader("")
	out.Reset()
	if launched, _ := h.Run(); !launched || gh.downloads != 1 || strings.Contains(out.String(), "[Y/n]") {
		t.Fatalf("second run: launched %v, downloads %d\n%s", launched, gh.downloads, out)
	}
}

// No is remembered: the TUI runs here, and the next start does not ask again.
func TestT10FirstRunNo(t *testing.T) {
	gh := newFakeGitHub(t, portable)
	h, starts, pref, out := testHost(t, gh.URL, "n\n")
	if launched, err := h.Run(); launched || err != nil {
		t.Fatalf("launched %v, err %v", launched, err)
	}
	if *pref != "none" || len(*starts) != 0 || gh.downloads != 0 || !strings.Contains(out.String(), "terminal.host wezterm") {
		t.Fatalf("pref %q, starts %d, downloads %d\n%s", *pref, len(*starts), gh.downloads, out)
	}
	out.Reset()
	h.In = strings.NewReader("y\n")
	if launched, _ := h.Run(); launched || out.Len() != 0 {
		t.Fatalf("asked again after no:\n%s", out)
	}
}

// A wrong checksum: nothing is extracted, nothing starts, the TUI runs here.
func TestT10ChecksumMismatch(t *testing.T) {
	gh := newFakeGitHub(t, portable)
	gh.sha = strings.Repeat("0", 64)
	h, starts, _, out := testHost(t, gh.URL, "y\n")
	if launched, _ := h.Run(); launched || len(*starts) != 0 {
		t.Fatal("started after a checksum mismatch")
	}
	if !strings.Contains(out.String(), "sha256 mismatch") {
		t.Fatalf("console:\n%s", out)
	}
	if h.Find() != "" {
		t.Fatal("extracted despite the mismatch")
	}
}

// A zip entry that climbs out of the target is refused before anything is written.
func TestT10ZipSlip(t *testing.T) {
	gh := newFakeGitHub(t, map[string]string{
		"WezTerm-windows-x/wezterm-gui.exe": "gui",
		"../../evil.txt":                    "x",
	})
	h, starts, _, out := testHost(t, gh.URL, "y\n")
	if launched, _ := h.Run(); launched || len(*starts) != 0 || !strings.Contains(out.String(), "outside the target") {
		t.Fatalf("zip slip not refused:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(h.Home, "..", "evil.txt")); err == nil {
		t.Fatal("evil.txt written")
	}
	if h.Find() != "" {
		t.Fatal("partly extracted")
	}
}

// Inside WezTerm, with terminal.host none, or with no console to ask on, nothing is asked
// or downloaded; WezTerm found on PATH or in Program Files is used without a download.
func TestT10NoQuestion(t *testing.T) {
	gh := newFakeGitHub(t, portable)
	h, starts, _, out := testHost(t, gh.URL, "y\n")
	h.Getenv = func(k string) string {
		if k == "TERM_PROGRAM" {
			return "WezTerm"
		}
		return ""
	}
	if launched, _ := h.Run(); launched || out.Len() != 0 {
		t.Fatal("inside WezTerm: started or asked")
	}
	h, _, _, out = testHost(t, gh.URL, "y\n")
	h.In = nil
	if launched, _ := h.Run(); launched || out.Len() != 0 || gh.downloads != 0 {
		t.Fatal("no console: started or asked")
	}
	h, starts, _, out = testHost(t, gh.URL, "")
	pf := filepath.Join(h.ProgramFiles[0], "WezTerm", "wezterm-gui.exe")
	os.MkdirAll(filepath.Dir(pf), 0o755)
	os.WriteFile(pf, []byte("gui"), 0o644)
	if launched, _ := h.Run(); !launched || (*starts)[0].name != pf || gh.downloads != 0 || out.Len() != 0 {
		t.Fatalf("Program Files: launched %v, %+v, downloads %d", launched, *starts, gh.downloads)
	}
	h, starts, _, _ = testHost(t, gh.URL, "")
	h.LookPath = func(string) (string, error) { return `C:\bin\wezterm-gui.exe`, nil }
	if launched, _ := h.Run(); !launched || (*starts)[0].name != `C:\bin\wezterm-gui.exe` {
		t.Fatalf("PATH: %+v", *starts)
	}
}
