package view

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// Saves (06 §2.11, 10 §4): the left card's colonies and their saves, and the selected
// save's middle card (where it belongs, can it load, its Mod list) and right card (the
// playthrough). Everything here comes from the save head (10 §1.7 L0) and the list it
// matched; play facts wait for the full read (L1) and are left empty, never made up.

// PageSaves is the Saves page under Library (06 §2.1).
const PageSaves = "saves"

// SavesPage is the left card.
type SavesPage struct {
	Lead     string          `json:"lead"` // "4 colonies  13 saves"
	Colonies []SaveColonyRow `json:"colonies"`
	Saves    int             `json:"saves"`
	// Empty says where RimArcho looked when it found no save.
	Empty   string   `json:"empty,omitempty"`
	Folders []string `json:"folders"`
	Errors  []string `json:"errors,omitempty"`
}

// SaveColonyRow is one colony: its name and its saves, newest first.
type SaveColonyRow struct {
	Key   string    `json:"key"`
	Name  string    `json:"name"`
	Saves []SaveRow `json:"saves"`
}

// SaveRow is one save in the tree: its name and when it was played.
type SaveRow struct {
	Ref  string `json:"ref"`
	Name string `json:"name"`
	// When is the in-game day once the full read knows it, else the date it was saved.
	When string `json:"when"`
	Auto bool   `json:"auto,omitempty"`
}

// SavesRows makes the left card from a listing.
func SavesRows(l *app.SaveList) *SavesPage {
	p := &SavesPage{Colonies: []SaveColonyRow{}, Folders: []string{}, Errors: l.Errors}
	for _, f := range l.Folders {
		p.Folders = append(p.Folders, f.Dir)
	}
	for _, c := range l.Colonies {
		row := SaveColonyRow{Key: c.Key, Name: colonyName(c), Saves: []SaveRow{}}
		for _, s := range c.Saves {
			row.Saves = append(row.Saves, SaveRow{Ref: s.Ref, Name: s.Name, When: saveWhen(s), Auto: s.Auto})
		}
		p.Saves += len(c.Saves)
		p.Colonies = append(p.Colonies, row)
	}
	switch {
	case p.Saves == 0 && len(p.Folders) == 0:
		p.Empty = T("no game is added yet, so there is no Saves folder to read")
	case p.Saves == 0:
		p.Empty = T("no save in the Saves folders of your games and packs")
	default:
		p.Lead = Tf("%d colonies  %d saves", len(p.Colonies), p.Saves)
	}
	return p
}

// colonyName is the colony as shown: "Autosaves" is translated, others are names.
func colonyName(c *app.Colony) string {
	if c.Name == "Autosaves" && c.By == "name" && strings.HasSuffix(c.Key, "|autosaves") {
		return T("autosaves")
	}
	return c.Name
}

// saveWhen is a save row's time: the date it was saved (the in-game day comes with L1).
func saveWhen(s *app.Save) string {
	if s.ModTime.IsZero() {
		return ""
	}
	return ShortDate(s.ModTime)
}

// SavesMatching lists the colonies whose saves hold a Mod whose packageId or name contains
// q (lower case): the left card's search matches Mods too (06 §2.11).
func SavesMatching(l *app.SaveList, q string) []string {
	q = strings.ToLower(strings.TrimSpace(q))
	out := []string{}
	if q == "" || l == nil {
		return out
	}
	for _, c := range l.Colonies {
	saves:
		for _, s := range c.Saves {
			if s.Head == nil {
				continue
			}
			for i, id := range s.Head.ModIDs {
				if strings.Contains(id, q) || strings.Contains(strings.ToLower(s.Head.Name(i)), q) {
					out = append(out, c.Key)
					break saves
				}
			}
		}
	}
	return out
}

