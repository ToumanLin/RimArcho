package cmds

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/source"
)

var envFlags = []Flag{
	{Name: "game", Value: "<dir>", Help: "game install directory"},
	{Name: "saves", Value: "shared|own", Help: "shared: the game's default user data folder; own: a folder of its own (-savedatafolder)"},
	{Name: "user-data", Value: "<dir>", Help: "a user data folder of your choice (contains Config/ModsConfig.xml)"},
	{Name: "launch", Value: "direct|steam"},
	{Name: "workshop", Value: "<dir>", Help: "= archo folder add workshop <dir>"},
	{Name: "steamcmd", Value: "<dir>", Multi: true, Help: "= archo folder add steamcmd <dir>"},
	{Name: "extra", Value: "<dir>", Multi: true, Help: "= archo folder add extra <dir>"},
}

var launchArgsFlag = Flag{Name: "launch-args", Value: "<args>", Help: "extra game arguments (JSON string array keeps token boundaries); use --user-data for -savedatafolder (\"\" removes arguments)"}

var removeSourceFlag = Flag{Name: "remove-source", Value: "<src:label>", Multi: true, Help: "= archo folder remove <src:label>"}

func init() {
	register(&Command{Name: "env list", Summary: "list Game Configs (grouped by installation) and the shared mod folders", MaxArgs: 0, Run: envList})
	register(&Command{Name: "env detect", Summary: "detect game, user data and workshop paths (suggestion only)", MaxArgs: 0, Run: envDetect})
	register(&Command{Name: "env add", Args: "<name>", Summary: "add a Game Config with separate saves and settings", MinArgs: 1, MaxArgs: 1,
		Flags: append([]Flag{{Name: "list", Value: "<plan>", Help: "select an existing global plan"}, {Name: "new-list", Help: "create a Core and DLC plan named after the Game Config"}, {Name: "detected", Help: "use the paths env detect finds"}, {Name: "like", Value: "<config>", Help: "copy its game folder, launch and settings; select its current plan (defaults to own saves)"}}, append(envFlags, launchArgsFlag)...), Run: envAdd})
	register(&Command{Name: "env set", Args: "<name>", Summary: "change a Game Config's game folder, saves or launch (re-binds a moved game)", MinArgs: 1, MaxArgs: 1,
		Flags: append(append([]Flag{{Name: "name", Value: "<name>", Help: "rename only this Game Config; folders and launch history stay"}}, envFlags...), removeSourceFlag, launchArgsFlag), Run: envSet})
	register(&Command{Name: "env remove", Args: "<name>", Summary: "forget a Game Config (RimArcho data only; no game files)", MinArgs: 1, MaxArgs: 1,
		Flags: []Flag{{Name: "yes", Help: "remove (without it: the dry run)"}}, Run: envRemove})
	register(&Command{Name: "env use", Args: "<name>", Summary: "select the current Game Config", MinArgs: 1, MaxArgs: 1, Run: envUse})
	register(&Command{Name: "env rebind", Args: "<src:label> yes|no", Summary: "answer a pending source re-binding", MinArgs: 2, MaxArgs: 2, Run: envRebind})
}

func envRef(e *env.Environment) string { return "env:" + e.Name }

func versionText(v string) string {
	if v == "" {
		return "unknown version"
	}
	return v
}

func addSourceLines(n *render.Node, srcs []*source.Source) {
	var rows [][]string
	for _, s := range srcs {
		st := ""
		switch s.Status {
		case source.StatusOK:
		case source.StatusRebindPending:
			st = "[rebind-pending → " + s.PendingRoot + "]"
		default:
			st = "[" + s.Status + "]"
		}
		rows = append(rows, []string{s.Ref(), s.Root, st})
	}
	n.AddColumns(rows)
}

