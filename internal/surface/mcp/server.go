// Package mcp is the MCP surface (05 §4): `archo mcp` over stdio. Every tool translates
// its arguments into the argv of one CLI command and returns that command's text, so the
// output is byte-identical to the CLI (05 §1, I1).
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/cmds"
)

// AnalyzeWait is how long a tool call waits for an out-of-date analysis (05 §4.1).
var AnalyzeWait = 10 * time.Second

func ptr[T any](v T) *T { return &v }

// tool is one MCP tool: annotations, description and the argv builder.
type tool struct {
	name        string
	readOnly    bool
	destructive bool
	desc        string
	add         func(s *sdk.Server, run func(env string, argv []string) (*sdk.CallToolResult, any, error), t *sdk.Tool)
	in          reflect.Type                       // the argument struct
	argvOf      func(raw []byte) ([]string, error) // JSON arguments → argv, as a tool call does
}

// Server serialises tool calls on one App (it is not safe for concurrent use).
type Server struct {
	app *app.App
	mu  sync.Mutex
	srv *sdk.Server
}

// Execute runs argv like the CLI and returns its text and exit code.
func (s *Server) Execute(env string, argv []string) (string, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if env != "" {
		prev := s.app.EnvName
		s.app.EnvName = env
		defer func() { s.app.EnvName = prev }()
	}
	return cmds.Execute(s.app, argv)
}

// MaxOutput caps one tool result. An agent that asked for --all on a large mod got 225-325 KB
// (G7 traces): more than fits in a useful context, and nothing it could not get by narrowing.
const MaxOutput = 24 << 10

// ClipOutput cuts a tool result at MaxOutput bytes on a line boundary and says how to narrow.
func ClipOutput(out string) string {
	if len(out) <= MaxOutput {
		return out
	}
	cut := strings.LastIndexByte(out[:MaxOutput], '\n')
	if cut < MaxOutput/2 {
		cut = MaxOutput
	}
	rest := strings.Count(out[cut:], "\n")
	return out[:cut] + fmt.Sprintf("\n… output clipped at %d KB of %d KB (%d more lines): narrow the call (mod=, target=, pattern, cluster) instead of all=true\n", cut>>10, len(out)>>10, rest)
}

func (s *Server) run(env string, argv []string) (*sdk.CallToolResult, any, error) {
	out, code := s.Execute(env, argv)
	out = ClipOutput(out)
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: out}}, IsError: code == cmds.ExitError || code == cmds.ExitUsage}, nil, nil
}

// New builds the server with every tool of 05 §4.2.
func New(a *app.App) *Server {
	s := &Server{app: a}
	// 05 §4.1: a first call that needs the analysis returns what is ready after 10 s.
	if a.AnalyzeWait == 0 {
		a.AnalyzeWait = AnalyzeWait
	}
	s.srv = sdk.NewServer(&sdk.Implementation{Name: "rimarcho", Version: buildinfo.Version}, &sdk.ServerOptions{Instructions: Instructions})
	for _, t := range Tools() {
		// get_description reaches the Steam Workshop (06 §6.4); everything else stays local.
		ann := &sdk.ToolAnnotations{ReadOnlyHint: t.readOnly, OpenWorldHint: ptr(t.name == "get_description")}
		if !t.readOnly {
			ann.DestructiveHint = ptr(t.destructive)
		}
		t.add(s.srv, s.run, &sdk.Tool{Name: t.name, Description: t.desc, Annotations: ann})
	}
	return s
}

// SDK returns the underlying SDK server (for in-memory tests).
func (s *Server) SDK() *sdk.Server { return s.srv }

// Run serves stdio until the client disconnects.
func Run(ctx context.Context, a *app.App) error {
	return New(a).srv.Run(ctx, &sdk.StdioTransport{})
}

