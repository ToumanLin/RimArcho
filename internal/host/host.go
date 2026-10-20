// Package host runs the TUI inside RimArcho's WezTerm host (06 §1.1): it finds WezTerm,
// offers to download the portable build when there is none (only after the player says
// yes, verified against the release's sha256), writes wezterm.lua and starts
// `wezterm-gui start -- archo tui`. A "no" is remembered in terminal.host. Only a bare
// `archo` reaches this package: `archo tui`, `archo --here`, CLI commands and MCP never do.
package host

import (
	"archive/zip"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/wezterm"
)

// GitHubAPI is where the latest stable release is looked up (GET /repos/wezterm/wezterm/releases/latest).
const GitHubAPI = "https://api.github.com"

// maxDownload bounds the zip (the portable build is about 66 MB).
const maxDownload = 300 << 20

// Host is the environment of the decision; tests replace every outside effect.
type Host struct {
	Home       string   // the data directory: wezterm/ and wezterm.lua go here
	ProgramDir string   // the directory of archo.exe: the bundled fonts are in fonts/
	Exe        string   // archo itself, started inside WezTerm
	Args       []string // passed on to `archo tui` (for example --env <name>)
	API        string   // GitHubAPI, or a test server
	HTTP       *http.Client
	// In is the console to ask on; nil when there is none (output redirected, a pipe): then
	// RimArcho never asks and runs in the current terminal.
	In           io.Reader
	Out          io.Writer
	Getenv       func(string) string
	LookPath     func(string) (string, error)
	ProgramFiles []string // where an installed WezTerm may be
	Start        func(name string, args ...string) error
	Pref         func() string // terminal.host: auto (not asked yet), wezterm, none
	SetPref      func(string) error
}

// Run decides where the TUI runs. launched is true when it was started in WezTerm (the
// caller then exits); false means run the TUI here, in compatibility mode if needed.
func (h *Host) Run() (launched bool, err error) {
	if h.Getenv("TERM_PROGRAM") == "WezTerm" {
		return false, nil // already inside WezTerm
	}
	pref := h.Pref()
	if pref == "none" {
		return false, nil
	}
	gui := h.Find()
	if gui == "" {
		if h.In == nil {
			return false, nil // nobody to ask
		}
		if !h.ask() {
			fmt.Fprintln(h.Out, "OK — running here without icons and previews. To change this later: archo config set terminal.host wezterm")
			return false, h.SetPref("none")
		}
		if gui, err = h.Download(); err != nil {
			fmt.Fprintln(h.Out, "WezTerm download failed:", err)
			fmt.Fprintln(h.Out, "Running here instead.")
			return false, nil
		}
		if err := h.SetPref("wezterm"); err != nil {
			return false, err
		}
	}
	lua := filepath.Join(h.Home, "wezterm.lua")
	if err := os.WriteFile(lua, []byte(wezterm.Config(filepath.Join(h.ProgramDir, "fonts"))), 0o644); err != nil {
		return false, err
	}
	args := append([]string{"start", "--config-file", lua, "--", h.Exe, "tui"}, h.Args...)
	if err := h.Start(gui, args...); err != nil {
		fmt.Fprintln(h.Out, "could not start WezTerm:", err)
		return false, nil
	}
	return true, nil
}

func (h *Host) ask() bool {
	fmt.Fprint(h.Out, "Download the WezTerm portable build (about 66 MB) to show icons and mod previews? [Y/n] ")
	line, _ := bufio.NewReader(h.In).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "", "y", "yes", "是":
		return true
	}
	return false
}

