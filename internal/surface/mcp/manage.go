package mcp

import "fmt"

// The tools for player actions that already had a CLI command but no tool (05 §1.2):
// packs, ignoring problems, identity answers, scan, list exchange, user rules, hidden mods
// and maintenance. Each one still maps to exactly one CLI argv.

// openWorld names the tools that may reach the network: the Workshop page (describe), a
// Workshop collection (exchange_list) and the community data download (maintenance).
var openWorld = map[string]bool{"get_description": true, "exchange_list": true, "maintenance": true}

func manageTools() []tool {
	return []tool{
		reg("manage_rules", false, false, descRules, func(in RuleIn) []string { return RulesArgv(in) }),
		reg("ignore_problem", false, false, descIgnore, func(in IgnoreIn) []string { return IgnoreArgv(in) }),
		reg("scan", false, false, descScan, func(in ScanIn) []string { return argv("scan").flag("full", in.Full).a }),
		reg("exchange_list", false, false, descExchange, func(in ExchangeIn) []string { return ExchangeArgv(in) }),
		reg("manage_packs", false, true, descPacks, func(in PackIn) []string { return PackArgv(in) }),
		reg("resolve_identity", false, true, descIdentity, func(in IdentityIn) []string { return IdentityArgv(in) }),
		reg("manage_hidden", false, true, descHidden, func(in HiddenIn) []string { return HiddenArgv(in) }),
		reg("maintenance", false, true, descMaintenance, func(in MaintenanceIn) []string { return MaintenanceArgv(in) }),
	}
}

func unknown(tool, action string) []string {
	return []string{tool + ": unknown action " + fmt.Sprintf("%q", action)}
}

// StatusArgv maps status: the overview, or one read-only page of RimArcho's own setup
// (environments, shared mod folders, settings, community data, database, caches). Changing them is left to the
// player (05 §1.2).
func StatusArgv(in StatusIn) []string {
	switch in.Show {
	case "", "overview":
		return []string{"status"}
	case "envs":
		return []string{"env", "list"}
	case "folders":
		return []string{"folder", "list"}
	case "settings":
		return []string{"config", "get"}
	case "data":
		return []string{"data", "status"}
	case "db":
		return []string{"db", "status"}
	case "cache":
		return []string{"cache", "status"}
	}
	return []string{"status: unknown show " + fmt.Sprintf("%q", in.Show)}
}

// RulesArgv maps manage_rules (and its old name add_rule): add by default.
func RulesArgv(in RuleIn) []string {
	switch in.Action {
	case "", "add":
		b := argv("rule", "add", in.Kind)
		if in.Kind == "ignore" {
			b.pos(in.Target)
		} else {
			b.pos(in.A).pos(in.B)
		}
		return b.str("note", in.Note).a
	case "list":
		return []string{"rule", "list"}
	case "remove":
		return argv("rule", "remove").pos(in.Rule).a
	}
	return unknown("manage_rules", in.Action)
}

// IgnoreArgv maps ignore_problem to problems ignore / unignore.
func IgnoreArgv(in IgnoreIn) []string {
	if in.Undo {
		return argv("problems", "unignore").pos(in.Ref).a
	}
	return argv("problems", "ignore").pos(in.Ref).a
}

// ExchangeArgv maps exchange_list to loadout export / import and collection.
func ExchangeArgv(in ExchangeIn) []string {
	switch in.Action {
	case "export":
		return argv("loadout", "export").str("format", in.Format).str("out", in.Out).a
	case "import":
		return argv("loadout", "import").pos(in.From).a
	case "collection":
		return argv("collection").pos(in.From).flag("import", in.Import).a
	}
	return unknown("exchange_list", in.Action)
}

// PackArgv maps manage_packs to one pack command.
func PackArgv(in PackIn) []string {
	switch in.Action {
	case "list":
		return []string{"pack", "list"}
	case "build":
		b := argv("pack", "build").pos(in.Name).flag("light", in.Light).flag("textures", in.Textures && in.TextureScale == 0)
		if in.TextureScale > 0 && in.TextureScale < 100 {
			b.flag("textures", true).str("texture-scale", fmt.Sprint(in.TextureScale))
		}
		return b.flag("dry-run", in.DryRun).a
	case "launch":
		return argv("pack", "launch").pos(in.Name).flag("dry-run", in.DryRun).flag("move-duplicates", in.MoveDuplicates).a
	case "export":
		return argv("pack", "export").pos(in.Name).pos(in.File).flag("light", in.Light).a
	case "import":
		return argv("pack", "import").pos(in.File).flag("as-plan", in.AsPlan).a
	case "remove":
		return argv("pack", "remove").pos(in.Name).flag("dry-run", in.DryRun).flag("unfinished", in.Unfinished).a
	case "stop":
		return argv("pack", "stop").pos(in.Name).a
	case "unlink":
		return argv("pack", "unlink").flag("dry-run", in.DryRun).a
	}
	return unknown("manage_packs", in.Action)
}

// IdentityArgv maps resolve_identity to identity list / resolve and lineage split / merge.
func IdentityArgv(in IdentityIn) []string {
	switch in.Action {
	case "list":
		return argv("identity", "list").flag("all", in.All).a
	case "resolve":
		return argv("identity", "resolve").pos(in.Question).pos(in.Answer).a
	case "split":
		return argv("lineage", "split").pos(first(in.Refs)).a
	case "merge":
		return argv(append([]string{"lineage", "merge"}, in.Refs...)...).a
	}
	return unknown("resolve_identity", in.Action)
}

