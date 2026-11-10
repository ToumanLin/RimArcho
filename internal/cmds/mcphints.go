package cmds

// MCP tool results suggest MCP calls, not CLI commands (13 §1.1): an agent reading
// `next: archo source fi7x --type Ns.T` has to translate it into read_source before using it
// (MCP review 2026-11-04 P11; h8 G8). The CLI keeps its commands. Only the hint text differs:
// MCPHints rewrites the archo commands in a result, and leaves any command it does not know
// exactly as it is.

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/render"
)

// hintSpec translates one CLI command into the MCP tool that does the same.
type hintSpec struct {
	cmd  string // the command words, "source search"
	tool string // "" marks a command with no MCP tool: it is kept as written
	// pos names the parameters the positional arguments fill, in order; the last may be
	// a list (posList) taking all the rest.
	pos     []string
	posList bool
	fixed   [][2]string       // parameters the command implies: {"action", "enable"}
	value   map[string]string // --flag <value> → parameter
	list    map[string]string // --flag <value> (repeatable) → list parameter
	flag    map[string]string // --flag → parameter=true
	ignore  map[string]bool   // --flag with no MCP counterpart (it is the tool's default)
	// min: list_problems shows warning and above unless asked; the CLI shows everything.
	minInfo bool
}

var hintSpecs = []hintSpec{
	{cmd: "log", tool: "analyze_log", value: map[string]string{"cluster": "cluster", "file": "file", "since": "since", "game-version": "game_version"},
		flag: map[string]string{"prev": "prev", "all": "all"}},
	{cmd: "source search", tool: "read_source", pos: []string{"search"},
		list: map[string]string{"in": "in"}, value: map[string]string{"budget": "budget"},
		flag:   map[string]string{"game": "game", "regex": "regex", "all": "all"},
		ignore: map[string]bool{"implicated": true, "yes": true}},
	{cmd: "source game", tool: "read_source", fixed: [][2]string{{"game", "true"}}, value: map[string]string{"type": "type"}, flag: map[string]string{"all": "all"}},
	{cmd: "source clean"},
	{cmd: "source", tool: "read_source", pos: []string{"ref"}, value: map[string]string{"type": "type"}, flag: map[string]string{"all": "all"}},
	{cmd: "harmony", tool: "get_harmony", value: map[string]string{"mod": "mod", "target": "target", "game-version": "game_version"},
		flag: map[string]string{"shared": "shared", "all": "all"}},
	{cmd: "patches", tool: "get_patches", value: map[string]string{"mod": "mod", "target": "target", "game-version": "game_version"},
		flag: map[string]string{"failed": "failed", "all": "all"}},
	{cmd: "def", tool: "get_def", pos: []string{"def"}, value: map[string]string{"mod": "mod", "game-version": "game_version"}, flag: map[string]string{"all": "all"}},
	{cmd: "mod", tool: "get_mod", pos: []string{"ref"}},
	{cmd: "why", tool: "explain_relation", pos: []string{"a", "b"}, value: map[string]string{"game-version": "game_version"}},
	{cmd: "files search", tool: "search_files", pos: []string{"pattern"},
		value: map[string]string{"mod": "mod", "kind": "kind", "game": "game", "mode": "mode", "page": "page", "limit": "limit", "per-file": "per_file", "game-version": "game_version"},
		flag:  map[string]string{"def": "def", "regex": "regex", "word": "word", "case": "case_sensitive", "languages": "languages", "all-versions": "all_versions", "all": "all"}},
	{cmd: "problems show", tool: "list_problems", pos: []string{"ref"}},
	{cmd: "problems ignore", tool: "ignore_problem", pos: []string{"ref"}},
	{cmd: "problems", tool: "list_problems", minInfo: true,
		value: map[string]string{"subject": "subject", "code": "code", "severity": "severity", "min-severity": "min_severity", "ref": "ref", "game-version": "game_version"},
		flag:  map[string]string{"all": "all", "detail": "detail", "ignored": "ignored", "include-muted": "include_muted"}},
	{cmd: "changes", tool: "list_changes", pos: []string{"ref"}, value: map[string]string{"since": "since"}, flag: map[string]string{"library": "library", "all": "all"}},
	{cmd: "enable", tool: "edit_loadout", fixed: [][2]string{{"action", "enable"}}, posList: true, pos: []string{"refs"},
		value: map[string]string{"at": "at", "before": "before", "after": "after"}},
	{cmd: "disable", tool: "edit_loadout", fixed: [][2]string{{"action", "disable"}}, posList: true, pos: []string{"refs"}},
	{cmd: "undo", tool: "edit_loadout", fixed: [][2]string{{"action", "undo"}}},
	{cmd: "redo", tool: "edit_loadout", fixed: [][2]string{{"action", "redo"}}},
	{cmd: "sort", tool: "edit_loadout", fixed: [][2]string{{"action", "sort"}}, flag: map[string]string{"dry-run": "dry_run"}},
	{cmd: "scan", tool: "scan", flag: map[string]string{"full": "full"}},
	{cmd: "save", tool: "save_loadout", flag: map[string]string{"dry-run": "dry_run"}},
	{cmd: "pattern", tool: "analyze_log", pos: []string{"pattern"}},
	{cmd: "workshop subscribe", tool: "manage_library", fixed: [][2]string{{"action", "subscribe"}}, posList: true, pos: []string{"refs"}, flag: map[string]string{"dry-run": "dry_run"}},
	{cmd: "workshop unsubscribe", tool: "manage_library", fixed: [][2]string{{"action", "unsubscribe"}}, posList: true, pos: []string{"refs"}, flag: map[string]string{"dry-run": "dry_run"}},
	{cmd: "workshop resubscribe", tool: "manage_library", fixed: [][2]string{{"action", "resubscribe"}}, posList: true, pos: []string{"refs"}, flag: map[string]string{"dry-run": "dry_run"}},
	{cmd: "workshop state", tool: "manage_library", fixed: [][2]string{{"action", "steam_state"}}, posList: true, pos: []string{"refs"}},
	{cmd: "pin", tool: "pin_mod", pos: []string{"ref"}, flag: map[string]string{"dry-run": "dry_run", "update": "update", "git": "git"}},
	{cmd: "unpin", tool: "unpin_mod", pos: []string{"ref"}, flag: map[string]string{"dry-run": "dry_run"}},
}

