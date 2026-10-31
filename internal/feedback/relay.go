package feedback

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"unicode/utf16"
)

// Relay is the feedback relay's address (feedback-relay/README.md, 06 §2.14 反馈): a
// Cloudflare Worker that files a report as an issue, so a player without a GitHub account
// can send one. Empty until it is deployed: 反馈 then offers only the GitHub link.
var Relay = ""

// RelayURL is where 发送 posts, or "" when there is no relay. RIMARCHO_FEEDBACK_RELAY
// overrides Relay, to try a deployment before its address is written here.
func RelayURL() string {
	if v := strings.TrimSpace(os.Getenv("RIMARCHO_FEEDBACK_RELAY")); v != "" {
		return v
	}
	return Relay
}

// MaxBody is the relay's limit for a body, in UTF-16 units as JavaScript counts (GitHub
// takes 65,536 characters).
const MaxBody = 60000

// Report is what goes to the relay. The contact is kept by the relay for the maintainer and
// never goes into the issue.
type Report struct {
	Title   string `json:"title"`
	Body    string `json:"body"`
	Contact string `json:"contact,omitempty"`
}

// Sent is the issue the relay opened.
type Sent struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
}

// RelayError is the relay's refusal. Code is the relay's word for it: slow_down (one a
// minute), ip_daily, daily (today's caps), too_long, empty, github (GitHub failed),
// not_configured.
type RelayError struct {
	Status int
	Code   string
}

func (e *RelayError) Error() string { return fmt.Sprintf("feedback relay: %d %s", e.Status, e.Code) }

// Send posts the report to the relay at endpoint (https; http only to this computer, for
// tests) and returns the new issue. agent is "RimArcho/<version>".
func Send(ctx context.Context, endpoint, agent string, r Report) (Sent, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && loopback(u.Hostname()))) {
		return Sent{}, fmt.Errorf("feedback relay address %q is not https", endpoint)
	}
	data, err := json.Marshal(r)
	if err != nil {
		return Sent{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return Sent{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", agent)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return Sent{}, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	var out struct {
		Sent
		Error string `json:"error"`
	}
	_ = json.Unmarshal(b, &out)
	if res.StatusCode != http.StatusOK || out.Number == 0 {
		code := out.Error
		if code == "" {
			code = http.StatusText(res.StatusCode)
		}
		return Sent{}, &RelayError{Status: res.StatusCode, Code: code}
	}
	return out.Sent, nil
}

func loopback(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}

// SendBody is Body cut to max UTF-16 units: the longest section loses lines from its end,
// with More (a format with %d, in the player's language) saying how many, until it fits.
func (d Draft) SendBody(max int) string {
	secs := append([]Section(nil), d.Sections...)
	cut := make([]int, len(secs))
	for {
		dd := d
		dd.Sections = make([]Section, len(secs))
		for i, s := range secs {
			dd.Sections[i] = s
			if cut[i] > 0 {
				dd.Sections[i].Lines = append(append([]string(nil), s.Lines...), fmt.Sprintf(d.More, cut[i]))
			}
		}
		b := dd.Body()
		over := units(b) - max
		if over <= 0 {
			return b
		}
		long := -1
		for i, s := range secs {
			if len(s.Lines) > 0 && (long < 0 || units(strings.Join(s.Lines, "\n")) > units(strings.Join(secs[long].Lines, "\n"))) {
				long = i
			}
		}
		if long < 0 {
			return truncate(b, max)
		}
		// drop about as many lines as the excess needs, at least one
		ls := secs[long].Lines
		n := 0
		for drop := 0; drop < over && n < len(ls); n++ {
			drop += units(ls[len(ls)-1-n]) + 1
		}
		secs[long].Lines = ls[:len(ls)-n]
		cut[long] += n
	}
}

func units(s string) int { return len(utf16.Encode([]rune(s))) }

// truncate cuts s to at most max UTF-16 units on a rune boundary.
func truncate(s string, max int) string {
	n := 0
	for i, r := range s {
		w := 1
		if r > 0xFFFF {
			w = 2
		}
		if n+w > max {
			return s[:i]
		}
		n += w
	}
	return s
}
