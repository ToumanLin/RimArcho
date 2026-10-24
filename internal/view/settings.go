package view

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// The settings page (06 §2.14): every item is a value and the command that changes it, so the
// interfaces only show and run commands; nothing here has logic of its own.

// SettingItem is one row of the settings page.
type SettingItem struct {
	Label string `json:"label"`
	Value string `json:"value"`
	// Edit: "path", "text", "choice", "swatches" or "segments" (Choices). The new
	// value replaces {} in Argv; "" has no editor, only Actions.
	Edit string `json:"edit,omitempty"`
	// EditValue separates an editable path from its displayed status/version.
	EditValue *string  `json:"editValue,omitempty"`
	File      bool     `json:"file,omitempty"` // choose a program file rather than a directory
	Choices   []string `json:"choices,omitempty"`
	// ChoiceLabels are the words shown for Choices (same order); empty shows the values.
	ChoiceLabels []string        `json:"choiceLabels,omitempty"`
	Argv         []string        `json:"argv,omitempty"`
	MoveArgv     []string        `json:"moveArgv,omitempty"`
	Directory    string          `json:"directory,omitempty"`
	Note         string          `json:"note,omitempty"`
	Hover        string          `json:"hover,omitempty"`
	ValueNote    string          `json:"valueNote,omitempty"`
	Actions      []SettingAction `json:"actions,omitempty"`
}

// SettingAction is a button: a command, confirmed first when Confirm is set.
type SettingAction struct {
	Label   string   `json:"label"`
	Argv    []string `json:"argv"`
	Confirm string   `json:"confirm,omitempty"`
	Task    string   `json:"task,omitempty"`
	Pending string   `json:"pending,omitempty"`
}

// SettingTask identifies commands that queue after confirmation (06 §2.14, 06 §2.16).
func SettingTask(argv []string) string {
	if len(argv) == 3 && argv[0] == "deps" {
		if argv[1] == "remove" {
			return TaskDepRemove
		}
		if argv[1] == "install" {
			return map[string]string{deps.Todds: TaskTodds, deps.SteamCMD: TaskSteamCMD, deps.WezTerm: TaskWezTerm}[argv[2]]
		}
	}
	if len(argv) > 0 && argv[0] == "gc" {
		return TaskGC
	}
	if len(argv) < 2 {
		return ""
	}
	return map[string]string{
		"data update": TaskData, "cache clear": TaskCache,
		"db backup": TaskBackup, "db vacuum": TaskVacuum,
	}[strings.Join(argv[:2], " ")]
}

// TaskPending is the action's label while this kind runs or waits.
func TaskPending(kind string) string {
	switch kind {
	case TaskSubscribe:
		return T("subscribe")
	case TaskSubscriptionFrom:
		return T("updating…")
	case TaskSteamCMD, TaskTodds, TaskWezTerm:
		return T("downloading…")
	case TaskDepRemove:
		return T("removing…")
	case TaskData:
		return T("updating…")
	case TaskBackup:
		return T("backing up…")
	case TaskVacuum:
		return T("compacting…")
	default:
		return T("clearing…")
	}
}

// SettingGroup is a heading and its rows.
type SettingGroup struct {
	Title string `json:"title"`
	// Note is said once under the heading instead of on every row.
	Note  string        `json:"note,omitempty"`
	Items []SettingItem `json:"items"`
}

