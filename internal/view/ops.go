package view

import (
	"slices"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// What the player can do with a row in the plan editor or Mod library > All
// (16 §3.5, §8.5): one table of operations with their label, key and menu group, and per
// row only the codes that apply. The right-click menu, the hint line, a copy's detail
// buttons, the selection bar and the keys all come from it, and the label table goes to the
// page once (OpTable) while each row carries only codes.

// Op is one operation.
type Op struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	// Key runs it ("" only from the menu: the menu key or shift+F10 opens the menu, so the
	// keyboard reaches every op).
	Key   string `json:"key,omitempty"`
	Group int    `json:"group"` // the menu's group: 1 list 2 organise 3 open 4 supported versions 5 disk
	// Confirm: it changes files, so the dry run shows first (LibPreview).
	Confirm bool `json:"confirm,omitempty"`
	Danger  bool `json:"danger,omitempty"` // red, last in the menu
	// Detail: a button in a copy's detail (Mod library).
	Detail bool `json:"detail,omitempty"`
	// One: it acts on one row at a time (the folder, a page, a note, a position).
	One bool `json:"one,omitempty"`
}

// Op codes. The library's LibDo takes the codes of groups 1, 4 and 5.
const (
	OpEnable    = "enable"     // enable (the editor's Not enabled)
	OpAdd       = "add"        // enable (Mod library: into the current list)
	OpUse       = "use"        // use this one in the current list: the current plan uses another copy of the mod
	OpDisable   = "disable"    // disable (the editor's Enabled)
	OpTakeOut   = "take-out"   // disable (Mod library: out of the current list)
	OpPutBack   = "put-back"   // put back (Staging)
	OpEnableAt  = "enable-at"  // enable at…
	OpMoveTo    = "move-to"    // move to…
	OpStage     = "stage"      // stage
	OpSwap      = "swap"       // switch version…
	OpRemove    = "remove"     // disable in every list
	OpTags      = "tags"       // a tag
	OpNote      = "note"       // a note (Mod library: this copy's note)
	OpSeparate  = "separate"   // do not list as a copy
	OpFolder    = "folder"     // open the folder
	OpWorkshop  = "workshop"   // open the Workshop page
	OpSteam     = "steam"      // Steam client page
	OpPin       = "pin"        // lock this one
	OpPinUpdate = "pin-update" // update to the Workshop version
	OpUnpin     = "unpin"      // unlock
	OpMove      = "move"       // move into the lock library
	OpUnhide    = "unhide"     // unhide
	OpResub     = "resubscribe"
	OpUnsub     = "unsubscribe"    // unsubscribe, through the Steam client
	OpDelete    = "delete"         // move to the recycle bin
	OpTextures  = "textures"       // optimise textures: the DDS of its PNGs next to them (10 §2)
	OpTexClean  = "textures-clean" // remove the converted textures: the DDS RimArcho wrote go
	OpSubRemove = "subscription-remove"
	OpSubCheck  = "subscription-check"
	OpSubUpdate = "subscription-update"
	OpSubPage   = "subscription-page"
	// OpLinked stands where delete would: the copy is a link another tool made (RimCrow
	// `_Link_`), which RimArcho never deletes (09 §3); it opens the folder holding the link
	// and says whose it is (16 §2.9).
	OpLinked = "linked"
)