func envList(c *Ctx, a *Args) (*render.Doc, error) {
	envs, err := c.App.Store.Envs()
	if err != nil {
		return nil, err
	}
	cur, _ := c.App.Store.CurrentEnv()
	d, root := render.New(fmt.Sprintf("Game Configs: %d", len(envs)))
	groups := map[string]*render.Node{}
	for _, e := range envs {
		group := groups[e.GameID]
		if group == nil {
			g, err := c.App.Store.GameByID(e.GameID)
			if err != nil {
				return nil, err
			}
			// Version.txt, not the last scan's: Steam may have updated the game since (03 §1).
			group = root.Add(g.Name + "  " + versionText(app.LiveGameVersion(e)))
			groups[e.GameID] = group
			group.KV("game", e.GameRoot)
			srcs, err := c.App.Store.Sources(e.GameID)
			if err != nil {
				return nil, err
			}
			var own []*source.Source
			for _, s := range srcs {
				if s.EnvID == e.GameID {
					own = append(own, s)
				}
			}
			addSourceLines(group.Section("sources"), own)
		}
		line := envRef(e)
		if cur != nil && cur.ID == e.ID {
			line += "  [current]"
		}
		n := group.Add(line)
		n.KV("saves", c.App.Saves(e))
		n.KV("user-data", e.UserDataRoot)
		n.KV("launch", e.Launch)
		n.KV("build", buildKind(e.GameRoot))
		n.KV("player-log", c.App.EnvPlayerLog(e))
	}
	if fs, err := c.App.Folders(); err != nil {
		return nil, err
	} else if len(fs) > 0 {
		addSourceLines(root.Section("mod folders (every Game Config)"), fs)
	}
	if len(envs) == 0 {
		d.AddNext("archo env detect")
	} else if len(envs) > 1 {
		d.AddNext("archo env use <name>")
	}
	return d, nil
}

func envDetect(c *Ctx, a *Args) (*render.Doc, error) {
	det := c.App.Detected()
	d, root := render.New("detected Game Config (suggestion, not saved)")
	if det.GameRoot == "" {
		root.KV("game", "not found")
	} else {
		root.KV("game", det.GameRoot)
		root.KV("version", versionText(det.GameVersion))
		root.KV("launch", det.Launch)
		root.KV("build", buildKind(det.GameRoot))
		root.KV("steam-library", det.SteamLibrary)
	}
	if det.UserDataRoot != "" {
		root.KV("user-data", det.UserDataRoot)
	} else {
		root.KV("user-data", "not found")
	}
	if det.WorkshopRoot != "" {
		root.KV("workshop", det.WorkshopRoot)
	}
	if det.GameRoot == "" {
		t := root.Section("tried")
		for _, p := range det.Tried {
			t.Add(p)
		}
		t.Truncate(10, "")
		d.AddNext("archo env add <name> --game <dir> [--user-data <dir>] [--workshop <dir>]")
	} else {
		d.AddNext("archo env add <name> --detected")
	}
	for _, other := range env.DetectAll(c.App.Detect) {
		if env.PathKey(other.GameRoot) == env.PathKey(det.GameRoot) {
			continue
		}
		n := root.Section("another detected Game Config (suggestion, not saved)")
		n.KV("game", other.GameRoot)
		n.KV("version", versionText(other.GameVersion))
		n.KV("launch", other.Launch)
		n.KV("build", buildKind(other.GameRoot))
		n.KV("steam-library", other.SteamLibrary)
		n.KV("user-data", other.UserDataRoot)
		n.KV("workshop", other.WorkshopRoot)
		d.AddNext("archo env add <name> --game " + quoteArg(other.GameRoot))
	}
	return d, nil
}

// buildKind is "proton" for a Windows game Steam runs through Proton (03 §3.2), else ""
// (the line is left out).
func buildKind(gameRoot string) string {
	if gameRoot != "" && env.IsProtonGame(gameRoot) {
		return "proton"
	}
	return ""
}

func specFromArgs(a *Args) app.EnvSpec {
	return app.EnvSpec{Rename: a.Str("name"), Like: a.Str("like"), List: a.Str("list"), NewList: a.Bool("new-list"), GameRoot: a.Str("game"), UserDataRoot: a.Str("user-data"), Saves: a.Str("saves"), WorkshopRoot: a.Str("workshop"),
		Launch: a.Str("launch"), SteamCMD: a.List("steamcmd"), Extra: a.List("extra"), RemoveSources: a.List("remove-source")}
}

func envRemove(c *Ctx, a *Args) (*render.Doc, error) {
	if !a.Bool("yes") {
		r, err := c.App.EnvRemovePreview(a.Pos[0])
		if err != nil {
			return nil, err
		}
		d, root := render.New("env remove: dry run, nothing changed")
		envRemovalLines(root, r)
		d.AddNext("archo env remove " + quoteArg(r.Env.Name) + " --yes")
		return d, nil
	}
	r, err := c.App.EnvRemove(a.Pos[0])
	if err != nil {
		return nil, err
	}
	d, root := render.New(envRef(r.Env) + "  removed")
	envRemovalLines(root, r)
	if r.Next == "" && r.Current {
		d.AddNext("archo env add <name> --game <dir>")
	} else {
		d.AddNext("archo env list")
	}
	return d, nil
}