// SaveDetail is the selected save: the middle and right cards.
type SaveDetail struct {
	Ref    string `json:"ref"`
	Name   string `json:"name"`
	File   string `json:"file"`
	Path   string `json:"path"`
	Folder string `json:"folder"`
	Prefix string `json:"prefix,omitempty"` // the RimSaves folder its file name keeps
	Colony string `json:"colony"`
	Auto   bool   `json:"auto,omitempty"`
	Size   string `json:"size"`
	Bytes  int64  `json:"bytes"`
	Saved  string `json:"saved"` // the date and time the file was written

	// Where it belongs (10 §4.3): the game it was made in and the environment it runs in,
	// the list or pack it matched and how.
	Game     string `json:"game"`               // the save's game version
	Env      string `json:"env,omitempty"`      // the environment chip: "steam  1.6"
	ListKind string `json:"listKind,omitempty"` // "list", "pack" or "" none
	List     string `json:"list,omitempty"`
	ListID   int64  `json:"listId,omitempty"` // the plan to make current or open
	Match    string `json:"match,omitempty"`  // how it matched, as a sentence

	// Can it load? (10 §4.3): the verdict, the quick-fix bar's one action and the rows
	// that need attention. Level: "" loads, warning, error, or writing (no verdict).
	Verdict string          `json:"verdict"`
	Level   string          `json:"level"`
	Against string          `json:"against"`
	Quick   *SaveFixView    `json:"quick,omitempty"`
	Issues  []SaveIssueView `json:"issues"`

	// Mod list: the save's Mods in load order, then the list's Mods it lacks.
	Mods     []SaveModView `json:"mods"`
	Missing  int           `json:"missing"`
	Moved    int           `json:"moved"`
	ListOnly []string      `json:"listOnly"`
	NewList  *SaveFixView  `json:"newList,omitempty"` // Make a list from this save

	// Locked says why Rename and Recycle are off now (a game runs, the file is written).
	Locked string `json:"locked,omitempty"`

	// The right card. Play is nil until the full read (L1) has read the file; PlayNote
	// then says why the card has no facts yet.
	Play     *SavePlay `json:"play,omitempty"`
	PlayNote string    `json:"playNote,omitempty"`
}

// SaveFixView is one fix as a button: the core's action and target, the label translated,
// and the archo command it runs (shown as its hover text).
type SaveFixView struct {
	Action  string `json:"action"`
	Label   string `json:"label"`
	Command string `json:"command"`
	Target  string `json:"target,omitempty"`
}

// SaveIssueView is one row of Can it load?: what is wrong or changed, its consequence, and
// its own fixes. Sign marks Mod rows: − missing, + added, ↕ reordered.
type SaveIssueView struct {
	Kind        string        `json:"kind"`
	Level       string        `json:"level"`
	Sign        string        `json:"sign,omitempty"`
	Head        string        `json:"head"`
	Consequence string        `json:"consequence"`
	Mods        []string      `json:"mods,omitempty"` // names, for the row's hover text
	Fixes       []SaveFixView `json:"fixes"`
}

// SaveModView is one Mod of the save's list.
type SaveModView struct {
	Name      string `json:"name"`
	PackageID string `json:"packageId"`
	// Missing: not installed; Off: installed but not in the matched list; Moved: out of the
	// save's order in the list.
	Missing bool `json:"missing,omitempty"`
	Off     bool `json:"off,omitempty"`
	Moved   bool `json:"moved,omitempty"`
}

// SavePlay is the playthrough at this save, from the full read (10 §4.3 Play facts,
// §4.5). Not filled yet: the L1 reader (savefile scan) supplies it.
type SavePlay struct {
	Day         string          `json:"day,omitempty"` // "day 214, 5th of Aprimay 5502"
	Storyteller string          `json:"storyteller,omitempty"`
	Difficulty  string          `json:"difficulty,omitempty"`
	Played      string          `json:"played,omitempty"` // real play time
	Biome       string          `json:"biome,omitempty"`
	Stats       []SaveStat      `json:"stats,omitempty"`
	Colonists   []SaveColonist  `json:"colonists,omitempty"`
	Events      []string        `json:"events,omitempty"`
	Timeline    []SaveTimepoint `json:"timeline,omitempty"`
}

// SaveStat is one number at this save and its change since the colony's previous save.
type SaveStat struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Change string `json:"change,omitempty"`
}

// SaveColonist is one colonist; the dead are struck through.
type SaveColonist struct {
	Name string `json:"name"`
	Dead bool   `json:"dead,omitempty"`
}

