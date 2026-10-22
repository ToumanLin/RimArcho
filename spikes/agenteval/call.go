package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
)

// CallRec is one logged tool call.
type CallRec struct {
	Time   string   `json:"t"`
	Task   string   `json:"task"`
	Tool   string   `json:"tool"`
	Args   string   `json:"args"`
	Argv   []string `json:"argv,omitempty"`
	MS     int64    `json:"ms"`
	Bytes  int      `json:"bytes"`
	Lines  int      `json:"lines"`
	Exit   int      `json:"exit"`
	Loaded bool     `json:"loaded,omitempty"` // this call switched the home to the task's Loadout
	Err    string   `json:"err,omitempty"`
}

// readUpload splits a HugsLib upload into its header (mod list, Harmony patches) and Player.log.
func readUpload(corpus, gist string) (head, body string, err error) {
	f, err := os.Open(filepath.Join(corpus, "raw", gist+".txt"))
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return "", "", err
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	if h, body, ok := strings.Cut(text, "\nLog file contents:\n"); ok {
		return h, body, nil
	}
	if i := strings.Index(text, "\n=== Waylith log ===\n"); i >= 0 {
		return text[:i], text[i+1:], nil
	}
	return "", "", fmt.Errorf("no log section")
}

var modLineRe = regexp.MustCompile(`\(([^()\s]+)\)((?:\[[a-z]+:[^\]]*\])*): (.*)$`)

