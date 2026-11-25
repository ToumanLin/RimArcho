package view

import (
	"fmt"
	"github.com/ToumanLin/RimArcho/internal/env"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/netcfg"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// The settings page (16 §2.14): every item is a value and the command that changes it, so the
// interfaces only show and run commands; nothing here has logic of its own.

// SettingItem is one row of the settings page.
type SettingItem struct {
	Key   string `json:"key,omitempty"` // stable identity for rows without a setting command
	Label string `json:"label"`
	Value string `json:"value"`
	// Path markers identify filesystem text independently of editing/status text.
	ValuePath bool     `json:"valuePath,omitempty"`
	NotePaths []string `json:"notePaths,omitempty"`
	// Edit: "directory", "path", "url", "text", "choice", "swatches" or "segments". The new
	// value replaces {} in Argv; "" has no editor, only Actions.
	Edit string `json:"edit,omitempty"`
	// EditValue separates an editable path from its displayed status/version.
	EditValue *string  `json:"editValue,omitempty"`
	File      bool     `json:"file,omitempty"` // choose a program file rather than a directory
	Choices   []string `json:"choices,omitempty"`
	// ChoiceLabels are the words shown for Choices (same order); empty shows the values.
	ChoiceLabels []string `json:"choiceLabels,omitempty"`
	Argv         []string `json:"argv,omitempty"`
	MoveArgv     []string `json:"moveArgv,omitempty"`
	Directory    string   `json:"directory,omitempty"`
	Note         string   `json:"note,omitempty"`
	Hover        string   `json:"hover,omitempty"`
	ValueNote    string   `json:"valueNote,omitempty"`
	// Custom labels a last swatch that picks any #rrggbb colour, which Value may then be
	// (16 §2.14 accent colour); empty offers only Choices.
	Custom string `json:"custom,omitempty"`
	// URL: the value is a web page; the interfaces open it in the browser (Community, 16 §2.14).
	URL     string          `json:"url,omitempty"`
	Links   []CommunityLink `json:"links,omitempty"`
	Actions []SettingAction `json:"actions,omitempty"`
	// Disk: the folder's drive, free of total, for the GUI's meter (16 §7 principle 9)
	Disk *DiskMeter `json:"disk,omitempty"`
	// Rel: a Folders row's folder under its fold's root (FolderFold), shown in its place.
	Rel string `json:"rel,omitempty"`
	// Details fold under the row (16 §3.1 rule 7): RimArcho's release notes.
	Details *SettingDetails `json:"details,omitempty"`
}

// SettingDetails is text folded under a row: Summary opens it, Link is where it came from.
type SettingDetails struct {
	Summary string `json:"summary"`
	Text    string `json:"text"`
	Link    string `json:"link,omitempty"`
}

// SettingAction is a button: a command, confirmed first when Confirm is set.
type SettingAction struct {
	Input   *string  `json:"input,omitempty"`
	Label   string   `json:"label"`
	Argv    []string `json:"argv"`
	Confirm string   `json:"confirm,omitempty"`
	Task    string   `json:"task,omitempty"`
	Pending string   `json:"pending,omitempty"`
	// URL opens a web page instead of running Argv; Note sits beside the button and Copy is
	// what its copy icon copies (the manual RimArcho update's share and password, 17 §5.1).
	URL  string `json:"url,omitempty"`
	Note string `json:"note,omitempty"`
	Copy string `json:"copy,omitempty"`
	// Pick "zip": the player chooses a downloaded file, whose path replaces {} in Argv.
	Pick string `json:"pick,omitempty"`
}

// SettingTask identifies commands that queue after confirmation (16 §2.14, 16 §2.16).
func SettingTask(argv []string) string {
	if len(argv) > 0 && argv[0] == "update" {
		if len(argv) == 1 || len(argv) == 3 && argv[1] == "--from" && argv[2] != "" {
			return TaskSelfUpdate
		}
		if len(argv) == 2 && argv[1] == "--check" {
			return TaskSelfCheck
		}
		return ""
	}
	if len(argv) == 3 && argv[0] == "deps" {
		if argv[1] == "remove" {
			return TaskDepRemove
		}
		if argv[1] == "install" {
			return map[string]string{deps.Todds: TaskTodds, deps.SteamCMD: TaskSteamCMD}[argv[2]]
		}
	}
	if len(argv) > 0 && argv[0] == "gc" {
		return TaskGC
	}
	if len(argv) == 2 && argv[0] == "agent" && argv[1] == "install" && buildinfo.AgentOn() {
		return TaskAgentInstall
	}
	if len(argv) < 2 {
		return ""
	}
	return map[string]string{
		"source decompile": TaskSourceDecompile, "source clean": TaskSourceClean,
		"data update": TaskData, "cache clear": TaskCache,
		"db backup": TaskBackup, "db vacuum": TaskVacuum,
	}[strings.Join(argv[:2], " ")]
}

// TaskPending is the action's label while this kind runs or waits.
func TaskPending(kind string) string {
	switch kind {
	case TaskSourceDecompile:
		return T("decompiling…")
	case TaskSubscribe:
		return T("subscribe")
	case TaskSubscriptionFrom:
		return T("updating…")
	case TaskSteamCMD, TaskTodds:
		return T("downloading…")
	case TaskDepRemove:
		return T("removing…")
	case TaskData:
		return T("updating…")
	case TaskSelfUpdate, TaskAgentInstall:
		return T("installing…")
	case TaskSelfCheck:
		return T("checking…")
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
	// Folds group Folders' directory rows by root in the GUI (16 §2.14); rows in no fold
	// (adding a folder) follow them.
	Folds []FolderFold `json:"folds,omitempty"`
}

// EnvRow is an environment on the environment page.
type EnvRow struct {
	Installation string `json:"installation"`
	GameID       string `json:"gameID"`
	Name         string `json:"name"`
	Path         string `json:"path"`
	Game         string `json:"game"`            // "1.6.4633  D:\…\RimWorld"
	Build        string `json:"build,omitempty"` // "Proton" (ProtonBadge), also in Installation
	Current      bool   `json:"current"`
}

// EnvPage is the environment page: the environments, and the current one's game folder,
// version, launch and saves (what belongs to one game, 03 §2; the mod folders every game
// shares are on the settings page).
type EnvPage struct {
	Envs    []EnvRow       `json:"envs"`
	Groups  []SettingGroup `json:"groups"`
	Notices []string       `json:"notices,omitempty"`
}

// AliasNoticeText is the shared-game-folder notice while the player has not acknowledged it
// (app.AliasNotice), or "".
func AliasNoticeText(a *app.App) string {
	names := a.AliasNotice()
	if len(names) == 0 {
		return ""
	}
	return Tf("these two Game Configs use the same game folder: %s", strings.Join(names, ", "))
}

// BuildEnvPage builds the environment page.
func BuildEnvPage(a *app.App, st *app.LoadoutState) *EnvPage {
	p := &EnvPage{Groups: EnvGroups(a, st)}
	cur := ""
	if st != nil {
		cur = st.Ix.Env.ID
	}
	p.Envs = EnvironmentRows(a, cur)
	if text := AliasNoticeText(a); text != "" {
		p.Notices = []string{text}
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
	if b := ProtonBadge(e.GameRoot); b != "" {
		version += "  " + b
	}
	current := ""
	if p, _ := a.Store.CurrentPlan(e.ID); p != nil {
		current = p.Name
	}
	return []SettingGroup{{Title: T("Game Config"), Items: []SettingItem{
		{Key: "environment.name", Label: T("name"), Value: e.Name, Edit: "text", Argv: []string{"env", "set", e.Name, "--name", "{}"}},
		{Label: T("game folder"), Value: e.GameRoot, ValuePath: true, Note: version, Edit: "path", Argv: []string{"env", "set", e.Name, "--game", "{}"}},
		// shared / own / a folder the player chooses (custom: the page asks for the folder,
		// then runs env set --user-data); the folder itself is the note
		{Key: "environment.saves", Label: T("saves folder"), Value: a.Saves(e), Edit: "choice",
			Choices:      []string{app.SavesShared, app.SavesOwn, app.SavesCustom},
			ChoiceLabels: []string{T("shared (the game's default folder)"), T("own folder in RimArcho"), T("a folder you choose")},
			Argv:         []string{"env", "set", e.Name, "--saves", "{}"}, Note: e.UserDataRoot, NotePaths: []string{e.UserDataRoot}},
		{Key: "environment.list", Label: T("Mod list"), Value: current},
		{Label: T("start the game through"), Value: e.Launch, Edit: "choice", Choices: []string{"steam", "direct"}, ChoiceLabels: []string{"Steam", T("the game's own exe")}, Argv: []string{"env", "set", e.Name, "--launch", "{}"}},
		{Label: T("extra arguments"), Value: app.DisplayLaunchArgs(a.LaunchArgs(e.ID)), Edit: "text", Argv: []string{"env", "set", e.Name, "--launch-args", "{}"}},
		{Key: "environment.remove", Label: T("remove Game Config"), Actions: []SettingAction{{Label: T("remove Game Config"), Argv: []string{"env", "remove", e.Name}}}},
	}}}
}

// FolderGroup is the settings page's "Mod folders" (03 §2): the folders every environment
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
			others = append(others, SettingItem{Label: label, Value: s.Root, ValuePath: true, Note: note(s), NotePaths: []string{s.PendingRoot},
				Actions: []SettingAction{{Label: T("remove"), Argv: []string{"folder", "remove", s.Ref()},
					Confirm: T("RimArcho forgets this folder; the files stay where they are")}}})
		}
	}
	w := SettingItem{Label: T("Workshop folder"), Edit: "path", Argv: []string{"folder", "add", "workshop", "{}"}}
	if ws != nil {
		w.Value, w.Note = ws.Root, note(ws)
		w.ValuePath, w.NotePaths = true, []string{ws.PendingRoot}
	} else if d := a.DetectWorkshop(); d != "" {
		w.Note = Tf("Steam has one at %s", d)
		w.NotePaths = []string{d}
		w.Actions = []SettingAction{{Label: T("use it"), Argv: []string{"folder", "add", "workshop", d}}}
	} else {
		w.Note = T("none found in the Steam libraries")
	}
	g.Items = append(g.Items, w)
	g.Items = append(g.Items, others...)
	g.Items = append(g.Items,
		SettingItem{Label: T("add a SteamCMD folder"), Edit: "path", Argv: []string{"folder", "add", "steamcmd", "{}"},
			Note: T("mods there can be enabled; saving links them into Mods/")},
		SettingItem{Label: T("add another mod folder"), Edit: "path", Argv: []string{"folder", "add", "extra", "{}"}})
	return g
}