// reg builds a tool whose arguments of type In map to argv.
func reg[In any](name string, readOnly, destructive bool, desc string, argv func(In) []string) tool {
	return tool{name: name, readOnly: readOnly, destructive: destructive, desc: desc,
		in: reflect.TypeOf((*In)(nil)).Elem(),
		argvOf: func(raw []byte) ([]string, error) {
			var in In
			if len(bytes.TrimSpace(raw)) > 0 {
				d := json.NewDecoder(bytes.NewReader(raw))
				d.DisallowUnknownFields()
				if err := d.Decode(&in); err != nil {
					return nil, fmt.Errorf("%s: %w", name, err)
				}
			}
			return argv(in), nil
		},
		add: func(s *sdk.Server, run func(string, []string) (*sdk.CallToolResult, any, error), t *sdk.Tool) {
			sdk.AddTool(s, t, func(_ context.Context, _ *sdk.CallToolRequest, in In) (*sdk.CallToolResult, any, error) {
				return run(reflect.ValueOf(in).FieldByName("Env").String(), argv(in))
			})
		}}
}

// flags appends --name value / --name for set options.
type builder struct{ a []string }

func (b *builder) pos(v string) *builder {
	if v != "" {
		b.a = append(b.a, v)
	}
	return b
}
func (b *builder) str(name, v string) *builder {
	if v != "" {
		b.a = append(b.a, "--"+name, v)
	}
	return b
}
func (b *builder) flag(name string, v bool) *builder {
	if v {
		b.a = append(b.a, "--"+name)
	}
	return b
}
func (b *builder) num(name string, v int) *builder {
	if v > 0 {
		b.a = append(b.a, "--"+name, strconv.Itoa(v))
	}
	return b
}
func argv(words ...string) *builder { return &builder{a: append([]string{}, words...)} }

// ---- tool arguments ----

type StatusIn struct {
	Env string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
}

type FindIn struct {
	Env    string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Query  string `json:"query,omitempty" jsonschema:"words to match against name, packageId, author, workshop id or folder; empty lists mods"`
	Active bool   `json:"active,omitempty" jsonschema:"list the enabled mods in load order (ignored with query)"`
	Filter string `json:"filter,omitempty" jsonschema:"list filter: source:<label|kind> status:live|tombstone|all residue unloadable copies pkg:<id> ws:<id> or words"`
	All    bool   `json:"all,omitempty" jsonschema:"no truncation"`
}

type RefIn struct {
	Env string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Ref string `json:"ref" jsonschema:"a mod reference: #<id>, L#<lineage>, pkg:<packageId>, ws:<workshopId> or src:<label>"`
}

type DefIn struct {
	Env         string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Def         string `json:"def,omitempty" jsonschema:"Type/defName (ThingDef/Gun_Revolver), Type@Name for abstract parents, def:… or a bare defName"`
	Mod         string `json:"mod,omitempty" jsonschema:"list the defs of this mod instead (#id, pkg:…)"`
	All         bool   `json:"all,omitempty"`
	GameVersion string `json:"game_version,omitempty" jsonschema:"analyse for another game major.minor than the environment's, e.g. 1.6"`
}

type PatchesIn struct {
	Env         string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Mod         string `json:"mod,omitempty" jsonschema:"only this mod's operations (#id, pkg:…)"`
	Target      string `json:"target,omitempty" jsonschema:"Type/defName of a touched def, or part of an XPath"`
	Failed      bool   `json:"failed,omitempty" jsonschema:"only operations that matched nothing (patch.no-match), with reasons"`
	All         bool   `json:"all,omitempty"`
	GameVersion string `json:"game_version,omitempty" jsonschema:"analyse for another game major.minor than the environment's, e.g. 1.6"`
}

