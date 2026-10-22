package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Cost is one labeller run (tier 2: what the Agent tool reported).
type Cost struct {
	Item     string  `json:"item"`
	Model    string  `json:"model"`
	Seconds  float64 `json:"seconds"`
	ToolUses int     `json:"tool_uses"`
	Tokens   int     `json:"tokens"`
}

func readCosts(path string) (map[string]*Cost, error) {
	out := map[string]*Cost{}
	if path == "" {
		return out, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var c Cost
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			return nil, err
		}
		out[c.Item] = &c
	}
	return out, sc.Err()
}

// gold is the reviewed label when there is one, else the labeller's (status "labeller").
func gold(name string, lab, rev map[string]*Label) (*Label, string) {
	if r := rev[name]; r != nil {
		return r, "reviewed"
	}
	if l := lab[name]; l != nil {
		return l, "labeller"
	}
	return nil, ""
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64{}, xs...)
	sort.Float64s(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// benchScore: labeller vs reviewer per tier, RimArcho (final run v4 and the packet re-run)
// vs gold per benchmark class, the gold label distribution and the tier-2 cost.
func benchScore(samplePath, packetDir, labelDir, reviewDir, raPath, execLogs, costPath string, o *vopts) error {
	items, ps, err := loadAll(samplePath, packetDir)
	if err != nil {
		return err
	}
	lab, _, err := readLabels(labelDir)
	if err != nil {
		return err
	}
	rev := map[string]*Label{}
	if reviewDir != "" {
		if rev, _, err = readLabels(reviewDir); err != nil {
			return err
		}
	}
	ra, err := readRimArchoV4(raPath)
	if err != nil {
		return err
	}
	costs, err := readCosts(costPath)
	if err != nil {
		return err
	}
	var names []string
	for n := range items {
		names = append(names, n)
	}
	sort.Strings(names)

	// --- labeller vs reviewer
	fmt.Println("== Labeller vs reviewer (reviewed items)")
	for _, tier := range []int{1, 2} {
		var n, invalid, src, verdict, fix, stage, ctype, diff int
		var dis []string
		for _, name := range names {
			it := items[name]
			l, r := lab[name], rev[name]
			if it.Tier != tier || r == nil {
				continue
			}
			n++
			if l == nil {
				invalid++
				dis = append(dis, name+" no labeller label")
				continue
			}
			if len(validate(l, items, ps, o)) > 0 {
				invalid++
			}
			ok := agree(sources(l), sources(r))
			if ok {
				src++
			}
			if (l.Verdict == "cause") == (r.Verdict == "cause") {
				verdict++
			}
			if l.Fix.Kind == r.Fix.Kind {
				fix++
			}
			if l.Stage == r.Stage {
				stage++
			}
			if l.CauseType == r.CauseType {
				ctype++
			}
			if l.Difficulty == r.Difficulty {
				diff++
			}
			if !ok {
				dis = append(dis, fmt.Sprintf("%s %-15s labeller %-7s %v | reviewer %-7s %v", name, it.Bench, l.Verdict, keysOf(sources(l)), r.Verdict, keysOf(sources(r))))
			}
		}
		if n == 0 {
			continue
		}
		fmt.Printf("  tier %d: %d reviewed, labeller labels failing validation %d\n", tier, n, invalid)
		fmt.Printf("    agree on source mods %s (disagreement %s); cause/no cause %s, cause type %s, fix kind %s, stage %s, difficulty %s\n",
			pct(src, n), pct(n-src, n), pct(verdict, n), pct(ctype, n), pct(fix, n), pct(stage, n), pct(diff, n))
		for _, d := range dis {
			fmt.Println("    " + d)
		}
	}

	// --- RimArcho vs gold
	type row struct{ n, gold, hit, wrong, silent, over, quiet, named, namedOK int }
	table := func(title string, only string, attrOf func(name string) []RAttr) {
		by := map[string]*row{}
		tot := &row{}
		for _, name := range names {
			it := items[name]
			g, status := gold(name, lab, rev)
			if g == nil || (only != "" && status != only) {
				continue
			}
			if by[it.Bench] == nil {
				by[it.Bench] = &row{}
			}
			named := map[string]bool{}
			for _, at := range attrOf(name) {
				if at.Role == "source" && at.Pkg != "" {
					named[strings.ToLower(at.Pkg)] = true
				}
			}
			gs, ga := sources(g), allCauses(g)
			for _, x := range []*row{by[it.Bench], tot} {
				x.n++
				switch {
				case len(gs) > 0 && meets(named, gs):
					x.hit++
				case len(gs) > 0 && len(named) > 0:
					x.wrong++
				case len(gs) > 0:
					x.silent++
				case len(named) > 0:
					x.over++
				default:
					x.quiet++
				}
				if len(gs) > 0 {
					x.gold++
				}
				for pk := range named {
					x.named++
					if ga[pk] {
						x.namedOK++
					}
				}
			}
		}
		fmt.Printf("\n== RimArcho %s\n", title)
		fmt.Println("  class             n  gold-src  hit wrong silent overclaim both-none  agree  precision  recall  mod-precision")
		line := func(c string, x *row) {
			fmt.Printf("  %-15s %3d %6d %7d %5d %6d %8d %9d  %s  %s   %s   %s (%d/%d)\n", c, x.n, x.gold, x.hit, x.wrong, x.silent, x.over, x.quiet,
				pct(x.hit+x.quiet, x.n), pct(x.hit, x.hit+x.wrong+x.over), pct(x.hit, x.gold), pct(x.namedOK, x.named), x.namedOK, x.named)
		}
		for _, bc := range benchClasses {
			if x := by[bc.Name]; x != nil {
				line(bc.Name, x)
			}
		}
		line("all", tot)
	}
	v4 := func(name string) []RAttr {
		if r := ra[name]; r != nil {
			return r.Attr
		}
		return nil
	}
	rerun := func(name string) []RAttr {
		if p := ps[name]; p != nil {
			return p.RimArcho.Attr
		}
		return nil
	}
	table("final run v4 vs gold (reviewed + labeller labels)", "", v4)
	table("final run v4 vs reviewed labels only", "reviewed", v4)
	table("packet re-run (fresh home) vs gold", "", rerun)
	fmt.Println("\n  per item (v4):")
	for _, name := range names {
		g, status := gold(name, lab, rev)
		if g == nil {
			continue
		}
		var named []string
		for _, at := range v4(name) {
			if at.Role == "source" {
				named = append(named, at.Pkg)
			}
		}
		sort.Strings(named)
		fmt.Printf("    %s %-15s %-8s gold %-7s %v | rimarcho %v\n", name, items[name].Bench, status, g.Verdict, keysOf(sources(g)), named)
	}

	// --- gold distribution
	fmt.Println("\n== Gold labels by class: cause type / fix kind / difficulty")
	for _, bc := range benchClasses {
		ct, fk, df := map[string]int{}, map[string]int{}, map[string]int{}
		n := 0
		for _, name := range names {
			if items[name].Bench != bc.Name {
				continue
			}
			g, _ := gold(name, lab, rev)
			if g == nil {
				continue
			}
			n++
			ct[g.CauseType]++
			fk[g.Fix.Kind]++
			df[g.Difficulty]++
		}
		if n > 0 {
			fmt.Printf("  %-15s %3d  %s | %s | %s\n", bc.Name, n, counts(ct), counts(fk), counts(df))
		}
	}

	// --- tier-2 cost
	fmt.Println("\n== Tier-2 cost per cluster (Agent tool usage; exec calls from the transcripts)")
	for _, bc := range benchClasses {
		if bc.Tier != 2 {
			continue
		}
		var secs, tools, toks, execs []float64
		for _, name := range names {
			if items[name].Bench != bc.Name {
				continue
			}
			if c := costs[name]; c != nil {
				secs = append(secs, c.Seconds)
				tools = append(tools, float64(c.ToolUses))
				toks = append(toks, float64(c.Tokens))
			}
			if execLogs != "" {
				b, err := os.ReadFile(filepath.Join(execLogs, name+".log"))
				if err == nil {
					execs = append(execs, float64(strings.Count(string(b), "\n=== ")))
				}
			}
		}
		fmt.Printf("  %-15s runs %2d  median %4.0f s (mean %4.0f)  tool calls median %3.0f (mean %4.1f)  archo exec median %3.0f  tokens median %6.0f\n",
			bc.Name, len(secs), median(secs), mean(secs), median(tools), mean(tools), median(execs), median(toks))
	}
	return nil
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

func counts(m map[string]int) string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool {
		if m[ks[i]] != m[ks[j]] {
			return m[ks[i]] > m[ks[j]]
		}
		return ks[i] < ks[j]
	})
	var out []string
	for _, k := range ks {
		out = append(out, fmt.Sprintf("%s %d", k, m[k]))
	}
	return strings.Join(out, ", ")
}

