package gui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// The window's import: preview changes nothing, apply is one step that u undoes, and a
// second apply needs a new preview. Without the window there are no dialogs.
func TestExchangeAPI(t *testing.T) {
	t.Parallel()
	_, api, _ := fixture(t)
	if x := api.ExchangeInfo(); x.Dialogs || len(x.Formats) != 8 || x.Formats[0].ID != "archo" {
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

// Native adapter fake: exercise selection/cancellation without opening an OS dialog.
type selectionNative struct {
	picked, title, start string
	filters              [][2]string
}

func (n *selectionNative) openFile(title string, filters [][2]string) (string, error) {
	n.title, n.filters = title, filters
	return n.picked, nil
}
func (n *selectionNative) openDir(title, start string) (string, error) {
	n.title, n.start = title, start
	return n.picked, nil
}
func (n *selectionNative) saveFile(string, string, [][2]string) (string, error) { return "", nil }
func (n *selectionNative) setClipboard(string) bool                             { return false }
func (n *selectionNative) clipboard() (string, bool)                            { return "", false }

func TestNativeAddressSelections(t *testing.T) {
	e, api, _ := fixture(t)
	n := &selectionNative{picked: filepath.Join(e.Root, "合成目录", "new")}
	api.x.native = n
	selected, err := api.PickFolder(filepath.Join(e.Root, "missing", "nested"))
	if err != nil || selected != n.picked || n.start != e.Root || n.title != view.T("choose a folder") {
		t.Fatal(selected, err, n)
	}
	selected, err = api.PickProgram()
	if err != nil || selected != n.picked || n.title != view.T("choose a program") || len(n.filters) != 0 {
		t.Fatal(selected, err, n)
	}
	selected, err = api.PickPackFile()
	if err != nil || selected != n.picked || n.title != view.T("choose a pack") || len(n.filters) != 1 || n.filters[0][1] != "*.archopack" {
		t.Fatal(selected, err, n)
	}
	n.picked = ""
	for _, pick := range []func() (string, error){func() (string, error) { return api.PickFolder(e.Root) }, api.PickProgram, api.PickImportFile, api.PickPackFile} {
		if path, err := pick(); err != nil || path != "" {
			t.Fatal(path, err)
		}
	}
}
