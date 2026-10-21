// Spike: measure the sorter's report changes (04 §4.4) on the log corpus, sort only (no
// analysis). Uses a research environment made with `corpusrun setup`; for each 1.6 log the
// mod list becomes ModsConfig.xml, then SortInput + order.Sort, and the load-order findings
// are counted the old way (every violation a warning) and the new way (by strength, DLC moves,
// dropped constraints). Only aggregate numbers are printed.
//
//	go run ./spikes/g2sev -home H -userdata U -corpus <RimArcho-corpus>\hugslogs [-max N]
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/order"
)

type record struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	MM      string `json:"mm"`
	Version string `json:"version"`
	Mods    []struct {
		Pkg string `json:"pkg"`
	} `json:"mods"`
}

func main() {
	home := flag.String("home", "", "research RIMARCHO home (made by corpusrun setup)")
	userData := flag.String("userdata", "", "its user data root")
	corpus := flag.String("corpus", "", "hugslogs folder (parsed.jsonl)")
	maxN := flag.Int("max", 0, "at most this many logs")
	flag.Parse()
	os.Setenv("RIMARCHO_HOME", *home)
	if err := run(*home, *userData, *corpus, *maxN); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

type counts struct {
	logs                                 int
	oldWarnLogs, newWarnLogs, infoLogs   int // logs with ≥ 1 order.violated warning (old / new), with info only
	oldWarn, newWarn, newInfo, harmless  int // findings
	byStrength                           map[string]int
	cycleLogs, droppedLogs, dropped      int
	droppedReason                        map[string]int
	officialLogs, dlcMove, modMove, none int
	moves                                []int
	needsDLC                             int // logs where a mod that needs a DLC loads before it
	cycleWithBefore                      int
}

func run(home, userData, corpus string, maxN int) error {
	a, err := app.Open(home)
	if err != nil {
		return err
	}
	defer a.Close()
	pf, err := os.Open(corpus + `\parsed.jsonl`)
	if err != nil {
		return err
	}
	defer pf.Close()
	c := counts{byStrength: map[string]int{}, droppedReason: map[string]int{}}
	sc := bufio.NewScanner(pf)
	sc.Buffer(make([]byte, 1<<20), 512<<20)
	t0 := time.Now()
	for sc.Scan() {
		var r record
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.Status != "ok" || r.MM != "1.6" {
			continue
		}
		if maxN > 0 && c.logs >= maxN {
			break
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
			fmt.Fprintln(os.Stderr, r.ID[:8], err)
			continue
		}
		in, err := a.SortInput(st, st.List)
		if err != nil {
			fmt.Fprintln(os.Stderr, r.ID[:8], err)
			continue
		}
		c.logs++
		one(&c, in, order.Sort(in))
		if c.logs%500 == 0 {
			fmt.Fprintf(os.Stderr, "%d logs (%s)\n", c.logs, time.Since(t0).Round(time.Second))
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	print(&c)
	return nil
}

// one counts a log's load-order findings as internal/app/problems.go builds them.
func one(c *counts, in order.Input, r order.Result) {
	inCycle := map[int]bool{}
	for _, cy := range r.Cycles {
		for _, m := range cy.Members {
			inCycle[m] = true
		}
	}
	if len(r.Cycles) > 0 {
		c.cycleLogs++
	}
	if len(r.Dropped) > 0 {
		c.droppedLogs++
		c.dropped += len(r.Dropped)
		for _, d := range r.Dropped {
			c.droppedReason[d.Reason]++
		}
	}
	officialBroken := false
	for _, e := range r.Edges {
		if e.C.Kind == order.KindOfficial && e.From > e.To {
			officialBroken = true
		}
	}
	oldW, newW, newI, needs := 0, 0, 0, false
	for _, e := range r.Edges {
		if e.From < e.To || inCycle[e.From] && inCycle[e.To] || e.C.Kind == order.KindOfficial {
			continue
		}
		oldW++ // before: every violation was a warning
		as := order.Assess(in, e)
		if officialBroken && order.IsOfficial(in.Nodes[e.From].PackageID) && !order.IsOfficial(in.Nodes[e.To].PackageID) {
			if as.Strength == order.Strong {
				needs = true
			}
			continue
		}
		c.byStrength[as.Strength.String()]++
		if as.Strength == order.Strong {
			newW++
		} else {
			newI++
			if as.Strength == order.Harmless {
				c.harmless++
			}
		}
	}
	c.oldWarn += oldW
	c.newWarn += newW
	c.newInfo += newI
	if oldW > 0 {
		c.oldWarnLogs++
	}
	if newW > 0 {
		c.newWarnLogs++
	} else if newI > 0 {
		c.infoLogs++
	}
	if needs {
		c.needsDLC++
	}
	if officialBroken {
		c.officialLogs++
		ms := order.OfficialMoves(in, r.Edges, 3)
		switch {
		case len(ms) == 0:
			c.none++
		case order.IsOfficial(in.Nodes[ms[0].Node].PackageID):
			c.dlcMove++
			c.moves = append(c.moves, len(ms))
		default:
			c.modMove++
			c.moves = append(c.moves, len(ms))
		}
	}
}

func pct(a, b int) float64 { return 100 * float64(a) / float64(max(b, 1)) }

func print(c *counts) {
	fmt.Printf("logs %d\n", c.logs)
	fmt.Printf("order.violated findings: before %d (all warning); after %d warning, %d info (%d of them observed harmless)\n", c.oldWarn, c.newWarn, c.newInfo, c.harmless)
	fmt.Printf("  by strength: %v\n", c.byStrength)
	fmt.Printf("logs with an order.violated warning: before %d (%.1f%%), after %d (%.1f%%); info only after %d\n",
		c.oldWarnLogs, pct(c.oldWarnLogs, c.logs), c.newWarnLogs, pct(c.newWarnLogs, c.logs), c.infoLogs)
	fmt.Printf("logs with a cycle %d (%.1f%%)\n", c.cycleLogs, pct(c.cycleLogs, c.logs))
	fmt.Printf("logs with constraints not used %d, constraints %d: %v\n", c.droppedLogs, c.dropped, c.droppedReason)
	sort.Ints(c.moves)
	hist := map[int]int{}
	for _, m := range c.moves {
		hist[m]++
	}
	fmt.Printf("official.order logs %d: move DLCs back %d, move mods down %d, sort only %d; moves %v\n", c.officialLogs, c.dlcMove, c.modMove, c.none, hist)
	fmt.Printf("logs where a mod that needs a DLC loads before it (official.order evidence) %d\n", c.needsDLC)
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
