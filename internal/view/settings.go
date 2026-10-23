package view

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// The settings page (06 §2.14): every item is a value and the command that changes it, so the
// interfaces only show and run commands; nothing here has logic of its own.

// SettingItem is one row of the settings page.
type SettingItem struct {
	Label string `json:"label"`
	Value string `json:"value"`
	// Edit: "path", "text" or "choice" (Choices); the new value replaces {} in Argv. "" has
	// no editor, only Actions.
	Edit    string   `json:"edit,omitempty"`
	Choices []string `json:"choices,omitempty"`
	// ChoiceLabels are the words shown for Choices (same order); empty shows the values.
	ChoiceLabels []string        `json:"choiceLabels,omitempty"`
	Argv         []string        `json:"argv,omitempty"`
	Note         string          `json:"note,omitempty"`
	Actions      []SettingAction `json:"actions,omitempty"`
}

// SettingAction is a button: a command, confirmed first when Confirm is set.
type SettingAction struct {
	Label   string   `json:"label"`
	Argv    []string `json:"argv"`
	Confirm string   `json:"confirm,omitempty"`
}

// SettingGroup is a heading and its rows.
type SettingGroup struct {
	Title string        `json:"title"`
	Items []SettingItem `json:"items"`
}

// EnvRow is an environment on the environment page.
type EnvRow struct {
	Name    string `json:"name"`
	Game    string `json:"game"` // "1.6.4633 · D:\…\RimWorld"
	Current bool   `json:"current"`
}

// EnvPage is the environment page: the environments, and the current one's game folder,
// version, launch and saves (what belongs to one game, 02 §0; the mod folders every game
// shares are on the settings page).
type EnvPage struct {
	Envs   []EnvRow       `json:"envs"`
	Groups []SettingGroup `json:"groups"`
}

// BuildEnvPage builds the environment page.
func BuildEnvPage(a *app.App, st *app.LoadoutState) *EnvPage {
	p := &EnvPage{Groups: EnvGroups(a, st)}
	cur := ""
	if st != nil {
		cur = st.Ix.Env.ID
	}
	if envs, err := a.Store.Envs(); err == nil {
		for _, e := range envs {
			p.Envs = append(p.Envs, EnvRow{Name: e.Name, Game: e.GameVersion + " · " + e.GameRoot, Current: e.ID == cur})
		}
	}
	return p
}

// EnvGroups are the current environment's game folder, version, launch and saves.
func EnvGroups(a *app.App, st *app.LoadoutState) []SettingGroup {
	if st == nil {
		return nil
	}
	e := st.Ix.Env
	version := e.GameVersion
	if version == "" {
		version = T("unknown")
	}
	saves := SettingItem{Label: T("saves"), Value: a.Saves(e), Edit: "choice", Choices: []string{app.SavesShared, app.SavesOwn},
		ChoiceLabels: []string{T("shared (the game's default folder)"), T("its own")}, Argv: []string{"env", "set", e.Name, "--saves", "{}"}, Note: e.UserDataRoot}
	if saves.Value == app.SavesCustom {
		saves.Choices = append([]string{app.SavesCustom}, saves.Choices...)
		saves.ChoiceLabels = append([]string{T("a folder you chose")}, saves.ChoiceLabels...)
	}
	return []SettingGroup{{Title: T("this game"), Items: []SettingItem{
		{Label: T("game folder"), Value: e.GameRoot, Edit: "path", Argv: []string{"env", "set", e.Name, "--game", "{}"},
			Note: T("RimArcho checks that the mods are still the same before it moves them over")},
		{Label: T("game version"), Value: version},
		{Label: T("start the game through"), Value: e.Launch, Edit: "choice", Choices: []string{"steam", "direct"},
			ChoiceLabels: []string{"Steam", T("the game's own exe")}, Argv: []string{"env", "set", e.Name, "--launch", "{}"}},
		saves,
		{Label: T("extra arguments"), Value: a.LaunchArgs(e.ID), Edit: "text", Argv: []string{"env", "set", e.Name, "--launch-args", "{}"},
			Note: T("for example -popupwindow")},
	}}}
}