// SaveTimepoint is one save of the colony on the timeline; a click opens it.
type SaveTimepoint struct {
	Ref    string  `json:"ref"`
	Day    int     `json:"day"`
	Wealth float64 `json:"wealth"`
	People int     `json:"people"`
	Deaths int     `json:"deaths,omitempty"`
}

// SaveDetailOf makes the middle and right cards from a load check; running: a game runs.
func SaveDetailOf(c *app.SaveCheck, running bool) *SaveDetail {
	s := c.Save
	d := &SaveDetail{Ref: s.Ref, Name: s.Name, File: s.File, Path: s.Path, Folder: filepath.Dir(s.Path), Prefix: s.Prefix,
		Auto: s.Auto, Size: SizeText(s.Size), Bytes: s.Size, Issues: []SaveIssueView{}, Mods: []SaveModView{}, ListOnly: []string{},
		PlayNote: T("play facts (day, storyteller, colonists, wealth, events) come with the full read of the save, which RimArcho does not do yet")}
	if s.Colony != nil {
		d.Colony = colonyName(s.Colony)
	}
	if !s.ModTime.IsZero() {
		d.Saved = LongDate(s.ModTime) + " " + s.ModTime.Local().Format("15:04")
	}
	if s.Head != nil {
		d.Game = s.Head.GameVersion
	}
	if s.Env != nil {
		d.Env = EnvChip(s.Env.Name, s.Env.GameVersion)
	}
	switch {
	case s.Plan != nil:
		d.ListKind, d.List, d.ListID = "list", s.Plan.Name, s.Plan.ID
	case s.Pack != "":
		d.ListKind, d.List = "pack", s.Pack
	}
	d.Match = matchText(s.Match)
	switch {
	case running:
		d.Locked = T("the game is running: rename and recycle wait until it is closed")
	case s.Writing:
		d.Locked = T("the game is writing this file: rename and recycle wait until it has saved")
	}

	d.Level, d.Verdict = c.Level, verdictText(c)
	switch {
	case d.ListKind == "list":
		d.Against = Tf("against %s as it is now", d.List)
	case d.ListKind == "pack":
		d.Against = Tf("against pack %s", d.List)
	default:
		d.Against = T("against the installed Mods")
	}
	if c.Quick != nil {
		q := fixView(*c.Quick, s)
		d.Quick = &q
	}
	for _, is := range c.Issues {
		d.Issues = append(d.Issues, issueView(is, s))
	}
	d.Issues = foldModRows(d.Issues, c.Issues, s)
	for _, m := range c.Mods {
		v := SaveModView{Name: m.Name, PackageID: m.PackageID, Missing: m.Instance == "", Moved: m.Moved}
		v.Off = !v.Missing && !m.InList && d.ListKind != ""
		if v.Missing {
			d.Missing++
		}
		if v.Moved {
			d.Moved++
		}
		d.Mods = append(d.Mods, v)
	}
	for _, m := range c.ListOnly {
		d.ListOnly = append(d.ListOnly, m.Name)
	}
	if s.Head != nil && s.Colony != nil {
		nl := fixView(app.SaveFix{Action: app.FixNewList, Label: "Make a list from this save",
			Command: "loadout import " + argQuote(s.Path) + " --new " + argQuote(s.Colony.Name), Target: s.Ref}, s)
		d.NewList = &nl
	}
	return d
}

// verdictText is the quick-fix bar's sentence (10 §4.3).
func verdictText(c *app.SaveCheck) string {
	s := c.Save
	switch c.Level {
	case "writing":
		return T("the game is writing this file; it is read again when it has saved")
	case app.LevelError:
		if s.Head == nil || !s.Head.Complete {
			return T("this save cannot be loaded")
		}
		return T("will not load cleanly")
	case app.LevelWarning:
		return T("loads, but content will be lost")
	}
	switch {
	case s.Plan != nil:
		return Tf("loads with %s as it is", s.Plan.Name)
	case s.Pack != "":
		return Tf("loads with pack %s", s.Pack)
	}
	return T("no list or pack matches this save")
}