// envRemovalLines says what removing an environment forgets and what stays (16 §2.14).
func envRemovalLines(n *render.Node, r *app.EnvRemoval) {
	n.KV("forgets", fmt.Sprintf("%d launch records, %d mods read from its Data/ and Mods/, its history; plans are shared by every game and stay", r.Launches, r.Mods))
	n.KV("keeps", "game files, mods and saves; the shared mod folders and what you noted on mods")
	if r.Current {
		if r.Next != "" {
			n.KV("current", "env:"+r.Next)
		} else {
			n.KV("current", "none left")
		}
	}
}

func envAdd(c *Ctx, a *Args) (*render.Doc, error) {
	if err := app.ValidateLaunchArgs(a.Pos[0], a.Str("launch-args")); err != nil {
		return nil, err
	}
	spec := specFromArgs(a)
	if a.Bool("detected") {
		det := c.App.Detected()
		if det.GameRoot == "" && spec.GameRoot == "" {
			return nil, fmt.Errorf("detection found no game install; pass --game <dir>")
		}
		if spec.GameRoot == "" {
			spec.GameRoot = det.GameRoot
		}
		if spec.UserDataRoot == "" && spec.Saves == "" {
			spec.UserDataRoot = det.UserDataRoot
		}
		if spec.WorkshopRoot == "" {
			spec.WorkshopRoot = det.WorkshopRoot
		}
		if spec.Launch == "" {
			spec.Launch = det.Launch
		}
	}
	e, changes, err := c.App.EnvAdd(a.Pos[0], spec)
	if err != nil {
		return nil, err
	}
	if a.Bool("launch-args") {
		if err := c.App.SetLaunchArgs(e.ID, a.Str("launch-args")); err != nil {
			return nil, err
		}
	}
	d := envDoc(c, e, "added", changes)
	if spec.Like != "" && c.App.CloneArgsOmitted(spec.Like) {
		d.Nodes[0].Add("left -savedatafolder out of copied launch arguments; use --user-data to choose the data folder")
	}
	return d, nil
}

func envSet(c *Ctx, a *Args) (*render.Doc, error) {
	if err := app.ValidateLaunchArgs(a.Pos[0], a.Str("launch-args")); err != nil {
		return nil, err
	}
	// What changing the game folder does to other Game Configs and to the installation it
	// leaves (16 §2.14), read before the change.
	var gameChange []string
	if a.Str("game") != "" {
		lines, err := c.App.EnvGameChangePreview(a.Pos[0], a.Str("game"))
		if err != nil {
			return nil, err
		}
		gameChange = lines
	}
	e, changes, err := c.App.EnvSet(a.Pos[0], specFromArgs(a))
	if err != nil {
		return nil, err
	}
	if a.Bool("launch-args") {
		if err := c.App.SetLaunchArgs(e.ID, a.Str("launch-args")); err != nil {
			return nil, err
		}
	}
	d := envDoc(c, e, "updated", changes)
	if a.Str("game") != "" {
		es, err := c.App.Store.Envs()
		if err != nil {
			return nil, err
		}
		names := []string{}
		for _, o := range es {
			if o.GameID == e.GameID {
				names = append(names, envRef(o))
			}
		}
		d.Nodes[0].KV("installation Game Configs", strings.Join(names, ", "))
		for _, line := range gameChange {
			d.Nodes[0].Add(line)
		}
	}
	return d, nil
}

