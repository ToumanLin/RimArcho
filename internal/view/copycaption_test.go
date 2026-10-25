package view_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestCopyCaptionsAcrossPages(t *testing.T) {
	view.SetLang("zh")
	e := libEnv(t)
	e.MustRun("pin", "ws:5100")
	e.WriteMod(filepath.Join(e.Game, "Mods", "Lockable Copy"), testkit.Sample("tester.lock", "Legacy Lockable"))
	e.MustRun("scan")
	b, err := view.BuildBoard(e.App)
	if err != nil {
		t.Fatal(err)
	}
	p := libPage(t, e, "")
	labels := map[string]string{}
	for _, row := range p.Rows {
		for _, copy := range row.Insts {
			labels[copy.ID] = copy.Label
		}
	}
	locked := false
	headings := map[string]string{}
	for _, item := range b.Cols[view.Inactive] {
		if name := headings[item.Lineage]; name != "" && name != item.Name {
			t.Fatalf("split lineage headings: %q and %q", name, item.Name)
		}
		headings[item.Lineage] = item.Name
	}
	for _, column := range b.Cols {
		for _, item := range column {
			label, ok := labels[item.ID]
			if !ok {
				continue
			} // official mods are not inventory entries
			if item.CopyLabel != label {
				t.Errorf("%s: editor %q, library %q", item.Name, item.CopyLabel, label)
			}
			if strings.Contains(label, "5100") || strings.Contains(label, "Lockable Copy") {
				t.Errorf("raw folder in caption: %q", label)
			}
			if strings.HasPrefix(label, "锁定版本") {
				locked = true
			}
		}
	}
	if !locked {
		t.Fatal("locked copy not tested")
	}
}
