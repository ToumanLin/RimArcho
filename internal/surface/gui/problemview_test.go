package gui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestFocusedProblemAPIAndEnable(t *testing.T) {
	e, s, _ := fixture(t)
	e.WriteFile(filepath.Join(e.Workshop, "1001", "Defs", "Missing.xml"), `<Defs><HediffDef ParentName="BaseIllness"><defName>Illness</defName></HediffDef></Defs>`)
	e.MustRun("scan")
	e.MustRun("analyze")
	e.AddWorkshop("1002", testkit.Mod{PackageID: "test.provider", Name: "Provider", Files: map[string]string{"Defs/Base.xml": `<Defs><HediffDef Name="BaseIllness" Abstract="True" /></Defs>`}})
	e.MustRun("scan")
	ps, err := e.App.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	var ref string
	for _, p := range ps.List {
		if p.Code == "def.parent-missing" {
			ref = p.Ref
			break
		}
	}
	if ref == "" {
		t.Fatal("missing problem")
	}
	panel, err := s.ProblemDetail(ref)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(panel.Title, "1 added things cannot load") || len(panel.Affected) != 1 {
		t.Fatalf("panel %+v", panel)
	}
	if !panel.Searching {
		t.Fatal("missing pending search", panel)
	}
	panel, err = s.ProblemProviders(ref, s.BeginProblemProviders())
	if err != nil {
		t.Fatal(err)
	}
	if !panel.Primary || panel.Searching {
		t.Fatalf("completed panel %+v", panel)
	}
	var enable *view.Action
	for i, act := range panel.Actions {
		if act.Kind == view.ActEnable {
			enable = &panel.Actions[i]
		}
		if act.Kind == view.ActView {
			t.Fatal("card dump action")
		}
	}
	if enable == nil || enable.Label != "enable Provider" {
		t.Fatalf("enable %+v", enable)
	}
	if _, err := s.DoAction(*enable, 0); err != nil {
		t.Fatal(err)
	}
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	m, err := st.Ix.ResolveOne("pkg:test.provider")
	if err != nil || st.Position(m.ID) == 0 {
		t.Fatal("provider not enabled", err)
	}
	if _, err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ProblemDetail("p:000000"); err == nil {
		t.Fatal("stale reference accepted")
	}
}

func TestProblemProviderCancellation(t *testing.T) {
	_, s, _ := fixture(t)
	old := s.BeginProblemProviders()
	current := s.BeginProblemProviders()
	if _, err := s.ProblemProviders("p:000000", old); err != context.Canceled {
		t.Fatalf("superseded: %v", err)
	}
	s.CancelProblemProviders(old)
	s.problemLookup.mu.Lock()
	err := s.problemLookup.ctx.Err()
	s.problemLookup.mu.Unlock()
	if err != nil {
		t.Fatal("old close cancelled current search")
	}
	s.CancelProblemProviders(current)
	if _, err := s.ProblemProviders("p:000000", current); err != context.Canceled {
		t.Fatalf("closed: %v", err)
	}
	s.closeProblemProviders()
	if _, err := s.ProblemProviders("p:000000", s.BeginProblemProviders()); err != context.Canceled {
		t.Fatalf("shutdown: %v", err)
	}
}
