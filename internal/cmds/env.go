package cmds

import (
	"fmt"
	"os"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/source"
)

var envFlags = []Flag{
	{Name: "game", Value: "<dir>", Help: "game install (contains RimWorldWin64.exe / RimWorld.exe)"},
	{Name: "saves", Value: "shared|own", Help: "shared: the game's default user data folder; own: a folder of its own (-savedatafolder)"},
	{Name: "user-data", Value: "<dir>", Help: "a user data folder of your choice (contains Config/ModsConfig.xml)"},
	{Name: "launch", Value: "direct|steam"},
	{Name: "workshop", Value: "<dir>", Help: "= archo folder add workshop <dir>"},
	{Name: "steamcmd", Value: "<dir>", Multi: true, Help: "= archo folder add steamcmd <dir>"},
	{Name: "extra", Value: "<dir>", Multi: true, Help: "= archo folder add extra <dir>"},
}

var removeSourceFlag = Flag{Name: "remove-source", Value: "<src:label>", Multi: true, Help: "= archo folder remove <src:label>"}

func init() {
	register(&Command{Name: "env list", Summary: "list environments (one per game folder) and the shared mod folders", MaxArgs: 0, Run: envList})
	register(&Command{Name: "env detect", Summary: "detect game, user data and workshop paths (suggestion only)", MaxArgs: 0, Run: envDetect})
	register(&Command{Name: "env add", Args: "<name>", Summary: "add an environment: one game folder", MinArgs: 1, MaxArgs: 1,
		Flags: append([]Flag{{Name: "detected", Help: "use the paths env detect finds"}}, envFlags...), Run: envAdd})
	register(&Command{Name: "env set", Args: "<name>", Summary: "change an environment's game folder, saves or launch (re-binds a moved game)", MinArgs: 1, MaxArgs: 1,
		Flags: append(append([]Flag(nil), envFlags...), removeSourceFlag, Flag{Name: "launch-args", Value: "<args>", Help: "extra game arguments, e.g. -popupwindow (\"\" removes them)"}), Run: envSet})
	register(&Command{Name: "env remove", Args: "<name>", Summary: "forget an environment (RimArcho data only; no game files)", MinArgs: 1, MaxArgs: 1,
		Flags: []Flag{{Name: "yes", Help: "confirm"}}, Run: envRemove})
	register(&Command{Name: "env use", Args: "<name>", Summary: "select the current environment", MinArgs: 1, MaxArgs: 1, Run: envUse})
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
	d, root := render.New(fmt.Sprintf("environments: %d", len(envs)))
	for _, e := range envs {
		line := envRef(e) + "  " + versionText(e.GameVersion)
		if cur != nil && cur.ID == e.ID {
			line += "  [current]"
		}
		n := root.Add(line)
		n.KV("game", e.GameRoot)
		n.KV("saves", c.App.Saves(e))
		n.KV("user-data", e.UserDataRoot)
		n.KV("launch", e.Launch)
		if e.UserDataRoot != "" {
			n.KV("player-log", env.PlayerLog(e.UserDataRoot, e.GameVersion))
		}
		srcs, err := c.App.Store.Sources(e.ID)
		if err != nil {
			return nil, err
		}
		var own []*source.Source
		for _, s := range srcs {
			if s.EnvID == e.ID {
				own = append(own, s)
			}
		}
		addSourceLines(n.Section("sources"), own)
	}
	if fs, err := c.App.Folders(); err != nil {
		return nil, err
	} else if len(fs) > 0 {
		addSourceLines(root.Section("mod folders (every environment)"), fs)
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
	d, root := render.New("detected environment (suggestion, not saved)")
	if det.GameRoot == "" {
		root.KV("game", "not found")
	} else {
		root.KV("game", det.GameRoot)
		root.KV("version", versionText(det.GameVersion))
		root.KV("launch", det.Launch)
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
	return d, nil
}

func specFromArgs(a *Args) app.EnvSpec {
	return app.EnvSpec{GameRoot: a.Str("game"), UserDataRoot: a.Str("user-data"), Saves: a.Str("saves"), WorkshopRoot: a.Str("workshop"),
		Launch: a.Str("launch"), SteamCMD: a.List("steamcmd"), Extra: a.List("extra"), RemoveSources: a.List("remove-source")}
}

func envRemove(c *Ctx, a *Args) (*render.Doc, error) {
	if !a.Bool("yes") {
		return nil, &UsageError{Msg: fmt.Sprintf("this forgets env:%s and its instance IDs, history and rules; add --yes to confirm", a.Pos[0])}
	}
	e, err := c.App.EnvRemove(a.Pos[0])
	if err != nil {
		return nil, err
	}
	d, root := render.New(envRef(e) + "  removed")
	root.KV("kept", "the shared mod folders and what you noted on mods; game and mod files are untouched")
	d.AddNext("archo env list")
	return d, nil
}

func envAdd(c *Ctx, a *Args) (*render.Doc, error) {
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
	return envDoc(c, e, "added", changes), nil
}

func envSet(c *Ctx, a *Args) (*render.Doc, error) {
	e, changes, err := c.App.EnvSet(a.Pos[0], specFromArgs(a))
	if err != nil {
		return nil, err
	}
	if a.Bool("launch-args") {
		if err := c.App.SetLaunchArgs(e.ID, a.Str("launch-args")); err != nil {
			return nil, err
		}
	}
	return envDoc(c, e, "updated", changes), nil
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

// ---- a game version downloaded with SteamCMD (08 §10) ----

var steamcmdFlag = Flag{Name: "install-steamcmd", Help: "download SteamCMD from Valve first when it is not installed"}

func init() {
	register(&Command{Name: "env branches", Summary: "list RimWorld's Steam branches (versions) SteamCMD can download", MaxArgs: 0,
		Flags: []Flag{steamcmdFlag}, Run: envBranches})
	register(&Command{Name: "env download", Args: "<branch>", Summary: "download a RimWorld version with SteamCMD and add it as an environment (you log in in SteamCMD)",
		MinArgs: 1, MaxArgs: 1, Flags: []Flag{
			{Name: "to", Value: "<dir>", Help: "the new game folder"},
			{Name: "account", Value: "<name>", Help: "your Steam account name (the password is typed in SteamCMD, never in RimArcho)"},
			{Name: "name", Value: "<env>", Help: "the environment's name (default: the version, e.g. 1.6)"},
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
		return &UsageError{Msg: fmt.Sprintf("SteamCMD is not installed. Add --install-steamcmd to download it from Valve (%s, %s) into %s", app.SteamCMDURL, size, c.App.SteamCMDDir())}
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
	d.AddNext("archo env download <branch> --to <dir> --account <name>")
	return d, nil
}

func envDownload(c *Ctx, a *Args) (*render.Doc, error) {
	if a.Str("to") == "" || a.Str("account") == "" {
		return nil, &UsageError{Msg: "give the folder (--to) and your Steam account name (--account)"}
	}
	if err := needSteamCMD(c, a); err != nil {
		return nil, err
	}
	fmt.Fprintln(os.Stderr, "SteamCMD asks for the password and the Steam Guard code below; RimArcho does not see them.")
	r, err := c.App.DownloadGame(app.DownloadSpec{Branch: a.Pos[0], Dir: a.Str("to"), Account: a.Str("account"), Name: a.Str("name"), Saves: a.Str("saves")}, nil)
	if err != nil {
		return nil, err
	}
	return envDoc(c, r.Env, "downloaded and added", r.Changes), nil
}
