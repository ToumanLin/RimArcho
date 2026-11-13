package app_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/lanzou"
	"github.com/ToumanLin/RimArcho/internal/selfupdate"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

type selfMirror struct {
	version              string
	listErr, downloadErr error
	data                 []byte
	sum                  []byte
	want                 string
	lists, downloads     int
	list                 func(context.Context)
}

func (m *selfMirror) List(ctx context.Context) (lanzou.Listing, error) {
	m.lists++
	if m.list != nil {
		m.list(ctx)
	}
	return lanzou.Listing{Version: m.version}, m.listErr
}

func (m *selfMirror) Download(ctx context.Context, version, dir string, sum []byte) (*lanzou.Package, error) {
	m.downloads++
	m.sum, m.want = sum, version
	if m.downloadErr != nil {
		return nil, m.downloadErr
	}
	path := filepath.Join(dir, selfupdate.ZipName(version, "windows", "amd64"))
	if err := os.WriteFile(path, m.data, 0600); err != nil {
		return nil, err
	}
	if sum != nil {
		if err := selfupdate.VerifyLocal(ctx, path, filepath.Base(path), sum, 0); err != nil {
			_ = os.Remove(path)
			return nil, err
		}
	}
	return &lanzou.Package{ZipPath: path, Size: int64(len(m.data))}, nil
}

type selfTransport func(*http.Request) (*http.Response, error)

