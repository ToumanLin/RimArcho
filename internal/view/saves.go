package view

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// Saves (16 §2.11, 12 §4): the left card's colonies and their saves, and the selected
// save's middle card (where it belongs, can it load, its Mod list) and right card (the
// playthrough). The left card and the middle card come from the save head (12 §1.7 L0)
// and the list it matched; the right card comes from the full read (L1, SavePlayOf) and is
// left empty until the save is read, never made up.

// PageSaves is the Saves page under Library (16 §2.1).
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
	// When is the in-game day once the full read knows it, else the date it was saved;
	// Saved is that date when When is the day (the row's hover text).
	When  string `json:"when"`
	Saved string `json:"saved,omitempty"`
	Auto  bool   `json:"auto,omitempty"`
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
			r := SaveRow{Ref: s.Ref, Name: s.Name, When: saveWhen(s), Auto: s.Auto}
			if d, ok := SaveDay(s); ok {
				r.Saved, r.When = r.When, Tf("day %d", d)
			}
			row.Saves = append(row.Saves, r)
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

// saveWhen is the date a save was written.
func saveWhen(s *app.Save) string {
	if s.ModTime.IsZero() {
		return ""
	}
	return ShortDate(s.ModTime)
}

// SaveDay is a save's in-game day from its full read's summary; false while it is not
// read, or when a malformed file broke off before its time.
func SaveDay(s *app.Save) (int, bool) {
	if s.Summary == nil || s.Summary.Malformed && s.Summary.Ticks == 0 {
		return 0, false
	}
	return s.Summary.Day, true
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

	// Where it belongs (12 §4.3): the game it was made in and the environment it runs in,
	// the list or pack it matched and how.
	Game     string `json:"game"`               // the save's game version
	Env      string `json:"env,omitempty"`      // the environment chip: "steam  1.6"
	ListKind string `json:"listKind,omitempty"` // "list", "pack" or "" none
	List     string `json:"list,omitempty"`
	ListID   int64  `json:"listId,omitempty"` // the plan to make current or open
	Match    string `json:"match,omitempty"`  // how it matched, as a sentence

	// Can it load? (12 §4.3): the verdict, the quick-fix bar's one action and the rows
	// that need attention. Level: "" loads, warning, error, or writing (no verdict).
	Verdict string          `json:"verdict"`
	Level   string          `json:"level"`
	Against string          `json:"against"`
	Quick   *SaveFixView    `json:"quick,omitempty"`
	Issues  []SaveIssueView `json:"issues"`

	// Mod list (12 §4.3, 16 §2.11): the save's Mods in load order, and how the list it is
	// checked against differs from it, order ignored: the list's Mods the save lacks
	// (added, in the list's order), then the save's Mods the list lacks (removed, in load
	// order). Without a list or pack (ListKind "") there are no changes. NewList is the
	// header's Make a list from this save.
	Mods    []SaveModView `json:"mods"`
	Changes []SaveModView `json:"changes"`
	NewList *SaveFixView  `json:"newList,omitempty"`

	// Locked says why Rename and Recycle are off now (a game runs, the file is written).
	Locked string `json:"locked,omitempty"`

	// The right card. Play is nil until the full read (L1) has read the file: Reading
	// says it is being read (the card shows skeletons), else PlayNote says why it has no
	// facts.
	Play     *SavePlay `json:"play,omitempty"`
	Reading  bool      `json:"reading,omitempty"`
	PlayNote string    `json:"playNote,omitempty"`
}

// SaveFixView is one fix as a button: the core's action and target, the label translated,
// and the archo command it runs (shown as its hover text).
type SaveFixView struct {
	Action  string `json:"action"`
	Label   string `json:"label"`
	Command string `json:"command"`
	Target  string `json:"target,omitempty"`
	Cluster string `json:"cluster,omitempty"` // open-log: the cluster to select
}

