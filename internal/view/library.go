package view

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// Mod 库存 › 全部 (06 §2.9) for both interfaces: the filter groups, one row per Lineage with
// its Instances, the right column's size · state · used-in, and each Instance's actions as
// data. The filtering itself is app.Library's, shared with `archo library`.

// LibPage is the page.
type LibPage struct {
	Lead   string      `json:"lead"`
	Total  int         `json:"total"` // lineages in the library
	Shown  int         `json:"shown"` // lineages that match
	Size   string      `json:"size"`  // of the matching copies
	Rows   []LibRow    `json:"rows"`
	Groups []LibGroup  `json:"groups"`
	Conds  []LibCond   `json:"conds"` // what is in force, in the order given
	Sort   string      `json:"sort"`
	Sorts  []LibChoice `json:"sorts"`
	Search string      `json:"search"`
}

// LibGroup is one filter key with its values.
type LibGroup struct {
	Key    string      `json:"key"`
	Title  string      `json:"title"`
	Values []LibChoice `json:"values"`
}

// LibChoice is a filter value (or a sort): Filter is "key:value", On 1 included, -1 excluded.
type LibChoice struct {
	Filter string `json:"filter"`
	Label  string `json:"label"`
	Count  int    `json:"count"`
	On     int    `json:"on,omitempty"`
}

// LibCond is a filter in force.
type LibCond struct {
	Filter  string `json:"filter"`
	Label   string `json:"label"`
	Exclude bool   `json:"exclude,omitempty"`
}

// LibRow is a Lineage. A Lineage with one copy is that copy's row (Single, Insts[0]).
type LibRow struct {
	ID     string       `json:"id"`
	Ref    string       `json:"ref"`
	Name   string       `json:"name"`
	Tags   []string     `json:"tags,omitempty"`
	Size   string       `json:"size"`
	State  string       `json:"state,omitempty"`
	Code   string       `json:"code,omitempty"` // the state (app.LibStates), for its colour
	Used   string       `json:"used,omitempty"`
	UsedAt string       `json:"usedTitle,omitempty"` // every plan and pack, for the tooltip
	Single bool         `json:"single,omitempty"`
	Insts  []LibInstRow `json:"insts"`
	Copies int          `json:"copies"` // all copies (Insts are the matching ones)
}

// LibInstRow is one copy.
type LibInstRow struct {
	ID       string      `json:"id"` // the instance ("" for a subscribed item RimArcho never saw)
	Ref      string      `json:"ref"`
	Label    string      `json:"label"`          // 工坊 · 本地 · 锁定版本 · 26/10/21 14:27 …
	Note     string      `json:"note,omitempty"` // its abnormal state, the row's second part
	Size     string      `json:"size"`
	State    string      `json:"state,omitempty"`
	Code     string      `json:"code,omitempty"`
	Used     string      `json:"used,omitempty"`
	UsedAt   string      `json:"usedTitle,omitempty"`
	InPlan   bool        `json:"inPlan,omitempty"`
	Path     string      `json:"path,omitempty"`
	URL      string      `json:"url,omitempty"`
	States   []string    `json:"states,omitempty"` // every state (the detail lists them all)
	UsedIn   []string    `json:"usedIn,omitempty"`
	Versions string      `json:"versions,omitempty"`
	Played   string      `json:"played,omitempty"`
	Actions  []LibAction `json:"actions,omitempty"`
}

// LibAction is something the player can do with a copy (06 §2.9's 动作). Confirm: it
// changes files, so the interface shows the dry run first (LibPreview).
type LibAction struct {
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	Confirm bool   `json:"confirm,omitempty"`
	Danger  bool   `json:"danger,omitempty"`
}

// Library action kinds.
const (
	LibEnable    = "enable"     // 加入当前方案
	LibRemove    = "remove"     // 从所有方案里拿掉
	LibPin       = "pin"        // 锁定版本
	LibPinUpdate = "pin-update" // 更新锁定版本
	LibUnpin     = "unpin"      // 解除锁定
	LibUnhide    = "unhide"     // 恢复给游戏
	LibMove      = "move"       // 收进锁定库
	LibTrash     = "trash"      // 移到回收站
	LibFolder    = "folder"     // 打开文件夹
	LibWorkshop  = "workshop"   // 打开工坊页面
)

var libActionOrder = []string{LibEnable, LibRemove, LibUnhide, LibMove, LibPin, LibPinUpdate, LibUnpin, LibTrash, LibFolder, LibWorkshop}

