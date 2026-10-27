package gui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

type needsOffline struct{}

func (needsOffline) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("offline measurement")
}
func TestNeedsRealCopy(t *testing.T) {
	home := os.Getenv("RIMARCHO_NEEDS_HOME")
	if home == "" {
		t.Skip("isolated real-data home required")
	}
	if _, err := os.Stat(home + "/needs-benchmark-copy"); err != nil {
		t.Fatal("refusing to open a home without the needs-benchmark-copy marker")
	}
	a, err := app.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.EnvName = os.Getenv("RIMARCHO_NEEDS_ENV")
	if a.EnvName == "" {
		a.EnvName = "main"
	}
	if plan := os.Getenv("RIMARCHO_NEEDS_PLAN"); plan != "" {
		a.OpenPlan, err = strconv.ParseInt(plan, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
	}
	openedPlan := a.OpenPlan
	a.LazyLog = true
	a.LogCacheOnly = true
	a.HTTP = &http.Client{Transport: needsOffline{}}
	events := make(chan *view.ListProblems, 100)
	envs := make(chan *EnvironmentResult, 10)
	s := NewAPI(a, func(name string, data any) {
		if name == "list-problems" {
			events <- data.(*view.ListProblems)
		}
		if name == "environment" {
			envs <- data.(*EnvironmentResult)
		}
	})
	defer s.Close()
	// Every analysis uses a separate App and only the copied DB. No game writes, scans or subscriptions.
	a.AnalyzeRunner = func(env string, progress func(string)) error {
		r, err := app.Open(home)
		if err != nil {
			return err
		}
		defer r.Close()
		r.EnvName = env
		r.OpenPlan = openedPlan
		r.HTTP = &http.Client{Transport: needsOffline{}}
		report, err := r.Analyze(progress)
		if report != nil {
			fmt.Printf("REAL analysis cachedMods=%d defsFiles=%d patchFiles=%d totalMs=%d\n", report.Stats.CachedMods, report.Stats.DefFiles, report.Stats.PatchFiles, report.Stats.TotalMs)
		}
		return err
	}
	ms := func(start time.Time) float64 { return float64(time.Since(start).Microseconds()) / 1000 }
	start := time.Now()
	b := s.Board()
	fmt.Printf("REAL board %.3f entries=%d mods=%d game=%s\n", ms(start), len(b.St.List), len(b.St.Ix.Mods), b.St.GameMM())
	fmt.Printf("REAL installed-enabled=%d missing=%d env=%s\n", len(b.St.List)-b.Missing, b.Missing, b.EnvName)
	before := app.ListFingerprint(b.St.List)
	start = time.Now()
	s.Workspace(1)
	w := <-envs
	if w.Error != "" {
		t.Fatal(w.Error)
	}
	fmt.Printf("REAL workspace first %.3f\n", ms(start))
	for i := 0; i < 4; i++ {
		start = time.Now()
		n := s.Needs()
		fmt.Printf("REAL needs %d %.3f items=%d count=%d updating=%v stale=%v never=%v\n", i, ms(start), len(n.Items), n.Count, n.Updating, n.Stale, n.Never)
	}
	// Wait for eager background log pass before counting and extracting actions.
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		n := s.Needs()
		if !n.Updating {
			break
		}
		select {
		case <-events:
		case <-time.After(100 * time.Millisecond):
		}
	}
	n := s.Needs()
	ps, err := a.Problems(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw := map[string]int{}
	for _, p := range ps.List {
		if strings.HasPrefix(p.Code, "dep.") {
			raw[strings.TrimSuffix(strings.ToLower(p.Key), "_steam")]++
		}
	}
	for _, it := range n.Items {
		if it.Target != "" && len(it.RequiredBy) > 1 {
			fmt.Printf("REAL merged target=%s raw=%d owners=%d kind=%s\n", it.Target, raw[it.Target], len(it.RequiredBy), it.Actions[0].Kind)
		}
	}
	for i := 0; i < 4; i++ {
		start = time.Now()
		p := s.ListProblems()
		elapsed := ms(start)
		j, _ := json.Marshal(p)
		fmt.Printf("REAL problems %d %.3f mods=%d items=%d bytes=%d\n", i, elapsed, len(p.Mods), len(p.Items), len(j))
	}
	for i := 2; i < 6; i++ {
		s.Leave("needs")
		start = time.Now()
		s.Workspace(uint64(i))
		w = <-envs
		if w.Error != "" {
			t.Fatal(w.Error)
		}
		fmt.Printf("REAL workspace hot %d %.3f\n", i, ms(start))
		s.Leave("plan")
		start = time.Now()
		_ = s.Needs()
		fmt.Printf("REAL switch needs %.3f\n", ms(start))
	}
	// Synchronous updates and reversible data actions. Keep background completion deferred
	// for these samples, then enable the real runner for one final content pass.
	s.mu.Lock()
	s.needsClosed = true
	s.mu.Unlock()
	var samples []view.Action
	for _, it := range n.Items {
		for _, act := range it.Actions {
			if act.Kind == view.ActEnable || act.Kind == view.ActMove {
				samples = append(samples, act)
				break
			}
		}
		if len(samples) >= 2 {
			break
		}
	}
	if len(samples) == 0 {
		fmt.Println("REAL no enable/move repair offered for this list")
	}
	for _, act := range samples {
		start = time.Now()
		_, err = s.DoAction(act, 0)
		if err != nil {
			t.Fatal(err)
		}
		editMs := ms(start)
		start = time.Now()
		p := s.ListProblems()
		fmt.Printf("REAL repair kind=%s edit %.3f read %.3f count=%d updating=%v\n", act.Kind, editMs, ms(start), p.Count, p.Updating)
		st, _ := a.Loadout()
		if app.ListFingerprint(st.List) == before {
			t.Fatal("repair did not change list")
		}
		if act.Kind == view.ActEnable {
			target, _ := st.Ix.ResolveOne(act.Refs[0])
			if st.Position(target.ID) == 0 {
				t.Fatal("dependency not enabled")
			}
			for _, it := range p.Items {
				if it.Target == target.PackageID {
					t.Fatal("dependency card not resolved")
				}
			}
		}
		if _, err = s.Undo(); err != nil {
			t.Fatal(err)
		}
		st, _ = a.Loadout()
		if app.ListFingerprint(st.List) != before {
			t.Fatal("repair undo differs")
		}
	}
	s.mu.Lock()
	s.needsClosed = false
	s.mu.Unlock()
	var id string
	for i := len(b.St.List) - 1; i >= 0; i-- {
		if b.St.List[i].Instance != "" {
			id = b.St.List[i].Instance
			break
		}
	}
	start = time.Now()
	result, err := s.Toggle(int(view.Active), []string{id}, nil)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("REAL toggle %.3f summaryMods=%d count=%d updating=%v\n", ms(start), len(result.Problems.Mods), result.Problems.Count, result.Problems.Updating)
	start = time.Now()
	_ = s.Needs()
	fmt.Printf("REAL needs after-edit %.3f\n", ms(start))
	s.mu.Lock()
	s.needsClosed = false
	c, _ := s.cachedNeedsLocked()
	s.refreshNeedsLocked(c)
	s.mu.Unlock()
	start = time.Now()
	deadline = time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case p := <-events:
			if !p.Updating && p.Error == "" {
				n = s.Needs()
				if !n.Stale && !n.Never && !n.Updating {
					fmt.Printf("REAL final ready %.3f items=%d count=%d\n", ms(start), len(n.Items), n.Count)
					goto ready
				}
			}
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatal("content analysis did not reach fresh result")
ready:
	if _, err = s.Undo(); err != nil {
		t.Fatal(err)
	}
	st, _ := a.Loadout()
	if app.ListFingerprint(st.List) != before {
		t.Fatal("final undo differs")
	}
	s.mu.Lock()
	s.needsClosed = true
	s.mu.Unlock()
}
