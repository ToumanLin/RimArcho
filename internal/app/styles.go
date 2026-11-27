package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/looks"
	"github.com/ToumanLin/RimArcho/internal/oplog"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Styles (21 §5): entries that keep a Mod's version of a look beside the one the game
// shows, as a style in a style category or as a random variant. They apply to all plans;
// the generated Mod RimArcho Styles Preference carries them into the game (stylesbuild.go
// writes its Defs and patches, generated.go builds it).

const (
	// StylesPackage is the packageId of the generated Mod.
	StylesPackage = "rimarcho.styles"
	// StylesName is the generated Mod's name, as the game and RimArcho show it.
	StylesName = "RimArcho Styles Preference"
	// StylesLink is its link name in Mods/ (09 §3).
	StylesLink   = DeployPrefix + "Styles"
	stylesFolder = "Styles"
	// StylesAddOp names the plan edit of the first add in the undo stack (21 §5 "In the plan").
	StylesAddOp = "add " + StylesName + " last (looks)"
)

// Kinds and targets of an entry.
const (
	StyleKindStyle   = store.StyleKindStyle
	StyleKindVariant = store.StyleKindVariant
	// StyleTargetDefault is a random variant's target that is the thing Def's own graphic.
	StyleTargetDefault = "default"
)

// States of an entry (21 §5 "States").
const (
	StyleActive  = "active"
	StyleSkipped = "skipped" // something it needs is not in the plan, or it cannot be built for it
	StyleBroken  = "broken"  // the Mod no longer supplies the look
	StyleAsleep  = "asleep"  // the Mod is not in the plan
)

// Reasons an entry is not active, or an add is refused (StyleReason.Code).
const (
	StyleWhyAsleep     = "asleep"     // the Mod is not in the plan
	StyleWhyBroken     = "broken"     // the Mod has no texture of the look
	StyleWhyNoThing    = "nothing"    // no thing Def of the plan uses the look
	StyleWhyNoCategory = "category"   // the style category is not in the plan (Arg)
	StyleWhyNoStyle    = "style"      // the target style is not a style of the look's thing Defs (Arg)
	StyleWhyKind       = "kind"       // a worn or folder look has no random variant
	StyleWhyClass      = "class"      // the graphic class cannot become a random set (Arg)
	StyleWhyFacings    = "facings"    // a look with facings needs game 1.7 (Arg: the plan's game)
	StyleWhyOfficial   = "official"   // the target's texture is no file of a Mod of the plan
	StyleWhyShown      = "shown"      // the game already shows this Mod's version
	StyleWhyShape      = "shape"      // the Mod lacks a texture the target has (Arg: the slot)
	StyleWhyExists     = "exists"     // the same entry is already stored
	StyleWhyUnknownMod = "unknownmod" // an add names a Mod that is not in the plan
)

// StyleReason says why an entry is not active or why an add is refused: a code the
// interfaces translate, and the name it is about.
type StyleReason struct{ Code, Arg string }

// Text is the reason as a sentence (the CLI's wording; the window translates Code).
func (r StyleReason) Text() string {
	switch r.Code {
	case StyleWhyAsleep, StyleWhyUnknownMod:
		return "that Mod is not in the plan"
	case StyleWhyBroken:
		return "that Mod no longer has a texture of the look"
	case StyleWhyNoThing:
		return "no thing Def of the plan uses the look as its graphic"
	case StyleWhyNoCategory:
		return "the style category " + r.Arg + " is not in the plan"
	case StyleWhyNoStyle:
		return "the style " + r.Arg + " is not a style of a thing that uses the look in this plan"
	case StyleWhyKind:
		return "only a look of one texture or one set of facings can get a random variant, not a worn look or a folder"
	case StyleWhyClass:
		return "the graphic class " + r.Arg + " cannot be turned into a random set"
	case StyleWhyFacings:
		return "a look with facings needs game 1.7 for a random variant (the plan's game is " + r.Arg + ")"
	case StyleWhyOfficial:
		return "the texture the game shows there is not a file of a Mod in the plan"
	case StyleWhyShown:
		return "the game already shows that Mod's version"
	case StyleWhyShape:
		return "that Mod has no " + r.Arg + " texture of the look"
	case StyleWhyExists:
		return "this entry already exists"
	}
	return ""
}