// opTable is every op in menu order (16 §8.5): within a group in this order, and unsubscribe /
// delete last of all.
var opTable = []Op{
	{Code: OpEnable, Key: "space", Group: 1},
	{Code: OpAdd, Key: "space", Group: 1},
	{Code: OpUse, Key: "x", Group: 1, One: true},
	{Code: OpDisable, Key: "space", Group: 1},
	{Code: OpTakeOut, Key: "space", Group: 1},
	{Code: OpPutBack, Key: "space", Group: 1},
	{Code: OpEnableAt, Key: "m", Group: 1},
	{Code: OpMoveTo, Key: "m", Group: 1},
	{Code: OpStage, Key: "t", Group: 1},
	{Code: OpSwap, Key: "x", Group: 1, One: true},
	{Code: OpRemove, Group: 1},
	{Code: OpTags, Key: "g", Group: 2},
	{Code: OpNote, Key: "n", Group: 2, One: true},
	{Code: OpSeparate, Group: 2, Detail: true, One: true},
	{Code: OpFolder, Key: "ctrl+o", Group: 3, One: true},
	{Code: OpWorkshop, Key: "ctrl+w", Group: 3, One: true},
	{Code: OpSteam, Key: "ctrl+s", Group: 3, One: true},
	{Code: OpSubPage, Group: 3, Detail: true, One: true},
	{Code: OpPin, Group: 4, Confirm: true},
	{Code: OpPinUpdate, Group: 4, Confirm: true},
	{Code: OpUnpin, Group: 4, Confirm: true},
	{Code: OpMove, Group: 4, Confirm: true, Detail: true},
	{Code: OpUnhide, Group: 5, Confirm: true, Detail: true},
	{Code: OpTextures, Group: 5, Confirm: true, Detail: true},
	{Code: OpTexClean, Group: 5, Confirm: true, Detail: true},
	{Code: OpResub, Group: 5, Confirm: true, Detail: true},
	{Code: OpSubCheck, Group: 5, Detail: true},
	{Code: OpSubUpdate, Group: 5, Confirm: true, Detail: true},
	{Code: OpLinked, Group: 5, One: true}, // the details say it on the linked field (FieldLinked)
	{Code: OpSubRemove, Key: "Del", Group: 5, Confirm: true, Danger: true, Detail: true},
	{Code: OpUnsub, Key: "Del", Group: 5, Confirm: true, Danger: true, Detail: true},
	{Code: OpDelete, Key: "Del", Group: 5, Confirm: true, Danger: true, Detail: true},
}

func opLabel(code string) string {
	return map[string]string{
		OpSubRemove: T("remove subscription"), OpSubCheck: T("check for updates"), OpSubUpdate: T("update subscription"), OpSubPage: T("open download page"),
		OpEnable: T("enable"), OpAdd: T("enable"), OpUse: T("use this one in the current list"),
		OpDisable: T("disable"), OpTakeOut: T("disable"), OpPutBack: T("put back"),
		OpEnableAt: T("enable at…"), OpMoveTo: T("move to…"), OpStage: T("stage"), OpSwap: T("switch version…"),
		OpRemove: T("disable in every list"), OpTags: T("tags"), OpNote: T("note"),
		OpSeparate: T("do not list as a copy"),
		OpFolder:   T("open the folder"), OpWorkshop: T("open the Workshop page"),
		OpSteam: T("steam page"),
		OpPin:   T("keep this version"), OpPinUpdate: T("update to the Workshop version"), OpUnpin: T("stop keeping this version"),
		OpMove: T("move into the lock library"), OpUnhide: T("unhide"), OpResub: T("resubscribe"),
		OpUnsub: T("unsubscribe"), OpDelete: T("move to the recycle bin"), OpLinked: T("a link, not a folder: show where it is"),
		OpTextures: T("convert textures to DDS"), OpTexClean: T("remove the converted textures"),
	}[code]
}

// OpTable is every op with its label in the interface language (sent to the page once).
func OpTable() []Op {
	out := make([]Op, len(opTable))
	for i, o := range opTable {
		o.Label = opLabel(o.Code)
		out[i] = o
	}
	return out
}

// OpOf is one op ("" Code: no such op).
func OpOf(code string) Op {
	for _, o := range opTable {
		if o.Code == code {
			o.Label = opLabel(o.Code)
			return o
		}
	}
	return Op{}
}

// sortOps puts codes in menu order and drops duplicates.
func sortOps(codes []string) []string {
	var out []string
	for _, o := range opTable {
		if slices.Contains(codes, o.Code) {
			out = append(out, o.Code)
		}
	}
	return out
}

// Where a copy is in the current plan.
type Where int

const (
	Nowhere  Where = iota // another environment's, a missing item, a copy that cannot be loaded
	Disabled              // installed here, not enabled (Not enabled)
	Enabled               // in the load order (Enabled)
	Staged                // in staging (Staging)
)

// WhereOf is where the copy is in the current plan.
func WhereOf(lib *app.Library, i *app.LibInstance) Where {
	switch {
	case i.Mod == nil || i.Missing():
		return Nowhere
	case i.InPlan:
		return Enabled
	case i.Staged:
		return Staged
	case lib != nil && lib.St != nil && lib.St.Ix.ByID[i.Mod.ID] != nil:
		return Disabled
	}
	return Nowhere
}

// EditorPage and LibraryPage name the page asking; only the editor offers plan ops.
const (
	EditorPage  = "plan"
	LibraryPage = "library"
	ChangesPage = "changes"
	// SubscribePage is Mod library › add a subscription (16 §2.9).
	SubscribePage = "subscribe"
	PackPages     = "packs" // the packs, and each pack's page
	BuildPageID   = "build" // Create a pack
	RulesPage     = "rules" // the TUI's Load-order rules screen (16 §9.1)
)