// Task is one agent-evaluation task (G7): rebuild the log's Loadout, diagnose the cluster,
// compare with Answer. Stored outside the repository (labels\v2\tasks.jsonl).
type Task struct {
	ID         string `json:"id"`
	Tier       int    `json:"tier"`
	Class      string `json:"class"`          // benchmark class
	RClass     string `json:"rimarcho_class"` // RimArcho's cluster class
	Stratum    string `json:"stratum"`
	Split      string `json:"split"` // dev | heldout (G7 must not tune on heldout)
	Gist       string `json:"gist"`
	ClusterKey string `json:"cluster_key"`
	Template   string `json:"template"`
	Loadout    struct {
		Source  string `json:"source"`
		Mods    int    `json:"mods"`
		InLib   int    `json:"in_library"`
		Game    string `json:"game"`
		Rebuild string `json:"rebuild"`
	} `json:"loadout"`
	Packet       string `json:"packet"`
	Question     string `json:"question"`
	Answer       *Label `json:"answer"`
	AnswerStatus string `json:"answer_status"` // reviewed | labeller
	Difficulty   string `json:"difficulty"`
}

const taskQuestion = "This RimWorld 1.6 player's log (HugsLib upload) has the error cluster below. Which mod(s) of the player's list cause it (role source / partner / missing), at which stage does it fail, why (cause type), and what should the player do (fix kind and mods)? Answer as a corpuslabel/2 label (SCHEMA.md); say \"unknown\" when the evidence does not decide."