// SaveIssueView is one row of Can it load?: what is wrong or changed, its consequence, and
// its own fixes. Sign marks Mod rows: − missing, + added, ↕ reordered. Group is the heading
// it goes under: "file", "game" or "mods" (the Mod list).
type SaveIssueView struct {
	Kind        string        `json:"kind"`
	Level       string        `json:"level"`
	Group       string        `json:"group"`
	Sign        string        `json:"sign,omitempty"`
	Head        string        `json:"head"`
	Title       string        `json:"title,omitempty"` // the heading's hover text
	Consequence string        `json:"consequence"`
	Mods        []string      `json:"mods,omitempty"` // names
	Fixes       []SaveFixView `json:"fixes"`
	// Details is what View opens under the row: the Mods with their packageIds and what
	// loading without them loses, by name.
	Details *SaveIssueDetails `json:"details,omitempty"`
}

// Issue groups (16 §2.11), in the order they show.
const (
	SaveGroupFile = "file"
	SaveGroupLoad = "load" // the last load's errors in the game's log
	SaveGroupGame = "game"
	SaveGroupMods = "mods"
)

// SaveIssueDetails lists a row's Mods and what is lost without them.
type SaveIssueDetails struct {
	Mods []SaveDetailMod  `json:"mods"`
	Lost []SaveDetailLost `json:"lost,omitempty"`
}

// SaveDetailMod is one Mod of a row: its name, packageId and where to get it.
type SaveDetailMod struct {
	Name      string `json:"name"`
	PackageID string `json:"packageId"`
	Note      string `json:"note,omitempty"` // "Workshop 123" or that it has no known page
}

// SaveDetailLost is one kind of what is lost: "ThingDef", "classes"; Names the most used
// first, More how many are not named.
type SaveDetailLost struct {
	Kind  string   `json:"kind"`
	Names []string `json:"names"`
	More  int      `json:"more,omitempty"`
}

// SaveModView is one Mod of the save's list.
type SaveModView struct {
	Name      string `json:"name"`
	PackageID string `json:"packageId"`
	// Missing: not installed; Moved: out of the save's order in the list.
	Missing bool `json:"missing,omitempty"`
	Moved   bool `json:"moved,omitempty"`
	// Change against the list: SaveModAdded (in the list, not in the save), SaveModRemoved
	// (in the save, not in the list) or "".
	Change string `json:"change,omitempty"`
	// Library is the installed copy the name links to in the Mod library ("" no link).
	Library string `json:"library,omitempty"`
}

// SaveDetailOf makes the middle and right cards from a load check; running: a game runs;
// current: the save's list is its environment's current list.
func SaveDetailOf(c *app.SaveCheck, running, current bool) *SaveDetail {
	s := c.Save
	d := &SaveDetail{Ref: s.Ref, Name: s.Name, File: s.File, Path: s.Path, Folder: filepath.Dir(s.Path), Prefix: s.Prefix,
		Auto: s.Auto, Size: SizeText(s.Size), Bytes: s.Size, Issues: []SaveIssueView{}, Mods: []SaveModView{}, Changes: []SaveModView{}}
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

	gap := unchecked(c)
	d.Level, d.Verdict = c.Level, verdictText(c, gap)
	switch {
	case d.ListKind == "list" && current:
		d.Against = Tf("against the current list %s", d.List)
	case d.ListKind == "list":
		d.Against = Tf("against list %s", d.List)
	case d.ListKind == "pack":
		d.Against = Tf("against pack %s", d.List)
	default:
		d.Against = T("against the installed Mods")
	}
	var newList *SaveFixView
	if s.Head != nil && s.Colony != nil {
		nl := fixView(app.SaveFix{Action: app.FixNewList, Label: "Make a list from this save",
			Command: "loadout import " + argQuote(s.Path) + " --new " + argQuote(s.Colony.Name), Target: s.Ref}, s)
		newList, d.NewList = &nl, &nl
	}
	// The bar's one action: Recycle for a file that cannot load; while what the Mods own
	// could not be checked, a list made from the save and analysed (it indexes the save's
	// Mods); else Launch or Make a list. Fix and launch is not offered: each row has its fix.
	switch q := c.Quick; {
	case q != nil && q.Action == app.FixRecycle:
		v := fixView(*q, s)
		d.Quick = &v
	case gap != "" && newList != nil:
		v := *newList
		v.Action, v.Label = SaveFixAnalyse, T("make a list from this save to analyse its assemblies")
		v.Command += " && archo analyze --open-plan " + argQuote(s.Colony.Name)
		d.Quick = &v
	case q != nil && q.Action != app.FixAll:
		v := fixView(*q, s)
		d.Quick = &v
	}
	d.Issues = issueViews(c, s)
	var removed []SaveModView
	for _, m := range c.Mods {
		v := SaveModView{Name: m.Name, PackageID: m.PackageID, Missing: m.Instance == "", Moved: m.Moved, Library: m.Library}
		if !m.InList && d.ListKind != "" {
			v.Change = SaveModRemoved
			removed = append(removed, v)
		}
		d.Mods = append(d.Mods, v)
	}
	if d.ListKind != "" {
		for _, m := range c.ListOnly { // a pack carries its Mods: none is missing
			d.Changes = append(d.Changes, SaveModView{Name: m.Name, PackageID: m.PackageID, Missing: m.Instance == "" && d.ListKind == "list",
				Library: m.Library, Change: SaveModAdded})
		}
		d.Changes = append(d.Changes, removed...)
	}
	return d
}

