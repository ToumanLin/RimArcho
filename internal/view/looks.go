package view

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/looks"
)

// The looks view of the plan editor (16 §2.5, 21 §6): which Mod's textures the game shows.
// Every winner, state and prediction is decided here or in app; the page draws what it gets.
// A Mod is named by its 0-based position in the plan (looks.Provider.Pos) in reads and by
// its reference in writes. Stamp is app.LooksKey: it changes when the plan's Mods or their
// order change, and goes into every texture address so a cached picture is never stale.

// Settings of the looks view (the page reads and writes them with Setting / SetSetting).
const (
	// PlanViewKey keeps which view the plan editor shows: "" or "editor", or "looks".
	PlanViewKey = "ui.gui.plan.view"
	// LooksScopeKey keeps where the next choice is stored: "" or "plan", or "all".
	LooksScopeKey = "ui.gui.looks.scope"
	// LooksConfirmedKey is "1" once the player asked not to see the first apply's
	// confirmation again (21 §6).
	LooksConfirmedKey = "ui.gui.looks.confirmed"
)

// LooksGalleryLimit is the most rows one gallery answer carries; the rest is counted.
const LooksGalleryLimit = 2000

// Gallery filters (21 §2.4, §3).
const (
	LooksOverridden = "overridden"
	LooksOverriding = "overriding"
	LooksMixed      = "mixed"
	LooksChosen     = "chosen"
	LooksNoDef      = "nodef"
)

// LooksStyled is the gallery's Mod argument for the looks that have a style or random
// variant entry; LooksChosen is the one for the looks that have a choice.
const LooksStyled = "styles"

// LooksFilters lists the filters in the order the page offers them.
var LooksFilters = []string{LooksOverridden, LooksOverriding, LooksMixed, LooksChosen, LooksNoDef}

// LooksMod is one Mod of the plan that supplies textures.
type LooksMod struct {
	Pos        int    `json:"pos"` // 0-based position in the plan: the Mod's name in reads
	No         int    `json:"no"`  // the row number the editor shows
	ID         string `json:"id"`
	Ref        string `json:"ref"` // the Mod's name in writes
	Name       string `json:"name"`
	Kind       string `json:"kind"` // SourceKind, as the editor's rows
	Looks      int    `json:"looks"`
	Overridden int    `json:"overridden"`
	Overriding int    `json:"overriding"`
	// Generated: the row is RimArcho Looks Preference; its Looks is the number of looks with a choice,
	// and the gallery shows them with the Mod "chosen".
	Generated bool `json:"generated,omitempty"`
}

// LooksGenerated is the facts of the RimArcho Looks Preference card.
type LooksGenerated struct {
	Name string `json:"name"`
	// Entry is the 1-based position of RimArcho Looks Preference in the plan; 0: not in it, so applied
	// choices do not reach the game until an apply or a rebuild adds it.
	Entry int `json:"entry"`
	// Choices by state (21 §3.4).
	Applied   int `json:"applied"`
	NotNeeded int `json:"notNeeded"`
	Asleep    int `json:"asleep"`
	Broken    int `json:"broken"`
	// The files RimArcho wrote there last.
	Files  int    `json:"files"`
	Bytes  int64  `json:"bytes"`
	Size   string `json:"size"`
	Linked int    `json:"linked"`
	Copied int    `json:"copied"`
	// Dir is the looks directory; DirChosen: none exists yet and the first apply would
	// choose this one.
	Dir       string `json:"dir"`
	DirChosen bool   `json:"dirChosen,omitempty"`
	// Absent are the choices whose look is not in the plan now (the Mod that supplied it
	// left, or its update dropped the textures): the gallery cannot show them, so the card
	// lists them with undo. AbsentMore counts those left out.
	Absent     []LooksAbsent `json:"absent,omitempty"`
	AbsentMore int           `json:"absentMore,omitempty"`
}

// LooksAbsentLimit is the most absent choices the card lists.
const LooksAbsentLimit = 6

// LooksAbsent is one choice whose look is not in the plan.
type LooksAbsent struct {
	ID    string `json:"id"`
	State string `json:"state"` // asleep or broken
	Mod   string `json:"mod"`   // the chosen Mod's name; "" when no copy is installed
	Scope string `json:"scope"` // plan or all: what an undo names
}

// LooksOverview is the looks view's right column and its totals.
type LooksOverview struct {
	Stamp string     `json:"stamp"`
	Plan  string     `json:"plan"`
	Mods  []LooksMod `json:"mods"` // load order
	// Totals of the plan.
	Looks      int `json:"looks"`
	Overridden int `json:"overridden"` // looks where a Mod's texture is overridden by different content
	Mixed      int `json:"mixed"`
	Chosen     int `json:"chosen"`
	NoDef      int `json:"noDef"`
	// Scope is where the next choice is stored: "plan" or "all" (LooksScopeKey).
	Scope     string         `json:"scope"`
	Generated LooksGenerated `json:"generated"`
	// Styles is the second Preference row: RimArcho Styles Preference (21 §5).
	Styles   StylesGenerated `json:"styles"`
	Warnings int             `json:"warnings"` // files the index could not read
	// GameRunning: a game runs, so an apply or undo keeps the choice and does not rebuild
	// RimArcho Looks Preference until the next write to the game.
	GameRunning bool `json:"gameRunning"`
	// Confirmed: the first apply's confirmation is not shown again (LooksConfirmedKey).
	Confirmed bool `json:"confirmed"`
}

// LookThumb names one texture to draw: provider position and slot, for the texture address.
type LookThumb struct {
	Pos  int    `json:"p"`
	Slot string `json:"s"`
	// Own: the selected Mod's texture where the game shows another.
	Own bool `json:"own,omitempty"`
	// NoPreview is the file's format when no picture can be made of it (BC7, PSD).
	NoPreview string `json:"noPreview,omitempty"`
}