func matchText(m string) string {
	switch m {
	case app.MatchPack:
		return T("saved in this pack's own folder")
	case app.MatchLaunch:
		return T("saved after a launch of this list")
	case app.MatchMods:
		return T("the same Mods as a version of this list")
	}
	return ""
}

// fixView translates a fix's label: the core's English label in lower case is the key;
// opening another save names it.
func fixView(f app.SaveFix, s *app.Save) SaveFixView {
	v := SaveFixView{Action: f.Action, Command: "archo " + f.Command, Target: f.Target}
	switch f.Action {
	case app.FixOpenSave:
		name := strings.TrimPrefix(f.Label, "Open ")
		v.Label = Tf("open %s", name)
	default:
		v.Label = T(lowerFirst(f.Label))
	}
	return v
}

func lowerFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return s
	}
	return string(unicode.ToLower(r)) + s[n:]
}

// issueView is one Can it load? row: its sentence and consequence by kind (10 §4.3).
func issueView(is app.SaveIssue, s *app.Save) SaveIssueView {
	v := SaveIssueView{Kind: is.Kind, Level: is.Level, Fixes: []SaveFixView{}}
	for _, f := range is.Fixes {
		v.Fixes = append(v.Fixes, fixView(f, s))
	}
	var names []string
	for _, m := range is.Mods {
		names = append(names, m.Name)
	}
	v.Mods = names
	name := ""
	if len(names) > 0 {
		name = names[0]
	}
	switch is.Kind {
	case app.IssueUnreadable:
		v.Head = T("the file cannot be read as a save")
		v.Consequence = T("it has no Mod list at its start, so the game cannot load it")
		if is.Text != "" {
			v.Consequence += "  (" + is.Text + ")"
		}
	case app.IssueCutOff:
		v.Head = T("the file is cut off")
		v.Consequence = T("it ends before </savegame>, usually because the game stopped while writing it; it can never load")
	case app.IssueNoList:
		v.Head = T("no list matches this save")
		v.Consequence = T("no launch of a list came before it, and no version of a list has the same Mods")
	case app.IssueGameVersion:
		v.Head = Tf("made in another game version (%s)", is.Text)
		v.Consequence = T("the game converts it when it loads; Mods made only for the old version may be missing")
	case app.IssueNotInstalled:
		v.Sign = "−"
		v.Head = Tf("%s is not installed", name)
		v.Consequence = T("what it stored in the save is removed when the save loads")
		if len(is.Mods) > 0 && is.Mods[0].SteamID == "" {
			v.Consequence += T("; it has no known Workshop page, so look for it yourself")
		}
	case app.IssueDisabled:
		v.Sign = "−"
		if s.Pack != "" {
			v.Head = Tf("%s is not in the pack", name)
		} else {
			v.Head = Tf("%s is installed but not in the list", name)
		}
		v.Consequence = T("what it stored in the save is removed when the save loads without it")
	case app.IssueAdded:
		v.Sign = "+"
		if len(names) <= 2 {
			v.Head = Tf("%s added since this save", strings.Join(names, T(", ")))
		} else {
			v.Head = Tf("%d Mods in the list are not in this save", len(names))
		}
		v.Consequence = T("usually safe: a new Mod starts fresh in an existing save")
		if len(names) > 2 {
			v.Consequence = Tf("%s and %d more; ", strings.Join(names[:2], T(", ")), len(names)-2) + v.Consequence
		}
	case app.IssueReordered:
		v.Sign = "↕"
		v.Head = Tf("%d Mods in another order", len(names))
		if len(names) == 1 {
			v.Head = Tf("%s is in another order", name)
		}
		v.Consequence = T("usually safe: the same Mods load, only the order their patches apply in differs")
	default:
		v.Head = is.Kind
		v.Consequence = is.Text
	}
	return v
}

