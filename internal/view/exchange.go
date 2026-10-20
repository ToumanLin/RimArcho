package view

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/loadout"
)

// Exchange with other mod managers (10 §4): a list read from a file, pasted text (any
// list, a share code, a Rentry link) or a Steam Workshop collection is previewed first —
// what is installed, what is missing and where to get it, what the enabled list would
// become — and then put into the workspace as one undoable step. Nothing writes
// ModsConfig.xml: that stays save's job.

// Import sources.
const (
	ImportFile       = "file"       // a path
	ImportText       = "text"       // pasted text, a share code, a Rentry or collection link, or a path
	ImportCollection = "collection" // a Workshop collection link or ID
)

// Import is a list read for import, kept between its preview and its apply.
type Import struct {
	List   *app.ImportedList
	Label  string            // what the undo step calls it
	Source string            // the file, "share code", the link
	Titles map[string]string // workshop ID → title (collections)
}

var (
	steamLinkRe = regexp.MustCompile(`^https?://steamcommunity\.com/(?:sharedfiles|workshop)/filedetails/?\?(?:.*&)?id=\d+`)
	bareIDRe    = regexp.MustCompile(`^\d{6,}$`)
)

// ReadImport reads a list from a source. Pasted text that is one line is looked at first:
// a file that exists is read as a file, a Rentry link is fetched, and a Steam Workshop
// link or ID is read as a collection (a list of one mod is not worth importing); the
// network is used only for those two.
func ReadImport(a *app.App, kind, src string) (*Import, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, errors.New(T("nothing to import"))
	}
	oneLine := !strings.ContainsAny(src, "\r\n")
	if kind == ImportText && oneLine {
		path := strings.Trim(src, `"'`)
		switch {
		case fileExists(path):
			kind, src = ImportFile, path
		case app.RentryID(src) != "":
			il, err := a.ReadRentry(src)
			if err != nil {
				return nil, err
			}
			return checked(&Import{List: il, Label: "rentry " + app.RentryID(src), Source: src})
		case steamLinkRe.MatchString(src) || bareIDRe.MatchString(src):
			kind = ImportCollection
		}
	}
	switch kind {
	case ImportFile:
		path := strings.Trim(src, `"'`)
		il, err := app.ReadLoadoutFile(path)
		if err != nil {
			return nil, err
		}
		return checked(&Import{List: il, Label: filepath.Base(path), Source: path})
	case ImportCollection:
		c, err := a.ReadCollection(src)
		if err != nil {
			return nil, err
		}
		il := &app.ImportedList{Format: "collection", Title: c.Title, Names: map[string]string{}, Workshop: map[string]string{}}
		titles := map[string]string{}
		for _, it := range c.Items {
			if !it.Sub {
				il.WorkshopOnly = append(il.WorkshopOnly, it.WorkshopID)
				titles[it.WorkshopID] = it.Title
			}
		}
		return checked(&Import{List: il, Label: "collection ws:" + c.ID, Source: "https://steamcommunity.com/sharedfiles/filedetails/?id=" + c.ID, Titles: titles})
	case ImportText:
		il, err := app.ParseList([]byte(src))
		if err != nil {
			return nil, err
		}
		label := "pasted list"
		if il.Format == "rimcrow-share" {
			label = "share code"
		}
		return checked(&Import{List: il, Label: label, Source: T(label)})
	}
	return nil, fmt.Errorf("unknown source %q", kind)
}

func checked(imp *Import) (*Import, error) {
	if len(imp.List.Tokens) == 0 && len(imp.List.WorkshopOnly) == 0 {
		return nil, errors.New(T("no mods found in it: a list from RimSort, RimCrow or RimWorld, a share code, packageIds or workshop links"))
	}
	return imp, nil
}

