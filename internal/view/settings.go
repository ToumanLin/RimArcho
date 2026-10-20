package view

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
)

// The settings page (10 §7): every item is a value and the command that changes it, so the
// interfaces only show and run commands; nothing here has logic of its own.

// SettingItem is one row of the settings page.
type SettingItem struct {
	Label string `json:"label"`
	Value string `json:"value"`
	// Edit: "path", "text" or "choice" (Choices); the new value replaces {} in Argv. "" has
	// no editor, only Actions.
	Edit    string          `json:"edit,omitempty"`
	Choices []string        `json:"choices,omitempty"`
	Argv    []string        `json:"argv,omitempty"`
	Note    string          `json:"note,omitempty"`
	Actions []SettingAction `json:"actions,omitempty"`
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

// EnvPage is the environment page: the environments, and the current one's folders and
// launch (what belongs to an environment rather than to RimArcho, 10 §7).
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

// EnvGroups are the current environment's folders and launch settings.
func EnvGroups(a *app.App, st *app.LoadoutState) []SettingGroup {
	var out []SettingGroup
	if st != nil {
		e := st.Ix.Env
		g := SettingGroup{Title: T("folders")}
		g.Items = append(g.Items,
			SettingItem{Label: T("game folder"), Value: e.GameRoot, Edit: "path", Argv: []string{"env", "set", e.Name, "--game", "{}"},
				Note: T("RimArcho checks that the mods are still the same before it moves them over")},
			SettingItem{Label: T("user data folder"), Value: e.UserDataRoot, Edit: "path", Argv: []string{"env", "set", e.Name, "--user-data", "{}"}})
		for _, s := range st.Ix.Sources {
			switch string(s.Kind) {
			case "workshop":
				g.Items = append(g.Items, SettingItem{Label: T("Workshop folder"), Value: s.Root, Edit: "path", Argv: []string{"env", "set", e.Name, "--workshop", "{}"}})
			case "steamcmd", "extra":
				g.Items = append(g.Items, SettingItem{Label: Tf("%s folder", s.Label), Value: s.Root,
					Actions: []SettingAction{{Label: T("remove this source"), Argv: []string{"env", "set", e.Name, "--remove-source", s.Ref()},
						Confirm: T("RimArcho forgets this folder; the files stay where they are")}}})
			}
		}
		g.Items = append(g.Items,
			SettingItem{Label: T("add a SteamCMD folder"), Edit: "path", Argv: []string{"env", "set", e.Name, "--steamcmd", "{}"},
				Note: T("mods there can be enabled; saving links them into Mods/")},
			SettingItem{Label: T("add another mod folder"), Edit: "path", Argv: []string{"env", "set", e.Name, "--extra", "{}"}})
		out = append(out, g)
		out = append(out, SettingGroup{Title: T("launch"), Items: []SettingItem{
			{Label: T("start the game through"), Value: e.Launch, Edit: "choice", Choices: []string{"steam", "direct"}, Argv: []string{"env", "set", e.Name, "--launch", "{}"}},
			{Label: T("extra arguments"), Value: a.LaunchArgs(e.ID), Edit: "text", Argv: []string{"env", "set", e.Name, "--launch-args", "{}"},
				Note: T("for example -popupwindow")},
		}})
	}
	return out
}

// SettingsPage builds the settings page: what is RimArcho's, for every environment (the
// environment's own folders and launch are on the environment page).
func SettingsPage(a *app.App, st *app.LoadoutState) []SettingGroup {
	var out []SettingGroup
	lang := cmds.Setting(a, "ui.lang")
	out = append(out, SettingGroup{Title: T("interface"), Items: []SettingItem{
		{Label: T("language"), Value: lang, Edit: "choice", Choices: []string{"auto", "zh", "en"}, Argv: []string{"config", "set", "ui.lang", "{}"}},
		{Label: T("Workshop descriptions"), Value: cmds.Setting(a, "workshop.fetch"), Edit: "choice", Choices: []string{"on", "off"}, Argv: []string{"config", "set", "workshop.fetch", "{}"}},
		{Label: T("images in descriptions"), Value: cmds.Setting(a, "workshop.images"), Edit: "choice", Choices: []string{"on", "off"}, Argv: []string{"config", "set", "workshop.images", "{}"}},
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
	out = append(out, SettingGroup{Title: T("packs"), Items: []SettingItem{
		{Label: T("where packs are kept"), Value: cmds.Setting(a, "pack.dir"), Edit: "path", Argv: []string{"config", "set", "pack.dir", "{}"},
			Note: T("a full pack takes as much space as its mods")},
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
	case "env set", "config set", "db backup", "db vacuum", "cache clear", "data update":
		return nil
	}
	if argv[0] == "gc" {
		return nil
	}
	return fmt.Errorf("not a settings command: %s", strings.Join(argv, " "))
}
