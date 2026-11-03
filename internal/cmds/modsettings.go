package cmds

import (
	"fmt"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// Mod settings, read only (06 §2.9 "Mod settings", 05 §4): the current list's Mods that have
// settings, and what one Mod's settings files store.
func init() {
	register(&Command{Name: "mod-settings", Args: "[<ref>]", Summary: "Mod settings: the current list's Mods that have settings, or what one Mod's files store (read only)",
		MinArgs: 0, MaxArgs: 1, Run: modSettingsCmd,
		Flags: []Flag{{Name: "all", Help: "every Mod or value, not only the first ones"}}})
}

func modSettingsCmd(c *Ctx, a *Args) (*render.Doc, error) {
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	limit := render.DefaultLimit
	if a.Bool("all") {
		limit = 0
	}
	if len(a.Pos) == 1 {
		m, err := ix.ResolveOne(a.Pos[0])
		if err != nil {
			return nil, err
		}
		sm, err := c.App.ModSettingsOf(m)
		if err != nil {
			return nil, err
		}
		ref := ix.Ref(m.ID)
		if len(sm.Files) == 0 {
			d, _ := render.New(fmt.Sprintf("%s %s: no settings known (no settings file for folder %s, and Archo Monitor saw no settings window)", ref, sm.Name, sm.Folder))
			return d, nil
		}
		d, root := render.New(fmt.Sprintf("%s %s: settings (folder %s)", ref, sm.Name, sm.Folder))
		settingsFiles(root, sm, limit, "archo mod-settings "+ref+" --all")
		return d, nil
	}
	l, err := c.App.ModSettings()
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("mod settings: %s in %s (%s)", render.Plural(len(l.Mods), "Mod"), l.Plan, l.Config))
	var rows [][]string
	for _, sm := range l.Mods {
		for _, f := range sm.Files {
			title := f.Title
			if title == "" {
				title = "-"
			}
			rows = append(rows, []string{ix.Ref(sm.Instance), sm.Name, title, f.File, fileState(f)})
		}
	}
	root.AddColumns(rows)
	root.Truncate(limit, "archo mod-settings --all")
	if len(l.Mods) == 0 {
		root.Add("no Mod of this list has a settings file, and Archo Monitor recorded no settings window yet")
	} else {
		d.AddNext("archo mod-settings <ref>")
	}
	return d, nil
}

func fileState(f app.ModSettingsFile) string {
	if !f.Exists {
		return "defaults (no file yet)"
	}
	return render.Size(f.Size)
}

// settingsFiles adds what each of a Mod's settings files stores, at most limit values each.
func settingsFiles(root *render.Node, sm *app.ModSettingsMod, limit int, expand string) {
	for _, f := range sm.Files {
		head := f.File
		if f.Title != "" {
			head = fmt.Sprintf("%q  %s", f.Title, f.File)
		}
		n := root.Section(head)
		if !f.Exists {
			n.Add("no file: the Mod uses its defaults until its settings are changed in the game")
			continue
		}
		sc, err := app.ReadSettingsFile(f.Path)
		if err != nil {
			n.Add("cannot read: " + err.Error())
			continue
		}
		if len(sc.Values) == 0 {
			n.Add("every value is the Mod's default (the file stores none)")
			continue
		}
		vals := n.Section(fmt.Sprintf("stored values (%d; a value at its default is not stored)", len(sc.Values)))
		for _, v := range sc.Values {
			vals.Add(v.Key + ": " + settingText(v))
		}
		vals.Truncate(limit, expand)
		if !sc.Simple {
			n.Add("lists and nested values are read only: change them in the game")
		}
	}
}

// settingText is a stored value as the CLI shows it.
func settingText(v app.SettingValue) string {
	switch v.Kind {
	case "bool":
		return v.Value + " (on/off)"
	case "number":
		return v.Value
	case "text":
		if v.Value == "" {
			return `""`
		}
		return fmt.Sprintf("%q", v.Value)
	case "null":
		return "null"
	case "list":
		return render.Plural(v.Items, "item") + " (list)"
	case "map":
		return render.Plural(v.Items, "entry") + " (dictionary)"
	default:
		return render.Plural(v.Items, "field") + " (nested)"
	}
}
