package mcp

import "strings"

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

// ListSavesIn is list_saves: archo saves list, or with review archo saves review --read (12 §3):
// the review first reads the saves that lack a current full read, as get_save reads its one. env both selects the game and keeps only
// its saves, as the global --env does on the CLI.
type ListSavesIn struct {
	Env       string `json:"env,omitempty" jsonschema:"only this Game Config's saves"`
	Colony    string `json:"colony,omitempty" jsonschema:"only this colony"`
	Mod       string `json:"mod,omitempty" jsonschema:"only saves using this Mod (#id, pkg:, packageId or ws:), installed or not"`
	Attention bool   `json:"attention,omitempty" jsonschema:"only warnings or errors"`
	Review    bool   `json:"review,omitempty" jsonschema:"playthrough review"`
	All       bool   `json:"all,omitempty" jsonschema:"no truncation"`
}

// ListSavesArgv maps list_saves.
func ListSavesArgv(in ListSavesIn) []string {
	if in.Review {
		return argv("saves", "review").str("colony", in.Colony).str("mod", in.Mod).flag("read", true).flag("all", in.All).a
	}
	return argv("saves", "list").str("colony", in.Colony).str("mod", in.Mod).flag("attention", in.Attention).flag("all", in.All).a
}

// GetSaveIn is get_save: archo saves show.
type GetSaveIn struct {
	Env     string `json:"env,omitempty" jsonschema:"Game Config; default current"`
	Save    string `json:"save" jsonschema:"sv:ref, name or colony/save"`
	Mods    bool   `json:"mods,omitempty" jsonschema:"all Mods"`
	Play    bool   `json:"play,omitempty" jsonschema:"all colonists and events"`
	Content bool   `json:"content,omitempty" jsonschema:"content by Mod"`
	All     bool   `json:"all,omitempty" jsonschema:"no truncation"`
}

// GetSaveArgv maps get_save.
func GetSaveArgv(in GetSaveIn) []string {
	return argv("saves", "show", in.Save).flag("mods", in.Mods).flag("play", in.Play).flag("content", in.Content).flag("all", in.All).a
}

// ManageSavesIn is manage_saves.
type ManageSavesIn struct {
	Env          string  `json:"env,omitempty" jsonschema:"Game Config (default: current)"`
	Action       string  `json:"action" jsonschema:"see the description"`
	Saves        StrList `json:"saves" jsonschema:"sv:<hex>, file names or <colony>/<save>; fix, rename, copy take one"`
	Name         string  `json:"name,omitempty" jsonschema:"rename: the new name"`
	Mods         StrList `json:"mods,omitempty" jsonschema:"fix: only these packageIds"`
	DisableAdded bool    `json:"disable_added,omitempty" jsonschema:"fix: disable Mods the save lacks"`
	Order        bool    `json:"order,omitempty" jsonschema:"fix: use the save's order"`
	Launch       bool    `json:"launch,omitempty" jsonschema:"fix: then launch"`
	DryRun       bool    `json:"dry_run,omitempty" jsonschema:"preview only"`
	Confirm      bool    `json:"confirm,omitempty" jsonschema:"backup, trash: do it once the player agreed"`
	Token        string  `json:"token,omitempty" jsonschema:"the preview's token"`
}

// ManageSavesArgv maps manage_saves to saves fix / rename / copy / backup / trash. Backup
// and trash only preview unless confirm=true (--yes) and dry_run is not set (12 §4.4); the
// preview's token goes with the confirmation (--token), and trash refuses one without it,
// so what the player agreed to is what is recycled.
func ManageSavesArgv(in ManageSavesIn) []string {
	switch in.Action {
	case "fix":
		b := argv("saves", "fix").pos(first(in.Saves))
		for _, m := range in.Mods {
			b.str("mod", m)
		}
		return b.flag("disable-added", in.DisableAdded).flag("order", in.Order).flag("launch", in.Launch).flag("dry-run", in.DryRun).a
	case "rename", "copy", "copy_to_current":
		if in.DryRun {
			return []string{"manage_saves: " + in.Action + " has no dry run; it only changes a file name in the save's folder and never overwrites"}
		}
		b := argv("saves", strings.TrimSuffix(in.Action, "_to_current")).pos(first(in.Saves))
		if in.Action == "rename" {
			b.pos(in.Name)
		}
		return b.flag("to-current", in.Action == "copy_to_current").a
	case "choose", "choose_colony":
		b := argv("saves", "choose")
		if len(in.Saves) > 0 {
			b.pos(first(in.Saves))
		}
		return b.flag("colony", in.Action == "choose_colony").a
	case "unchoose":
		return argv("saves", "choose").flag("clear", true).a
	case "backup", "trash":
		yes := in.Confirm && !in.DryRun
		if yes && in.Action == "trash" && in.Token == "" {
			return []string{"manage_saves: trash with confirm=true needs the token of its preview: call it without confirm, show the player the files, then confirm with that token"}
		}
		b := argv(append([]string{"saves", in.Action}, in.Saves...)...).flag("yes", yes)
		if yes {
			b.str("token", in.Token)
		}
		return b.a
	}
	return unknown("manage_saves", in.Action)
}
