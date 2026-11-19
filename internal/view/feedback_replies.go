package view

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/feedback"
)

// Feedback › My reports (16 §2.14): the reports this computer sent through the relay, their
// state and the developer's replies, read back with each report's token. Checked at start and
// when the Feedback page opens, never otherwise; the setting feedback.replies turns it off.
// Any failure is quiet: the last answer stays and no dot appears for it.

var reportsMu sync.Mutex

// feedbackNow is the clock, for tests.
var feedbackNow = time.Now

// FeedbackReport is one sent report as the page shows it.
type FeedbackReport struct {
	Number   int              `json:"number"`
	Title    string           `json:"title"`
	URL      string           `json:"url,omitempty"`
	SentAt   time.Time        `json:"sentAt"`
	State    string           `json:"state"` // open, closed, or "" before the first answer
	ClosedAt *time.Time       `json:"closedAt,omitempty"`
	Replies  []feedback.Reply `json:"replies"`
	Unseen   bool             `json:"unseen"`
	Gone     bool             `json:"gone,omitempty"`
}

// FeedbackReports is the My reports section.
type FeedbackReports struct {
	Checking bool             `json:"checking"` // the setting is on and there is a relay
	Off      bool             `json:"off"`      // the player turned checking off
	Failed   bool             `json:"failed"`   // the last check could not reach the relay
	Unseen   int              `json:"unseen"`
	Reports  []FeedbackReport `json:"reports"`
}

var lastCheckFailed bool

// RepliesOn reports whether RimArcho asks the relay for replies.
func RepliesOn(a *app.App) bool {
	return cmds.Setting(a, "feedback.replies") != "off" && feedback.RelayURL() != ""
}

// RecordFeedbackSent adds a report the relay accepted to the record. A relay without tokens
// (before replies) leaves nothing to check, so it is not recorded.
func RecordFeedbackSent(a *app.App, s feedback.Sent, title string) {
	if s.Number <= 0 || s.Token == "" {
		return
	}
	reportsMu.Lock()
	defer reportsMu.Unlock()
	rs := feedback.LoadReports(a.Home)
	rs = append([]feedback.Record{{Number: s.Number, Token: s.Token, Title: strings.TrimSpace(title), URL: s.URL, SentAt: feedbackNow().UTC(), State: "open"}}, rs...)
	_ = feedback.SaveReports(a.Home, rs)
}

// CheckFeedbackReplies asks the relay about every report still due and keeps the answers.
// It returns the reports; with checking off or no relay it only reads the record.
func CheckFeedbackReplies(ctx context.Context, a *app.App) FeedbackReports {
	if !RepliesOn(a) {
		return ReadFeedbackReports(a)
	}
	reportsMu.Lock()
	rs := feedback.LoadReports(a.Home)
	reportsMu.Unlock()
	now := feedbackNow()
	relay := feedback.RelayURL()
	got := map[int]feedback.Status{}
	gone := map[int]bool{}
	failed := false
	for _, r := range rs {
		if !r.Due(now) {
			continue
		}
		c, cancel := context.WithTimeout(ctx, 15*time.Second)
		st, err := feedback.FetchStatus(c, relay, "RimArcho/"+buildinfo.Version, r.Number, r.Token)
		cancel()
		switch {
		case errors.Is(err, feedback.ErrGone):
			gone[r.Number] = true
		case err != nil:
			failed = true
		default:
			got[r.Number] = st
		}
		if failed {
			break // the relay is down or refusing: ask again next time
		}
	}
	reportsMu.Lock()
	defer reportsMu.Unlock()
	lastCheckFailed = failed
	if len(got) > 0 || len(gone) > 0 {
		// read again: a report sent meanwhile stays
		rs = feedback.LoadReports(a.Home)
		for i := range rs {
			if st, ok := got[rs[i].Number]; ok {
				rs[i].State, rs[i].ClosedAt, rs[i].Replies, rs[i].CheckedAt = st.State, st.ClosedAt, st.Replies, now.UTC()
			}
			if gone[rs[i].Number] {
				rs[i].Gone = true
			}
		}
		_ = feedback.SaveReports(a.Home, rs)
	}
	return reportsView(a, rs)
}

// ReadFeedbackReports is the record as it is, without asking the relay.
func ReadFeedbackReports(a *app.App) FeedbackReports {
	reportsMu.Lock()
	defer reportsMu.Unlock()
	return reportsView(a, feedback.LoadReports(a.Home))
}

// SeenFeedbackReplies marks every reply shown so far as seen (the page showed them).
func SeenFeedbackReplies(a *app.App) {
	reportsMu.Lock()
	defer reportsMu.Unlock()
	rs := feedback.LoadReports(a.Home)
	changed := false
	for i := range rs {
		for _, p := range rs[i].Replies {
			if p.CreatedAt.After(rs[i].LastSeenReply) {
				rs[i].LastSeenReply, changed = p.CreatedAt, true
			}
		}
	}
	if changed {
		_ = feedback.SaveReports(a.Home, rs)
	}
}

func reportsView(a *app.App, rs []feedback.Record) FeedbackReports {
	on := RepliesOn(a)
	out := FeedbackReports{Checking: on, Off: cmds.Setting(a, "feedback.replies") == "off", Failed: on && lastCheckFailed, Reports: []FeedbackReport{}}
	for _, r := range rs {
		v := FeedbackReport{Number: r.Number, Title: r.Title, URL: r.URL, SentAt: r.SentAt, State: r.State, ClosedAt: r.ClosedAt, Replies: r.Replies, Gone: r.Gone}
		if v.Replies == nil {
			v.Replies = []feedback.Reply{}
		}
		// the dot only for replies this check could read; with checking off, none
		v.Unseen = on && r.Unseen()
		if v.Unseen {
			out.Unseen++
		}
		out.Reports = append(out.Reports, v)
	}
	return out
}

// FollowUpTitle is the title a follow-up about report n starts with.
func FollowUpTitle(n int, title string) string {
	if title == "" {
		return Tf("follow-up to #%d", n)
	}
	return Tf("follow-up to #%d: %s", n, title)
}