// FolderGroup is the settings page's "Mod 文件夹" (02 §0): the folders every environment
// shares. The Workshop folder can be moved; SteamCMD and other folders added and removed;
// RimArcho's own folders (Archo Monitor, the lock library) are shown.
func FolderGroup(a *app.App) SettingGroup {
	g := SettingGroup{Title: T("mod folders")}
	fs, _ := a.Folders()
	counts, _ := a.Store.LiveCounts()
	note := func(s *source.Source) string {
		t := Tf("%d mods", counts[s.ID])
		switch s.Status {
		case source.StatusMissing:
			t += " · " + T("not there now")
		case source.StatusRebindPending:
			t += " · " + Tf("moving to %s waits for your answer", s.PendingRoot)
		}
		return t
	}
	var ws *source.Source
	var others, own []SettingItem
	for _, s := range fs {
		if s.Status == source.StatusRetired {
			continue
		}
		switch {
		case s.Kind == source.Workshop:
			ws = s
		case app.OwnFolder(s):
			label := T("Archo Monitor folder")
			if s.Label != app.MonitorLabel {
				label = T("lock library")
				if s.Label != app.LockLabel {
					label = Tf("lock library (%s, older)", strings.TrimPrefix(s.Label, app.LockLabel+"-"))
				}
			}
			own = append(own, SettingItem{Label: label, Value: s.Root, Note: note(s)})
		default:
			label := T("mod folder")
			if s.Kind == source.SteamCMD {
				label = T("SteamCMD folder")
			}
			others = append(others, SettingItem{Label: label, Value: s.Root, Note: note(s),
				Actions: []SettingAction{{Label: T("remove"), Argv: []string{"folder", "remove", s.Ref()},
					Confirm: T("RimArcho forgets this folder; the files stay where they are")}}})
		}
	}
	w := SettingItem{Label: T("Workshop folder"), Edit: "path", Argv: []string{"folder", "add", "workshop", "{}"}}
	if ws != nil {
		w.Value, w.Note = ws.Root, note(ws)
	} else if d := a.DetectWorkshop(); d != "" {
		w.Note = Tf("Steam has one at %s", d)
		w.Actions = []SettingAction{{Label: T("use it"), Argv: []string{"folder", "add", "workshop", d}}}
	} else {
		w.Note = T("none found in the Steam libraries")
	}
	g.Items = append(g.Items, w)
	g.Items = append(g.Items, others...)
	g.Items = append(g.Items, own...)
	g.Items = append(g.Items,
		SettingItem{Label: T("add a SteamCMD folder"), Edit: "path", Argv: []string{"folder", "add", "steamcmd", "{}"},
			Note: T("mods there can be enabled; saving links them into Mods/")},
		SettingItem{Label: T("add another mod folder"), Edit: "path", Argv: []string{"folder", "add", "extra", "{}"}})
	return g
}

