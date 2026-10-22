package mcp

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
	Env            string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	DryRun         bool   `json:"dry_run,omitempty" jsonschema:"show what would be written and the command line; write nothing, start nothing"`
	MoveDuplicates bool   `json:"move_duplicates,omitempty" jsonschema:"first move plain Mods/ folders holding another version of a mod RimArcho links into the lock library (the refusal names them; show the dry run first)"`
}

type PlansIn struct {
	Env     string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Action  string `json:"action,omitempty" jsonschema:"list (default), new, use, rename, remove or import"`
	Name    string `json:"name,omitempty" jsonschema:"the plan (new, use, rename, remove); import: the new plan's name (default: the list's own name or the file name)"`
	NewName string `json:"new_name,omitempty" jsonschema:"rename: the new name"`
	From    string `json:"from,omitempty" jsonschema:"new: current, v<n> (a version of the current plan) or a list file (default: an empty list); import: the list file or RimCrow share code"`
}

type HistoryIn struct {
	Env    string `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Action string `json:"action,omitempty" jsonschema:"log (default: the versions), diff, or steps (the undo/redo steps and the newest versions)"`
	From   string `json:"from,omitempty" jsonschema:"diff: the older side, v<n> or <n> (default: the newest version)"`
	To     string `json:"to,omitempty" jsonschema:"diff: the newer side, v<n> or workspace (default: the current plan's list)"`
	All    bool   `json:"all,omitempty" jsonschema:"no truncation (steps: every version)"`
}