// SaveFixText says what a fix did to the list: "default: enabled Gamma, 1 to subscribe".
func SaveFixText(r *app.SaveFixResult) string {
	var parts []string
	if n := len(r.Enable); n == 1 {
		parts = append(parts, Tf("enabled %s", r.Enable[0].Name))
	} else if n > 1 {
		parts = append(parts, Tf("enabled %d Mods", n))
	}
	if n := len(r.Subscribe); n > 0 {
		parts = append(parts, Tf("added %d to subscribe", n))
	}
	if n := len(r.Waiting); n > 0 {
		parts = append(parts, Tf("%d in the list still to download", n))
	}
	if n := len(r.Disable); n > 0 {
		parts = append(parts, Tf("disabled %d", n))
	}
	if r.Reordered > 0 {
		parts = append(parts, Tf("moved %d into the save's order", r.Reordered))
	}
	if n := len(r.Unresolved); n > 0 {
		parts = append(parts, Tf("%d without a Workshop page left out", n))
	}
	if r.Edit == nil {
		if len(parts) == 0 {
			return Tf("%s already matches the save", r.Plan)
		}
		return Tf("%s already matches the save", r.Plan) + "  " + strings.Join(parts, T(", "))
	}
	return Tf("%s fixed from %s: ", r.Plan, r.Save.Name) + strings.Join(parts, T(", "))
}

// SaveFilesPreview is the inline confirmation of Back up or Recycle (06 §6): the files and
// their sizes, then where they go.
type SaveFilesPreview struct {
	Title  string   `json:"title"`
	Lines  []string `json:"lines"`
	To     string   `json:"to"`
	Button string   `json:"button"`
}

// SaveBackupPreview lists what a backup copies and where.
func SaveBackupPreview(r *app.SaveFilesResult) *SaveFilesPreview {
	p := &SaveFilesPreview{Title: Tf("back up %d saves (%s)?", len(r.Saves), SizeText(r.Bytes)), Button: T("back up"),
		To: Tf("copies go to a new folder in %s; the saves stay where they are", filepath.Dir(r.Dir))}
	if len(r.Saves) == 1 {
		p.Title = Tf("back up %s (%s)?", r.Saves[0].Name, SizeText(r.Bytes))
	}
	for _, s := range r.Saves {
		p.Lines = append(p.Lines, s.File+"  "+SizeText(s.Size))
	}
	return p
}

// SaveTrashPreview lists what recycling moves.
func SaveTrashPreview(r *app.SaveFilesResult) *SaveFilesPreview {
	p := &SaveFilesPreview{Title: Tf("move %d saves to the recycle bin (%s)?", len(r.Saves), SizeText(r.Bytes)), Button: T("recycle"),
		To: T("they go to the recycle bin, where they can be restored; nothing is deleted for good")}
	if len(r.Saves) == 1 {
		p.Title = Tf("move %s to the recycle bin (%s)?", r.Saves[0].Name, SizeText(r.Bytes))
	}
	for _, s := range r.Saves {
		p.Lines = append(p.Lines, s.File+"  "+SizeText(s.Size))
	}
	return p
}

// SaveFilesText says what a backup or recycle did.
func SaveFilesText(r *app.SaveFilesResult, backup bool) string {
	n := len(r.Saves) - len(r.Errs)
	var t string
	switch {
	case backup && n == 1:
		t = Tf("backed up %s to %s", r.Saves[0].Name, r.Dir)
	case backup:
		t = Tf("backed up %d saves to %s", n, r.Dir)
	case n == 1 && len(r.Saves) == 1:
		t = Tf("moved %s to the recycle bin", r.Saves[0].Name)
	default:
		t = Tf("moved %d saves to the recycle bin", n)
	}
	if len(r.Errs) > 0 {
		t += "  " + Tf("%d failed: %s", len(r.Errs), strings.Join(r.Errs, T("; ")))
	}
	return t
}

// SaveNameProblem is what is wrong with a new name for s ("" none): empty, path characters,
// or a save of that name in the folder. Checked while the player types (06 §2.17).
func SaveNameProblem(s *app.Save, name string) string {
	name = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(name), ".rws"))
	switch {
	case name == "":
		return T("the name is empty")
	case strings.ContainsAny(name, `\/:*?"<>|`) || strings.Contains(name, "#§#"):
		return T(`a save name cannot hold \ / : * ? " < > |`)
	case strings.EqualFold(name, s.Name):
		return ""
	}
	file := name + ".rws"
	if s.Prefix != "" {
		file = s.Prefix + "#§#" + file
	}
	if fileThere(filepath.Join(filepath.Dir(s.Path), file)) {
		return Tf("a save named %s is already there", name)
	}
	return ""
}