func libActionLabel(k string) string {
	return map[string]string{
		LibEnable: T("add to the current plan"), LibRemove: T("take out of every plan"), LibPin: T("lock this version"),
		LibPinUpdate: T("update the locked version"), LibUnpin: T("unlock"), LibUnhide: T("give it back to the game"),
		LibMove: T("move into the lock library"), LibTrash: T("move to the recycle bin"), LibFolder: T("open the folder"), LibWorkshop: T("open the Workshop page"),
	}[k]
}

// LibStateText is a state as the page says it.
func LibStateText(s, mm string) string {
	switch s {
	case app.LibUnloadable:
		return T("cannot be loaded")
	case app.LibMissing:
		return T("subscribed, but not on disk")
	case app.LibIgnored:
		return T("the game ignores this copy")
	case app.LibResidue:
		return T("left over after unsubscribing")
	case app.LibHidden:
		return T("hidden by another manager")
	case app.LibUpdated:
		return T("upstream has an update")
	case app.LibUnsupported:
		return Tf("does not support %s", mm)
	case app.LibUnused:
		return T("in no plan")
	}
	return s
}

// LibSourceText names a source value.
func LibSourceText(s string) string {
	switch s {
	case app.LibWorkshop:
		return T("workshop")
	case app.LibLocal:
		return T("local")
	case app.LibLocked:
		return T("lock library")
	case app.LibSteamCMD:
		return "SteamCMD"
	case app.LibExtra:
		return T("other folder")
	case app.LibMonitor:
		return "Archo Monitor"
	}
	return s
}

func libKeyTitle(k string) string {
	return map[string]string{"tag": T("tags"), "source": T("library source"), "version": T("supported versions"), "state": T("state"), "used": T("used in")}[k]
}

func libUseText(v string) string {
	switch {
	case v == "none":
		return T("in no plan")
	case strings.HasPrefix(v, "plan:"):
		return v[5:]
	case strings.HasPrefix(v, "pack:"):
		return Tf("%s (pack)", v[5:])
	}
	return v
}

func libValueText(key, v, mm string) string {
	switch key {
	case "source":
		return LibSourceText(v)
	case "state":
		return LibStateText(v, mm)
	case "used":
		return libUseText(v)
	}
	return v
}

// libStamp is a locked copy's time: 26/10/21 14:27 (06 §2.9).
func libStamp(t time.Time) string { return t.Local().Format("06/01/02 15:04") }

func libLabel(i *app.LibInstance) string {
	if i.Source == app.LibLocked {
		if i.Created.IsZero() {
			return T("locked version")
		}
		return T("locked version") + " · " + libStamp(i.Created)
	}
	if i.Source == app.LibLocal && i.Locked != nil {
		return T("locked version") + " · " + T("local") // made in Mods/ before the lock library
	}
	return LibSourceText(i.Source)
}

func libSize(n int64, ok bool) string {
	if !ok {
		return "—"
	}
	return render.Size(n)
}

func libUsed(us []app.LibUse) (short, all string) {
	if len(us) == 0 {
		return "", ""
	}
	names := make([]string, len(us))
	for i, u := range us {
		names[i] = libUseText(u.Value())
	}
	all = strings.Join(names, "\n")
	if len(us) == 1 {
		return names[0], all
	}
	return Tf("%s +%d", names[0], len(us)-1), all
}

// LibQueryOf is the page's query: search text, filters (a leading "-" excludes) and sort.
func LibQueryOf(search string, filters []string, sort string) app.LibQuery {
	q := app.LibQuery{Search: search, Sort: sort}
	for _, f := range filters {
		if x, ok := strings.CutPrefix(f, "-"); ok {
			q.Exclude = append(q.Exclude, x)
		} else {
			q.Include = append(q.Include, f)
		}
	}
	return q
}

