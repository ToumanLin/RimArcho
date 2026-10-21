// Spike: run RimArcho's own log attribution and sorter over the HugsLib log corpus
// (docs/research/00 §5 L2, §6.5 "virtual environment"). One research environment in its own
// RIMARCHO home holds the research mod library; each log's mod list becomes the Loadout
// (ModsConfig.xml), then the product code runs unchanged: analysis (patch simulation),
// AnalyzeLogText on the log body (G1), SortInput + order.Sort on the log's order (G2).
//
//	go run ./spikes/corpusrun setup -home H -game D:\RimWorld16 -userdata U -extra <kb>\mods -extra …
//	go run ./spikes/corpusrun run -home H -corpus <RimArcho-corpus>\hugslogs -out results.jsonl [-max N]
//
// Only aggregate numbers leave the machine (design §8); results.jsonl stays with the corpus.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/order"
)

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: corpusrun setup|run [flags]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	home := fs.String("home", "", "RimArcho data directory for the research environment (not the user's)")
	game := fs.String("game", `D:\RimWorld16`, "setup: game install (the version the logs were written by)")
	userData := fs.String("userdata", "", "setup: user data root; run writes Config/ModsConfig.xml here")
	workshop := fs.String("workshop", "", "setup: a workshop content/294100 folder")
	rules := fs.String("rules", "", "setup: communityRules.json to install into <home>/cache")
	var extra multi
	fs.Var(&extra, "extra", "setup: extra mod root (repeatable)")
	corpus := fs.String("corpus", "", "run: the hugslogs folder of RimArcho-corpus (parsed.jsonl, raw/)")
	out := fs.String("out", "", "run: results JSONL (appended; logs already in it are skipped)")
	mm := fs.String("mm", "1.6", "run: game major.minor of the logs to process")
	cpuprof := fs.String("cpuprofile", "", "write a CPU profile to this file")
	maxN := fs.Int("max", 0, "run: process at most this many logs (0 = all)")
	minCov := fs.Float64("mincov", 0.8, "report: coverage threshold for the 'covered' subset")
	fs.Parse(os.Args[2:])
	if os.Args[1] == "report" {
		if err := report(*out, *minCov); err != nil {
			fail(err)
		}
		return
	}
	if *home == "" {
		fail(fmt.Errorf("-home is required"))
	}
	if *cpuprof != "" {
		pf, err := os.Create(*cpuprof)
		if err != nil {
			fail(err)
		}
		pprof.StartCPUProfile(pf)
		defer pprof.StopCPUProfile()
	}
	if os.Getenv("RIMARCHO_HOME") == "" {
		os.Setenv("RIMARCHO_HOME", *home) // the analysis child process, if any, uses the same home
	}
	var err error
	switch os.Args[1] {
	case "setup":
		err = setup(*home, *game, *userData, *workshop, *rules, extra)
	case "run":
		err = run(*home, *corpus, *out, *mm, *maxN)
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func open(home string) (*app.App, error) {
	a, err := app.Open(home)
	if err != nil {
		return nil, err
	}
	a.AnalyzeRunner = app.InProcessAnalyze(a)
	return a, nil
}

func setup(home, game, userData, workshop, rules string, extra []string) error {
	if userData == "" {
		return fmt.Errorf("-userdata is required")
	}
	if err := os.MkdirAll(filepath.Join(userData, "Config"), 0o755); err != nil {
		return err
	}
	// An empty list to start with; run replaces it per log.
	if err := writeModsConfig(userData, "1.6", []string{"ludeon.rimworld"}); err != nil {
		return err
	}
	if rules != "" {
		b, err := os.ReadFile(rules)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(home, "cache"), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(home, "cache", "communityRules.json"), b, 0o644); err != nil {
			return err
		}
	}
	a, err := open(home)
	if err != nil {
		return err
	}
	defer a.Close()
	e, changes, err := a.EnvAdd("corpus", app.EnvSpec{GameRoot: game, UserDataRoot: userData, WorkshopRoot: workshop, Launch: "direct", Extra: extra})
	if err != nil {
		return err
	}
	fmt.Printf("environment %s: game %s (%s), %d source changes\n", e.Name, e.GameRoot, e.GameVersion, len(changes))
	t := time.Now()
	rep, err := a.Scan(false)
	if err != nil {
		return err
	}
	fmt.Printf("scan: %+v in %s\n", rep.Stats, time.Since(t).Round(time.Second))
	return nil
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

// record is the part of spikes/hugslogs parsed.jsonl this spike reads.
type record struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	MM      string `json:"mm"`
	Version string `json:"version"`
	Mods    []struct {
		Pkg string `json:"pkg"`
	} `json:"mods"`
	Harmony []HPatch `json:"harmony"`
}

