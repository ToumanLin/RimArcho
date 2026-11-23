//go:build windows || cgo

package gui

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/feedback"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// feedbackState keeps the attachments read last, so typing only composes (16 §2.14 Feedback),
// and the open feature requests read this visit, for the similar ones as the player types.
type feedbackState struct {
	mu       sync.Mutex
	at       *view.FeedbackAttach
	requests []view.FeedbackRequest
	readAt   time.Time
}

// feedbackNeeds is Can it run as last built, without building it: the problems attachment and
// the diagnoses to pick from need it, and nothing on the Feedback page should start a check.
func (s *API) feedbackNeeds() *view.Needs {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, _ := s.cachedNeedsLocked(); c != nil {
		return c.n
	}
	return nil
}

// readFeedbackAttach reads what goes with a report under the options. It shares the game
// log's reader: the Game log section is that page's error groups.
func (s *API) readFeedbackAttach(o view.FeedbackOptions) (*view.FeedbackAttach, error) {
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
	return view.FeedbackAttachments(r, st, o, s.feedbackNeeds()), nil
}

// FeedbackAttach reads what can go with the problem shown (its template's attachments and the
// ticked ones) and keeps it for FeedbackCompose.
func (s *API) FeedbackAttach(p view.FeedbackProblem) (*view.FeedbackAttach, error) {
	at, err := s.readFeedbackAttach(p.Options())
	if err != nil {
		return nil, err
	}
	s.feedback.mu.Lock()
	s.feedback.at = at
	s.feedback.mu.Unlock()
	return at, nil
}

// FeedbackTemplates are the report kinds with their questions, in the player's language.
func (s *API) FeedbackTemplates() []view.FeedbackTemplate { return view.FeedbackTemplates() }

// FeedbackChoices are the diagnoses and Mods "which diagnosis" and "which Mod" offer.
func (s *API) FeedbackChoices() view.FeedbackChoices {
	st, _ := s.a.Loadout()
	return view.FeedbackChoicesOf(st, s.feedbackNeeds())
}

// FeedbackCompose is the issue's body and link for the problem shown with the attachments
// read last; the page calls it as the player types.
func (s *API) FeedbackCompose(p view.FeedbackProblem) feedback.Result {
	s.feedback.mu.Lock()
	at := s.feedback.at
	s.feedback.mu.Unlock()
	return view.ComposeProblem(p, at)
}

// FeedbackSend files each problem as its own issue through the feedback relay (send, offered
// when FeedbackAttach says send), each with its own attachments read again now. The contact
// goes to the relay only. Issues filed before a failure come back with it.
func (s *API) FeedbackSend(ps []view.FeedbackProblem, contact string) ([]feedback.Sent, error) {
	ats := make([]*view.FeedbackAttach, len(ps))
	for i, p := range ps {
		at, err := s.readFeedbackAttach(p.Options())
		if err != nil {
			return nil, err
		}
		if at.Pending {
			return nil, errors.New(view.T("the game log is still being analysed…"))
		}
		ats[i] = at
	}
	sent, err := view.SendFeedbackProblems(context.Background(), ps, ats, contact)
	for i, x := range sent {
		if i < len(ps) {
			view.RecordFeedbackSent(s.a, x, ps[i].TitleOrFirst())
		}
	}
	if sent == nil {
		sent = []feedback.Sent{}
	}
	return sent, err
}

// feedbackRequestsAgain is how long the open feature requests read stand within a visit.
const feedbackRequestsAgain = 10 * time.Minute

// FeedbackSimilar are the open feature requests most like text, read through the relay the
// first time a feature request is written (and again after feedbackRequestsAgain). A relay
// that cannot be reached gives none, quietly.
func (s *API) FeedbackSimilar(text string) []view.FeedbackRequest {
	s.feedback.mu.Lock()
	rs, at := s.feedback.requests, s.feedback.readAt
	s.feedback.mu.Unlock()
	if at.IsZero() || time.Since(at) > feedbackRequestsAgain {
		got, _ := join(&s.flight, "FeedbackRequests", func() ([]view.FeedbackRequest, error) {
			return view.FeedbackRequests(context.Background(), s.a)
		})
		rs = got
		s.feedback.mu.Lock()
		s.feedback.requests, s.feedback.readAt = got, time.Now()
		s.feedback.mu.Unlock()
	}
	out := view.SimilarFeedbackRequests(text, rs)
	if out == nil {
		out = []view.FeedbackRequest{}
	}
	return out
}

// FeedbackPlus gives open feature request n this computer's +1 and returns its new count.
func (s *API) FeedbackPlus(n int) (int, error) {
	c, err := view.PlusFeedbackRequest(context.Background(), s.a, n)
	if err != nil {
		return 0, err
	}
	s.feedback.mu.Lock()
	for i := range s.feedback.requests {
		if s.feedback.requests[i].Number == n {
			s.feedback.requests[i].Plus, s.feedback.requests[i].Given = c, true
		}
	}
	s.feedback.mu.Unlock()
	return c, nil
}

// FeedbackReports is Feedback › My reports as recorded, without asking the relay.
func (s *API) FeedbackReports() view.FeedbackReports { return view.ReadFeedbackReports(s.a) }

// feedbackAgain is how long a relay answer stands: entering Feedback again within it
// shows the recorded replies without asking the relay (16 §8.9).
const feedbackAgain = time.Minute

// FeedbackCheck asks the relay for replies (the Feedback page opened) and updates the
// sidebar's dot. Entries while a check is on its way join it; within feedbackAgain of the
// last one the recorded replies stand.
func (s *API) FeedbackCheck() view.FeedbackReports {
	r, _ := join(&s.flight, "FeedbackCheck", func() (view.FeedbackReports, error) {
		s.mu.Lock()
		recent := !s.feedbackAt.IsZero() && time.Since(s.feedbackAt) < feedbackAgain
		s.mu.Unlock()
		if recent {
			return view.ReadFeedbackReports(s.a), nil
		}
		r := view.CheckFeedbackReplies(context.Background(), s.a)
		s.mu.Lock()
		s.feedbackAt = time.Now()
		s.mu.Unlock()
		return r, nil
	})
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