// StyleEntryState is one stored entry with its state in this plan.
type StyleEntryState struct {
	store.LookStyle
	Mod    *store.Mod // the Lineage's Instance in the plan; nil: asleep
	Pos    int        // its 0-based position in the plan (-1: asleep)
	State  string
	Why    StyleReason // "" Code when active
	Things []string    // active: the thing Defs it reaches
	// Shared: a style whose category already has a style for one of those things, so the
	// game uses one of them per ideoligion, not per item.
	Shared bool
}

// StylesState is the entries resolved against a plan's looks.
type StylesState struct {
	Ls      *LooksState
	Entries []StyleEntryState
	// Entry is the 1-based position of RimArcho Styles Preference in the plan; 0: not in
	// it, so the entries do not reach the game.
	Entry                           int
	Active, Skipped, Broken, Asleep int

	files []LooksFile
	texts []GenText
}

// Files are the textures RimArcho Styles Preference holds for this state.
func (ss *StylesState) Files() []LooksFile { return ss.files }

// Texts are its Def and patch files.
func (ss *StylesState) Texts() []GenText { return ss.texts }

// Of lists the entries on one look.
func (ss *StylesState) Of(look string) []StyleEntryState {
	var out []StyleEntryState
	for _, e := range ss.Entries {
		if e.Look == look {
			out = append(out, e)
		}
	}
	return out
}

// StylesState builds the index of st's plan and resolves the stored entries against it.
func (a *App) StylesState(ctx context.Context, st *LoadoutState, progress func(done, total int)) (*StylesState, error) {
	ls, err := a.LooksState(ctx, st, progress)
	if err != nil {
		return nil, err
	}
	return a.StylesStateOf(ls)
}

// StylesStateOf resolves the stored entries against ls. It reads the entries only.
func (a *App) StylesStateOf(ls *LooksState) (*StylesState, error) {
	rows, err := a.Store.LookStyles()
	if err != nil {
		return nil, err
	}
	return stylesState(ls, rows), nil
}

// styleThing is one thing Def an entry reaches.
type styleThing struct {
	name   string
	t      *looks.Thing
	ground bool // the look is its graphicData/texPath
	worn   bool // the look is its apparel/wornGraphicPath
}

// styleThings lists the concrete thing Defs that use l as their graphic or worn graphic,
// by name. A Def a later Mod replaced with another path does not count.
func styleThings(idx *looks.Index, l *looks.Look) []styleThing {
	var out []styleThing
	at := map[string]int{}
	for _, d := range l.Defs {
		if d.Type != "ThingDef" || d.Abstract || strings.ContainsAny(d.Name, `"'<>&`) {
			continue
		}
		t := idx.Things[d.Name]
		if t == nil || t.ModPos != d.ModPos {
			continue
		}
		i, ok := at[d.Name]
		if !ok {
			i = len(out)
			at[d.Name] = i
			out = append(out, styleThing{name: d.Name, t: t})
		}
		switch d.Field {
		case "graphicData/texPath":
			out[i].ground = true
		case "apparel/wornGraphicPath":
			out[i].worn = true
		}
	}
	kept := out[:0]
	for _, th := range out {
		if th.ground || th.worn {
			kept = append(kept, th)
		}
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].name < kept[j].name })
	return kept
}

// styleClass is the last part of a graphic class name ("Verse.Graphic_Single").
func styleClass(class string) string {
	return class[strings.LastIndexByte(class, '.')+1:]
}

// facingsGame reports whether the game version mm draws a random set of facings: from 1.7
// a folder's files are grouped by the name before the first "_" (21 §5).
func facingsGame(mm string) bool {
	major, minor, ok := strings.Cut(mm, ".")
	a, err1 := strconv.Atoi(major)
	b, err2 := strconv.Atoi(minor)
	return ok && err1 == nil && err2 == nil && (a > 1 || a == 1 && b >= 7)
}

