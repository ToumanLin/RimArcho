package feedback

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// The slow-check report (16 §2.14 "Slow checks"): after a can-it-run check that took longer
// than 30 s, the window or the terminal offers to send its timings, which the relay appends
// as a comment to one fixed issue (feedback-relay/README.md). Timings is all that goes:
// nothing names a Mod, a path, the player or a Steam account.
type Timings struct {
	Version   string        `json:"version"`        // RimArcho's
	OS        string        `json:"os"`             // System()
	Game      string        `json:"game,omitempty"` // the game version ("1.6")
	Active    int           `json:"active_mods"`
	Total     int           `json:"total_mods"`
	Automatic bool          `json:"automatic"` // an automatic check: low priority, two threads (16 §8.9)
	WallMs    int64         `json:"wall_ms"`   // from the start of the check to its findings on screen
	Phases    []TimingPhase `json:"phases"`
	Threads   int           `json:"cpu_threads,omitempty"`
	RAMGB     int           `json:"ram_gb,omitempty"`
	Disk      string        `json:"disk,omitempty"` // ssd, hdd: the disk of the game's Mods folder
}

// TimingPhase is one phase of a check and its milliseconds.
type TimingPhase struct {
	Name string `json:"name"`
	Ms   int64  `json:"ms"`
}

// Body is the request body: the preview shows these bytes, and Send posts them unchanged.
// Indented JSON with each phase on one line, so the preview stays short.
func (t Timings) Body() string {
	b, _ := json.MarshalIndent(t, "", "  ")
	return phaseLines.ReplaceAllString(string(b), `{"name": "$1", "ms": $2}`)
}

var phaseLines = regexp.MustCompile(`\{\s*"name": "([^"]*)",\s*"ms": (\d+)\s*\}`)

// TimingsURL is the relay's timings route beside its report route, "" when there is no relay.
func TimingsURL() string {
	r := RelayURL()
	if r == "" {
		return ""
	}
	return strings.TrimSuffix(strings.TrimSuffix(r, "/"), "/report") + "/timings"
}

// ErrNoTimingsRoute: the relay answered 404, so it has no timings route yet (or no issue
// for it). The caller says nothing to the player.
var ErrNoTimingsRoute = errors.New("feedback relay: no timings route")

// SendTimings posts body (Timings.Body) to the relay's timings route at endpoint.
func SendTimings(ctx context.Context, endpoint, agent, body string) error {
	if err := relayAddress(endpoint); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader([]byte(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", agent)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 16<<10))
	if res.StatusCode == http.StatusNotFound {
		return ErrNoTimingsRoute
	}
	if res.StatusCode != http.StatusOK {
		var out struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(b, &out)
		if out.Error == "" {
			out.Error = http.StatusText(res.StatusCode)
		}
		return &RelayError{Status: res.StatusCode, Code: out.Error}
	}
	return nil
}

// String is a short form for the process log.
func (t Timings) String() string {
	return fmt.Sprintf("%d ms, %d/%d Mods, %d phases", t.WallMs, t.Active, t.Total, len(t.Phases))
}