type HarmonyIn struct {
	Env         string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Mod         string `json:"mod,omitempty" jsonschema:"this mod's assemblies and patches (#id, pkg:…)"`
	Target      string `json:"target,omitempty" jsonschema:"patched method, e.g. Verse.Pawn.Tick; partial names search"`
	Shared      bool   `json:"shared,omitempty" jsonschema:"methods patched by several mods (with mod: those involving it)"`
	All         bool   `json:"all,omitempty"`
	GameVersion string `json:"game_version,omitempty" jsonschema:"analyse for another game major.minor than the environment's, e.g. 1.6"`
}

type WhyIn struct {
	Env         string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	A           string `json:"a" jsonschema:"first mod reference"`
	B           string `json:"b" jsonschema:"second mod reference"`
	GameVersion string `json:"game_version,omitempty" jsonschema:"analyse for another game major.minor than the environment's, e.g. 1.6"`
}

type ProblemsIn struct {
	Env         string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Subject     string `json:"subject,omitempty" jsonschema:"only problems involving this mod"`
	Code        string `json:"code,omitempty" jsonschema:"code or prefix: dep, order, patch.no-match, harmony, log …"`
	Severity    string `json:"severity,omitempty" jsonschema:"exactly this severity: error, warning or info"`
	MinSeverity string `json:"min_severity,omitempty" jsonschema:"lowest severity to list (default warning); info shows everything"`
	Ref         string `json:"ref,omitempty" jsonschema:"show one problem in full (p:<hex>)"`
	All         bool   `json:"all,omitempty" jsonschema:"no truncation and no folding; include def.override / patch.contested"`
	Detail      bool   `json:"detail,omitempty" jsonschema:"evidence and fix commands for every problem"`
	Ignored     bool   `json:"ignored,omitempty" jsonschema:"list the problems the player ignored"`
	GameVersion string `json:"game_version,omitempty" jsonschema:"analyse for another game major.minor than the environment's, e.g. 1.6"`
}

type ChangesIn struct {
	Env   string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Ref   string `json:"ref,omitempty" jsonschema:"one mod's history"`
	Since string `json:"since,omitempty" jsonschema:"scan:<n>, <days>d or YYYY-MM-DD"`
	All   bool   `json:"all,omitempty"`
}

type LogIn struct {
	Env         string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Cluster     string `json:"cluster,omitempty" jsonschema:"one cluster in full (E3 or log:E3): frames, Harmony lines, attribution, next steps"`
	Text        string `json:"text,omitempty" jsonschema:"analyse this pasted log excerpt or stack trace instead of Player.log"`
	File        string `json:"file,omitempty" jsonschema:"another log file path"`
	Prev        bool   `json:"prev,omitempty" jsonschema:"Player-prev.log (the run before the last)"`
	All         bool   `json:"all,omitempty" jsonschema:"include informational clusters"`
	GameVersion string `json:"game_version,omitempty" jsonschema:"analyse for another game major.minor than the environment's, e.g. 1.6"`
}

type FilesIn struct {
	Env       string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Pattern   string `json:"pattern" jsonschema:"text to find (case-insensitive) in the effective XML/TXT of enabled mods"`
	Mod       string `json:"mod,omitempty" jsonschema:"only this mod"`
	Regex     bool   `json:"regex,omitempty"`
	Languages bool   `json:"languages,omitempty" jsonschema:"also search Languages/ (translations)"`
	All       bool   `json:"all,omitempty"`
}

