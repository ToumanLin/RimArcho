package mcp

// Mod 库存 (06 §2.9, 05 §1.2): the read-only inventory and its two actions that had no tool.
// Each call maps to one CLI argv, so the text is the CLI's (I1).

func libraryTools() []tool {
	return []tool{
		reg("list_library", true, false, descLibrary, func(in LibraryIn) []string { return LibraryArgv(in) }),
		// Moving a folder out of Mods/ changes the game's folder (a recycle-bin copy on another
		// volume), so the tool is marked destructive.
		reg("manage_library", false, true, descManageLibrary, func(in ManageLibraryIn) []string { return ManageLibraryArgv(in) }),
	}
}

// LibraryArgv maps list_library to archo library.
func LibraryArgv(in LibraryIn) []string {
	b := argv("library").str("search", in.Search)
	for _, f := range in.Include {
		b.str("include", f)
	}
	for _, f := range in.Exclude {
		b.str("exclude", f)
	}
	return b.str("sort", in.Sort).str("lineage", in.Lineage).flag("facets", in.Facets).flag("all", in.All).a
}

// ManageLibraryArgv maps manage_library to library move-to-locked / remove-from-plans and
// workshop unsubscribe / subscribe / state.
func ManageLibraryArgv(in ManageLibraryIn) []string {
	switch in.Action {
	case "move_to_locked":
		return argv("library", "move-to-locked").pos(first(in.Refs)).str("reason", in.Reason).flag("dry-run", in.DryRun).a
	case "remove_from_plans":
		return append([]string{"library", "remove-from-plans"}, in.Refs...)
	case "unsubscribe", "subscribe":
		return append(append([]string{"workshop", in.Action}, in.Refs...), argv().flag("dry-run", in.DryRun).a...)
	case "steam_state":
		return append([]string{"workshop", "state"}, in.Refs...)
	}
	return unknown("manage_library", in.Action)
}

type LibraryIn struct {
	Env     string   `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Search  string   `json:"search,omitempty" jsonschema:"words that must all appear in a name, package id, author, alias, tag, note, Workshop ID or folder"`
	Include []string `json:"include,omitempty" jsonschema:"key:value filters to keep (keys tag, source, version, state, used); values of one key are alternatives, different keys must all hold"`
	Exclude []string `json:"exclude,omitempty" jsonschema:"key:value filters to drop"`
	Sort    string   `json:"sort,omitempty" jsonschema:"name (default), size, state, source, used, updated, version, author, tags, changed or played; a leading - reverses"`
	Lineage string   `json:"lineage,omitempty" jsonschema:"one mod (L#…, #…, pkg:…, ws:…): every copy with path, states, plans and last launch"`
	Facets  bool     `json:"facets,omitempty" jsonschema:"list each filter's values with counts instead of the mods"`
	All     bool     `json:"all,omitempty" jsonschema:"do not truncate the list"`
}

type ManageLibraryIn struct {
	Env    string   `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
	Action string   `json:"action" jsonschema:"move_to_locked, remove_from_plans, unsubscribe, subscribe or steam_state"`
	Refs   []string `json:"refs" jsonschema:"the instances (#id); move_to_locked takes one"`
	Reason string   `json:"reason,omitempty" jsonschema:"move_to_locked: why, kept in the copy's note"`
	DryRun bool     `json:"dry_run,omitempty" jsonschema:"move_to_locked, unsubscribe, subscribe: show what would happen, change nothing"`
}

const descLibrary = `The mod library ("Mod 库"): every mod on this machine for the environment, one line per lineage (the same mod) with its copies, total size, its most important state and the plans using it. States: unloadable, subscribed but not on disk, ignored by the game (another copy in Mods/ wins), unsubscribed leftover, hidden by another manager, upstream updated (locked version). "Used in" counts every plan of every environment of the same game and every pack. Filter with search (all words), include / exclude key:value (keys: tag, source = workshop|local|locked|steamcmd|extra|monitor, version, state = unloadable|missing|ignored|residue|hidden|updated, used = plan:<name>|pack:<name>|none, updated = week|month|half-year|older); facets=true lists the values with counts; lineage=<ref> shows every copy with its path, states, plans and last launch, and the commands for its actions. Use it to answer "what do I have, where, how big, used where", and to find cleanup candidates; use find_mods for the current plan's view and get_mod for one instance's facts.
Example: list_library {"include": ["used:none"], "exclude": ["source:local"]}
  library: 4 of 1352 mods match (used:none, -source:local)
    L#a6o7  Gone   [workshop, 533 B, unsubscribed leftover]
    L#u2mx  Idle   [workshop, 533 B]`

const descManageLibrary = `Mod library actions. action=move_to_locked refs=[<#id>] moves a spare copy of a mod out of the game's Mods/ folder (a copy made by hand or left by another manager, beside another copy of the same mod) into RimArcho's lock library: renamed on the same volume; on another volume copied, every file checked, then the original goes to the recycle bin; refused when the folder has links pointing outside it. The instance stays the same, so every plan keeps it (the game gets it through an _Archo_ link on the next save_loadout or launch_game); .rimarcho/origin.json is written only when it has none. action=remove_from_plans refs=[…] takes the mods out of every plan that has them (every environment of this game), one undoable step in each plan; nothing is written to the game. action=unsubscribe refs=[…] unsubscribes Workshop items through the running Steam client (a short helper process using steam_api64.dll copied from the game install), and Steam then deletes their folders: refused while a mod is enabled, used by any plan, or the game runs. action=subscribe refs=[…] subscribes again to unsubscribed leftovers; Steam downloads them in the background. action=steam_state refs=[…] asks Steam whether items are subscribed, installed or waiting for an update (changes nothing). When Steam is not running or the DLL cannot be loaded the call fails with "Steam is not available" and the Workshop page links: give them to the player instead. Marked destructive because move_to_locked moves a folder of the game's Mods/ and unsubscribe makes Steam delete folders: call them with dry_run=true first, show the player and ask. Other actions are other tools: enable (edit_loadout), pin / pin --update / unpin (pin_mod, unpin_mod), unhide and trash (manage_hidden).
Example: manage_library {"action": "move_to_locked", "refs": ["#iqmu"], "dry_run": true}
  #iqmu Lockable dry run, nothing changed: would move to the lock library
    from: D:\Steam\steamapps\common\RimWorld\Mods\Lockable Copy
    to: C:\Users\me\AppData\Local\RimArcho\locked\main\Lockable Copy`