// SaveModView.Change values.
const (
	SaveModAdded   = "added"
	SaveModRemoved = "removed"
)

// SaveFixAnalyse is the bar's Make a list from this save, then analyse it (the page's own
// action: SaveNewList with analyze).
const SaveFixAnalyse = "new-list-analyse"

// unchecked says what Can it load? could not check (12 §4.6): "defs" when the save's Mods
// are not analysed for the game version, "classes" when their assemblies (or the game's)
// are not indexed, "" when everything was checked or nothing needed it.
func unchecked(c *app.SaveCheck) string {
	out := ""
	note := func(o *app.SaveOwned) {
		switch {
		case o == nil:
		case o.DefsUnchecked:
			out = "defs"
		case o.ClassesUnchecked && out == "":
			out = "classes"
		}
	}
	for _, is := range c.Issues {
		if is.Kind == app.IssueNotInstalled || is.Kind == app.IssueDisabled {
			note(is.Owns)
		}
	}
	note(c.GoneOwns)
	return out
}

// verdictText is the quick-fix bar's sentence (12 §4.3), with what could not be checked.
func verdictText(c *app.SaveCheck, gap string) string {
	v := verdictOf(c)
	switch gap {
	case "defs":
		return Tf("%s; %s", v, T("Defs and assemblies not analysed yet"))
	case "classes":
		return Tf("%s; %s", v, T("assemblies not analysed yet"))
	}
	return v
}

func verdictOf(c *app.SaveCheck) string {
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
		return T("content will be lost")
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
	case app.FixEnable: // the row says which Mod and which list
		v.Label = T("enable")
	case app.FixDisableAdded:
		v.Label = T("disable")
	default:
		v.Label = T(lowerFirst(f.Label))
	}
	return v
}

// issueViews makes Can it load?'s rows (12 §4.3, 16 §2.11): the Mods that are not
// installed as one row with what they lose together, the others one row each (rows of
// installed Mods not in the list fold past ModRowsShown), by group: the file, the game,
// the Mod list.
func issueViews(c *app.SaveCheck, s *app.Save) []SaveIssueView {
	var views []SaveIssueView
	var src, gone []app.SaveIssue
	goneAt := -1
	for _, is := range c.Issues {
		if is.Kind == app.IssueNotInstalled {
			if goneAt < 0 {
				goneAt = len(views)
				views, src = append(views, SaveIssueView{}), append(src, app.SaveIssue{Kind: is.Kind})
			}
			gone = append(gone, is)
			src[goneAt].Mods = append(src[goneAt].Mods, is.Mods...)
			continue
		}
		views, src = append(views, issueView(is, s)), append(src, is)
	}
	if goneAt >= 0 {
		views[goneAt] = goneView(gone, c.GoneOwns, s)
	}
	views = foldModRows(views, src, s)
	sort.SliceStable(views, func(i, j int) bool { return groupRank(views[i].Group) < groupRank(views[j].Group) })
	return views
}

func groupRank(g string) int {
	switch g {
	case SaveGroupFile:
		return 0
	case SaveGroupLoad:
		return 1
	case SaveGroupGame:
		return 1
	}
	return 2
}