// variantTarget is one graphicData a random variant can replace.
type variantTarget struct {
	defType, defName string      // ThingDef and the thing, or ThingStyleDef and the style
	look             *looks.Look // the look that graphicData draws now; nil: no Mod's file
	class            string
}

// variantTargets lists what the entry's target names: for "default" every thing Def whose
// graphic is l, else the one style.
func variantTargets(idx *looks.Index, l *looks.Look, target string) []variantTarget {
	var out []variantTarget
	for _, th := range styleThings(idx, l) {
		if !th.ground {
			continue
		}
		if target == StyleTargetDefault {
			out = append(out, variantTarget{"ThingDef", th.name, l, th.t.GraphicClass})
			continue
		}
		for _, sr := range idx.Styles[th.name] {
			if sr.StyleDef != target || strings.ContainsAny(target, `"'<>&`) {
				continue
			}
			vt := variantTarget{defType: "ThingStyleDef", defName: target, look: idx.ByID[sr.LookID]}
			if vt.look != nil {
				for _, d := range vt.look.Defs {
					if d.Type == "ThingStyleDef" && d.Name == target && d.Field == "graphicData/texPath" {
						vt.class = d.GraphicClass
					}
				}
			}
			return []variantTarget{vt}
		}
	}
	return out
}

// variantCheck says whether the Mod at pos can be a random variant of vt with l's
// textures: the target is one texture or (from game 1.7) one set of facings drawn by a
// class that maps onto Graphic_Random, a Mod of the plan supplies what the game shows
// there now, and the Mod at pos has the same textures and is not what the game shows.
func variantCheck(ls *LooksState, l *looks.Look, pos int, vt variantTarget) StyleReason {
	if l.Kind != looks.KindPlain || vt.look != nil && vt.look.Kind != looks.KindPlain {
		return StyleReason{Code: StyleWhyKind}
	}
	if vt.look == nil {
		return StyleReason{Code: StyleWhyOfficial}
	}
	multi := false
	switch styleClass(vt.class) {
	case "Graphic_Single":
	case "Graphic_Multi":
		if multi = true; !facingsGame(ls.St.GameMM()) {
			return StyleReason{StyleWhyFacings, ls.St.GameMM()}
		}
	default:
		return StyleReason{StyleWhyClass, vt.class}
	}
	p := lookProviderAt(l, pos)
	wins := vt.look.InGame(ls.ChoiceOf(vt.look.ID))
	shown := vt.look == l
	need := []string{"single"}
	if multi {
		need = []string{"south"}
	}
	for _, slot := range need {
		if wins[slot].File == nil {
			return StyleReason{Code: StyleWhyOfficial}
		}
		if p.Slots[slot] == nil {
			return StyleReason{StyleWhyShape, slot}
		}
	}
	for slot, w := range wins {
		if !looks.IsMask(slot) && w.Pos != pos && !looks.Identical(w.File, p.Slots[slot]) {
			shown = false
		}
	}
	if shown {
		return StyleReason{Code: StyleWhyShown}
	}
	return StyleReason{}
}

func lookProviderAt(l *looks.Look, pos int) *looks.Provider {
	for _, p := range l.Providers {
		if p.Pos == pos {
			return p
		}
	}
	return nil
}