// EnvRow is an environment on the environment page.
type EnvRow struct {
	Name    string `json:"name"`
	Game    string `json:"game"` // "1.6.4633  D:\…\RimWorld"
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
			p.Envs = append(p.Envs, EnvRow{Name: e.Name, Game: e.GameVersion + "  " + e.GameRoot, Current: e.ID == cur})
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
			t += "  " + T("not there now")
		case source.StatusRebindPending:
			t += "  " + Tf("moving to %s waits for your answer", s.PendingRoot)
		}
		return t
	}
	var ws *source.Source
	var others []SettingItem
	for _, s := range fs {
		if s.Status == source.StatusRetired {
			continue
		}
		switch {
		case s.Kind == source.Workshop:
			ws = s
		case app.OwnFolder(s):
			continue // listed with relocation controls in DirectoryGroup
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
	g.Items = append(g.Items,
		SettingItem{Label: T("add a SteamCMD folder"), Edit: "path", Argv: []string{"folder", "add", "steamcmd", "{}"},
			Note: T("mods there can be enabled; writing to the game links them into Mods/")},
		SettingItem{Label: T("add another mod folder"), Edit: "path", Argv: []string{"folder", "add", "extra", "{}"}})
	return g
}

// SettingsPage starts with every managed directory. Game launch and save options
// also remain available on the environment page.
func SettingsPage(a *app.App, st *app.LoadoutState) []SettingGroup {
	folders := FolderGroup(a)
	// Existing folders have one row, with relocation controls, in DirectoryGroup.
	var add []SettingItem
	for _, item := range folders.Items {
		if item.Value == "" {
			add = append(add, item)
		}
	}
	folders.Items = add
	out := []SettingGroup{{Title: T("appearance"), Items: []SettingItem{
		{Label: T("accent colour"), Value: cmds.Setting(a, "ui.accent"), Edit: "swatches", Choices: cmds.SettingChoices("ui.accent"),
			ChoiceLabels: []string{T("default"), T("indigo"), T("blue"), T("cyan"), T("teal"), T("green"), T("grass"), T("orange"), T("amber"), T("red"), T("crimson"), T("pink"), T("plum"), T("purple"), T("violet"), T("iris")},
			Argv:         []string{"config", "set", "ui.accent", "{}"}},
		{Label: T("day/night theme"), Value: cmds.Setting(a, "ui.theme"), Edit: "segments", Choices: cmds.SettingChoices("ui.theme"),
			ChoiceLabels: []string{T("follow system"), T("light"), T("dark")}, Argv: []string{"config", "set", "ui.theme", "{}"}},
	}}, DirectoryGroup(a), folders, {Title: "SteamCMD", Items: []SettingItem{
		{Label: T("SteamCMD account (optional)"), Value: cmds.Setting(a, "steamcmd.account"), Edit: "text", Argv: []string{"config", "set", "steamcmd.account", "{}"},
			Note: T("empty or anonymous: anonymous login; enter an account name to log in. Password and Steam Guard are entered only in SteamCMD")},
	}}}
	out = append(out, DependencyGroup(a))
	lang := cmds.Setting(a, "ui.lang")
	gpuNote := ""
	if cmds.Setting(a, "ui.gpu") == "off" {
		gpuNote = T("takes effect next time you open RimArcho")
	}
	out = append(out, SettingGroup{Title: T("interface"), Items: []SettingItem{
		{Label: T("language"), Value: lang, Edit: "choice", Choices: []string{"auto", "zh", "en"}, Argv: []string{"config", "set", "ui.lang", "{}"}},
		{Label: T("Workshop descriptions"), Value: cmds.Setting(a, "workshop.fetch"), Edit: "choice", Choices: []string{"on", "off"}, Argv: []string{"config", "set", "workshop.fetch", "{}"}},
		{Label: T("images in descriptions"), Value: cmds.Setting(a, "workshop.images"), Edit: "choice", Choices: []string{"on", "off"}, Argv: []string{"config", "set", "workshop.images", "{}"}},
		{Label: T("GPU acceleration"), Value: cmds.Setting(a, "ui.gpu"), Edit: "choice", Choices: []string{"on", "off"}, Argv: []string{"config", "set", "ui.gpu", "{}"},
			Hover: T("uses about 20–35 MB less memory and draws in software"), ValueNote: gpuNote},
	}})
	data := SettingGroup{Title: T("community data")}
	status := a.DataStatus()
	for _, d := range app.Datasets {
		v := T("not downloaded")
		if di := status[d.Key]; di != nil {
			v = Tf("%d entries  %s", di.Entries, ShortDate(di.Fetched))
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
		caches.Items = append(caches.Items, SettingItem{Label: T(c.Desc), Value: Tf("%s  %d items", SizeText(c.Bytes), c.Files),
			Actions: []SettingAction{{Label: T("clear"), Argv: []string{"cache", "clear", c.Kind}}}})
	}
	out = append(out, caches)
	mon := SettingItem{Label: T("add Archo Monitor to lists"), Value: cmds.Setting(a, "monitor.default"), Edit: "choice", Choices: []string{"on", "off"},
		Argv: []string{"config", "set", "monitor.default", "{}"}, Hover: T("only new lists and lists not handled yet")}
	if s, err := a.MonitorState(); err == nil {
		v := T("not installed")
		if len(s.Installed) > 0 {
			v = s.Installed[0]
		}
		out = append(out, SettingGroup{Title: T("game log"), Items: []SettingItem{mon, {Label: T("Archo Monitor installed"), Value: v, Note: s.Dir}}})
	} else {
		out = append(out, SettingGroup{Title: T("game log"), Items: []SettingItem{mon}})
	}
	// Last: its own page with the plan (the interfaces open it; archo uninstall is not a
	// settings command).
	out = append(out, SettingGroup{Title: T("uninstall"), Items: []SettingItem{
		{Label: T("uninstall RimArcho"), Note: T("remove what RimArcho put on this machine and put back what it changed"),
			Actions: []SettingAction{{Label: T("uninstall RimArcho…"), Argv: []string{"uninstall"}}}},
	}})
	for gi := range out {
		for ii := range out[gi].Items {
			for ai := range out[gi].Items[ii].Actions {
				a := &out[gi].Items[ii].Actions[ai]
				a.Task = SettingTask(a.Argv)
				if a.Task != "" {
					a.Pending = TaskPending(a.Task)
				}
			}
		}
	}
	return out
}

func init() {
	Register(map[string]string{
		"downloading…": "正在下载…",
		"updating…":    "正在更新…",
		"backing up…":  "正在备份…",
		"compacting…":  "正在压缩…",
		"clearing…":    "正在清理…",
		"removing…":    "正在移除…",
	})
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
	case "env set", "env rebind", "folder add", "folder remove", "folder set", "folder move", "config set", "db backup", "db vacuum", "cache clear", "data update", "deps install", "deps remove":
		return nil
	}
	if argv[0] == "gc" {
		return nil
	}
	return fmt.Errorf("not a settings command: %s", strings.Join(argv, " "))
}