func fileExists(p string) bool {
	if p == "" || len(p) > 1024 {
		return false
	}
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// FormatName is a list format as the player knows it.
func FormatName(f string) string {
	n := map[string]string{
		app.FormatText: "text list", app.FormatModsConfig: "ModsConfig.xml", app.FormatSave: "save", app.FormatRML: "RimWorld mod list (.rml)",
		"rimcrow-modlist": "RimCrow ModList.xml", "rimcrow-json": "RimCrow JSON list", "rimcrow-share": "RimCrow share code",
		"rimsort-json": "RimSort JSON list", "rimsort-text": "RimSort clipboard list", "rentry": "Rentry list", "collection": "Workshop collection",
	}[f]
	if n == "" {
		return f
	}
	return T(n)
}

// ImportRow is one mod of an imported list.
type ImportRow struct {
	Name    string `json:"name"`
	Package string `json:"package,omitempty"`
	ID      string `json:"id,omitempty"`      // the installed instance
	Enabled bool   `json:"enabled,omitempty"` // already in the enabled list
	URL     string `json:"url,omitempty"`     // its Workshop page (subscribe there)
	// Instead is a mod the community data names as its replacement.
	Instead    string `json:"instead,omitempty"`
	InsteadURL string `json:"insteadUrl,omitempty"`
}

// ImportEffect is what the enabled list would become.
type ImportEffect struct {
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Moved   int    `json:"moved"`
	Swapped int    `json:"swapped"`
	Count   int    `json:"count"` // entries after it
	Same    bool   `json:"same"`  // nothing would change
	Text    string `json:"text"`
}

// ImportPreview is what an interface shows before an import.
type ImportPreview struct {
	Format    string       `json:"format"`
	Source    string       `json:"source"`
	Name      string       `json:"name,omitempty"` // the list's own title
	Game      string       `json:"game,omitempty"`
	GameNote  string       `json:"gameNote,omitempty"` // the list is for another game version
	Total     int          `json:"total"`
	Installed []ImportRow  `json:"installed"`
	Missing   []ImportRow  `json:"missing"`
	Replace   ImportEffect `json:"replace"`
	Merge     ImportEffect `json:"merge"`
	Summary   string       `json:"summary"`
}

// PreviewImport compares an imported list with the installed mods and the workspace;
// it changes nothing.
func PreviewImport(a *app.App, imp *Import) (*ImportPreview, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	il := imp.List
	p := &ImportPreview{Format: FormatName(il.Format), Source: imp.Source, Name: il.Title, Game: il.Game, Installed: []ImportRow{}, Missing: []ImportRow{}}
	if mm := st.GameMM(); il.Game != "" && mm != "" && !strings.HasPrefix(il.Game, mm) {
		p.GameNote = Tf("the list is for RimWorld %s, this environment runs %s", majorMinor(il.Game), mm)
	}
	list, unresolved := app.ImportedEntries(st, il)
	p.Total = len(list) + len(unresolved)
	for _, e := range list {
		if e.Instance != "" {
			r := ImportRow{ID: e.Instance, Enabled: st.List.Index(e.Instance) >= 0}
			if m := st.Ix.ByID[e.Instance]; m != nil {
				r.Name, r.Package = m.DisplayName(), m.PackageID
			}
			p.Installed = append(p.Installed, r)
			continue
		}
		pkg := e.Missing
		r := ImportRow{Name: il.Names[pkg], Package: pkg}
		ws := il.Workshop[pkg]
		if w, ok := a.WorkshopFor(pkg); ok {
			if ws == "" {
				ws = w.WorkshopID
			}
			if r.Name == "" {
				r.Name = w.Name
			}
		}
		if r.Name == "" {
			r.Name = pkg
		}
		p.Missing = append(p.Missing, missingRow(a, r, ws))
	}
	for _, ws := range unresolved {
		r := ImportRow{Name: imp.Titles[ws]}
		if r.Name == "" {
			r.Name = "ws:" + ws
		}
		p.Missing = append(p.Missing, missingRow(a, r, ws))
	}
	p.Replace = effect(st, list)
	p.Merge = effect(st, app.MergedEntries(st, il))
	p.Summary = Tf("%s: %d mods, %d installed, %d not installed", p.Format, p.Total, len(p.Installed), len(p.Missing))
	return p, nil
}

func missingRow(a *app.App, r ImportRow, ws string) ImportRow {
	if ws != "" {
		r.URL = workshopURL(ws)
	}
	if rp := a.ReplacementFor(ws, r.Package); rp != nil {
		r.Instead = rp.NewName
		if r.Instead == "" {
			r.Instead = rp.NewPackageID
		}
		if rp.NewWorkshopID != "" {
			r.InsteadURL = workshopURL(rp.NewWorkshopID)
		}
	}
	return r
}

func workshopURL(ws string) string {
	return "https://steamcommunity.com/sharedfiles/filedetails/?id=" + ws
}

func majorMinor(v string) string {
	if p := strings.Split(v, "."); len(p) >= 2 {
		return p[0] + "." + p[1]
	}
	return v
}

func effect(st *app.LoadoutState, after loadout.List) ImportEffect {
	e := ImportEffect{Count: len(after), Same: after.Equal(st.List)}
	for _, c := range loadout.Diff(st.List, after, st.LineageOf) {
		switch c.Kind {
		case loadout.Added:
			e.Added++
		case loadout.Removed:
			e.Removed++
		case loadout.Moved:
			e.Moved++
		case loadout.Replaced:
			e.Swapped++
		}
	}
	if e.Same {
		e.Text = T("the enabled list already is this")
		return e
	}
	var parts []string
	for _, x := range []struct {
		n int
		f string
	}{{e.Added, "%d added"}, {e.Removed, "%d removed"}, {e.Moved, "%d moved"}, {e.Swapped, "%d swapped for another copy"}} {
		if x.n > 0 {
			parts = append(parts, Tf(x.f, x.n))
		}
	}
	e.Text = Tf("%s · %d enabled after it", strings.Join(parts, ", "), e.Count)
	return e
}

// Lines is the preview as text (the TUI's panel).
func (p *ImportPreview) Lines() []string {
	out := []string{p.Summary}
	if p.Name != "" {
		out = append(out, Tf("list: %s", p.Name))
	}
	if p.GameNote != "" {
		out = append(out, p.GameNote)
	}
	out = append(out, "", T("replace the enabled list")+": "+p.Replace.Text, T("add the ones you lack")+": "+p.Merge.Text)
	if len(p.Missing) > 0 {
		out = append(out, "", Tf("not installed: %d", len(p.Missing)))
		for _, r := range p.Missing {
			l := "  " + r.Name
			if r.Package != "" && r.Package != r.Name {
				l += "  (" + r.Package + ")"
			}
			out = append(out, l)
			if r.URL != "" {
				out = append(out, "    "+r.URL)
			}
			if r.Instead != "" {
				out = append(out, "    "+Tf("replaced by %s", r.Instead)+" "+r.InsteadURL)
			}
		}
	}
	return out
}

// ApplyImport puts the list into the workspace (replace, or with merge add the installed
// mods it has that the list lacks) as one undoable step, and says what happened.
func ApplyImport(a *app.App, imp *Import, merge bool) (string, error) {
	r, err := a.ApplyImport(imp.List, imp.Label, merge)
	if err != nil {
		return "", err
	}
	if len(r.Changes) == 0 {
		return T("the enabled list already is this"), nil
	}
	e := ImportEffect{}
	for _, c := range r.Changes {
		switch c.Kind {
		case loadout.Added:
			e.Added++
		case loadout.Removed:
			e.Removed++
		case loadout.Moved:
			e.Moved++
		case loadout.Replaced:
			e.Swapped++
		}
	}
	if merge {
		return Tf("added %s from %s · not saved yet, u undoes", Plural(e.Added, "mod"), imp.Source), nil
	}
	return Tf("imported %s: %d added, %d removed, %d moved · not saved yet, u undoes", imp.Source, e.Added+e.Swapped, e.Removed, e.Moved), nil
}

// ---- export ----

// ExportFormat is one way to write the list.
type ExportFormat struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Desc string `json:"desc"`
	File string `json:"file"` // the suggested file name
	Text bool   `json:"text"` // short enough to copy (share code, text)
}