func envDoc(c *Ctx, e *env.Environment, verb string, changes []app.SourceChange) *render.Doc {
	line := envRef(e) + "  " + verb
	if cur, _ := c.App.Store.CurrentEnv(); cur != nil && cur.ID == e.ID {
		line += "  [current]"
	}
	d, root := render.New(line)
	root.KV("game", e.GameRoot)
	root.KV("version", versionText(e.GameVersion))
	root.KV("saves", c.App.Saves(e))
	if e.UserDataRoot != "" {
		root.KV("user-data", e.UserDataRoot)
	} else {
		root.KV("user-data", "not set (ModsConfig.xml and Player.log unavailable)")
	}
	launch := e.Launch
	if args := c.App.LaunchArgs(e.ID); args != "" {
		launch += "  " + args
	}
	root.KV("launch", launch)
	srcs := root.Section("sources")
	var pending []string
	for _, ch := range changes {
		s := ch.Source
		ref := s.Ref()
		if s.EnvID == "" {
			ref += " (shared)"
		}
		n := srcs.Add(fmt.Sprintf("%s  %s  %s", ref, ch.Action, s.Root))
		if ch.Action == "added" && s.Status == source.StatusMissing {
			n.Line += "  [missing]"
		}
		if r := ch.Rebind; r != nil {
			if ch.Action == "rebind-pending" {
				n.Line = fmt.Sprintf("%s  rebind-pending  %s", s.Ref(), s.PendingRoot)
				pending = append(pending, s.Ref())
			}
			n.KV("from", ch.OldRoot)
			n.KV("sample", fmt.Sprintf("%d/%d matched (%.0f%%, need %.0f%%)", r.Matched, len(r.Samples), r.Rate*100, source.RebindRate*100))
			if ch.Action == "rebind-pending" {
				var rows [][]string
				for i, smp := range r.Samples {
					if i == 5 {
						break
					}
					sim := "no old instance"
					if smp.Similarity >= 0 {
						sim = fmt.Sprintf("similarity %.2f", smp.Similarity)
					}
					rows = append(rows, []string{smp.Name, sim})
				}
				n.Section("examples").AddColumns(rows)
			}
		}
	}
	for _, p := range pending {
		d.AddNext("archo env rebind " + p + " yes|no")
	}
	d.AddNext("archo scan")
	return d
}

func envUse(c *Ctx, a *Args) (*render.Doc, error) {
	e, err := c.App.EnvUse(a.Pos[0])
	if err != nil {
		return nil, err
	}
	d, _ := render.New(envRef(e) + "  current")
	d.AddNext("archo status")
	return d, nil
}

func envRebind(c *Ctx, a *Args) (*render.Doc, error) {
	ans := strings.ToLower(a.Pos[1])
	if ans != "yes" && ans != "no" {
		return nil, &UsageError{Msg: "answer yes or no"}
	}
	e, err := c.App.Env()
	if err != nil {
		return nil, err
	}
	old, created, err := c.App.EnvRebind(e, a.Pos[0], ans == "yes")
	if err != nil {
		return nil, err
	}
	var d *render.Doc
	if created == nil {
		var root *render.Node
		d, root = render.New(old.Ref() + "  rebound")
		root.KV("root", old.Root)
	} else {
		var root *render.Node
		d, root = render.New(created.Ref() + "  new source")
		root.KV("root", created.Root)
		root.KV("retired", old.Ref()+" "+old.Root+" (its instances become tombstones on the next scan)")
	}
	d.AddNext("archo scan")
	return d, nil
}

// ---- a game version downloaded with SteamCMD (11 §3) ----

var steamcmdFlag = Flag{Name: "install-steamcmd", Help: "download SteamCMD from Valve first when it is not installed"}

func init() {
	register(&Command{Name: "env branches", Summary: "list RimWorld's Steam branches (versions) SteamCMD can download", MaxArgs: 0,
		Flags: []Flag{steamcmdFlag}, Run: envBranches})
	register(&Command{Name: "env download", Args: "<branch>", Summary: "download a RimWorld version with SteamCMD and add it as a Game Config (anonymous by default)",
		MinArgs: 1, MaxArgs: 1, Flags: []Flag{
			{Name: "to", Value: "<dir>", Help: "the new game folder"},
			{Name: "account", Value: "<name>", Help: "override steamcmd.account for this download; anonymous forces anonymous login (the default when no account is set)"},
			{Name: "name", Value: "<env>", Help: "the Game Config's name (default: the version, e.g. 1.6)"},
			{Name: "saves", Value: "shared|own"},
			steamcmdFlag,
		}, Run: envDownload})
}

