package view

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
	"github.com/ToumanLin/RimArcho/internal/store"
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
	// Columns are the table's columns in their default order (06 §2.9); the window keeps
	// the player's order, widths and hidden ones (ui.gui.library.columns).
	Columns []LibColumn `json:"columns"`
	Search  string      `json:"search"`
	// Ops is the op table (labels, keys, menu groups) the rows' codes refer to.
	Ops []Op `json:"opTable"`
	// Complete is the grey text after the search box's last word that tab (or →) takes
	// (06 §3.7): a key ("so" → "urce:") or a value ("state:un" → "used").
	Complete string `json:"complete,omitempty"`
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
	Cur    bool   `json:"cur,omitempty"` // the environment's game version (listed first, marked)
}

// LibCond is a filter in force.
type LibCond struct {
	Filter  string `json:"filter"`
	Label   string `json:"label"`
	Exclude bool   `json:"exclude,omitempty"`
}

// LibCells are a row's table cells (06 §2.9): 名称 大小 状态 来源 用在, and from the
// header's menu 更新日期 版本 作者 标签 上次游玩. State is only one worth acting on.
type LibCells struct {
	Name    string `json:"name"`
	Size    string `json:"size"`
	State   string `json:"state,omitempty"`
	Code    string `json:"code,omitempty"` // the state (app.LibStates), for its colour
	Source  string `json:"source"`
	Used    string `json:"used,omitempty"`
	UsedAt  string `json:"usedTitle,omitempty"` // every plan and pack, for the tooltip
	Updated string `json:"updated,omitempty"`
	Version string `json:"version,omitempty"`
	Author  string `json:"author,omitempty"`
	Tags    string `json:"tags,omitempty"`
	Played  string `json:"played,omitempty"`
}

// LibColumns are the table's columns: key (= the sort), shown by default.
var LibColumns = []struct {
	Key   string
	Shown bool
}{{"name", true}, {"size", true}, {"state", true}, {"source", true}, {"used", true},
	{"updated", false}, {"version", false}, {"author", false}, {"tags", false}, {"played", false}}

// LibColumn is a table column: its key (= its sort), header, shown by default.
type LibColumn struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Shown bool   `json:"shown"`
}

// LibRow is a Lineage. A Lineage with one copy is that copy's row (Single, Insts[0]).
type LibRow struct {
	ID   string   `json:"id"`
	Ref  string   `json:"ref"`
	Name string   `json:"name"`
	Tags []string `json:"tags,omitempty"`
	LibCells
	Single bool         `json:"single,omitempty"`
	Insts  []LibInstRow `json:"insts"`
	Copies int          `json:"copies"` // all copies (Insts are the matching ones)
}

// LibInstRow is one copy.
type LibInstRow struct {
	ID    string `json:"id"` // the instance ("" for a subscribed item RimArcho never saw)
	Ref   string `json:"ref"`
	Label string `json:"label"` // 工坊 · 本地 · 锁定版本 · 26/10/21 14:27 …
	// Cells: Name is the copy's folder; Source its Label.
	LibCells
	// Note is the copy's own note (the player's or an agent's); Auto what RimArcho knows
	// about how it made the copy, shown (and offered as a first version) while there is none.
	Note     string      `json:"note,omitempty"`
	Auto     string      `json:"auto,omitempty"`
	InPlan   bool        `json:"inPlan,omitempty"`
	Path     string      `json:"path,omitempty"`
	URL      string      `json:"url,omitempty"`
	States   []string    `json:"states,omitempty"` // every state (the detail lists them all)
	UsedIn   []string    `json:"usedIn,omitempty"`
	Versions string      `json:"versions,omitempty"`
	Played   string      `json:"played,omitempty"`
	// Ops are the op codes that apply (ops.go, CopyOps); the labels and keys are the page's
	// Ops table, sent once.
	Ops []string `json:"ops,omitempty"`
}

// OpenPages is LibDo's answer when Steam is out of reach (not running, the DLL would not
// load): the interface opens the Workshop pages and says Msg, so the player can do it there.
type OpenPages struct {
	Msg  string
	URLs []string
}

func (e *OpenPages) Error() string { return e.Msg }

