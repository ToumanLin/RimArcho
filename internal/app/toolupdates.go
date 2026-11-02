package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/textures"
)

const steamCMDManifest = "https://client-update.akamai.steamstatic.com/steam_cmd_win32"

var steamVersionRE = regexp.MustCompile(`"version"\s*"([0-9]+)"`)

type ToolUpdate struct {
	Name       string    `json:"name"`
	Installed  string    `json:"installed"`
	Latest     string    `json:"latest"`
	Checked    time.Time `json:"checked"`
	Pending    bool      `json:"pending"`
	Compatible bool      `json:"compatible"`
	State      string    `json:"state"`
}

func (a *App) ToolUpdate(name string) ToolUpdate {
	s := ToolUpdate{Name: name}
	_ = json.Unmarshal([]byte(a.Store.Meta("tool-update:"+name)), &s)
	return s
}

func steamManifestVersion(b []byte) string {
	m := steamVersionRE.FindSubmatch(b)
	if len(m) > 1 {
		return string(m[1])
	}
	return ""
}

func (a *App) recordToolInstalled(name string) {
	s := a.ToolUpdate(name)
	if s.Checked.IsZero() {
		return
	}
	info := a.Deps().Status(name)
	installed := info.Version
	if name == deps.SteamCMD {
		if b, err := os.ReadFile(filepath.Join(info.Dir, "package", "steam_cmd_win32.installed")); err == nil {
			installed = steamManifestVersion(b)
		}
	}
	if installed == "" {
		return
	}
	s.Installed = installed
	s.Pending = newerToolVersion(installed, s.Latest)
	if !s.Pending {
		s.State = "up to date"
	}
	b, _ := json.Marshal(s)
	_ = a.Store.SetMeta("tool-update:"+name, string(b))
	status := a.AutomaticStatus("tools")
	status.Pending = 0
	for _, n := range []string{deps.SteamCMD, deps.Todds} {
		if a.ToolUpdate(n).Pending {
			status.Pending++
		}
	}
	b, _ = json.Marshal(status)
	_ = a.Store.SetMeta("automatic:tools", string(b))
}

// Reuses the dependency finder's cached version probes; never downloads a tool.
func (a *App) CheckToolUpdates(ctx context.Context) (int, string, error) {
	var failures []error
	pending := 0
	for _, name := range []string{deps.SteamCMD, deps.Todds} {
		if ctx.Err() != nil {
			return pending, "check interrupted", ctx.Err()
		}
		info := a.Deps().Status(name)
		if !info.Own {
			continue
		}
		s := a.ToolUpdate(name)
		s.Name, s.Installed, s.Checked = name, info.Version, a.Now().UTC()
		var err error
		var latest string
		if name == deps.SteamCMD {
			installed, e := os.ReadFile(filepath.Join(info.Dir, "package", "steam_cmd_win32.installed"))
			if e == nil {
				s.Installed = steamManifestVersion(installed)
			}
			var b []byte
			b, err = a.readToolRelease(ctx, steamCMDManifest)
			latest = steamManifestVersion(b)
		} else {
			var b []byte
			b, err = a.readToolRelease(ctx, "https://api.github.com/repos/todds-encoder/todds/releases/latest")
			var release struct {
				Tag        string `json:"tag_name"`
				Draft      bool   `json:"draft"`
				Prerelease bool   `json:"prerelease"`
			}
			if err == nil {
				err = json.Unmarshal(b, &release)
			}
			if release.Draft || release.Prerelease {
				err = errors.New("no stable release")
			}
			latest = release.Tag
		}
		if err == nil && latest == "" {
			err = errors.New("release has no version")
		}
		if err != nil {
			s.State = "check unavailable offline" // retain the last known release on failure
			failures = append(failures, err)
		} else {
			s.Latest = latest
			s.Pending = newerToolVersion(s.Installed, latest)
			s.Compatible = name != deps.Todds || strings.TrimPrefix(latest, "v") == textures.ToddsVersion
			s.State = "up to date"
			if s.Installed == "" {
				s.State = "installed version unknown"
			}
			if s.Pending {
				s.State = "new version available"
			}
		}
		if s.Pending {
			pending++
		}
		b, _ := json.Marshal(s)
		failures = append(failures, a.Store.SetMeta("tool-update:"+name, string(b)))
	}
	err := errors.Join(failures...)
	state := "check complete"
	if err != nil {
		state = "some checks failed"
	}
	return pending, state, err
}

func (a *App) readToolRelease(ctx context.Context, target string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := a.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release lookup: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if len(b) > 1<<20 {
		return nil, errors.New("release metadata too large")
	}
	return b, err
}

func newerToolVersion(installed, latest string) bool {
	installed = strings.TrimPrefix(strings.TrimSpace(installed), "v")
	latest = strings.TrimPrefix(latest, "v")
	if installed == "" || installed == latest {
		return false
	}
	numbers := func(s string) []int64 {
		var out []int64
		for _, v := range strings.Split(s, ".") {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil
			}
			out = append(out, n)
		}
		return out
	}
	x, y := numbers(installed), numbers(latest)
	if x == nil || y == nil {
		return false
	}
	for i := 0; i < max(len(x), len(y)); i++ {
		var a, b int64
		if i < len(x) {
			a = x[i]
		}
		if i < len(y) {
			b = y[i]
		}
		if a != b {
			return b > a
		}
	}
	return false
}