// CopyOps are the codes that apply to a copy now, in menu order (16 §8.5): the plan ops by
// where it is, a tag and a note, open, the version ops and the disk ops (16 §2.9 actions).
func CopyOps(lib *app.Library, l *app.LibLineage, i *app.LibInstance, page string) []string {
	f := newCopyFacts(lib, l, i, page)
	ks := f.placeOps()
	ks = f.copyOps(ks)
	ks = f.diskOps(ks)
	return sortOps(ks)
}

// copyFacts is what CopyOps reads of a copy to choose its ops.
type copyFacts struct {
	lib  *app.Library
	l    *app.LibLineage
	i    *app.LibInstance
	m    *store.Mod
	page string

	plans, live, workshop, own, linked, loadable, url bool
	// another manager's link into the game (RimCrow `_Link_`) is not a copy: no copy
	// actions (separate, keep this version, textures), whatever the page (04 §1.3)
	foreign bool
	others  int // the other loadable copies
}

// newCopyFacts reads the copy's facts.
func newCopyFacts(lib *app.Library, l *app.LibLineage, i *app.LibInstance, page string) copyFacts {
	f := copyFacts{lib: lib, l: l, i: i, m: i.Mod, page: page}
	m := i.Mod
	f.plans = slices.ContainsFunc(i.UsedIn, func(u app.LibUse) bool { return u.Kind == "plan" })
	f.live = m != nil && !i.Missing()
	f.workshop = i.Source == app.LibWorkshop || i.Source == app.LibSteamCMD
	f.own = m != nil && m.Kind.Managed()
	f.linked = source.IsLink(i.Path)
	if m != nil && f.ix() != nil {
		f.linked = lib.St.Ix.ModIsLink(m)
	}
	f.loadable = f.live && !f.has(app.LibUnloadable) && !f.has(app.LibHidden) && !f.has(app.LibResidue)
	f.foreign = i.Link != ""
	if l != nil {
		copies := l.Instances
		if page != LibraryPage {
			copies = l.All() // the editor swaps between whatever the game can load
		}
		for _, o := range copies {
			if o != i && o.Mod != nil && !o.Missing() {
				f.others++
			}
		}
	}
	f.url = IsNumber(i.WorkshopID)
	return f
}

// has reports whether the copy has the library state s.
func (f copyFacts) has(s string) bool { return slices.Contains(f.i.States, s) }

// ix is the library's index, nil when it has none.
func (f copyFacts) ix() *app.Index {
	if f.lib != nil && f.lib.St != nil && f.lib.St.Ix != nil {
		return f.lib.St.Ix
	}
	return nil
}

// placeOps are the plan ops by where the copy is; none on the Mod library page.
func (f copyFacts) placeOps() []string {
	var ks []string
	if f.page == LibraryPage {
		return ks
	}
	page := f.page
	switch WhereOf(f.lib, f.i) {
	case Disabled:
		inPlan := f.l != nil && f.l.InPlan()
		switch {
		case page == EditorPage:
			ks = append(ks, OpEnable, OpEnableAt, OpStage)
		case inPlan && f.loadable:
			ks = append(ks, OpUse) // the plan uses another copy of it
		case f.loadable && !f.has(app.LibIgnored):
			ks = append(ks, OpAdd, OpEnableAt, OpStage)
		}
	case Enabled:
		off := OpTakeOut
		if page == EditorPage {
			off = OpDisable
		}
		ks = append(ks, off, OpMoveTo, OpStage)
		if f.others > 0 {
			ks = append(ks, OpSwap)
		}
	case Staged:
		ks = append(ks, OpPutBack, OpEnableAt)
	}
	if f.plans && f.m != nil {
		ks = append(ks, OpRemove)
	}
	return ks
}

