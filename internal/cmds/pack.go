package cmds

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// Packs (10 §8).
func init() {
	dry := Flag{Name: "dry-run", Help: "show what would happen"}
	register(&Command{Name: "pack build", Args: "<name>", Summary: "compile the workspace into a pack: checked, sorted, each mod copied without the version folders this game does not load", MinArgs: 1, MaxArgs: 1, Run: packBuildCmd,
		Flags: []Flag{{Name: "light", Help: "the list only, no files (for sharing)"}, dry}})
	register(&Command{Name: "pack list", Summary: "the packs: mods, size, game version, last played", MaxArgs: 0, Run: packListCmd})
	register(&Command{Name: "pack launch", Args: "<name>", Summary: "play a pack: link its mods into Mods/, its own list and saves, start the game", MinArgs: 1, MaxArgs: 1, Run: packLaunchCmd, Flags: []Flag{dry}})
	register(&Command{Name: "pack export", Args: "<name> <file>", Summary: "write a pack to a file (.archopack zip, or --light .archopack.json)", MinArgs: 2, MaxArgs: 2, Run: packExportCmd,
		Flags: []Flag{{Name: "light", Help: "the list only"}}})
	register(&Command{Name: "pack import", Args: "<file>", Summary: "unpack a .archopack, or compare a light pack with the installed mods", MinArgs: 1, MaxArgs: 1, Run: packImportCmd,
		Flags: []Flag{{Name: "as-plan", Help: "light pack: also load its list into the workspace (undoable)"}}})
	register(&Command{Name: "pack remove", Args: "<name>", Summary: "move a pack to the recycle bin", MinArgs: 1, MaxArgs: 1, Run: packRemoveCmd, Flags: []Flag{dry}})
}

func packBuildCmd(c *Ctx, a *Args) (*render.Doc, error) {
	r, err := c.App.BuildPack(a.Pos[0], a.Bool("light"), a.Bool("dry-run"))
	if err != nil {
		return nil, err
	}
	if len(r.Blockers) > 0 {
		d, root := render.New(fmt.Sprintf("cannot build pack %s: %d things to fix first", a.Pos[0], len(r.Blockers)))
		for _, b := range r.Blockers {
			root.Add(b)
		}
		root.Truncate(render.DefaultLimit, "")
		d.AddNext("archo problems")
		return nil, &UsageError{Msg: d.String()}
	}
	head := "built pack " + r.Pack.Name
	if r.DryRun {
		head = "pack build: dry run, nothing written"
	}
	d, root := render.New(head)
	carried, official := 0, 0
	var bytes int64
	for _, e := range r.Pack.Entries {
		if e.Official {
			official++
		} else {
			carried++
			bytes += e.Bytes
		}
	}
	root.KV("folder", r.Dir)
	root.KV("mods", fmt.Sprintf("%d carried, %d from the game (Core and DLCs)", carried, official))
	if !r.Pack.Light {
		root.KV("size", fmt.Sprintf("%s; %s of other game versions left out", mb(bytes), mb(r.Pack.Trimmed)))
		n := root.Add("trimmed:")
		for _, e := range r.Pack.Entries {
			if len(e.TrimmedDir) > 0 {
				n.Addf("%s: %s (%s)", e.Name, strings.Join(e.TrimmedDir, ", "), mb(e.Trimmed))
			}
		}
		n.Truncate(10, "")
		if len(n.Children) == 0 {
			n.Line = "trimmed: nothing (no folders for other game versions)"
		}
	}
	if r.Moved > 0 {
		root.KV("order", fmt.Sprintf("sorted for the pack: %d moved (the workspace is unchanged)", r.Moved))
	}
	root.KV("game", r.Pack.Game)
	if r.DryRun {
		d.AddNext("archo pack build " + quoteArg(r.Pack.Name))
	} else {
		d.AddNext("archo pack launch " + quoteArg(r.Pack.Name))
	}
	return d, nil
}