// needSteamCMD installs SteamCMD when asked to, else explains how.
func needSteamCMD(c *Ctx, a *Args) error {
	if c.App.SteamCMDPath() != "" {
		return nil
	}
	if !a.Bool("install-steamcmd") {
		o, _ := c.App.OfferSteamCMD()
		size := "its size unknown"
		if o != nil && o.Bytes > 0 {
			size = render.Size(o.Bytes)
		}
		return &UsageError{Msg: fmt.Sprintf("SteamCMD is not installed. Add --install-steamcmd to download it from Valve (%s, %s) into %s", c.App.SteamCMDDownloadURL(), size, c.App.SteamCMDDir())}
	}
	_, err := c.App.InstallSteamCMD()
	return err
}

func envBranches(c *Ctx, a *Args) (*render.Doc, error) {
	if err := needSteamCMD(c, a); err != nil {
		return nil, err
	}
	bs, err := c.App.GameBranches()
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("RimWorld branches: %d", len(bs)))
	var rows [][]string
	for _, b := range bs {
		v := b.Version
		switch b.Name {
		case "public":
			v = "current"
		case "unstable":
			v = "test"
		}
		when := ""
		if !b.Updated.IsZero() {
			when = b.Updated.Format("2006-01-02")
		}
		rows = append(rows, []string{b.Name, v, b.Description, when})
	}
	root.AddColumns(rows)
	d.AddNext("archo env download <branch> --to <dir>")
	return d, nil
}

func envDownload(c *Ctx, a *Args) (*render.Doc, error) {
	if a.Str("to") == "" {
		return nil, &UsageError{Msg: "give the folder (--to)"}
	}
	if err := needSteamCMD(c, a); err != nil {
		return nil, err
	}
	account, err := c.App.SteamCMDAccount(a.Str("account"))
	if err != nil {
		return nil, err
	}
	r, err := c.App.DownloadGame(app.DownloadSpec{Branch: a.Pos[0], Dir: a.Str("to"), Account: account, Name: a.Str("name"), Saves: a.Str("saves")}, nil)
	if err != nil {
		return nil, err
	}
	return envDoc(c, r.Env, "downloaded and added", r.Changes), nil
}

func init() {
	register(&Command{Name: "env copy", Args: "<name>", Summary: "copy a Game Config's game folder (a backup before the game updates) and add the copy as a Game Config",
		MinArgs: 1, MaxArgs: 1, Flags: []Flag{
			{Name: "to", Value: "<dir>", Help: "the new game folder (absent or empty)"},
			{Name: "name", Value: "<config>", Help: "the new Game Config's name (default: <name>-copy)"},
			{Name: "mods", Help: "copy the game's Mods folder too (local Mods; RimArcho's links are never copied)"},
			{Name: "saves", Value: "shared|own", Help: "own: the copy keeps its saves and settings apart (default shared)"},
			{Name: "dry-run", Help: "say what would be copied and how much, write nothing"},
		}, Run: envCopy})
}

// envCopy copies a game folder (03 §3.5).
func envCopy(c *Ctx, a *Args) (*render.Doc, error) {
	if a.Str("to") == "" {
		return nil, &UsageError{Msg: "give the folder (--to)"}
	}
	saves := a.Str("saves")
	if saves != "" && saves != app.SavesShared && saves != app.SavesOwn {
		return nil, &UsageError{Msg: "--saves is shared or own"}
	}
	spec := app.CopyGameSpec{From: a.Pos[0], Dir: a.Str("to"), Name: a.Str("name"), Mods: a.Bool("mods"), OwnSaves: saves == app.SavesOwn}
	if a.Bool("dry-run") {
		p, err := c.App.PlanGameCopy(spec)
		if err != nil {
			return nil, err
		}
		d, root := render.New(fmt.Sprintf("env:%s  would be copied to %s as env:%s", p.From.Name, p.Dir, p.Name))
		root.KV("copies", fmt.Sprintf("%d bytes", p.Bytes))
		mods := "not copied"
		if spec.Mods {
			mods = "copied"
		}
		root.KV("Mods/", fmt.Sprintf("%d bytes, %s (RimArcho's links never are)", p.ModBytes, mods))
		if p.Free >= 0 {
			root.KV("free", fmt.Sprintf("%d bytes", p.Free))
		}
		d.AddNext(fmt.Sprintf("archo env copy %s --to %s", p.From.Name, strconv.Quote(p.Dir)))
		return d, nil
	}
	r, err := c.App.CopyGame(context.Background(), spec, nil)
	if err != nil {
		return nil, err
	}
	return envDoc(c, r.Env, "copied and added", r.Changes), nil
}