// goneView is the one row of the Mods that are not installed: their names, what loading
// without them loses (owns: theirs together, from the full read), one fix subscribing the
// ones with a Workshop page, and View's details.
func goneView(issues []app.SaveIssue, owns *app.SaveOwned, s *app.Save) SaveIssueView {
	v := SaveIssueView{Kind: app.IssueNotInstalled, Group: SaveGroupMods, Sign: "−", Fixes: []SaveFixView{}}
	var mods []app.SaveMod
	var subs []SaveFixView
	var pkgs []string
	noFacts := 0
	for _, is := range issues {
		if levelRank(is.Level) > levelRank(v.Level) {
			v.Level = is.Level
		}
		mods = append(mods, is.Mods...)
		if is.Owns != nil && is.Owns.NoFacts {
			noFacts++
		}
		for _, f := range is.Fixes {
			if f.Action == app.FixSubscribe {
				subs = append(subs, fixView(f, s))
				pkgs = append(pkgs, f.Target)
			}
		}
	}
	for _, m := range mods {
		v.Mods = append(v.Mods, m.Name)
	}
	v.Head = namesText(v.Mods, 6)
	if owns == nil && len(issues) == 1 {
		owns = issues[0].Owns
	}
	v.Consequence = goneConsequence(owns, len(mods), noFacts)
	switch {
	case len(subs) == 1:
		v.Fixes = subs
	case len(subs) > 1:
		f := SaveFixView{Action: "mods", Label: T("subscribe and enable these"), Target: strings.Join(pkgs, " "),
			Command: "archo saves fix " + s.Ref + " --mod " + strings.Join(pkgs, " --mod ")}
		if len(subs) < len(mods) {
			f.Label = Tf("subscribe and enable %d of them", len(subs))
		}
		if len(f.Command) > 200 {
			f.Command = f.Command[:200] + "…"
		}
		v.Fixes = []SaveFixView{f}
	}
	v.Details = lostDetails(mods, owns)
	return v
}

// goneConsequence says what loading without n Mods that are not installed loses; noFacts
// of them are unknown to the database, so what they own is inferred.
func goneConsequence(o *app.SaveOwned, n, noFacts int) string {
	one := n == 1
	if o == nil {
		if one {
			return T("what it stored in the save is removed when the save loads")
		}
		return T("what they stored in the save is removed when the save loads")
	}
	var b strings.Builder
	switch {
	case o.Empty() && one:
		b.WriteString(T("nothing in the save is known to need it; what else it stored is removed when the save loads"))
	case o.Empty():
		b.WriteString(T("nothing in the save is known to need them; what else they stored is removed when the save loads"))
	case one:
		b.WriteString(Tf("loading without it loses %s", strings.Join(ownedParts(o), T(", "))))
	default:
		b.WriteString(Tf("loading without them loses %s", strings.Join(ownedParts(o), T(", "))))
	}
	switch {
	case noFacts == n && one:
		b.WriteString(T("; the database has no data on this Mod"))
	case noFacts == n:
		b.WriteString(Tf("; the database has no data on these %d Mods", n))
	case noFacts > 0:
		b.WriteString(Tf("; the database has no data on %d of them", noFacts))
	case o.Inferred:
		b.WriteString(T("; inferred: nothing installed provides them"))
	}
	if o.Among > 0 {
		b.WriteString(Tf("; they may belong to the other %s without facts instead", Plural(o.Among, "Mod")))
	}
	if o.DefsUnchecked {
		b.WriteString(T("; Defs of the save's list not analysed yet"))
	}
	return b.String()
}

// lostDetails is View's list: each Mod with its packageId and Workshop page, then what is
// lost without them by kind, most used first.
func lostDetails(mods []app.SaveMod, o *app.SaveOwned) *SaveIssueDetails {
	d := &SaveIssueDetails{Mods: []SaveDetailMod{}}
	for _, m := range mods {
		note := T("no known Workshop page")
		if m.SteamID != "" {
			note = Tf("Workshop %s", m.SteamID)
		}
		d.Mods = append(d.Mods, SaveDetailMod{Name: m.Name, PackageID: m.PackageID, Note: note})
	}
	if o == nil {
		return d
	}
	if len(o.Colonists) > 0 {
		var races []string
		for _, r := range o.Colonists {
			races = append(races, Tf("%s (%d)", r.Race, r.Alive+r.Dead))
		}
		d.Lost = append(d.Lost, SaveDetailLost{Kind: T("colonists by race"), Names: races})
	}
	if len(o.Factions) > 0 {
		d.Lost = append(d.Lost, SaveDetailLost{Kind: T("factions"), Names: o.Factions})
	}
	if n := len(o.Classes); n > 0 {
		shown := o.Classes[:min(n, app.OwnedDefNames)]
		d.Lost = append(d.Lost, SaveDetailLost{Kind: T("classes"), Names: shown, More: n - len(shown)})
	}
	for _, x := range o.Defs {
		kind := x.Type
		if kind == "" {
			kind = T("Defs of an unknown type")
		}
		d.Lost = append(d.Lost, SaveDetailLost{Kind: kind, Names: append([]string{}, x.Names...), More: x.Defs - len(x.Names)})
	}
	return d
}

func lowerFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return s
	}
	return string(unicode.ToLower(r)) + s[n:]
}

// lastLoadText is the last-load row (12 §4.3): errors and warnings logged while loading,
// and the first message of the most severe class.
func lastLoadText(l *app.SaveLoad) (head, consequence string) {
	if l == nil {
		return "", ""
	}
	switch {
	case l.Errors == 1 && l.Warnings == 1:
		head = T("the last load logged 1 error and 1 warning")
	case l.Errors == 1 && l.Warnings > 1:
		head = Tf("the last load logged 1 error and %d warnings", l.Warnings)
	case l.Errors > 1 && l.Warnings == 1:
		head = Tf("the last load logged %d errors and 1 warning", l.Errors)
	case l.Errors > 1 && l.Warnings > 1:
		head = Tf("the last load logged %d errors and %d warnings", l.Errors, l.Warnings)
	case l.Errors == 1:
		head = T("the last load logged 1 error")
	case l.Errors > 1:
		head = Tf("the last load logged %d errors", l.Errors)
	case l.Warnings == 1:
		head = T("the last load logged 1 warning")
	default:
		head = Tf("the last load logged %d warnings", l.Warnings)
	}
	if !l.At.IsZero() {
		head += "  " + ShortDate(l.At) + " " + l.At.Local().Format("15:04")
	}
	consequence = T("it may load with missing or broken content")
	if l.First != "" {
		consequence += "; " + Tf("first: %s", firstLine(l.First))
	}
	if l.Kinds == 1 {
		consequence += "  " + T("(1 kind)")
	} else if l.Kinds > 1 {
		consequence += "  " + Tf("(%d kinds)", l.Kinds)
	}
	return head, consequence
}

// firstLine is a log message's first line, cut to 160 characters.
func firstLine(m string) string {
	if i := strings.IndexByte(m, '\n'); i >= 0 {
		m = m[:i]
	}
	m = strings.TrimSpace(m)
	if r := []rune(m); len(r) > 160 {
		m = string(r[:159]) + "…"
	}
	return m
}

