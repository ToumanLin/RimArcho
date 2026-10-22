package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Label is a corpuslabel/2 answer (spikes/corpuslabel/SCHEMA.md), the fields scored here.
type Label struct {
	Schema     string   `json:"schema"`
	Item       string   `json:"item"`
	Verdict    string   `json:"verdict"`
	Causes     []Cause  `json:"causes"`
	Stage      string   `json:"stage"`
	CauseType  string   `json:"cause_type"`
	Fix        Fix      `json:"fix"`
	Confidence string   `json:"confidence"`
	Difficulty string   `json:"difficulty"`
	Evidence   []string `json:"evidence"`
	Note       string   `json:"note"`
}

type Cause struct {
	Pkg  string `json:"pkg"`
	Role string `json:"role"`
}

type Fix struct {
	Kind string   `json:"kind"`
	Mods []string `json:"mods"`
	Note string   `json:"note"`
}

func sources(l *Label) map[string]bool {
	out := map[string]bool{}
	if l == nil || l.Verdict != "cause" {
		return out
	}
	for _, c := range l.Causes {
		if c.Role == "source" {
			out[strings.ToLower(c.Pkg)] = true
		}
	}
	return out
}

func meets(a, b map[string]bool) bool {
	for k := range a {
		if b[k] {
			return true
		}
	}
	return false
}

func pct(n, d int) string {
	if d == 0 {
		return "    - "
	}
	return fmt.Sprintf("%5.1f%%", 100*float64(n)/float64(d))
}

