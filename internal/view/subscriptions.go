package view

import (
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/modchanges"
)

type SubscriptionSource struct {
	Key      string               `json:"key"`
	Name     string               `json:"name"`
	Count    int                  `json:"count"`
	Checked  string               `json:"checked"`
	Result   string               `json:"result"`
	Account  string               `json:"account,omitempty"`
	Failures []SourceFailureGroup `json:"failures,omitempty"`
}

type SourceFailureGroup struct {
	Label string   `json:"label"`
	Items []string `json:"items"`
}

type PendingUpdate struct {
	ID        string   `json:"id"`
	Ref       string   `json:"ref"`
	Name      string   `json:"name"`
	Source    string   `json:"source"`
	Kind      string   `json:"kind"`
	Installed string   `json:"installed"`
	Available string   `json:"available"`
	UsedIn    []string `json:"usedIn"`
}

type SubscriptionsPage struct {
	Sources []SubscriptionSource `json:"sources"`
	Pending []PendingUpdate      `json:"pending"`
	History *modchanges.Page     `json:"history"`
}

func BuildSubscriptions(a *app.App, st *app.LoadoutState, o modchanges.Opts) (*SubscriptionsPage, error) {
	history, err := BuildChanges(a, st, o)
	if err != nil {
		return nil, err
	}
	p := &SubscriptionsPage{History: history, Pending: []PendingUpdate{}}
	keys := []string{app.LibWorkshop, app.LibSteamCMD, app.LibGit, app.LibPage, app.LibLocal}
	names := []string{T("Steam Workshop (client)"), "SteamCMD", "Git", T("download page / LoversLab"), T("local (no source)")}
	checks := map[string]app.SourceCheck{}
	for i, key := range keys {
		c := a.SubscriptionSourceCheck(key)
		checks[key] = c
		r := SubscriptionSource{Key: key, Name: names[i], Result: T("not checked yet")}
		if !c.At.IsZero() {
			r.Checked, r.Result = a.FormatTime(c.At), Text(c.Result)
			groups := map[string]int{}
			for _, failure := range c.Failures {
				reason := failure.Reason
				if failure.Code != 0 {
					reason = app.WorkshopResultText(failure.Code)
				}
				groupKey := reason
				at, ok := groups[groupKey]
				if !ok {
					at = len(r.Failures)
					groups[groupKey] = at
					r.Failures = append(r.Failures, SourceFailureGroup{})
				}
				g := &r.Failures[at]
				g.Items = append(g.Items, failure.Name+" ("+failure.ID+"): "+Text(reason))
				g.Label = Tf("%d: %s", len(g.Items), Text(reason))
				if failure.Code == 9 {
					g.Label = Tf("%d no longer on the Workshop", len(g.Items))
				}
			}
			if len(c.Failures) > 0 {
				r.Result = Tf("%d failed", len(c.Failures))
			}
		}
		if key == app.LibSteamCMD {
			r.Account = SteamAccountLine(a.SteamAccountStatus())
		}
		if key == app.LibPage {
			r.Account, _ = LoversLabLine(a)
		}

		p.Sources = append(p.Sources, r)
	}
	lib, err := a.Library()
	if err != nil {
		return nil, err
	}
	for _, l := range lib.Lineages {
		for _, inst := range l.Instances {
			for i := range p.Sources {
				s := &p.Sources[i]
				if s.Key != inst.Source {
					continue
				}
				s.Count++
				if sub := inst.Subscription; sub != nil && sub.CheckedAt.After(checks[s.Key].At) {
					check := checks[s.Key]
					check.At = sub.CheckedAt
					checks[s.Key] = check
					s.Checked, s.Result = a.FormatTime(sub.CheckedAt), T("check complete")
					if sub.CheckError != "" {
						s.Result = Text(sub.CheckError)
					}
				}
				if inst.Mod == nil || inst.Missing() || s.Key == app.LibLocal {
					continue
				}
				r := PendingUpdate{ID: inst.Mod.ID, Ref: inst.Ref, Name: l.Name, Source: s.Name, Kind: s.Key, Installed: "—", Available: "—", UsedIn: []string{}}
				for _, u := range inst.UsedIn {
					r.UsedIn = append(r.UsedIn, u.Name)
				}
				if sub := inst.Subscription; sub != nil && sub.HasUpdate() {
					r.Installed, r.Available = sub.Installed, sub.Latest
				} else if s.Key == app.LibWorkshop || s.Key == app.LibSteamCMD {
					latest := checks[s.Key].Latest[inst.Mod.ID]
					installed := inst.Mod.ACFTimeUpdated
					if s.Key == app.LibSteamCMD && inst.Subscription != nil && !inst.Subscription.InstalledAt.IsZero() {
						installed = max(installed, inst.Subscription.InstalledAt.Unix())
					}
					if installed <= 0 || latest <= installed {
						continue
					}
					r.Installed, r.Available = a.FormatTime(time.Unix(installed, 0)), a.FormatTime(time.Unix(latest, 0))
				} else {
					continue
				}
				p.Pending = append(p.Pending, r)
			}
		}
	}
	return p, nil
}