// ExportFormats are the formats the export offers, for the environment's list.
func ExportFormats(a *app.App) []ExportFormat {
	base := "RimArcho"
	if e, err := a.Env(); err == nil && e.Name != "" {
		base += "-" + safeName(e.Name)
	}
	base += "-" + a.Now().In(a.Loc).Format("20060102")
	return []ExportFormat{
		{app.FormatRimSort, "RimSort", T("a ModsConfig-style XML that RimSort imports"), base + ".xml", false},
		{app.FormatRML, T("RimWorld (.rml)"), T("the game's own saved mod list; RimCrow reads it too"), base + ".rml", false},
		{app.FormatRimCrowXML, "RimCrow", T("RimCrow's ModList.xml"), base + "-ModList.xml", false},
		{app.FormatRimCrow, T("share code"), T("one line to paste in RimCrow or here"), base + ".txt", true},
		{app.FormatText, T("plain text"), T("one packageId per line, with names"), base + ".txt", true},
		{app.FormatModsConfig, "ModsConfig.xml", T("the game's file format, for copying to another computer"), "ModsConfig.xml", false},
	}
}

var unsafeRe = regexp.MustCompile(`[^\p{L}\p{N}._\-]+`)

func safeName(s string) string { return strings.Trim(unsafeRe.ReplaceAllString(s, "_"), "_") }