// Result is one log's line in results.jsonl.
type Result struct {
	ID         string     `json:"id"`
	Mods       int        `json:"mods"`
	Resolved   int        `json:"resolved"` // entries the research library has (design §5 L2 coverage)
	Missing    []string   `json:"missing,omitempty"`
	AnalysisMS int64      `json:"analysis_ms"`
	AnalysisOK bool       `json:"analysis_ok"`
	Clusters   []RCluster `json:"clusters"`
	Sort       *RSort     `json:"sort,omitempty"`
	Harmony    *RHarmony  `json:"harmony,omitempty"`  // G3; absent in results written before it
	Findings   []RFinding `json:"findings,omitempty"` // G4; absent in results written before it
	Err        string     `json:"err,omitempty"`
}

type RCluster struct {
	Key        string   `json:"key"`
	Tpl        string   `json:"tpl"`
	Class      string   `json:"class"`
	Count      int      `json:"count"`
	Confidence string   `json:"conf"`
	Attr       []RAttr  `json:"attr,omitempty"`
	Details    []string `json:"details,omitempty"`
}

type RAttr struct {
	Pkg      string   `json:"pkg"`
	Role     string   `json:"role"`
	Evidence []string `json:"ev"`
}

type RSort struct {
	Nodes      int            `json:"nodes"`
	Edges      map[string]int `json:"edges"`      // constraint edges by kind
	Violations map[string]int `json:"violations"` // edges the log's own order breaks, by kind
	Violated   []RViolation   `json:"violated,omitempty"`
	Cycles     int            `json:"cycles"`
	Moved      int            `json:"moved"` // mods the sorter moves: nodes minus the longest kept subsequence
}

type RViolation struct {
	Kind, From, To string
}

func run(home, corpus, out, mm string, maxN int) error {
	done := map[string]bool{}
	if f, err := os.Open(out); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 256<<20)
		for sc.Scan() {
			var r struct{ ID string }
			if json.Unmarshal(sc.Bytes(), &r) == nil {
				done[r.ID] = true
			}
		}
		f.Close()
	}
	a, err := open(home)
	if err != nil {
		return err
	}
	defer a.Close()
	st, err := a.Loadout()
	if err != nil {
		return err
	}
	userData := st.Ix.Env.UserDataRoot

	pf, err := os.Open(filepath.Join(corpus, "parsed.jsonl"))
	if err != nil {
		return err
	}
	defer pf.Close()
	of, err := os.OpenFile(out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer of.Close()
	sc := bufio.NewScanner(pf)
	sc.Buffer(make([]byte, 1<<20), 512<<20)
	n := 0
	t0 := time.Now()
	for sc.Scan() {
		var r record
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.Status != "ok" || r.MM != mm || done[r.ID] {
			continue
		}
		if maxN > 0 && n >= maxN {
			break
		}
		n++
		res := one(a, userData, corpus, r)
		j, _ := json.Marshal(res)
		of.Write(append(j, '\n'))
		fmt.Printf("%4d %s mods %d resolved %d clusters %d analysis %dms %s (%s elapsed)\n", n, r.ID[:8], res.Mods, res.Resolved,
			len(res.Clusters), res.AnalysisMS, res.Err, time.Since(t0).Round(time.Second))
	}
	return sc.Err()
}