// resolve decides one entry's state against ls: asleep, broken, skipped or active, with
// the reason and, for an active one, the thing Defs it reaches.
func (e *StyleEntryState) resolve(ls *LooksState, byLineage map[string]int) {
	e.State, e.Why, e.Things, e.Shared, e.Pos, e.Mod = "", StyleReason{}, nil, false, -1, nil
	idx := ls.Index
	pos, ok := byLineage[e.Lineage]
	if !ok {
		e.State, e.Why = StyleAsleep, StyleReason{Code: StyleWhyAsleep}
		return
	}
	e.Pos, e.Mod = pos, ls.Mods[pos]
	l := idx.ByID[e.Look]
	if l == nil || lookProviderAt(l, pos) == nil {
		e.State, e.Why = StyleBroken, StyleReason{Code: StyleWhyBroken}
		return
	}
	skip := func(code, arg string) { e.State, e.Why = StyleSkipped, StyleReason{code, arg} }
	things := styleThings(idx, l)
	switch e.Kind {
	case StyleKindStyle:
		has := false
		for _, c := range idx.Categories {
			has = has || c == e.Target
		}
		for _, th := range things {
			if th.t.GraphicClass == "" && th.ground {
				continue // a thing with no graphic class cannot be given a style Def
			}
			e.Things = append(e.Things, th.name)
			for _, sr := range idx.Styles[th.name] {
				e.Shared = e.Shared || sr.Category == e.Target
			}
		}
		switch {
		case len(e.Things) == 0:
			skip(StyleWhyNoThing, "")
		case !has || strings.ContainsAny(e.Target, `"'<>&`):
			skip(StyleWhyNoCategory, e.Target)
		}
	case StyleKindVariant:
		targets := variantTargets(idx, l, e.Target)
		if len(targets) == 0 {
			if e.Target == StyleTargetDefault || l.Kind != looks.KindPlain {
				code := StyleWhyNoThing
				if l.Kind != looks.KindPlain {
					code = StyleWhyKind
				}
				skip(code, "")
			} else {
				skip(StyleWhyNoStyle, e.Target)
			}
			break
		}
		var why StyleReason
		for _, vt := range targets {
			if r := variantCheck(ls, l, pos, vt); r.Code != "" {
				why = r
				continue
			}
			e.Things = append(e.Things, vt.defName)
		}
		if len(e.Things) == 0 {
			skip(why.Code, why.Arg)
		}
	default:
		skip(StyleWhyNoThing, "")
	}
	if e.State == "" {
		e.State = StyleActive
	} else {
		e.Things, e.Shared = nil, false
	}
}

func lineagePositions(ls *LooksState) map[string]int {
	out := map[string]int{}
	for i := range ls.St.List {
		if m := ls.Mods[i]; m != nil {
			if _, ok := out[m.LineageID]; !ok {
				out[m.LineageID] = i
			}
		}
	}
	return out
}

// stylesState resolves rows against ls and plans the generated Mod's files.
func stylesState(ls *LooksState, rows []store.LookStyle) *StylesState {
	ss := &StylesState{Ls: ls, Entry: ls.StylesEntry}
	byLineage := lineagePositions(ls)
	for _, r := range rows {
		e := StyleEntryState{LookStyle: r}
		e.resolve(ls, byLineage)
		switch e.State {
		case StyleActive:
			ss.Active++
		case StyleSkipped:
			ss.Skipped++
		case StyleBroken:
			ss.Broken++
		case StyleAsleep:
			ss.Asleep++
		}
		ss.Entries = append(ss.Entries, e)
	}
	ss.files, ss.texts = stylesPlan(ss)
	return ss
}

// StyleChoice is one thing the "add" row can offer for a look and a Mod.
type StyleChoice struct {
	Kind   string // StyleKindStyle or StyleKindVariant
	Target string // a style category; "default" or a style Def
	// Why: the add would be refused, and why ("" Code: it can be added).
	Why StyleReason
	// Shared: the category already has a style for a thing that uses the look.
	Shared bool
	// Things are the thing Defs the entry would reach.
	Things []string
}

