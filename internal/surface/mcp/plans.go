package mcp

import "strings"

// The 0.9 tools for plans (05 §1.2): starting the game with the current plan, managing the
// environment's plans, and reading a plan's history. Each one still maps to exactly one CLI
// argv. Going back to a version is edit_loadout action=checkout (an undoable edit).

func planTools() []tool {
	return []tool{
		reg("launch_game", false, true, descLaunch, func(in LaunchIn) []string {
			return argv("launch").flag("dry-run", in.DryRun).flag("move-duplicates", in.MoveDuplicates).a
		}),
		// Removing a plan deletes its history and cannot be undone (04 §1.3), so the whole
		// tool is marked destructive, like resolve_identity.
		reg("manage_plans", false, true, descPlans, func(in PlansIn) []string { return PlansArgv(in) }),
		reg("plan_history", true, false, descHistory, func(in HistoryIn) []string { return HistoryArgv(in) }),
	}
}

// PlansArgv maps manage_plans to plan list/new/use/rename/remove and loadout import --new.
func PlansArgv(in PlansIn) []string {
	switch in.Action {
	case "", "list":
		return []string{"plan", "list"}
	case "new":
		return argv("plan", "new").pos(in.Name).str("from", in.From).a
	case "use":
		return argv("plan", "use").pos(in.Name).a
	case "rename":
		return argv("plan", "rename").pos(in.Name).pos(in.NewName).a
	case "remove":
		// The player confirms in the agent's harness (destructiveHint); the CLI's --yes is
		// that confirmation.
		return argv("plan", "remove").pos(in.Name).flag("yes", true).a
	case "import":
		return argv("loadout", "import").pos(in.From).pos(in.Name).flag("new", true).a
	case "versions":
		b := argv("plan", "versions").pos(in.Name)
		for _, v := range strings.Fields(strings.ReplaceAll(in.Versions, ",", " ")) {
			b.pos(v)
		}
		return b.a
	case "follow":
		return argv("plan", "follow").pos(in.Name).pos(in.Collection).str("mode", in.Mode).a
	case "unfollow":
		return argv("plan", "unfollow").pos(in.Name).pos(in.Collection).a
	case "sync":
		return argv("plan", "sync").pos(in.Name).pos(in.Collection).flag("check", in.Check).flag("dry-run", in.DryRun).a
	}
	return unknown("manage_plans", in.Action)
}

// HistoryArgv maps plan_history to loadout log, loadout diff and history.
func HistoryArgv(in HistoryIn) []string {
	switch in.Action {
	case "", "log":
		return argv("loadout", "log").flag("all", in.All).a
	case "diff":
		return argv("loadout", "diff").pos(in.From).pos(in.To).flag("all", in.All).a
	case "steps":
		return argv("history").flag("all", in.All).a
	}
	return unknown("plan_history", in.Action)
}

// ---- tool arguments ----

type LaunchIn struct {
	Env            string `json:"env,omitempty" jsonschema:"environment name (default: current)"`
	DryRun         bool   `json:"dry_run,omitempty" jsonschema:"show what would be written and the command line; write nothing, start nothing"`
	MoveDuplicates bool   `json:"move_duplicates,omitempty" jsonschema:"first move plain Mods/ folders holding another version of a mod RimArcho links into the lock library (the refusal names them; show the dry run first)"`
}

type PlansIn struct {
	Env     string `json:"env,omitempty" jsonschema:"environment name (default: current)"`
	Action  string `json:"action,omitempty" jsonschema:"list (default), new, use, rename, remove, import, versions, follow, unfollow or sync"`
	Name    string `json:"name,omitempty" jsonschema:"the plan (new, use, rename, remove); import: the new plan's name (default: the list's own name or the file name)"`
	NewName string `json:"new_name,omitempty" jsonschema:"rename: the new name"`
	From    string `json:"from,omitempty" jsonschema:"new: current, v<n> (a version of the current plan) or a list file (default: an empty list); import: the list file or RimCrow share code"`
	// Versions sets a plan's game versions (04 §1.6).
	Versions string `json:"versions,omitempty" jsonschema:"versions: the game versions to set, e.g. 1.5-1.7 or 1.6 1.7, or default (needs name; leave empty to show them)"`
	// Following a Steam Workshop collection (04 §9.6).
	Collection string `json:"collection,omitempty" jsonschema:"follow, unfollow, sync: the Steam Workshop collection link or ID (sync: optional when the plan follows one)"`
	Mode       string `json:"mode,omitempty" jsonschema:"follow: add (default: catching up enables the new mods) or follow (also disables the mods the collection dropped)"`
	Check      bool   `json:"check,omitempty" jsonschema:"sync: only read the collections and list what changed"`
	DryRun     bool   `json:"dry_run,omitempty" jsonschema:"sync: show what catching up would enable and disable, change nothing"`
}

type HistoryIn struct {
	Env    string `json:"env,omitempty" jsonschema:"environment name (default: current)"`
	Action string `json:"action,omitempty" jsonschema:"log (default: the versions), diff, or steps (the undo/redo steps and the newest versions)"`
	From   string `json:"from,omitempty" jsonschema:"diff: the older side, v<n> or <n> (default: the newest version)"`
	To     string `json:"to,omitempty" jsonschema:"diff: the newer side, v<n> or workspace (default: the current plan's list)"`
	All    bool   `json:"all,omitempty" jsonschema:"no truncation (steps: every version)"`
}