// copyOps appends a tag and a note, open, and the version ops.
func (f copyFacts) copyOps(ks []string) []string {
	i, m, live := f.i, f.m, f.live
	if m != nil {
		ks = append(ks, OpTags, OpNote) // the editor: the mod's note; Mod library: this copy's
	}
	if live && f.others > 0 && !f.foreign {
		ks = append(ks, OpSeparate)
	}
	if live && i.Path != "" {
		ks = append(ks, OpFolder)
	}
	if f.url {
		ks = append(ks, OpWorkshop, OpSteam)
	}
	if f.loadable && (f.workshop || f.own) && !f.foreign {
		ks = append(ks, OpPin)
	}
	if live && f.own && !f.workshop && !f.linked && f.ix() != nil && f.ix().WorkshopUpdateSource(m) != nil {
		ks = append(ks, OpPinUpdate)
	}
	if i.Locked != nil {
		ks = append(ks, OpUnpin)
	}
	if live && i.Spare {
		ks = append(ks, OpMove)
	}
	if live && f.has(app.LibHidden) {
		ks = append(ks, OpUnhide)
	}
	return ks
}

// diskOps appends the texture, subscription and disk ops.
func (f copyFacts) diskOps(ks []string) []string {
	i, m, live, linked, url := f.i, f.m, f.live, f.linked, f.url
	// Texture optimisation: a copy with PNG textures (not RimArcho's own mod)
	if f.page != LibraryPage && live && !f.foreign && i.PNG && i.Path != "" && !app.IsMonitor(m) && !f.has(app.LibHidden) && !f.has(app.LibResidue) {
		ks = append(ks, OpTextures)
	}
	if i.Textures != nil && i.Textures.Files > 0 {
		ks = append(ks, OpTexClean)
	}
	resub := m != nil && i.Source == app.LibWorkshop && url && i.Locked == nil && m.Origin == "" && !linked
	if resub && f.lib != nil && f.lib.St != nil {
		resub = f.lib.St.Ix.CanResubscribe(m)
	}
	if resub {
		ks = append(ks, OpResub)
	}
	if live && i.Source == app.LibWorkshop && !f.has(app.LibResidue) && url {
		ks = append(ks, OpUnsub)
	}
	// a copy the lists use may go too: the confirm warns, and the lists show it missing
	if live && i.Subscription != nil {
		if !linked {
			ks = append(ks, OpSubRemove)
		}
		if i.Subscription.Kind != "steamcmd" {
			ks = append(ks, OpSubCheck)
		}
		if i.Subscription.HasUpdate() {
			ks = append(ks, OpSubUpdate)
		}
		if i.Subscription.Kind == "page" {
			ks = append(ks, OpSubPage)
		}
	} else if live && (f.own || f.workshop && (f.has(app.LibResidue) || f.has(app.LibHidden))) && !linked {
		ks = append(ks, OpDelete)
	}
	// a link cannot go to the recycle bin (02 §3), and another tool's is not RimArcho's to
	// remove: the menu says why and opens where it is, never a menu with no way out
	if live && linked {
		ks = append(ks, OpLinked)
	}
	return ks
}

// LinkedText says whose link a linked copy is and how to remove it (OpLinked; the same
// reason archo trash gives).
func LinkedText(ix *app.Index, m *store.Mod) string {
	link := ix.LinkEntry(m)
	if link == "" {
		link = ix.ModDir(m)
	}
	switch who := app.LinkMaker(link); who {
	case "":
		return Tf("%s is a link another tool made: delete the link there or in this folder; its target stays", link)
	default:
		return Tf("%s is a link %s made: remove it in %s, or delete the link in this folder; its target stays", link, who, who)
	}
}

// RowOps are the ops of the editor's row: its copy's ops when the library knows it (the
// version and disk ops need it), else the plan's own for the column.
func RowOps(lib *app.Library, id string, col Column) []string {
	if lib != nil {
		for _, l := range lib.Lineages {
			for _, i := range l.All() {
				if i.Mod != nil && i.Mod.ID == id {
					ops := CopyOps(lib, l, i, EditorPage)
					// the column the player sees decides space (the library reads the plan
					// as written; the board may be newer by a step)
					return fixColumn(ops, col)
				}
			}
		}
	}
	return fixColumn([]string{OpTags, OpNote}, col)
}

// fixColumn keeps the plan ops of the row's column.
func fixColumn(ops []string, col Column) []string {
	plan := map[Column][]string{Inactive: {OpEnable, OpEnableAt, OpStage}, Active: {OpDisable, OpMoveTo, OpStage}, Staging: {OpPutBack, OpEnableAt}}[col]
	out := slices.DeleteFunc(slices.Clone(ops), func(c string) bool {
		return slices.Contains([]string{OpEnable, OpAdd, OpUse, OpDisable, OpTakeOut, OpPutBack, OpEnableAt, OpMoveTo, OpStage}, c) ||
			c == OpSwap && col != Active
	})
	return sortOps(append(out, plan...))
}
