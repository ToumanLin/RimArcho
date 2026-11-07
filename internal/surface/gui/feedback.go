package gui

import (
	"context"
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
	return view.SendFeedback(context.Background(), title, text, contact, at)
}

// Community is RimArcho's public pages for the settings page.
func (s *API) Community() []view.CommunityLink { return view.CommunityLinks() }
