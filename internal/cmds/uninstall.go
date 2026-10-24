package cmds

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// uninstall (05 §4.2, 01 §4's 卸载 column): a dry run unless --yes. MCP has only the dry run
// (preview_uninstall, 05 §5.2): removing RimArcho is the player's decision alone.
func init() {
	register(&Command{Name: "uninstall", Summary: "remove what RimArcho put on this machine and restore what it changed (a dry run; --yes runs it)", MaxArgs: 0, Run: uninstallCmd,
		Flags: []Flag{
			{Name: "dry-run", Help: "show the plan (the default)"},
			{Name: "yes", Help: "run it: refused while a game or another RimArcho runs"},
			{Name: "keep-locked", Help: "move the lock library's copies into a game's Mods/ as ordinary mods (default: recycle bin)"},
			{Name: "locked-to", Value: "<env>", Help: "the environment whose Mods/ gets them (default: the current one)"},
			{Name: "remove-packs", Help: "packs to the recycle bin (default: kept, moved out of the data folder)"},
			{Name: "packs-to", Value: "<dir>", Help: "where kept packs go (default: Documents\\RimArcho packs)"},
			{Name: "remove-saves", Help: "the environments' own save folders to the recycle bin (default: moved out)"},
			{Name: "saves-to", Value: "<dir>", Help: "where they go (default: Documents\\RimArcho saves)"},
			{Name: "remove-games", Help: "game folders downloaded with SteamCMD to the recycle bin (default: kept)"},
			{Name: "restore-modsconfig-backup", Help: "put ModsConfig.xml.rimarcho.bak back instead of cleaning the current list"},
			{Name: "all", Help: "no truncation"},
		}})
}

// UninstallOptions reads the answers from the flags.
func UninstallOptions(a *Args) app.UninstallOptions {
	return app.UninstallOptions{KeepLocked: a.Bool("keep-locked"), LockedTo: a.Str("locked-to"), RemovePacks: a.Bool("remove-packs"),
		PacksTo: a.Str("packs-to"), RemoveSaves: a.Bool("remove-saves"), SavesTo: a.Str("saves-to"), RemoveGames: a.Bool("remove-games"),
		RestoreBackup: a.Bool("restore-modsconfig-backup")}
}

// UninstallArgv is the command line of a set of answers (without --yes / --dry-run).
func UninstallArgv(o app.UninstallOptions) []string {
	argv := []string{"uninstall"}
	flag := func(on bool, name string) {
		if on {
			argv = append(argv, "--"+name)
		}
	}
	str := func(v, name string) {
		if v != "" {
			argv = append(argv, "--"+name, v)
		}
	}
	flag(o.KeepLocked, "keep-locked")
	str(o.LockedTo, "locked-to")
	flag(o.RemovePacks, "remove-packs")
	str(o.PacksTo, "packs-to")
	flag(o.RemoveSaves, "remove-saves")
	str(o.SavesTo, "saves-to")
	flag(o.RemoveGames, "remove-games")
	flag(o.RestoreBackup, "restore-modsconfig-backup")
	return argv
}

func uninstallCmd(c *Ctx, a *Args) (*render.Doc, error) {
	if a.Bool("yes") && a.Bool("dry-run") {
		return nil, &UsageError{Msg: "--yes runs it, --dry-run shows it: pick one"}
	}
	o := UninstallOptions(a)
	if !a.Bool("yes") {
		p, err := c.App.PlanUninstall(o)
		if err != nil {
			return nil, err
		}
		d, root := render.New("uninstall: dry run, nothing changed")
		UninstallOutline(root, p, a.Bool("all"), false)
		if err := p.Blocked(); err != nil {
			root.KV("refused", strings.TrimPrefix(err.Error(), app.ErrUninstallBlocked.Error()+": "))
		} else {
			line := "archo"
			for _, w := range append(UninstallArgv(o), "--yes") {
				line += " " + quoteArg(w)
			}
			d.AddNext(line)
		}
		return d, nil
	}
	var progress func(app.UninstallProgress)
	if Progress != nil {
		progress = func(p app.UninstallProgress) {
			Progress(fmt.Sprintf("progress: %s %d/%d %s", p.Step, p.I, p.N, p.Item))
		}
	}
	p, err := c.App.Uninstall(o, progress)
	if err != nil {
		if errors.Is(err, app.ErrUninstallBlocked) {
			return nil, fmt.Errorf("%v (nothing changed)", err)
		}
		return nil, err
	}
	failed := 0
	for _, s := range p.Steps {
		for _, it := range s.Items {
			if it.Err != "" {
				failed++
			}
		}
	}
	title := "uninstalled: RimArcho's folders and program are deleted once this command ends"
	if failed > 0 {
		title = fmt.Sprintf("uninstall stopped short: %d failed; the data folder stays, so this can run again", failed)
	}
	d, root := render.New(title)
	UninstallOutline(root, p, a.Bool("all"), true)
	if failed == 0 {
		root.KV("left over", "anything that could not be deleted is listed in "+p.Log)
	}
	return d, nil
}

// unVerb names what happens to an item.
var unVerb = map[string]string{
	app.DoUnlink: "unlink", app.DoClean: "clean", app.DoRestore: "restore", app.DoTrash: "recycle bin", app.DoMove: "move",
	app.DoKeep: "keep", app.DoDelete: "delete", app.DoLater: "after exit", app.DoLeave: "left alone",
}

