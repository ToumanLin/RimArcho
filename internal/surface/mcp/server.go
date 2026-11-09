// Package mcp is the MCP surface (14): `archo mcp` over stdio. Every tool translates
// its arguments into the argv of one CLI command and returns that command's text, so the
// output is byte-identical to the CLI (13 §1.1, I1).
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/cmds"
)

// AnalyzeWait is how long a tool call waits for an out-of-date analysis (14 §1).
var AnalyzeWait = 10 * time.Second

func ptr[T any](v T) *T { return &v }

// Group is a set of tools `archo mcp --tools` serves (14 §1): Core by default, All for
// every tool. Each tool's group is part of its row in the tool table.
type Group string

const (
	Core Group = "core" // the agent's tools: diagnosis, plan edits, saving, launching, testing
	All  Group = "all"  // Core plus the player's management tools
)

// ParseGroup reads a --tools value; "" is Core.
func ParseGroup(s string) (Group, error) {
	switch Group(s) {
	case "", Core:
		return Core, nil
	case All:
		return All, nil
	}
	return "", fmt.Errorf("unknown --tools value %q: use core or all", s)
}

// tool is one MCP tool: annotations, description and the argv builder.
type tool struct {
	name        string
	group       Group // Core, or All for a tool only `--tools all` serves
	readOnly    bool
	destructive bool
	desc        string
	add         func(s *sdk.Server, run runFunc, t *sdk.Tool)
	in          reflect.Type                       // the argument struct
	argvOf      func(raw []byte) ([]string, error) // JSON arguments → argv, as a tool call does
}

// runFunc runs one tool call's argv (Server.run).
type runFunc func(context.Context, *sdk.CallToolRequest, string, []string) (*sdk.CallToolResult, any, error)

// Server serialises tool calls on one App (it is not safe for concurrent use).
type Server struct {
	app *app.App
	mu  sync.Mutex
	srv *sdk.Server
}

// Execute runs argv like the CLI and returns its text and exit code.
func (s *Server) Execute(env string, argv []string) (string, int) {
	return s.executeContext(context.Background(), env, argv)
}

func (s *Server) executeContext(ctx context.Context, env string, argv []string) (string, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if env != "" {
		prev := s.app.EnvName
		s.app.EnvName = env
		defer func() { s.app.EnvName = prev }()
	}
	out, code := cmds.ExecuteContext(cmds.ForMCP(ctx), s.app, argv)
	if len(argv) >= 2 && argv[0] == "config" && argv[1] == "get" && s.app.Store.Meta("cfg:steamcmd.account") != "" {
		lines := strings.Split(out, "\n")
		for i, line := range lines {
			if strings.Contains(line, "steamcmd.account") {
				lines[i] = "steamcmd.account: player login required"
			}
		}
		out = strings.Join(lines, "\n")
	}
	return out, code
}

// MaxOutput caps outline results. An agent that asked for --all on a large mod got 225-325 KB
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

func (s *Server) run(ctx context.Context, req *sdk.CallToolRequest, env string, argv []string) (*sdk.CallToolResult, any, error) {
	ctx, stop := heartbeat(ctx, req)
	out, code := s.executeContext(ctx, env, argv)
	stop()
	// Test reports already limit clusters; clipping their JSON would corrupt it.
	if len(argv) < 2 || (argv[0] != "test" || argv[1] != "run") && argv[0] != "links" {
		out = ClipOutput(out)
	}
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: out}}, IsError: code == cmds.ExitError || code == cmds.ExitUsage}, nil, nil
}

// ProgressEvery is how often a running tool call sends a progress notification (14 §1).
var ProgressEvery = 10 * time.Second

// heartbeat sends a progress notification every ProgressEvery until stop is called, when
// the client asked for progress. Clients that restart their request timeout on progress
// (Pi does, after 60 s) then wait for a test run instead of cancelling it, which stops
// the game. The message is the command's latest progress line, with the elapsed time.
func heartbeat(ctx context.Context, req *sdk.CallToolRequest) (context.Context, func()) {
	if req == nil || req.Session == nil || req.Params == nil || req.Params.GetProgressToken() == nil {
		return ctx, func() {}
	}
	token := req.Params.GetProgressToken()
	var step atomic.Pointer[string]
	ctx = cmds.WithProgress(ctx, func(line string) { step.Store(&line) })
	done, finished := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(finished)
		start := time.Now()
		tick := time.NewTicker(ProgressEvery)
		defer tick.Stop()
		for n := 1; ; n++ {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			msg := "running"
			if p := step.Load(); p != nil {
				msg = strings.TrimPrefix(*p, "progress: ")
			}
			msg = fmt.Sprintf("%s (%ds)", msg, int(time.Since(start).Seconds()))
			req.Session.NotifyProgress(ctx, &sdk.ProgressNotificationParams{ProgressToken: token, Progress: float64(n), Message: msg})
		}
	}()
	// Waiting for the goroutine keeps every notification before the result.
	return ctx, func() { close(done); <-finished }
}

