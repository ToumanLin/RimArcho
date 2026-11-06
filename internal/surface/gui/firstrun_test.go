package gui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/surface/gui"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestManagerPreviewApply(t *testing.T) {
	e := testkit.New(t)
	e.AddWorkshop("3000000001", testkit.Sample("tester.alpha", "Alpha"))
	e.AddEnv(false)
	e.MustRun("scan")
	api := gui.NewAPI(e.App, nil)
	defer api.Close()
	path := filepath.Join(e.Root, "RimSort", "dbs", "userRules.json")
	original := `{"rules":{"tester.alpha":{"loadAfter":{"ludeon.rimworld":{}}},"tester.absent":{"loadTop":{"value":true}}}}`
	e.WriteFile(path, original)
	p, err := api.PreviewManagerImport("rimsort-rules", path)
	if err != nil {
		t.Fatal(err)
	}
	env, _ := e.App.Env()
	rules, _ := e.App.Store.Rules(env.ID)
	if len(rules) != 0 || !strings.Contains(p.Text, "1 new") || !strings.Contains(p.Text, "1 skipped") {
		t.Fatalf("preview wrote data or wrong counts: %+v %+v", rules, p)
	}
	if _, err := api.ApplyManagerImport(p.Request + 1); err == nil {
		t.Fatal("accepted another preview")
	}
	e.WriteFile(path, `{"rules":{}}`)
	if _, err := api.ApplyManagerImport(p.Request); err == nil {
		t.Fatal("accepted changed source")
	}
	e.WriteFile(path, original)
	p, err = api.PreviewManagerImport("rimsort-rules", path)
	if err != nil {
		t.Fatal(err)
	}

	if err := e.App.Store.SetMeta("test-preview-change", "changed"); err != nil {
		t.Fatal(err)
	}
	if _, err := api.ApplyManagerImport(p.Request); err == nil {
		t.Fatal("accepted changed RimArcho data")
	}
	p, err = api.PreviewManagerImport("rimsort-rules", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.ApplyManagerImport(p.Request); err != nil {
		t.Fatal(err)
	}
	rules, _ = e.App.Store.Rules(env.ID)
	if len(rules) != 1 {
		t.Fatalf("rules: %+v", rules)
	}
	p, err = api.PreviewManagerImport("rimsort-rules", path)
	if err != nil || !strings.Contains(p.Text, "1 already present") {
		t.Fatalf("repeat: %+v %v", p, err)
	}
	if b, _ := os.ReadFile(path); string(b) != original {
		t.Fatal("source changed")
	}
}
