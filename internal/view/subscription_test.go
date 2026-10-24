package view_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestSubscriptionLibrary(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	dir := filepath.Join(e.App.SubscribedDir(), "Example")
	e.WriteMod(dir, testkit.Sample("example.subscription", "Example Colony Tweaks"))
	r := &subscribe.Record{Kind: "git", URL: "https://example.invalid/page", Repo: "https://gitgud.io/Example/colony", RefType: "branch", Ref: "main",
		Installed: "1234567890abcdef", Latest: "abcdef1234567890", InstalledAt: e.Now(), CheckedAt: e.Now()}
	if err := subscribe.Write(dir, r); err != nil {
		t.Fatal(err)
	}
	if err := e.App.Store.InsertSource(&source.Source{ID: e.App.NewID(), EnvID: store.Shared, Kind: source.Extra, Label: app.SubscribedLabel, Root: e.App.SubscribedDir(), Status: source.StatusOK}); err != nil {
		t.Fatal(err)
	}
	e.MustRun("scan")
	lib, err := e.App.Library()
	if err != nil {
		t.Fatal(err)
	}
	p, err := view.BuildLibrary(lib, "", []string{"source:git", "state:source-update"}, "")
	if err != nil || len(p.Rows) != 1 {
		t.Fatalf("filtered: %+v %v", p, err)
	}
	i := p.Rows[0].Insts[0]
	if i.Source != "Git" || i.Code != app.LibSourceUpdate {
		t.Fatalf("row: %+v", i)
	}
	for _, code := range []string{view.OpSubCheck, view.OpSubUpdate, view.OpSubRemove} {
		if !slices.Contains(i.Ops, code) {
			t.Errorf("missing op %s: %v", code, i.Ops)
		}
	}
	if slices.Contains(i.Ops, view.OpDelete) || slices.Contains(i.Ops, view.OpSubPage) {
		t.Fatalf("inapplicable ops: %v", i.Ops)
	}
	if view.CompleteLibrary("source:gi", lib) != "t" || view.CompleteLibrary("state:source-u", lib) != "pdate" {
		t.Fatal("missing subscription completion")
	}
	if slices.Index(app.LibStates, app.LibSourceUpdate) != slices.Index(app.LibStates, app.LibUpdated)+1 {
		t.Fatal("state order")
	}
	fields := view.LibFields(e.App, lib, lib.Instance(i.Ref), nil)
	values := map[string]view.Field{}
	for _, f := range fields {
		values[f.Kind] = f
	}
	if values["subscription-url"].URL != r.URL || values["subscription-repo"].URL != r.Repo || values["subscription-tracking"].Value != "branch main" || !strings.Contains(values["subscription-installed"].Value, "1234567") || strings.Contains(values["subscription-installed"].Value, "12345678") || !strings.Contains(values["subscription-check"].Value, "update available") {
		t.Fatalf("fields: %+v", fields)
	}
	for _, args := range [][]string{{"library", "--include", "source:git"}, {"library", "--lineage", i.Ref}, {"mod", i.Ref}} {
		out := e.MustRun(args...)
		if args[0] == "library" && args[1] == "--include" {
			if !strings.Contains(out, "Git") {
				t.Fatal(out)
			}
			continue
		}
		if !strings.Contains(out, "repository:") || !strings.Contains(out, "last check:") {
			t.Fatal(out)
		}
	}
	e.MustRun("enable", i.Ref)
	preview, err := view.LibPreview(e.App, view.OpSubRemove, []string{i.Ref})
	if err != nil || !preview.Apply || !strings.Contains(strings.Join(preview.Lines, " "), "default") {
		t.Fatalf("removal warning: %+v %v", preview, err)
	}
	copy := lib.Instance(i.Ref)
	copy.Subscription.Kind = "page"
	copy.Source = app.LibPage
	if !slices.Contains(view.CopyOps(lib, nil, copy, view.LibraryPage), view.OpSubPage) {
		t.Fatal("page has no open action")
	}
	copy.Subscription.Latest = copy.Subscription.Installed
	if slices.Contains(view.CopyOps(lib, nil, copy, view.LibraryPage), view.OpSubUpdate) {
		t.Fatal("update without an update")
	}
}

func TestSubscriptionFields(t *testing.T) {
	view.SetLang("zh")
	defer view.SetLang("en")
	for _, typ := range []string{"branch", "tag", "commit", "page", "steamcmd"} {
		r := &subscribe.Record{Kind: "git", RefType: typ, Ref: "release", Installed: "123456789", InstalledAt: testkit.Start, CheckedAt: testkit.Start, CheckError: "offline"}
		if typ == "page" {
			r.Kind, r.Installed = "page", "1.2 @ 2026-10-24T00:00:00Z"
		}
		if typ == "steamcmd" {
			r.Kind, r.WorkshopID = "steamcmd", "1234"
		}
		fs := view.SubscriptionFields(r)
		text := ""
		for _, f := range fs {
			text += f.Label + " " + f.Value + "\n"
		}
		if typ == "steamcmd" {
			if !strings.Contains(text, "1234") || !strings.Contains(text, "上次下载") {
				t.Fatal(text)
			}
		} else if !strings.Contains(text, "失败：offline") || !strings.Contains(text, "2026") {
			t.Fatal(text)
		}
		if typ == "page" && !strings.Contains(text, r.Installed) {
			t.Fatal("page stamp shortened")
		}
	}
	fs := view.SubscriptionFields(&subscribe.Record{Kind: "git", RefType: "branch"})
	if fs[0].Value != "默认分支" {
		t.Fatalf("default: %+v", fs)
	}
	m := &store.Mod{ID: "example", Kind: source.SteamCMD, Rel: "1234"}
	i := &app.LibInstance{Mod: m, Source: app.LibSteamCMD, WorkshopID: "1234", Subscription: &subscribe.Record{Kind: "steamcmd", WorkshopID: "1234", InstalledAt: testkit.Start}}
	fs = view.LibFields(nil, &app.Library{}, i, nil)
	count := 0
	for _, f := range fs {
		if f.Kind == view.FieldWorkshop {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("SteamCMD workshop ID appears %d times: %+v", count, fs)
	}
	ops := view.CopyOps(nil, nil, i, view.LibraryPage)
	if !slices.Contains(ops, view.OpSubRemove) || slices.Contains(ops, view.OpDelete) || slices.Contains(ops, view.OpSubCheck) || slices.Contains(ops, view.OpSubUpdate) {
		t.Fatalf("SteamCMD ops: %v", ops)
	}
}
