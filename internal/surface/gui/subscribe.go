package gui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
	"github.com/ToumanLin/RimArcho/internal/view"
)

type SubscriptionRequest struct {
	Kind     string `json:"kind"`
	URL      string `json:"url"`
	RefType  string `json:"refType"`
	Ref      string `json:"ref"`
	Git      bool   `json:"git"`
	From     string `json:"from"`
	SteamCMD bool   `json:"steamcmd"`
	Batch    bool   `json:"batch"`
}

func subscriptionArgs(r SubscriptionRequest) ([]string, error) {
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

// The CLI supplies the same dry-run text used by existing file actions.
func (s *API) SubscribePreview(r SubscriptionRequest) (*view.Preview, error) {
	args, err := subscriptionArgs(r)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	a, err := s.a.ReadSession()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	defer a.Close()
	return subscriptionPreview(a, args, view.T("subscribe…")), nil
}

func subscriptionPreview(a *app.App, args []string, title string) *view.Preview {
	out, code := cmds.Execute(a, append(args, "--dry-run"))
	st, _ := a.Loadout()
	p := &view.Preview{What: "subscribe", Title: title, Apply: code == 0}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		p.Lines = append(p.Lines, view.Humanize(st, line))
	}
	return p
}

func (s *API) Subscribe(r SubscriptionRequest) (string, error) {
	if r.Kind == "page" && !r.Git && r.From == "" {
		return "", errors.New(view.T("choose a repository or downloaded files first"))
	}
	args, err := subscriptionArgs(r)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Kind == "workshop" && r.SteamCMD && r.Batch {
		args = append(args, "--credit-batch")
	}
	return settingQueued(s.queueSettingTaskLocked(view.TaskSubscribe, args, "")), nil
}

func (s *API) SubscriptionPage(url string) (*subscribe.PageInfo, error) {
	s.mu.Lock()
	client := s.a.HTTP
	s.mu.Unlock()
	return subscribe.ReadPage(context.Background(), client, url)
}

func (s *API) PickSubscriptionZip() (string, error) {
	if s.x.native == nil {
		return "", nil
	}
	return s.x.native.openFile(view.T("choose downloaded zip"), [][2]string{{"Zip (*.zip)", "*.zip"}})
}

func (s *API) SubscriptionFromPreview(ref, from string) (*view.Preview, error) {
	s.mu.Lock()
	a, err := s.a.ReadSession()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	defer a.Close()
	return subscriptionPreview(a, []string{"subscribe", "update", ref, "--from", from}, view.T("update subscription")), nil
}

func (s *API) SubscriptionFrom(ref, from string) (string, error) {
	if strings.TrimSpace(from) == "" {
		return "", errors.New(view.T("choose downloaded files first"))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return settingQueued(s.queueSettingTaskLocked(view.TaskSubscriptionFrom, []string{"subscribe", "update", ref, "--from", from}, "")), nil
}

func (s *API) runSubscriptionTask(a *app.App, id, kind string, args []string) (string, error) {
	if len(args) > 0 && args[len(args)-1] == "--credit-batch" {
		// This is a persisted GUI task argument, not a public CLI flag.
		op, err := a.SubscribeWorkshopBatch(args[2:len(args)-2], true, true, false)
		if op != nil {
			items := make([]app.TaskItem, 0, len(op.Items))
			for _, it := range op.Items {
				state := app.ItemOK
				if it.Result != "downloaded with SteamCMD" {
					state = app.ItemFail
				}
				items = append(items, app.TaskItem{Ref: it.ID, Name: it.Name, State: state, Why: it.Result})
			}
			s.tasks.Update(id, func(t *app.Task) { t.Items = items; t.Total = len(items) })
		}
		if err != nil {
			return "", err
		}
		return view.T("subscription finished"), nil
	}
	before, err := a.Subscriptions()
	if err != nil {
		return "", err
	}
	out, code := cmds.Execute(a, args)
	if code != 0 {
		return "", errors.New(strings.TrimSpace(out))
	}
	known := map[string]bool{}
	for _, sub := range before {
		known[sub.Mod.ID] = true
	}
	after, err := a.Subscriptions()
	if err != nil {
		return "", err
	}
	target := ""
	for _, sub := range after {
		if !known[sub.Mod.ID] {
			target = sub.Ref
			break
		}
	}
	if kind == view.TaskSubscriptionFrom {
		target = args[2]
	}
	s.tasks.Update(id, func(t *app.Task) { t.Target = target })
	if strings.Contains(out, "Steam downloads it; it appears after the next scan.") {
		return view.T("Steam downloads it; it appears after the next scan"), nil
	}
	return view.T("subscription finished"), nil
}
