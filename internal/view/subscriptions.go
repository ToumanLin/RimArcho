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

// PendingUpdate is one row of waiting for you to update (06 §2.9) and its detail.
type PendingUpdate struct {
	ID        string   `json:"id"`
	Ref       string   `json:"ref"`
	Name      string   `json:"name"`
	Source    string   `json:"source"`
	Kind      string   `json:"kind"`
	Installed string   `json:"installed"`
	Available string   `json:"available"`
	UsedIn    []string `json:"usedIn"`
	// InPlan: the current plan uses it (the scope bar's list shows only these).
	InPlan bool `json:"inPlan,omitempty"`
	// Tracking: a Git subscription's branch, tag or commit.
	Tracking string `json:"tracking,omitempty"`
	// InstalledAt: when the installed one was installed; Checked: the last check.
	InstalledAt string `json:"installedAt,omitempty"`
	Checked     string `json:"checked,omitempty"`
	// LoversLab: a download page subscription on LoversLab (its files can be read with the session).
	LoversLab bool `json:"loversLab,omitempty"`
	// Opens: what open the source page opens: "compare" (two commits on GitHub / GitLab), "page" (the
	// repository or download page), "notes" (the Workshop change notes), "" nothing.
	Opens string `json:"opens,omitempty"`
}

// RemovedSubscription is an installed copy whose Workshop page was deleted or made private.
type RemovedSubscription struct {
	ID     string `json:"id"`
	Ref    string `json:"ref"`
	Name   string `json:"name"`
	InPlan bool   `json:"inPlan,omitempty"`
}

// SourceSummary is the selectable Update checks row. Only real check failures count here.
type SourceSummary struct {
	Line     string `json:"line"`
	Failures int    `json:"failures,omitempty"`
}

// RecentSubscription is one of the newest subscriptions RimArcho installed (add a subscription).
type RecentSubscription struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Added  string `json:"added"`
}

type SubscriptionsPage struct {
	Sources []SubscriptionSource  `json:"sources"`
	Summary SourceSummary         `json:"summary"`
	Pending []PendingUpdate       `json:"pending"`
	Gone    []RemovedSubscription `json:"gone"`
	Recent  []RecentSubscription  `json:"recent"`
	History *modchanges.Page      `json:"history"`
}

func BuildSubscriptions(a *app.App, st *app.LoadoutState, o modchanges.Opts) (*SubscriptionsPage, error) {
	lib, err := a.Library()
	if err != nil {
		return nil, err
	}
	return BuildSubscriptionsFromLibrary(a, st, o, lib)
}