// SettingsPage is the settings page's groups (16 §2.14): About, Interface, Accounts, update,
// Folders, game log, network, external tools, community data, Storage and, last, uninstall. The GUI and
// the TUI show them in this order. Game launch and save options are on the environment page.
func SettingsPage(a *app.App, st *app.LoadoutState) []SettingGroup {
	gpuNote := ""
	if cmds.Setting(a, "ui.gpu") == "off" {
		gpuNote = T("takes effect next time you open RimArcho")
	}
	onOff := func(label, key string) SettingItem {
		return SettingItem{Label: label, Value: cmds.Setting(a, key), Edit: "choice", Choices: []string{"on", "off"}, ChoiceLabels: []string{T("on"), T("off")}, Argv: []string{"config", "set", key, "{}"}}
	}
	gpu := onOff(T("GPU acceleration"), "ui.gpu")
	gpu.Hover, gpu.ValueNote = T("uses about 20–35 MB less memory and draws in software"), gpuNote
	// Start page background closes this card in the GUI (it is not a setting command).
	out := []SettingGroup{AboutGroup(a), {Title: T("interface"), Items: []SettingItem{
		{Label: T("language"), Value: cmds.Setting(a, "ui.lang"), Edit: "choice", Choices: languageChoices(), ChoiceLabels: languageLabels(), Argv: []string{"config", "set", "ui.lang", "{}"}},
		{Label: T("day/night theme"), Value: cmds.Setting(a, "ui.theme"), Edit: "segments", Choices: cmds.SettingChoices("ui.theme"),
			ChoiceLabels: []string{T("follow system"), T("light"), T("dark")}, Argv: []string{"config", "set", "ui.theme", "{}"}},
		{Label: T("interface size"), Value: cmds.Setting(a, "ui.zoom"), Edit: "segments", Choices: cmds.SettingChoices("ui.zoom"),
			ChoiceLabels: []string{"100%", "110%", "125%", "150%"},
			Hover:        T("makes text and controls larger together; for 2K and 4K screens at 100% Windows scaling"),
			Argv:         []string{"config", "set", "ui.zoom", "{}"}},
		{Label: T("accent colour"), Value: cmds.Setting(a, "ui.accent"), Edit: "swatches", Choices: cmds.SettingChoices("ui.accent"),
			ChoiceLabels: []string{T("default"), T("indigo"), T("blue"), T("cyan"), T("teal"), T("green"), T("grass"), T("orange"), T("amber"), T("red"), T("crimson"), T("pink"), T("plum"), T("purple"), T("violet"), T("iris")},
			Custom:       T("custom…"),
			Argv:         []string{"config", "set", "ui.accent", "{}"}},
		onOff(T("Workshop descriptions"), "workshop.fetch"),
		onOff(T("images in descriptions"), "workshop.images"),
		gpu,
		{Label: T("Archo animation"), Value: cmds.Setting(a, "ui.archo.motion"), Edit: "segments",
			Choices: []string{"off", "still", "live"}, ChoiceLabels: []string{T("off"), T("still"), T("animated")},
			Hover: T("Archo's figure while pages load and in the first-run guide; still shows one picture per expression"),
			Argv:  []string{"config", "set", "ui.archo.motion", "{}"}},
		{Label: T("motion"), Value: cmds.Setting(a, "ui.motion"), Edit: "segments", Choices: cmds.SettingChoices("ui.motion"),
			ChoiceLabels: []string{T("follow system"), T("reduced")},
			Hover:        T("reduced keeps the window still: the sidebar, lists, pages and status line change without sliding or fading; follow system uses the Windows animation setting"),
			Argv:         []string{"config", "set", "ui.motion", "{}"}},
		// the first run, shown again without writing anything (16 §2.15)
		{Key: "guide.preview", Label: T("first-run guide"), Note: T("nothing you choose is saved"),
			Actions: []SettingAction{{Label: T("preview…"), Argv: []string{"guide-preview"}}}},
	}}, AccountGroup(a), AutomaticGroup(a)}

	// RimArcho's folders, each with relocation controls, then adding a mod folder.
	folders := DirectoryGroup(a)
	folders.Title = T("folders")
	for _, item := range FolderGroup(a).Items {
		if item.Value == "" {
			folders.Items = append(folders.Items, item)
		}
	}
	out = append(out, folders)

	// 16 §8.9: when the window checks a list's files by itself
	autoCheck := SettingItem{Label: T("check files automatically"), Value: cmds.Setting(a, "analysis.auto"), Edit: "segments", Choices: cmds.SettingChoices("analysis.auto"),
		ChoiceLabels: []string{T("after edits"), T("when saved"), T("off")},
		Hover:        T("Can it run checks the Mods' files (the patch simulation) by itself, at low priority. After edits: 2 seconds after you stop editing; when saved: when you save the list or open Can it run or Launch; off: only check again"),
		Argv:         []string{"config", "set", "analysis.auto", "{}"}}
	mon := SettingItem{Label: T("add Archo Monitor to lists"), Value: cmds.Setting(a, "monitor.default"), Edit: "choice", Choices: []string{"on", "off"},
		ChoiceLabels: []string{T("on"), T("off")}, Argv: []string{"config", "set", "monitor.default", "{}"}, Hover: T("new lists, and every sort")}
	// 12 §4.4: the newest save is copied before each launch
	saveCopy := onOff(T("back up the newest save at launch"), "saves.launch_copy")
	saveCopy.Hover = Tf("keeps the last %d copies in backups/saves; your own backups stay", app.LaunchCopyKeep)
	if s, err := a.MonitorState(); err == nil {
		v, path := T("not installed"), s.Dir
		if s.Newest != nil {
			v, path = s.Newest.Version, s.Newest.Mod.RealPath
			if s.Newest.Mod.Kind == source.Workshop {
				// 19 §6: the subscribed copy is used and the bundled one is not copied
				v = Tf("%s (Workshop copy)", v)
			}
		} else if len(s.Installed) > 0 {
			v = s.Installed[0]
		}
		out = append(out, SettingGroup{Title: T("game log"), Items: []SettingItem{mon, {Key: "monitor.installed", Label: T("Archo Monitor installed"), Value: v, Note: path, NotePaths: []string{path}}, saveCopy, autoCheck}})
	} else {
		out = append(out, SettingGroup{Title: T("game log"), Items: []SettingItem{mon, saveCopy, autoCheck}})
	}
	out = append(out, NetworkGroup(a))
	if g, ok := AgentGroup(a); ok {
		out = append(out, g)
	}
	out = append(out, DependencyGroup(a))

	data := SettingGroup{Title: T("community data")}
	status := a.DataStatus()
	checked := a.AutomaticStatus("data").Checked
	for _, info := range status {
		if info != nil {
			if info.Checked.After(checked) {
				checked = info.Checked
			}
			if info.Fetched.After(checked) {
				checked = info.Fetched
			}
		}
	}
	data.Note = lastChecked(a, checked)
	for _, d := range app.Datasets {
		v := T("not downloaded")
		if di := status[d.Key]; di != nil {
			v = Tf("%d entries  %s", di.Entries, ShortDate(di.Fetched))
		}
		data.Items = append(data.Items, SettingItem{Label: T(d.Desc), Value: a.DataSource(d), Edit: "url", Argv: []string{"config", "set", "data." + d.Key + ".url", "{}"},
			Note: v})
	}
	data.Items = append(data.Items, SettingItem{Label: T("update the community data"), Actions: []SettingAction{{Label: T("update now"), Argv: []string{"data", "update"},
		Confirm: T("RimArcho downloads the three files from their sources (about 50 MB)")}}})
	out = append(out, data)

	// Storage: the database and the caches, what takes room on the disk.
	db, _ := a.DBInfo()
	storage := SettingGroup{Title: T("storage"), Items: []SettingItem{
		{Label: T("database"), Value: SizeText(db.Bytes), Note: db.Path, NotePaths: []string{db.Path}, Actions: []SettingAction{
			{Label: T("back up"), Argv: []string{"db", "backup"}},
			{Label: T("compact"), Argv: []string{"db", "vacuum"}},
			{Label: T("remove old tombstones"), Argv: []string{"gc"}, Confirm: T("mods gone for longer than the tombstone time are forgotten")},
		}},
		{Key: "import.managers", Label: T("import from RimSort, RimCrow, RimPy or RimStack"), Actions: []SettingAction{{Label: T("import…"), Argv: []string{"import-managers"}}}},
	}}
	for _, c := range a.Caches() {
		storage.Items = append(storage.Items, SettingItem{Label: T(c.Desc), Value: Tf("%s  %d items", SizeText(c.Bytes), c.Files),
			Actions: []SettingAction{{Label: T("clear"), Argv: []string{"cache", "clear", c.Kind}}}})
	}
	out = append(out, storage)

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

// NetworkGroup: the proxy and the GitHub mirror for RimArcho's own downloads (17 §2.2),
// each with the route its last request actually took. The window's Workshop pages follow
// the Windows system proxy on their own.
func NetworkGroup(a *app.App) SettingGroup {
	proxy := cmds.Setting(a, "network.proxy")
	var now string
	if u, ok := netcfg.LastUsed(false); ok {
		now = Tf("last: %s", RouteText(u.Route))
	} else if via := netcfg.Describe("https://github.com/"); via == "direct" {
		now = Tf("now: %s", T("direct"))
	} else {
		now = Tf("now: %s", Tf("through %s", via))
	}
	mirror := cmds.Setting(a, "network.github_mirror")
	mv := mirror
	switch mirror {
	case "auto":
		mv = T("automatic")
	case "off":
		mv = T("off")
	}
	var mnow string
	if u, ok := netcfg.LastUsed(true); ok {
		mnow = Tf("last GitHub request: %s", RouteText(u.Route))
	}
	var hosts []string
	for _, m := range netcfg.AutoMirrors {
		hosts = append(hosts, netcfg.MirrorHost(m))
	}
	return SettingGroup{Title: T("network"), Items: []SettingItem{
		{Label: T("proxy"), Value: proxy, Edit: "text", Argv: []string{"config", "set", "network.proxy", "{}"},
			ValueNote: now,
			Hover:     T("system follows the Windows proxy (Clash, v2rayN) and goes direct when it is not running; Steam goes direct first, so game accelerators keep working; off goes direct only; or a proxy such as http://127.0.0.1:7890 or socks5://127.0.0.1:7891")},
		{Label: T("GitHub mirror"), Value: mv, EditValue: &mirror, Edit: "text", Argv: []string{"config", "set", "network.github_mirror", "{}"},
			ValueNote: mnow,
			Hover:     Tf("automatic tries GitHub first, then %s; off never uses a mirror; or one mirror tried first, such as https://gh-proxy.org/. RimArcho's own updates are checked against GitHub's checksum, never one from an automatic mirror", strings.Join(hosts, ", "))},
		{Key: "feedback.replies", Label: T("check for replies to my reports"), Value: cmds.Setting(a, "feedback.replies"), Edit: "choice", Choices: []string{"on", "off"}, ChoiceLabels: []string{T("on"), T("off")},
			Argv: []string{"config", "set", "feedback.replies", "{}"},
			Note: T("asks the feedback relay when RimArcho starts and when Feedback opens; nothing else is sent")},
	}}
}

// RouteText is a route as netcfg names it (direct, proxy host:port, mirror host) in the
// display language.
func RouteText(route string) string {
	switch {
	case route == "direct":
		return T("direct")
	case strings.HasPrefix(route, "proxy "):
		return Tf("through proxy %s", strings.TrimPrefix(route, "proxy "))
	case strings.HasPrefix(route, "mirror "):
		return Tf("through mirror %s", strings.TrimPrefix(route, "mirror "))
	}
	return route
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
	case "env set", "env rebind", "folder add", "folder remove", "folder set", "folder move", "config set", "db backup", "db vacuum", "cache clear", "data update", "deps install", "deps remove", "source clean", "source decompile":
		return nil
	}
	if argv[0] == "gc" {
		return nil
	}
	// Archo Agent (15 §7), in a build that offers it; the key has its own call (API.SetAgentKey),
	// never a command line
	if buildinfo.AgentOn() && (len(argv) == 2 && argv[0] == "agent" && (argv[1] == "install" || argv[1] == "remove") ||
		len(argv) == 3 && argv[0] == "agent" && argv[1] == "run" && argv[2] == "--window") {
		return nil
	}
	if argv[0] == "update" && (len(argv) == 1 || len(argv) == 2 && argv[1] == "--check" || len(argv) == 3 && argv[1] == "--from" && argv[2] != "") {
		return nil
	}
	return fmt.Errorf("not a settings command: %s", strings.Join(argv, " "))
}

// EnvironmentRows returns flat config metadata without loading any inventories.
func EnvironmentRows(a *app.App, current string) []EnvRow {
	es, err := a.Store.Envs()
	if err != nil {
		return nil
	}
	var out []EnvRow
	for _, e := range es {
		title := "RimWorld"
		if v := env.MajorMinor(e.GameVersion); v != "" {
			title += " " + v
		}
		build := ProtonBadge(e.GameRoot)
		if build != "" {
			title += "  " + build
		}
		out = append(out, EnvRow{Name: e.Name, GameID: e.GameID, Installation: title, Path: e.GameRoot, Game: e.GameVersion + "  " + e.GameRoot, Build: build, Current: e.ID == current})
	}
	return out
}

// ProtonBadge is "Proton" for a Windows game that Steam runs through Proton (the layout
// of 03 §3.2), else "": the badge the guide, the Game Config page and the switchers show
// beside an install (16 §2.8, §2.15). Only a Linux Steam library has that layout.
func ProtonBadge(gameRoot string) string {
	if gameRoot != "" && env.IsProtonGame(gameRoot) {
		return "Proton"
	}
	return ""
}
