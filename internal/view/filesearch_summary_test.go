package view_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestFileSearchSummaryStates(t *testing.T) {
	view.SetLang("en")
	run := view.NewFileSearchRun()
	if summary := run.Update(0).Summary; summary != "searching…" {
		t.Fatalf("undiscovered scope: %q", summary)
	}
	e := testkit.New(t)
	e.AddEnv(false)
	e.MustRun("scan")
	run.Search(context.Background(), e.App, view.FileSearchReq{Pattern: "no-synthetic-match"})
	if u := run.Update(0); u.Summary != "not found" || u.Hint == "" {
		t.Fatalf("empty completion: %+v", u)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	run = view.NewFileSearchRun()
	run.Search(ctx, nil, view.FileSearchReq{})
	if summary := run.Update(0).Summary; !strings.HasPrefix(summary, "stopped:") {
		t.Fatalf("stopped is not empty completion: %q", summary)
	}
	run = view.NewFileSearchRun()
	run.Fail(errors.New("synthetic failure"))
	if u := run.Update(0); u.Summary == "not found" || u.Error == "" {
		t.Fatalf("failed is not empty completion: %+v", u)
	}
}