// LookRow is one look in the gallery.
type LookRow struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"` // plain, worn or folder
	Title     string `json:"title"`
	Providers int    `json:"n"`
	// For the selected Mod, with the choices resolved: InGame, the game shows its textures;
	// else OverriddenBy names the Mod whose texture the game shows instead.
	InGame       bool   `json:"inGame,omitempty"`
	OverriddenBy string `json:"overriddenBy,omitempty"`
	// By load order alone (21 §2.4), as the filters count them.
	Overridden bool `json:"overridden,omitempty"`
	Overriding bool `json:"overriding,omitempty"`
	Mixed      bool `json:"mixed,omitempty"`
	// Choice is the state of the look's choice: applied, not needed, asleep or broken; ""
	// without one. From names the chosen Mod.
	Choice string      `json:"choice,omitempty"`
	From   string      `json:"from,omitempty"`
	NoDef  bool        `json:"noDef,omitempty"`
	Thumbs []LookThumb `json:"thumbs"`
	More   int         `json:"more,omitempty"` // textures of a folder look not in Thumbs
}

// LooksGroup is the rows of one category.
type LooksGroup struct {
	Category string    `json:"category"`
	Rows     []LookRow `json:"rows"`
}

// LooksGallery is the looks of one Mod, of the choices, or of a search.
type LooksGallery struct {
	Stamp string `json:"stamp"`
	// Mod is the selected Mod's position; -1 with Chosen (the looks that have a choice),
	// with Styles (the looks that have a style or random variant entry) or with none of
	// them (every look, for a search).
	Mod    int    `json:"mod"`
	Chosen bool   `json:"chosen,omitempty"`
	Styles bool   `json:"styles,omitempty"`
	Filter string `json:"filter,omitempty"`
	// Counts are the rows each filter would show, with the search applied; "all" is the
	// rows without a filter.
	Counts map[string]int `json:"counts"`
	Groups []LooksGroup   `json:"groups"`
	Rows   int            `json:"rows"` // rows in Groups
	// Mods, with a search over the whole plan: the positions of the Mods whose name has
	// every word, else of the Mods that supply a matching look. The Mods card lists these.
	Mods []int `json:"mods,omitempty"`
	More int   `json:"more,omitempty"` // matching rows left out (LooksGalleryLimit)
}

// LookFileFacts is one texture file.
type LookFileFacts struct {
	Rel    string `json:"rel"` // relative to the Mod folder
	Format string `json:"format"`
	W      int    `json:"w"`
	H      int    `json:"h"`
	Bytes  int64  `json:"bytes"`
	// Alt is the second format of the same texture in this Mod ("" none).
	Alt string `json:"alt,omitempty"`
	// SameAs is the position of an earlier provider with identical content; -1 none.
	SameAs int `json:"sameAs"`
	// Preview: a picture can be made of it; false for BC7 and other DX10 DDS, and PSD.
	Preview bool `json:"preview"`
}

// LookChainRow is one provider of a slot.
type LookChainRow struct {
	Pos  int            `json:"pos"`
	File LookFileFacts  `json:"file"`
	Mask *LookFileFacts `json:"mask,omitempty"` // address it with the slot "<slot>_mask"
}

// LookSlot is one texture of a look with its override chain.
type LookSlot struct {
	Slot   string `json:"slot"`
	Body   string `json:"body,omitempty"`   // worn body type
	Facing string `json:"facing,omitempty"` // south, east, north, west or single; "" for a folder's file
	// Chain is the providers in load order. InGame is the position of the one the game
	// shows; ByChoice: because of a choice, not the load order.
	Chain    []LookChainRow `json:"chain"`
	InGame   int            `json:"inGame"`
	ByChoice bool           `json:"byChoice,omitempty"`
	// State is the state of the choice that decides this slot ("" none).
	State string `json:"state,omitempty"`
	// MaskInGame is the position of the provider whose mask the game shows; -1 no mask.
	MaskInGame int `json:"maskInGame"`
	// Contested: providers differ in content here.
	Contested bool `json:"contested,omitempty"`
}

// LookFacing is one entry of the page's "which texture" lists.
type LookFacing struct {
	Facing string `json:"facing,omitempty"`
	Slot   string `json:"slot"`
	// Mirrored: no Mod ships west, so the game draws east mirrored; Slot is the east slot.
	Mirrored  bool `json:"mirrored,omitempty"`
	Contested bool `json:"contested,omitempty"`
}

// LookBody is the facings of one worn body type, or of the look itself (Body "").
type LookBody struct {
	Body      string       `json:"body"`
	Facings   []LookFacing `json:"facings"`
	Contested bool         `json:"contested,omitempty"`
}

// LookAxes is what a look has to choose between, derived from its slots.
type LookAxes struct {
	// Ground is the texture on the ground of a worn look that also has worn textures.
	Ground *LookFacing `json:"ground,omitempty"`
	// Bodies: a plain look has one entry with Body ""; a worn look one per body type.
	Bodies []LookBody `json:"bodies,omitempty"`
	// Files are a folder look's files.
	Files []LookFacing `json:"files,omitempty"`
	// Open is the slot the page shows first: worn before on the ground, south first.
	Open string `json:"open"`
}

// LookDef is one Def field that names the look.
type LookDef struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Label    string `json:"label,omitempty"`
	Field    string `json:"field"`
	Class    string `json:"class,omitempty"` // graphic class
	Mod      string `json:"mod,omitempty"`   // the Mod that defines the Def
	ModPos   int    `json:"modPos"`
	Abstract bool   `json:"abstract,omitempty"`
}

