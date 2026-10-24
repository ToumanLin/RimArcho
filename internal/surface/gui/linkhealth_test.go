package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestNeedsLinkHealthRefreshAndConfirmedSnapshot(t *testing.T) {
	e, api, _ := fixture(t)
	target := filepath.Join(e.Root, "temporary-mod")
	e.MkdirAll(target)
	link := filepath.Join(e.Game, "Mods", "_Archo_unproven")
	e.Junction(link, target)
	_ = api.needsLocked()
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	needs := api.needsLocked()
	var action view.Action
	for _, item := range needs.Items {
		for _, act := range item.Actions {
			if act.Kind == view.ActRepairLinks {
				action = act
			}
		}
	}
	if action.Kind == "" || len(action.Lines) == 0 || action.Confirm == "" || !strings.Contains(action.Warn, "temporary-mod") {
		t.Fatal("missing confirmation evidence")
	}
	if !source.IsLink(link) {
		t.Fatal("needs page removed link")
	}
	// Repeated needs reads preserve this preview, but execution rechecks the entry.
	for i := 0; i < 35; i++ {
		_ = api.needsLocked()
	}
	e.MkdirAll(target)
	if _, err := api.DoAction(action, 0); err == nil {
		t.Fatal("stale preview executed after target restored")
	}
	if !source.IsLink(link) {
		t.Fatal("healthy link removed")
	}
}