// Find looks for wezterm-gui: on PATH, in Program Files, then in <data dir>/wezterm/.
func (h *Host) Find() string {
	if p, err := h.LookPath("wezterm-gui"); err == nil {
		return p
	}
	for _, pf := range h.ProgramFiles {
		if p := filepath.Join(pf, "WezTerm", "wezterm-gui.exe"); isFile(p) {
			return p
		}
	}
	dir := filepath.Join(h.Home, "wezterm")
	if p := filepath.Join(dir, "wezterm-gui.exe"); isFile(p) {
		return p
	}
	ms, _ := filepath.Glob(filepath.Join(dir, "*", "wezterm-gui.exe"))
	if len(ms) > 0 {
		return ms[len(ms)-1] // the newest name sorts last (WezTerm-windows-<date>-…)
	}
	return ""
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

type release struct {
	Tag    string `json:"tag_name"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

var portableRe = regexp.MustCompile(`^WezTerm-windows-[0-9A-Za-z._-]+\.zip$`)

// Download fetches the latest stable portable zip, checks it against the release's .sha256
// file and extracts it into <data dir>/wezterm/. It returns wezterm-gui's path.
func (h *Host) Download() (string, error) {
	var rel release
	if err := h.getJSON(strings.TrimRight(h.API, "/")+"/repos/wezterm/wezterm/releases/latest", &rel); err != nil {
		return "", fmt.Errorf("release lookup: %w", err)
	}
	var zipURL, shaURL, zipName string
	var size int64
	for _, a := range rel.Assets {
		if portableRe.MatchString(a.Name) {
			zipURL, zipName, size = a.URL, a.Name, a.Size
		}
	}
	for _, a := range rel.Assets {
		if zipName != "" && a.Name == zipName+".sha256" {
			shaURL = a.URL
		}
	}
	if zipURL == "" || shaURL == "" {
		return "", fmt.Errorf("release %s has no portable Windows zip with a .sha256", rel.Tag)
	}
	want, err := h.getSHA(shaURL)
	if err != nil {
		return "", fmt.Errorf("checksum: %w", err)
	}
	dest := filepath.Join(h.Home, "wezterm")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", err
	}
	tmp := filepath.Join(dest, zipName+".part")
	defer os.Remove(tmp)
	fmt.Fprintf(h.Out, "downloading %s (%s, %.0f MB)…\n", zipName, rel.Tag, float64(size)/(1<<20))
	got, err := h.fetch(zipURL, tmp)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(got, want) {
		return "", fmt.Errorf("sha256 mismatch: got %s, the release says %s", got, want)
	}
	fmt.Fprintln(h.Out, "sha256 verified; extracting…")
	if err := extract(tmp, dest); err != nil {
		return "", err
	}
	if gui := h.Find(); gui != "" && strings.HasPrefix(gui, dest) {
		return gui, nil
	}
	return "", errors.New("the zip has no wezterm-gui.exe")
}

func (h *Host) get(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "RimArcho (+https://github.com/ToumanLin/RimArcho)")
	req.Header.Set("Accept", "application/vnd.github+json, */*")
	c := h.HTTP
	if c == nil {
		c = &http.Client{Timeout: 10 * time.Minute}
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return resp, nil
}

func (h *Host) getJSON(url string, v any) error {
	resp, err := h.get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(v)
}

var hexRe = regexp.MustCompile(`(?i)\b[0-9a-f]{64}\b`)

// getSHA reads a .sha256 file ("<hex>  <name>" or just "<hex>").
func (h *Host) getSHA(url string) (string, error) {
	resp, err := h.get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	s := hexRe.FindString(string(b))
	if s == "" {
		return "", errors.New("no sha256 in the checksum file")
	}
	return s, nil
}

// fetch downloads url into path and returns its sha256.
func (h *Host) fetch(url, path string) (string, error) {
	resp, err := h.get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	sum := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, sum), io.LimitReader(resp.Body, maxDownload+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	if n > maxDownload {
		return "", errors.New("download larger than expected")
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// extract unpacks a zip into dest; an entry that would land outside dest fails it.
func extract(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	root, _ := filepath.Abs(dest)
	for _, f := range r.File { // check every entry before writing any
		p := filepath.Join(root, filepath.FromSlash(f.Name))
		if p != root && !strings.HasPrefix(p, root+string(filepath.Separator)) {
			return fmt.Errorf("zip entry outside the target: %s", f.Name)
		}
	}
	for _, f := range r.File {
		p := filepath.Join(root, filepath.FromSlash(f.Name))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := writeEntry(f, p); err != nil {
			return err
		}
	}
	return nil
}

func writeEntry(f *zip.File, p string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(p)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, io.LimitReader(rc, maxDownload)); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