// fileThere: a file or folder exists at p.
func fileThere(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// argQuote quotes a command argument that holds spaces or quotes.
func argQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " 	\"'#&|;<>()") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

// ModRowsShown is how many rows of one Mod kind (not installed, not in the list) Can it load?
// shows before it folds the rest into one row with one fix (06 §3.1 rule 8): a save of a
// thousand missing Mods stays one screen and within the page's size limits (06 §11 A18).
const ModRowsShown = 4

// foldModRows folds each kind's rows past ModRowsShown into one row: its names, the shared
// consequence and one fix for all of them (enable or subscribe them; else the rows' own).
func foldModRows(views []SaveIssueView, issues []app.SaveIssue, s *app.Save) []SaveIssueView {
	count := map[string]int{}
	for _, v := range views {
		if v.Kind == app.IssueNotInstalled || v.Kind == app.IssueDisabled {
			count[v.Kind]++
		}
	}
	type fold struct {
		at    int // where the folded row goes
		names []string
		pkgs  []string
		fix   *SaveFixView
		level string
	}
	folds := map[string]*fold{}
	seen := map[string]int{}
	var out []SaveIssueView
	for i, v := range views {
		n := count[v.Kind]
		if n <= ModRowsShown+1 || (v.Kind != app.IssueNotInstalled && v.Kind != app.IssueDisabled) {
			out = append(out, v)
			continue
		}
		seen[v.Kind]++
		if seen[v.Kind] <= ModRowsShown {
			out = append(out, v)
			continue
		}
		f := folds[v.Kind]
		if f == nil {
			f = &fold{at: len(out), level: v.Level}
			folds[v.Kind] = f
			out = append(out, SaveIssueView{}) // filled below
		}
		if levelRank(v.Level) > levelRank(f.level) {
			f.level = v.Level
		}
		f.names = append(f.names, v.Mods...)
		for _, m := range issues[i].Mods {
			f.pkgs = append(f.pkgs, m.PackageID)
		}
		for _, fx := range v.Fixes {
			switch fx.Action {
			case app.FixEnable, app.FixSubscribe:
				if f.fix == nil || f.fix.Action != "mods" {
					f.fix = &SaveFixView{Action: "mods", Label: T("enable or subscribe to these")}
				}
			default:
				if f.fix == nil {
					fx := fx
					f.fix = &fx
				}
			}
		}
	}
	for kind, f := range folds {
		v := SaveIssueView{Kind: kind, Level: f.level, Sign: "−", Mods: f.names, Fixes: []SaveFixView{}}
		if kind == app.IssueNotInstalled {
			v.Head = Tf("%d more Mods are not installed", len(f.names))
			v.Consequence = T("what they stored in the save is removed when the save loads")
		} else {
			v.Head = Tf("%d more Mods are installed but not in the list", len(f.names))
			v.Consequence = T("what they stored in the save is removed when the save loads without them")
		}
		v.Consequence = namesText(f.names, 12) + ": " + v.Consequence
		if f.fix != nil {
			fx := *f.fix
			if fx.Action == "mods" {
				fx.Target = strings.Join(f.pkgs, " ")
				fx.Command = "archo saves fix " + s.Ref + " --mod " + strings.Join(f.pkgs, " --mod ")
				if len(fx.Command) > 200 {
					fx.Command = fx.Command[:200] + "…"
				}
			}
			v.Fixes = append(v.Fixes, fx)
		}
		out[f.at] = v
	}
	return out
}

// namesText is up to max names and how many more.
func namesText(names []string, max int) string {
	if len(names) <= max {
		return strings.Join(names, T(", "))
	}
	return strings.Join(names[:max], T(", ")) + " " + Tf("and %d more", len(names)-max)
}

func levelRank(l string) int {
	switch l {
	case app.LevelError:
		return 3
	case app.LevelWarning:
		return 2
	case app.LevelInfo:
		return 1
	}
	return 0
}
