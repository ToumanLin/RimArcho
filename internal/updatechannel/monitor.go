package updatechannel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/selfupdate"
)

var stableVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var workshopID = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)

func validVersion(version string, prerelease bool) bool {
	_, ok := selfupdate.Compare(version, version)
	return ok && len(version) <= 64 && (prerelease || stableVersion.MatchString(version))
}

// MonitorItem is the standalone Monitor item's Workshop ID ("" until it is assigned).
func MonitorItem(options Options) string { return monitorItem(options) }

// MonitorUpdated reads the Monitor item's time_updated (Unix seconds) with the same
// anonymous request as the release check (19 §6 "anonymous fetch"). Unlike the release
// check it needs no version stamp: a data-only update keeps the Monitor's version.
func MonitorUpdated(ctx context.Context, options Options) (int64, error) {
	r, _, updated := monitorRequest(ctx, options)
	if r.Status != "" {
		return 0, errors.New(r.Error)
	}
	return updated, nil
}

// This mirrors postWorkshopDetails' anonymous, single-item request without
// importing app. Tags, subscribed files and SteamCMD are deliberately unused.
func checkMonitor(ctx context.Context, options Options) SourceResult {
	r, _ := monitorDetails(ctx, options)
	return r
}

func monitorDetails(ctx context.Context, options Options) (SourceResult, int64) {
	r, desc, updated := monitorRequest(ctx, options)
	if r.Status != "" {
		return r, 0
	}
	version := selfupdate.DescriptionVersion(desc)
	if !validVersion(version, false) {
		return SourceResult{Status: "failed", Error: "Monitor description has no stable version stamp"}, updated
	}
	return SourceResult{Status: "ok", Version: version}, updated
}

func monitorRequest(ctx context.Context, options Options) (SourceResult, string, int64) {
	id := monitorItem(options)
	if id == "" {
		return SourceResult{Status: "unconfigured", Error: "standalone Monitor item ID is not assigned"}, "", 0
	}
	if !workshopID.MatchString(id) || id == "3835688284" {
		return SourceResult{Status: "failed", Error: "invalid or retired Monitor item ID"}, "", 0
	}
	api := options.SteamAPI
	if api == "" {
		api = DefaultSteamAPI
	}
	client := shortHTTP(options.HTTP)
	form := url.Values{"itemcount": {"1"}, "publishedfileids[0]": {id}}
	req, err := http.NewRequestWithContext(ctx, "POST", api, strings.NewReader(form.Encode()))
	if err != nil {
		return SourceResult{Status: "failed", Error: "invalid Steam Web API endpoint"}, "", 0
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "RimArcho")
	resp, err := client.Do(req)
	if err != nil {
		return SourceResult{Status: "failed", Error: "Steam Web API request failed"}, "", 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return SourceResult{Status: "failed", Error: fmt.Sprintf("Steam Web API returned HTTP %d", resp.StatusCode)}, "", 0
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return SourceResult{Status: "failed", Error: "Steam Web API response exceeds limit or could not be read"}, "", 0
	}
	var data struct {
		Response struct {
			Details []struct {
				ID          string `json:"publishedfileid"`
				Result      int    `json:"result"`
				Description string `json:"description"`
				Updated     int64  `json:"time_updated"`
			} `json:"publishedfiledetails"`
		} `json:"response"`
	}
	if json.Unmarshal(b, &data) != nil {
		return SourceResult{Status: "failed", Error: "invalid Steam Web API response"}, "", 0
	}
	if len(data.Response.Details) != 1 || data.Response.Details[0].ID != id || data.Response.Details[0].Result != 1 {
		return SourceResult{Status: "failed", Error: "Monitor item is unavailable"}, "", 0
	}
	d := data.Response.Details[0]
	return SourceResult{}, d.Description, d.Updated
}
