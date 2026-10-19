package cmds

import (
	"fmt"
	"os"
	"strings"

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/render"
)

func init() {
	register(&Command{Name: "loadout export", Summary: "write the workspace as a text list or a ModsConfig.xml", MaxArgs: 0, Run: loadoutExport,
		Flags: []Flag{{Name: "format", Value: "text|modsconfig"}, {Name: "out", Value: "<file>", Help: "default: print to stdout"}}})
	register(&Command{Name: "loadout import", Args: "<file>", Summary: "replace the workspace with a text list, a ModsConfig.xml or a save's mod list (.rws); undoable", MinArgs: 1, MaxArgs: 1, Run: loadoutImport})
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
	root.KV("format", map[bool]string{true: "modsconfig", false: "text"}[a.Str("format") == app.FormatModsConfig])
	d.AddNext("archo loadout import " + quoteArg(out))
	return d, nil
}

func loadoutImport(c *Ctx, a *Args) (*render.Doc, error) {
	r, il, err := c.App.ImportLoadout(a.Pos[0])
	if err != nil {
		return nil, err
	}
	d := editDoc(c, r)
	missing := 0
	var names []string
	for _, e := range r.State.List {
		if e.Instance == "" {
			missing++
			n := e.Missing
			if nm := il.Names[e.Missing]; nm != "" {
				n += " (" + nm + ")"
			}
			names = append(names, n)
		}
	}
	line := fmt.Sprintf("imported %d entries from a %s", len(il.Tokens), map[string]string{app.FormatText: "text list", app.FormatModsConfig: "ModsConfig.xml", app.FormatSave: "save"}[il.Format])
	if il.Game != "" {
		line += ", game " + il.Game
	}
	head := []*render.Node{{Line: line}}
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
	d.Nodes[0].Children = append(head, d.Nodes[0].Children...)
	if !strings.Contains(strings.Join(d.Next, " "), "archo undo") {
		d.AddNext("archo undo")
	}
	return d, nil
}