// BuildSubscriptionsFromLibrary reuses a read-only inventory; history still follows
// the requested scope. The caller invalidates the inventory when provenance changes.
func BuildSubscriptionsFromLibrary(a *app.App, st *app.LoadoutState, o modchanges.Opts, lib *app.Library) (*SubscriptionsPage, error) {
	history, err := BuildChanges(a, st, o)
	if err != nil {
		return nil, err
	}
	p := &SubscriptionsPage{History: history, Pending: []PendingUpdate{}, Gone: []RemovedSubscription{}}
	keys := []string{app.LibWorkshop, app.LibSteamCMD, app.LibGit, app.LibPage, app.LibLocal}
	names := []string{T("Steam Workshop (client)"), "SteamCMD", "Git", T("download page / LoversLab"), T("local (no source)")}
	checks := map[string]app.SourceCheck{}
	gone := map[string]map[string]bool{}
	for i, key := range keys {
		c := a.SubscriptionSourceCheck(key)
		checks[key] = c
		r := SubscriptionSource{Key: key, Name: names[i], Result: T("not checked yet")}
		if !c.At.IsZero() {
			r.Checked, r.Result = a.FormatTime(c.At), Text(c.Result)
			gone[key] = map[string]bool{}
			for _, failure := range c.Failures {
				if failure.Gone() {
					gone[key][failure.ID] = true
					continue
				}
				reason := failure.Reason
				if failure.Code != 0 {
					reason = app.WorkshopResultText(failure.Code)
				}
				addSourceFailure(&r, failure.Name, Text(reason))
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
				if sub := inst.Subscription; sub != nil && sub.CheckError != "" && inst.Mod != nil && !inst.Missing() {
					addSourceFailure(s, l.Name, Text(sub.CheckError))
				}
				if inst.Mod == nil || inst.Missing() || s.Key == app.LibLocal {
					continue
				}
				if gone[s.Key][inst.WorkshopID] {
					p.Gone = append(p.Gone, RemovedSubscription{ID: inst.Mod.ID, Ref: inst.Ref, Name: l.Name, InPlan: inst.InPlan})
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
	for i := range p.Sources {
		s := &p.Sources[i]
		n := 0
		for _, update := range p.Pending {
			if update.Kind == s.Key {
				n++
			}
		}
		if s.Result == T("check complete") || s.Result == T("up to date") {
			s.Result = UpdateCheckResult(n, "check complete")
		}
		failed := 0
		for _, g := range s.Failures {
			failed += len(g.Items)
		}
		if failed == 1 {
			s.Result = T("1 check failed")
		} else if failed > 1 {
			s.Result = Tf("%d checks failed", failed)
		}
	}

	p.Recent = recentSubscriptions(a, lib.Lineages)
	p.Summary = sourceSummary(a, p.Sources, checks)
	return p, nil
}

// addSourceFailure groups reasons without hiding the affected mod names.
func addSourceFailure(s *SubscriptionSource, name, reason string) {
	for i := range s.Failures {
		if s.Failures[i].Label == reason {
			s.Failures[i].Items = append(s.Failures[i].Items, name+": "+reason)
			return
		}
	}
	s.Failures = append(s.Failures, SourceFailureGroup{Label: reason, Items: []string{name + ": " + reason}})
}

func sourceSummary(a *app.App, sources []SubscriptionSource, checks map[string]app.SourceCheck) SourceSummary {
	var sum SourceSummary
	var last time.Time
	n := 0
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
		}
	}
	sum.Line = T("sources not checked yet")
	if n > 0 {
		sum.Line = Tf("checked %d sources, the last at %s", n, a.FormatTime(last))
	}
	if sum.Failures == 1 {
		sum.Line = Tf("%s, 1 check failed", sum.Line)
	} else if sum.Failures > 1 {
		sum.Line = Tf("%s, %d checks failed", sum.Line, sum.Failures)
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

// subscriptionOpens: what open the source page opens for a subscription (see PendingUpdate.Opens).
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

// SubscriptionSourceURL is the page open the source page opens: the compare view of the installed and
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

// AddSubscriptionInfo is the account and recent-install data used by the add form.
type AddSubscriptionInfo struct {
	Sources []SubscriptionSource `json:"sources"`
	Recent  []RecentSubscription `json:"recent"`
}

func BuildAddSubscriptionInfo(a *app.App) (*AddSubscriptionInfo, error) {
	ls, err := a.SubscriptionLineages()
	if err != nil {
		return nil, err
	}
	ll, _ := LoversLabLine(a)
	return &AddSubscriptionInfo{
		Sources: []SubscriptionSource{
			{Key: app.LibSteamCMD, Account: SteamAccountLine(a.SteamAccountStatus())},
			{Key: app.LibPage, Account: ll},
		},
		Recent: recentSubscriptions(a, ls),
	}, nil
}

func recentSubscriptions(a *app.App, ls []*app.LibLineage) []RecentSubscription {
	type added struct {
		r  RecentSubscription
		at time.Time
	}
	names := map[string]string{app.LibSteamCMD: "SteamCMD", app.LibGit: "Git", app.LibPage: T("download page / LoversLab")}
	var recent []added
	for _, l := range ls {
		for _, inst := range l.Instances {
			name, ok := names[inst.Source]
			if !ok || inst.Mod == nil || inst.Missing() {
				continue
			}
			if sub := inst.Subscription; sub != nil && !sub.InstalledAt.IsZero() {
				recent = append(recent, added{RecentSubscription{ID: inst.Mod.ID, Name: l.Name, Kind: inst.Source, Source: name, Added: a.FormatDate(sub.InstalledAt)}, sub.InstalledAt})
			}
		}
	}
	sort.SliceStable(recent, func(i, j int) bool { return recent[i].at.After(recent[j].at) })
	out := []RecentSubscription{}
	for i := 0; i < len(recent) && i < 5; i++ {
		out = append(out, recent[i].r)
	}
	return out
}