// LookProvider is one Mod that supplies the look.
type LookProvider struct {
	Pos   int      `json:"pos"`
	No    int      `json:"no"`
	ID    string   `json:"id"`
	Ref   string   `json:"ref"`
	Name  string   `json:"name"`
	Kind  string   `json:"kind"`
	Slots []string `json:"slots"` // the textures it supplies (no masks)
	// InGame: the game shows at least one of its textures.
	InGame bool `json:"inGame,omitempty"`
}

// LookChoice is one stored choice on the look.
type LookChoice struct {
	Scope string `json:"scope"` // plan or all
	Slot  string `json:"slot"`  // all (the entire look) or a slot
	Mod   string `json:"mod"`
	Pos   int    `json:"pos"` // the chosen Mod's position; -1 asleep
	// State: applied, not needed, asleep or broken; "" when Overruled (another choice
	// decides every slot this one names).
	State     string `json:"state,omitempty"`
	Overruled bool   `json:"overruled,omitempty"`
}

// Outcomes of a predicted apply.
const (
	LookWouldNone      = "none"      // the Mod has no texture for it
	LookWouldInGame    = "inGame"    // the game already shows the Mod's textures there
	LookWouldOverruled = "overruled" // a choice of this plan decides these textures; one for all plans would not show
	LookWouldMixed     = "mixed"     // applies, and the look then takes textures from several Mods
	LookWouldApply     = "apply"     // applies
)

// LookPreview is what an apply would lead to.
type LookPreview struct {
	Outcome string `json:"outcome"`
	// Mixed: Still textures would still come from the Mods StillFrom; Text says so.
	Still     int      `json:"still,omitempty"`
	StillFrom []string `json:"stillFrom,omitempty"`
	Text      string   `json:"text,omitempty"`
}

// LookTarget is one thing an apply can name.
type LookTarget struct {
	// Kind: all (the entire look), worn (all worn textures), ground (the texture on the
	// ground), body (one body type) or slot (one texture).
	Kind   string `json:"kind"`
	Body   string `json:"body,omitempty"`
	Facing string `json:"facing,omitempty"`
	// Slots is what LooksApply takes; empty for the entire look.
	Slots []string `json:"slots"`
	// Preview is the prediction for the selected Mod; nil without one.
	Preview *LookPreview `json:"preview,omitempty"`
}

// LookDetail is everything the page shows of one look.
type LookDetail struct {
	Stamp     string         `json:"stamp"`
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	Category  string         `json:"category"`
	Title     string         `json:"title"`
	Tags      []string       `json:"tags,omitempty"`
	Defs      []LookDef      `json:"defs,omitempty"`
	Providers []LookProvider `json:"providers"` // load order
	Selected  int            `json:"selected"`  // the selected provider's position; -1 none
	Scope     string         `json:"scope"`
	Axes      LookAxes       `json:"axes"`
	Slots     []LookSlot     `json:"slots"`
	Mixed     bool           `json:"mixed,omitempty"`
	Choices   []LookChoice   `json:"choices,omitempty"`
	Targets   []LookTarget   `json:"targets"`
	// CanUndo: the look has a choice in Scope.
	CanUndo bool `json:"canUndo,omitempty"`
	// Add is the add row: what the selected Mod's version can be kept as (a style in a
	// category, a random variant), and the look's entries (21 §5).
	Add *LookAdd `json:"add"`
}

// LooksOutcome is one look of an apply or undo.
type LooksOutcome struct {
	Look  string `json:"look"`
	Title string `json:"title"`
	Error string `json:"error,omitempty"` // why it was left out
	Mixed bool   `json:"mixed,omitempty"`
}

// LooksDone is the result of an apply, an undo or a rebuild, or with DryRun what it would do
// (the first apply's confirmation shows that).
type LooksDone struct {
	DryRun bool           `json:"dryRun,omitempty"`
	Text   string         `json:"text"` // the status line
	Mod    string         `json:"mod,omitempty"`
	Scope  string         `json:"scope"`
	Plan   string         `json:"plan"`
	Items  []LooksOutcome `json:"items,omitempty"`
	Done   int            `json:"done"`  // looks applied or undone
	Mixed  int            `json:"mixed"` // of them, with a mixed result
	// Added: RimArcho Looks Preference was added to the plan (one undoable edit).
	Added bool `json:"added,omitempty"`
	// Dir is the looks directory; DirChosen: this call chose it (the first apply).
	Dir       string `json:"dir,omitempty"`
	DirChosen bool   `json:"dirChosen,omitempty"`
	// The generated Mod after the rebuild: its files, and how many are links.
	Files     int    `json:"files"`
	Bytes     int64  `json:"bytes"`
	Size      string `json:"size"`
	Linked    int    `json:"linked"`
	Copied    int    `json:"copied"`
	Unchanged bool   `json:"unchanged,omitempty"` // the rebuild had nothing to write
	// BuildError: the rebuild did not run (a game is running) or failed; the choices are kept.
	BuildError string `json:"buildError,omitempty"`
	// Overview is the plan's looks after the call (nil in a dry run).
	Overview *LooksOverview `json:"overview,omitempty"`
}

func looksScopeOf(v string) string {
	if v == app.LooksAll {
		return app.LooksAll
	}
	return app.LooksPlan
}

// LookTitle names a look: the label of a Def that uses it, else the last part of its path.
func LookTitle(l *looks.Look) string {
	best := ""
	for _, d := range l.Defs {
		if d.Label == "" {
			continue
		}
		if !d.Abstract {
			return d.Label
		}
		if best == "" {
			best = d.Label
		}
	}
	if best != "" {
		return best
	}
	return l.ID[strings.LastIndexByte(l.ID, '/')+1:]
}