// Library action kinds: the op codes (ops.go) that LibDo runs.
const (
	LibEnable    = OpAdd       // 加入当前方案
	LibRemove    = OpRemove    // 从所有方案里拿掉
	LibPin       = OpPin       // 锁定版本
	LibPinUpdate = OpPinUpdate // 更新锁定版本
	LibUnpin     = OpUnpin     // 解除锁定
	LibUnhide    = OpUnhide    // 恢复
	LibUnsub     = OpUnsub     // 退订, through the Steam client
	LibDelete    = OpDelete    // 删除 (to the recycle bin)
	LibResub     = OpResub     // 重新订阅, through the Steam client
	LibMove      = OpMove      // 备份锁定版本 (收进锁定库)
	LibFolder    = OpFolder    // 打开文件夹
	LibWorkshop  = OpWorkshop  // 打开网页
	// LibTrash is the old name of LibDelete.
	LibTrash = LibDelete
)

func libActionLabel(k string) string { return opLabel(k) }

// rowState is the state a row shows: the most important one (06 §2.9).
func rowState(states []string) string {
	if len(states) == 0 {
		return ""
	}
	return states[0]
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
	return map[string]string{"tag": T("tags"), "source": T("library source"), "version": T("supported versions"), "state": T("state"), "used": T("used in"), "updated": T("update date")}[k]
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
	case "updated":
		return map[string]string{"week": T("within a week"), "month": T("within a month"), "half-year": T("within half a year"), "older": T("earlier")}[v]
	}
	return v
}

// LibSetNote sets a copy's own note from the page (06 §2.9); "" clears it.
func LibSetNote(a *app.App, ref, text string) (string, error) {
	m, err := a.SetCopyNote(ref, text)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(text) == "" {
		return Tf("cleared the note of %s", m.DisplayName()), nil
	}
	return Tf("saved the note of %s", m.DisplayName()), nil
}

