package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
)

// GoldTask is one line of G6's tasks.jsonl (with the answer: harness and scorer only).
type GoldTask struct {
	ID         string `json:"id"`
	Tier       int    `json:"tier"`
	Class      string `json:"class"`
	RAClass    string `json:"rimarcho_class"`
	Stratum    string `json:"stratum"`
	Split      string `json:"split"`
	Gist       string `json:"gist"`
	Key        string `json:"cluster_key"`
	Template   string `json:"template"`
	Packet     string `json:"packet"`
	Answer     *Label `json:"answer"`
	Status     string `json:"answer_status"`
	Difficulty string `json:"difficulty"`
}

func readGold(path string) (map[string]*GoldTask, error) {
	if path == "" {
		return nil, fmt.Errorf("-gold is required")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]*GoldTask{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var g GoldTask
		if err := json.Unmarshal(sc.Bytes(), &g); err != nil {
			return nil, err
		}
		out[g.ID] = &g
	}
	return out, sc.Err()
}

// packetJSON is the part of a G6 packet .json the harness uses (it also holds RimArcho's
// verdict, which is never copied).
type packetJSON struct {
	Gist    string `json:"gist"`
	Key     string `json:"key"`
	Tpl     string `json:"tpl"`
	Count   int    `json:"count"`
	Game    string `json:"game"`
	Mods    int    `json:"mods"`
	Created string `json:"created"`
	Text    string `json:"text"`
}

// section returns the lines of a "## <name>" section of a packet text.
func section(text, name string) []string {
	var out []string
	in := false
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, "## ") {
			in = strings.HasPrefix(l, "## "+name)
			continue
		}
		if in && strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func prepare(cfg *Config, goldPath, packetDir string, slots []string) error {
	gold, err := readGold(goldPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.Tasks, 0o755); err != nil {
		return err
	}
	var ids []string
	for id := range gold {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for i, id := range ids {
		g := gold[id]
		b, err := os.ReadFile(filepath.Join(packetDir, id+".json"))
		if err != nil {
			return err
		}
		var p packetJSON
		if err := json.Unmarshal(b, &p); err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		t := Task{ID: id, Slot: slots[i%len(slots)], Gist: p.Gist, Game: p.Game, Key: p.Key, Template: p.Tpl, Class: g.RAClass,
			Message: strings.Join(section(p.Text, "Message"), "\n"), Extra: section(p.Text, "Extra lines"), Count: p.Count, Mods: p.Mods, Uploaded: p.Created}
		if t.Key == "" {
			t.Key = g.Key
		}
		out, _ := json.MarshalIndent(t, "", "  ")
		if err := os.WriteFile(filepath.Join(cfg.Tasks, id+".json"), out, 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("%d tasks written to %s\n", len(ids), cfg.Tasks)
	return nil
}

func runDir(cfg *Config, run string) (string, error) {
	if run == "" || strings.ContainsAny(run, `/\.`) {
		return "", fmt.Errorf("bad -run %q", run)
	}
	return filepath.Join(cfg.Runs, run), nil
}

// allowedTool: the server's read-only tools (the write tools would change the research home).
func allowedTool(name string) bool {
	for _, d := range mcp.Catalog() {
		if d.Name == name {
			return d.ReadOnly
		}
	}
	return name == "read_upload" || name == "task"
}

// brief writes runs/<run>/AGENT.md from agent.md, the server's Instructions and the catalog.
func brief(cfg *Config, run string) error {
	dir, err := runDir(cfg, run)
	if err != nil {
		return err
	}
	for _, sub := range []string{"answers", "calls", "rimarcho"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return err
		}
	}
	exe, _ := os.Executable()
	var tb strings.Builder
	for _, d := range mcp.Catalog() {
		if !d.ReadOnly {
			continue
		}
		fmt.Fprintf(&tb, "### %s\n\n%s\n\nParameters:", d.Name, d.Description)
		if len(d.Params) == 0 {
			tb.WriteString(" none")
		}
		tb.WriteString("\n")
		for _, p := range d.Params {
			req := ""
			if p.Required {
				req = ", required"
			}
			help := ""
			if p.Help != "" {
				help = ": " + p.Help
			}
			fmt.Fprintf(&tb, "- `%s` (%s%s)%s\n", p.Name, p.Type, req, help)
		}
		tb.WriteString("\n")
	}
	text := strings.NewReplacer("{{EXE}}", filepath.ToSlash(exe), "{{RUN}}", run, "{{ANSWERS}}", filepath.Join(dir, "answers"),
		"{{INSTRUCTIONS}}", mcp.Instructions, "{{TOOLS}}", strings.TrimSpace(tb.String())).Replace(agentBrief)
	p := filepath.Join(dir, "AGENT.md")
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		return err
	}
	fmt.Printf("%s (%d bytes)\n", p, len(text))
	return nil
}

// prompt prints the Agent prompt for one task (the same text for every task but the id).
func prompt(cfg *Config, run, id string) error {
	dir, err := runDir(cfg, run)
	if err != nil {
		return err
	}
	exe, _ := os.Executable()
	fmt.Print(promptText(filepath.ToSlash(exe), run, id, filepath.Join(dir, "AGENT.md")))
	return nil
}

func promptText(exe, run, id, briefPath string) string {
	return fmt.Sprintf(`Diagnose RimWorld log error task %[2]s (run %[1]s) with RimArcho only.
1. Read the brief %[4]s (Read tool) and follow it exactly.
2. Start with: %[3]s call -run %[1]s -task %[2]s task '{}'
3. Use only that command for facts (no other files, folders or tools), write the answer file the brief describes, and reply with one line.
`, run, id, exe, briefPath)
}