func packListCmd(c *Ctx, a *Args) (*render.Doc, error) {
	ps, err := c.App.Packs()
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("packs: %d (%s)", len(ps), c.App.PacksDir()))
	for _, p := range ps {
		kind := mb(p.Bytes)
		if p.Light {
			kind = "light"
		}
		played := "never played"
		if !p.LastPlayed.IsZero() {
			played = "played " + c.App.FormatDate(p.LastPlayed)
		}
		where := ""
		if p.Env != "" {
			where = "  env " + p.Env
		}
		root.Addf("%s  %d mods  %s  %s%s  built %s  %s", p.Name, len(p.Entries), kind, p.Game, where, c.App.FormatDate(p.Created), played)
	}
	if len(ps) == 0 {
		d.AddNext("archo pack build <name>")
	}
	return d, nil
}

func packLaunchCmd(c *Ctx, a *Args) (*render.Doc, error) {
	r, err := c.App.LaunchPack(a.Pos[0], a.Bool("dry-run"))
	if err != nil {
		return nil, err
	}
	head := "pack " + r.Pack.Name + ": the game is starting"
	if r.DryRun {
		head = "pack launch: dry run, nothing written"
	}
	d, root := render.New(head)
	ln := root.KV("links", fmt.Sprintf("%d changes in Mods/", len(r.Links)))
	for _, l := range r.Links {
		if l.Op == "remove" {
			ln.Add("- " + l.Name)
		} else {
			ln.Add("+ " + l.Name)
		}
	}
	ln.Truncate(10, "")
	root.KV("list", fmt.Sprintf("%s (%d entries; the environment's ModsConfig.xml is not touched)", r.ModsConfig, len(r.Tokens)))
	root.KV("saves", r.UserData)
	if len(r.Copied) > 0 {
		root.KV("settings", "copied from the environment: "+strings.Join(r.Copied, ", "))
	}
	root.KV("command", r.Plan.Command())
	return d, nil
}

func packExportCmd(c *Ctx, a *Args) (*render.Doc, error) {
	n, err := c.App.ExportPack(a.Pos[0], a.Pos[1], a.Bool("light"))
	if err != nil {
		return nil, err
	}
	d, _ := render.New(fmt.Sprintf("exported pack %s to %s (%s)", a.Pos[0], a.Pos[1], mb(n)))
	return d, nil
}

func packImportCmd(c *Ctx, a *Args) (*render.Doc, error) {
	r, err := c.App.ImportPack(a.Pos[0], a.Bool("as-plan"))
	if err != nil {
		return nil, err
	}
	if r.Dir != "" {
		d, root := render.New("unpacked pack " + r.Pack.Name)
		root.KV("folder", r.Dir)
		d.AddNext("archo pack launch " + quoteArg(r.Pack.Name))
		return d, nil
	}
	d, root := render.New(fmt.Sprintf("light pack %s: %d mods", r.Pack.Name, len(r.Pack.Entries)))
	for _, g := range []struct {
		k  string
		es []app.PackEntry
	}{{"same version installed", r.Same}, {"another version installed", r.Different}, {"not installed", r.Missing}} {
		n := root.KV(g.k, fmt.Sprint(len(g.es)))
		for _, e := range g.es {
			line := e.Name + "  pkg:" + e.PackageID
			if e.WorkshopID != "" {
				line += "  https://steamcommunity.com/sharedfiles/filedetails/?id=" + e.WorkshopID
			}
			n.Add(line)
		}
		n.Truncate(10, "")
	}
	if r.Imported != nil {
		root.KV("workspace", fmt.Sprintf("now holds the pack's list (%d changes, unsaved; archo undo)", len(r.Imported.Changes)))
	} else {
		d.AddNext("archo pack import " + quoteArg(a.Pos[0]) + " --as-plan")
	}
	return d, nil
}

func packRemoveCmd(c *Ctx, a *Args) (*render.Doc, error) {
	dir, err := c.App.RemovePack(a.Pos[0], a.Bool("dry-run"))
	if err != nil {
		return nil, err
	}
	if a.Bool("dry-run") {
		d, _ := render.New("dry run: would move " + dir + " to the recycle bin")
		return d, nil
	}
	d, _ := render.New("moved " + dir + " to the recycle bin")
	return d, nil
}