// LibColumnTitle is a table column's header.
func LibColumnTitle(k string) string {
	return map[string]string{"name": T("name"), "size": T("size"), "state": T("state"), "source": T("library source"), "used": T("used in"),
		"updated": T("update date"), "version": T("supported versions"), "author": T("author"), "tags": T("tags"), "played": T("last played")}[k]
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

// LibSplitQuery splits what was typed into the search box into words and filters: a word
// "key:value" (one of app.LibKeys) keeps, "-key:value" leaves out, like a click and a
// right click in the filter column.
func LibSplitQuery(text string) (words string, filters []string) {
	var ws []string
	for _, w := range strings.Fields(text) {
		if _, _, err := app.ParseLibFilter(strings.TrimPrefix(w, "-")); err == nil {
			filters = append(filters, w)
		} else {
			ws = append(ws, w)
		}
	}
	return strings.Join(ws, " "), filters
}

// CompleteLibrary is the search box's completion (the plan's CompleteQuery with the
// library's keys): keys from the second letter, then tag words, sources, versions, states
// and the plans and packs in use.
func CompleteLibrary(text string, lib *app.Library) string {
	return completeWith(text, app.LibKeys, func(k string) []string {
		switch k {
		case "source":
			return app.LibSources
		case "state":
			return app.LibStates
		}
		var tags, out []string
		seen := map[string]bool{}
		for _, l := range lib.Lineages {
			tags = append(tags, l.Tags...)
			for _, i := range l.Instances {
				var vs []string
				switch k {
				case "version":
					vs = i.Versions
				case "used":
					for _, u := range i.UsedIn {
						vs = append(vs, u.Value())
					}
				}
				for _, v := range vs {
					if !seen[v] {
						seen[v] = true
						out = append(out, v)
					}
				}
			}
		}
		switch k {
		case "tag":
			return tagWords(tags)
		case "version":
			slices.SortFunc(out, func(a, b string) int { return -strings.Compare(a, b) })
		case "used":
			slices.Sort(out)
			out = append([]string{"none"}, out...)
		}
		return out
	})
}

// BuildLibrary builds the page for a query. filters are "key:value", "-key:value" to exclude;
// search may hold such words too (LibSplitQuery).
func BuildLibrary(lib *app.Library, search string, filters []string, sort string) (*LibPage, error) {
	words, typed := LibSplitQuery(search)
	q := LibQueryOf(words, append(append([]string(nil), filters...), typed...), sort)
	res, err := lib.Filter(q)
	if err != nil {
		return nil, err
	}
	if sort == "" {
		sort = "name"
	}
	p := &LibPage{Total: res.Total, Shown: len(res.Matches), Sort: sort, Search: search, Complete: CompleteLibrary(search, lib), Ops: OpTable()}
	var bytes int64
	for _, m := range res.Matches {
		r := LibRow{ID: m.ID, Ref: m.Ref, Name: m.Name, Tags: m.Tags, Copies: len(m.Instances), Single: len(m.Instances) == 1}
		if m.Instances[0].Mod == nil {
			r.Name = Tf("Workshop item %s", m.Instances[0].WorkshopID)
		}
		r.LibCells.Name = r.Name
		n, ok := m.Bytes()
		r.Size = libSize(n, ok)
		us := m.UsedIn()
		r.Used, r.UsedAt = libUsed(us)
		if len(us) > 1 {
			r.Used = Tf("in %d plans", len(us))
		}
		var srcs []string
		best := len(app.LibStates)
		var updated, played time.Time
		for _, i := range m.Instances {
			if l := LibSourceText(i.Source); !slices.Contains(srcs, l) {
				srcs = append(srcs, l)
			}
			if s := rowState(i.States); s != "" {
				best = min(best, slices.Index(app.LibStates, s))
			}
			if i.Updated.After(updated) {
				updated = i.Updated
			}
			if i.Played.After(played) {
				played = i.Played
			}
		}
		r.Source = strings.Join(srcs, " · ")
		if best < len(app.LibStates) {
			r.Code = app.LibStates[best]
			r.State = LibStateText(r.Code, lib.GameMM)
		}
		r.Updated, r.Played = libDate(updated), libDate(played)
		r.Version, r.Author, r.LibCells.Tags = lineageVersion(m.LibLineage), lineageAuthor(m.LibLineage), strings.Join(m.Tags, " · ")
		for _, i := range m.Shown {
			if i.Sized {
				bytes += i.Bytes
			}
			r.Insts = append(r.Insts, libInst(lib, m.LibLineage, i))
		}
		if r.Single {
			r.Source = r.Insts[0].Label
		}
		p.Rows = append(p.Rows, r)
	}
	p.Size = render.Size(bytes)
	p.Lead = T("the mods on this machine: how many copies, where, used where, how big")
	for _, f := range res.Facets {
		g := LibGroup{Key: f.Key, Title: libKeyTitle(f.Key)}
		for _, v := range f.Values {
			c := LibChoice{Filter: f.Key + ":" + v.Value, Label: libValueText(f.Key, v.Value, lib.GameMM), Count: v.Count, On: v.On}
			if f.Key == "version" && v.Value == lib.GameMM {
				// the game's own version first, marked (06 §2.9)
				c.Cur = true
				g.Values = append([]LibChoice{c}, g.Values...)
				continue
			}
			g.Values = append(g.Values, c)
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
	for _, c := range LibColumns {
		p.Columns = append(p.Columns, LibColumn{Key: c.Key, Title: LibColumnTitle(c.Key), Shown: c.Shown})
	}
	for _, s := range app.LibSorts {
		p.Sorts = append(p.Sorts, LibChoice{Filter: s, Label: map[string]string{"name": T("name"), "size": T("size"), "changed": T("last changed"), "played": T("last played")}[s]})
	}
	return p, nil
}

func libInst(lib *app.Library, l *app.LibLineage, i *app.LibInstance) LibInstRow {
	r := LibInstRow{Ref: i.Ref, Label: libLabel(i), InPlan: i.InPlan, Path: i.Path}
	r.Size, r.Source = libSize(i.Bytes, i.Sized), r.Label
	if i.Mod != nil {
		r.ID = i.Mod.ID
		r.LibCells.Name = i.Mod.Rel // the copy's folder: the Lineage row has the name
	}
	if s := rowState(i.States); s != "" {
		r.State, r.Code = LibStateText(s, lib.GameMM), s
	}
	r.Updated, r.Version, r.Author = libDate(i.Updated), newest(i.Versions), i.Authors
	r.LibCells.Tags = strings.Join(l.Tags, " · ")
	if i.Note != nil {
		r.Note = i.Note.Text
	}
	r.Auto = CopyNoteAuto(i)
	for _, s := range i.States {
		r.States = append(r.States, LibStateText(s, lib.GameMM))
	}
	r.Used, r.UsedAt = libUsed(i.UsedIn)
	for _, u := range i.UsedIn {
		r.UsedIn = append(r.UsedIn, libUseText(u.Value()))
	}
	r.Versions = strings.Join(i.Versions, " ")
	r.Played = libDate(i.Played)
	if IsNumber(i.WorkshopID) {
		r.URL = WorkshopURL(i.WorkshopID)
	}
	r.Ops = CopyOps(lib, l, i, LibraryPage)
	return r
}

// libDate is a table date: 26/10/21 (as the locked version's stamp), "" when unknown.
func libDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("06/01/02")
}

func newest(vs []string) string {
	best := ""
	for _, v := range vs {
		if best == "" || versionNewer(v, best) {
			best = v
		}
	}
	return best
}

// versionNewer: "1.10" is newer than "1.9".
func versionNewer(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		x, e1 := strconv.Atoi(pa[i])
		y, e2 := strconv.Atoi(pb[i])
		if e1 == nil && e2 == nil && x != y {
			return x > y
		}
		if (e1 != nil || e2 != nil) && pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return len(pa) > len(pb)
}

func lineageVersion(l *app.LibLineage) string {
	var all []string
	for _, i := range l.Instances {
		all = append(all, i.Versions...)
	}
	return newest(all)
}

func lineageAuthor(l *app.LibLineage) string {
	for _, i := range l.Instances {
		if i.Authors != "" {
			return i.Authors
		}
	}
	return ""
}

// CopyNoteAuto is RimArcho's first version of a copy's note (06 §2.9): "锁定版本创建于 10 月
// 21 日，为方案「测试」创建；之后上游有 2 次更新；原因：…" ("" when it knows nothing). The
// player's note replaces it; editing starts from it.
func CopyNoteAuto(i *app.LibInstance) string {
	var parts []string
	var made *store.Made
	if i.Note != nil {
		made = i.Note.Made
	}
	plans := func() string {
		if made == nil || len(made.Plans) == 0 {
			return ""
		}
		q := make([]string, len(made.Plans))
		for j, p := range made.Plans {
			q[j] = Tf("“%s”", p)
		}
		return Tf(", for plan %s", strings.Join(q, T(", ")))
	}
	switch {
	case made != nil && made.Kind == "move":
		parts = append(parts, Tf("moved into the lock library on %s", ShortDate(made.At))+plans())
	case made != nil && made.Kind == "pin-update":
		parts = append(parts, Tf("new locked version made on %s", ShortDate(made.At))+plans())
	case made != nil:
		parts = append(parts, Tf("locked version made on %s", ShortDate(made.At))+plans())
	case i.Locked != nil && !i.Created.IsZero():
		parts = append(parts, Tf("locked version made on %s", ShortDate(i.Created)))
	case i.Locked != nil || i.Missing():
	default:
		// a copy RimArcho did not make: where it came from and when RimArcho first saw it
		from := map[string]string{
			app.LibWorkshop: T("subscribed on the Workshop"),
			app.LibLocal:    T("put in the game's Mods folder"),
			app.LibLocked:   T("in the lock library"),
			app.LibSteamCMD: T("downloaded with SteamCMD"),
			app.LibExtra:    T("in an extra mod folder"),
			app.LibMonitor:  T("RimArcho's own companion mod"),
		}[i.Source]
		if slices.Contains(i.States, app.LibResidue) {
			from = T("unsubscribed; Steam left the folder")
		}
		if from != "" {
			if !i.Created.IsZero() && i.Source != app.LibMonitor {
				from += Tf(", first seen on %s", ShortDate(i.Created))
			}
			parts = append(parts, from)
		}
	}
	switch {
	case i.Locked != nil && i.UpstreamUpdates > 0:
		parts = append(parts, Tf("the upstream has updated %d times since", i.UpstreamUpdates))
	case i.Locked != nil && i.Locked.Upstream != nil && len(parts) > 0:
		parts = append(parts, T("the upstream has not updated since"))
	case i.UpstreamWorkshop && i.UpstreamUpdates > 0:
		parts = append(parts, Tf("the Workshop copy has updated %d times since", i.UpstreamUpdates))
	}
	if made != nil && made.Reason != "" {
		parts = append(parts, Tf("reason: %s", made.Reason))
	}
	return strings.Join(parts, T("; "))
}

// libConfirm: the actions that change files (a dry run first); the plan edits are undone with u.
var libConfirm = map[string]bool{LibPin: true, LibPinUpdate: true, LibUnpin: true, LibUnhide: true, LibMove: true, LibDelete: true, LibUnsub: true}

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
	case LibUnsub:
		return []string{"workshop", "unsubscribe", refs[0]}
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
			l = Humanize(st, l)
			// "#ref Name" became "Name Name"
			if m, err := st.Ix.ResolveOne(ref); err == nil {
				n := m.DisplayName()
				l = strings.ReplaceAll(l, n+" "+n, n)
			}
			p.Lines = append(p.Lines, l)
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
	case LibUnsub, LibResub:
		return libSteam(a, kind, refs, what)
	case OpAdd, OpEnable, OpTakeOut, OpDisable, OpPutBack, OpStage, OpUse, OpSwap:
		r, err := LibPlan(a, kind, refs)
		if err != nil {
			return "", err
		}
		return r.Text, nil
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

// LibPlan runs a plan op of Mod 库存 on copies (06 §2.9): space adds and enables, or takes
// out of the current plan, or puts back from staging; t stages; x uses this copy (the plan
// had another) or swaps for another. Each is the editor's verb, one undo step.
func LibPlan(a *app.App, kind string, refs []string) (*Result, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	ids := idsOfRefs(st, refs)
	if len(ids) == 0 {
		return nil, errors.New(T("RimArcho has not seen this copy"))
	}
	o := ObjectOf(st, ids)
	switch kind {
	case OpAdd, OpEnable:
		return Toggle(a, Inactive, o)
	case OpTakeOut, OpDisable:
		return Toggle(a, Active, o)
	case OpPutBack:
		return Toggle(a, Staging, o)
	case OpStage:
		return Stage(a, Inactive, o) // enabled ones leave the list and remember their place
	case OpSwap:
		t, err := Swap(a, st, ids[0])
		return &Result{Text: t}, err
	case OpUse:
		// the copy the plan uses now, of the same mod
		mod := st.Ix.ByID[ids[0]]
		for _, e := range st.List {
			if other := st.Ix.ByID[e.Instance]; other != nil && other.ID != mod.ID && other.LineageID == mod.LineageID {
				if _, err := a.Swap(st.Ix.Ref(other.ID), o.Refs[0]); err != nil {
					return nil, err
				}
				return &Result{Text: Tf("the current plan now uses %s", libCopyName(st, mod)), Placed: ids[:1]}, nil
			}
		}
		return Toggle(a, Inactive, o)
	}
	return nil, fmt.Errorf("unknown action %s", kind)
}

// libCopyName: the mod's name and which copy ("Harmony（锁定版本）").
func libCopyName(st *app.LoadoutState, m *store.Mod) string {
	return Tf("%s (%s)", m.DisplayName(), SourceText(m))
}

// libSteam unsubscribes or subscribes again through the Steam client (06 §2.9); when Steam
// is out of reach the answer is OpenPages.
func libSteam(a *app.App, kind string, refs []string, what string) (string, error) {
	action := steamworks.Unsubscribe
	if kind == LibResub {
		action = steamworks.Subscribe
	}
	_, err := a.Workshop(action, refs, false)
	var u *app.SteamUnavailable
	switch {
	case errors.As(err, &u):
		msg := Tf("Steam could not be reached (%s): the Workshop page is open, unsubscribe there", u.Why)
		if kind == LibResub {
			msg = Tf("Steam could not be reached (%s): the Workshop page is open, subscribe there", u.Why)
		}
		return "", &OpenPages{Msg: msg, URLs: u.URLs}
	case err != nil:
		return "", err
	case kind == LibResub:
		return Tf("subscribed to %s again · Steam downloads it", what), nil
	}
	return Tf("unsubscribed from %s · Steam deletes the folder", what), nil
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

// SetModNote sets a mod's own note (`n` in the editor, 06 §3.5; the note on the Lineage,
// `archo note <ref>`); "" clears it.
func SetModNote(a *app.App, ref, text string) (string, error) {
	st, err := a.Loadout()
	if err != nil {
		return "", err
	}
	if err := a.SetNote(ref, strings.TrimSpace(text)); err != nil {
		return "", err
	}
	name := ref
	if m, err := st.Ix.ResolveOne(ref); err == nil {
		name = m.DisplayName()
	}
	if strings.TrimSpace(text) == "" {
		return Tf("cleared the note of %s", name), nil
	}
	return Tf("saved the note of %s", name), nil
}
