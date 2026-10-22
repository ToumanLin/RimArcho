package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/logs"
)

// report prints the G1 (attribution), G2 (ordering), G3 (Harmony recall) and G4 (calibration) metrics of results.jsonl
// (docs/research/00 §3). Every number is reported for all logs and for the logs whose mod list
// the research library covers at least minCov (attribution needs the mods' files).
func report(path string, minCov float64) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var all []*Result
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 256<<20)
	for sc.Scan() {
		var r Result
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			all = append(all, &r)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	var covered []*Result
	var cov []float64
	errs := map[string]int{}
	for _, r := range all {
		c := coverage(r)
		cov = append(cov, c)
		if c >= minCov {
			covered = append(covered, r)
		}
		if r.Err != "" {
			errs[strings.SplitN(r.Err, ":", 2)[0]]++
		}
	}
	sort.Float64s(cov)
	fmt.Printf("== logs %d; errors %v\n", len(all), errs)
	fmt.Printf("library coverage per log: p10 %.2f p25 %.2f median %.2f p75 %.2f p90 %.2f; >= %.2f: %d logs\n",
		q(cov, .1), q(cov, .25), q(cov, .5), q(cov, .75), q(cov, .9), minCov, len(covered))

	for _, set := range []struct {
		name string
		rs   []*Result
	}{{"all logs", all}, {fmt.Sprintf("covered logs (>= %.0f%%)", minCov*100), covered}} {
		fmt.Printf("\n== G1 attribution, %s (%d)\n", set.name, len(set.rs))
		g1(set.rs)
	}
	fmt.Printf("\n== G2 ordering, all logs (%d)\n", len(all))
	g2(all)
	fmt.Printf("\n== G2 ordering, covered logs (%d)\n", len(covered))
	g2(covered)
	g3report(all)
	g4report(all)
	return nil
}

func coverage(r *Result) float64 {
	if r.Mods == 0 {
		return 0
	}
	return float64(r.Resolved) / float64(r.Mods)
}

func q(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	return v[min(len(v)-1, int(p*float64(len(v))))]
}

type tally struct{ n, attr, source int }

func (t tally) String() string {
	return fmt.Sprintf("%6d clusters, attributed %5.1f%%, with a source %5.1f%%", t.n, pct(t.attr, t.n), pct(t.source, t.n))
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}

func g1(rs []*Result) {
	var total tally
	byClass := map[string]*tally{}
	byTpl := map[string]*tally{}
	tplLogs := map[string]map[string]bool{}
	conf := map[string]int{}
	evKind := map[string]int{}
	officialSource, officialSourceXref := 0, 0
	for _, r := range rs {
		for _, c := range r.Clusters {
			if logs.Informational(c.Class) {
				continue
			}
			hasSrc := false
			for _, a := range c.Attr {
				if a.Role == "source" {
					hasSrc = true
					if strings.HasPrefix(a.Pkg, "ludeon.rimworld") {
						officialSource++
						if c.Class == logs.ClassXrefMissing {
							officialSourceXref++
						}
					}
				}
				for _, e := range a.Evidence {
					evKind[a.Role+" "+evidenceKind(e)]++
				}
			}
			add := func(t *tally) {
				t.n++
				if len(c.Attr) > 0 {
					t.attr++
				}
				if hasSrc {
					t.source++
				}
			}
			add(&total)
			if byClass[c.Class] == nil {
				byClass[c.Class] = &tally{}
			}
			add(byClass[c.Class])
			if byTpl[c.Tpl] == nil {
				byTpl[c.Tpl] = &tally{}
				tplLogs[c.Tpl] = map[string]bool{}
			}
			add(byTpl[c.Tpl])
			tplLogs[c.Tpl][r.ID] = true
			conf[c.Confidence]++
		}
	}
	fmt.Printf("problem clusters: %s\n", total)
	for _, k := range logs.Classes {
		if t := byClass[k]; t != nil {
			fmt.Printf("  %-16s %s\n", k, t)
		}
	}
	fmt.Printf("confidence: %v\n", conf)
	fmt.Printf("source attributed to Core/DLC (ludeon.*): %d clusters, %d of them xref-missing\n", officialSource, officialSourceXref)
	fmt.Printf("evidence by role and kind: %s\n", fmtTop(evKind, 20))
	type kv struct {
		k    string
		logs int
		t    *tally
	}
	var ts []kv
	for k, t := range byTpl {
		ts = append(ts, kv{k, len(tplLogs[k]), t})
	}
	sort.Slice(ts, func(a, b int) bool { return ts[a].logs > ts[b].logs || ts[a].logs == ts[b].logs && ts[a].k < ts[b].k })
	fmt.Println("top 30 templates by logs, with their attribution (where attribution fails):")
	for i := 0; i < len(ts) && i < 30; i++ {
		fmt.Printf("  %4d logs %s  %s\n", ts[i].logs, ts[i].t, trunc(ts[i].k, 100))
	}
}

