// Command agenteval (G7) lets an AI agent diagnose real log errors with RimArcho's MCP tools
// only, and scores the answers against the G6 benchmark (spikes/corpuslabel, tasks.jsonl).
//
//	prepare   tasks.jsonl + packets → one task file per task (no answers) in the tasks folder
//	brief     write runs/<run>/AGENT.md: the agent's brief with the server's current MCP
//	          Instructions and tool descriptions (internal/surface/mcp)
//	call      one tool call for one task: rebuild the task's Loadout in its research home (the
//	          player's mod list and Player.log), map the MCP tool + JSON arguments to the CLI
//	          argv exactly as the server does, run it, log it (runs/<run>/calls/<task>.jsonl)
//	warm      RimArcho alone: load every task of a home and record the cluster's attribution
//	          (runs/<run>/rimarcho/<task>.json); also warms the home before agents use it
//	list      task ids of a split / home (for launching agents)
//	score     agent answers (and RimArcho alone) vs gold, per class and difficulty; cost and
//	          leak checks from the agents' transcripts
//
// The gold file and the packets stay outside the tasks/runs folders the agent is told about;
// score flags every transcript that touches them.
package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed agent.md
var agentBrief string

// Config sits next to the executable (agenteval.json) or is given with -config.
type Config struct {
	Corpus string          `json:"corpus"` // hugslogs folder (raw/<gist>.txt)
	Tasks  string          `json:"tasks"`  // task files (no answers)
	Runs   string          `json:"runs"`   // runs/<run>/{AGENT.md,answers,calls,rimarcho}
	Homes  map[string]Home `json:"homes"`  // slot → research home
}

type Home struct {
	Home     string `json:"home"`
	UserData string `json:"userdata"`
}

// Task is what the harness knows about a task; the agent never reads it (it only passes the
// id), and it holds no answer.
type Task struct {
	ID       string   `json:"id"`
	Slot     string   `json:"slot"`
	Gist     string   `json:"gist"`
	Game     string   `json:"game"`
	Key      string   `json:"key"`
	Template string   `json:"template"`
	Class    string   `json:"class"` // RimArcho's cluster class
	Message  string   `json:"message"`
	Extra    []string `json:"extra,omitempty"`
	Count    int      `json:"count"`
	Mods     int      `json:"mods"`
	Uploaded string   `json:"uploaded,omitempty"`
}

func loadConfig(path string) (*Config, error) {
	if path == "" {
		exe, err := os.Executable()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(filepath.Dir(exe), "agenteval.json")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

func readTask(cfg *Config, id string) (*Task, error) {
	if id == "" || strings.ContainsAny(id, `/\.`) {
		return nil, fmt.Errorf("bad task id %q", id)
	}
	b, err := os.ReadFile(filepath.Join(cfg.Tasks, id+".json"))
	if err != nil {
		return nil, fmt.Errorf("unknown task %s", id)
	}
	var t Task
	return &t, json.Unmarshal(b, &t)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: agenteval prepare|brief|call|warm|list|score|prompt [flags]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	cfgPath := fs.String("config", "", "config JSON (default: agenteval.json next to the executable)")
	run := fs.String("run", "", "run name (runs/<run>)")
	task := fs.String("task", "", "call: task id")
	goldF := fs.String("gold", "", "prepare/list/score: G6 tasks.jsonl (holds the answers: never in the tasks folder)")
	packets := fs.String("packets", "", "prepare: G6 packets folder")
	slots := fs.String("slots", "a,b,c,d,e,f,g,h", "prepare: research home slots, round-robin")
	slot := fs.String("slot", "", "warm/list: only this home's tasks")
	split := fs.String("split", "", "list/score: dev or heldout (empty: all)")
	ids := fs.String("ids", "", "warm/list/score: comma-separated task ids")
	items := fs.Bool("items", false, "score: one line per task (refused for heldout)")
	traces := fs.String("traces", "", "score: folder(s) of agent transcripts (subagents/*.jsonl), comma-separated")
	baseline := fs.String("vs", "", "score: another run to compare with")
	fs.Parse(os.Args[2:])
	cfg, err := loadConfig(*cfgPath)
	if err != nil && os.Args[1] != "help" {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	switch os.Args[1] {
	case "prepare":
		err = prepare(cfg, *goldF, *packets, strings.Split(*slots, ","))
	case "brief":
		err = brief(cfg, *run)
	case "call":
		var code int
		code, err = call(cfg, *run, *task, fs.Args())
		if err == nil && code != 0 {
			os.Exit(code)
		}
	case "warm":
		err = warm(cfg, *run, *slot, splitList(*ids))
	case "list":
		var out []string
		out, err = list(cfg, *goldF, *split, *slot, splitList(*ids))
		fmt.Println(strings.Join(out, " "))
	case "prompt":
		err = prompt(cfg, *run, *task)
	case "score":
		err = score(cfg, *run, *baseline, *goldF, *split, splitList(*ids), *items, splitList(*traces))
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func splitList(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func allTasks(cfg *Config) ([]*Task, error) {
	m, err := filepath.Glob(filepath.Join(cfg.Tasks, "*.json"))
	if err != nil {
		return nil, err
	}
	var out []*Task
	for _, p := range m {
		t, err := readTask(cfg, strings.TrimSuffix(filepath.Base(p), ".json"))
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// list prints task ids (only id, split and slot are read from the gold file).
func list(cfg *Config, goldPath, split, slot string, only []string) ([]string, error) {
	gold, err := readGold(goldPath)
	if err != nil {
		return nil, err
	}
	ts, err := allTasks(cfg)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, x := range only {
		want[x] = true
	}
	var out []string
	for _, t := range ts {
		g := gold[t.ID]
		if g == nil || (split != "" && g.Split != split) || (slot != "" && t.Slot != slot) || (len(want) > 0 && !want[t.ID]) {
			continue
		}
		out = append(out, t.ID)
	}
	return out, nil
}