// StyleChoices lists what can be added for the look id from the Mod at pos (-1: no Mod is
// selected, so only what does not depend on the Mod is checked): a style in each style
// category of the plan, and a random variant of the default graphic and of each style the
// look's thing Defs have. A choice that would be refused carries the reason.
func (ss *StylesState) StyleChoices(id string, pos int) []StyleChoice {
	ls := ss.Ls
	l := ls.Index.ByID[id]
	if l == nil {
		return nil
	}
	var lineage string
	byLineage := lineagePositions(ls)
	if m := ls.Mods[pos]; m != nil && lookProviderAt(l, pos) != nil {
		lineage = m.LineageID
	}
	try := func(kind, target string) StyleChoice {
		c := StyleChoice{Kind: kind, Target: target}
		e := StyleEntryState{LookStyle: store.LookStyle{Kind: kind, Look: l.ID, Lineage: lineage, Target: target}}
		if lineage == "" {
			c.Why = StyleReason{Code: StyleWhyBroken}
			if ls.Mods[pos] == nil {
				c.Why = StyleReason{Code: StyleWhyUnknownMod}
			}
			return c
		}
		e.resolve(ls, byLineage)
		c.Why, c.Shared, c.Things = e.Why, e.Shared, e.Things
		for _, o := range ss.Entries {
			if o.Kind == kind && o.Look == l.ID && o.Lineage == lineage && o.Target == target {
				c.Why = StyleReason{Code: StyleWhyExists}
			}
		}
		return c
	}
	var out []StyleChoice
	for _, cat := range ls.Index.Categories {
		out = append(out, try(StyleKindStyle, cat))
	}
	out = append(out, try(StyleKindVariant, StyleTargetDefault))
	seen := map[string]bool{}
	for _, th := range styleThings(ls.Index, l) {
		for _, sr := range ls.Index.Styles[th.name] {
			if th.ground && !seen[sr.StyleDef] {
				seen[sr.StyleDef] = true
				out = append(out, try(StyleKindVariant, sr.StyleDef))
			}
		}
	}
	return out
}

// StyleAddInput names a new entry.
type StyleAddInput struct {
	Look   string
	Mod    string // a reference to a Mod of the current plan
	Kind   string // StyleKindStyle or StyleKindVariant
	Target string // a style category; "default" (or "") or a style Def
	Random bool   // style: new items may receive it at random
}

// StylesResult is what an add, a remove or a rebuild did, or with DryRun would do.
type StylesResult struct {
	DryRun bool
	Plan   string
	// Entry is the entry added or removed (nil for a rebuild), with its state after the call.
	Entry   *StyleEntryState
	Removed bool
	// Added: RimArcho Styles Preference was added to the plan, or moved to its place at the
	// end, as one undoable edit.
	Added bool
	// Dir is the looks directory; DirChosen: this call chose it.
	Dir       string
	DirChosen bool
	Build     *LooksBuild // nil when the rebuild did not run
	// BuildErr says why the rebuild did not run or failed (a game is running); the entries
	// are kept and the next write to the game rebuilds.
	BuildErr string
	State    *StylesState // the plan's entries after the call
}

// ErrStyleRefused wraps the reason an add is refused; errors.As gives the StyleReason.
type ErrStyleRefused struct {
	Why  StyleReason
	Look string
}

func (e *ErrStyleRefused) Error() string {
	return "the look " + e.Look + " cannot be added like this: " + e.Why.Text()
}

