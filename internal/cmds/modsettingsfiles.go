package cmds

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// The Config folder as a whole (16 §2.9 "Mod settings"): every file and where it stands,
// moving Mod settings files to the recycle bin, resetting a Mod to its defaults, and copying
// settings from another Game Config (02 §1).
func init() {
	register(&Command{Name: "mod-settings files", Args: "",
		Summary: "every file in the Game Config's Config folder: read by this list, an installed Mod's not in it, an orphan (no installed Mod), or the game's own",
		MaxArgs: 0, Run: modSettingsFilesCmd,
		Flags: []Flag{
			{Name: "filter", Value: "listed|unlisted|orphans|game", Help: "only these files"},
			{Name: "all", Help: "every file, not only the first ones"},
		}})
	register(&Command{Name: "mod-settings recycle", Args: "[<file>…]",
		Summary: "move Mod options files (Mod_*.xml in the Config folder) to the recycle bin, with the game closed",
		MaxArgs: -1, Run: modSettingsRecycleCmd,
		Flags: []Flag{
			{Name: "orphans", Help: "every orphan: the files no installed Mod reads"},
			{Name: "dry-run", Help: "list the files without moving them"},
		}})
	register(&Command{Name: "mod-settings reset", Args: "<ref>",
		Summary: "reset a Mod to its default settings: its settings files go to the recycle bin, with the game closed",
		MinArgs: 1, MaxArgs: 1, Run: modSettingsResetCmd,
		Flags: []Flag{{Name: "dry-run", Help: "list the files without moving them"}}})
	register(&Command{Name: "mod-settings copy-from", Args: "<game-config> [<ref>…]",
		Summary: "copy the current list's Mods' settings from another Game Config (all Mods, or those named); a replaced file goes to the recycle bin first",
		MinArgs: 1, MaxArgs: -1, Run: modSettingsCopyCmd,
		Flags: []Flag{{Name: "dry-run", Help: "show what would be copied and replaced"}}})
	register(&Command{Name: "mod-settings apply", Args: "<list file> [<package>…]",
		Summary: "apply the Mod options a RimArcho list carries to the installed Mods (all, or the packages named); existing files are kept unless --replace",
		MinArgs: 1, MaxArgs: -1, Run: modSettingsApplyCmd,
		Flags: []Flag{{Name: "replace", Help: "also replace settings files that are here (each goes to the recycle bin first)"},
			{Name: "dry-run", Help: "the per-Mod preview: what would be written, kept or replaced"}}})
}

func modSettingsApplyCmd(c *Ctx, a *Args) (*render.Doc, error) {
	il, err := app.ReadLoadoutFile(a.Pos[0])
	if err != nil {
		return nil, err
	}
	p, err := c.App.ApplyListSettings(il.Settings, a.Pos[1:], a.Bool("replace"), a.Bool("dry-run"))
	if err != nil {
		return nil, err
	}
	verb := "wrote"
	if p.DryRun {
		verb = "would write"
	}
	d, root := render.New(fmt.Sprintf("Mod options from %s: %s %s into %s", a.Pos[0], verb, render.Plural(p.Writes(), "file"), p.Config))
	failed, kept := 0, 0
	for _, m := range p.Mods {
		n := root.Add(m.Mod + "  pkg:" + m.PackageID)
		if m.Missing {
			n.Add("skipped: not installed here")
			continue
		}
		for _, f := range m.Files {
			line := f.To
			if f.HugsLib {
				line += " (HugsLib " + f.Class + ")"
			}
			switch {
			case f.Err != "":
				failed++
				line += "  failed: " + f.Err
			case f.Same:
				line += "  (already the same)"
			case f.Write && f.Exists:
				line += "  (replaces the one here; it goes to the recycle bin)"
			case f.Write:
				line += "  (new)"
			default:
				kept++
				line += "  (kept: one is here; --replace replaces it)"
			}
			n.Add(line)
		}
	}
	if p.DryRun && p.Writes() > 0 {
		args := append([]string{"archo mod-settings apply", quoteArg(a.Pos[0])}, quoteAll(a.Pos[1:])...)
		if p.Replace {
			args = append(args, "--replace")
		}
		d.AddNext(strings.Join(args, " "))
	} else if p.DryRun && kept > 0 {
		d.AddNext(strings.Join(append(append([]string{"archo mod-settings apply", quoteArg(a.Pos[0])}, quoteAll(a.Pos[1:])...), "--replace", "--dry-run"), " "))
	}
	if failed > 0 && failed == p.Writes()+failed && !p.DryRun {
		return d, fmt.Errorf("%s", d.String())
	}
	return d, nil
}

