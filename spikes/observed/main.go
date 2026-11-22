// Spike observed — per-rule load-order judgments from the log corpus (08 §1.3), the generator of
// internal/order/observed.json.
//
// For every shared log with RimArcho attribution (a corpusrun results record) the mod list
// becomes ModsConfig.xml in a research environment made by `corpusrun setup`, and the sorter's
// constraint edges (SortInput) are read in the log's own order. Per rule (kind, from, to): logs
// that break it and logs with both mods that keep it, and in how many of each a problem cluster
// names either mod as an error source. Official edges and rules touching Core/DLC are left out
// (official.order judges those).
//
// Harmless: broken in >= 30 logs, a source share when broken no higher than when kept (or, when
// fewer than 30 logs keep it, broken in >= 100 logs with a share <= 2%). Harmful: broken in >= 20
// logs, a share >= 20%, more than twice the kept share, z >= 3 (two-proportion test).
// Aggregate counts only.
//
//	go run ./spikes/observed -home H -userdata U -parsed parsed.jsonl -results r1.jsonl,r2.jsonl -out observed.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/order"
	"github.com/ToumanLin/RimArcho/spikes/internal/jsonl"
)

type record struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	MM      string `json:"mm"`
	Version string `json:"version"`
	Created string `json:"created"`
	Mods    []struct {
		Pkg string `json:"pkg"`
	} `json:"mods"`
}

type result struct {
	ID       string `json:"id"`
	Clusters []struct {
		Attr []struct {
			Pkg  string `json:"pkg"`
			Role string `json:"role"`
		} `json:"attr"`
	} `json:"clusters"`
}

type rule struct {
	Kind        string `json:"kind"`
	From        string `json:"from"`
	To          string `json:"to"`
	Violated    int    `json:"violated"`
	ViolatedSrc int    `json:"violatedSrc"`
	Kept        int    `json:"kept"`
	KeptSrc     int    `json:"keptSrc"`
	LastLog     string `json:"lastLog,omitempty"` // newest log that broke it, YYYY-MM-DD
}

