package view

import (
	"slices"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
)

func appendSubscriptionFields(fs []Field, r *subscribe.Record) []Field {
	for _, f := range SubscriptionFields(r) {
		if f.Kind == FieldWorkshop && slices.ContainsFunc(fs, func(existing Field) bool { return existing.Kind == FieldWorkshop }) {
			continue
		}
		fs = append(fs, f)
	}
	return fs
}

func SubscriptionResult(op *app.SubscribeOp) string {
	if op == nil {
		return ""
	}
	var lines []string
	for _, it := range op.Items {
		result := Text(it.Result)
		switch result {
		case "up to date":
			result = T("up to date")
		case "update available":
			result = T("update available")
		case "updated":
			result = T("updated")
		case "installed":
			result = T("subscription finished")
		case "moved to recycle bin":
			result = T("moved to the recycle bin")
		}
		lines = append(lines, strings.TrimSpace(it.Name+"  "+result))
	}
	for _, note := range op.Notes {
		if note == "Steam downloads it; it appears after the next scan." {
			note = T("Steam downloads it; it appears after the next scan")
		}
		lines = append(lines, Text(note))
	}
	return strings.Join(lines, "  ")
}

// SubscriptionFields are shared by both detail views; reading them never checks the network.
func SubscriptionFields(r *subscribe.Record) []Field {
	if r == nil {
		return nil
	}
	var fs []Field
	url := func(kind, label, value string) {
		if value == "" {
			return
		}
		f := Field{Kind: kind, Label: T(label), Value: value}
		if strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://") {
			f.Act, f.ActAt, f.URL = ActURL, T("web page"), value
		}
		fs = append(fs, f)
	}
	url("subscription-url", "subscription origin", r.URL)
	url("subscription-repo", "repository", r.Repo)
	if r.Kind == "steamcmd" {
		fs = append(fs, workshopField(r.WorkshopID))
		if !r.InstalledAt.IsZero() {
			fs = append(fs, Field{Kind: "subscription-installed", Label: T("last download"), Value: LongDate(r.InstalledAt)})
		}
		return fs
	}
	if r.Kind == "git" {
		tracking := T("default branch")
		if r.Ref != "" {
			tracking = T(r.RefType) + " " + r.Ref
		}
		fs = append(fs, Field{Kind: "subscription-tracking", Label: T("tracking"), Value: tracking})
	}
	installed := r.Installed
	if r.Kind == "git" && len(installed) > 7 {
		installed = installed[:7]
	}
	if !r.InstalledAt.IsZero() {
		installed += "  " + LongDate(r.InstalledAt)
	}
	fs = append(fs, Field{Kind: "subscription-installed", Label: T("installed version"), Value: installed})
	check := T("not checked")
	if !r.CheckedAt.IsZero() {
		check = checkedAt(r.CheckedAt) + "  " + T("up to date")
		if r.HasUpdate() {
			check = checkedAt(r.CheckedAt) + "  " + T("update available")
		}
	}
	if r.CheckError != "" {
		check = Tf("failed: %s", Text(r.CheckError))
		if !r.CheckedAt.IsZero() {
			check = checkedAt(r.CheckedAt) + "  " + check
		}
	}
	fs = append(fs, Field{Kind: "subscription-check", Label: T("last check"), Value: check})
	return fs
}

// checkedAt is a check time as the rest of the page writes dates: "Oct 24 12:49".
func checkedAt(t time.Time) string { return ShortDate(t) + " " + t.Local().Format("15:04") }