func configStatus(s string) string {
	switch s {
	case app.ConfigFileListed:
		return "this list"
	case app.ConfigFileUnlisted:
		return "not in this list"
	case app.ConfigFileOrphan:
		return "not installed"
	}
	return "game file"
}

func modSettingsFilesCmd(c *Ctx, a *Args) (*render.Doc, error) {
	filter := a.Str("filter")
	want := map[string]string{"": "", "listed": app.ConfigFileListed, "unlisted": app.ConfigFileUnlisted,
		"orphans": app.ConfigFileOrphan, "orphan": app.ConfigFileOrphan, "game": app.ConfigFileGame}
	status, ok := want[filter]
	if !ok {
		return nil, fmt.Errorf("--filter %q: use listed, unlisted, orphans or game", filter)
	}
	cf, err := c.App.ModSettingsFiles()
	if err != nil {
		return nil, err
	}
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	var rows [][]string
	counts := map[string]int{}
	var orphanBytes int64
	for _, f := range cf.Files {
		counts[f.Status]++
		if f.Status == app.ConfigFileOrphan {
			orphanBytes += f.Size
		}
		if status != "" && f.Status != status {
			continue
		}
		mod := "-"
		if f.Instance != "" {
			mod = ix.Ref(f.Instance) + " " + f.Mod
		}
		rows = append(rows, []string{f.File, configStatus(f.Status), mod, render.Size(f.Size), f.Modified.In(c.App.Loc).Format("2006-01-02 15:04")})
	}
	d, root := render.New(fmt.Sprintf("Config folder %s: %s (%d this list, %d not in this list, %d not installed, %d game files)",
		cf.Config, render.Plural(len(cf.Files), "file"), counts[app.ConfigFileListed], counts[app.ConfigFileUnlisted], counts[app.ConfigFileOrphan], counts[app.ConfigFileGame]))
	root.AddColumns(rows)
	limit := render.DefaultLimit
	if a.Bool("all") {
		limit = 0
	}
	expand := "archo mod-settings files --all"
	if filter != "" {
		expand = "archo mod-settings files --filter " + filter + " --all"
	}
	root.Truncate(limit, expand)
	if len(rows) == 0 {
		root.Add("no such file")
	}
	if counts[app.ConfigFileOrphan] > 0 {
		root.KV("orphans", fmt.Sprintf("%s, %s: no installed Mod reads them", render.Plural(counts[app.ConfigFileOrphan], "file"), render.Size(orphanBytes)))
		d.AddNext("archo mod-settings recycle --orphans --dry-run")
	}
	return d, nil
}

func modSettingsRecycleCmd(c *Ctx, a *Args) (*render.Doc, error) {
	names := append([]string(nil), a.Pos...)
	if a.Bool("orphans") {
		cf, err := c.App.ModSettingsFiles()
		if err != nil {
			return nil, err
		}
		for _, f := range cf.Files {
			if f.Status == app.ConfigFileOrphan {
				names = append(names, f.File)
			}
		}
		if len(names) == 0 {
			d, _ := render.New("no orphaned settings file: every Mod options file belongs to an installed Mod")
			return d, nil
		}
	}
	if len(names) == 0 {
		return nil, errors.New("name the files (archo mod-settings files), or give --orphans")
	}
	r, err := c.App.RecycleModSettings(names, a.Bool("dry-run"))
	if err != nil {
		return nil, err
	}
	again := "archo mod-settings recycle " + strings.Join(quoteAll(a.Pos), " ")
	if a.Bool("orphans") {
		again = strings.TrimSpace(again) + " --orphans"
	}
	return recycleDoc(r, "", strings.TrimSpace(again))
}

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = quoteArg(s)
	}
	return out
}

