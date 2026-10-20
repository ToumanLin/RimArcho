package cmds

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/source"
)

var envFlags = []Flag{
	{Name: "game", Value: "<dir>", Help: "game install (contains RimWorldWin64.exe / RimWorld.exe)"},
	{Name: "user-data", Value: "<dir>", Help: "user data (contains Config/ModsConfig.xml)"},
	{Name: "workshop", Value: "<dir>", Help: "workshop content dir …/workshop/content/294100"},
	{Name: "steamcmd", Value: "<dir>", Multi: true, Help: "SteamCMD content dir (read-only)"},
	{Name: "extra", Value: "<dir>", Multi: true, Help: "other mod folder"},
	{Name: "launch", Value: "direct|steam"},
}

var removeSourceFlag = Flag{Name: "remove-source", Value: "<src:label>", Multi: true, Help: "retire a steamcmd or extra source (env set)"}

func init() {
	register(&Command{Name: "env list", Summary: "list environments and their sources", MaxArgs: 0, Run: envList})
	register(&Command{Name: "env detect", Summary: "detect game, user data and workshop paths (suggestion only)", MaxArgs: 0, Run: envDetect})
	register(&Command{Name: "env add", Args: "<name>", Summary: "add an environment", MinArgs: 1, MaxArgs: 1,
		Flags: append([]Flag{{Name: "detected", Help: "use the paths env detect finds"}}, envFlags...), Run: envAdd})
	register(&Command{Name: "env set", Args: "<name>", Summary: "change an environment's paths (re-binds moved sources)", MinArgs: 1, MaxArgs: 1,
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
		n.KV("user-data", e.UserDataRoot)
		n.KV("launch", e.Launch)
		if e.UserDataRoot != "" {
			n.KV("player-log", env.PlayerLog(e.UserDataRoot, e.GameVersion))
		}
		srcs, err := c.App.Store.Sources(e.ID)
		if err != nil {
			return nil, err
		}
		addSourceLines(n.Section("sources"), srcs)
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
	return app.EnvSpec{GameRoot: a.Str("game"), UserDataRoot: a.Str("user-data"), WorkshopRoot: a.Str("workshop"),
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
	root.KV("kept", "game, workshop and mod folders are untouched")
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
		if spec.UserDataRoot == "" {
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
		n := srcs.Add(fmt.Sprintf("%s  %s  %s", s.Ref(), ch.Action, s.Root))
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