// ExportText renders the list in a format.
func ExportText(a *app.App, format string) (string, error) {
	b, err := a.ExportLoadout(format)
	return string(b), err
}

// DefaultExportDir is where a typed export path starts: Documents, else the home folder.
func DefaultExportDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if d := filepath.Join(h, "Documents"); dirExists(d) {
		return d
	}
	return h
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// ExportTo writes the list to a file and says what it wrote. It never writes the game's
// own ModsConfig.xml (that is save, with its preview and history), and replaces an
// existing file only when overwrite is set (the save dialog asked).
func ExportTo(a *app.App, format, path string, overwrite bool) (string, error) {
	path = strings.Trim(strings.TrimSpace(path), `"'`)
	if path == "" {
		return "", errors.New(T("no file chosen"))
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if st, err := a.Loadout(); err == nil && st.FilePath != "" {
		if game, err := filepath.Abs(st.FilePath); err == nil && strings.EqualFold(game, abs) {
			return "", errors.New(T("that is the game's own ModsConfig.xml: save writes it (with a preview and a version)"))
		}
	}
	if fi, err := os.Stat(abs); err == nil {
		if fi.IsDir() {
			return "", fmt.Errorf("%s: %s", abs, T("is a folder"))
		}
		if !overwrite {
			return "", fmt.Errorf("%s: %s", abs, T("exists; choose another name (nothing is overwritten)"))
		}
	}
	b, err := a.ExportLoadout(format)
	if err != nil {
		return "", err
	}
	if !dirExists(filepath.Dir(abs)) {
		return "", fmt.Errorf("%s: %s", filepath.Dir(abs), T("no such folder"))
	}
	if err := os.WriteFile(abs, b, 0o644); err != nil {
		return "", err
	}
	name := format
	for _, f := range ExportFormats(a) {
		if f.ID == format {
			name = f.Name
		}
	}
	st, _ := a.Loadout()
	n := 0
	if st != nil {
		n = len(st.List)
	}
	return Tf("wrote %s (%s, %d mods) to %s", filepath.Base(abs), name, n, abs), nil
}