// headerMods are the "Loaded mods:" lines (every [mv:]/[v:]/[ov:] entry, in order).
func headerMods(head string) (lines, pkgs []string) {
	_, rest, ok := strings.Cut(head, "Loaded mods:\n")
	if !ok {
		return nil, nil
	}
	for _, l := range strings.Split(rest, "\n") {
		if strings.HasPrefix(l, "Active Harmony patches:") || strings.HasPrefix(l, "Platform information:") {
			break
		}
		if m := modLineRe.FindStringSubmatch(l); m != nil {
			lines = append(lines, l)
			pkgs = append(pkgs, strings.ToLower(m[1]))
		}
	}
	return lines, pkgs
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

// lockHome takes <home>/agenteval.lock (waits up to 20 min; a lock older than 30 min is stale).
func lockHome(home string) (func(), error) {
	p := filepath.Join(home, "agenteval.lock")
	deadline := time.Now().Add(20 * time.Minute)
	for {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintf(f, "%d %s\n", os.Getpid(), time.Now().Format(time.RFC3339))
			f.Close()
			return func() { os.Remove(p) }, nil
		}
		if fi, e := os.Stat(p); e == nil && time.Since(fi.ModTime()) > 30*time.Minute {
			os.Remove(p)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("research home busy (%s)", p)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func openHome(h Home) (*app.App, error) {
	os.Setenv("RIMARCHO_HOME", h.Home)
	a, err := app.Open(h.Home)
	if err != nil {
		return nil, err
	}
	a.AnalyzeRunner = app.InProcessAnalyze(a)
	return a, nil
}

// loadTask makes the task's log the home's Loadout and Player.log, unless the home holds it
// already (marker file), and brings the analysis up to date. It returns the Player.log path.
func loadTask(a *app.App, cfg *Config, h Home, t *Task) (string, bool, error) {
	marker := filepath.Join(h.Home, "agenteval-current.txt")
	logPath := filepath.Join(h.UserData, "Player.log") // 1.6: the user data root
	switched := false
	if cur, _ := os.ReadFile(marker); string(cur) != t.Gist {
		os.Remove(marker)
		head, body, err := readUpload(cfg.Corpus, t.Gist)
		if err != nil {
			return "", false, err
		}
		_, pkgs := headerMods(head)
		if len(pkgs) == 0 {
			return "", false, fmt.Errorf("no mod list in the upload")
		}
		if err := writeModsConfig(h.UserData, t.Game, pkgs); err != nil {
			return "", false, err
		}
		os.Remove(filepath.Join(h.UserData, "Player-prev.log"))
		if err := os.WriteFile(logPath, []byte(body), 0o644); err != nil {
			return "", false, err
		}
		switched = true
	}
	if _, err := a.Loadout(); err != nil {
		return "", false, err
	}
	if _, err := a.EnsureAnalysis(nil); err != nil {
		return "", false, fmt.Errorf("analysis: %w", err)
	}
	if switched {
		os.WriteFile(marker, []byte(t.Gist), 0o644)
	}
	return logPath, switched, nil
}

// taskCluster finds the task's cluster in the analysed log: same key (the stored key may be
// clipped), else the same template with the most occurrences.
func taskCluster(rep *app.LogReport, t *Task) *app.LogCluster {
	key := strings.TrimSuffix(t.Key, "…")
	for _, lc := range rep.Clusters {
		if lc.Key == t.Key || (len(key) >= 200 && strings.HasPrefix(lc.Key, key)) {
			return lc
		}
	}
	var best *app.LogCluster
	for _, lc := range rep.Clusters {
		if lc.Template == t.Template && (best == nil || lc.Count > best.Count) {
			best = lc
		}
	}
	return best
}

func call(cfg *Config, run, id string, args []string) (int, error) {
	dir, err := runDir(cfg, run)
	if err != nil {
		return 0, err
	}
	t, err := readTask(cfg, id)
	if err != nil {
		return 0, err
	}
	if len(args) == 0 {
		return 0, fmt.Errorf("usage: call -run R -task ID <tool> '<json>'")
	}
	tool, raw := args[0], ""
	if len(args) > 1 {
		raw = strings.Join(args[1:], " ")
	}
	rec := &CallRec{Time: time.Now().Format(time.RFC3339), Task: id, Tool: tool, Args: raw}
	start := time.Now()
	out, code := "", 0
	defer func() {
		rec.MS = time.Since(start).Milliseconds()
		rec.Bytes, rec.Lines, rec.Exit = len(out), strings.Count(out, "\n"), code
		os.MkdirAll(filepath.Join(dir, "calls"), 0o755)
		if f, err := os.OpenFile(filepath.Join(dir, "calls", id+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			b, _ := json.Marshal(rec)
			f.Write(append(b, '\n'))
			f.Close()
		}
		if f, err := os.OpenFile(filepath.Join(dir, "calls", id+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			fmt.Fprintf(f, "\n=== %s  %s %s  (%d ms, %d bytes)\n%s", rec.Time, tool, raw, rec.MS, rec.Bytes, out)
			f.Close()
		}
	}()
	fail := func(msg string) (int, error) {
		out, code, rec.Err = "error: "+msg+"\n", cmds.ExitUsage, msg
		fmt.Print(out)
		return code, nil
	}
	if !allowedTool(tool) {
		return fail(fmt.Sprintf("tool %q is not available here (read-only RimArcho tools, read_upload, task)", tool))
	}
	switch tool {
	case "task":
		out = taskText(t)
		fmt.Print(out)
		return 0, nil
	case "read_upload":
		var in struct {
			What, Pattern string
			From, To      int
		}
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &in); err != nil {
				return fail("read_upload: " + err.Error())
			}
		}
		head, body, err := readUpload(cfg.Corpus, t.Gist)
		if err != nil {
			return fail(err.Error())
		}
		out, err = uploadText(head, body, in.What, in.Pattern, in.From, in.To)
		if err != nil {
			return fail(err.Error())
		}
		fmt.Print(out)
		return 0, nil
	}
	if tool == "get_description" && raw != "" { // stay offline: the cache only
		var m map[string]any
		if json.Unmarshal([]byte(raw), &m) == nil {
			m["offline"] = true
			b, _ := json.Marshal(m)
			raw = string(b)
		}
	}
	var m map[string]any
	if strings.TrimSpace(raw) != "" && json.Unmarshal([]byte(raw), &m) != nil {
		return fail("arguments must be one JSON object, e.g. '{\"cluster\": \"task\"}'")
	}
	if tool == "analyze_log" && (m["file"] != nil || m["prev"] != nil) {
		return fail("analyze_log: file and prev are not available here (the task's Player.log is the current log)")
	}
	argv, err := mcp.ArgvFor(tool, []byte(raw))
	if err != nil {
		return fail(err.Error())
	}
	h, ok := cfg.Homes[t.Slot]
	if !ok {
		return 0, fmt.Errorf("no home for slot %q", t.Slot)
	}
	unlock, err := lockHome(h.Home)
	if err != nil {
		return 0, err
	}
	defer unlock()
	a, err := openHome(h)
	if err != nil {
		return 0, err
	}
	defer a.Close()
	logPath, switched, err := loadTask(a, cfg, h, t)
	if err != nil {
		return 0, err
	}
	rec.Loaded = switched
	// "task" as a cluster id is the task's cluster.
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == "--cluster" && strings.TrimPrefix(argv[i+1], "log:") == "task" {
			rep, err := a.AnalyzeLog(logPath)
			if err != nil {
				return 0, err
			}
			lc := taskCluster(rep, t)
			if lc == nil {
				return fail("the task's cluster is not in this log's analysis (run analyze_log without cluster)")
			}
			argv[i+1] = lc.ID
		}
	}
	rec.Argv = argv
	out, code = cmds.Execute(a, argv) // what the MCP server returns for this tool call
	out = mcp.ClipOutput(out)
	fmt.Print(out)
	return code, nil
}

func taskText(t *Task) string {
	var b strings.Builder
	fmt.Fprintf(&b, "task %s: a RimWorld %s player's log (HugsLib upload", t.ID, t.Game)
	if t.Uploaded != "" {
		fmt.Fprintf(&b, " of %s", t.Uploaded)
	}
	fmt.Fprintf(&b, "), %d mods in the list.\n", t.Mods)
	fmt.Fprintf(&b, "The error cluster to diagnose (class %s, %d×):\n\n%s\n", t.Class, t.Count, t.Message)
	for _, x := range t.Extra {
		b.WriteString(x + "\n")
	}
	b.WriteString("\nWhich mod(s) of the player's list cause it (role source / partner / missing), at which stage does it fail, why (cause type), and what should the player do (fix kind and mods)? Say \"unknown\" when the evidence does not decide.\n")
	b.WriteString("analyze_log with {\"cluster\": \"task\"} opens this cluster.\n")
	return b.String()
}

func uploadText(head, body, what, pattern string, from, to int) (string, error) {
	var b strings.Builder
	match := func(l string) bool {
		return pattern == "" || strings.Contains(strings.ToLower(l), strings.ToLower(pattern))
	}
	switch what {
	case "head":
		ls := strings.Split(head, "\n")
		for i, l := range ls {
			if i >= 12 || strings.HasPrefix(l, "Loaded mods:") {
				break
			}
			b.WriteString(l + "\n")
		}
	case "mods":
		lines, _ := headerMods(head)
		n := 0
		for i, l := range lines {
			if match(l) {
				n++
				if n <= 80 {
					fmt.Fprintf(&b, "%d. %s\n", i+1, l)
				}
			}
		}
		fmt.Fprintf(&b, "(%d of %d list entries match)\n", n, len(lines))
	case "harmony":
		_, rest, ok := strings.Cut(head, "Active Harmony patches:\n")
		if !ok {
			return "no \"Active Harmony patches\" section in this upload\n", nil
		}
		rest, _, _ = strings.Cut(rest, "\nPlatform information:")
		if pattern == "" {
			return "", fmt.Errorf("read_upload harmony needs a pattern (a method, type or mod name)")
		}
		n := 0
		for _, l := range strings.Split(rest, "\n") {
			if match(l) {
				n++
				if n <= 60 {
					b.WriteString(strings.TrimRight(l, " ") + "\n")
				}
			}
		}
		fmt.Fprintf(&b, "(%d lines match)\n", n)
	case "grep":
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil || pattern == "" {
			return "", fmt.Errorf("read_upload grep needs a valid regex pattern")
		}
		n := 0
		for i, l := range strings.Split(body, "\n") {
			if re.MatchString(l) {
				n++
				if n <= 60 {
					fmt.Fprintf(&b, "%d: %s\n", i+1, clip(l, 300))
				}
			}
		}
		fmt.Fprintf(&b, "(%d lines match)\n", n)
	case "lines":
		ls := strings.Split(body, "\n")
		if from < 1 {
			from = 1
		}
		if to < from || to-from >= 200 {
			to = from + 199
		}
		for i := from; i <= to && i <= len(ls); i++ {
			fmt.Fprintf(&b, "%d: %s\n", i, clip(ls[i-1], 400))
		}
	default:
		return "", fmt.Errorf("read_upload: what must be head, mods, harmony, grep or lines")
	}
	return b.String(), nil
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…(" + strconv.Itoa(len(s)) + " chars)"
}