// issueView is one Can it load? row: its sentence and consequence by kind (12 §4.3).
func issueView(is app.SaveIssue, s *app.Save) SaveIssueView {
	v := SaveIssueView{Kind: is.Kind, Level: is.Level, Group: SaveGroupMods, Fixes: []SaveFixView{}}
	switch is.Kind {
	case app.IssueUnreadable, app.IssueCutOff, app.IssueMalformed:
		v.Group = SaveGroupFile
	case app.IssueGameVersion:
		v.Group = SaveGroupGame
	case app.IssueLastLoad:
		v.Group = SaveGroupLoad
	}
	for _, f := range is.Fixes {
		fv := fixView(f, s)
		if f.Action == app.FixOpenLog && is.Load != nil {
			fv.Cluster = is.Load.Cluster
		}
		v.Fixes = append(v.Fixes, fv)
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
	case app.IssueMalformed:
		v.Head = T("the file is malformed")
		v.Consequence = T("its XML breaks off, so the game cannot load it") + "  (" + is.Text + ")"
	case app.IssueClassMissing:
		var classes []string
		if is.Owns != nil {
			classes = is.Owns.Classes
		}
		if len(classes) == 1 {
			v.Head = Tf("type %s is missing", classes[0])
		} else {
			v.Head = Tf("%d types are missing", len(classes))
		}
		if name != "" {
			v.Consequence = Tf("the game drops these objects with errors: only %s defines them, and the list does not load it", name)
		} else {
			v.Consequence = T("the game drops these objects with errors: no installed Mod nor the game defines them (inferred: a Mod update removed them, or their Mod is gone)")
		}
		if len(classes) > 1 {
			v.Consequence += "  (" + is.Text + ")"
		}
	case app.IssueNoList:
		v.Head = T("no list matches this save")
		v.Consequence = T("no launch of a list came before it, and no version of a list has the same Mods")
	case app.IssueLastLoad:
		v.Head, v.Consequence = lastLoadText(is.Load)
	case app.IssueGameVersion:
		v.Head = Tf("the game version changed since this save (%s)", is.Text)
		v.Consequence = T("the game converts it when it loads; Mods made only for the old version may be missing")
	case app.IssueNotInstalled: // issueViews makes one row of these (goneView)
		return goneView([]app.SaveIssue{is}, is.Owns, s)
	case app.IssueDisabled:
		v.Sign = "−"
		v.Head = name
		v.Title = T("installed but not in the list")
		if s.Pack != "" {
			v.Title = T("not in the pack")
		}
		v.Consequence = ownedConsequence(is.Owns, T("what it stored in the save is removed when the save loads without it"))
	case app.IssueAdded:
		v.Sign = "+"
		v.Head = namesText(names, 8)
		v.Title = T("in the list, not in this save")
		v.Consequence = T("usually safe: a new Mod starts fresh in an existing save")
	case app.IssueReordered:
		v.Sign = "↕"
		v.Head = Tf("%d Mods in another order", len(names))
		if len(names) == 1 {
			v.Head = Tf("%s is in another order", name)
		}
		v.Title = namesText(names, 12)
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

// SaveFilesPreview is the inline confirmation of Back up or Recycle (16 §6): the files and
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
// or a save of that name in the folder. Checked while the player types (16 §2.17).
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

// ModRowsShown is how many rows of installed Mods not in the list Can it load? shows before
// it folds the rest into one row with one fix (16 §3.1 rule 8): a save of a thousand such
// Mods stays one screen and within the page's size limits (16 §11 A18). The Mods that are
// not installed are one row already (goneView).
const ModRowsShown = 4

// foldModRows folds the disabled rows past ModRowsShown into one row: their names, the
// shared consequence and one fix enabling all of them (else the rows' own).
func foldModRows(views []SaveIssueView, issues []app.SaveIssue, s *app.Save) []SaveIssueView {
	count := 0
	for _, v := range views {
		if v.Kind == app.IssueDisabled {
			count++
		}
	}
	if count <= ModRowsShown+1 {
		return views
	}
	var out []SaveIssueView
	var names, pkgs []string
	var fix *SaveFixView
	at, seen, level := -1, 0, ""
	for i, v := range views {
		if v.Kind != app.IssueDisabled {
			out = append(out, v)
			continue
		}
		if seen++; seen <= ModRowsShown {
			out = append(out, v)
			continue
		}
		if at < 0 {
			at, level = len(out), v.Level
			out = append(out, SaveIssueView{}) // filled below
		}
		if levelRank(v.Level) > levelRank(level) {
			level = v.Level
		}
		names = append(names, v.Mods...)
		for _, m := range issues[i].Mods {
			pkgs = append(pkgs, m.PackageID)
		}
		for _, fx := range v.Fixes {
			switch {
			case fx.Action == app.FixEnable:
				fix = &SaveFixView{Action: "mods", Label: T("enable these")}
			case fix == nil:
				fx := fx
				fix = &fx
			}
		}
	}
	v := SaveIssueView{Kind: app.IssueDisabled, Level: level, Group: SaveGroupMods, Sign: "−", Mods: names, Fixes: []SaveFixView{},
		Head: Tf("%d more Mods are installed but not in the list", len(names)), Title: namesText(names, 40),
		Consequence: namesText(names, 12) + ": " + T("what they stored in the save is removed when the save loads without them")}
	if fix != nil {
		fx := *fix
		if fx.Action == "mods" {
			fx.Target = strings.Join(pkgs, " ")
			fx.Command = "archo saves fix " + s.Ref + " --mod " + strings.Join(pkgs, " --mod ")
			if len(fx.Command) > 200 {
				fx.Command = fx.Command[:200] + "…"
			}
		}
		v.Fixes = append(v.Fixes, fx)
	}
	out[at] = v
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