// New builds the server with the tools of group g (14 §2): Core, or All for every tool.
func New(a *app.App, g Group) *Server {
	s := &Server{app: a}
	// 14 §1: a first call that needs the analysis returns what is ready after 10 s.
	if a.AnalyzeWait == 0 {
		a.AnalyzeWait = AnalyzeWait
	}
	s.srv = sdk.NewServer(&sdk.Implementation{Name: "rimarcho", Version: buildinfo.Version}, &sdk.ServerOptions{Instructions: Instructions})
	for _, t := range Tools() {
		if t.group == All && g != All {
			continue
		}
		// Only the tools in openWorld may reach the network; everything else stays local.
		ann := &sdk.ToolAnnotations{ReadOnlyHint: t.readOnly, OpenWorldHint: ptr(openWorld[t.name])}
		if !t.readOnly {
			ann.DestructiveHint = ptr(t.destructive)
		}
		t.add(s.srv, s.run, &sdk.Tool{Name: t.name, Description: t.desc, Annotations: ann})
	}
	return s
}

// SDK returns the underlying SDK server (for in-memory tests).
func (s *Server) SDK() *sdk.Server { return s.srv }

// Run serves group g's tools over stdio until the client disconnects.
func Run(ctx context.Context, a *app.App, g Group) error {
	return New(a, g).srv.Run(ctx, &sdk.StdioTransport{})
}

// reg builds a tool whose arguments of type In map to argv.
func reg[In any](name string, readOnly, destructive bool, desc string, argv func(In) []string) tool {
	return tool{name: name, group: Core, readOnly: readOnly, destructive: destructive, desc: desc,
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
		add: func(s *sdk.Server, run runFunc, t *sdk.Tool) {
			if sc, err := inputSchema[In](); err == nil {
				t.InputSchema = sc // list arguments also accept a string (StrList)
			}
			sdk.AddTool(s, t, func(ctx context.Context, req *sdk.CallToolRequest, in In) (*sdk.CallToolResult, any, error) {
				return run(ctx, req, envOf(in), argv(in))
			})
		}}
}

// allOnly moves a tool to All: only `archo mcp --tools all` serves it.
func (t tool) allOnly() tool { t.group = All; return t }

