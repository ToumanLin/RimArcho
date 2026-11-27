package view

import (
	"context"
	"errors"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// The looks editor's "add" row and the RimArcho Styles Preference card (21 §5, §6): a
// Mod's version of a look kept as a style in a style category or as a random variant.
// What can be added, and why something cannot, is decided in app (StylesState.StyleChoices);
// this file words it.

// StylesConfirmedKey is "1" once the player asked not to see the first add's confirmation
// again (21 §6).
const StylesConfirmedKey = "ui.gui.styles.confirmed"

// StylesGenerated is the facts of the RimArcho Styles Preference card.
type StylesGenerated struct {
	Name string `json:"name"`
	// Entry is the 1-based position of RimArcho Styles Preference in the plan; 0: not in
	// it, so the entries do not reach the game until an add or a rebuild adds it.
	Entry int `json:"entry"`
	// Entries by state (21 §5); they apply to all plans, the states are this plan's.
	Active  int `json:"active"`
	Skipped int `json:"skipped"`
	Broken  int `json:"broken"`
	Asleep  int `json:"asleep"`
	// The files RimArcho wrote there last (textures; the Def files are not counted).
	Files  int    `json:"files"`
	Bytes  int64  `json:"bytes"`
	Size   string `json:"size"`
	Linked int    `json:"linked"`
	Copied int    `json:"copied"`
	// Dir is the looks directory, which holds both generated Mods; DirChosen: none exists
	// yet and the first add or apply would choose this one.
	Dir       string `json:"dir"`
	DirChosen bool   `json:"dirChosen,omitempty"`
	// Confirmed: the first add's confirmation is not shown again (StylesConfirmedKey).
	Confirmed bool `json:"confirmed"`
}

// LookStyleChoice is one thing the add row offers for the selected Mod.
type LookStyleChoice struct {
	Kind string `json:"kind"` // style or variant
	// Target is what LookStyleAdd takes: a style category, or "default" or a style Def.
	Target string `json:"target"`
	Label  string `json:"label"`
	// OK: it can be added; else Why says why not and Code is the reason's code.
	OK   bool   `json:"ok"`
	Why  string `json:"why,omitempty"`
	Code string `json:"code,omitempty"`
	// Shared: the category already has a style for a thing that uses the look.
	Shared bool `json:"shared,omitempty"`
	// Note is what the player has to know before adding it.
	Note   string   `json:"note,omitempty"`
	Things []string `json:"things,omitempty"` // the thing Defs it would reach
}

// LookStyleEntry is one stored entry with its state in the plan.
type LookStyleEntry struct {
	ID     int64    `json:"id"` // what LookStyleRemove takes
	Look   string   `json:"look"`
	Kind   string   `json:"kind"`
	Target string   `json:"target"`
	Label  string   `json:"label"`
	Random bool     `json:"random,omitempty"`
	Mod    string   `json:"mod"`
	Pos    int      `json:"pos"`   // the Mod's position; -1 asleep
	State  string   `json:"state"` // active, skipped, broken or asleep
	Why    string   `json:"why,omitempty"`
	Shared bool     `json:"shared,omitempty"`
	Things []string `json:"things,omitempty"`
}

// LookAdd is the add row of one look: what the selected Mod's version can be added as,
// and the look's entries.
type LookAdd struct {
	// Styles are the style categories of the plan; StylesNone says why no style can be
	// added at all (no Mod is selected, no thing Def uses the look, no category).
	Styles     []LookStyleChoice `json:"styles"`
	StylesNone string            `json:"stylesNone,omitempty"`
	// RandomLabel is the option a style can be added with (LookStyleAdd's random).
	RandomLabel string `json:"randomLabel"`
	// Variants are the random variant targets: the default look and each style the thing
	// has; VariantsNone says why none can be added.
	Variants     []LookStyleChoice `json:"variants"`
	VariantsNone string            `json:"variantsNone,omitempty"`
	Entries      []LookStyleEntry  `json:"entries"`
}

// LookStyleDone is the result of an add or a remove, or with DryRun what it would do (the
// first add's confirmation shows that).
type LookStyleDone struct {
	DryRun  bool            `json:"dryRun,omitempty"`
	Text    string          `json:"text"` // the status line
	Removed bool            `json:"removed,omitempty"`
	Entry   *LookStyleEntry `json:"entry,omitempty"`
	Note    string          `json:"note,omitempty"`
	// Added: RimArcho Styles Preference was added to the plan (one undoable edit).
	Added bool `json:"added,omitempty"`
	// Dir is the looks directory; DirChosen: this call chose it.
	Dir       string `json:"dir,omitempty"`
	DirChosen bool   `json:"dirChosen,omitempty"`
	// The generated Mod after the rebuild: its textures, how many are links, its Def files.
	Files     int    `json:"files"`
	DefFiles  int    `json:"defFiles"`
	Bytes     int64  `json:"bytes"`
	Size      string `json:"size"`
	Linked    int    `json:"linked"`
	Copied    int    `json:"copied"`
	Unchanged bool   `json:"unchanged,omitempty"`
	// BuildError: the rebuild did not run (a game is running) or failed; the entry is kept.
	BuildError string `json:"buildError,omitempty"`
	// Overview is the plan's looks after the call (nil in a dry run).
	Overview *LooksOverview `json:"overview,omitempty"`
}

// styleWhy words a reason.
func styleWhy(r app.StyleReason) string {
	switch r.Code {
	case "":
		return ""
	case app.StyleWhyAsleep, app.StyleWhyUnknownMod:
		return T("that Mod is not in the list")
	case app.StyleWhyBroken:
		return T("that Mod no longer has a texture of this look")
	case app.StyleWhyCopy:
		return T("this list's copy of that Mod has no texture of this look")
	case app.StyleWhyNoThing:
		return T("no thing Def in the list uses this look as its graphic")
	case app.StyleWhyNoCategory:
		return Tf("the style category %s is not in the list", r.Arg)
	case app.StyleWhyNoStyle:
		return Tf("the style %s is not in the list", r.Arg)
	case app.StyleWhyKind:
		return T("only a look of one texture or one set of facings can get a random variant")
	case app.StyleWhyClass:
		return Tf("the graphic class %s cannot be turned into a random set", r.Arg)
	case app.StyleWhyFacings:
		return Tf("a look with facings needs game 1.7 for a random variant; this game is %s", r.Arg)
	case app.StyleWhyOfficial:
		return T("the texture the game shows there is not a file of a Mod in the list")
	case app.StyleWhyShown:
		return T("the game already shows that Mod's version")
	case app.StyleWhyShape:
		return Tf("that Mod has no %s texture of this look", r.Arg)
	case app.StyleWhyExists:
		return T("already added")
	}
	return r.Text()
}

// styleLabel names a kind and target.
func styleLabel(kind, target string) string {
	switch {
	case kind == app.StyleKindStyle:
		return Tf("style in %s", target)
	case target == app.StyleTargetDefault:
		return T("random variant of the default look")
	}
	return Tf("random variant of the style %s", target)
}

// styleNote is what the player has to know before adding a choice.
func styleNote(kind string, shared bool) string {
	switch {
	case kind == app.StyleKindVariant:
		return T("each item shows one variant, by its ID; items already in a save may change look")
	case shared:
		return T("this category already has a style for the thing: the game uses one of them per ideoligion, not per item")
	}
	return T("an ideoligion with this style category gives the thing this look")
}

func styleEntry(ls *app.LooksState, e *app.StyleEntryState) LookStyleEntry {
	row := LookStyleEntry{ID: e.ID, Look: e.Look, Kind: e.Kind, Target: e.Target, Label: styleLabel(e.Kind, e.Target), Random: e.Random,
		Pos: e.Pos, State: e.State, Why: styleWhy(e.Why), Shared: e.Shared, Things: e.Things}
	if e.Mod != nil {
		row.Mod = e.Mod.DisplayName()
	} else {
		row.Mod = lineageName(ls.St, e.Lineage)
	}
	return row
}

// lookAdd builds the add row of the look id for the Mod at selected (-1: none).
func lookAdd(ss *app.StylesState, id string, selected int) *LookAdd {
	add := &LookAdd{Styles: []LookStyleChoice{}, Variants: []LookStyleChoice{}, Entries: []LookStyleEntry{},
		RandomLabel: T("allow new items to receive this style at random")}
	for _, e := range ss.Of(id) {
		add.Entries = append(add.Entries, styleEntry(ss.Ls, &e))
	}
	if selected < 0 {
		add.StylesNone = T("select a Mod to add its version")
		add.VariantsNone = add.StylesNone
		return add
	}
	// none is the one reason every choice of a kind shares, when none can be added.
	none := func(list []LookStyleChoice) string {
		why := ""
		for i, c := range list {
			if c.OK || c.Code == app.StyleWhyExists || i > 0 && c.Why != why {
				return ""
			}
			why = c.Why
		}
		return why
	}
	for _, c := range ss.StyleChoices(id, selected) {
		row := LookStyleChoice{Kind: c.Kind, Target: c.Target, Label: styleLabel(c.Kind, c.Target), OK: c.Why.Code == "",
			Why: styleWhy(c.Why), Code: c.Why.Code, Shared: c.Shared, Things: c.Things}
		if row.OK {
			row.Note = styleNote(c.Kind, c.Shared)
		}
		if c.Kind == app.StyleKindStyle {
			add.Styles = append(add.Styles, row)
		} else {
			add.Variants = append(add.Variants, row)
		}
	}
	if add.StylesNone = none(add.Styles); len(add.Styles) == 0 {
		add.StylesNone = T("the list has no style category (Ideology or a Mod adds them)")
	}
	add.VariantsNone = none(add.Variants)
	return add
}

// stylesGenerated fills the RimArcho Styles Preference card.
func stylesGenerated(a *app.App, ss *app.StylesState) (StylesGenerated, error) {
	st := ss.Ls.St
	g := StylesGenerated{Name: app.StylesName, Entry: ss.Entry, Active: ss.Active, Skipped: ss.Skipped, Broken: ss.Broken, Asleep: ss.Asleep,
		Dir: a.LooksDir(), Confirmed: a.Store.Meta(StylesConfirmedKey) == "1"}
	if g.Dir == "" {
		g.Dir, g.DirChosen = a.LooksDirDefault(st.Ix.Env.GameRoot), true
	}
	files, err := a.Store.StyleFiles()
	if err != nil {
		return g, err
	}
	for _, f := range files {
		g.Files++
		g.Bytes += f.Size
		if f.Linked {
			g.Linked++
		} else {
			g.Copied++
		}
	}
	g.Size = SizeText(g.Bytes)
	return g, nil
}

// styleDone turns an app result into the page's.
func styleDone(a *app.App, r *app.StylesResult) (*LookStyleDone, error) {
	d := &LookStyleDone{DryRun: r.DryRun, Removed: r.Removed, Added: r.Added, Dir: r.Dir, DirChosen: r.DirChosen}
	if r.Entry != nil && r.State != nil {
		e := styleEntry(r.State.Ls, r.Entry)
		d.Entry = &e
	}
	if b := r.Build; b != nil {
		d.Files, d.DefFiles, d.Linked, d.Copied, d.Unchanged = len(b.Files), len(b.Texts), b.Links, b.Copies, b.Unchanged
		for _, f := range b.Files {
			d.Bytes += f.Size
		}
	}
	d.Size = SizeText(d.Bytes)
	if r.BuildErr != "" {
		d.BuildError = Humanize(nil, r.BuildErr)
	}
	if r.State != nil && !r.DryRun {
		o, err := BuildLooksOverview(a, r.State.Ls)
		if err != nil {
			return nil, err
		}
		d.Overview = o
	}
	return d, nil
}

// styleErr words a refused add.
func styleErr(err error) error {
	var ref *app.ErrStyleRefused
	if errors.As(err, &ref) {
		return errors.New(styleWhy(ref.Why))
	}
	return err
}

// LookStyleAdd keeps the Mod modRef's version of a look as a style in the category target
// (kind "style", with random: also in the thing's random styles) or as a random variant of
// target (kind "variant": "default" or a style Def). dryRun writes nothing and reports what
// would be written.
func LookStyleAdd(ctx context.Context, a *app.App, lookID, modRef, kind, target string, random, dryRun bool) (*LookStyleDone, error) {
	r, err := a.StyleAdd(ctx, app.StyleAddInput{Look: lookID, Mod: modRef, Kind: kind, Target: target, Random: random}, dryRun)
	if r == nil || err != nil && r.State == nil {
		return nil, styleErr(err)
	}
	d, err := styleDone(a, r)
	if err != nil {
		return nil, err
	}
	d.Text = Tf("%s added for all lists", d.Entry.Label)
	d.Note = styleNote(r.Entry.Kind, r.Entry.Shared)
	d.tail()
	return d, nil
}

// LookStyleRemove deletes one entry.
func LookStyleRemove(ctx context.Context, a *app.App, id int64, dryRun bool) (*LookStyleDone, error) {
	r, err := a.StyleRemove(ctx, id, dryRun)
	if r == nil || err != nil && r.State == nil {
		return nil, err
	}
	d, err := styleDone(a, r)
	if err != nil {
		return nil, err
	}
	d.Text = Tf("%s removed", d.Entry.Label)
	if r.Entry.Kind == app.StyleKindStyle {
		d.Note = T("a save with items in this style logs a missing Def when it loads and shows them in their default look")
	}
	d.tail()
	return d, nil
}

// tail adds a rebuild that did not run.
func (d *LookStyleDone) tail() {
	if d.BuildError != "" && !d.DryRun {
		d.Text += "  " + T("the entry is kept; RimArcho Styles Preference is rebuilt at the next write to the game")
	}
}