// MCPHints rewrites the archo commands in an MCP result as tool calls: "next: archo log
// --cluster E9" becomes "next: analyze_log cluster=E9", "(archo log --all)" becomes
// "(analyze_log all=true)". A command that has no tool, or uses a flag or placeholder the
// translation does not cover, stays as written.
func MCPHints(text string) string {
	spans := render.CommandSpans(text)
	if len(spans) == 0 {
		return text
	}
	var sb strings.Builder
	last := 0
	for _, sp := range spans {
		sb.WriteString(text[last:sp[0]])
		if call, ok := MCPCall(text[sp[0]:sp[1]]); ok {
			sb.WriteString(call)
		} else {
			sb.WriteString(text[sp[0]:sp[1]])
		}
		last = sp[1]
	}
	sb.WriteString(text[last:])
	return sb.String()
}

// MCPCall translates one printed command ("archo source search \"X\" --in fi7x") into the MCP
// call that does the same ("read_source search=X in=[fi7x]"). ok is false when it cannot.
func MCPCall(cmdline string) (string, bool) {
	toks, ok := hintTokens(cmdline)
	if !ok || len(toks) < 2 || toks[0] != "archo" {
		return "", false
	}
	toks = toks[1:]
	var sp *hintSpec
	for i := range hintSpecs {
		words := strings.Fields(hintSpecs[i].cmd)
		if len(toks) < len(words) || (sp != nil && len(strings.Fields(sp.cmd)) >= len(words)) {
			continue
		}
		match := true
		for j, w := range words {
			match = match && toks[j] == w
		}
		if match {
			sp = &hintSpecs[i]
		}
	}
	if sp == nil || sp.tool == "" {
		return "", false
	}
	toks = toks[len(strings.Fields(sp.cmd)):]
	type kv struct{ k, v string }
	var out []kv
	lists := map[string][]string{}
	var listOrder []string
	setList := func(k, v string) {
		if _, ok := lists[k]; !ok {
			listOrder = append(listOrder, k)
		}
		lists[k] = append(lists[k], v)
	}
	for _, f := range sp.fixed {
		out = append(out, kv{f[0], f[1]})
	}
	var pos []string
	set := map[string]bool{}
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		name, isFlag := strings.CutPrefix(t, "--")
		if !isFlag {
			pos = append(pos, t)
			continue
		}
		switch {
		case sp.ignore[name]:
		case name == "active" && sp.cmd == "source search":
			out = append(out, kv{"scope", "active"})
		case sp.flag[name] != "":
			out = append(out, kv{sp.flag[name], "true"})
			set[sp.flag[name]] = true
		case sp.value[name] != "":
			if i+1 >= len(toks) {
				return "", false
			}
			i++
			out = append(out, kv{sp.value[name], toks[i]})
			set[sp.value[name]] = true
		case sp.list[name] != "":
			if i+1 >= len(toks) {
				return "", false
			}
			i++
			setList(sp.list[name], toks[i])
		default:
			return "", false
		}
	}
	if len(pos) < minPos(sp) || !sp.posList && len(pos) > len(sp.pos) {
		return "", false
	}
	var head []kv
	for i, p := range pos {
		switch {
		case sp.posList && i >= len(sp.pos)-1:
			setList(sp.pos[len(sp.pos)-1], p)
		case i < len(sp.pos):
			head = append(head, kv{sp.pos[i], p})
		}
	}
	// list_problems shows warning and above unless a severity is given; the CLI shows
	// everything.
	if sp.minInfo && !set["severity"] && !set["min_severity"] && !set["all"] && !set["include_muted"] && !set["ref"] {
		out = append(out, kv{"min_severity", "info"})
	}
	parts := []string{sp.tool}
	for _, p := range head {
		parts = append(parts, p.k+"="+hintValue(p.v))
	}
	for _, p := range out {
		parts = append(parts, p.k+"="+hintValue(p.v))
	}
	for _, k := range listOrder {
		var vs []string
		for _, v := range lists[k] {
			vs = append(vs, hintValue(v))
		}
		parts = append(parts, k+"=["+strings.Join(vs, ",")+"]")
	}
	return strings.Join(parts, " "), true
}

