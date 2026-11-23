package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Replies (16 §2.14 Feedback, "my reports"): the relay hands back a token with each report,
// and GET /status/<n>?t=<token> on the relay returns the issue's state and the repository
// owner's comments only. RimArcho keeps the reports it sent in ReportsFile and asks at start
// and when the Feedback page opens; nothing is pushed and nothing is sent back.

// ReportsFile is the record of sent reports inside RimArcho's data directory (02 §1).
const ReportsFile = "feedback-reports.json"

// StopAfterClose is how long a closed issue is still checked, for a late reply.
const StopAfterClose = 30 * 24 * time.Hour

// Reply is one comment by the repository owner, as plain text (never rendered as HTML).
type Reply struct {
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// Status is what the relay says about one issue.
type Status struct {
	State    string     `json:"state"` // open or closed
	ClosedAt *time.Time `json:"closed_at"`
	Replies  []Reply    `json:"replies"`
}

// Record is one sent report: the token reads its replies; LastSeenReply is the newest reply
// the player has seen on the Feedback page. State, ClosedAt and Replies are the last answer.
type Record struct {
	Number        int        `json:"number"`
	Token         string     `json:"token"`
	Title         string     `json:"title"`
	URL           string     `json:"url,omitempty"`
	SentAt        time.Time  `json:"sent_at"`
	LastSeenReply time.Time  `json:"last_seen_reply"`
	State         string     `json:"state,omitempty"`
	ClosedAt      *time.Time `json:"closed_at,omitempty"`
	Replies       []Reply    `json:"replies,omitempty"`
	CheckedAt     time.Time  `json:"checked_at,omitempty"`
	Gone          bool       `json:"gone,omitempty"` // the relay no longer knows it (deleted, or 180 days passed)
}

// Unseen reports whether the record has a reply newer than the one the player saw.
func (r Record) Unseen() bool {
	for _, p := range r.Replies {
		if p.CreatedAt.After(r.LastSeenReply) {
			return true
		}
	}
	return false
}

// Due reports whether the record is still worth asking about at now.
func (r Record) Due(now time.Time) bool {
	if r.Token == "" || r.Gone {
		return false
	}
	return r.State != "closed" || r.ClosedAt == nil || now.Sub(*r.ClosedAt) < StopAfterClose
}

// LoadReports reads the records in dir, newest first; a missing or broken file is none.
func LoadReports(dir string) []Record {
	b, err := os.ReadFile(filepath.Join(dir, ReportsFile))
	if err != nil {
		return nil
	}
	var rs []Record
	if json.Unmarshal(b, &rs) != nil {
		return nil
	}
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].SentAt.After(rs[j].SentAt) })
	return rs
}

// SaveReports writes the records (a temporary file, then rename).
func SaveReports(dir string, rs []Record) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(rs, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, ReportsFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// StatusURL is the relay's status address for issue n: the report address with /report
// replaced, or "" when the relay address does not end in /report.
func StatusURL(relay string, n int, token string) string {
	base, ok := strings.CutSuffix(strings.TrimRight(relay, "/"), "/report")
	if !ok || n <= 0 || token == "" {
		return ""
	}
	return base + "/status/" + strconv.Itoa(n) + "?t=" + url.QueryEscape(token)
}

// ErrGone is the relay's 404: a wrong token, a deleted issue or an expired token.
var ErrGone = errors.New("feedback relay: no such report")

// FetchStatus asks the relay about issue n with its token. agent is "RimArcho/<version>".
func FetchStatus(ctx context.Context, relay, agent string, n int, token string) (Status, error) {
	endpoint := StatusURL(relay, n, token)
	u, err := url.Parse(endpoint)
	if endpoint == "" || err != nil || (u.Scheme != "https" && !(u.Scheme == "http" && loopback(u.Hostname()))) {
		return Status{}, fmt.Errorf("feedback relay address %q has no status", relay)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Status{}, err
	}
	req.Header.Set("User-Agent", agent)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return Status{}, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode == http.StatusNotFound {
		return Status{}, ErrGone
	}
	if res.StatusCode != http.StatusOK {
		return Status{}, &RelayError{Status: res.StatusCode, Code: http.StatusText(res.StatusCode)}
	}
	var s Status
	if err := json.Unmarshal(b, &s); err != nil {
		return Status{}, err
	}
	if s.State != "closed" {
		s.State = "open"
	}
	return s, nil
}
