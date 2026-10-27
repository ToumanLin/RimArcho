package gui

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/view"
	"strings"
)

// Opt-in acceptance: copies the DB/config/cache inputs; only reads real Mod folders.
func TestRealSortDependencies(t *testing.T) {
	source := os.Getenv("RIMARCHO_SORT_DEPS_HOME")
	if source == "" {
		t.Skip("set RIMARCHO_SORT_DEPS_HOME to copy and test real data")
	}
	home := copySortRealHome(t, source)
	a, err := app.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.EnvName = "main"
	a.PlanName = "The Progression Modpack [UPDATED 1.7]"
	st, err := a.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	enabled, missing := 0, 0
	for _, e := range st.List {
		if e.Instance != "" {
			enabled++
		} else {
			missing++
		}
	}
	t.Logf("source copy: plan=%q game=%s enabled=%d missing entries=%d", st.Plan.Name, st.GameMM(), enabled, missing)
	api := &API{a: a}
	started := time.Now()
	sorted, err := api.Run("sort")
	elapsed := time.Since(started)
	if err != nil || len(sorted.Refused) > 0 {
		t.Fatalf("sort: %+v %v", sorted, err)
	}
	counts := map[string]int{}
	for _, d := range sorted.MissingDependencies {
		counts[d.Kind]++
	}
	raw, _ := json.Marshal(sorted)
	t.Logf("sort API: %s; missing dependencies=%d categories=%v; JSON=%s", elapsed, len(sorted.MissingDependencies), counts, raw)
	st, _ = a.Loadout()
	var durations []time.Duration
	for range 10 {
		started = time.Now()
		_, err = a.MissingDependencies(st)
		d := time.Since(started)
		durations = append(durations, d)
		if err != nil {
			t.Fatal(err)
		}
		if d >= 100*time.Millisecond {
			t.Errorf("dependency check exceeded 100 ms: %s", d)
		}
	}
	t.Logf("dependency check (10 samples): %v", durations)
	// Record one live XML declaration per returned dependency.
	for i, dep := range sorted.MissingDependencies {
		if i >= 5 {
			break
		}
		found := false
		for _, entry := range st.List {
			m := st.Ix.ByID[entry.Instance]
			if m == nil {
				continue
			}
			decl := scan.ReadDecl(m.RealPath, m.Kind == "official")
			deps := append(decl.Dependencies, decl.DependenciesByVersion["v"+st.GameMM()]...)
			for _, d := range deps {
				if strings.TrimSuffix(strings.ToLower(d.PackageID), "_steam") != dep.PackageID {
					continue
				}
				t.Logf("CHECK %s required by %s: %s; live dependency=%+v parse error=%s", dep.PackageID, m.DisplayName(), filepath.Join(m.RealPath, "About", "About.xml"), d, decl.ParseError)
				found = true
				break
			}
			if found {
				break
			}
		}
		if !found {
			t.Fatalf("no live XML evidence for %s", dep.PackageID)
		}
	}
	before := st.List.Clone()
	last := a.Store.LastStepID(st.Plan.ID, "undo")
	started = time.Now()
	completed, err := api.CompleteDependencies()
	if err != nil {
		t.Fatal(err)
	}
	st, _ = a.Loadout()
	afterEnabled := 0
	for _, e := range st.List {
		if e.Instance != "" {
			afterEnabled++
		}
	}
	raw, _ = json.Marshal(completed)
	t.Logf("completion API: %s; enabled %d -> %d; JSON=%s", time.Since(started), enabled, afterEnabled, raw)
	if len(completed.Enabled) != afterEnabled-enabled {
		t.Fatal("enabled count mismatch")
	}
	if completed.Undo {
		if _, err := api.Undo(); err != nil {
			t.Fatal(err)
		}
		restored, _ := a.Loadout()
		if !restored.List.Equal(before) || a.Store.LastStepID(st.Plan.ID, "undo") != last {
			t.Fatal("single undo did not restore list/history")
		}
		t.Log("one undo restored the exact sorted list and undo-stack head")
	} else {
		t.Log("no installed dependencies to enable; no undo step created")
	}
	// Supplementary performance load: 713 real installed instances, preserving the
	// actual plan above as the acceptance target. No synthetic declarations.
	bench := *st
	bench.List = nil
	used := map[string]bool{}
	for _, e := range st.List {
		if m := st.Ix.ByID[e.Instance]; m != nil {
			bench.List = append(bench.List, e)
			used[strings.ToLower(m.PackageID)] = true
		}
	}
	for _, m := range st.Ix.Mods {
		p := strings.ToLower(m.PackageID)
		if len(bench.List) >= 713 {
			break
		}
		if used[p] || m.Status != "live" || m.Residue || m.Unloadable != "" || !(st.Reg.Loadable(m.ID) || st.Deployable(m)) {
			continue
		}
		used[p] = true
		bench.List = append(bench.List, loadout.Entry{Instance: m.ID})
	}
	if len(bench.List) != 713 {
		t.Fatalf("not enough live real instances for 713 load: %d", len(bench.List))
	}
	started = time.Now()
	deps, err := a.MissingDependencies(&bench)
	elapsed = time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("supplementary 713 real installed instances (not original plan): check=%s missing deps=%d", elapsed, len(deps))
	if elapsed >= 100*time.Millisecond {
		t.Fatal("713-instance check exceeded 100 ms")
	}
}