// envOf is a call's env argument; "" (the current environment) when the tool has none.
func envOf(in any) string {
	if f := reflect.ValueOf(in).FieldByName("Env"); f.IsValid() {
		return f.String()
	}
	return ""
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

// GuideIn is the guide tool: a workflow or a tool name (14 §3). The guide is the same in
// every environment, so it takes no env.
type GuideIn struct {
	Topic string `json:"topic,omitempty" jsonschema:"diagnose, errors, plans, rebuild, words, or a tool name; empty lists the topics"`
}

type StatusIn struct {
	Env  string `json:"env,omitempty" jsonschema:"environment (default: current)"`
	Show string `json:"show,omitempty" jsonschema:"overview (default), envs, folders (the shared mod folders), settings, data (community data), db or cache"`
}

type FindIn struct {
	Env    string `json:"env,omitempty" jsonschema:"environment (default: current)"`
	Query  string `json:"query,omitempty" jsonschema:"words to match against name, packageId, author, workshop id or folder; empty lists mods"`
	Active bool   `json:"active,omitempty" jsonschema:"list the enabled mods in load order (ignored with query)"`
	Filter string `json:"filter,omitempty" jsonschema:"list filter: source:<label|kind> status:live|tombstone|all residue unloadable copies pkg:<id> ws:<id> or words"`
	All    bool   `json:"all,omitempty" jsonschema:"no truncation"`
}

type RefIn struct {
	Env string `json:"env,omitempty" jsonschema:"environment (default: current)"`
	Ref string `json:"ref" jsonschema:"a mod reference: #<id>, L#<lineage>, pkg:<packageId>, ws:<workshopId> or src:<label>"`
}

// ModSettingsSetIn is set_mod_settings: archo mod-settings set (16 §2.9 "Mod settings").
type ModSettingsSetIn struct {
	Env    string            `json:"env,omitempty" jsonschema:"environment (default: current)"`
	Ref    string            `json:"ref" jsonschema:"the mod: #<id>, L#<lineage>, pkg:<packageId>, ws:<workshopId>"`
	File   string            `json:"file,omitempty" jsonschema:"which settings when the Mod has several: its Mod class or hugslib:<pack>, as get_mod_settings names them"`
	Set    map[string]string `json:"set,omitempty" jsonschema:"setting name (or HugsLib title) → new value: on/off, a number, one line of text, a HugsLib choice"`
	Reset  StrList           `json:"reset,omitempty" jsonschema:"settings whose stored value is removed: the Mod uses its default"`
	DryRun bool              `json:"dry_run,omitempty" jsonschema:"check and show the change without writing"`
}

// ModSettingsSetArgv maps set_mod_settings to its command (set pairs in name order).
func ModSettingsSetArgv(in ModSettingsSetIn) []string {
	b := argv("mod-settings", "set", in.Ref)
	names := make([]string, 0, len(in.Set))
	for k := range in.Set {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		b.a = append(b.a, k+"="+in.Set[k])
	}
	for _, r := range in.Reset {
		b.str("reset", r)
	}
	return b.str("file", in.File).flag("dry-run", in.DryRun).a
}

// ModSettingsIn is get_mod_settings: archo mod-settings (16 §2.9 "Mod settings").
type ModSettingsIn struct {
	Env string `json:"env,omitempty" jsonschema:"environment (default: current)"`
	Ref string `json:"ref,omitempty" jsonschema:"a mod reference (#<id>, L#<lineage>, pkg:<packageId>, ws:<workshopId>): its stored values; empty lists the current list's Mods that have settings"`
	All bool   `json:"all,omitempty" jsonschema:"no truncation"`
}

type DefIn struct {
	Env         string `json:"env,omitempty" jsonschema:"environment (default: current)"`
	Def         string `json:"def,omitempty" jsonschema:"Type/defName (ThingDef/Gun_Revolver), Type@Name for abstract parents, def:… or a bare defName"`
	Mod         string `json:"mod,omitempty" jsonschema:"list the defs of this mod instead (#id, pkg:…)"`
	All         bool   `json:"all,omitempty"`
	GameVersion string `json:"game_version,omitempty" jsonschema:"another game major.minor than the environment's, e.g. 1.6"`
}

type PatchesIn struct {
	Env         string `json:"env,omitempty" jsonschema:"environment (default: current)"`
	Mod         string `json:"mod,omitempty" jsonschema:"only this mod's operations (#id, pkg:…)"`
	Target      string `json:"target,omitempty" jsonschema:"Type/defName of a touched def; a bare defName also finds operations nested in FindMod/Sequence; or part of an XPath"`
	Failed      bool   `json:"failed,omitempty" jsonschema:"only operations that matched nothing (patch.no-match), with reasons"`
	All         bool   `json:"all,omitempty"`
	GameVersion string `json:"game_version,omitempty" jsonschema:"another game major.minor than the environment's, e.g. 1.6"`
}

type HarmonyIn struct {
	Env         string `json:"env,omitempty" jsonschema:"environment (default: current)"`
	Mod         string `json:"mod,omitempty" jsonschema:"this mod's assemblies and patches (#id, pkg:…)"`
	Target      string `json:"target,omitempty" jsonschema:"patched method, e.g. Verse.Pawn.Tick; partial names search"`
	Shared      bool   `json:"shared,omitempty" jsonschema:"methods patched by several mods (with mod: those involving it)"`
	All         bool   `json:"all,omitempty"`
	GameVersion string `json:"game_version,omitempty" jsonschema:"another game major.minor than the environment's, e.g. 1.6"`
}

type WhyIn struct {
	Env         string `json:"env,omitempty" jsonschema:"environment (default: current)"`
	A           string `json:"a" jsonschema:"first mod reference"`
	B           string `json:"b" jsonschema:"second mod reference"`
	GameVersion string `json:"game_version,omitempty" jsonschema:"another game major.minor than the environment's, e.g. 1.6"`
}

type ProblemsIn struct {
	Env         string `json:"env,omitempty" jsonschema:"environment (default: current)"`
	Subject     string `json:"subject,omitempty" jsonschema:"only problems involving this mod"`
	Code        string `json:"code,omitempty" jsonschema:"code or prefix: dep, order, patch.no-match, harmony, log …"`
	Severity    string `json:"severity,omitempty" jsonschema:"exactly this severity: error, warning or info"`
	MinSeverity string `json:"min_severity,omitempty" jsonschema:"lowest severity to list (default warning); info shows everything"`
	Ref         string `json:"ref,omitempty" jsonschema:"show one problem in full (p:<hex>)"`
	All         bool   `json:"all,omitempty" jsonschema:"no truncation and no folding; include def.override / patch.contested"`
	Detail      bool   `json:"detail,omitempty" jsonschema:"evidence and fix commands for every problem"`
	Ignored     bool   `json:"ignored,omitempty" jsonschema:"list the problems the player ignored"`
	GameVersion string `json:"game_version,omitempty" jsonschema:"another game major.minor than the environment's, e.g. 1.6"`

	IncludeMuted bool `json:"include_muted,omitempty" jsonschema:"also show ignored version warnings, marked ignored"`
}

type ChangesIn struct {
	Env     string `json:"env,omitempty" jsonschema:"environment (default: current)"`
	Ref     string `json:"ref,omitempty" jsonschema:"one mod's history"`
	Library bool   `json:"library,omitempty" jsonschema:"every mod, not only the current plan's"`
	Since   string `json:"since,omitempty" jsonschema:"scan:<n>, <days>d or YYYY-MM-DD"`
	All     bool   `json:"all,omitempty"`
}

type LogIn struct {
	Env         string `json:"env,omitempty" jsonschema:"environment (default: current)"`
	Cluster     string `json:"cluster,omitempty" jsonschema:"one cluster in full (E3 or log:E3): frames, Harmony lines, attribution, next steps"`
	Text        string `json:"text,omitempty" jsonschema:"analyse this pasted log excerpt or stack trace instead of Player.log"`
	File        string `json:"file,omitempty" jsonschema:"another log file path"`
	Prev        bool   `json:"prev,omitempty" jsonschema:"Player-prev.log (the run before the last)"`
	All         bool   `json:"all,omitempty" jsonschema:"include informational clusters"`
	Since       string `json:"since,omitempty" jsonschema:"the cursor an earlier analyze_log call returned: only clusters new or grown after it (poll a running game)"`
	GameVersion string `json:"game_version,omitempty" jsonschema:"another game major.minor than the environment's, e.g. 1.6"`
	Pattern     string `json:"pattern,omitempty" jsonschema:"explain an error pattern instead (pattern:g:xref.def from a cluster; list = all kinds): causes, route, fixes, how often public logs show it"`
}

type FilesIn struct {
	Env           string  `json:"env,omitempty" jsonschema:"environment (default: current)"`
	Pattern       string  `json:"pattern" jsonschema:"text, defName, or Go regular expression to find"`
	Mod           string  `json:"mod,omitempty" jsonschema:"one mod reference; pkg: prefers its enabled copy"`
	Mods          StrList `json:"mods,omitempty" jsonschema:"mod references to search, including inactive copies"`
	Def           bool    `json:"def,omitempty" jsonschema:"find a case-sensitive whole defName with roles and summary"`
	Regex         bool    `json:"regex,omitempty" jsonschema:"use a Go regular expression"`
	Word          bool    `json:"word,omitempty" jsonschema:"require identifier boundaries"`
	CaseSensitive bool    `json:"case_sensitive,omitempty" jsonschema:"match case (default: insensitive)"`
	Kind          string  `json:"kind,omitempty" jsonschema:"comma-separated defs,patches,about,languages,textures,sounds,assemblies or all"`
	Languages     bool    `json:"languages,omitempty" jsonschema:"also search translations in Languages/"`
	Lang          StrList `json:"lang,omitempty" jsonschema:"language folder names to include; implies languages"`
	AllVersions   bool    `json:"all_versions,omitempty" jsonschema:"search every version and LoadFolders branch without shadowing"`
	Game          string  `json:"game,omitempty" jsonschema:"official Core/DLC content: with (default), without, or only"`
	Mode          string  `json:"mode,omitempty" jsonschema:"matches (default), files, or count by mod"`
	Page          int     `json:"page,omitempty" jsonschema:"1-based page number (default 1)"`
	Limit         int     `json:"limit,omitempty" jsonschema:"rows per page (default 40)"`
	PerFile       int     `json:"per_file,omitempty" jsonschema:"matching lines kept per file before paging (default 5)"`
	All           bool    `json:"all,omitempty" jsonschema:"remove paging; per-file cap and 24 KB output cap still apply"`
	GameVersion   string  `json:"game_version,omitempty" jsonschema:"game major.minor to resolve, e.g. 1.6 (default: environment)"`
}

// FilesArgv translates search options into the same flags used by the CLI.
func FilesArgv(in FilesIn) []string {
	b := argv("files", "search").str("mod", in.Mod)
	for _, mod := range in.Mods {
		b.str("mod", mod)
	}
	b.flag("def", in.Def).flag("regex", in.Regex).flag("word", in.Word).flag("case", in.CaseSensitive).
		str("kind", in.Kind).flag("languages", in.Languages)
	for _, lang := range in.Lang {
		b.str("lang", lang)
	}
	b.flag("all-versions", in.AllVersions).str("game", in.Game).str("mode", in.Mode)
	for _, n := range []struct {
		name  string
		value int
	}{{"page", in.Page}, {"limit", in.Limit}, {"per-file", in.PerFile}} {
		if n.value != 0 {
			b.a = append(b.a, "--"+n.name, strconv.Itoa(n.value))
		}
	}
	b.flag("all", in.All).str("game-version", in.GameVersion)
	if strings.HasPrefix(in.Pattern, "--") {
		b.a = append(b.a, "--")
	}
	return append(b.a, in.Pattern)
}

type SourceIn struct {
	Game   bool    `json:"game,omitempty" jsonschema:"game code (with search: also searched)"`
	Env    string  `json:"env,omitempty" jsonschema:"environment (default: current)"`
	Ref    string  `json:"ref,omitempty" jsonschema:"a mod, #id or pkg:…"`
	Type   string  `json:"type,omitempty" jsonschema:"full type name (callers: Ns.Type.Method)"`
	Member string  `json:"member,omitempty" jsonschema:"with type: one member, with line numbers"`
	Search string  `json:"search,omitempty" jsonschema:"search decompiled C# for this text instead"`
	In     StrList `json:"in,omitempty" jsonschema:"mods to search (default: the log's suspects)"`
	Scope  string  `json:"scope,omitempty" jsonschema:"active: every enabled mod; repeat while pending"`
	Regex  bool    `json:"regex,omitempty"`
	All    bool    `json:"all,omitempty"`
	Budget int     `json:"budget,omitempty" jsonschema:"decompile seconds per call (default 10)"`
	Mode   string  `json:"mode,omitempty" jsonschema:"callers (of the method in type) or diff (of dlls)"`
	DLLs   StrList `json:"dlls,omitempty" jsonschema:"old and new DLL"`
}

type EditIn struct {
	Env     string  `json:"env,omitempty" jsonschema:"environment (default: current)"`
	Action  string  `json:"action" jsonschema:"enable, disable, move, swap, sort, undo, redo, checkout, stage, unstage, restore or ignore_external"`
	Refs    StrList `json:"refs,omitempty" jsonschema:"mods to enable/disable; for move and swap the first is the mod (swap: optional second = the instance to use)"`
	At      int     `json:"at,omitempty" jsonschema:"enable at / move to this 1-based position"`
	Before  string  `json:"before,omitempty"`
	After   string  `json:"after,omitempty"`
	Up      bool    `json:"up,omitempty"`
	Down    bool    `json:"down,omitempty"`
	DryRun  bool    `json:"dry_run,omitempty" jsonschema:"sort only: show the result without changing the workspace"`
	Version string  `json:"version,omitempty" jsonschema:"checkout: the version to load into the current plan, v<n> or <n> (plan_history action=log lists them)"`
}

type DescribeIn struct {
	Env     string `json:"env,omitempty" jsonschema:"environment (default: current)"`
	Ref     string `json:"ref" jsonschema:"a mod reference (#id, pkg:…, ws:…)"`
	Source  string `json:"source,omitempty" jsonschema:"auto (default), about, workshop or both"`
	Refresh bool   `json:"refresh,omitempty" jsonschema:"fetch the Workshop page again"`
	Offline bool   `json:"offline,omitempty" jsonschema:"use the cache only"`
}

type PlayerIn struct {
	Env    string  `json:"env,omitempty" jsonschema:"environment (default: current)"`
	Action string  `json:"action" jsonschema:"tag_add, tag_remove, tag_order, alias, note, copy_note (one copy's own); on every mod: tag_rename, tag_merge, tag_delete, tag_note, tag_color"`
	Refs   StrList `json:"refs,omitempty" jsonschema:"the mods (tag_add / tag_remove: one or more; others: one)"`
	Tags   StrList `json:"tags,omitempty" jsonschema:"the tags (tag_rename, tag_note: one)"`
	Text   string  `json:"text,omitempty" jsonschema:"alias, note, copy_note, tag_note: text, empty clears it; tag_rename: new name; tag_merge: target; tag_color: colour name or #rrggbb"`
	DryRun bool    `json:"dry_run,omitempty" jsonschema:"preview a tag edit"`
}

type SaveIn struct {
	Env            string `json:"env,omitempty" jsonschema:"environment (default: current)"`
	DryRun         bool   `json:"dry_run,omitempty" jsonschema:"show what would be written without writing"`
	MoveDuplicates bool   `json:"move_duplicates,omitempty" jsonschema:"first move plain Mods/ folders holding another version of a mod RimArcho links into the lock library (the refusal names them; show the dry run first)"`
}

type RuleIn struct {
	Env    string `json:"env,omitempty" jsonschema:"environment (default: current)"`
	Action string `json:"action,omitempty" jsonschema:"add (default), list or remove"`
	Rule   string `json:"rule,omitempty" jsonschema:"remove: the rule ref r:<n> from action=list"`
	Kind   string `json:"kind,omitempty" jsonschema:"add: after, before, incompatible, attach (A B), top, bottom (A) or ignore (target)"`
	A      string `json:"a,omitempty" jsonschema:"the mod the rule is about"`
	B      string `json:"b,omitempty" jsonschema:"the other mod"`
	Target string `json:"target,omitempty" jsonschema:"for ignore: the constraint ref printed by sort/problems, e.g. decl:#ab12:loadafter:pkg.id"`
	Note   string `json:"note,omitempty"`
}

type PinIn struct {
	Env           string `json:"env,omitempty" jsonschema:"environment (default: current)"`
	Ref           string `json:"ref" jsonschema:"the mod to lock (#id, pkg:…); with update: the locked copy or its upstream"`
	Name          string `json:"name,omitempty" jsonschema:"folder name under Mods/ (default <folder>_local; update picks a free one)"`
	Git           bool   `json:"git,omitempty" jsonschema:"make the copy a git repository with a baseline commit, so edits can be diffed"`
	Update        bool   `json:"update,omitempty" jsonschema:"remake the locked version from the upstream's current content in a new folder; the old copy stays"`
	MoveToLibrary bool   `json:"move_to_library,omitempty" jsonschema:"move a copy RimArcho made in Mods/ (before the lock library) into the library"`
	Reason        string `json:"reason,omitempty" jsonschema:"why it is locked (e.g. the problem it avoids): kept in the copy's note"`
	DryRun        bool   `json:"dry_run,omitempty" jsonschema:"show what would be copied and swapped without changing anything"`
}

type UnpinIn struct {
	Env    string `json:"env,omitempty" jsonschema:"environment (default: current)"`
	Ref    string `json:"ref" jsonschema:"the locked copy or its upstream (#id, pkg:…)"`
	DryRun bool   `json:"dry_run,omitempty" jsonschema:"show what would change"`
}

// UninstallIn is preview_uninstall: the player's answers, as archo uninstall's flags.
type UninstallIn struct {
	KeepLocked    bool   `json:"keep_locked,omitempty" jsonschema:"move the lock library's copies into a game's Mods/ as ordinary mods (default: recycle bin)"`
	LockedTo      string `json:"locked_to,omitempty" jsonschema:"the environment whose Mods/ gets them (default: the current one)"`
	RemovePacks   bool   `json:"remove_packs,omitempty" jsonschema:"packs to the recycle bin (default: kept)"`
	PacksTo       string `json:"packs_to,omitempty" jsonschema:"where kept packs inside the data folder go (default: Documents/RimArcho packs)"`
	RemoveSaves   bool   `json:"remove_saves,omitempty" jsonschema:"the environments' own save folders to the recycle bin (default: moved out)"`
	SavesTo       string `json:"saves_to,omitempty" jsonschema:"where they are moved (default: Documents/RimArcho saves)"`
	RemoveGames   bool   `json:"remove_games,omitempty" jsonschema:"game folders downloaded with SteamCMD to the recycle bin (default: kept)"`
	RestoreBackup bool   `json:"restore_modsconfig_backup,omitempty" jsonschema:"put ModsConfig.xml.rimarcho.bak back instead of cleaning the current list"`
	All           bool   `json:"all,omitempty" jsonschema:"no truncation"`
}

// UninstallArgv maps preview_uninstall: always the dry run, never --yes (14 §2).
func UninstallArgv(in UninstallIn) []string {
	return argv("uninstall", "--dry-run").flag("keep-locked", in.KeepLocked).str("locked-to", in.LockedTo).flag("remove-packs", in.RemovePacks).
		str("packs-to", in.PacksTo).flag("remove-saves", in.RemoveSaves).str("saves-to", in.SavesTo).flag("remove-games", in.RemoveGames).
		flag("restore-modsconfig-backup", in.RestoreBackup).flag("all", in.All).a
}

func gv(b *builder, mm string) []string { return b.str("game-version", mm).a }

func toolList() []tool {
	return []tool{
		reg("guide", true, false, descGuide, func(in GuideIn) []string { return argv("guide").pos(in.Topic).a }),
		reg("status", true, false, descStatus, func(in StatusIn) []string { return StatusArgv(in) }),
		reg("find_mods", true, false, descFind, func(in FindIn) []string {
			if in.Query != "" {
				return argv("find").pos(in.Query).flag("all", in.All).a
			}
			return argv("list").flag("active", in.Active).str("filter", in.Filter).flag("all", in.All).a
		}),
		reg("get_mod", true, false, descMod, func(in RefIn) []string { return []string{"mod", in.Ref} }),
		reg("get_mod_settings", true, false, descModSettings, func(in ModSettingsIn) []string {
			return argv("mod-settings").pos(in.Ref).flag("all", in.All).a
		}),
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
			return argv("changes").pos(in.Ref).flag("library", in.Library).str("since", in.Since).flag("all", in.All).a
		}),
		reg("analyze_log", true, false, descLog, func(in LogIn) []string {
			if in.Pattern != "" {
				return argv("pattern").pos(in.Pattern).a
			}
			return gv(argv("log").str("cluster", in.Cluster).str("text", in.Text).str("file", in.File).flag("prev", in.Prev).flag("all", in.All).str("since", in.Since), in.GameVersion)
		}),
		reg("search_files", true, false, descFiles, func(in FilesIn) []string {
			return FilesArgv(in)
		}),
		reg("read_source", true, false, descSource, func(in SourceIn) []string { return SourceArgv(in) }),
		reg("edit_loadout", false, false, descEdit, func(in EditIn) []string { return EditArgv(in) }),
		reg("save_loadout", false, true, descSave, func(in SaveIn) []string {
			return argv("save").flag("dry-run", in.DryRun).flag("move-duplicates", in.MoveDuplicates).a
		}),
		reg("set_player_data", false, false, descPlayer, func(in PlayerIn) []string { return PlayerArgv(in) }).allOnly(),
		reg("set_mod_settings", false, false, descModSettingsSet, func(in ModSettingsSetIn) []string { return ModSettingsSetArgv(in) }).allOnly(),
		reg("pin_mod", false, false, descPin, func(in PinIn) []string {
			return argv("pin", in.Ref).str("name", in.Name).flag("git", in.Git).flag("update", in.Update).flag("move-to-library", in.MoveToLibrary).str("reason", in.Reason).flag("dry-run", in.DryRun).a
		}),
		reg("preview_uninstall", true, false, descUninstall, UninstallArgv).allOnly(),
		reg("unpin_mod", false, false, descUnpin, func(in UnpinIn) []string { return argv("unpin", in.Ref).flag("dry-run", in.DryRun).a }),
	}
}