// StyleAdd stores a new entry (21 §5): the Mod's version of the look as a style in a style
// category, or as a random variant of the default graphic or of a style. The entry applies
// to all plans. RimArcho Styles Preference becomes one of the current plan's last entries
// as one undoable edit and is rebuilt. An entry that would not be active in the current
// plan is refused with the reason. dryRun writes nothing.
func (a *App) StyleAdd(ctx context.Context, in StyleAddInput, dryRun bool) (*StylesResult, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	ls, err := a.LooksState(ctx, st, nil)
	if err != nil {
		return nil, err
	}
	rows, err := a.Store.LookStyles()
	if err != nil {
		return nil, err
	}
	l := ls.Index.ByID[strings.ToLower(strings.TrimSpace(in.Look))]
	if l == nil {
		return nil, fmt.Errorf("no look %q in plan:%s (archo looks)", in.Look, st.Plan.Name)
	}
	m, err := st.Ix.ResolveOne(in.Mod)
	if err != nil {
		return nil, err
	}
	if st.List.Index(m.ID) < 0 || IsGenerated(m) {
		return nil, fmt.Errorf("%s %s is not in plan:%s", st.Ix.Ref(m.ID), m.DisplayName(), st.Plan.Name)
	}
	row := store.LookStyle{Kind: in.Kind, Look: l.ID, Lineage: m.LineageID, Target: strings.TrimSpace(in.Target), Random: in.Random, At: a.Now().UTC()}
	switch row.Kind {
	case StyleKindStyle:
		if row.Target == "" {
			return nil, errors.New("name the style category (archo looks style choices " + l.ID + " --from <mod>)")
		}
	case StyleKindVariant:
		if row.Random = false; row.Target == "" {
			row.Target = StyleTargetDefault
		}
	default:
		return nil, fmt.Errorf("kind %q: style or variant", in.Kind)
	}
	for _, o := range rows {
		if o.Kind == row.Kind && o.Look == row.Look && o.Lineage == row.Lineage && o.Target == row.Target {
			return nil, &ErrStyleRefused{StyleReason{Code: StyleWhyExists}, l.ID}
		}
	}
	entry := StyleEntryState{LookStyle: row}
	if entry.resolve(ls, lineagePositions(ls)); entry.State != StyleActive {
		return nil, &ErrStyleRefused{entry.Why, l.ID}
	}
	if fs := a.modsVolumeFS(st.Ix); noLinks(fs) {
		return nil, fmt.Errorf("Mods/ is on %s, which cannot hold the link %s needs (09 §3)", fs, StylesName)
	}
	r := &StylesResult{DryRun: dryRun, Plan: st.Plan.Name, Entry: &entry}
	if dryRun {
		// the ID the add would get: the style's texture folder is named by it
		if row.ID, err = a.Store.NextLookStyleID(); err != nil {
			return nil, err
		}
		entry.ID = row.ID
		return a.stylesPreview(r, ls, append(rows, row), true)
	}
	if _, r.Dir, r.DirChosen, err = a.ensureGenMod(stylesGen, st.Ix.Env.GameRoot); err != nil {
		return nil, err
	}
	if entry.LookStyle, err = a.Store.AddLookStyle(row); err != nil {
		return nil, err
	}
	a.logStyle(st, &entry, "add: "+styleText(&entry)+" from "+m.DisplayName())
	if r.Added, err = a.genLast(stylesGen); err != nil {
		return r, err
	}
	return r, a.stylesAfter(ctx, r)
}

// styleText names an entry's kind and target.
func styleText(e *StyleEntryState) string {
	switch {
	case e.Kind == StyleKindStyle && e.Random:
		return "style in " + e.Target + ", also at random"
	case e.Kind == StyleKindStyle:
		return "style in " + e.Target
	case e.Target == StyleTargetDefault:
		return "random variant of the default look"
	}
	return "random variant of the style " + e.Target
}

// StyleRemove deletes one entry (21 §5); the plan keeps its RimArcho Styles Preference
// entry, and the entry's Defs and files go with the rebuild. dryRun writes nothing.
func (a *App) StyleRemove(ctx context.Context, id int64, dryRun bool) (*StylesResult, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	ls, err := a.LooksState(ctx, st, nil)
	if err != nil {
		return nil, err
	}
	rows, err := a.Store.LookStyles()
	if err != nil {
		return nil, err
	}
	r := &StylesResult{DryRun: dryRun, Plan: st.Plan.Name, Removed: true}
	var rest []store.LookStyle
	for _, o := range rows {
		if o.ID != id {
			rest = append(rest, o)
			continue
		}
		e := StyleEntryState{LookStyle: o}
		e.resolve(ls, lineagePositions(ls))
		r.Entry = &e
	}
	if r.Entry == nil {
		return nil, fmt.Errorf("no style entry %d (archo looks style list)", id)
	}
	if dryRun {
		return a.stylesPreview(r, ls, rest, false)
	}
	if _, err := a.Store.DeleteLookStyle(id); err != nil {
		return nil, err
	}
	a.logStyle(st, r.Entry, "remove: "+styleText(r.Entry))
	return r, a.stylesAfter(ctx, r)
}