type SourceIn struct {
	Env    string   `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Ref    string   `json:"ref,omitempty" jsonschema:"decompile this mod (with type: one type) — #id or pkg:…"`
	Type   string   `json:"type,omitempty" jsonschema:"full type name to show, e.g. MyMod.HarmonyPatches.Pawn_Tick"`
	Search string   `json:"search,omitempty" jsonschema:"search decompiled C# for this text instead"`
	In     []string `json:"in,omitempty" jsonschema:"mods to search (default: the mods the newest log and Harmony warnings implicate)"`
	Scope  string   `json:"scope,omitempty" jsonschema:"active: search every enabled mod (decompiles everything not cached; can take many minutes)"`
	Regex  bool     `json:"regex,omitempty"`
	All    bool     `json:"all,omitempty"`
	Budget int      `json:"budget,omitempty" jsonschema:"seconds to spend decompiling before searching what is cached (default 10)"`
}

type EditIn struct {
	Env    string   `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Action string   `json:"action" jsonschema:"enable, disable, move, swap, sort, undo, redo, stage, unstage or restore"`
	Refs   []string `json:"refs,omitempty" jsonschema:"mods to enable/disable; for move and swap the first is the mod (swap: optional second = the instance to use)"`
	At     int      `json:"at,omitempty" jsonschema:"enable at / move to this 1-based position"`
	Before string   `json:"before,omitempty"`
	After  string   `json:"after,omitempty"`
	Up     bool     `json:"up,omitempty"`
	Down   bool     `json:"down,omitempty"`
	DryRun bool     `json:"dry_run,omitempty" jsonschema:"sort only: show the result without changing the workspace"`
}

type DescribeIn struct {
	Env     string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Ref     string `json:"ref" jsonschema:"a mod reference (#id, pkg:…, ws:…)"`
	Source  string `json:"source,omitempty" jsonschema:"auto (default), about, workshop or both"`
	Refresh bool   `json:"refresh,omitempty" jsonschema:"fetch the Workshop page again"`
	Offline bool   `json:"offline,omitempty" jsonschema:"use the cache only"`
}