// UninstallStepText is the one-line summary of a step.
func UninstallStepText(p *app.UninstallPlan, s *app.UninstallStep) string {
	count := map[string]int{}
	for _, it := range s.Items {
		count[it.Do]++
	}
	size := ""
	if b := s.Bytes(); b > 0 {
		size = ", " + render.Size(b)
	}
	var parts []string
	// add adds a part when some item does do; {n} is the count with the word.
	add := func(do, word, f string) {
		if n := count[do]; n > 0 {
			parts = append(parts, strings.ReplaceAll(f, "{n}", render.Plural(n, word)))
		}
	}
	switch s.Key {
	case app.UnLinks:
		add(app.DoUnlink, "link", "delete {n} in the games' Mods/ (never their targets)")
		add(app.DoLeave, "folder", "{n} named like one but not a link: left alone")
	case app.UnModsConfig:
		add(app.DoClean, "user data folder", "keep the ModsConfig.xml list of {n} without Archo Monitor and the mods only reached through a removed link; remove RimArcho's backup")
		add(app.DoRestore, "user data folder", "put the ModsConfig.xml backup back in {n}")
	case app.UnLocked:
		add(app.DoTrash, "lock library copy", "{n} to the recycle bin"+size+" (--keep-locked: into a game's Mods/)")
		add(app.DoMove, "lock library copy", "{n} into env:"+p.LockedTo+"'s Mods/ as ordinary mods"+size)
	case app.UnMonitor:
		add(app.DoDelete, "folder", "delete Archo Monitor ({n})"+size)
	case app.UnPacks:
		add(app.DoKeep, "pack", "keep {n} where they are (--remove-packs: recycle bin)")
		add(app.DoMove, "pack", "keep {n}: moved to "+p.PacksTo+size+" (--remove-packs: recycle bin)")
		add(app.DoTrash, "pack", "{n} to the recycle bin"+size)
		add(app.DoDelete, "unfinished build", "delete {n}")
	case app.UnSaves:
		add(app.DoMove, "save folder", "keep {n}: moved to "+p.SavesTo+size+" (--remove-saves: recycle bin)")
		add(app.DoTrash, "save folder", "{n} to the recycle bin"+size)
	case app.UnGames:
		add(app.DoKeep, "game folder", "keep {n} downloaded with SteamCMD (--remove-games: recycle bin)")
		add(app.DoTrash, "downloaded game folder", "{n} to the recycle bin"+size)
	case app.UnSteamCMD:
		add(app.DoDelete, "folder", "delete the SteamCMD RimArcho downloaded"+size)
		add(app.DoLeave, "folder", "left alone")
	case app.UnData:
		add(app.DoLater, "folder", "once RimArcho has closed, delete its data: database, settings, caches, the Steam runtime folder ({n})"+size)
		add(app.DoLeave, "folder", "{n} kept")
	case app.UnProgram:
		add(app.DoLater, "folder", "once RimArcho has closed, delete the program")
		add(app.DoLeave, "folder", "the program stays")
	}
	return strings.Join(parts, "; ")
}

// UninstallItemText is one item: what happens, the path, where it goes, why.
func UninstallItemText(it *app.UninstallItem) string {
	s := fmt.Sprintf("%-11s %s", unVerb[it.Do], it.Path)
	if it.To != "" {
		s += " → " + it.To
	}
	if it.Name != "" && it.Do != app.DoUnlink {
		s += "  (" + it.Name + ")"
	}
	if it.Bytes > 0 {
		s += "  " + render.Size(it.Bytes)
	}
	if len(it.Drop) > 0 {
		s += "  takes out " + strings.Join(it.Drop, ", ")
	}
	if it.Note != "" {
		s += "  — " + it.Note
	}
	return s
}

// UninstallOutline writes the plan, or with done the report, under n.
func UninstallOutline(n *render.Node, p *app.UninstallPlan, all, done bool) {
	for _, s := range p.Steps {
		sec := n.Add(s.Key + ": " + UninstallStepText(p, s))
		for _, it := range s.Items {
			l := UninstallItemText(it)
			switch {
			case !done:
			case it.Err != "":
				l = "FAILED " + l + "  error: " + it.Err
			case it.Done:
				l = "done   " + l
			case it.Do == app.DoLater:
				l = "later  " + l
			default:
				l = "       " + l
			}
			sec.Add(l)
			for _, line := range it.Links.Lines() {
				sec.KV("affected link", line)
			}
		}
		if !all {
			sec.Truncate(30, "archo uninstall --all")
		}
	}
}

// ParseUninstall reads an `uninstall …` command line's answers (the TUI's typed
// confirmation); --yes and --dry-run are ignored.
func ParseUninstall(argv []string) (app.UninstallOptions, error) {
	c, rest := lookup(argv)
	if c == nil || c.Name != "uninstall" {
		return app.UninstallOptions{}, &UsageError{Msg: "not an uninstall command"}
	}
	a, err := parse(c, rest)
	if err != nil {
		return app.UninstallOptions{}, err
	}
	return UninstallOptions(a), nil
}