// StylesRebuild rebuilds RimArcho Styles Preference for the current plan. When the plan
// has active entries and no entry of the Mod, it adds the Mod like an add.
func (a *App) StylesRebuild(ctx context.Context, dryRun bool) (*StylesResult, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	ss, err := a.StylesState(ctx, st, nil)
	if err != nil {
		return nil, err
	}
	r := &StylesResult{DryRun: dryRun, Plan: st.Plan.Name}
	if dryRun {
		rows, _ := a.Store.LookStyles()
		return a.stylesPreview(r, ss.Ls, rows, ss.Active > 0)
	}
	if ss.Active > 0 {
		if _, r.Dir, r.DirChosen, err = a.ensureGenMod(stylesGen, st.Ix.Env.GameRoot); err != nil {
			return nil, err
		}
		if r.Added, err = a.genLast(stylesGen); err != nil {
			return r, err
		}
	}
	return r, a.stylesAfter(ctx, r)
}

// stylesPlace is where RimArcho Styles Preference belongs in a plan of n entries that
// holds it: last, or last but one when RimArcho Looks Preference is there (1-based).
func stylesPlace(ls *LooksState) int {
	if ls.Entry > 0 {
		return len(ls.St.List) - 1
	}
	return len(ls.St.List)
}

// stylesPreview fills a dry run's result: the state and the rebuild as they would be with
// rows; enter: the call would put the Mod into the plan.
func (a *App) stylesPreview(r *StylesResult, ls *LooksState, rows []store.LookStyle, enter bool) (*StylesResult, error) {
	r.State = stylesState(ls, rows)
	r.Added = enter && (ls.StylesEntry == 0 || ls.StylesEntry != stylesPlace(ls))
	if r.Dir = a.LooksDir(); r.Dir == "" {
		r.Dir, r.DirChosen = a.LooksDirDefault(ls.St.Ix.Env.GameRoot), true
	}
	var err error
	if r.Build, err = a.stylesRebuildOf(r.State, r.Dir, true, nil); err != nil {
		r.BuildErr = err.Error()
	}
	return r, nil
}

// stylesAfter rebuilds the generated Mod for the current plan after an add or a remove and
// fills the result's state. A rebuild that fails leaves the entries as they are.
func (a *App) stylesAfter(ctx context.Context, r *StylesResult) error {
	st, err := a.Loadout()
	if err != nil {
		return err
	}
	if r.State, err = a.StylesState(ctx, st, nil); err != nil {
		return err
	}
	if r.Entry != nil && !r.Removed {
		for _, e := range r.State.Entries {
			if e.ID == r.Entry.ID {
				e := e
				r.Entry = &e
			}
		}
	}
	if r.Dir = a.LooksDir(); r.Dir == "" {
		return nil // nothing was ever added: there is no generated Mod to rebuild
	}
	if r.Build, err = a.stylesRebuildOf(r.State, r.Dir, false, nil); err != nil {
		r.BuildErr = err.Error()
	}
	return nil
}

// stylesRebuild rebuilds RimArcho Styles Preference in the looks directory dir for ls's
// plan from the stored entries.
func (a *App) stylesRebuild(ls *LooksState, dir string, dryRun bool, h *oplog.Handle) (*LooksBuild, error) {
	ss, err := a.StylesStateOf(ls)
	if err != nil {
		return nil, err
	}
	return a.stylesRebuildOf(ss, dir, dryRun, h)
}

func (a *App) stylesRebuildOf(ss *StylesState, dir string, dryRun bool, h *oplog.Handle) (*LooksBuild, error) {
	return a.genRebuild(stylesGen, ss.Ls.St, dir, append([]LooksFile(nil), ss.files...), ss.texts, dryRun, h)
}

// styleKey is a short stable name for a folder of the generated Mod.
func styleKey(parts ...string) string {
	h := sha1.Sum([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(h[:5])
}