// HiddenArgv maps manage_hidden to unhide / trash.
func HiddenArgv(in HiddenIn) []string {
	switch in.Action {
	case "unhide", "trash":
		return argv(in.Action).pos(in.Ref).flag("dry-run", in.DryRun).a
	}
	return unknown("manage_hidden", in.Action)
}

// MaintenanceArgv maps maintenance to data update, import rimcrow and gc.
func MaintenanceArgv(in MaintenanceIn) []string {
	switch in.Action {
	case "data_update":
		return argv(append([]string{"data", "update"}, in.Datasets...)...).flag("dry-run", in.DryRun).a
	case "import_rimcrow":
		return argv("import", "rimcrow").str("db", in.DB).str("rules", in.Rules).flag("dry-run", in.DryRun).
			flag("include-disabled-rules", in.IncludeDisabledRules).flag("all", in.All).a
	case "gc":
		return argv("gc").flag("dry-run", in.DryRun).a
	}
	return unknown("maintenance", in.Action)
}

func first(refs []string) string {
	if len(refs) > 0 {
		return refs[0]
	}
	return ""
}

// ---- tool arguments ----

type IgnoreIn struct {
	Env  string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Ref  string `json:"ref" jsonschema:"the problem: p:<hex> as printed by list_problems"`
	Undo bool   `json:"undo,omitempty" jsonschema:"stop ignoring it (problems unignore)"`
}

type ScanIn struct {
	Env  string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Full bool   `json:"full,omitempty" jsonschema:"re-hash every semantic file instead of trusting unchanged sizes and times"`
}

type ExchangeIn struct {
	Env    string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Action string `json:"action" jsonschema:"export, import or collection"`
	Format string `json:"format,omitempty" jsonschema:"export: text (default), modsconfig, rimsort, rml or rimcrow"`
	Out    string `json:"out,omitempty" jsonschema:"export: write to this new file (never overwrites); default: return the text"`
	From   string `json:"from,omitempty" jsonschema:"import: a list file path or a RimCrow share code; collection: a Workshop collection link or ID"`
	Import bool   `json:"import,omitempty" jsonschema:"collection: also replace the Loadout with the collection's installed mods in its order (undoable)"`
}

type PackIn struct {
	Env            string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Action         string `json:"action" jsonschema:"list, build, launch, export, import, remove, stop or unlink"`
	Name           string `json:"name,omitempty" jsonschema:"the pack (build, launch, export, remove, stop)"`
	File           string `json:"file,omitempty" jsonschema:"export: the new .archopack / .archopack.json file; import: the file to read"`
	Light          bool   `json:"light,omitempty" jsonschema:"build / export: the list only, no mod files"`
	Textures       bool   `json:"textures,omitempty" jsonschema:"build: convert the copies' PNG textures to DDS and delete the PNGs (pre-convert; lighter loading and video memory, a bigger pack at full size)"`
	TextureScale   int    `json:"texture_scale,omitempty" jsonschema:"build: also scale textures down to this percent (20 25 40 50 60 75 80): slim"`
	AsPlan         bool   `json:"as_plan,omitempty" jsonschema:"import of a light pack: also load its list into the Loadout (undoable)"`
	Unfinished     bool   `json:"unfinished,omitempty" jsonschema:"remove: delete what an interrupted build of name copied"`
	DryRun         bool   `json:"dry_run,omitempty" jsonschema:"build / launch / remove / unlink: show what would happen, change nothing"`
	MoveDuplicates bool   `json:"move_duplicates,omitempty" jsonschema:"launch: first move plain Mods/ folders holding another version of a pack's mod into the lock library (the refusal names them; show the dry run first)"`
}

type IdentityIn struct {
	Env      string   `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Action   string   `json:"action" jsonschema:"list, resolve, split or merge"`
	Question string   `json:"question,omitempty" jsonschema:"resolve: the question ref q:<n> from action=list"`
	Answer   string   `json:"answer,omitempty" jsonschema:"resolve: same (the same mod came back), copy (a copy of it) or unrelated"`
	Refs     []string `json:"refs,omitempty" jsonschema:"split: the instance to move into a lineage of its own; merge: two refs, the second lineage joins the first"`
	All      bool     `json:"all,omitempty" jsonschema:"list: include answered questions"`
}

type HiddenIn struct {
	Env    string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Action string `json:"action" jsonschema:"unhide or trash"`
	Ref    string `json:"ref" jsonschema:"the hidden or unsubscribed mod (#id, ws:…)"`
	DryRun bool   `json:"dry_run,omitempty" jsonschema:"show what would change, change nothing"`
}

type MaintenanceIn struct {
	Env                  string   `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Action               string   `json:"action" jsonschema:"data_update, import_rimcrow or gc"`
	Datasets             []string `json:"datasets,omitempty" jsonschema:"data_update: rules, steamdb, replacements (default: all)"`
	DB                   string   `json:"db,omitempty" jsonschema:"import_rimcrow: RimCrow's data\\mod_manager.db"`
	Rules                string   `json:"rules,omitempty" jsonschema:"import_rimcrow: RimCrow's user_rules.json (default: next to the database)"`
	IncludeDisabledRules bool     `json:"include_disabled_rules,omitempty" jsonschema:"import_rimcrow: also import rules switched off in RimCrow"`
	All                  bool     `json:"all,omitempty" jsonschema:"import_rimcrow: no truncation"`
	DryRun               bool     `json:"dry_run,omitempty" jsonschema:"report what would happen, change nothing"`
}