func lookNoDef(l *looks.Look) bool { return len(l.Defs) == 0 }

func lookProvider(l *looks.Look, pos int) *looks.Provider {
	for _, p := range l.Providers {
		if p.Pos == pos {
			return p
		}
	}
	return nil
}

// lookTextures lists the look's texture slots in display order (no masks).
func lookTextures(l *looks.Look) []string {
	var out []string
	for _, s := range l.SlotNames() {
		if !looks.IsMask(s) {
			out = append(out, s)
		}
	}
	return out
}

func looksModName(ls *app.LooksState, pos int) string {
	if m := ls.Mods[pos]; m != nil {
		return m.DisplayName()
	}
	return ""
}

// lookChoiceState is the state a look's row shows: a broken or sleeping choice first, since
// those are what the player has to know about.
func lookChoiceState(s *app.LookState) (state, from string) {
	rank := map[string]int{app.LookBroken: 4, app.LookAsleep: 3, app.LookApplied: 2, app.LookNotNeeded: 1}
	for _, c := range s.Choices {
		if rank[c.State] > rank[state] {
			state = c.State
			from = ""
			if c.Mod != nil {
				from = c.Mod.DisplayName()
			}
		}
	}
	return state, from
}

// BuildLooksOverview counts the plan's looks per Mod.
func BuildLooksOverview(a *app.App, ls *app.LooksState) (*LooksOverview, error) {
	st := ls.St
	o := &LooksOverview{Stamp: a.LooksKey(st), Plan: st.Plan.Name, Mods: []LooksMod{}, Looks: len(ls.Index.Looks), Chosen: len(ls.Looks),
		Scope: looksScopeOf(a.Store.Meta(LooksScopeKey)), Warnings: len(ls.Index.Warnings), Confirmed: a.Store.Meta(LooksConfirmedKey) == "1"}
	type count struct{ looks, overridden, overriding int }
	counts := map[int]*count{}
	for _, l := range ls.Index.Looks {
		if lookNoDef(l) {
			o.NoDef++
		}
		if l.Mixed(ls.ChoiceOf(l.ID)) {
			o.Mixed++
		}
		any := false
		for _, p := range l.Providers {
			c := counts[p.Pos]
			if c == nil {
				c = &count{}
				counts[p.Pos] = c
			}
			c.looks++
			if len(l.Providers) < 2 {
				continue
			}
			if l.Overridden(p.Pos) {
				c.overridden++
				any = true
			}
			if l.Overriding(p.Pos) {
				c.overriding++
			}
		}
		if any {
			o.Overridden++
		}
	}
	for i, e := range st.List {
		m := st.Ix.ByID[e.Instance]
		if m == nil {
			continue
		}
		row := LooksMod{Pos: i, No: i + 1, ID: m.ID, Ref: st.Ix.Ref(m.ID), Name: m.DisplayName(), Kind: SourceKind(m)}
		switch c := counts[i]; {
		case app.IsLooks(m):
			row.Generated, row.Looks = true, len(ls.Looks)
		case c == nil:
			continue
		default:
			row.Looks, row.Overridden, row.Overriding = c.looks, c.overridden, c.overriding
		}
		o.Mods = append(o.Mods, row)
	}
	g := LooksGenerated{Name: app.LooksName, Entry: ls.Entry, Applied: ls.Applied, NotNeeded: ls.NotNeeded, Asleep: ls.Asleep, Broken: ls.Broken, Dir: a.LooksDir()}
	if g.Dir == "" {
		g.Dir, g.DirChosen = a.LooksDirDefault(st.Ix.Env.GameRoot), true
	}
	for _, s := range ls.Absent() {
		for _, c := range s.Choices {
			if c.Overruled {
				continue
			}
			if len(g.Absent) == LooksAbsentLimit {
				g.AbsentMore++
				continue
			}
			scope := app.LooksAll
			if c.Plan {
				scope = app.LooksPlan
			}
			g.Absent = append(g.Absent, LooksAbsent{ID: s.ID, State: c.State, Mod: ls.LineageName(c.Lineage), Scope: scope})
		}
	}
	files, err := a.Store.LookFiles()
	if err != nil {
		return nil, err
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
	o.Generated = g
	ss, err := a.StylesStateOf(ls)
	if err != nil {
		return nil, err
	}
	if o.Styles, err = stylesGenerated(a, ss); err != nil {
		return nil, err
	}
	if run, _ := a.RunningGame(); run != nil {
		o.GameRunning = true
	}
	return o, nil
}

func lookThumb(l *looks.Look, pos int, slot string, own bool) LookThumb {
	t := LookThumb{Pos: pos, Slot: slot, Own: own}
	if p := lookProvider(l, pos); p != nil && p.Slots[slot] != nil && looks.Shown(p.Slots[slot]) == nil {
		t.NoPreview = p.Slots[slot].Format
	}
	return t
}

// lookRow builds a gallery row; pos is the selected Mod's position, -1 none.
func lookRow(ls *app.LooksState, l *looks.Look, pos int) LookRow {
	choice := ls.ChoiceOf(l.ID)
	wins := l.InGame(choice)
	row := LookRow{ID: l.ID, Kind: l.Kind, Title: LookTitle(l), Providers: len(l.Providers), NoDef: lookNoDef(l), Mixed: l.Mixed(choice), Thumbs: []LookThumb{}}
	if s := ls.Looks[l.ID]; s != nil {
		row.Choice, row.From = lookChoiceState(s)
	}
	slots := lookTextures(l)
	if p := lookProvider(l, pos); p != nil {
		row.Overridden, row.Overriding = l.Overridden(pos), l.Overriding(pos)
		row.InGame = true
		var own []string
		for _, s := range slots {
			f := p.Slots[s]
			if f == nil {
				continue
			}
			own = append(own, s)
			if w := wins[s]; w.Pos != pos && !looks.Identical(w.File, f) && row.InGame {
				row.InGame, row.OverriddenBy = false, looksModName(ls, w.Pos)
			}
		}
		slots = own
	}
	if len(slots) == 0 {
		return row
	}
	if l.Kind == looks.KindWorn && len(slots) > 1 && slots[0] == "single" {
		slots = slots[1:] // a worn look shows a worn texture, not the one on the ground
	}
	if l.Kind == looks.KindFolder {
		for i, s := range slots {
			if i == 3 {
				row.More = len(slots) - 3
				break
			}
			row.Thumbs = append(row.Thumbs, lookThumb(l, wins[s].Pos, s, false))
		}
		return row
	}
	s := slots[0]
	w := wins[s]
	row.Thumbs = append(row.Thumbs, lookThumb(l, w.Pos, s, false))
	if p := lookProvider(l, pos); p != nil && w.Pos != pos && !looks.Identical(w.File, p.Slots[s]) {
		row.Thumbs = append(row.Thumbs, lookThumb(l, pos, s, true))
	}
	return row
}

func (r *LookRow) is(filter string) bool {
	switch filter {
	case LooksOverridden:
		return r.Overridden
	case LooksOverriding:
		return r.Overriding
	case LooksMixed:
		return r.Mixed
	case LooksChosen:
		return r.Choice != ""
	case LooksNoDef:
		return r.NoDef
	}
	return true
}

// BuildLooksGallery lists the looks of the Mod at the position mod names ("12"), the looks
// that have a choice (mod "chosen"), the looks that have a style or random variant entry
// (mod "styles"), or every look (mod ""), narrowed by filter (one of
// LooksFilters, "" none) and by search: every word must be in the look's path, a Def name
// or label, or, when no Mod is selected, a provider's name.
func BuildLooksGallery(a *app.App, ls *app.LooksState, mod, filter, search string) (*LooksGallery, error) {
	g := &LooksGallery{Stamp: a.LooksKey(ls.St), Mod: -1, Counts: map[string]int{}, Groups: []LooksGroup{}}
	switch mod {
	case "":
	case LooksChosen:
		g.Chosen = true
	case LooksStyled:
		g.Styles = true
	default:
		pos, err := strconv.Atoi(mod)
		if err != nil || pos < 0 {
			return nil, fmt.Errorf("mod %q: a position, chosen, styles or nothing", mod)
		}
		switch m := ls.St.Ix.ByID[instanceAt(ls.St, pos)]; {
		case app.IsLooks(m):
			g.Chosen = true // the RimArcho Looks Preference row shows the choices
		case app.IsStyles(m):
			g.Styles = true // and RimArcho Styles Preference the looks with an entry
		default:
			g.Mod = pos
		}
	}
	styled := map[string]bool{}
	if g.Styles {
		ss, err := a.StylesStateOf(ls)
		if err != nil {
			return nil, err
		}
		for _, e := range ss.Entries {
			styled[e.Look] = true
		}
	}
	for _, f := range LooksFilters {
		if f == filter {
			g.Filter = f
		}
		g.Counts[f] = 0
	}
	words := strings.Fields(strings.ToLower(search))
	match := func(l *looks.Look) bool {
		for _, w := range words {
			ok := strings.Contains(l.ID, w)
			for i := 0; !ok && i < len(l.Defs); i++ {
				ok = strings.Contains(strings.ToLower(l.Defs[i].Name), w) || strings.Contains(strings.ToLower(l.Defs[i].Label), w)
			}
			for i := 0; !ok && g.Mod < 0 && i < len(l.Providers); i++ {
				ok = strings.Contains(strings.ToLower(looksModName(ls, l.Providers[i].Pos)), w)
			}
			if !ok {
				return false
			}
		}
		return true
	}
	groups := map[string]int{}
	found := map[int]bool{} // the providers of the matching looks
	for _, l := range ls.Index.Looks {
		switch {
		case g.Mod >= 0 && lookProvider(l, g.Mod) == nil, g.Chosen && ls.Looks[l.ID] == nil, g.Styles && !styled[l.ID], !match(l):
			continue
		}
		for i := range l.Providers {
			found[l.Providers[i].Pos] = true
		}
		row := lookRow(ls, l, g.Mod)
		g.Counts["all"]++
		for _, f := range LooksFilters {
			if row.is(f) {
				g.Counts[f]++
			}
		}
		if !row.is(g.Filter) {
			continue
		}
		if g.Rows == LooksGalleryLimit {
			g.More++
			continue
		}
		key := strings.ToLower(l.Category)
		i, ok := groups[key]
		if !ok {
			i = len(g.Groups)
			groups[key] = i
			g.Groups = append(g.Groups, LooksGroup{Category: l.Category})
		}
		g.Groups[i].Rows = append(g.Groups[i].Rows, row)
		g.Rows++
	}
	sort.SliceStable(g.Groups, func(i, j int) bool {
		return strings.ToLower(g.Groups[i].Category) < strings.ToLower(g.Groups[j].Category)
	})
	if len(words) > 0 && g.Mod < 0 && !g.Chosen && !g.Styles {
		g.Mods = foundMods(ls, found, words)
	}
	return g, nil
}

// foundMods names the Mods a search over the plan found, in load order: those among the
// providers of the matching looks whose name has every word, else all those providers.
func foundMods(ls *app.LooksState, found map[int]bool, words []string) []int {
	named := map[int]bool{}
	for pos := range found {
		name := strings.ToLower(looksModName(ls, pos))
		ok := true
		for _, w := range words {
			ok = ok && strings.Contains(name, w)
		}
		if ok {
			named[pos] = true
		}
	}
	if len(named) > 0 {
		found = named
	}
	out := make([]int, 0, len(found))
	for pos := range found {
		out = append(out, pos)
	}
	sort.Ints(out)
	return out
}

func instanceAt(st *app.LoadoutState, pos int) string {
	if pos < 0 || pos >= len(st.List) {
		return ""
	}
	return st.List[pos].Instance
}

func lookFileFacts(f *looks.File) LookFileFacts {
	out := LookFileFacts{Rel: f.Rel, Format: f.Format, W: f.W, H: f.H, Bytes: f.Size, SameAs: f.SameAs, Preview: looks.Shown(f) != nil}
	if f.Alt != nil {
		out.Alt = f.Alt.Format
	}
	return out
}

// slotParts splits a slot of a plain or worn look into its body type and facing.
func slotParts(l *looks.Look, slot string) (body, facing string) {
	if l.Kind == looks.KindFolder {
		return "", ""
	}
	if i := strings.LastIndexByte(slot, '_'); i >= 0 {
		return slot[:i], slot[i+1:]
	}
	return "", slot
}

func lookContested(l *looks.Look, slot string) bool {
	w := l.OrderWinner(slot)
	if w == nil {
		return false
	}
	for _, p := range l.Providers {
		if f := p.Slots[slot]; f != nil && !looks.Identical(f, w.Slots[slot]) {
			return true
		}
	}
	return false
}

func lookAxes(l *looks.Look, slots []LookSlot) LookAxes {
	var ax LookAxes
	if len(slots) == 0 {
		return ax
	}
	if l.Kind == looks.KindFolder {
		for _, s := range slots {
			ax.Files = append(ax.Files, LookFacing{Slot: s.Slot, Contested: s.Contested})
		}
		ax.Open = slots[0].Slot
		return ax
	}
	ground := l.Kind == looks.KindWorn && len(slots) > 1
	at := map[string]int{}
	for _, s := range slots {
		f := LookFacing{Facing: s.Facing, Slot: s.Slot, Contested: s.Contested}
		if ground && s.Slot == "single" {
			ax.Ground = &f
			continue
		}
		i, ok := at[s.Body]
		if !ok {
			i = len(ax.Bodies)
			at[s.Body] = i
			ax.Bodies = append(ax.Bodies, LookBody{Body: s.Body})
		}
		ax.Bodies[i].Facings = append(ax.Bodies[i].Facings, f)
		ax.Bodies[i].Contested = ax.Bodies[i].Contested || f.Contested
	}
	for i := range ax.Bodies {
		var east *LookFacing
		west := false
		for j := range ax.Bodies[i].Facings {
			switch f := &ax.Bodies[i].Facings[j]; f.Facing {
			case "east":
				east = f
			case "west":
				west = true
			}
		}
		if east != nil && !west { // the game draws east mirrored (21 §2.2)
			ax.Bodies[i].Facings = append(ax.Bodies[i].Facings, LookFacing{Facing: "west", Slot: east.Slot, Mirrored: true, Contested: east.Contested})
		}
	}
	switch {
	case len(ax.Bodies) > 0:
		ax.Open = ax.Bodies[0].Facings[0].Slot
	case ax.Ground != nil:
		ax.Open = ax.Ground.Slot
	}
	return ax
}

// lookTargets lists what an apply can name on this look.
func lookTargets(l *looks.Look, ax LookAxes, slots []LookSlot) []LookTarget {
	out := []LookTarget{{Kind: "all", Slots: []string{}}}
	if len(slots) < 2 {
		return out
	}
	if ax.Ground != nil {
		var worn []string
		for _, s := range slots {
			if s.Slot != ax.Ground.Slot {
				worn = append(worn, s.Slot)
			}
		}
		out = append(out, LookTarget{Kind: "worn", Slots: worn}, LookTarget{Kind: "ground", Slots: []string{ax.Ground.Slot}})
	}
	if len(ax.Bodies) > 1 {
		for _, b := range ax.Bodies {
			t := LookTarget{Kind: "body", Body: b.Body}
			for _, f := range b.Facings {
				if !f.Mirrored {
					t.Slots = append(t.Slots, f.Slot)
				}
			}
			out = append(out, t)
		}
	}
	for _, s := range slots {
		if ax.Ground == nil || s.Slot != ax.Ground.Slot {
			out = append(out, LookTarget{Kind: "slot", Body: s.Body, Facing: s.Facing, Slots: []string{s.Slot}})
		}
	}
	return out
}

// BuildLookDetail describes one look. selected is the position of the Mod the page has
// selected (-1 none): the targets then carry its predictions for scope (plan or all).
func BuildLookDetail(a *app.App, ls *app.LooksState, id string, selected int, scope string) (*LookDetail, error) {
	l := ls.Index.ByID[strings.ToLower(strings.TrimSpace(id))]
	if l == nil {
		return nil, fmt.Errorf("no look %q in the plan", id)
	}
	st := ls.St
	d := &LookDetail{Stamp: a.LooksKey(st), ID: l.ID, Kind: l.Kind, Category: l.Category, Title: LookTitle(l), Tags: l.Tags,
		Selected: -1, Scope: looksScopeOf(scope), Slots: []LookSlot{}, Providers: []LookProvider{}}
	choice := ls.ChoiceOf(l.ID)
	wins := l.InGame(choice)
	d.Mixed = l.Mixed(choice)
	for _, r := range l.Defs {
		d.Defs = append(d.Defs, LookDef{Type: r.Type, Name: r.Name, Label: r.Label, Field: r.Field, Class: r.GraphicClass, Mod: looksModName(ls, r.ModPos), ModPos: r.ModPos, Abstract: r.Abstract})
	}
	state := ls.Looks[l.ID]
	textures := lookTextures(l)
	for _, s := range textures {
		w := wins[s]
		row := LookSlot{Slot: s, InGame: w.Pos, ByChoice: w.ByChoice, MaskInGame: -1, Contested: lookContested(l, s), Chain: []LookChainRow{}}
		row.Body, row.Facing = slotParts(l, s)
		if state != nil {
			row.State = state.Slots[s]
		}
		if m, ok := wins[looks.MaskOf(s)]; ok {
			row.MaskInGame = m.Pos
		}
		for _, p := range l.Providers {
			f := p.Slots[s]
			if f == nil {
				continue
			}
			c := LookChainRow{Pos: p.Pos, File: lookFileFacts(f)}
			if m := p.Slots[looks.MaskOf(s)]; m != nil {
				facts := lookFileFacts(m)
				c.Mask = &facts
			}
			row.Chain = append(row.Chain, c)
		}
		d.Slots = append(d.Slots, row)
	}
	for _, p := range l.Providers {
		m := ls.Mods[p.Pos]
		if m == nil {
			continue
		}
		row := LookProvider{Pos: p.Pos, No: p.Pos + 1, ID: m.ID, Ref: st.Ix.Ref(m.ID), Name: m.DisplayName(), Kind: SourceKind(m), Slots: []string{}}
		for _, s := range textures {
			if p.Slots[s] != nil {
				row.Slots = append(row.Slots, s)
				row.InGame = row.InGame || wins[s].Pos == p.Pos
			}
		}
		if p.Pos == selected {
			d.Selected = selected
		}
		d.Providers = append(d.Providers, row)
	}
	d.Axes = lookAxes(l, d.Slots)
	d.Targets = lookTargets(l, d.Axes, d.Slots)
	if d.Selected >= 0 {
		for i := range d.Targets {
			p, err := lookPreview(ls, l, d.Selected, d.Targets[i].Slots, d.Scope)
			if err != nil {
				return nil, err
			}
			d.Targets[i].Preview = p
		}
	}
	if state != nil {
		for _, c := range state.Choices {
			row := LookChoice{Scope: app.LooksAll, Slot: c.Slot, Pos: c.Pos, State: c.State, Overruled: c.Overruled}
			if c.Plan {
				row.Scope = app.LooksPlan
			}
			if c.Mod != nil {
				row.Mod = c.Mod.DisplayName()
			} else {
				row.Mod = lineageName(st, c.Lineage)
			}
			d.CanUndo = d.CanUndo || row.Scope == d.Scope
			d.Choices = append(d.Choices, row)
		}
	}
	ss, err := a.StylesStateOf(ls)
	if err != nil {
		return nil, err
	}
	d.Add = lookAdd(ss, l.ID, d.Selected)
	return d, nil
}

// lineageName names a Lineage none of whose Instances is in the plan: an installed copy's
// name, else nothing.
func lineageName(st *app.LoadoutState, lineage string) string {
	for _, m := range st.Ix.Mods {
		if m.LineageID == lineage {
			return m.DisplayName()
		}
	}
	return ""
}

// BuildLookPreview predicts an apply of the Mod at pos to slots (none: the entire look) of
// a look in scope, without writing.
func BuildLookPreview(ls *app.LooksState, id string, pos int, slots []string, scope string) (*LookPreview, error) {
	l := ls.Index.ByID[strings.ToLower(strings.TrimSpace(id))]
	if l == nil {
		return nil, fmt.Errorf("no look %q in the plan", id)
	}
	return lookPreview(ls, l, pos, slots, looksScopeOf(scope))
}

func lookPreview(ls *app.LooksState, l *looks.Look, pos int, slots []string, scope string) (*LookPreview, error) {
	would, ok, err := ls.Would(l, pos, slots, scope)
	if err != nil {
		return nil, err
	}
	p := lookProvider(l, pos)
	if !ok || p == nil {
		return &LookPreview{Outcome: LookWouldNone}, nil
	}
	named := slots
	if len(named) == 0 {
		named = lookTextures(l)
	}
	now := l.InGame(ls.ChoiceOf(l.ID))
	inGame, overruled := true, false
	for _, s := range named {
		s = strings.ToLower(strings.TrimSpace(s))
		f := p.Slots[s]
		if f == nil || looks.IsMask(s) {
			continue
		}
		if w := now[s]; w.Pos != pos && !looks.Identical(w.File, f) {
			inGame = false
		}
		if got, decided := would.Choice[s]; !decided || got != pos {
			overruled = true
		}
	}
	switch {
	case inGame:
		return &LookPreview{Outcome: LookWouldInGame}, nil
	case overruled:
		return &LookPreview{Outcome: LookWouldOverruled}, nil
	case !would.Mixed:
		return &LookPreview{Outcome: LookWouldApply}, nil
	}
	out := &LookPreview{Outcome: LookWouldMixed}
	seen := map[int]bool{}
	wins := l.InGame(would.Choice)
	for _, s := range lookTextures(l) {
		if w := wins[s]; w.Pos != pos && !looks.Identical(w.File, p.Slots[s]) {
			out.Still++
			if !seen[w.Pos] {
				seen[w.Pos] = true
				out.StillFrom = append(out.StillFrom, looksModName(ls, w.Pos))
			}
		}
	}
	out.Text = Tf("mixed result: %d textures still from %s", out.Still, strings.Join(out.StillFrom, ", "))
	return out, nil
}

// looksDone turns an app result into the page's.
func looksDone(a *app.App, r *app.LooksResult, items []app.LooksManyItem) (*LooksDone, error) {
	d := &LooksDone{DryRun: r.DryRun, Scope: r.Scope, Plan: r.Plan, Added: r.Added, Dir: r.Dir, DirChosen: r.DirChosen}
	if r.From != nil {
		d.Mod = r.From.DisplayName()
	}
	title := func(id string) string {
		if r.State != nil && r.State.Index.ByID[id] != nil {
			return LookTitle(r.State.Index.ByID[id])
		}
		return id[strings.LastIndexByte(id, '/')+1:]
	}
	for _, it := range items {
		o := LooksOutcome{Look: it.Look, Title: title(it.Look), Mixed: it.Mixed}
		if it.Err != "" {
			o.Error = Humanize(nil, it.Err)
		} else {
			d.Done++
			if it.Mixed {
				d.Mixed++
			}
		}
		d.Items = append(d.Items, o)
	}
	if b := r.Build; b != nil {
		d.Files, d.Linked, d.Copied, d.Unchanged = len(b.Files), b.Links, b.Copies, b.Unchanged
		for _, f := range b.Files {
			d.Bytes += f.Size
		}
	}
	d.Size = SizeText(d.Bytes)
	if r.BuildErr != "" {
		d.BuildError = Humanize(nil, r.BuildErr)
	}
	if r.State != nil && !r.DryRun {
		o, err := BuildLooksOverview(a, r.State)
		if err != nil {
			return nil, err
		}
		d.Overview = o
	}
	return d, nil
}

// tail adds what the player has to know after the sentence: a mixed result, and a rebuild
// that did not run.
func (d *LooksDone) tail() {
	switch {
	case d.Mixed > 0 && len(d.Items) == 1:
		d.Text += "  " + T("mixed result: some of its textures still come from another Mod")
	case d.Mixed > 0:
		d.Text += "  " + Tf("%d looks have a mixed result", d.Mixed)
	}
	if d.BuildError != "" && !d.DryRun {
		d.Text += "  " + T("the choice is kept; RimArcho Looks Preference is rebuilt at the next write to the game")
	}
}

// LooksApply chooses that the looks ids come from the Mod modRef names (21 §3.3). One look
// may name slots (none: the entire look); several are applied whole, as one undoable plan
// edit and one rebuild. dryRun writes nothing and reports what would be written.
func LooksApply(ctx context.Context, a *app.App, ids []string, modRef string, slots []string, scope string, dryRun bool) (*LooksDone, error) {
	scope = looksScopeOf(scope)
	if len(ids) == 0 {
		return nil, errors.New(T("no look is selected"))
	}
	var (
		r     *app.LooksResult
		items []app.LooksManyItem
	)
	if len(ids) == 1 {
		one, err := a.LooksApply(ctx, ids[0], modRef, slots, scope, dryRun)
		if err != nil {
			return nil, err
		}
		r, items = one, []app.LooksManyItem{{Look: one.Look, Mixed: one.Mixed}}
	} else {
		many, err := a.LooksApplyMany(ctx, ids, modRef, scope, dryRun)
		if many == nil {
			return nil, err
		}
		if err != nil && many.State == nil {
			return nil, err
		}
		r, items = many.LooksResult, many.Items
	}
	d, err := looksDone(a, r, items)
	if err != nil {
		return nil, err
	}
	if len(ids) == 1 {
		d.Text = Tf("the look %s now comes from %s", d.Items[0].Title, d.Mod)
	} else {
		d.Text = Tf("%d looks now come from %s", d.Done, d.Mod)
		if left := len(d.Items) - d.Done; left > 0 {
			d.Text += "  " + Tf("%d left out: it has no texture of them", left)
		}
	}
	d.tail()
	return d, nil
}

// LooksUndo removes the choices of the looks ids in one scope (21 §3.3).
func LooksUndo(ctx context.Context, a *app.App, ids []string, scope string, dryRun bool) (*LooksDone, error) {
	scope = looksScopeOf(scope)
	if len(ids) == 0 {
		return nil, errors.New(T("no look is selected"))
	}
	var (
		r     *app.LooksResult
		items []app.LooksManyItem
	)
	if len(ids) == 1 {
		one, err := a.LooksUndo(ctx, ids[0], scope, dryRun)
		if err != nil {
			return nil, err
		}
		r, items = one, []app.LooksManyItem{{Look: one.Look, Removed: one.Removed}}
	} else {
		many, err := a.LooksUndoMany(ctx, ids, scope, dryRun)
		if many == nil || (err != nil && many.State == nil) {
			return nil, err
		}
		r, items = many.LooksResult, many.Items
	}
	d, err := looksDone(a, r, items)
	if err != nil {
		return nil, err
	}
	d.Mixed = 0
	if len(ids) == 1 {
		d.Text = Tf("the choice for the look %s was removed", d.Items[0].Title)
	} else {
		d.Text = Tf("the choices for %d looks were removed", d.Done)
	}
	d.tail()
	return d, nil
}

// LooksRebuild rebuilds RimArcho Looks Preference for the current plan (21 §4).
func LooksRebuild(ctx context.Context, a *app.App, dryRun bool) (*LooksDone, error) {
	r, err := a.LooksRebuild(ctx, dryRun)
	if err != nil {
		return nil, err
	}
	d, err := looksDone(a, r, nil)
	if err != nil {
		return nil, err
	}
	switch {
	case d.BuildError != "":
		d.Text = d.BuildError
	case d.Unchanged || r.Build == nil:
		d.Text = T("RimArcho Looks Preference is up to date")
	default:
		d.Text = Tf("RimArcho Looks Preference rebuilt: %d files", d.Files)
	}
	return d, nil
}