// evidenceKind reduces an evidence string to its rule ("log frame", "patch", "references" …).
func evidenceKind(e string) string {
	for _, p := range []string{"log message prefix", "log file", "log frame", "log harmony", "log message names type",
		"patch file", "patch ", "defines ", "references ", "the patch targets"} {
		if strings.HasPrefix(e, p) {
			return strings.TrimSpace(p)
		}
	}
	if i := strings.IndexByte(e, ' '); i > 0 {
		return e[:i]
	}
	return e
}

func g2(rs []*Result) {
	if len(rs) == 0 {
		return
	}
	kinds := map[string]bool{}
	withViol := map[string]int{}
	viol := map[string]int{}
	edges := map[string]int{}
	cycles, anyViol := 0, 0
	var moved []float64
	pairs := map[string]int{}
	pairSrc := map[string]int{} // logs where a problem cluster names either side as source
	// Co-occurrence with errors, stratified by list size (bigger lists have more errors anyway).
	type cell struct{ logs, withErr int }
	strata := map[string]map[bool]*cell{}
	for _, r := range rs {
		if r.Sort == nil {
			continue
		}
		s := r.Sort
		for k, v := range s.Edges {
			edges[k] += v
			kinds[k] = true
		}
		for k, v := range s.Violations {
			viol[k] += v
			withViol[k]++
		}
		if s.Cycles > 0 {
			cycles++
		}
		moved = append(moved, float64(s.Moved))
		hard := s.Violations["dependency"]+s.Violations["loadAfter"]+s.Violations["loadBefore"]+
			s.Violations["forceLoadAfter"]+s.Violations["forceLoadBefore"] > 0
		if hard {
			anyViol++
		}
		src := map[string]bool{}
		hasErr := false
		for _, c := range r.Clusters {
			if c.Class == logs.ClassPatchFailed || c.Class == logs.ClassXrefMissing || c.Class == logs.ClassXMLLoad {
				hasErr = true
			}
			if logs.Informational(c.Class) {
				continue
			}
			for _, a := range c.Attr {
				if a.Role == "source" {
					src[a.Pkg] = true
				}
			}
		}
		b := sizeBucket(r.Mods)
		if strata[b] == nil {
			strata[b] = map[bool]*cell{false: {}, true: {}}
		}
		strata[b][hard].logs++
		if hasErr {
			strata[b][hard].withErr++
		}
		for _, v := range s.Violated {
			if v.Kind == "official" {
				continue
			}
			k := v.Kind + " " + v.From + " < " + v.To
			pairs[k]++
			if src[v.From] || src[v.To] {
				pairSrc[k]++
			}
		}
	}
	sort.Float64s(moved)
	fmt.Printf("constraint edges by kind: %s\n", fmtTop(edges, 0))
	fmt.Printf("violated edges by kind: %s\n", fmtTop(viol, 0))
	var ks []string
	for k := range kinds {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		fmt.Printf("  logs violating %-16s %5d (%.1f%%)\n", k, withViol[k], pct(withViol[k], len(rs)))
	}
	fmt.Printf("logs violating a declared constraint (dependency / load rules): %d (%.1f%%); with a cycle: %d\n", anyViol, pct(anyViol, len(rs)), cycles)
	fmt.Printf("mods the sorter moves per log: p25 %.0f median %.0f p75 %.0f p90 %.0f max %.0f\n", q(moved, .25), q(moved, .5), q(moved, .75), q(moved, .9), q(moved, 1))
	fmt.Println("logs with patch-failed / xref-missing / xml-load clusters, by list size and declared-constraint violation:")
	for _, b := range []string{"<100", "100-199", "200-399", ">=400"} {
		if c := strata[b]; c != nil {
			fmt.Printf("  %-8s violating %4d logs, %5.1f%% with errors | not violating %4d logs, %5.1f%% with errors\n", b,
				c[true].logs, pct(c[true].withErr, c[true].logs), c[false].logs, pct(c[false].withErr, c[false].logs))
		}
	}
	type kv struct {
		k    string
		n, s int
	}
	var ps []kv
	for k, n := range pairs {
		ps = append(ps, kv{k, n, pairSrc[k]})
	}
	sort.Slice(ps, func(a, b int) bool { return ps[a].n > ps[b].n || ps[a].n == ps[b].n && ps[a].k < ps[b].k })
	fmt.Println("most frequent violated constraints (logs; logs where a problem cluster names either mod as source):")
	for i := 0; i < len(ps) && i < 25; i++ {
		fmt.Printf("  %4d %4d  %s\n", ps[i].n, ps[i].s, ps[i].k)
	}
}

func sizeBucket(n int) string {
	switch {
	case n < 100:
		return "<100"
	case n < 200:
		return "100-199"
	case n < 400:
		return "200-399"
	}
	return ">=400"
}

func fmtTop(m map[string]int, limit int) string {
	type kv struct {
		k string
		v int
	}
	var s []kv
	for k, v := range m {
		s = append(s, kv{k, v})
	}
	sort.Slice(s, func(a, b int) bool { return s[a].v > s[b].v || s[a].v == s[b].v && s[a].k < s[b].k })
	var out []string
	for i, x := range s {
		if limit > 0 && i == limit {
			break
		}
		out = append(out, fmt.Sprintf("%s %d", x.k, x.v))
	}
	return strings.Join(out, ", ")
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