func (f selfTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSelfUpdateChannels(t *testing.T) {
	if !app.SelfUpdateSupported() {
		t.Skip("Windows update channels")
	}
	t.Setenv("RIMARCHO_UPDATE_MONITOR_ITEM", "8100")
	for _, winner := range []string{"github", "monitor", "lanzou", "tie"} {
		t.Run(winner, func(t *testing.T) {
			e := testkit.New(t)
			f := newSelfRelease(t)
			f.use(e.App)
			e.App.SteamAPI = f.srv.URL + "/workshop"
			m := &selfMirror{version: "0.9.0"}
			e.App.SelfLanzou = m
			want, source := "0.9.1", "GitHub"
			switch winner {
			case "monitor":
				f.description = selfupdate.Stamp("Monitor", "0.9.2")
				want, source = "0.9.2", "Monitor"
			case "lanzou":
				m.version = "0.9.3"
				want, source = "0.9.3", "Lanzou"
			case "tie":
				m.version = "0.9.1"
				source = "GitHub, Monitor, Lanzou"
			default:
				f.description = selfupdate.Stamp("Monitor", "0.9.0")
			}
			// All three must enter before any can finish: sequential checks time out.
			started := make(chan struct{}, 3)
			barrier := func(ctx context.Context) {
				started <- struct{}{}
				for len(started) < 3 {
					select {
					case <-ctx.Done():
						return
					case <-time.After(time.Millisecond):
					}
				}
			}
			m.list = barrier
			base := f.srv.Client().Transport
			if base == nil {
				base = http.DefaultTransport
			}
			e.App.HTTP = &http.Client{Transport: selfTransport(func(r *http.Request) (*http.Response, error) { barrier(r.Context()); return base.RoundTrip(r) })}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if _, _, err := e.App.CheckSelfUpdate(ctx); err != nil {
				t.Fatal(err)
			}
			info := e.App.SelfUpdateInfo()
			if info.Release == nil || info.Release.Version != want || info.Source != source || len(info.Sources) != 3 || len(started) != 3 {
				t.Fatalf("%+v", info)
			}
			for _, result := range info.Sources {
				if result.Status != "ok" {
					t.Fatal(result)
				}
			}
			if winner == "github" || winner == "tie" {
				if !strings.Contains(info.Release.Notes, "fixed things") || info.Release.Zip.Size != int64(len(f.zip)) {
					t.Fatal("lost GitHub release", info.Release)
				}
			} else if !info.Release.Mirror || info.Release.Installable() != nil || info.Release.Page != "https://github.com/ToumanLin/RimArcho/releases/tag/v"+want {
				t.Fatal(info.Release)
			}
		})
	}
}

func TestSelfUpdateChannelRoutingAndOffline(t *testing.T) {
	if !app.SelfUpdateSupported() {
		t.Skip("Windows update channels")
	}
	t.Setenv("RIMARCHO_UPDATE_MONITOR_ITEM", "")
	for _, mode := range []string{"unconfigured", "github", "prerelease", "all fail"} {
		t.Run(mode, func(t *testing.T) {
			e := testkit.New(t)
			f := newSelfRelease(t)
			f.use(e.App)
			e.App.SteamAPI = f.srv.URL + "/workshop"
			m := &selfMirror{version: "0.9.2"}
			e.App.SelfLanzou = m
			if mode == "github" {
				e.MustRun("config", "set", "updates.source", "github")
			}
			if mode == "prerelease" {
				e.App.SelfVersion = "0.9.1-rc1"
			}
			if _, _, err := e.App.CheckSelfUpdate(context.Background()); err != nil {
				t.Fatal(err)
			}
			info := e.App.SelfUpdateInfo()
			if mode == "github" || mode == "prerelease" {
				if len(info.Sources) != 1 || m.lists != 0 || f.workshopHits.Load() != 0 || info.Source != "GitHub" {
					t.Fatal(info, m.lists)
				}
			} else if info.Sources[1].Status != "unconfigured" || f.workshopHits.Load() != 0 {
				t.Fatal(info)
			}
			if mode == "all fail" {
				f.githubDown = true
				m.listErr = errors.New("offline")
				_, state, err := e.App.CheckSelfUpdate(context.Background())
				after := e.App.SelfUpdateInfo()
				if err == nil || state != "check unavailable offline" || after.Release.Version != info.Release.Version || after.Source != info.Source || len(after.Sources) != 3 {
					t.Fatal(after, err)
				}
				out := e.MustRun("update", "--check")
				for _, text := range []string{"github: failed", "monitor: not configured", "lanzou: failed", "latest: 0.9.2", "check unavailable offline"} {
					if !strings.Contains(out, text) {
						t.Fatal(out)
					}
				}
			}
		})
	}
}

func TestSelfUpdateNotificationAcrossChannels(t *testing.T) {
	if !app.SelfUpdateSupported() {
		t.Skip("Windows update channels")
	}
	t.Setenv("RIMARCHO_UPDATE_MONITOR_ITEM", "")
	e := testkit.New(t)
	f := newSelfRelease(t)
	f.use(e.App)
	m := &selfMirror{version: "0.9.1"}
	e.App.SelfLanzou = m
	if _, err := e.App.RunAutomaticNow(context.Background(), "self"); err != nil {
		t.Fatal(err)
	}
	before := e.App.AutomaticStatus("self")
	f.githubDown = true
	if _, err := e.App.RunAutomaticNow(context.Background(), "self"); err != nil {
		t.Fatal(err)
	}
	if text := view.AutomaticNotice(e.App, before, e.App.AutomaticStatus("self")); text != "" {
		t.Fatal("duplicate notice", text)
	}
	m.version = "0.9.2"
	if _, err := e.App.RunAutomaticNow(context.Background(), "self"); err != nil {
		t.Fatal(err)
	}
	if text := view.AutomaticNotice(e.App, before, e.App.AutomaticStatus("self")); text == "" {
		t.Fatal("new version had no notice")
	}
}

func TestSelfUpdateMirrorInstall(t *testing.T) {
	if !app.SelfUpdateSupported() {
		t.Skip("Windows replacement")
	}
	t.Setenv("RIMARCHO_UPDATE_MONITOR_ITEM", "")
	for _, mode := range []string{"GitHub", "GitHub sum", "no sum", "missing sidecar asset", "metadata offline", "bad sum", "corrupt ZIP", "both fail", "github only"} {
		t.Run(mode, func(t *testing.T) {
			e := testkit.New(t)
			f := newSelfRelease(t)
			f.use(e.App)
			m := &selfMirror{version: "0.9.1", data: f.zip}
			e.App.SelfLanzou = m
			if mode == "missing sidecar asset" {
				f.missingSum = true
			}
			if _, _, err := e.App.CheckSelfUpdate(context.Background()); err != nil {
				t.Fatal(err)
			}
			f.zipDown = mode != "GitHub"
			if mode == "no sum" {
				f.sumDown = true
			}
			if mode == "metadata offline" {
				f.tagDown = true
			}
			if mode == "bad sum" {
				f.sum = strings.Repeat("0", 64)
			}
			if mode == "both fail" {
				m.downloadErr = errors.New("human verification required")
			}
			if mode == "github only" {
				e.MustRun("config", "set", "updates.source", "github")
			}
			if mode == "corrupt ZIP" {
				f.sumDown = true
				m.data = append([]byte(nil), f.zip...)
				zr, err := zip.NewReader(bytes.NewReader(m.data), int64(len(m.data)))
				if err != nil {
					t.Fatal(err)
				}
				offset, err := zr.File[0].DataOffset()
				if err != nil {
					t.Fatal(err)
				}
				m.data[offset] ^= 0xff
			}
			e.WriteFile(filepath.Join(e.App.LocalDir(), "steamcmd-self", "legacy"), "old")
			var phases []string
			res, err := e.App.InstallSelfUpdate(context.Background(), func(s app.SelfUpdateStep) { phases = append(phases, s.Phase) })
			failure := mode == "bad sum" || mode == "both fail" || mode == "corrupt ZIP" || mode == "github only"
			if failure {
				if err == nil || f.files(t) != allWith("old") {
					t.Fatal(res, err)
				}
				if mode != "corrupt ZIP" {
					if !errors.Is(err, app.ErrSelfDownloadManual) || e.App.SelfUpdateInfo().Manual != "0.9.1" {
						t.Fatal(err, e.App.SelfUpdateInfo())
					}
					for _, text := range []string{lanzou.DefaultShareURL, "password ctot", "archo-0.9.1-windows-amd64.zip", "archo update --from <path>"} {
						if !strings.Contains(err.Error(), text) {
							t.Fatal(err)
						}
					}
				}
			} else if err != nil || res.To != "0.9.1" || f.files(t) != allWith("new") || e.App.SelfUpdateInfo().Manual != "" {
				t.Fatal(res, err)
			}
			if mode == "GitHub" || mode == "github only" {
				if m.downloads != 0 {
					t.Fatal("unnecessary Lanzou download")
				}
			} else if m.downloads != 1 || m.want != "0.9.1" {
				t.Fatal(m)
			}
			if mode == "GitHub sum" && string(m.sum) != f.sum {
				t.Fatal("lost GitHub checksum")
			}
			if (mode == "no sum" || mode == "missing sidecar asset" || mode == "metadata offline") && m.sum != nil {
				t.Fatal("unexpected checksum")
			}
			if !failure {
				if _, err := os.Stat(filepath.Join(e.App.LocalDir(), "steamcmd-self")); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("legacy updater retained", err)
				}
			}
			if mode == "both fail" {
				f.zipDown = false
				if _, err := e.App.InstallSelfUpdate(context.Background(), nil); err != nil {
					t.Fatal(err)
				}
				if e.App.SelfUpdateInfo().Manual != "" {
					t.Fatal("manual state retained after install")
				}
			}
		})
	}
}

