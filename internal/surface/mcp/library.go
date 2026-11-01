package mcp

// Mod library (06 §2.9, 05 §1.2): the read-only inventory and its two actions that had no tool.
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
// workshop unsubscribe / subscribe / resubscribe / state.
func ManageLibraryArgv(in ManageLibraryIn) []string {
	switch in.Action {
	case "move_to_locked":
		return argv("library", "move-to-locked").pos(first(in.Refs)).str("reason", in.Reason).flag("dry-run", in.DryRun).a
	case "remove_from_plans":
		return append([]string{"library", "remove-from-plans"}, in.Refs...)
	case "unsubscribe", "subscribe", "resubscribe":
		return append(append([]string{"workshop", in.Action}, in.Refs...), argv().flag("dry-run", in.DryRun).a...)
	case "steam_state":
		return append([]string{"workshop", "state"}, in.Refs...)
	}
	return unknown("manage_library", in.Action)
}

type LibraryIn struct {
	Env     string   `json:"env,omitempty" jsonschema:"environment name (default: current)"`
	Search  string   `json:"search,omitempty" jsonschema:"words that must all appear in a name, package id, author, alias, tag, note, Workshop ID or folder"`
	Include []string `json:"include,omitempty" jsonschema:"key:value filters to keep (keys lists, tag, tags, source, location, version, state, used, updated); lists is a case-insensitive list name and repeated lists require every list; location is a registered source folder path; other keys use alternatives within a key and all across keys"`
	Exclude []string `json:"exclude,omitempty" jsonschema:"key:value filters to drop"`
	Sort    string   `json:"sort,omitempty" jsonschema:"name (default), size, state, source, used, updated, version, author, tags, changed or played; a leading - reverses"`
	Lineage string   `json:"lineage,omitempty" jsonschema:"one mod (L#…, #…, pkg:…, ws:…): every copy with path, states, plans and last launch"`
	Facets  bool     `json:"facets,omitempty" jsonschema:"list each filter's values with counts instead of the mods"`
	All     bool     `json:"all,omitempty" jsonschema:"do not truncate the list"`
}

type ManageLibraryIn struct {
	Env    string   `json:"env,omitempty" jsonschema:"environment name (default: current)"`
	Action string   `json:"action" jsonschema:"move_to_locked, remove_from_plans, unsubscribe, subscribe, resubscribe or steam_state"`
	Refs   []string `json:"refs" jsonschema:"the instances (#id); move_to_locked takes one"`
	Reason string   `json:"reason,omitempty" jsonschema:"move_to_locked: why, kept in the copy's note"`
	DryRun bool     `json:"dry_run,omitempty" jsonschema:"move_to_locked, unsubscribe, subscribe, resubscribe: show what would happen, change nothing"`
}