func copySortRealHome(t *testing.T, source string) string {
	t.Helper()
	home := t.TempDir()
	paths, err := filepath.Glob(filepath.Join(source, "archo.db*"))
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, filepath.Join(source, "config.toml"))
	for _, name := range []string{"steamDB.index.json", "communityRules.json", "replacements.json.gz"} {
		path := filepath.Join(source, "cache", name)
		if _, err := os.Stat(path); err == nil {
			paths = append(paths, path)
		}
	}
	for _, path := range paths {
		rel, _ := filepath.Rel(source, path)
		dest := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			t.Fatal(err)
		}
		in, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		out, err := os.Create(dest)
		if err != nil {
			in.Close()
			t.Fatal(err)
		}
		_, err = io.Copy(out, in)
		in.Close()
		out.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func TestRealSortNotices(t *testing.T) {
	source := os.Getenv("RIMARCHO_SORT_DEPS_HOME")
	if source == "" {
		t.Skip("set RIMARCHO_SORT_DEPS_HOME to test real data copies")
	}
	home := copySortRealHome(t, source)
	a, err := app.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.EnvName = "main"
	for _, name := range []string{"The Progression Modpack [UPDATED 1.7]", "default"} {
		t.Run(name, func(t *testing.T) {
			a.PlanName = name
			api := &API{a: a}
			st, err := a.Loadout()
			if err != nil {
				t.Fatal(err)
			}
			started := time.Now()
			out, err := api.Run("sort")
			if err != nil || len(out.Refused) > 0 {
				t.Fatalf("sort refused: %+v %v", out, err)
			}
			took := time.Since(started)
			known, available := 0, 0
			for _, m := range out.MissingMods {
				if m.CanEnable {
					available++
				}
				if m.WorkshopID != "" {
					known++
				}
			}
			t.Logf("plan=%q entries=%d sort=%s missingMods=%d (known WS=%d canEnable=%d) replacements=%d cycles=%d deps=%d", name, len(st.List), took, len(out.MissingMods), known, available, len(out.Replacements), len(out.Cycles), len(out.MissingDependencies))
			data, _ := json.Marshal(out)
			t.Logf("Outcome JSON=%s", data)
			// Cross-check unresolved entries against the local instance records.
			for i, m := range out.MissingMods {
				matches := 0
				for _, local := range st.Ix.Mods {
					if strings.EqualFold(local.PackageID, m.PackageID) && local.Status == "live" && !local.Residue && local.Unloadable == "" {
						matches++
					}
				}
				if m.Installed != (matches > 0) {
					t.Logf("installed includes blocked instances for %s", m.PackageID)
				}
				if i < 3 {
					t.Logf("MISSING CHECK %s title=%s ws=%s live matching installed instances=%d", m.PackageID, m.Name, m.WorkshopID, matches)
				}
			}
			// A real cycle must sort successfully, name the discarded rule, and leave a dim note.
			n := api.needsLocked()
			notes := 0
			for _, it := range n.Items {
				if !strings.Contains(it.Text, "cycle") {
					continue
				}
				notes++
				if !it.Dim || it.Counted || it.Status == view.StatusErr || it.Why == "" {
					t.Fatal("cycle displayed as an error", it)
				}
				if notes <= 3 {
					t.Logf("NEEDS CYCLE: %s; %s", it.Text, it.Why)
				}
			}
			if len(out.Cycles) > 0 && notes == 0 {
				t.Fatal("no cycle note")
			}
			for i, cy := range out.Cycles {
				if i >= 2 {
					break
				}
				for _, edge := range cy.Broken {
					t.Logf("CYCLE CHECK %s -> %s kind=%s strength=%s reason=%s", edge.Before.Name, edge.After.Name, edge.Kind, edge.Strength, edge.Reason)
					// Read declarations on disk for every member of this sampled cycle.
					for _, member := range cy.Members {
						m := st.Ix.ByID[member.Instance]
						if m == nil {
							continue
						}
						d := scan.ReadDecl(m.RealPath, m.Kind == "official")
						targets := map[string]bool{}
						for _, x := range cy.Members {
							targets[strings.ToLower(x.PackageID)] = true
						}
						var before, after []string
						for _, p := range d.LoadBefore {
							if targets[strings.ToLower(p)] {
								before = append(before, p)
							}
						}
						for _, p := range d.LoadAfter {
							if targets[strings.ToLower(p)] {
								after = append(after, p)
							}
						}
						t.Logf("LIVE XML %s: path=%s deps=%+v cycle loadBefore=%v cycle loadAfter=%v", member.Name, filepath.Join(m.RealPath, "About", "About.xml"), d.Dependencies, before, after)
					}
				}
			}
		})
	}
}