// Tools returns the whole tool table, every group.
func Tools() []tool {
	return append(append(append(append(append(append(toolList(), manageTools()...), planTools()...), libraryTools()...), saveTools()...), subscriptionTools()...), textureTools()...)
}

// ToolDoc describes one tool as a client sees it (name, description, parameters): for
// harnesses that drive the tools without an MCP client (spikes/agenteval).
type ToolDoc struct {
	Name, Description string
	Group             Group
	ReadOnly          bool
	Params            []ParamDoc
}

// ParamDoc is one argument: JSON name, JSON type, the schema help text.
type ParamDoc struct {
	Name, Type, Help string
	Required         bool
}

// Catalog lists every tool (both groups) with its parameters (the env parameter is left out).
func Catalog() []ToolDoc {
	var out []ToolDoc
	for _, t := range Tools() {
		d := ToolDoc{Name: t.name, Description: t.desc, Group: t.group, ReadOnly: t.readOnly}
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

// ArgvFor maps a tool call to any tool (both groups) (name and JSON arguments) to the CLI argv the server would run.
// Unknown argument names are an error.
func ArgvFor(name string, args []byte) ([]string, error) {
	for _, t := range Tools() {
		if t.name == name {
			return t.argvOf(args)
		}
	}
	return nil, fmt.Errorf("unknown tool %q", name)
}

// ProblemsArgv maps list_problems: warning and above by default (05 §4.5).
func ProblemsArgv(in ProblemsIn) []string {
	if in.Ref != "" {
		return gv(argv("problems").str("ref", in.Ref).flag("all", in.All), in.GameVersion)
	}
	min := in.MinSeverity
	if min == "" && in.Severity == "" && !in.All && !in.IncludeMuted {
		min = "warning"
	}
	return gv(argv("problems").str("subject", in.Subject).str("code", in.Code).str("severity", in.Severity).str("min-severity", min).
		flag("all", in.All).flag("detail", in.Detail).flag("ignored", in.Ignored).flag("include-muted", in.IncludeMuted), in.GameVersion)
}

// SourceArgv maps read_source: a search gets a 10 s decompile budget by default (14 §1).
func SourceArgv(in SourceIn) []string {
	switch in.Mode {
	case "callers":
		b := argv("source", "callers", in.Type)
		for _, r := range in.In {
			b.str("in", r)
		}
		return b.flag("all", in.All).a
	case "diff":
		b := argv("source", "diff")
		for _, p := range in.DLLs {
			b.pos(p)
		}
		return b.flag("all", in.All).a
	}
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
			b.flag("implicated", true) // 05 §5.4
		}
		budget := in.Budget
		if budget == 0 {
			budget = 10
		}
		return b.flag("game", in.Game).flag("regex", in.Regex).flag("all", in.All).num("budget", budget).a
	}
	if in.Game {
		return argv("source", "game").str("type", in.Type).str("member", in.Member).flag("all", in.All).a
	}
	return argv("source").pos(in.Ref).str("type", in.Type).str("member", in.Member).flag("all", in.All).a
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
	case "ignore_external":
		return []string{"loadout", "ignore-external"}
	case "stage":
		return append([]string{"stage", "add"}, in.Refs...)
	case "unstage":
		return append([]string{"stage", "drop"}, in.Refs...)
	case "restore":
		// no refs: every staged mod, as the tool says (the CLI needs --all for that)
		b := argv(append([]string{"stage", "restore"}, in.Refs...)...).flag("all", len(in.Refs) == 0)
		if in.At > 0 {
			b.str("at", strconv.Itoa(in.At))
		}
		return b.str("before", in.Before).str("after", in.After).a
	case "checkout":
		return argv("loadout", "checkout").pos(in.Version).a
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
	case "copy_note":
		if in.Text == "" {
			return []string{"note", first, "--copy", "--clear"}
		}
		return []string{"note", first, in.Text, "--copy"}
	case "tag_rename":
		return argv(append([]string{"tag", "rename"}, append(in.Tags, in.Text)...)...).flag("dry-run", in.DryRun).a
	case "tag_merge":
		return argv(append([]string{"tag", "merge"}, in.Tags...)...).str("into", in.Text).flag("dry-run", in.DryRun).a
	case "tag_delete":
		return argv(append([]string{"tag", "delete"}, in.Tags...)...).flag("dry-run", in.DryRun).a
	case "tag_color":
		b := argv(append([]string{"tag", "color"}, in.Tags...)...)
		if in.Text == "" {
			return b.flag("clear", true).a
		}
		return b.pos(in.Text).a
	case "tag_order":
		return argv(append([]string{"tag", "order", first}, in.Tags...)...).a
	case "tag_note":
		b := argv(append([]string{"tag", "note"}, in.Tags...)...)
		if in.Text == "" {
			return b.flag("clear", true).a
		}
		return b.pos(in.Text).a
	}
	return []string{"set_player_data: unknown action " + fmt.Sprintf("%q", in.Action)}
}