// minPos is how many positional arguments a command needs.
func minPos(sp *hintSpec) int {
	switch sp.cmd {
	case "source", "source search", "def", "mod", "why", "files search", "problems show", "problems ignore", "pin", "unpin", "pattern":
		return len(sp.pos)
	case "enable", "disable", "workshop subscribe", "workshop unsubscribe", "workshop resubscribe", "workshop state":
		return 1
	}
	return 0
}

// hintTokens splits a printed command into words; a word in double quotes keeps its spaces.
// A placeholder (<ref>, …) means the command is a pattern, not a call.
func hintTokens(s string) ([]string, bool) {
	var toks []string
	var cur strings.Builder
	quoted, has := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			quoted, has = !quoted, true
		case c == '\\' && quoted && i+1 < len(s) && s[i+1] == '"':
			cur.WriteByte('"')
			i++
		case c == ' ' && !quoted:
			if has {
				toks = append(toks, cur.String())
				cur.Reset()
				has = false
			}
		default:
			cur.WriteByte(c)
			has = true
		}
	}
	if quoted {
		return nil, false
	}
	if has {
		toks = append(toks, cur.String())
	}
	for _, t := range toks {
		if strings.ContainsAny(t, "<>…") || t == "..." {
			return nil, false
		}
	}
	return toks, true
}

// hintValue writes a parameter value: bare when it is made of name characters, else in
// double quotes.
func hintValue(v string) string {
	if v != "" && strings.IndexFunc(v, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_.:/-+@#", r))
	}) < 0 {
		return v
	}
	return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
}