// BuildLibrary builds the page for a query. filters are "key:value", "-key:value" to exclude.
func BuildLibrary(lib *app.Library, search string, filters []string, sort string) (*LibPage, error) {
	q := LibQueryOf(search, filters, sort)
	res, err := lib.Filter(q)
	if err != nil {
		return nil, err
	}
	if sort == "" {
		sort = "name"
	}
	p := &LibPage{Total: res.Total, Shown: len(res.Matches), Sort: sort, Search: search}
	var bytes int64
	for _, m := range res.Matches {
		r := LibRow{ID: m.ID, Ref: m.Ref, Name: m.Name, Tags: m.Tags, Copies: len(m.Instances), Single: len(m.Instances) == 1}
		if m.Instances[0].Mod == nil {
			r.Name = Tf("Workshop item %s", m.Instances[0].WorkshopID)
		}
		n, ok := m.Bytes()
		r.Size = libSize(n, ok)
		if s := m.State(); s != "" {
			r.State, r.Code = LibStateText(s, lib.GameMM), s
		}
		us := m.UsedIn()
		r.Used, r.UsedAt = libUsed(us)
		if len(us) > 1 {
			r.Used = Tf("in %d plans", len(us))
		}
		for _, i := range m.Shown {
			if i.Sized {
				bytes += i.Bytes
			}
			r.Insts = append(r.Insts, libInst(lib, m.LibLineage, i))
		}
		p.Rows = append(p.Rows, r)
	}
	p.Size = render.Size(bytes)
	p.Lead = T("the mods on this machine: how many copies, where, used where, how big")
	for _, f := range res.Facets {
		g := LibGroup{Key: f.Key, Title: libKeyTitle(f.Key)}
		for _, v := range f.Values {
			g.Values = append(g.Values, LibChoice{Filter: f.Key + ":" + v.Value, Label: libValueText(f.Key, v.Value, lib.GameMM), Count: v.Count, On: v.On})
		}
		p.Groups = append(p.Groups, g)
	}
	for _, f := range filters {
		ex := strings.HasPrefix(f, "-")
		k, v, err := app.ParseLibFilter(strings.TrimPrefix(f, "-"))
		if err != nil {
			continue
		}
		p.Conds = append(p.Conds, LibCond{Filter: f, Label: libKeyTitle(k) + " " + libValueText(k, v, lib.GameMM), Exclude: ex})
	}
	for _, s := range app.LibSorts {
		p.Sorts = append(p.Sorts, LibChoice{Filter: s, Label: map[string]string{"name": T("name"), "size": T("size"), "changed": T("last changed"), "played": T("last played")}[s]})
	}
	return p, nil
}

func libInst(lib *app.Library, l *app.LibLineage, i *app.LibInstance) LibInstRow {
	r := LibInstRow{Ref: i.Ref, Label: libLabel(i), Size: libSize(i.Bytes, i.Sized), InPlan: i.InPlan, Path: i.Path}
	if i.Mod != nil {
		r.ID = i.Mod.ID
	}
	if s := i.State(); s != "" {
		r.State, r.Code = LibStateText(s, lib.GameMM), s
		if s != app.LibUnused {
			r.Note = r.State
		}
	}
	for _, s := range i.States {
		r.States = append(r.States, LibStateText(s, lib.GameMM))
	}
	r.Used, r.UsedAt = libUsed(i.UsedIn)
	for _, u := range i.UsedIn {
		r.UsedIn = append(r.UsedIn, libUseText(u.Value()))
	}
	r.Versions = strings.Join(i.Versions, " ")
	if !i.Played.IsZero() {
		r.Played = ShortDate(i.Played)
	}
	if IsNumber(i.WorkshopID) {
		r.URL = WorkshopURL(i.WorkshopID)
	}
	r.Actions = libActions(lib, l, i)
	return r
}

// libActions is what can be done with a copy now (06 §2.9's table).
func libActions(lib *app.Library, l *app.LibLineage, i *app.LibInstance) []LibAction {
	has := func(s string) bool {
		for _, x := range i.States {
			if x == s {
				return true
			}
		}
		return false
	}
	plans := false
	for _, u := range i.UsedIn {
		if u.Kind == "plan" {
			plans = true
		}
	}
	m := i.Mod
	live := m != nil && !i.Missing()
	loadable := live && !has(app.LibUnloadable) && !has(app.LibHidden) && !has(app.LibResidue)
	var ks []string
	if loadable && !l.InPlan() && !has(app.LibIgnored) {
		ks = append(ks, LibEnable)
	}
	if plans && i.Ref != "" && m != nil {
		ks = append(ks, LibRemove)
	}
	if live && has(app.LibHidden) {
		ks = append(ks, LibUnhide)
	}
	if live && i.Spare && i.Source == app.LibLocal {
		ks = append(ks, LibMove)
	}
	if loadable && (i.Source == app.LibWorkshop || i.Source == app.LibSteamCMD) {
		ks = append(ks, LibPin)
	}
	if i.Locked != nil && i.Locked.Updated {
		ks = append(ks, LibPinUpdate)
	}
	if i.Locked != nil {
		ks = append(ks, LibUnpin)
	}
	if live && (i.Source == app.LibWorkshop || i.Source == app.LibSteamCMD) && (has(app.LibHidden) || has(app.LibResidue)) && !plans {
		ks = append(ks, LibTrash)
	}
	if live && i.Path != "" {
		ks = append(ks, LibFolder)
	}
	if IsNumber(i.WorkshopID) {
		ks = append(ks, LibWorkshop)
	}
	var out []LibAction
	for _, k := range libActionOrder {
		for _, x := range ks {
			if x == k {
				out = append(out, LibAction{Kind: k, Label: libActionLabel(k), Confirm: libConfirm[k], Danger: k == LibTrash})
			}
		}
	}
	return out
}

