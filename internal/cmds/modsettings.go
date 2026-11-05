package cmds

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// Mod settings (16 §2.9 "Mod settings", 13 §4): the current list's Mods that have settings,
// and what one Mod's settings files store.
func init() {
	register(&Command{Name: "mod-settings", Args: "[<ref>]", Summary: "Mod settings: the current list's Mods that have settings, or what one Mod's files store (change values with mod-settings set)",
		MinArgs: 0, MaxArgs: 1, Run: modSettingsCmd,
		Flags: []Flag{{Name: "all", Help: "every Mod or value, not only the first ones"}}})
}

// Changing simple values (16 §2.9 "Mod settings", 02 §1): only with the game closed, after
// backing the file up.
func init() {
	register(&Command{Name: "mod-settings set", Args: "<ref> [<name>=<value>…]",
		Summary: "change a Mod's simple settings values (on/off, numbers, one line of text, a HugsLib choice) with the game closed; backs the file up first",
		MinArgs: 1, MaxArgs: -1, Run: modSettingsSetCmd,
		Flags: []Flag{
			{Name: "reset", Value: "<name>", Multi: true, Help: "remove a stored value: the Mod uses its default"},
			{Name: "file", Value: "<key>", Help: "which settings when the Mod has several: its Mod class, or hugslib:<pack>"},
			{Name: "dry-run", Help: "check and show the change without writing"},
		}})
}

func modSettingsSetCmd(c *Ctx, a *Args) (*render.Doc, error) {
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	m, err := ix.ResolveOne(a.Pos[0])
	if err != nil {
		return nil, err
	}
	var edits []app.SettingEdit
	for _, p := range a.Pos[1:] {
		name, value, ok := strings.Cut(p, "=")
		if !ok || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("%q: write <name>=<value>", p)
		}
		edits = append(edits, app.SettingEdit{Name: strings.TrimSpace(name), Value: value})
	}
	for _, name := range a.List("reset") {
		edits = append(edits, app.SettingEdit{Name: name, Reset: true})
	}
	if len(edits) == 0 {
		return nil, errors.New("nothing to change: give <name>=<value> or --reset <name>")
	}
	w, err := c.App.EditModSettings(m, a.Str("file"), edits, a.Bool("dry-run"))
	if err != nil {
		return nil, err
	}
	ref := ix.Ref(m.ID)
	verb := "changed"
	if w.DryRun {
		verb = "would change"
	}
	if len(w.Changes) == 0 {
		d, _ := render.New(fmt.Sprintf("%s %s: nothing to change in %s (the values are already so)", ref, w.Mod, w.File))
		return d, nil
	}
	d, root := render.New(fmt.Sprintf("%s %s: %s %s in %s", ref, w.Mod, verb, render.Plural(len(w.Changes), "value"), w.File))
	for _, ch := range w.Changes {
		switch {
		case ch.Removed:
			root.Add(fmt.Sprintf("%s: %s → removed (the Mod uses its default)", ch.Name, quoteSetting(ch.From)))
		case ch.From == "":
			root.Add(fmt.Sprintf("%s: added %s", ch.Name, quoteSetting(ch.To)))
		default:
			root.Add(fmt.Sprintf("%s: %s → %s", ch.Name, quoteSetting(ch.From), quoteSetting(ch.To)))
		}
	}
	if w.Backup != "" {
		root.KV("backup", w.Backup)
	}
	if w.DryRun {
		again := []string{"archo mod-settings set", ref}
		for _, p := range a.Pos[1:] {
			again = append(again, quoteArg(p))
		}
		for _, name := range a.List("reset") {
			again = append(again, "--reset", quoteArg(name))
		}
		if k := a.Str("file"); k != "" {
			again = append(again, "--file", quoteArg(k))
		}
		d.AddNext(strings.Join(again, " "))
	} else {
		d.AddNext("archo mod-settings " + ref)
	}
	return d, nil
}