func one(a *app.App, userData, corpus string, r record) *Result {
	res := &Result{ID: r.ID, Mods: len(r.Mods)}
	var pkgs []string
	for _, m := range r.Mods {
		pkgs = append(pkgs, strings.ToLower(m.Pkg))
	}
	if err := writeModsConfig(userData, r.Version, pkgs); err != nil {
		res.Err = err.Error()
		return res
	}
	st, err := a.Loadout()
	if err != nil {
		res.Err = err.Error()
		return res
	}
	for _, e := range st.List {
		if e.Instance != "" {
			res.Resolved++
		} else {
			res.Missing = append(res.Missing, e.Missing)
		}
	}
	t := time.Now()
	as, err := a.EnsureAnalysis(nil)
	res.AnalysisMS = time.Since(t).Milliseconds()
	res.AnalysisOK = err == nil
	if err != nil {
		res.Err = "analysis: " + err.Error()
	}
	body, err := logBody(filepath.Join(corpus, "raw", r.ID+".txt"))
	if err != nil {
		res.Err = err.Error()
		return res
	}
	rep, err := a.AnalyzeLogText(body)
	if err != nil {
		res.Err = "log: " + err.Error()
		return res
	}
	pkgOf := func(inst string) string {
		if m := st.Ix.ByID[inst]; m != nil {
			return strings.ToLower(m.PackageID)
		}
		return inst
	}
	for _, lc := range rep.Clusters {
		c := RCluster{Key: lc.Key, Tpl: lc.Template, Class: lc.Class, Count: lc.Count, Confidence: lc.Confidence, Details: lc.Details}
		for _, at := range lc.Attr {
			c.Attr = append(c.Attr, RAttr{Pkg: pkgOf(at.Instance), Role: at.Role, Evidence: at.Evidence})
		}
		res.Clusters = append(res.Clusters, c)
	}
	if res.AnalysisOK {
		res.Harmony = g3(a, st, r.Harmony)
		g4(a, st, as, rep, res)
	}
	in, err := a.SortInput(st, st.List)
	if err != nil {
		res.Err = "sort: " + err.Error()
		return res
	}
	sr := order.Sort(in)
	s := &RSort{Nodes: len(in.Nodes), Edges: map[string]int{}, Violations: map[string]int{}, Cycles: len(sr.Cycles)}
	name := func(i int) string {
		if in.Nodes[i].PackageID != "" {
			return strings.ToLower(in.Nodes[i].PackageID)
		}
		return in.Nodes[i].Instance
	}
	for _, e := range sr.Edges {
		s.Edges[e.C.Kind]++
		if e.From > e.To { // the log's order loads To before From
			s.Violations[e.C.Kind]++
			s.Violated = append(s.Violated, RViolation{e.C.Kind, name(e.From), name(e.To)})
		}
	}
	s.Moved = len(sr.Order) - lis(sr.Order)
	res.Sort = s
	return res
}

// logBody is the Player.log part of a HugsLib upload.
func logBody(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	if _, body, ok := strings.Cut(text, "\nLog file contents:\n"); ok {
		return body, nil
	}
	if i := strings.Index(text, "\n=== Waylith log ===\n"); i >= 0 {
		return text[i+1:], nil
	}
	return "", fmt.Errorf("no log section")
}

// lis is the length of the longest increasing subsequence: the mods the sorter keeps in place.
func lis(a []int) int {
	var tails []int
	for _, x := range a {
		i := sort.SearchInts(tails, x)
		if i == len(tails) {
			tails = append(tails, x)
		} else {
			tails[i] = x
		}
	}
	return len(tails)
}