func keysOf(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Trace is what an agent transcript (Claude Code subagent .jsonl) shows about one task.
type Trace struct {
	File       string
	Seconds    float64
	ToolCalls  int
	ArchoCalls int
	Context    int      // input + cache tokens of the last request, plus its output: the final context size
	Output     int      // output tokens over the run
	Processed  int      // input + cache tokens summed over requests (what was billed as input)
	Leaks      []string // touched the gold, the packets or the harness config
	OffTool    []string // other tool use than the brief allows
	Pipes      int      // archo calls post-filtered with a pipe (grep/head …)
}

// leakWords are path parts of the gold, the packets and the harness config. Answer text is not
// checked (evidence may quote cache paths under the research homes).
var leakWords = []string{`labels/`, `tasks.jsonl`, `/review`, `rimarcho-v4`, `/packets`, `agenteval.json`, `execlogs`, `hugslogs`, `corpus-kb/labels`, `agenteval/tasks`}

// readTraces maps "<run> <task>" (the Agent description "g7 <run> <task>") to its trace.
func readTraces(dirs []string) (map[string]*Trace, error) {
	out := map[string]*Trace{}
	for _, dir := range dirs {
		metas, err := filepath.Glob(filepath.Join(dir, "agent-*.meta.json"))
		if err != nil {
			return nil, err
		}
		for _, mp := range metas {
			b, err := os.ReadFile(mp)
			if err != nil {
				continue
			}
			var meta struct {
				Description string `json:"description"`
			}
			json.Unmarshal(b, &meta)
			f := strings.Fields(meta.Description)
			if len(f) != 3 || f[0] != "g7" {
				continue
			}
			tr, err := parseTrace(strings.TrimSuffix(mp, ".meta.json")+".jsonl", f[1], f[2])
			if err != nil {
				return nil, err
			}
			k := f[1] + " " + f[2]
			if old := out[k]; old != nil { // a retried task: keep both costs, both leak lists
				tr.Seconds += old.Seconds
				tr.ToolCalls += old.ToolCalls
				tr.ArchoCalls += old.ArchoCalls
				tr.Output += old.Output
				tr.Processed += old.Processed
				tr.Leaks = append(tr.Leaks, old.Leaks...)
				tr.OffTool = append(tr.OffTool, old.OffTool...)
			}
			out[k] = tr
		}
	}
	return out, nil
}

func parseTrace(path, run, task string) (*Trace, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tr := &Trace{File: path}
	var first, last time.Time
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<28)
	for sc.Scan() {
		var d struct {
			Type      string `json:"type"`
			Timestamp string `json:"timestamp"`
			Message   struct {
				Usage struct {
					In     int `json:"input_tokens"`
					CacheW int `json:"cache_creation_input_tokens"`
					CacheR int `json:"cache_read_input_tokens"`
					Out    int `json:"output_tokens"`
				} `json:"usage"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &d) != nil {
			continue
		}
		if ts, err := time.Parse(time.RFC3339Nano, d.Timestamp); err == nil {
			if first.IsZero() {
				first = ts
			}
			last = ts
		}
		if d.Type != "assistant" {
			continue
		}
		u := d.Message.Usage
		tr.Output += u.Out
		tr.Processed += u.In + u.CacheW + u.CacheR
		if n := u.In + u.CacheW + u.CacheR + u.Out; n > 0 {
			tr.Context = n
		}
		var blocks []struct {
			Type  string          `json:"type"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		}
		if json.Unmarshal(d.Message.Content, &blocks) != nil {
			continue
		}
		for _, bl := range blocks {
			if bl.Type != "tool_use" {
				continue
			}
			tr.ToolCalls++
			in := string(bl.Input)
			var x struct {
				Command  string `json:"command"`
				FilePath string `json:"file_path"`
				Path     string `json:"path"`
				Pattern  string `json:"pattern"`
			}
			json.Unmarshal(bl.Input, &x)
			cmd := x.Command
			if i := strings.Index(cmd, "<<"); i >= 0 && strings.Contains(cmd, "answers") { // heredoc writing the answer
				cmd = cmd[:i]
			}
			probe := strings.ToLower(strings.ReplaceAll(strings.Join([]string{cmd, x.FilePath, x.Path, x.Pattern}, " "), `\`, "/"))
			for _, w := range leakWords {
				if strings.Contains(probe, w) {
					tr.Leaks = append(tr.Leaks, bl.Name+": "+clip(in, 160))
					break
				}
			}
			switch bl.Name {
			case "Bash", "PowerShell":
				switch {
				case strings.Contains(x.Command, "agenteval") && strings.Contains(x.Command, " call "):
					tr.ArchoCalls += strings.Count(x.Command, " call -run")
					if strings.Contains(x.Command, "|") {
						tr.Pipes++
					}
				case strings.Contains(x.Command, "answers") && strings.Contains(x.Command, task+".json"): // writes the answer
				default:
					tr.OffTool = append(tr.OffTool, bl.Name+": "+clip(x.Command, 160))
				}
			case "Read":
				// the brief, or the output file of a call the Bash tool moved to the background
				if p := filepath.ToSlash(x.FilePath); !strings.HasSuffix(p, run+"/AGENT.md") && !strings.HasSuffix(p, ".output") {
					tr.OffTool = append(tr.OffTool, "Read: "+x.FilePath)
				}
			case "Write", "Edit":
				if !strings.HasSuffix(filepath.ToSlash(x.FilePath), "answers/"+task+".json") {
					tr.OffTool = append(tr.OffTool, bl.Name+": "+x.FilePath)
				}
			case "Monitor", "ToolSearch", "TaskOutput", "BashOutput": // waiting for a slow call
			default:
				tr.OffTool = append(tr.OffTool, bl.Name)
			}
		}
	}
	if !first.IsZero() {
		tr.Seconds = last.Sub(first).Seconds()
	}
	return tr, sc.Err()
}

func readJSON[T any](path string) (*T, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

type row struct{ n, gold, hit, wrong, silent, over, quiet, stage, ctype, fix, verdict, missing int }

func (x *row) add(g, a *Label, named map[string]bool) {
	x.n++
	gs := sources(g)
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
	if a == nil {
		x.missing++
		return
	}
	if a.Stage == g.Stage {
		x.stage++
	}
	if a.CauseType == g.CauseType {
		x.ctype++
	}
	if a.Fix.Kind == g.Fix.Kind {
		x.fix++
	}
	if (a.Verdict == "cause") == (g.Verdict == "cause") {
		x.verdict++
	}
}

func printTable(title string, groups []string, by map[string]*row, tot *row, withFields bool) {
	fmt.Printf("\n== %s\n", title)
	fmt.Print("  group             n gold-src  hit wrong silent over  both-none   agree  precision  recall  undecidable-ok")
	if withFields {
		fmt.Print("   stage  cause-type  fix-kind")
	}
	fmt.Println()
	line := func(name string, x *row) {
		fmt.Printf("  %-15s %3d %6d %6d %5d %6d %4d %8d    %s   %s   %s     %s", name, x.n, x.gold, x.hit, x.wrong, x.silent, x.over, x.quiet,
			pct(x.hit+x.quiet, x.n), pct(x.hit, x.hit+x.wrong+x.over), pct(x.hit, x.gold), pct(x.quiet, x.n-x.gold))
		if withFields {
			fmt.Printf("    %s     %s    %s", pct(x.stage, x.n), pct(x.ctype, x.n), pct(x.fix, x.n))
		}
		if x.missing > 0 {
			fmt.Printf("   (%d without answer)", x.missing)
		}
		fmt.Println()
	}
	for _, g := range groups {
		if x := by[g]; x != nil {
			line(g, x)
		}
	}
	line("all", tot)
}

var classOrder = []string{"patch-failed", "xref-missing", "texture-missing", "xml-load", "exception", "harmony-stack", "assembly", "static-ctor"}

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

func score(cfg *Config, run, _ string, goldPath, split string, only []string, items bool, traceDirs []string) error {
	dir, err := runDir(cfg, run)
	if err != nil {
		return err
	}
	if items && split != "dev" {
		return fmt.Errorf("-items only with -split dev (held-out tasks are scored in aggregate)")
	}
	gold, err := readGold(goldPath)
	if err != nil {
		return err
	}
	traces, err := readTraces(traceDirs)
	if err != nil {
		return err
	}
	want := map[string]bool{}
	for _, x := range only {
		want[x] = true
	}
	var ids []string
	for id, g := range gold {
		if (split == "" || g.Split == split) && (len(want) == 0 || want[id]) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	agentBy, raBy := map[string]*row{}, map[string]*row{}
	agentTot, raTot := &row{}, &row{}
	get := func(m map[string]*row, k string) *row {
		if m[k] == nil {
			m[k] = &row{}
		}
		return m[k]
	}
	var secs, calls, archo, ctx, outTok, proc []float64
	var invalid, offTool []string
	var pipes, answered int
	var lines []string
	for _, id := range ids {
		g := gold[id]
		tr := traces[run+" "+id]
		if tr != nil && len(tr.Leaks) > 0 {
			invalid = append(invalid, id)
			continue
		}
		a, _ := readJSON[Label](filepath.Join(dir, "answers", id+".json"))
		if a != nil {
			answered++
		}
		named := sources(a)
		for _, k := range []string{g.Class, g.Difficulty, fmt.Sprintf("tier %d", g.Tier)} {
			get(agentBy, k).add(g.Answer, a, named)
		}
		agentTot.add(g.Answer, a, named)
		ra, _ := readJSON[RimArcho](filepath.Join(dir, "rimarcho", id+".json"))
		raNamed := map[string]bool{}
		if ra != nil {
			for _, at := range ra.Attr {
				if at.Role == "source" && at.Pkg != "" {
					raNamed[strings.ToLower(at.Pkg)] = true
				}
			}
		}
		for _, k := range []string{g.Class, g.Difficulty} {
			get(raBy, k).add(g.Answer, nil, raNamed)
		}
		raTot.add(g.Answer, nil, raNamed)
		if tr != nil {
			secs = append(secs, tr.Seconds)
			calls = append(calls, float64(tr.ToolCalls))
			archo = append(archo, float64(tr.ArchoCalls))
			ctx = append(ctx, float64(tr.Context))
			outTok = append(outTok, float64(tr.Output))
			proc = append(proc, float64(tr.Processed))
			pipes += tr.Pipes
			if len(tr.OffTool) > 0 {
				offTool = append(offTool, id)
			}
		}
		if items {
			mark := func(n map[string]bool) string {
				gs := sources(g.Answer)
				switch {
				case len(gs) > 0 && meets(n, gs):
					return "HIT"
				case len(gs) > 0 && len(n) > 0:
					return "WRONG"
				case len(gs) > 0:
					return "silent"
				case len(n) > 0:
					return "OVER"
				}
				return "ok-none"
			}
			al := "(no answer)"
			if a != nil {
				al = fmt.Sprintf("%s %v %s/%s/%s", a.Verdict, keysOf(named), a.Stage, a.CauseType, a.Fix.Kind)
			}
			cost := ""
			if tr != nil {
				cost = fmt.Sprintf("  %.0fs %d calls %dk", tr.Seconds, tr.ArchoCalls, tr.Context/1000)
				if len(tr.OffTool) > 0 {
					cost += fmt.Sprintf(" OFFTOOL%v", tr.OffTool)
				}
			}
			lines = append(lines, fmt.Sprintf("  %s %-15s %s %-7s | gold %s %v %s/%s/%s | agent %s %s | rimarcho %s %v%s",
				id, g.Class, g.Difficulty, mark(named), g.Answer.Verdict, keysOf(sources(g.Answer)), g.Answer.Stage, g.Answer.CauseType, g.Answer.Fix.Kind,
				mark(named), al, mark(raNamed), keysOf(raNamed), cost))
		}
	}
	label := split
	if label == "" {
		label = "all"
	}
	fmt.Printf("run %s, split %s: %d tasks, %d answered, %d invalid (touched gold/packets/config: %v), %d with other tool use %v\n",
		run, label, len(ids), answered, len(invalid), invalid, len(offTool), offTool)
	diffs := []string{"L1", "L2", "L3"}
	printTable("agent vs gold, by class", classOrder, agentBy, agentTot, true)
	printTable("agent vs gold, by difficulty and tier", append(diffs, "tier 1", "tier 2"), agentBy, agentTot, true)
	printTable("RimArcho alone (same build, same homes) vs gold, by class", classOrder, raBy, raTot, false)
	printTable("RimArcho alone, by difficulty", diffs, raBy, raTot, false)
	if len(secs) > 0 {
		sum := func(xs []float64) float64 {
			t := 0.0
			for _, x := range xs {
				t += x
			}
			return t
		}
		fmt.Printf("\n== cost per task (%d transcripts)\n", len(secs))
		fmt.Printf("  median: %.0f s, %.0f tool calls, %.0f archo calls, final context %.0fk tokens, output %.1fk tokens, input processed %.0fk tokens\n",
			median(secs), median(calls), median(archo), median(ctx)/1000, median(outTok)/1000, median(proc)/1000)
		fmt.Printf("  total:  %.0f min agent time, %.0f archo calls, %.1f M input tokens processed, %.2f M output tokens; %d archo calls piped through grep/head\n",
			sum(secs)/60, sum(archo), sum(proc)/1e6, sum(outTok)/1e6, pipes)
	}
	toolStats(dir, ids, invalid)
	if items {
		fmt.Println("\n== per task")
		for _, l := range lines {
			fmt.Println(l)
		}
	}
	return nil
}

// toolStats summarises the harness call logs: calls per tool, output size, errors, and the
// share of tasks that used each tool.
func toolStats(dir string, ids, invalid []string) {
	skip := map[string]bool{}
	for _, x := range invalid {
		skip[x] = true
	}
	type st struct {
		n, errs, tasks int
		bytes          []float64
		max            int
	}
	by := map[string]*st{}
	total, nt := 0, 0
	for _, id := range ids {
		if skip[id] {
			continue
		}
		f, err := os.Open(filepath.Join(dir, "calls", id+".jsonl"))
		if err != nil {
			continue
		}
		nt++
		used := map[string]bool{}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var r CallRec
			if json.Unmarshal(sc.Bytes(), &r) != nil {
				continue
			}
			k := r.Tool
			if k == "read_source" && strings.Contains(r.Args, `"search"`) {
				k = "read_source search"
			}
			if by[k] == nil {
				by[k] = &st{}
			}
			s := by[k]
			s.n++
			total++
			s.bytes = append(s.bytes, float64(r.Bytes))
			if r.Bytes > s.max {
				s.max = r.Bytes
			}
			if r.Exit != 0 || r.Err != "" {
				s.errs++
			}
			if !used[k] {
				used[k] = true
				s.tasks++
			}
		}
		f.Close()
	}
	if total == 0 {
		return
	}
	var names []string
	for k := range by {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool { return by[names[i]].n > by[names[j]].n })
	fmt.Printf("\n== tool calls (%d calls in %d tasks)\n  tool                 calls  tasks  errors  median bytes  max bytes\n", total, nt)
	for _, k := range names {
		s := by[k]
		fmt.Printf("  %-20s %5d  %5d  %6d  %12.0f  %9d\n", k, s.n, s.tasks, s.errs, median(s.bytes), s.max)
	}
}
