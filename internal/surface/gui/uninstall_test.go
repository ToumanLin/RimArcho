package gui

import (
	"github.com/ToumanLin/RimArcho/internal/source"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestUninstallPreviewCachesOnlySizes(t *testing.T) {
	t.Parallel()
	e, s, _ := fixture(t)
	file := filepath.Join(e.Home, "size-test")
	e.WriteFile(file, strings.Repeat("x", 1<<20))
	choice := view.UninstallChoice{}
	first, err := s.UninstallPlan(choice)
	if err != nil {
		t.Fatal(err)
	}
	item := func(p *view.UninstallPage, path string) view.UnItem {
		t.Helper()
		for _, st := range p.Steps {
			for _, it := range st.Items {
				if it.Path == path || samePhysicalPath(it.Path, path) {
					return it
				}
			}
		}
		t.Fatalf("preview missing %s", path)
		return view.UnItem{}
	}
	before := item(first, e.Home).Size
	if !strings.HasPrefix(before, "≈ ") {
		t.Fatalf("size is not marked as estimated: %q", before)
	}
	e.WriteFile(file, strings.Repeat("x", 2<<20))
	pack := filepath.Join(e.Home, "packs", "P")
	e.WriteFile(filepath.Join(pack, "pack.json"), `{"format":1,"name":"P","entries":[]}`)
	// These appear after the first preview: discovery and ownership checks must run again.
	foreign := filepath.Join(e.Game, "Mods", "_Archo_foreign")
	e.WriteMod(foreign, testkit.Sample("test.foreign", "Foreign"))
	choice.RemovePacks = true
	next, err := s.UninstallPlan(choice)
	if err != nil {
		t.Fatal(err)
	}
	if got := item(next, e.Home).Size; got != before {
		t.Fatalf("size was rescanned: %s -> %s", before, got)
	}
	if got := item(next, pack).Verb; got != view.T("recycle bin") {
		t.Fatalf("new pack/changed answer not planned: %s", got)
	}
	if got := item(next, foreign).Verb; got != view.T("left alone") {
		t.Fatalf("real directory deletion permitted: %s", got)
	}
	// The uncached preview entry point still measures current sizes.
	fresh, err := e.App.PlanUninstall(choice.Options())
	if err != nil {
		t.Fatal(err)
	}
	if fresh.EstimatedSizes {
		t.Fatal("execution plan uses preview estimates")
	}
	if got := item(view.UninstallView(fresh, false), e.Home).Size; got == strings.TrimPrefix(before, "≈ ") {
		t.Fatalf("execution planning reused stale size: %s", got)
	}
	e.App.ArchoProbe = func() []int { return []int{999} }
	blocked, err := s.UninstallPlan(choice)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Blocked == "" {
		t.Fatal("new process did not block warm preview")
	}
	e.App.ArchoProbe = func() []int { return nil }
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 77} }
	blocked, err = s.UninstallPlan(choice)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Blocked == "" {
		t.Fatal("running game did not block warm preview")
	}
}

func samePhysicalPath(a, b string) bool {
	x, err := source.RealPath(a)
	if err != nil {
		return false
	}
	y, err := source.RealPath(b)
	return err == nil && x == y
}

func TestUninstallPreviewRechecksForeignLinks(t *testing.T) {
	t.Parallel()
	e, s, _ := fixture(t)
	pack := filepath.Join(e.Home, "packs", "P")
	e.WriteFile(filepath.Join(pack, "pack.json"), `{"format":1,"name":"P","entries":[]}`)
	c := view.UninstallChoice{RemovePacks: true}
	p, err := s.UninstallPlan(c)
	if err != nil || p.Blocked != "" {
		t.Fatalf("initial preview: %+v %v", p, err)
	}
	link := filepath.Join(e.Game, "Mods", "foreign-pack")
	e.Junction(link, pack)
	p, err = s.UninstallPlan(c)
	if err != nil || p.Blocked == "" {
		t.Fatalf("new foreign incoming link not refused: %+v %v", p, err)
	}
	fresh, err := e.App.CheckUninstall(c.Options())
	if err != nil || fresh.Blocked() == nil {
		t.Fatalf("execution planning accepted foreign link: %+v %v", fresh, err)
	}
	if !source.IsLink(link) {
		t.Fatal("preview detached foreign link")
	}
}

func TestUninstallRunBlockedReleasesLocks(t *testing.T) {
	t.Parallel()
	e, s, _ := fixture(t)
	e.App.ArchoProbe = func() []int { return []int{77} }
	if err := s.UninstallRun(view.UninstallChoice{}); err == nil {
		t.Fatal("running process did not block execution")
	}
	if !s.mu.TryLock() {
		t.Fatal("blocked execution retained the GUI lock")
	}
	s.mu.Unlock()
	if !s.accountMu.TryLock() {
		t.Fatal("blocked execution retained the account lock")
	}
	s.accountMu.Unlock()
	if s.UninstallStatus().Running {
		t.Fatal("blocked preflight retained close guard")
	}
	if e.App.Uninstalled {
		t.Fatal("blocked execution uninstalled the fixture")
	}
}