// exportTasks writes tasks.jsonl: every item with a valid gold label; ~30% per benchmark
// class held out (deterministic by hash). Code-quote files are copied under out/code/<item>/
// and the answer points at the copies.
func exportTasks(samplePath, packetDir, labelDir, reviewDir, out string, o *vopts) error {
	items, ps, err := loadAll(samplePath, packetDir)
	if err != nil {
		return err
	}
	lab, _, err := readLabels(labelDir)
	if err != nil {
		return err
	}
	rev := map[string]*Label{}
	if reviewDir != "" {
		if rev, _, err = readLabels(reviewDir); err != nil {
			return err
		}
	}
	byClass := map[string][]string{}
	for n, it := range items {
		byClass[it.Bench] = append(byClass[it.Bench], n)
	}
	held := map[string]bool{}
	for _, ns := range byClass {
		sort.Slice(ns, func(i, j int) bool { return pickHash("heldout-v2", ns[i], "") < pickHash("heldout-v2", ns[j], "") })
		for i := 0; i < (len(ns)*3+5)/10; i++ {
			held[ns[i]] = true
		}
	}
	var names []string
	for n := range items {
		names = append(names, n)
	}
	sort.Strings(names)
	var tasks []Task
	skipped := 0
	for _, n := range names {
		it, p := items[n], ps[n]
		g, status := gold(n, lab, rev)
		if g == nil || p == nil || len(validate(g, items, ps, o)) > 0 {
			skipped++
			continue
		}
		ans := *g
		ans.Code = nil
		for i, c := range g.Code {
			dst := filepath.Join(out, "code", n, fmt.Sprintf("%d-%s", i+1, filepath.Base(c.File)))
			if b, err := os.ReadFile(c.File); err == nil {
				os.MkdirAll(filepath.Dir(dst), 0o755)
				os.WriteFile(dst, b, 0o644)
				c.File = filepath.ToSlash(filepath.Join("code", n, filepath.Base(dst)))
			}
			ans.Code = append(ans.Code, c)
		}
		t := Task{ID: n, Tier: it.Tier, Class: it.Bench, RClass: it.Class, Stratum: it.Stratum, Gist: it.Gist, ClusterKey: p.Key, Template: it.Tpl,
			Packet: "packets/" + n + ".txt", Question: taskQuestion, Answer: &ans, AnswerStatus: status, Difficulty: g.Difficulty, Split: "dev"}
		if held[n] {
			t.Split = "heldout"
		}
		t.Loadout.Source = "RimArcho-corpus/hugslogs/raw/" + it.Gist + ".txt, \"Loaded mods:\" header (every [mv:]/[v:]/[ov:] entry, in order)"
		t.Loadout.Mods, t.Loadout.InLib, t.Loadout.Game = p.Mods, p.Resolved, p.Game
		t.Loadout.Rebuild = "corpuslabel exec -home <research home> -corpus <hugslogs> -packet packets/" + n + ".txt -log <transcript> -- <archo command>"
		tasks = append(tasks, t)
	}
	if err := writeJSONL(filepath.Join(out, "tasks.jsonl"), tasks); err != nil {
		return err
	}
	nh := 0
	by := map[string][2]int{}
	for _, t := range tasks {
		x := by[t.Class]
		x[0]++
		if t.Split == "heldout" {
			x[1]++
			nh++
		}
		by[t.Class] = x
	}
	for _, bc := range benchClasses {
		fmt.Printf("  %-15s %3d tasks, %d held out\n", bc.Name, by[bc.Name][0], by[bc.Name][1])
	}
	fmt.Printf("%d tasks (%d held out), %d items without a valid gold label\n", len(tasks), nh, skipped)
	return nil
}
