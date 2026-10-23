package cmds

import (
	"fmt"
	"os"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

func init() {
	register(&Command{Name: "loadout export", Summary: "write the workspace as a list file: a RimArcho list (each mod with its version), text, ModsConfig.xml (rimsort), RimWorld's .rml, or a RimCrow share code", MaxArgs: 0, Run: loadoutExport,
		Flags: []Flag{{Name: "format", Value: "text|archo|modsconfig|rimsort|rml|rimcrow|rimcrow-xml"}, {Name: "out", Value: "<file>", Help: "default: print to stdout"}}})
	register(&Command{Name: "loadout import", Args: "<file|share code> [<name>]", Summary: "replace the workspace with a list (RimArcho, text, ModsConfig.xml, save, .rml, RimSort, RimCrow, workshop IDs; undoable), or with --new make it a new plan",
		MinArgs: 1, MaxArgs: 2, Flags: []Flag{{Name: "new", Help: "import as a new plan named <name> (default: the list's own name or the file name)"}}, Run: loadoutImport})
}

func loadoutExport(c *Ctx, a *Args) (*render.Doc, error) {
	b, err := c.App.ExportLoadout(a.Str("format"))
	if err != nil {
		return nil, err
	}
	out := a.Str("out")
	if out == "" {
		return &render.Doc{Raw: string(b)}, nil
	}
	if _, err := os.Stat(out); err == nil {
		return nil, fmt.Errorf("%s exists; choose another --out (nothing is overwritten)", out)
	}
	if err := os.WriteFile(out, b, 0o644); err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("exported %d bytes to %s", len(b), out))
	f := a.Str("format")
	if f == "" {
		f = app.FormatText
	}
	root.KV("format", f)
	d.AddNext("archo loadout import " + quoteArg(out))
	return d, nil
}

func loadoutImport(c *Ctx, a *Args) (*render.Doc, error) {
	if a.Bool("new") {
		name := ""
		if len(a.Pos) == 2 {
			name = a.Pos[1]
		}
		r, err := c.App.ImportLoadoutAsPlan(a.Pos[0], name)
		if err != nil {
			return nil, err
		}
		return newPlanDoc(c, r), nil
	}
	if len(a.Pos) == 2 {
		return nil, &UsageError{Msg: "a plan name goes with --new (archo loadout import <file> --new [<name>])"}
	}
	r, il, err := c.App.ImportLoadout(a.Pos[0])
	if err != nil {
		return nil, err
	}
	d := editDoc(c, r)
	missing := 0
	var names, other []string
	for _, e := range r.State.List {
		if m := r.State.Ix.ByID[e.Instance]; m != nil {
			if h := il.Hash(m.PackageID); h != "" && !app.SameContent(m, h) {
				other = append(other, m.DisplayName())
			}
		}
		if e.Instance == "" {
			missing++
			n := e.Missing
			if nm := il.Names[e.Missing]; nm != "" {
				n += " (" + nm + ")"
			}
			names = append(names, n)
		}
	}
	kind := map[string]string{app.FormatArcho: "RimArcho list", app.FormatText: "text list", app.FormatModsConfig: "ModsConfig.xml", app.FormatSave: "save", app.FormatRML: "RimWorld mod list (.rml)",
		"rimcrow-modlist": "RimCrow ModList.xml", "rimcrow-json": "RimCrow JSON list", "rimcrow-share": "RimCrow share code", "rimsort-json": "RimSort JSON list",
		"rimsort-text": "RimSort clipboard list", "rentry": "Rentry list"}[il.Format]
	line := fmt.Sprintf("imported %d entries from a %s", len(r.State.List), kind)
	if il.Game != "" {
		line += ", game " + il.Game
	}
	head := []*render.Node{{Line: line}}
	if il.Title != "" {
		head = append(head, &render.Node{Line: "list: " + il.Title})
	}
	if len(il.Unresolved) > 0 {
		n := &render.Node{Line: fmt.Sprintf("skipped: %d workshop IDs no installed mod has", len(il.Unresolved))}
		for i, w := range il.Unresolved {
			if i == 8 {
				n.Children = append(n.Children, &render.Node{Line: fmt.Sprintf("… %d more", len(il.Unresolved)-8)})
				break
			}
			n.Children = append(n.Children, &render.Node{Line: "https://steamcommunity.com/sharedfiles/filedetails/?id=" + w})
		}
		head = append(head, n)
	}
	if missing > 0 {
		n := &render.Node{Line: fmt.Sprintf("not installed: %d", missing)}
		for i, x := range names {
			if i == 8 {
				n.Children = append(n.Children, &render.Node{Line: fmt.Sprintf("… %d more", missing-8)})
				break
			}
			n.Children = append(n.Children, &render.Node{Line: x})
		}
		head = append(head, n)
	}
	if len(other) > 0 {
		n := &render.Node{Line: fmt.Sprintf("another version installed: %d (the list was made with a different version of these)", len(other))}
		for i, x := range other {
			if i == 8 {
				n.Children = append(n.Children, &render.Node{Line: fmt.Sprintf("… %d more", len(other)-8)})
				break
			}
			n.Children = append(n.Children, &render.Node{Line: x})
		}
		head = append(head, n)
	}
	d.Nodes[0].Children = append(head, d.Nodes[0].Children...)
	if !strings.Contains(strings.Join(d.Next, " "), "archo undo") {
		d.AddNext("archo undo")
	}
	return d, nil
}
