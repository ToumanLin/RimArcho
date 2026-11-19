package gui

import (
	"context"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/feedback"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// feedbackState keeps the attachments read last, so typing only composes (16 §2.14 Feedback).
type feedbackState struct {
	mu sync.Mutex
	at *view.FeedbackAttach
}

// FeedbackAttach reads what can go with a report under the ticked options (Settings > Feedback,
// 16 §2.14). It shares the game log's reader: the Game log section is that page's error groups.
func (s *API) FeedbackAttach(o view.FeedbackOptions) (*view.FeedbackAttach, error) {
	s.logRead.mu.Lock()
	defer s.logRead.mu.Unlock()
	defer s.free.soon()
	var r *app.App
	var st *app.LoadoutState
	if lr, err := s.logSession(); err == nil {
		r = lr
		st, _ = r.Loadout() // no game yet: only the basic lines
	} else {
		dr, err := s.detachedReader()
		if err != nil {
			return nil, err
		}
		defer dr.Close()
		r = dr
	}
	at := view.FeedbackAttachments(r, st, o)
	s.feedback.mu.Lock()
	s.feedback.at = at
	s.feedback.mu.Unlock()
	return at, nil
}

// FeedbackCompose is the issue's body and link for the title and text with the attachments
// read last; the page calls it as the player types.
func (s *API) FeedbackCompose(title, text string) feedback.Result {
	s.feedback.mu.Lock()
	at := s.feedback.at
	s.feedback.mu.Unlock()
	return view.ComposeFeedback(title, text, at)
}

// FeedbackSend files the report through the feedback relay with the attachments read last
// (send, offered when FeedbackAttach says send). The contact goes to the relay only.
func (s *API) FeedbackSend(title, text, contact string) (feedback.Sent, error) {
	s.feedback.mu.Lock()
	at := s.feedback.at
	s.feedback.mu.Unlock()
	sent, err := view.SendFeedback(context.Background(), title, text, contact, at)
	if err == nil {
		view.RecordFeedbackSent(s.a, sent, sentTitle(title, text))
	}
	return sent, err
}

// sentTitle is the title the record keeps: the one typed, or the description's first line.
func sentTitle(title, text string) string {
	if t := strings.TrimSpace(title); t != "" {
		return t
	}
	first, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return strings.TrimSpace(first)
}

// FeedbackReports is Feedback › My reports as recorded, without asking the relay.
func (s *API) FeedbackReports() view.FeedbackReports { return view.ReadFeedbackReports(s.a) }

// FeedbackCheck asks the relay for replies (the Feedback page opened) and updates the
// sidebar's dot.
func (s *API) FeedbackCheck() view.FeedbackReports {
	r := view.CheckFeedbackReplies(context.Background(), s.a)
	s.send("feedback-replies", map[string]any{"unseen": r.Unseen})
	return r
}

// FeedbackSeen marks the replies shown on the page as seen; the dot goes.
func (s *API) FeedbackSeen() {
	view.SeenFeedbackReplies(s.a)
	s.send("feedback-replies", map[string]any{"unseen": 0})
}

// FeedbackFollowUp is the title a follow-up about report n starts with.
func (s *API) FeedbackFollowUp(n int, title string) string { return view.FollowUpTitle(n, title) }

// checkFeedbackReplies is the start-up check (16 §2.14): quiet, and only the dot shows it.
func (s *API) checkFeedbackReplies() {
	if !view.RepliesOn(s.a) {
		return
	}
	go func() {
		r := view.CheckFeedbackReplies(context.Background(), s.a)
		if r.Unseen > 0 {
			s.send("feedback-replies", map[string]any{"unseen": r.Unseen})
		}
	}()
}

// Community is RimArcho's public pages for the settings page.
func (s *API) Community() []view.CommunityLink { return view.CommunityLinks() }
