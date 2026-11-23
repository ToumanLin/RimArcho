package view

import (
	"context"
	"errors"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/feedback"
)

// Feedback › feature request › similar requests (16 §2.14): the open feature requests on
// GitHub, read through the relay, the ones most like what the player is writing shown under
// the form with a +1. Read only while a feature request is being written; a failure is quiet.

// FeedbackRequest is one open feature request; Given: this computer gave it a +1.
type FeedbackRequest struct {
	feedback.Request
	Given bool `json:"given,omitempty"`
}

// FeedbackSimilarMax is how many similar requests the page shows.
const FeedbackSimilarMax = 4

// FeedbackRequests reads the open feature requests through the relay, with the ones this
// computer gave a +1 marked. No relay, or the relay failed: none, and the error.
func FeedbackRequests(ctx context.Context, a *app.App) ([]FeedbackRequest, error) {
	relay := feedback.RelayURL()
	if relay == "" {
		return nil, errors.New(T("RimArcho has no feedback relay; open GitHub instead"))
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	rs, err := feedback.FetchRequests(ctx, relay, "RimArcho/"+buildinfo.Version)
	if err != nil {
		return nil, err
	}
	given := feedback.LoadPlus(a.Home)
	out := make([]FeedbackRequest, len(rs))
	for i, r := range rs {
		out[i] = FeedbackRequest{Request: r, Given: given[r.Number]}
	}
	return out, nil
}

// SimilarFeedbackRequests are the requests most like text, best first (feedback.Similar).
func SimilarFeedbackRequests(text string, rs []FeedbackRequest) []FeedbackRequest {
	plain := make([]feedback.Request, len(rs))
	by := map[int]FeedbackRequest{}
	for i, r := range rs {
		plain[i] = r.Request
		by[r.Number] = r
	}
	var out []FeedbackRequest
	for _, r := range feedback.Similar(text, plain, FeedbackSimilarMax) {
		out = append(out, by[r.Number])
	}
	return out
}

// PlusFeedbackRequest gives request n this computer's +1 through the relay, once, and
// returns its count.
func PlusFeedbackRequest(ctx context.Context, a *app.App, n int) (int, error) {
	if feedback.LoadPlus(a.Home)[n] {
		return 0, errors.New(T("you gave this request a +1 already"))
	}
	relay := feedback.RelayURL()
	if relay == "" {
		return 0, errors.New(T("RimArcho has no feedback relay; open GitHub instead"))
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	c, err := feedback.Plus(ctx, relay, "RimArcho/"+buildinfo.Version, n)
	if err != nil {
		var re *feedback.RelayError
		if errors.As(err, &re) && re.Code == "slow_down" {
			return 0, errors.New(T("enough +1 for today; try again tomorrow"))
		}
		return 0, errors.New(Tf("could not reach the feedback relay: %v", err))
	}
	_ = feedback.SavePlus(a.Home, n)
	return c, nil
}

// FeedbackChoice is one pick for a question: a diagnosis or a Mod.
type FeedbackChoice struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// FeedbackChoices are the picks for "which diagnosis" (Can it run's items) and "which Mod"
// (the current list); n nil: Can it run is not built yet, no diagnoses to pick.
type FeedbackChoices struct {
	Findings []FeedbackChoice `json:"findings"`
	Mods     []FeedbackChoice `json:"mods"`
}

// FeedbackChoicesOf reads the picks from the list and Can it run as last built.
func FeedbackChoicesOf(st *app.LoadoutState, n *Needs) FeedbackChoices {
	out := FeedbackChoices{Findings: []FeedbackChoice{}, Mods: []FeedbackChoice{}}
	if n != nil {
		for _, it := range n.Items {
			if !it.Counted || it.ID == "" {
				continue
			}
			t := it.Title
			if t == "" {
				t = it.Text
			}
			out.Findings = append(out.Findings, FeedbackChoice{ID: it.ID, Text: truncateRunes(t, 90)})
		}
	}
	if st != nil && st.Ix != nil {
		for _, e := range st.List {
			if m := st.Ix.ByID[e.Instance]; e.Instance != "" && m != nil && m.PackageID != "" {
				out.Mods = append(out.Mods, FeedbackChoice{ID: m.PackageID, Text: m.DisplayName()})
			}
		}
	}
	return out
}
