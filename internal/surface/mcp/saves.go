package mcp

// Saves (12 §1.6, §4; 14 §2): two read-only tools for agents diagnosing a save that will
// not load or asking about a playthrough, and the save actions for the player. Each call
// maps to one archo saves argv, so the text is the CLI's (I1).

func saveTools() []tool {
	return []tool{
		reg("list_saves", true, false, descListSaves, ListSavesArgv),
		reg("get_save", true, false, descGetSave, GetSaveArgv),
		// fix edits a plan, subscribes and can start the game; trash moves saves to the
		// recycle bin: destructive, and only the player's management group serves it.
		reg("manage_saves", false, true, descManageSaves, ManageSavesArgv).allOnly(),
	}
}

// ListSavesIn is list_saves: archo saves list. env both selects the game and keeps only
// its saves, as the global --env does on the CLI.
type ListSavesIn struct {
	Env       string `json:"env,omitempty" jsonschema:"environment name: only its saves (default: all)"`
	Colony    string `json:"colony,omitempty" jsonschema:"only this colony"`
	Mod       string `json:"mod,omitempty" jsonschema:"only saves using this Mod (#id, pkg:, packageId or ws:), installed or not"`
	Attention bool   `json:"attention,omitempty" jsonschema:"only saves with warnings or errors"`
	All       bool   `json:"all,omitempty" jsonschema:"no truncation"`
}

// ListSavesArgv maps list_saves.
func ListSavesArgv(in ListSavesIn) []string {
	return argv("saves", "list").str("colony", in.Colony).str("mod", in.Mod).flag("attention", in.Attention).flag("all", in.All).a
}

// GetSaveIn is get_save: archo saves show.
type GetSaveIn struct {
	Env  string `json:"env,omitempty" jsonschema:"environment name (default: current)"`
	Save string `json:"save" jsonschema:"sv:<hex>, file name or <colony>/<save>"`
	Mods bool   `json:"mods,omitempty" jsonschema:"every Mod, not only the differences"`
	Play bool   `json:"play,omitempty" jsonschema:"the whole playthrough: colonists, events"`
	All  bool   `json:"all,omitempty" jsonschema:"no truncation"`
}

// GetSaveArgv maps get_save.
func GetSaveArgv(in GetSaveIn) []string {
	return argv("saves", "show", in.Save).flag("mods", in.Mods).flag("play", in.Play).flag("all", in.All).a
}

// ManageSavesIn is manage_saves.
type ManageSavesIn struct {
	Env          string   `json:"env,omitempty" jsonschema:"environment name (default: current)"`
	Action       string   `json:"action" jsonschema:"fix, rename, copy, backup or trash"`
	Saves        []string `json:"saves" jsonschema:"sv:<hex>, file names or <colony>/<save>; fix, rename, copy take one"`
	Name         string   `json:"name,omitempty" jsonschema:"rename: the new name"`
	Mods         []string `json:"mods,omitempty" jsonschema:"fix: only these packageIds"`
	DisableAdded bool     `json:"disable_added,omitempty" jsonschema:"fix: disable the list's Mods the save lacks"`
	Order        bool     `json:"order,omitempty" jsonschema:"fix: use the save's order"`
	Launch       bool     `json:"launch,omitempty" jsonschema:"fix: then start the game with the list"`
	DryRun       bool     `json:"dry_run,omitempty" jsonschema:"fix, backup, trash: preview only"`
	Confirm      bool     `json:"confirm,omitempty" jsonschema:"backup, trash: do it, after the player agreed"`
}

// ManageSavesArgv maps manage_saves to saves fix / rename / copy / backup / trash. Backup
// and trash only preview unless confirm=true (--yes) and dry_run is not set (12 §4.4).
func ManageSavesArgv(in ManageSavesIn) []string {
	switch in.Action {
	case "fix":
		b := argv("saves", "fix").pos(first(in.Saves))
		for _, m := range in.Mods {
			b.str("mod", m)
		}
		return b.flag("disable-added", in.DisableAdded).flag("order", in.Order).flag("launch", in.Launch).flag("dry-run", in.DryRun).a
	case "rename", "copy":
		if in.DryRun {
			return []string{"manage_saves: " + in.Action + " has no dry run; it only changes a file name in the save's folder and never overwrites"}
		}
		b := argv("saves", in.Action).pos(first(in.Saves))
		if in.Action == "rename" {
			b.pos(in.Name)
		}
		return b.a
	case "backup", "trash":
		return append(append([]string{"saves", in.Action}, in.Saves...), argv().flag("yes", in.Confirm && !in.DryRun).a...)
	}
	return unknown("manage_saves", in.Action)
}