// libConfirm: the actions that change files (a dry run first); the plan edits are undone with u.
var libConfirm = map[string]bool{LibPin: true, LibPinUpdate: true, LibUnpin: true, LibUnhide: true, LibMove: true, LibTrash: true}

// libArgv is an action's archo command (its dry run with --dry-run).
func libArgv(kind string, refs []string) []string {
	switch kind {
	case LibPin:
		return []string{"pin", refs[0]}
	case LibPinUpdate:
		return []string{"pin", "--update", refs[0]}
	case LibUnpin:
		return []string{"unpin", refs[0]}
	case LibUnhide:
		return []string{"unhide", refs[0]}
	case LibMove:
		return []string{"library", "move-to-locked", refs[0]}
	case LibTrash:
		return []string{"trash", refs[0]}
	}
	return nil
}

// LibPreview is the dry run of a file-changing action for each copy (06 §6: preview, then
// confirm). Each copy's dry run is the CLI's text with names for #refs.
func LibPreview(a *app.App, kind string, refs []string) (*Preview, error) {
	if !libConfirm[kind] || len(refs) == 0 {
		return nil, fmt.Errorf("%s has no preview", kind)
	}
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	p := &Preview{What: kind, Title: libActionLabel(kind), Apply: true}
	for _, ref := range refs {
		out, code := cmds.Execute(a, append(libArgv(kind, []string{ref}), "--dry-run"))
		for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
			if strings.HasPrefix(l, "next:") {
				break
			}
			p.Lines = append(p.Lines, Humanize(st, l))
		}
		if code != 0 {
			p.Apply = false
		}
	}
	return p, nil
}

// LibDo runs an action on copies and says what it did.
func LibDo(a *app.App, kind string, refs []string) (string, error) {
	if len(refs) == 0 {
		return "", errors.New("nothing selected")
	}
	st, err := a.Loadout()
	if err != nil {
		return "", err
	}
	what := Named(st, idsOfRefs(st, refs))
	switch kind {
	case LibEnable:
		if _, err := a.Enable(refs, app.Placement{}); err != nil {
			return "", err
		}
		return Tf("added %s to the current plan", what), nil
	case LibRemove:
		rs, err := a.RemoveFromPlans(refs)
		if err != nil {
			return "", err
		}
		if len(rs) == 0 {
			return Tf("no plan uses %s", what), nil
		}
		return Tf("took %s out of %d plans · each plan's u undoes it", what, len(rs)), nil
	}
	if !libConfirm[kind] {
		return "", fmt.Errorf("unknown action %s", kind)
	}
	done := 0
	for _, ref := range refs {
		var err error
		switch kind {
		case LibPin:
			_, err = a.Pin(ref, "", false, false)
		case LibPinUpdate:
			_, err = a.PinUpdate(ref, "", false, false)
		case LibUnpin:
			_, err = a.Unpin(ref, false)
		case LibUnhide:
			_, err = a.Unhide(ref, false)
		case LibMove:
			_, err = a.MoveToLocked(ref, false)
		case LibTrash:
			_, err = a.Trash(ref, false)
		}
		if err != nil {
			if done > 0 {
				return "", fmt.Errorf("%d done, then: %w", done, err)
			}
			return "", err
		}
		done++
	}
	return Tf("%s: %s", libActionLabel(kind), what), nil
}

func idsOfRefs(st *app.LoadoutState, refs []string) []string {
	var out []string
	for _, r := range refs {
		if m, err := st.Ix.ResolveOne(r); err == nil {
			out = append(out, m.ID)
		}
	}
	return out
}