func quoteSetting(s string) string {
	if s == "" || strings.ContainsAny(s, " \"") {
		return fmt.Sprintf("%q", s)
	}
	return s
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
			d, _ := render.New(fmt.Sprintf("%s %s: no settings known (no settings file for folder %s, no HugsLib settings, and Archo Monitor saw no settings window)", ref, sm.Name, sm.Folder))
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
			rows = append(rows, []string{ix.Ref(sm.Instance), sm.Name, fileTitle(f), f.File, fileState(f)})
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

// fileTitle is the settings window's title, or HugsLib and the pack's entry.
func fileTitle(f app.ModSettingsFile) string {
	switch {
	case f.Kind == app.SettingsKindHugsLib && f.Title != "":
		return "HugsLib: " + f.Title
	case f.Kind == app.SettingsKindHugsLib:
		return "HugsLib (" + f.Class + ")"
	case f.Title == "":
		return "-"
	}
	return f.Title
}

func fileState(f app.ModSettingsFile) string {
	switch {
	case !f.Exists && f.Kind == app.SettingsKindHugsLib:
		return "defaults (HugsLib stores none)"
	case !f.Exists:
		return "defaults (no file yet)"
	case f.Kind == app.SettingsKindHugsLib:
		return "stored"
	}
	return render.Size(f.Size)
}

// settingsFiles adds what each of a Mod's settings files stores, at most limit values each.
func settingsFiles(root *render.Node, sm *app.ModSettingsMod, limit int, expand string) {
	for _, f := range sm.Files {
		if f.Kind == app.SettingsKindHugsLib {
			hugsLibSection(root, &f, limit, expand)
			continue
		}
		head := f.File
		if f.Title != "" {
			head = fmt.Sprintf("%q  %s", f.Title, f.File)
		}
		n := root.Section(head)
		if len(sm.Files) > 1 {
			n.KV("file key", f.Key())
		}
		if f.Window != nil {
			windowSection(n, f.Window, limit, expand)
		}
		if !f.Exists {
			n.Add("no file: the Mod uses its defaults until its settings are changed in the game")
			continue
		}
		sc, err := f.Read()
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

// hugsLibSection adds a HugsLib pack: with Archo Monitor's record every setting with its
// title, value and default; without, the values HugsLib stores.
func hugsLibSection(root *render.Node, f *app.ModSettingsFile, limit int, expand string) {
	head := fmt.Sprintf("HugsLib settings %s  %s", f.Class, f.File)
	if f.Title != "" {
		head = fmt.Sprintf("HugsLib %q (%s)  %s", f.Title, f.Class, f.File)
	}
	n := root.Section(head)
	n.KV("file key", f.Key())
	sc, err := f.Read()
	if err != nil {
		n.Add("cannot read: " + err.Error())
		return
	}
	rows, hidden := app.HugsLibRows(f.HugsLib, sc)
	if len(rows) == 0 {
		n.Add("every value is the Mod's default (HugsLib stores none)")
	}
	title := fmt.Sprintf("stored values (%d; a value at its default is not stored)", len(sc.Values))
	if f.HugsLib != nil {
		changed := 0
		for _, r := range rows {
			if r.Stored {
				changed++
			}
		}
		title = fmt.Sprintf("settings (%d changed; titles and defaults as Archo Monitor saw them %s)", changed, f.HugsLib.Seen.Local().Format("2006-01-02"))
	}
	vals := n
	if len(rows) > 0 {
		vals = n.Section(title)
	}
	for _, r := range rows {
		line := r.Name + ": " + settingText(r.Value)
		if r.Title != "" {
			line = fmt.Sprintf("%s (%s): %s", r.Title, r.Name, settingText(r.Value))
		}
		switch {
		case r.Stored && r.Recorded:
			line += "  [default " + settingText(r.Default) + "]"
		case r.Recorded:
			line += "  [default]"
		}
		vals.Add(line)
	}
	vals.Truncate(limit, expand)
	if hidden > 0 {
		n.Add(fmt.Sprintf("%s the Mod keeps out of its window not shown", render.Plural(hidden, "setting")))
	}
}

// windowSection adds what the game's settings window drew when the player last opened it
// (Archo Monitor): headings, and each control with its value, at most limit controls.
func windowSection(n *render.Node, w *app.SeenSettingsWindow, limit int, expand string) {
	win := n.Section(fmt.Sprintf("the game's window (seen %s; labels as the game drew them, values as they were then)", w.Seen.Local().Format("2006-01-02")))
	at, shown := win, 0
	for i, c := range w.Controls {
		if limit > 0 && shown == limit {
			win.Add(fmt.Sprintf("… %d more (%s)", len(w.Controls)-i, expand))
			break
		}
		if c.Kind == "label" {
			at = win.Section(c.Label)
			continue
		}
		shown++
		at.Add(windowText(c))
	}
	if !w.Full {
		win.Add("the window draws more than Archo Monitor keeps")
	}
}

// windowText is one control of a settings window as the CLI shows it.
func windowText(c logs.SettingsControl) string {
	label := c.Label
	if label == "" {
		label = "-"
	}
	switch c.Kind {
	case "button":
		return "[" + c.Label + "] (button)"
	case "slider":
		if c.Min != "" {
			return fmt.Sprintf("%s: %s (slider %s to %s)", label, c.Value, c.Min, c.Max)
		}
	case "text":
		return fmt.Sprintf("%s: %q (text)", label, c.Value)
	}
	return fmt.Sprintf("%s: %s (%s)", label, c.Value, c.Kind)
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
	case "empty":
		return "empty (a list or a text; change it in the game)"
	case "list":
		return render.Plural(v.Items, "item") + " (list)"
	case "map":
		return render.Plural(v.Items, "entry") + " (dictionary)"
	default:
		return render.Plural(v.Items, "field") + " (nested)"
	}
}