// SettingsPage builds the settings page: what is RimArcho's, for every environment — the
// mod folders first (the game's own folder and launch are on the environment page).
func SettingsPage(a *app.App, st *app.LoadoutState) []SettingGroup {
	out := []SettingGroup{FolderGroup(a)}
	lang := cmds.Setting(a, "ui.lang")
	out = append(out, SettingGroup{Title: T("interface"), Items: []SettingItem{
		{Label: T("language"), Value: lang, Edit: "choice", Choices: []string{"auto", "zh", "en"}, Argv: []string{"config", "set", "ui.lang", "{}"}},
		{Label: T("Workshop descriptions"), Value: cmds.Setting(a, "workshop.fetch"), Edit: "choice", Choices: []string{"on", "off"}, Argv: []string{"config", "set", "workshop.fetch", "{}"}},
		{Label: T("images in descriptions"), Value: cmds.Setting(a, "workshop.images"), Edit: "choice", Choices: []string{"on", "off"}, Argv: []string{"config", "set", "workshop.images", "{}"}},
		{Label: T("GPU acceleration"), Value: cmds.Setting(a, "ui.gpu"), Edit: "choice", Choices: []string{"on", "off"}, Argv: []string{"config", "set", "ui.gpu", "{}"},
			Note: T("the window; off uses about 20–35 MB less memory and draws in software. Takes effect the next time RimArcho opens")},
	}})
	data := SettingGroup{Title: T("community data")}
	status := a.DataStatus()
	for _, d := range app.Datasets {
		v := T("not downloaded")
		if di := status[d.Key]; di != nil {
			v = Tf("%d entries · %s", di.Entries, ShortDate(di.Fetched))
		}
		data.Items = append(data.Items, SettingItem{Label: T(d.Desc), Value: a.DataSource(d), Edit: "text", Argv: []string{"config", "set", "data." + d.Key + ".url", "{}"},
			Note: v})
	}
	data.Items = append(data.Items, SettingItem{Label: T("update the community data"), Actions: []SettingAction{{Label: T("update now"), Argv: []string{"data", "update"},
		Confirm: T("RimArcho downloads the three files from their sources (about 50 MB)")}}})
	out = append(out, data)
	db, _ := a.DBInfo()
	out = append(out, SettingGroup{Title: T("database"), Items: []SettingItem{
		{Label: T("size"), Value: SizeText(db.Bytes), Note: db.Path, Actions: []SettingAction{
			{Label: T("back up"), Argv: []string{"db", "backup"}},
			{Label: T("compact"), Argv: []string{"db", "vacuum"}},
			{Label: T("remove old tombstones"), Argv: []string{"gc"}, Confirm: T("mods gone for longer than the tombstone time are forgotten")},
		}},
	}})
	caches := SettingGroup{Title: T("caches")}
	for _, c := range a.Caches() {
		caches.Items = append(caches.Items, SettingItem{Label: T(c.Desc), Value: Tf("%s · %d items", SizeText(c.Bytes), c.Files),
			Actions: []SettingAction{{Label: T("clear"), Argv: []string{"cache", "clear", c.Kind}}}})
	}
	out = append(out, caches)
	mon := SettingItem{Label: T("add Archo Monitor to plans"), Value: cmds.Setting(a, "monitor.default"), Edit: "choice", Choices: []string{"on", "off"},
		Argv: []string{"config", "set", "monitor.default", "{}"}, Note: T("new plans, and each plan once; after that it is a normal mod you can disable or move")}
	if s, err := a.MonitorState(); err == nil {
		v := T("not installed")
		if len(s.Installed) > 0 {
			v = s.Installed[0]
		}
		out = append(out, SettingGroup{Title: T("game log"), Items: []SettingItem{mon, {Label: T("Archo Monitor installed"), Value: v, Note: s.Dir}}})
	} else {
		out = append(out, SettingGroup{Title: T("game log"), Items: []SettingItem{mon}})
	}
	// 贴图优化 is a page under 库 (06 §2.1); here only where its encoder is (08 §5.2).
	tool := a.TextureTool()
	toolNote := T("the DDS encoder (todds) is not installed")
	if tool.Err == "" {
		toolNote = Tf("todds %s · %s", tool.Version, tool.Path)
	} else if tool.Path != "" {
		toolNote = tool.Err
	}
	out = append(out, SettingGroup{Title: T("texture optimisation"), Items: []SettingItem{
		{Label: T("the DDS encoder"), Value: cmds.Setting(a, "textures.todds"), Edit: "path", Argv: []string{"config", "set", "textures.todds", "{}"},
			Note: toolNote + " · " + T("empty: RimArcho's own copy; or a todds you have, e.g. RimSort's")},
	}})
	out = append(out, SettingGroup{Title: T("packs"), Items: []SettingItem{
		{Label: T("where packs are kept"), Value: cmds.Setting(a, "pack.dir"), Edit: "path", Argv: []string{"config", "set", "pack.dir", "{}"},
			Note: T("a full pack takes as much space as its mods")},
	}})
	// Last: its own page with the plan (the interfaces open it; archo uninstall is not a
	// settings command).
	out = append(out, SettingGroup{Title: T("uninstall"), Items: []SettingItem{
		{Label: T("uninstall RimArcho"), Note: T("remove what RimArcho put on this machine and put back what it changed"),
			Actions: []SettingAction{{Label: T("uninstall RimArcho…"), Argv: []string{"uninstall"}}}},
	}})
	return out
}

// SettingCommand checks that argv is one of the settings page's commands (the interfaces
// run nothing else from it).
func SettingCommand(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("no command")
	}
	two := argv[0]
	if len(argv) > 1 {
		two += " " + argv[1]
	}
	switch two {
	case "env set", "folder add", "folder remove", "config set", "db backup", "db vacuum", "cache clear", "data update":
		return nil
	}
	if argv[0] == "gc" {
		return nil
	}
	return fmt.Errorf("not a settings command: %s", strings.Join(argv, " "))
}