func main() {
	home := flag.String("home", "", "research RIMARCHO home (made by corpusrun setup)")
	userData := flag.String("userdata", "", "its user data root")
	parsed := flag.String("parsed", "", "hugslogs parsed.jsonl (comma-separated list, .gz allowed)")
	results := flag.String("results", "", "corpusrun results (comma-separated list, .gz allowed)")
	mm := flag.String("mm", "1.6", "game major.minor")
	out := flag.String("out", "", "observed.json to write")
	flag.Parse()
	os.Setenv("RIMARCHO_HOME", *home)
	if err := run(*userData, *parsed, *results, *mm, *out); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(userData, parsedPath, resultsPath, mm, outPath string) error {
	// Error sources per log.
	srcs := map[string]map[string]bool{}
	if err := jsonl.Each(resultsPath, func(b []byte) error {
		var r result
		if json.Unmarshal(b, &r) != nil {
			return nil
		}
		s := map[string]bool{}
		for _, c := range r.Clusters {
			for _, a := range c.Attr {
				if a.Role == "source" {
					s[strings.ToLower(a.Pkg)] = true
				}
			}
		}
		srcs[r.ID] = s
		return nil
	}); err != nil {
		return err
	}
	a, err := app.Open(os.Getenv("RIMARCHO_HOME"))
	if err != nil {
		return err
	}
	defer a.Close()
	rules := map[string]*rule{}
	logs := 0
	first, last := "", ""
	t0 := time.Now()
	err = jsonl.Each(parsedPath, func(b []byte) error {
		var r record
		if json.Unmarshal(b, &r) != nil || r.Status != "ok" || r.MM != mm {
			return nil
		}
		s, ok := srcs[r.ID]
		if !ok {
			return nil
		}
		var pkgs []string
		for _, m := range r.Mods {
			pkgs = append(pkgs, strings.ToLower(m.Pkg))
		}
		if err := writeModsConfig(userData, r.Version, pkgs); err != nil {
			return err
		}
		st, err := a.Loadout()
		if err != nil {
			return nil
		}
		in, err := a.SortInput(st, st.List)
		if err != nil {
			return nil
		}
		logs++
		day := ""
		if len(r.Created) >= 10 {
			day = r.Created[:10]
			if first == "" || day < first {
				first = day
			}
			if day > last {
				last = day
			}
		}
		seen := map[string]bool{}
		for _, e := range order.Sort(in).Edges {
			if e.C.Kind == order.KindOfficial || e.C.Kind == order.KindUser {
				continue
			}
			from, to := strings.ToLower(in.Nodes[e.From].PackageID), strings.ToLower(in.Nodes[e.To].PackageID)
			if order.IsOfficial(from) || order.IsOfficial(to) {
				continue
			}
			k := e.C.Kind + " " + from + " " + to
			if seen[k] {
				continue
			}
			seen[k] = true
			x := rules[k]
			if x == nil {
				x = &rule{Kind: e.C.Kind, From: from, To: to}
				rules[k] = x
			}
			src := s[from] || s[to]
			if e.From > e.To {
				x.Violated++
				if src {
					x.ViolatedSrc++
				}
				if day > x.LastLog {
					x.LastLog = day
				}
			} else {
				x.Kept++
				if src {
					x.KeptSrc++
				}
			}
		}
		if logs%500 == 0 {
			fmt.Fprintf(os.Stderr, "%d logs (%s)\n", logs, time.Since(t0).Round(time.Second))
		}
		return nil
	})
	if err != nil {
		return err
	}
	var harmless, harmful []*rule
	for _, x := range rules {
		switch {
		case isHarmful(x):
			harmful = append(harmful, x)
		case isHarmless(x):
			harmless = append(harmless, x)
		}
	}
	for _, l := range [][]*rule{harmless, harmful} {
		sort.Slice(l, func(i, j int) bool {
			if l[i].Kind != l[j].Kind {
				return l[i].Kind < l[j].Kind
			}
			if l[i].From != l[j].From {
				return l[i].From < l[j].From
			}
			return l[i].To < l[j].To
		})
	}
	fmt.Printf("logs %d (%s..%s), rules %d: harmless %d, harmful %d\n", logs, first, last, len(rules), len(harmless), len(harmful))
	if outPath == "" {
		return nil
	}
	var sb strings.Builder
	src := fmt.Sprintf("RimArcho log corpus: %d shared HugsLib logs of RimWorld %s (%s..%s). Per rule: logs that break it and logs that keep it (both mods enabled), and in how many of each a problem cluster names either mod as an error source. Aggregate counts only.",
		logs, mm, month(first), month(last))
	sb.WriteString("{\n\"source\": ")
	jb, _ := json.Marshal(src)
	sb.Write(jb)
	fmt.Fprintf(&sb, ",\n\"game\": %q,\n\"logs\": %d", mm, logs)
	for _, part := range []struct {
		name string
		l    []*rule
	}{{"harmless", harmless}, {"harmful", harmful}} {
		fmt.Fprintf(&sb, ",\n%q: [", part.name)
		for i, x := range part.l {
			if i > 0 {
				sb.WriteString(",")
			}
			b, _ := json.Marshal(x)
			sb.WriteString("\n")
			sb.Write(b)
		}
		sb.WriteString("\n]")
	}
	sb.WriteString("\n}\n")
	return os.WriteFile(outPath, []byte(sb.String()), 0o644)
}

func month(day string) string {
	if len(day) >= 7 {
		return day[:7]
	}
	return day
}

func share(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func isHarmless(x *rule) bool {
	if x.Kept < 30 {
		return x.Violated >= 100 && share(x.ViolatedSrc, x.Violated) <= 0.02
	}
	return x.Violated >= 30 && share(x.ViolatedSrc, x.Violated) <= share(x.KeptSrc, x.Kept)
}

func isHarmful(x *rule) bool {
	pv, pk := share(x.ViolatedSrc, x.Violated), share(x.KeptSrc, x.Kept)
	if x.Violated < 20 || pv < 0.2 || pv <= 2*pk {
		return false
	}
	return z(x.ViolatedSrc, x.Violated, x.KeptSrc, x.Kept) >= 3
}

// z is the two-proportion z statistic (pooled).
func z(a, n, b, m int) float64 {
	if n == 0 || m == 0 {
		return math.Inf(1)
	}
	p := float64(a+b) / float64(n+m)
	se := math.Sqrt(p * (1 - p) * (1/float64(n) + 1/float64(m)))
	if se == 0 {
		return 0
	}
	return (share(a, n) - share(b, m)) / se
}

func writeModsConfig(userData, version string, active []string) error {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<ModsConfigData>\n  <version>" + version + "</version>\n  <activeMods>\n")
	for _, p := range active {
		b.WriteString("    <li>" + p + "</li>\n")
	}
	b.WriteString("  </activeMods>\n</ModsConfigData>\n")
	return os.WriteFile(env.ModsConfigPath(userData), []byte(b.String()), 0o644)
}
