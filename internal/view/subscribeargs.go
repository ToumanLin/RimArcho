package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/modchanges"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
)

// SubscribeRequest is what the add a subscription card decided (06 §2.9): the source and its one
// choice. The window sends it as JSON; the TUI builds it from its card.
type SubscribeRequest struct {
	Kind     string `json:"kind"`
	URL      string `json:"url"`
	RefType  string `json:"refType"`
	Ref      string `json:"ref"`
	Git      bool   `json:"git"`
	From     string `json:"from"`
	SteamCMD bool   `json:"steamcmd"`
	Batch    bool   `json:"batch"`
}

// SubscribeArgv is the archo subscribe command of a request (its preview adds --dry-run).
func SubscribeArgv(r SubscribeRequest) ([]string, error) {
	args := []string{"subscribe", r.Kind}
	switch r.Kind {
	case "git":
		args = append(args, r.URL)
		if r.RefType != "" && (r.Ref != "" || r.RefType != "branch") {
			args = append(args, "--"+r.RefType, r.Ref)
		}
	case "page":
		args = append(args, r.URL)
		if r.Git {
			args = append(args, "--git")
		}
		if r.From != "" {
			args = append(args, "--from", r.From)
		}
	case "workshop":
		args = append(args, strings.Fields(r.URL)...)
		if r.SteamCMD {
			args = append(args, "--steamcmd")
		}
	default:
		return nil, fmt.Errorf("unknown subscription kind %q", r.Kind)
	}
	return args, nil
}

// WorkshopUpdatePreview is the confirmation of update for a Workshop item the Steam client
// keeps (06 §2.9): installed → latest, and that Steam downloads it. Refused while a game
// runs, like the update itself.
func WorkshopUpdatePreview(a *app.App, ref string) (*Preview, error) {
	if err := a.UpdateWorkshopClient(ref, true); err != nil {
		return nil, err
	}
	ix, err := a.Index()
	if err != nil {
		return nil, err
	}
	m, err := ix.ResolveOne(ref)
	if err != nil {
		return nil, err
	}
	latest := a.SubscriptionSourceCheck("workshop").Latest[m.ID]
	lines := []string{m.DisplayName()}
	if latest > 0 {
		lines = append(lines, a.FormatTime(time.Unix(m.ACFTimeUpdated, 0))+" → "+a.FormatTime(time.Unix(latest, 0)))
	}
	lines = append(lines, T("Steam will download this Workshop update"))
	return &Preview{What: OpSubUpdate, Title: T("update subscription"), Apply: true, Lines: lines}, nil
}

// SourceCheckResult checks one source of updates (check, 06 §2.9) and words the result as
// the page does: "everything is up to date" or "N updates available", never "check complete"; a failure keeps its reason.
func SourceCheckResult(a *app.App, kind string, pageReader ...func(string) (*subscribe.PageInfo, error)) (string, error) {
	if err := a.CheckSubscriptionSource(kind, pageReader...); err != nil {
		return "", err
	}
	st, err := a.Loadout()
	if err != nil {
		return "", err
	}
	page, err := BuildSubscriptions(a, st, modchanges.Opts{})
	if err != nil {
		return "", err
	}
	for _, source := range page.Sources {
		if source.Key == kind {
			return source.Result, nil
		}
	}
	return "", nil
}

// TaskQueuedText is the status line after queuing a task: "update subscription, Git Mod: queued in files".
func TaskQueuedText(t app.Task) string {
	lane, _ := TaskLaneText(t.Lane)
	return Tf("%s, %s: in the %s queue", TaskTitle(t.Kind), t.Obj, lane)
}