func TestSelfUpdateManualInstall(t *testing.T) {
	if !app.SelfUpdateSupported() {
		t.Skip("Windows replacement")
	}
	for _, mode := range []string{"renamed", "browser suffix", "own sidecar", "canonical sidecar", "GitHub sum", "CRC only", "wrong sidecar", "wrong version", "downgrade", "CLI", "Workshop"} {
		t.Run(mode, func(t *testing.T) {
			e := testkit.New(t)
			f := newSelfRelease(t)
			f.use(e.App)
			// Manual selection works without a check when its filename names a version.
			if mode == "renamed" {
				b, _ := json.Marshal(app.SelfUpdate{Release: &selfupdate.Release{Version: "0.9.1"}})
				if err := e.App.Store.SetMeta("self-update", string(b)); err != nil {
					t.Fatal(err)
				}
			}
			name := "archo-0.9.1-windows-amd64.zip"
			if mode == "renamed" {
				name = "download.zip"
			}
			if mode == "browser suffix" {
				name = "archo-0.9.1-windows-amd64 (1).zip"
			}
			if mode == "wrong version" {
				name = "archo-0.9.2-windows-amd64.zip"
			}
			if mode == "downgrade" {
				name = "archo-0.8.0-windows-amd64.zip"
			}
			picked := filepath.Join(t.TempDir(), name)
			if err := os.WriteFile(picked, f.zip, 0600); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "own sidecar", "wrong sidecar":
				sum := f.sum
				if mode == "wrong sidecar" {
					sum = strings.Repeat("0", 64)
				}
				if err := os.WriteFile(picked+".sha256", []byte(sum), 0600); err != nil {
					t.Fatal(err)
				}
			case "canonical sidecar":
				picked = filepath.Join(filepath.Dir(picked), "archo-0.9.1-windows-amd64 (2).zip")
				if err := os.WriteFile(picked, f.zip, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(filepath.Dir(picked), name+".sha256"), []byte(f.sum), 0600); err != nil {
					t.Fatal(err)
				}
			case "CRC only":
				f.tagDown = true
			case "Workshop":
				e.App.SelfProgram = filepath.Join(t.TempDir(), "steamapps", "workshop", "content", "294100", "8100")
			}
			var err error
			if mode == "CLI" {
				out := e.MustRun("update", "--from", picked)
				if !strings.Contains(out, "updated RimArcho 0.9.0") || f.stableHits.Load() != 0 {
					t.Fatal(out)
				}
			} else {
				_, err = e.App.InstallSelfUpdateFrom(context.Background(), picked, nil)
			}
			failure := mode == "wrong sidecar" || mode == "wrong version" || mode == "downgrade" || mode == "Workshop"
			if failure {
				if err == nil || f.files(t) != allWith("old") {
					t.Fatal(err)
				}
				if mode == "wrong version" && !strings.Contains(err.Error(), "was put back") {
					t.Fatal(err)
				}
			} else if err != nil || f.files(t) != allWith("new") {
				t.Fatal(err)
			}
			got, err := os.ReadFile(picked)
			if err != nil || !bytes.Equal(got, f.zip) {
				t.Fatal("selected file changed", err)
			}
			if f.zipHits.Load() != 0 {
				t.Fatal("manual install downloaded ZIP")
			}
		})
	}
}

func TestSelfUpdateManualClearedByNewerCheckAndLegacyMeta(t *testing.T) {
	if !app.SelfUpdateSupported() {
		t.Skip("Windows update channels")
	}
	t.Setenv("RIMARCHO_UPDATE_MONITOR_ITEM", "")
	e := testkit.New(t)
	f := newSelfRelease(t)
	f.use(e.App)
	if err := e.App.Store.SetMeta("self-update", `{"manual":"0.9.1","release":{"version":"0.9.1","workshop_item":"3835688284"}}`); err != nil {
		t.Fatal(err)
	}
	if e.App.SelfUpdatePending() == nil {
		t.Fatal("legacy meta did not decode")
	}
	e.App.SelfLanzou = &selfMirror{version: "0.9.1"}
	if _, _, err := e.App.CheckSelfUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e.App.SelfUpdateInfo().Manual != "0.9.1" {
		t.Fatal("same version cleared manual state")
	}
	e.App.SelfLanzou = &selfMirror{version: "0.9.2"}
	if _, _, err := e.App.CheckSelfUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e.App.SelfUpdateInfo().Manual != "" {
		t.Fatal("new version retained manual state")
	}
}
