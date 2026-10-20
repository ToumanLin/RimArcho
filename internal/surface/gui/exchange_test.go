package gui

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// The window's import: preview changes nothing, apply is one step that u undoes, and a
// second apply needs a new preview. Without the window there are no dialogs.
func TestExchangeAPI(t *testing.T) {
	_, api, _ := fixture(t)
	if x := api.ExchangeInfo(); x.Dialogs || len(x.Formats) != 6 {
		t.Fatalf("info: %+v", x)
	}
	if f, err := api.PickImportFile(); f != "" || err != nil || api.Clipboard() != "" {
		t.Fatal("a dialog without the window")
	}
	p, err := api.ImportPreview(view.ImportText, "ludeon.rimworld\nbrrainz.harmony\ntester.a\ntester.b")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Installed) != 4 || p.Replace.Added != 2 {
		t.Fatalf("preview: %+v", p)
	}
	if got := names(api.Board().Cols[view.Active]); got != "Core,Alpha" {
		t.Fatalf("preview changed the list: %s", got)
	}
	if _, err := api.ImportApply(false); err != nil {
		t.Fatal(err)
	}
	if got := names(api.Board().Cols[view.Active]); got != "Core,Harmony,Alpha,Beta" {
		t.Fatalf("after import: %s", got)
	}
	if _, err := api.ImportApply(true); err == nil {
		t.Fatal("applied twice from one preview")
	}
	api.Undo()
	if got := names(api.Board().Cols[view.Active]); got != "Core,Alpha" {
		t.Fatalf("after undo: %s", got)
	}
	c, err := api.ExportCopy("rimcrow")
	if err != nil || c.Copied || !strings.HasPrefix(c.Text, "RC-") {
		t.Fatalf("copy: %v %+v", err, c)
	}
}
