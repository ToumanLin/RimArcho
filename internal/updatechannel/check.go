// Package updatechannel coordinates release metadata sources (17 §5.1).
package updatechannel

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/ToumanLin/RimArcho/internal/lanzou"
	"github.com/ToumanLin/RimArcho/internal/selfupdate"
)

const MonitorWorkshopItem = "" // Assign only after a standalone Monitor item exists.
const MonitorItemEnv = "RIMARCHO_UPDATE_MONITOR_ITEM"
const DefaultSteamAPI = "https://api.steampowered.com/ISteamRemoteStorage/GetPublishedFileDetails/v1/"

type Options struct {
	GitHub                selfupdate.Client
	Lanzou                LanzouClient
	HTTP                  *http.Client // Steam Web API; no account/session is used.
	MonitorItem, SteamAPI string
	Source                string // auto (default) or github
	Prerelease            bool
	Timeout               time.Duration // One overall check deadline; default 30 s, maximum 60 s.
}

// LanzouClient allows synthetic app tests without relaxing Lanzou's network limits.
type LanzouClient interface {
	List(context.Context) (lanzou.Listing, error)
	Download(context.Context, string, string, []byte) (*lanzou.Package, error)
}

type SourceResult struct {
	Source      string        `json:"source"`
	Status      string        `json:"status"` // ok, failed, timeout, or unconfigured
	Version     string        `json:"version,omitempty"`
	HasChecksum bool          `json:"has_checksum"`
	Error       string        `json:"error,omitempty"`
	Elapsed     time.Duration `json:"elapsed_ns"`
	release     *selfupdate.Release
}

type Report struct {
	Version string              `json:"version,omitempty"`
	Sources []SourceResult      `json:"sources"`
	GitHub  *selfupdate.Release `json:"-"`
}

type checkTask struct {
	name string
	run  func(context.Context) SourceResult
}

// Check runs metadata sources concurrently. Results retain source order for
// archo update --check. Version choice does not reorder downloads.
func Check(ctx context.Context, options Options) (Report, error) {
	if options.Source != "" && options.Source != "auto" && options.Source != "github" {
		return Report{}, errors.New("unknown update source setting")
	}
	timeout := options.Timeout
	if timeout <= 0 || timeout > 60*time.Second {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	github := options.GitHub
	if github.GOOS == "" {
		github.GOOS = "windows"
	}
	if github.GOARCH == "" {
		github.GOARCH = "amd64"
	}
	tasks := []checkTask{{"github", func(ctx context.Context) SourceResult {
		github.HTTP = shortHTTP(github.HTTP)
		release, err := github.Latest(ctx, options.Prerelease)
		if err != nil {
			return SourceResult{Status: "failed", Error: "GitHub release lookup failed"}
		}
		if !validVersion(release.Version, options.Prerelease) {
			return SourceResult{Status: "failed", Error: "GitHub returned an invalid release version"}
		}
		return SourceResult{Status: "ok", Version: release.Version, HasChecksum: release.Sum.URL != "", release: release}
	}}}
	if options.Source != "github" && !options.Prerelease {
		tasks = append(tasks, checkTask{"monitor", func(ctx context.Context) SourceResult { return checkMonitor(ctx, options) }})
		tasks = append(tasks, checkTask{"lanzou", func(ctx context.Context) SourceResult {
			c := options.Lanzou
			if c == nil {
				c = &lanzou.Client{ShareURL: lanzou.DefaultShareURL, Password: lanzou.DefaultPassword}
			}
			listing, err := c.List(ctx)
			if err != nil {
				return SourceResult{Status: "failed", Error: "Lanzou folder listing failed"}
			}
			if listing.Version == "" {
				return SourceResult{Status: "failed", Error: "Lanzou folder has no stable Windows ZIP"}
			}
			return SourceResult{Status: "ok", Version: listing.Version, HasChecksum: listing.HasChecksum}
		}})
	}
	return collect(ctx, tasks)
}

func collect(ctx context.Context, tasks []checkTask) (Report, error) {
	type completion struct {
		index  int
		result SourceResult
	}
	done := make(chan completion, len(tasks))
	started := time.Now()
	report := Report{Sources: make([]SourceResult, len(tasks))}
	for i, task := range tasks {
		report.Sources[i] = SourceResult{Source: task.name, Status: "timeout", Error: "overall update-check deadline reached"}
		go func(i int, task checkTask) {
			start := time.Now()
			result := task.run(ctx)
			result.Source = task.name
			result.Elapsed = time.Since(start)
			done <- completion{i, result}
		}(i, task)
	}
	remaining := len(tasks)
	accept := func(c completion) { report.Sources[c.index] = c.result; remaining-- }
	for remaining > 0 {
		select {
		case result := <-done:
			accept(result)
		case <-ctx.Done():
			// Preserve completions already queued when the deadline fired.
			draining := true
			for draining {
				select {
				case result := <-done:
					accept(result)
				default:
					draining = false
				}
			}
			for i := range report.Sources {
				if report.Sources[i].Status == "timeout" {
					report.Sources[i].Elapsed = time.Since(started)
				}
			}
			remaining = 0
		}
	}
	for _, source := range report.Sources {
		if source.Source == "github" && source.Status == "ok" {
			report.GitHub = source.release
		}
		if source.Status == "ok" && (report.Version == "" || selfupdate.Newer(report.Version, source.Version)) {
			report.Version = source.Version
		}
	}
	if report.Version == "" {
		return report, errors.New("no update source returned a version")
	}
	return report, nil
}

// shortHTTP preserves netcfg's transport wrapper, including proxy/mirror routing.
// Check also bounds all sources with one context deadline (17 §2.2, §5.1).
func shortHTTP(base *http.Client) *http.Client {
	client := http.Client{}
	if base != nil {
		client = *base
	}
	client.Jar = nil
	if client.Timeout <= 0 || client.Timeout > 15*time.Second {
		client.Timeout = 15 * time.Second
	}
	return &client
}

func monitorItem(options Options) string {
	if options.MonitorItem != "" {
		return options.MonitorItem
	}
	if value, set := os.LookupEnv(MonitorItemEnv); set {
		return value
	}
	return MonitorWorkshopItem
}