// recycleDoc reports a SettingsRecycle; again repeats a dry run for real.
func recycleDoc(r *app.SettingsRecycle, subject, again string) (*render.Doc, error) {
	verb := "moved"
	if r.DryRun {
		verb = "would move"
	}
	var size int64
	for _, f := range r.Files {
		size += f.Size
	}
	head := fmt.Sprintf("%s%s %s (%s) to the recycle bin", subject, verb, render.Plural(len(r.Files), "settings file"), render.Size(size))
	d, root := render.New(head)
	for _, f := range r.Files {
		root.Add(f.File)
	}
	for _, f := range r.Failed {
		root.Add(f.File + ": not moved: " + f.Err)
	}
	if r.DryRun {
		d.AddNext(again)
	}
	if len(r.Failed) > 0 && len(r.Files) == 0 {
		return d, fmt.Errorf("%s", d.String())
	}
	return d, nil
}

func modSettingsResetCmd(c *Ctx, a *Args) (*render.Doc, error) {
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	m, err := ix.ResolveOne(a.Pos[0])
	if err != nil {
		return nil, err
	}
	r, err := c.App.ResetModSettings(m, a.Bool("dry-run"))
	if err != nil {
		return nil, err
	}
	ref := ix.Ref(m.ID)
	d, err := recycleDoc(r, ref+" "+m.DisplayName()+": ", "archo mod-settings reset "+ref)
	if err == nil && !r.DryRun {
		d.Nodes[0].Line += "; the game starts it with its defaults"
	}
	return d, err
}

func modSettingsCopyCmd(c *Ctx, a *Args) (*render.Doc, error) {
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, r := range a.Pos[1:] {
		m, err := ix.ResolveOne(r)
		if err != nil {
			return nil, err
		}
		ids = append(ids, m.ID)
	}
	p, err := c.App.CopyModSettingsFrom(a.Pos[0], ids, a.Bool("dry-run"))
	if err != nil {
		return nil, err
	}
	n, replace, failed := 0, 0, 0
	for _, cp := range p.Copies {
		switch {
		case cp.Err != "":
			failed++
		case cp.Same:
		default:
			n++
			if cp.Replace {
				replace++
			}
		}
	}
	verb := "copied"
	if p.DryRun {
		verb = "would copy"
	}
	d, root := render.New(fmt.Sprintf("settings from %s (%s): %s %s, %d replacing a file here (recycled first)", p.From, p.Config, verb, render.Plural(n, "file"), replace))
	for _, cp := range p.Copies {
		line := fmt.Sprintf("%s %s: %s → %s", ix.Ref(cp.Instance), cp.Mod, cp.From, cp.To)
		switch {
		case cp.Err != "":
			line += "  failed: " + cp.Err
		case cp.Same:
			line += "  (already the same)"
		case cp.Replace:
			line += "  (replaces the file here)"
		default:
			line += "  (new)"
		}
		root.Add(line)
	}
	if len(p.Copies) == 0 {
		root.Add("no settings file there for the Mods asked about")
	}
	if p.DryRun && n > 0 {
		d.AddNext(strings.Join(append([]string{"archo mod-settings copy-from", quoteArg(p.From)}, quoteAll(a.Pos[1:])...), " "))
	}
	if failed > 0 && n == failed {
		return d, fmt.Errorf("%s", d.String())
	}
	return d, nil
}