type PlayerIn struct {
	Env    string   `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Action string   `json:"action" jsonschema:"tag_add, tag_remove, alias, note"`
	Refs   []string `json:"refs" jsonschema:"the mods (tag actions: one or more; alias and note: exactly one)"`
	Tags   []string `json:"tags,omitempty" jsonschema:"tag_add / tag_remove: the tags"`
	Text   string   `json:"text,omitempty" jsonschema:"alias / note: the text; empty clears it"`
}

type CopyIn struct {
	Env  string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Ref  string `json:"ref" jsonschema:"the mod to copy (#id, pkg:…)"`
	Name string `json:"name,omitempty" jsonschema:"folder name under Mods/ (default <folder>_local)"`
	Git  bool   `json:"git,omitempty" jsonschema:"make the copy a git repository with a baseline commit, so edits can be diffed"`
}

type SaveIn struct {
	Env    string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	DryRun bool   `json:"dry_run,omitempty" jsonschema:"show what would be written without writing"`
}

type RuleIn struct {
	Env    string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Kind   string `json:"kind" jsonschema:"after, before, incompatible, attach (A B) or ignore (target)"`
	A      string `json:"a,omitempty" jsonschema:"the mod the rule is about"`
	B      string `json:"b,omitempty" jsonschema:"the other mod"`
	Target string `json:"target,omitempty" jsonschema:"for ignore: the constraint ref printed by sort/problems, e.g. decl:#ab12:loadafter:pkg.id"`
	Note   string `json:"note,omitempty"`
}

func gv(b *builder, mm string) []string { return b.str("game-version", mm).a }

func toolList() []tool {
	return []tool{
		reg("status", true, false, descStatus, func(in StatusIn) []string { return []string{"status"} }),
		reg("find_mods", true, false, descFind, func(in FindIn) []string {
			if in.Query != "" {
				return argv("find").pos(in.Query).flag("all", in.All).a
			}
			return argv("list").flag("active", in.Active).str("filter", in.Filter).flag("all", in.All).a
		}),
		reg("get_mod", true, false, descMod, func(in RefIn) []string { return []string{"mod", in.Ref} }),
		reg("get_description", true, false, descDescribe, func(in DescribeIn) []string {
			return argv("describe", in.Ref).str("source", in.Source).flag("refresh", in.Refresh).flag("offline", in.Offline).a
		}),
		reg("get_def", true, false, descDef, func(in DefIn) []string {
			return gv(argv("def").pos(in.Def).str("mod", in.Mod).flag("all", in.All), in.GameVersion)
		}),
		reg("get_patches", true, false, descPatches, func(in PatchesIn) []string {
			return gv(argv("patches").str("mod", in.Mod).str("target", in.Target).flag("failed", in.Failed).flag("all", in.All), in.GameVersion)
		}),
		reg("get_harmony", true, false, descHarmony, func(in HarmonyIn) []string {
			return gv(argv("harmony").str("mod", in.Mod).str("target", in.Target).flag("shared", in.Shared).flag("all", in.All), in.GameVersion)
		}),
		reg("explain_relation", true, false, descWhy, func(in WhyIn) []string {
			return gv(argv("why", in.A, in.B), in.GameVersion)
		}),
		reg("list_problems", true, false, descProblems, func(in ProblemsIn) []string { return ProblemsArgv(in) }),
		reg("list_changes", true, false, descChanges, func(in ChangesIn) []string {
			return argv("changes").pos(in.Ref).str("since", in.Since).flag("all", in.All).a
		}),
		reg("analyze_log", true, false, descLog, func(in LogIn) []string {
			return gv(argv("log").str("cluster", in.Cluster).str("text", in.Text).str("file", in.File).flag("prev", in.Prev).flag("all", in.All), in.GameVersion)
		}),
		reg("search_files", true, false, descFiles, func(in FilesIn) []string {
			return argv("files", "search", in.Pattern).str("mod", in.Mod).flag("regex", in.Regex).flag("languages", in.Languages).flag("all", in.All).a
		}),
		reg("read_source", true, false, descSource, func(in SourceIn) []string { return SourceArgv(in) }),
		reg("edit_loadout", false, false, descEdit, func(in EditIn) []string { return EditArgv(in) }),
		reg("save_loadout", false, true, descSave, func(in SaveIn) []string { return argv("save").flag("dry-run", in.DryRun).a }),
		reg("copy_local", false, false, descCopy, func(in CopyIn) []string {
			return argv("copy-local", in.Ref).str("name", in.Name).flag("git", in.Git).a
		}),
		reg("set_player_data", false, false, descPlayer, func(in PlayerIn) []string { return PlayerArgv(in) }),
		reg("add_rule", false, false, descRule, func(in RuleIn) []string {
			b := argv("rule", "add", in.Kind)
			if in.Kind == "ignore" {
				b.pos(in.Target)
			} else {
				b.pos(in.A).pos(in.B)
			}
			return b.str("note", in.Note).a
		}),
	}
}

// Tools returns the tool table.
func Tools() []tool { return toolList() }

// ToolDoc describes one tool as a client sees it (name, description, parameters): for
// harnesses that drive the tools without an MCP client (spikes/agenteval).
type ToolDoc struct {
	Name, Description string
	ReadOnly          bool
	Params            []ParamDoc
}

// ParamDoc is one argument: JSON name, JSON type, the schema help text.
type ParamDoc struct {
	Name, Type, Help string
	Required         bool
}

// Catalog lists every tool with its parameters (the env parameter is left out).
func Catalog() []ToolDoc {
	var out []ToolDoc
	for _, t := range toolList() {
		d := ToolDoc{Name: t.name, Description: t.desc, ReadOnly: t.readOnly}
		for i := 0; i < t.in.NumField(); i++ {
			f := t.in.Field(i)
			name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "" || name == "env" {
				continue
			}
			typ := "string"
			switch f.Type.Kind() {
			case reflect.Bool:
				typ = "boolean"
			case reflect.Int:
				typ = "integer"
			case reflect.Slice:
				typ = "array of strings"
			}
			d.Params = append(d.Params, ParamDoc{Name: name, Type: typ, Help: f.Tag.Get("jsonschema"), Required: !strings.Contains(opts, "omitempty")})
		}
		out = append(out, d)
	}
	return out
}

// ArgvFor maps a tool call (name and JSON arguments) to the CLI argv the server would run.
// Unknown argument names are an error.
func ArgvFor(name string, args []byte) ([]string, error) {
	for _, t := range toolList() {
		if t.name == name {
			return t.argvOf(args)
		}
	}
	return nil, fmt.Errorf("unknown tool %q", name)
}

// ProblemsArgv maps list_problems: warning and above by default (decisions-m3 §12.2).
func ProblemsArgv(in ProblemsIn) []string {
	if in.Ref != "" {
		return []string{"problems", "show", in.Ref}
	}
	min := in.MinSeverity
	if min == "" && in.Severity == "" && !in.All {
		min = "warning"
	}
	return gv(argv("problems").str("subject", in.Subject).str("code", in.Code).str("severity", in.Severity).str("min-severity", min).
		flag("all", in.All).flag("detail", in.Detail).flag("ignored", in.Ignored), in.GameVersion)
}

// SourceArgv maps read_source: a search gets a 10 s decompile budget by default (05 §4.1).
func SourceArgv(in SourceIn) []string {
	if in.Search != "" {
		b := argv("source", "search", in.Search)
		for _, r := range in.In {
			b.str("in", r)
		}
		switch {
		case len(in.In) > 0:
		case in.Scope == "active":
			b.flag("active", true).flag("yes", true)
		default:
			b.flag("implicated", true) // decisions-m4 §7.4
		}
		budget := in.Budget
		if budget == 0 {
			budget = 10
		}
		return b.flag("regex", in.Regex).flag("all", in.All).num("budget", budget).a
	}
	return argv("source").pos(in.Ref).str("type", in.Type).flag("all", in.All).a
}

// EditArgv maps edit_loadout to one workspace command.
func EditArgv(in EditIn) []string {
	first := ""
	if len(in.Refs) > 0 {
		first = in.Refs[0]
	}
	switch in.Action {
	case "enable":
		b := argv(append([]string{"enable"}, in.Refs...)...)
		if in.At > 0 {
			b.str("at", strconv.Itoa(in.At))
		}
		return b.str("before", in.Before).str("after", in.After).a
	case "disable":
		return argv(append([]string{"disable"}, in.Refs...)...).a
	case "move":
		b := argv("move").pos(first)
		if in.At > 0 {
			b.str("to", strconv.Itoa(in.At))
		}
		return b.str("before", in.Before).str("after", in.After).flag("up", in.Up).flag("down", in.Down).a
	case "swap":
		return argv(append([]string{"swap"}, in.Refs...)...).a
	case "sort":
		return argv("sort").flag("dry-run", in.DryRun).a
	case "undo", "redo":
		return []string{in.Action}
	case "stage":
		return append([]string{"stage", "add"}, in.Refs...)
	case "unstage":
		return append([]string{"stage", "drop"}, in.Refs...)
	case "restore":
		b := argv(append([]string{"stage", "restore"}, in.Refs...)...)
		if in.At > 0 {
			b.str("at", strconv.Itoa(in.At))
		}
		return b.str("before", in.Before).str("after", in.After).a
	}
	return []string{"edit_loadout: unknown action " + fmt.Sprintf("%q", in.Action)}
}

// PlayerArgv maps set_player_data to tag / alias / note.
func PlayerArgv(in PlayerIn) []string {
	first := ""
	if len(in.Refs) > 0 {
		first = in.Refs[0]
	}
	switch in.Action {
	case "tag_add", "tag_remove":
		b := argv(append([]string{"tag", strings.TrimPrefix(in.Action, "tag_"), first}, in.Tags...)...)
		for _, r := range in.Refs[min(1, len(in.Refs)):] {
			b.str("mod", r)
		}
		return b.a
	case "alias", "note":
		if in.Text == "" {
			return []string{in.Action, first, "--clear"}
		}
		return []string{in.Action, first, in.Text}
	}
	return []string{"set_player_data: unknown action " + fmt.Sprintf("%q", in.Action)}
}
