package view

import (
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/modchanges"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
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

// PendingUpdate is one row of 等你更新 (06 §2.9) and its detail.
type PendingUpdate struct {
	ID        string   `json:"id"`
	Ref       string   `json:"ref"`
	Name      string   `json:"name"`
	Source    string   `json:"source"`
	Kind      string   `json:"kind"`
	Installed string   `json:"installed"`
	Available string   `json:"available"`
	UsedIn    []string `json:"usedIn"`
	// InPlan: the current plan uses it (the scope bar's 列表 shows only these).
	InPlan bool `json:"inPlan,omitempty"`
	// Tracking: a Git subscription's branch, tag or commit.
	Tracking string `json:"tracking,omitempty"`
	// InstalledAt: when the installed one was installed; Checked: the last check.
	InstalledAt string `json:"installedAt,omitempty"`
	Checked     string `json:"checked,omitempty"`
	// LoversLab: a download page subscription on LoversLab (its files can be read with the session).
	LoversLab bool `json:"loversLab,omitempty"`
	// Opens: what 打开来源页 opens: "compare" (two commits on GitHub / GitLab), "page" (the
	// repository or download page), "notes" (the Workshop change notes), "" nothing.
	Opens string `json:"opens,omitempty"`
}

// SourceSummary is the one line under the list that replaces the sources section.
type SourceSummary struct {
	Line     string `json:"line"`               // "checked 4 sources, the last at …"
	Failures int    `json:"failures,omitempty"` // failed checks of the last runs
	Failed   string `json:"failed,omitempty"`   // "2 no longer on the Workshop"
}

// RecentSubscription is one of the newest subscriptions RimArcho installed (添加订阅).
type RecentSubscription struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Added  string `json:"added"`
}

type SubscriptionsPage struct {
	Sources []SubscriptionSource `json:"sources"`
	Summary SourceSummary        `json:"summary"`
	Pending []PendingUpdate      `json:"pending"`
	Recent  []RecentSubscription `json:"recent"`
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
	type added struct {
		r  RecentSubscription
		at time.Time
	}
	var recent []added
	for _, l := range lib.Lineages {
		for _, inst := range l.Instances {
			for i := range p.Sources {
				s := &p.Sources[i]
				if s.Key != inst.Source {
					continue
				}
				s.Count++
				if sub := inst.Subscription; sub != nil && !sub.InstalledAt.IsZero() && inst.Mod != nil && !inst.Missing() {
					recent = append(recent, added{RecentSubscription{Name: l.Name, Kind: s.Key, Source: s.Name, Added: a.FormatDate(sub.InstalledAt)}, sub.InstalledAt})
				}
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
				r := PendingUpdate{ID: inst.Mod.ID, Ref: inst.Ref, Name: l.Name, Source: s.Name, Kind: s.Key, Installed: "—", Available: "—", UsedIn: []string{}, InPlan: inst.InPlan}
				for _, u := range inst.UsedIn {
					r.UsedIn = append(r.UsedIn, u.Name)
				}
				if sub := inst.Subscription; sub != nil {
					if !sub.InstalledAt.IsZero() {
						r.InstalledAt = a.FormatTime(sub.InstalledAt)
					}
					if !sub.CheckedAt.IsZero() {
						r.Checked = a.FormatTime(sub.CheckedAt)
					}
					r.Tracking = subscriptionTracking(sub)
					r.LoversLab = sub.Kind == "page" && subscribe.LoversLabPage(sub.URL)
					r.Opens = subscriptionOpens(sub)
				}
				if s.Key == app.LibWorkshop || s.Key == app.LibSteamCMD {
					r.Opens = "notes"
				}
				if sub := inst.Subscription; sub != nil && sub.HasUpdate() {
					r.Installed, r.Available = subscriptionValue(a, sub.Kind, sub.Installed), subscriptionValue(a, sub.Kind, sub.Latest)
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
	sort.SliceStable(recent, func(i, j int) bool { return recent[i].at.After(recent[j].at) })
	p.Recent = []RecentSubscription{}
	for i := 0; i < len(recent) && i < 5; i++ {
		p.Recent = append(p.Recent, recent[i].r)
	}
	p.Summary = sourceSummary(a, p.Sources, checks)
	return p, nil
}

// sourceSummary words the sources in one line: how many were checked and when the last was,
// and the failures of the last checks.
func sourceSummary(a *app.App, sources []SubscriptionSource, checks map[string]app.SourceCheck) SourceSummary {
	var sum SourceSummary
	var last time.Time
	n := 0
	var groups []SourceFailureGroup
	for _, s := range sources {
		if s.Key == app.LibLocal {
			continue
		}
		if c := checks[s.Key]; !c.At.IsZero() {
			n++
			if c.At.After(last) {
				last = c.At
			}
		}
		for _, g := range s.Failures {
			sum.Failures += len(g.Items)
			groups = append(groups, g)
		}
	}
	sum.Line = T("sources not checked yet")
	if n > 0 {
		sum.Line = Tf("checked %d sources, the last at %s", n, a.FormatTime(last))
	}
	if len(groups) == 1 {
		sum.Failed = groups[0].Label
	} else if sum.Failures > 0 {
		sum.Failed = Tf("%d checks failed", sum.Failures)
	}
	return sum
}

// subscriptionTracking: what a Git subscription follows.
func subscriptionTracking(r *subscribe.Record) string {
	if r.Kind != "git" || r.RefType == "" {
		return ""
	}
	if r.RefType == "branch" && r.Ref == "" {
		return T("the default branch")
	}
	return T(r.RefType) + " " + r.Ref
}

// subscriptionOpens: what 打开来源页 opens for a subscription (see PendingUpdate.Opens).
func subscriptionOpens(r *subscribe.Record) string {
	switch r.Kind {
	case "git":
		if compareURL(r) != "" {
			return "compare"
		}
		return "page"
	case "page":
		return "page"
	}
	return ""
}

// SubscriptionSourceURL is the page 打开来源页 opens: the compare view of the installed and
// the latest commit on GitHub or GitLab, else the repository or download page.
func SubscriptionSourceURL(r *subscribe.Record) string {
	if r.Kind == "git" {
		if c := compareURL(r); c != "" {
			return c
		}
		return r.GitURL()
	}
	return r.URL
}

func compareURL(r *subscribe.Record) string {
	if !r.HasUpdate() {
		return ""
	}
	u, err := url.Parse(strings.TrimSuffix(r.GitURL(), ".git"))
	if err != nil || u.Scheme != "https" {
		return ""
	}
	path := strings.Trim(u.Path, "/")
	switch u.Host {
	case "github.com":
		return "https://github.com/" + path + "/compare/" + r.Installed + "..." + r.Latest
	case "gitlab.com":
		return "https://gitlab.com/" + path + "/-/compare/" + r.Installed + "..." + r.Latest
	}
	return ""
}

// subscriptionValue words an installed or latest value: a short commit, or a page's
// "version @ date" with the date in the app's time.
func subscriptionValue(a *app.App, kind, v string) string {
	if kind == "git" && len(v) > 7 {
		return v[:7]
	}
	version, date, ok := strings.Cut(v, " @ ")
	if !ok {
		version, date = "", v
	}
	if t, err := time.Parse(time.RFC3339, date); err == nil {
		date = a.FormatDate(t)
	} else if !ok {
		return v
	}
	if version == "" {
		return date
	}
	return version + "  " + date
}
