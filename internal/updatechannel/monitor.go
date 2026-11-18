package updatechannel

import (
	"context"
	"encoding/json"
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

// This mirrors postWorkshopDetails' anonymous, single-item request without
// importing app. Tags, subscribed files and SteamCMD are deliberately unused.
func checkMonitor(ctx context.Context, options Options) SourceResult {
	id := monitorItem(options)
	if id == "" {
		return SourceResult{Status: "unconfigured", Error: "standalone Monitor item ID is not assigned"}
	}
	if !workshopID.MatchString(id) || id == "3835688284" {
		return SourceResult{Status: "failed", Error: "invalid or retired Monitor item ID"}
	}
	api := options.SteamAPI
	if api == "" {
		api = DefaultSteamAPI
	}
	client := shortHTTP(options.HTTP)
	form := url.Values{"itemcount": {"1"}, "publishedfileids[0]": {id}}
	req, err := http.NewRequestWithContext(ctx, "POST", api, strings.NewReader(form.Encode()))
	if err != nil {
		return SourceResult{Status: "failed", Error: "invalid Steam Web API endpoint"}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "RimArcho")
	resp, err := client.Do(req)
	if err != nil {
		return SourceResult{Status: "failed", Error: "Steam Web API request failed"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return SourceResult{Status: "failed", Error: fmt.Sprintf("Steam Web API returned HTTP %d", resp.StatusCode)}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return SourceResult{Status: "failed", Error: "Steam Web API response exceeds limit or could not be read"}
	}
	var data struct {
		Response struct {
			Details []struct {
				ID          string `json:"publishedfileid"`
				Result      int    `json:"result"`
				Description string `json:"description"`
			} `json:"publishedfiledetails"`
		} `json:"response"`
	}
	if json.Unmarshal(b, &data) != nil {
		return SourceResult{Status: "failed", Error: "invalid Steam Web API response"}
	}
	if len(data.Response.Details) != 1 || data.Response.Details[0].ID != id || data.Response.Details[0].Result != 1 {
		return SourceResult{Status: "failed", Error: "Monitor item is unavailable"}
	}
	version := selfupdate.DescriptionVersion(data.Response.Details[0].Description)
	if !validVersion(version, false) {
		return SourceResult{Status: "failed", Error: "Monitor description has no stable version stamp"}
	}
	return SourceResult{Status: "ok", Version: version}
}
