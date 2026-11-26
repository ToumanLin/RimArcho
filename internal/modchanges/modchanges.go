package modchanges

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/corpus"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Package modchanges builds the change history (16 §2.9) for every surface: the GUI and the
// TUI word it through view.T, the CLI and MCP in English.
//
// Changes (16 §2.9): one row per mod, merging its change events in the time range, with a
// sentence built only from what the scans recorded (the declarations and file lists of the
// two snapshots, the Workshop time, locked versions). The default is the current plan's
// mods since that plan was last played; older changes follow by day.

// Text words the page: T translates an English string (the identity for English), Date
// writes a day (a locale-appropriate date such as "Oct 21").
type Text struct {
	T    func(string) string
	Date func(time.Time) string
}

// Tf translates a format string, then formats.
func (x Text) Tf(f string, a ...any) string { return fmt.Sprintf(x.T(f), a...) }

// English is the CLI's text: the dates in loc.
func English(loc *time.Location) Text {
	return Text{T: func(s string) string { return s }, Date: func(t time.Time) string { return t.In(loc).Format("Jan 2") }}
}

// Tiers: what a change means for the player.
const (
	TierLaunch  = 1 // affects startup
	TierWatch   = 2 // watch
	TierContent = 3 // content only
)

// TierText names a tier.
func TierText(tx Text, t int) string {
	switch t {
	case TierLaunch:
		return tx.T("affects launch")
	case TierWatch:
		return tx.T("worth a look")
	}
	return tx.T("content only")
}

// Row actions (16 §2.9), each with its key on the page.
const (
	ActWorkshop = "workshop" // open page: the Workshop change notes (ctrl+w)
	ActDeps     = "deps"     // see dependencies: Can it run (D)
	ActSort     = "sort"     // sort (S)
	ActLock     = "lock"     // keep the updated version: a locked copy (p)
)

// Action is a row's action.
type Action struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Key   string `json:"key"`
}

// Row is one mod's changes in a range.
type Row struct {
	ID       string `json:"id"`
	Ref      string `json:"ref"`
	Mod      string `json:"mod"`
	Tier     int    `json:"tier"`
	TierText string `json:"tierText"`
	Text     string `json:"text"` // one sentence
	// Details: the grouped file summary first, then every fact and the Workshop time.
	Details []string `json:"details,omitempty"`
	// Before and After are the two snapshots compared (Files lists their files).
	Before     int64    `json:"before,omitempty"`
	After      int64    `json:"after,omitempty"`
	WorkshopID string   `json:"workshopId,omitempty"`
	Actions    []Action `json:"actions,omitempty"`
	InPlan     bool     `json:"inPlan,omitempty"`
	Present    bool     `json:"present,omitempty"` // the library can select this installed copy
	// Files: the files that differ, by kind, for the GUI's change marks (16 §7 principle 9);
	// Others: the sentence's facts that are not about files, shown beside the marks. Text
	// and Details still say everything; the CLI and MCP print those.
	Files  []FileCount `json:"files,omitempty"`
	Others string      `json:"others,omitempty"`
}

// FileCount is one kind of file that differs: how many were added, changed and deleted, and
// for textures and sounds the size change.
type FileCount struct {
	Kind    string `json:"kind"` // in the player's words: code, Defs, textures…
	Added   int    `json:"added,omitempty"`
	Changed int    `json:"changed,omitempty"`
	Gone    int    `json:"gone,omitempty"`
	Delta   int64  `json:"delta,omitempty"` // textures and sounds only
}

// HasFiles: the raw file list has something to compare.
func (r *Row) HasFiles() bool { return r.Before != 0 && r.After != 0 && r.Before != r.After }

// Group names newly found and removed mods, including the name for a single mod.
type Group struct {
	Text string `json:"text"`
	Rows []Row  `json:"rows"` // Each row explains where the mod was found or removed.
}

// Day is one day of the older changes.
type Day struct {
	Day      string `json:"day"`
	Rows     []Row  `json:"rows"`
	Arrivals *Group `json:"arrivals,omitempty"`
}

// Page is the page: the rows in range, new and gone, then the older days.
type Page struct {
	Library  bool   `json:"library"` // the whole library; else the current plan
	Plan     string `json:"plan"`
	Since    string `json:"since"` // "since you last played · Oct 21"
	Played   bool   `json:"played"`
	Rows     []Row  `json:"rows"`
	Arrivals *Group `json:"arrivals,omitempty"`
	Older    []Day  `json:"older,omitempty"`
	Of       string `json:"of,omitempty"` // one mod's Lineage: every change, by day
	OfName   string `json:"ofName,omitempty"`
	NoScan   bool   `json:"noScan,omitempty"`
	Launch   int    `json:"launch"` // rows that affect launch
}

// Opts says what the page shows.
type Opts struct {
	Library bool
	// Of: instances (a mod's Lineage) whose every change is listed by day.
	Of     []string
	OfName string
	// The range starts at MinScan, or at Since; neither: when the plan was last played,
	// or the last scan when it never was.
	MinScan int64
	Since   time.Time
	// Older is how many scans before the range are listed by day (default OlderScans).
	Older int
	// NoFiles skips the file lists (the home page only counts).
	NoFiles bool
}

// OlderScans is the number of scans before the range shown by day.
const OlderScans = 10

// Build builds the page.
func Build(a *app.App, st *app.LoadoutState, o Opts, tx Text) (*Page, error) {
	ix := st.Ix
	p := &Page{Library: o.Library, Plan: st.Plan.Name, OfName: o.OfName}
	if len(o.Of) > 0 {
		p.Of = o.Of[0]
	}
	scans, err := a.Store.Scans(ix.Env.GameID, 1<<30)
	if err != nil {
		return nil, err
	}
	if len(scans) == 0 {
		p.NoScan = true
		return p, nil
	}
	first := scans[len(scans)-1].ID
	from := scans[0].ID
	switch {
	case len(o.Of) > 0:
		from = 1 << 62 // every change goes by day
	case o.MinScan > 0:
		from = o.MinScan
		p.Since = tx.Tf("since scan %d", o.MinScan)
	case !o.Since.IsZero():
		from = scanFrom(scans, o.Since)
		p.Since = tx.Tf("since %s", tx.Date(o.Since))
	default:
		if t := a.Store.LastPlanLaunch(st.Plan.ID); !t.IsZero() {
			from, p.Played = scanFrom(scans, t), true
			p.Since = tx.Tf("since you last played · %s", tx.Date(t))
		} else {
			p.Since = tx.Tf("the last scan · %s", tx.Date(scans[0].Started)+" "+scans[0].Started.In(a.Loc).Format("15:04"))
		}
	}
	older := o.Older
	if older == 0 {
		older = OlderScans
	}
	minScan := from
	if len(o.Of) > 0 {
		minScan = 0
	} else {
		n := 0
		for _, s := range scans {
			if s.ID < from && n < older {
				minScan = s.ID
				n++
			}
		}
	}
	evs, err := a.Store.Events(store.EventFilter{EnvID: ix.Env.GameID, MinScan: minScan, InstanceIDs: o.Of})
	if err != nil {
		return nil, err
	}
	scope := map[string]bool(nil)
	if !o.Library && len(o.Of) == 0 {
		scope = planScope(st)
	}
	var now []*store.Event
	byDay := map[string][]*store.Event{}
	var days []string
	for _, e := range evs {
		if scope != nil && !scope[e.InstanceID] {
			continue
		}
		// The first scan finds everything: that is not a change.
		if e.ScanID == first && (e.Kind == identity.EvDiscovered || e.Kind == identity.EvCopied) && len(o.Of) == 0 {
			continue
		}
		if e.ScanID >= from {
			now = append(now, e)
			continue
		}
		// Older changes go by day; one mod's history by scan, so that the day it was first
		// seen does not swallow an update the same day.
		d := e.ScanTime.In(a.Loc).Format("2006-01-02")
		if len(o.Of) > 0 {
			d = fmt.Sprintf("%s %020d", d, e.ScanID)
		}
		if byDay[d] == nil {
			days = append(days, d)
		}
		byDay[d] = append(byDay[d], e)
	}
	b := &changeBuilder{a: a, st: st, mm: st.GameMM(), noFiles: o.NoFiles, tx: tx}
	b.locks = map[string]*app.Locked{}
	for _, l := range st.Locked() {
		if cur := b.locks[l.Upstream.ID]; cur == nil || st.List.Index(l.Copy.ID) >= 0 {
			b.locks[l.Upstream.ID] = l
		}
	}
	p.Rows, p.Arrivals = b.build(now)
	for _, r := range p.Rows {
		if r.Tier == TierLaunch {
			p.Launch++
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	for _, d := range days {
		rows, arr := b.build(byDay[d])
		if len(rows) == 0 && arr == nil {
			continue
		}
		t, _ := time.ParseInLocation("2006-01-02", d[:10], a.Loc)
		label := tx.Date(t)
		if len(o.Of) > 0 {
			label += " " + byDay[d][0].ScanTime.In(a.Loc).Format("15:04")
		}
		p.Older = append(p.Older, Day{Day: label, Rows: rows, Arrivals: arr})
	}
	return p, nil
}

// scanFrom is the first scan started at or after t (past the newest when none).
func scanFrom(scans []*store.Scan, t time.Time) int64 {
	from := scans[0].ID + 1
	for _, s := range scans {
		if !s.Started.Before(t) {
			from = s.ID
		}
	}
	return from
}

// planScope is the current plan's mods, with the upstream of each locked version it uses
// (its updates are what the lock holds back).
func planScope(st *app.LoadoutState) map[string]bool {
	in := map[string]bool{}
	for _, e := range st.List {
		if e.Instance != "" {
			in[e.Instance] = true
		}
	}
	for _, l := range st.Locked() {
		if in[l.Copy.ID] {
			in[l.Upstream.ID] = true
		}
	}
	return in
}

type changeBuilder struct {
	a       *app.App
	st      *app.LoadoutState
	mm      string
	noFiles bool
	tx      Text
	locks   map[string]*app.Locked // by upstream instance
}

// fact is one thing a mod's changes mean.
type fact struct {
	tier int
	text string
	file bool // said by the grouped file summary (left out of the details)
}

// build merges events into one row per mod, sorted by tier then name; new and gone mods
// become the summary group.
func (b *changeBuilder) build(evs []*store.Event) ([]Row, *Group) {
	tx := b.tx
	ix := b.st.Ix
	var order []string
	by := map[string][]*store.Event{}
	var snaps []int64
	for _, e := range evs {
		if by[e.InstanceID] == nil {
			order = append(order, e.InstanceID)
		}
		by[e.InstanceID] = append(by[e.InstanceID], e)
		snaps = append(snaps, e.Before, e.After)
	}
	for _, id := range order {
		if m := ix.ByID[id]; m != nil {
			snaps = append(snaps, m.SnapID)
		}
	}
	info, _ := b.a.Store.SnapInfos(snaps)
	var rows []Row
	var arrived, gone []Row
	for _, id := range order {
		r, kind := b.mod(id, by[id], info)
		if kind == "" && b.corpusOnly(ix.ByID[id], r) {
			continue // a weekly data update of Archo Monitor (19 §6): not a change to show
		}
		switch kind {
		case identity.EvDiscovered:
			arrived = append(arrived, r)
		case identity.EvDisappeared:
			gone = append(gone, r)
		default:
			rows = append(rows, r)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Tier != rows[j].Tier {
			return rows[i].Tier < rows[j].Tier
		}
		return strings.ToLower(rows[i].Mod) < strings.ToLower(rows[j].Mod)
	})
	if len(arrived)+len(gone) == 0 {
		return rows, nil
	}
	byName := func(l []Row) {
		sort.SliceStable(l, func(i, j int) bool { return strings.ToLower(l[i].Mod) < strings.ToLower(l[j].Mod) })
	}
	byName(arrived)
	byName(gone)
	g := &Group{}
	var parts []string
	var subscriptions, mods []Row
	for _, r := range arrived {
		if r.Text == tx.T("new subscription from Workshop") {
			subscriptions = append(subscriptions, r)
		} else {
			mods = append(mods, r)
		}
	}
	part := func(rs []Row, single, multiple string) {
		if len(rs) == 1 {
			parts = append(parts, tx.Tf(single, rs[0].Mod))
		} else if len(rs) > 1 {
			parts = append(parts, tx.Tf(multiple, len(rs)))
		}
	}
	part(subscriptions, "1 new subscription: %s", "%d new subscriptions")
	part(mods, "1 new mod: %s", "%d new mods")
	part(gone, "1 mod removed: %s", "%d mods removed")
	g.Text = strings.Join(parts, tx.T(" · "))
	g.Rows = append(arrived, gone...)
	return rows, g
}

// mod is one mod's row; kind is EvDiscovered for a new mod, EvDisappeared for a gone one,
// "" for a change.
func (b *changeBuilder) mod(id string, evs []*store.Event, info map[int64]*store.SnapInfo) (Row, string) {
	tx := b.tx
	r, m := b.rowHead(id)
	kinds := map[string]bool{}
	var detail = map[string]string{}
	for _, e := range evs {
		kinds[e.Kind] = true
		detail[e.Kind] = e.Detail
		if e.Before != 0 && r.Before == 0 && e.Kind != identity.EvDiscovered && e.Kind != identity.EvCopied {
			r.Before = e.Before
		}
		if e.After != 0 {
			r.After = e.After
		}
	}
	last := evs[len(evs)-1].Kind
	if last == identity.EvDisappeared || kinds[identity.EvDisappeared] && !kinds[identity.EvReappeared] {
		return b.goneRow(r, m), identity.EvDisappeared
	}
	if kinds[identity.EvDiscovered] || kinds[identity.EvCopied] {
		return b.arrivedRow(r, m, kinds), identity.EvDiscovered
	}
	f := &modFacts{}
	if kinds[identity.EvReappeared] {
		f.add(TierWatch, tx.T("came back"))
	}
	var od, nd *scan.Decl
	if x := info[r.Before]; x != nil && r.Before != r.After {
		od = &scan.Decl{}
		json.Unmarshal([]byte(x.DeclBlob), od)
	}
	if r.After == 0 && m != nil {
		r.After = m.SnapID
	}
	if x := info[r.After]; x != nil && od != nil {
		nd = &scan.Decl{}
		json.Unmarshal([]byte(x.DeclBlob), nd)
	}
	var c declChange
	if od != nil && nd != nil {
		c = b.declFacts(f, r.InPlan, od, nd)
	} else {
		c = b.eventFacts(f, r.InPlan, kinds, detail)
	}
	if kinds[identity.EvAuthorChanged] {
		f.add(TierContent, tx.Tf("author: %s", detail[identity.EvAuthorChanged]))
	}
	if kinds[identity.EvMoved] {
		f.add(TierContent, tx.T("its folder moved"))
	}
	fs := b.fileFacts(f, r, info, c.modVersion)
	// A locked version keeps the old one: say how far behind it is.
	if l := b.locks[id]; l != nil && kinds[identity.EvUpdated] {
		n := b.a.Store.UpdatesSince(id, copiedAt(l))
		if n > 0 {
			f.add(TierWatch, tx.Tf("your locked version stays behind (%d updates)", n))
		} else {
			f.add(TierWatch, tx.T("your locked version stays behind"))
		}
	}
	if len(f.facts) == 0 {
		if kinds[identity.EvUpdated] {
			f.add(TierContent, tx.T("updated"))
		} else {
			f.add(TierContent, eventText(tx, evs[0].Kind))
		}
	}
	sorted := b.sayFacts(&r, f.facts)
	b.rowDetails(&r, fs, sorted, info)
	b.rowActions(&r, m, kinds, c)
	return r, ""
}

// rowHead is a mod's row before its events are read: its name, refs and whether the
// current list uses it.
func (b *changeBuilder) rowHead(id string) (Row, *store.Mod) {
	ix, st := b.st.Ix, b.st
	m := ix.ByID[id]
	r := Row{ID: id, Ref: ix.Ref(id), Mod: b.tx.T("a removed mod"), InPlan: st.List.Index(id) >= 0}
	if m != nil {
		r.Mod = m.DisplayName()
		r.Present = m.Status == identity.Live
		if isNumber(m.WorkshopID()) {
			r.WorkshopID = m.WorkshopID()
		}
	}
	return r, m
}

// goneRow is the row of a mod no longer in its folder.
func (b *changeBuilder) goneRow(r Row, m *store.Mod) Row {
	tx := b.tx
	r.Tier, r.Text = TierWatch, tx.T("removed from the mod folder")
	if m != nil && m.Kind == source.Local {
		r.Text = tx.T("removed from the Mods folder")
	}
	if r.InPlan {
		r.Tier = TierLaunch
		r.Text = tx.Tf("%s; the current list uses it", r.Text)
	}
	r.TierText = TierText(tx, r.Tier)
	return r
}

// arrivedRow is the row of a mod found for the first time, or of a new copy.
func (b *changeBuilder) arrivedRow(r Row, m *store.Mod, kinds map[string]bool) Row {
	tx := b.tx
	r.Tier, r.TierText = TierContent, TierText(tx, TierContent)
	r.Text = tx.T("found in a mod folder")
	if m != nil && m.Kind == source.Local {
		r.Text = tx.T("found in the Mods folder")
	}
	if m != nil && m.Kind.WorkshopLike() {
		r.Text = tx.T("new subscription from Workshop")
	}
	if kinds[identity.EvCopied] {
		r.Text = tx.T("a new copy")
	}
	return r
}

// modFacts collects what a mod's changes mean.
type modFacts struct{ facts []fact }

// add is a fact the row's details also list.
func (f *modFacts) add(tier int, text string) { f.facts = append(f.facts, fact{tier, text, false}) }

// addFile is a fact the grouped file summary already says.
func (f *modFacts) addFile(tier int, text string) { f.facts = append(f.facts, fact{tier, text, true}) }

// declChange is what changed beyond the facts' sentences.
type declChange struct {
	deps, rules bool   // the dependencies, the load rules
	modVersion  string // "version 1.0 → 1.1", placed by the file facts
}

// launchTier is the tier of a change that can stop the game: TierLaunch for a mod the
// current list uses.
func launchTier(inPlan bool) int {
	if inPlan {
		return TierLaunch
	}
	return TierWatch
}

// declFacts compares the two declarations: package id, name, versions, dependencies,
// incompatibilities and load rules.
func (b *changeBuilder) declFacts(f *modFacts, inPlan bool, od, nd *scan.Decl) declChange {
	tx := b.tx
	var c declChange
	if od.PackageID != nd.PackageID {
		f.add(launchTier(inPlan), tx.Tf("package id changed: %s → %s", od.PackageID, nd.PackageID))
	}
	if od.Name != nd.Name && od.Name != "" {
		f.add(TierContent, tx.Tf("renamed from %s", od.Name))
	}
	b.versionFacts(f, inPlan, od, nd)
	if od.ModVersion != nd.ModVersion && nd.ModVersion != "" {
		if od.ModVersion != "" {
			c.modVersion = tx.Tf("version %s → %s", od.ModVersion, nd.ModVersion)
		} else {
			c.modVersion = tx.Tf("version %s", nd.ModVersion)
		}
	}
	c.deps = b.depFacts(f, inPlan, od, nd)
	ia, ir := setDiff(b.byMM(od.IncompatibleWith, od.IncompatibleByVersion), b.byMM(nd.IncompatibleWith, nd.IncompatibleByVersion))
	for _, p := range ia {
		name, state := b.pkgState(p)
		if state == DepEnabled && inPlan {
			f.add(TierLaunch, tx.Tf("now incompatible with %s (enabled)", name))
		} else {
			f.add(TierWatch, tx.Tf("now incompatible with %s", name))
		}
	}
	for _, p := range ir {
		name, _ := b.pkgState(p)
		f.add(TierContent, tx.Tf("no longer incompatible with %s", name))
	}
	c.rules = b.ruleFacts(f, od, nd)
	if !c.rules && od.LoadRulesKey() != nd.LoadRulesKey() && len(ia)+len(ir) == 0 {
		c.rules = true
		f.add(TierWatch, tx.T("load rules changed"))
	}
	return c
}

// versionFacts are the game versions the mod no longer supports, then the new ones.
func (b *changeBuilder) versionFacts(f *modFacts, inPlan bool, od, nd *scan.Decl) {
	tx := b.tx
	added, dropped := setDiff(od.SupportedVersions, nd.SupportedVersions)
	for _, v := range dropped {
		if v == b.mm {
			f.add(launchTier(inPlan), tx.Tf("no longer supports %s (your game)", v))
		} else {
			f.add(TierWatch, tx.Tf("no longer supports %s", v))
		}
	}
	for _, v := range added {
		if v == b.mm {
			f.add(TierWatch, tx.Tf("now supports %s (your game)", v))
		} else {
			f.add(TierWatch, tx.Tf("now supports %s", v))
		}
	}
}

// depFacts are the dependencies added and dropped; it reports whether there were any.
func (b *changeBuilder) depFacts(f *modFacts, inPlan bool, od, nd *scan.Decl) bool {
	tx := b.tx
	changed := false
	da, dr := setDiff(b.deps(od), b.deps(nd))
	for _, p := range da {
		changed = true
		name, state := b.pkgState(p)
		switch {
		case state == DepMissing:
			f.add(launchTier(inPlan), tx.Tf("needs %s now (not installed)", name))
		case state == DepDisabled && inPlan:
			f.add(TierLaunch, tx.Tf("needs %s now (not enabled)", name))
		default:
			f.add(TierWatch, tx.Tf("needs %s now", name))
		}
	}
	for _, p := range dr {
		changed = true
		name, _ := b.pkgState(p)
		f.add(TierWatch, tx.Tf("no longer needs %s", name))
	}
	return changed
}

// ruleFacts are the load-after and load-before rules added and dropped; it reports
// whether there were any.
func (b *changeBuilder) ruleFacts(f *modFacts, od, nd *scan.Decl) bool {
	tx := b.tx
	rules := false
	for _, x := range []struct {
		on, off  string
		old, cur []string
	}{
		{"now loads after %s", "no longer loads after %s", b.byMM(append(od.LoadAfter, od.ForceLoadAfter...), od.LoadAfterByVersion),
			b.byMM(append(nd.LoadAfter, nd.ForceLoadAfter...), nd.LoadAfterByVersion)},
		{"now loads before %s", "no longer loads before %s", b.byMM(append(od.LoadBefore, od.ForceLoadBefore...), od.LoadBeforeByVersion),
			b.byMM(append(nd.LoadBefore, nd.ForceLoadBefore...), nd.LoadBeforeByVersion)},
	} {
		on, off := setDiff(x.old, x.cur)
		if len(on) > 0 {
			rules = true
			f.add(TierWatch, tx.Tf(x.on, b.names(on)))
		}
		if len(off) > 0 {
			rules = true
			f.add(TierWatch, tx.Tf(x.off, b.names(off)))
		}
	}
	return rules
}

// eventFacts: no declaration to compare, what the events say.
func (b *changeBuilder) eventFacts(f *modFacts, inPlan bool, kinds map[string]bool, detail map[string]string) declChange {
	tx := b.tx
	var c declChange
	if d := detail[identity.EvPackageIDChanged]; d != "" {
		f.add(launchTier(inPlan), tx.Tf("package id changed: %s", strings.ReplaceAll(d, "pkg:", "")))
	}
	if d := detail[identity.EvVersionSupport]; d != "" {
		f.add(TierWatch, tx.Tf("supported versions: %s", d))
	}
	if kinds[identity.EvDependencies] {
		c.deps = true
		f.add(TierWatch, tx.T("dependencies changed"))
	}
	if kinds[identity.EvLoadRules] {
		c.rules = true
		f.add(TierWatch, tx.T("load rules changed"))
	}
	if d := detail[identity.EvRenamed]; d != "" {
		f.add(TierContent, tx.Tf("renamed: %s", d))
	}
	return c
}

// rowFiles is a row's files grouped by kind, their size change, and whether the file
// lists are no longer stored.
type rowFiles struct {
	groups  []fileGroup
	delta   int64
	missing bool
}

// fileFacts groups the changed files by kind and adds what they mean, the mod's new
// version among them.
func (b *changeBuilder) fileFacts(f *modFacts, r Row, info map[int64]*store.SnapInfo, modVersion string) rowFiles {
	tx := b.tx
	var fs rowFiles
	if r.HasFiles() && !b.noFiles {
		fs.groups, fs.delta, fs.missing = b.files(r.Before, r.After)
	}
	if (b.noFiles || fs.missing) && r.HasFiles() {
		// The snapshot rows keep their total size after maintenance cleans the file lists.
		if x, y := info[r.Before], info[r.After]; x != nil && y != nil {
			fs.delta = y.TotalSize - x.TotalSize
		}
	}
	code, defs := false, false
	var media []string
	for _, g := range fs.groups {
		switch g.kind {
		case fkCode:
			code = true
		case fkDefs, fkPatches:
			defs = true
		case fkAbout:
			// About.xml is what the facts above read; LoadFolders.xml decides which folders load.
			if g.loadFolders {
				f.add(TierWatch, tx.T("LoadFolders.xml changed"))
			}
		default:
			media = append(media, kindName(tx, g.kind))
		}
	}
	if code {
		f.addFile(TierWatch, tx.T("code changed"))
	}
	if modVersion != "" {
		if defs || code {
			f.add(TierWatch, modVersion)
		} else {
			f.add(TierContent, modVersion)
		}
	}
	if defs {
		var names []string
		for _, g := range fs.groups {
			if g.kind == fkDefs || g.kind == fkPatches {
				names = append(names, kindName(tx, g.kind))
			}
		}
		f.addFile(TierWatch, tx.Tf("%s changed", strings.Join(names, tx.T(", "))))
	}
	if !code && !defs && len(media) > 0 {
		f.addFile(TierContent, tx.Tf("only %s changed", strings.Join(media, tx.T(", "))))
	} else if len(media) > 0 {
		f.addFile(TierContent, tx.Tf("%s changed", strings.Join(media, tx.T(", "))))
	}
	return fs
}

// sayFacts sets the row's tier, its most serious fact's, and its sentence, the first
// facts in tier order; it returns the facts in that order.
func (b *changeBuilder) sayFacts(r *Row, facts []fact) []fact {
	tx := b.tx
	r.Tier = TierContent
	for _, f := range facts {
		r.Tier = min(r.Tier, f.tier)
	}
	r.TierText = TierText(tx, r.Tier)
	sorted := slices.Clone(facts)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].tier < sorted[j].tier })
	var say []string
	for i, f := range sorted {
		if i == 3 {
			break
		}
		say = append(say, f.text)
	}
	r.Text = strings.Join(say, tx.T("; "))
	var others []string
	for _, f := range sorted {
		if !f.file && len(others) < 3 {
			others = append(others, f.text)
		}
	}
	r.Others = strings.Join(others, tx.T("; "))
	return sorted
}

// rowDetails sets the row's file counts and details: the file summary, what the sentence
// had no room for, and when the Workshop updated it.
func (b *changeBuilder) rowDetails(r *Row, fs rowFiles, sorted []fact, info map[int64]*store.SnapInfo) {
	tx := b.tx
	for _, g := range fs.groups {
		if g.kind == fkAbout && !g.loadFolders {
			continue // About.xml's own changes are the facts above
		}
		fc := FileCount{Kind: kindName(tx, g.kind), Added: g.added, Changed: g.changed, Gone: g.gone}
		if g.kind == fkTextures || g.kind == fkSounds {
			fc.Delta = g.delta // media say their size, as the details do
		}
		r.Files = append(r.Files, fc)
	}
	if fs.missing {
		r.Details = append(r.Details, tx.T("(file lists no longer stored)"))
	}
	if s := groupText(tx, fs.groups, fs.delta); s != "" {
		r.Details = append(r.Details, s)
	} else if fs.delta != 0 && r.HasFiles() {
		r.Details = append(r.Details, tx.Tf("size %s", signedSize(fs.delta)))
	}
	// then what the sentence had no room for
	for i, f := range sorted {
		if i >= 3 && !f.file {
			r.Details = append(r.Details, f.text)
		}
	}
	if x, y := info[r.Before], info[r.After]; y != nil && y.ACFTimeUpdated != 0 && (x == nil || x.ACFTimeUpdated != y.ACFTimeUpdated) {
		r.Details = append(r.Details, tx.Tf("updated on the Workshop %s", tx.Date(time.Unix(y.ACFTimeUpdated, 0))))
	}
}

// rowActions adds the actions that apply: the change notes, dependencies, sort, lock.
func (b *changeBuilder) rowActions(r *Row, m *store.Mod, kinds map[string]bool, c declChange) {
	tx := b.tx
	if r.WorkshopID != "" {
		r.Actions = append(r.Actions, Action{ActWorkshop, tx.T("open the change notes"), "ctrl+w"})
	}
	if c.deps && r.Tier == TierLaunch && r.InPlan {
		r.Actions = append(r.Actions, Action{ActDeps, tx.T("see dependencies"), "D"})
	}
	if (c.deps || c.rules) && r.InPlan {
		r.Actions = append(r.Actions, Action{ActSort, tx.T("sort"), "S"})
	}
	if kinds[identity.EvUpdated] && m != nil && m.Status == identity.Live && m.Kind.WorkshopLike() && b.locks[r.ID] == nil && r.InPlan {
		r.Actions = append(r.Actions, Action{ActLock, tx.T("keep this updated version"), "p"})
	}
}

// copiedAt is when a lock was made (origin.json), else when its copy was first seen.
func copiedAt(l *app.Locked) time.Time {
	if t, err := time.Parse(time.RFC3339, l.Origin.CopiedAt); err == nil {
		return t
	}
	return l.Copy.FirstSeen
}

// deps is a declaration's dependencies for the game version, lower-cased.
func (b *changeBuilder) deps(d *scan.Decl) []string {
	var out []string
	for _, x := range append(slices.Clone(d.Dependencies), d.DependenciesByVersion["v"+b.mm]...) {
		if p := strings.ToLower(x.PackageID); p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// byMM is a rule list plus its by-version list for the game version, lower-cased.
func (b *changeBuilder) byMM(base []string, byVer map[string][]string) []string {
	var out []string
	for _, x := range append(slices.Clone(base), byVer["v"+b.mm]...) {
		if p := strings.ToLower(strings.TrimSpace(x)); p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// pkgState names a package id and says whether it is installed and enabled (view.Deps).
func (b *changeBuilder) pkgState(pkg string) (string, string) {
	name, state := pkg, DepMissing
	for _, m := range b.st.Ix.Mods {
		if m.PackageID != pkg || m.Status != identity.Live {
			continue
		}
		if b.st.List.Index(m.ID) >= 0 {
			return m.DisplayName(), DepEnabled
		}
		name, state = m.DisplayName(), DepDisabled
	}
	return name, state
}

// names lists package ids by name: "A, B +2".
func (b *changeBuilder) names(pkgs []string) string {
	tx := b.tx
	var out []string
	for i, p := range pkgs {
		if i == 2 {
			out = append(out, fmt.Sprintf("+%d", len(pkgs)-2))
			break
		}
		n, _ := b.pkgState(p)
		out = append(out, n)
	}
	return strings.Join(out, tx.T(", "))
}

// setDiff: what cur adds to old, and what it drops.
func setDiff(old, cur []string) (added, dropped []string) {
	for _, x := range cur {
		if !slices.Contains(old, x) {
			added = append(added, x)
		}
	}
	for _, x := range old {
		if !slices.Contains(cur, x) {
			dropped = append(dropped, x)
		}
	}
	return
}

// File kinds, in the order the summary lists them (their English names are the keys).
const (
	fkCode     = "code"
	fkDefs     = "Defs"
	fkPatches  = "Patches"
	fkAbout    = "About/LoadFolders"
	fkTextures = "textures"
	fkSounds   = "sounds"
	fkLang     = "translations"
	fkOther    = "other files"
)

var fileKinds = []string{fkCode, fkDefs, fkPatches, fkAbout, fkTextures, fkSounds, fkLang, fkOther}

// fileKind sorts a lower-cased path into a kind.
func fileKind(p string) string {
	seg := func(name string) bool { return strings.HasPrefix(p, name+"/") || strings.Contains(p, "/"+name+"/") }
	switch {
	case strings.HasSuffix(p, ".dll"):
		return fkCode
	case seg("defs"):
		return fkDefs
	case seg("patches"):
		return fkPatches
	case seg("about") || p == "loadfolders.xml":
		return fkAbout
	case seg("languages"):
		return fkLang
	case seg("textures"):
		return fkTextures
	case seg("sounds"):
		return fkSounds
	}
	return fkOther
}

type fileGroup struct {
	loadFolders          bool // LoadFolders.xml is among the About/LoadFolders files
	kind                 string
	added, changed, gone int
	delta                int64
}

// fileChange is one file that differs between two snapshots: '+', '~' or '-'.
type fileChange struct {
	op         byte
	rel        string
	size, prev int64
}

// corpusOnly: an Archo Monitor change that touched only its corpus data folder, which the
// game never loads (19 §6). Such data-only updates are hidden from Updates.
func (b *changeBuilder) corpusOnly(m *store.Mod, r Row) bool {
	if !app.IsMonitor(m) || r.Before == 0 || r.After == 0 || r.Before == r.After {
		return false
	}
	x, err1 := b.a.Store.SnapshotFiles(r.Before)
	y, err2 := b.a.Store.SnapshotFiles(r.After)
	if err1 != nil || err2 != nil || x == nil || y == nil {
		return false
	}
	cs := diffFiles(x, y)
	if len(cs) == 0 {
		return false
	}
	d := strings.ToLower(corpus.Dir) + "/"
	for _, c := range cs {
		if !strings.HasPrefix(strings.ToLower(filepath.ToSlash(c.rel)), d) {
			return false
		}
	}
	return true
}

func diffFiles(x, y map[string]store.SnapFile) []fileChange {
	var out []fileChange
	for k, f := range y {
		o, ok := x[k]
		switch {
		case !ok:
			out = append(out, fileChange{'+', f.Rel, f.Size, 0})
		case o.Hash != nil && f.Hash != nil:
			if string(o.Hash) != string(f.Hash) {
				out = append(out, fileChange{'~', f.Rel, f.Size, o.Size})
			}
		case o.Size != f.Size:
			out = append(out, fileChange{'~', f.Rel, f.Size, o.Size})
		}
	}
	for k, f := range x {
		if _, ok := y[k]; !ok {
			out = append(out, fileChange{'-', f.Rel, 0, f.Size})
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].rel) < strings.ToLower(out[j].rel) })
	return out
}

// files groups what differs between two snapshots by kind, with the total size change.
func (b *changeBuilder) files(before, after int64) ([]fileGroup, int64, bool) {
	x, err1 := b.a.Store.SnapshotFiles(before)
	y, err2 := b.a.Store.SnapshotFiles(after)
	if err1 != nil || err2 != nil || x == nil || y == nil {
		return nil, 0, true
	}
	by := map[string]*fileGroup{}
	var delta int64
	for _, c := range diffFiles(x, y) {
		k := fileKind(strings.ToLower(c.rel))
		g := by[k]
		if g == nil {
			g = &fileGroup{kind: k}
			by[k] = g
		}
		if strings.EqualFold(c.rel, "loadfolders.xml") {
			g.loadFolders = true
		}
		switch c.op {
		case '+':
			g.added++
		case '-':
			g.gone++
		default:
			g.changed++
		}
		g.delta += c.size - c.prev
		delta += c.size - c.prev
	}
	var out []fileGroup
	for _, k := range fileKinds {
		if g := by[k]; g != nil {
			out = append(out, *g)
		}
	}
	return out, delta, false
}

// groupText: "Defs: 4 changed, 1 added; textures +3.2 MB; total +3.4 MB".
func groupText(tx Text, gs []fileGroup, delta int64) string {
	if len(gs) == 0 {
		return ""
	}
	var parts []string
	for _, g := range gs {
		var p []string
		media := g.kind == fkTextures || g.kind == fkSounds
		if !media || g.delta == 0 {
			if g.changed > 0 {
				p = append(p, tx.Tf("%d changed", g.changed))
			}
			if g.added > 0 {
				p = append(p, tx.Tf("%d new", g.added))
			}
			if g.gone > 0 {
				p = append(p, tx.Tf("%d deleted", g.gone))
			}
		}
		s := kindName(tx, g.kind) + " " + strings.Join(p, tx.T(", "))
		if media && g.delta != 0 {
			s = kindName(tx, g.kind) + " " + signedSize(g.delta)
		}
		parts = append(parts, strings.TrimSpace(s))
	}
	s := strings.Join(parts, tx.T("; "))
	if delta != 0 && len(gs) > 1 {
		s += tx.T(" · ") + tx.Tf("in all %s", signedSize(delta))
	}
	return s
}

func signedSize(n int64) string {
	if n < 0 {
		return "−" + render.Size(-n)
	}
	return "+" + render.Size(n)
}

// Files is the raw file list behind a row: "+ path  1.2 MB", "~ path", "- path".
func Files(a *app.App, tx Text, before, after int64) []string {
	if before == 0 || after == 0 || before == after {
		return []string{tx.T("(no file-level difference recorded)")}
	}
	x, err1 := a.Store.SnapshotFiles(before)
	y, err2 := a.Store.SnapshotFiles(after)
	if err1 != nil || err2 != nil || x == nil || y == nil {
		return []string{tx.T("(file lists no longer stored)")}
	}
	var out []string
	for _, c := range diffFiles(x, y) {
		l := string(c.op) + " " + c.rel
		switch c.op {
		case '+':
			l += "  " + render.Size(c.size)
		case '~':
			if c.size != c.prev {
				l += "  " + signedSize(c.size-c.prev)
			}
		}
		out = append(out, l)
	}
	if len(out) == 0 {
		return []string{tx.T("(the files are the same)")}
	}
	if len(out) > 300 {
		out = append(out[:300], tx.Tf("… %d more", len(out)-300))
	}
	return out
}

// Home is the home page's line about the current plan (16 §2.3).
type Home struct {
	Title  string `json:"title"` // "the current plan since you last played"
	Text   string `json:"text"`  // "5 mods updated, 1 needs attention" / "nothing changed"
	Count  int    `json:"count"`
	Launch int    `json:"launch"`
	NoScan bool   `json:"noScan,omitempty"`
}

// BuildHome counts the current plan's changes since it was last played (the last
// scan when it never was), without reading file lists.
func BuildHome(a *app.App, st *app.LoadoutState, tx Text) *Home {
	p, err := Build(a, st, Opts{NoFiles: true, Older: -1}, tx)
	if err != nil || p.NoScan {
		return &Home{NoScan: true, Title: tx.T("the current plan"), Text: tx.T("no scan yet (R scans)")}
	}
	h := &Home{Count: len(p.Rows), Launch: p.Launch, Title: tx.T("the current plan since the last scan")}
	if p.Played {
		h.Title = tx.T("the current plan since you last played")
	}
	if p.Arrivals != nil {
		h.Count += len(p.Arrivals.Rows)
	}
	for _, r := range groupRows(p.Arrivals) {
		if r.Tier == TierLaunch {
			h.Launch++
		}
	}
	h.Text = homeText(tx, h.Count, h.Launch)
	return h
}

// homeText is the line's count: "3 mods changed, 1 need attention"; one changed Mod is
// singular (it read "1 mods changed", VM 2026-11-22).
func homeText(tx Text, count, launch int) string {
	switch {
	case count == 0:
		return tx.T("nothing changed")
	case count == 1 && launch > 0:
		return tx.T("1 mod changed, it needs attention")
	case launch > 0:
		return tx.Tf("%d mods changed, %d need attention", count, launch)
	case count == 1:
		return tx.T("1 mod changed")
	default:
		return tx.Tf("%d mods changed", count)
	}
}

// Dependency states (as view.Deps).
const (
	DepEnabled  = "enabled"
	DepDisabled = "disabled"
	DepMissing  = "missing"
)

func isNumber(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// eventText is an event kind in words (view.EventText).
func eventText(tx Text, kind string) string {
	switch kind {
	case identity.EvMoved:
		return tx.T("moved")
	case identity.EvReappeared:
		return tx.T("reappeared")
	case identity.EvRenamed:
		return tx.T("renamed")
	case identity.EvAuthorChanged:
		return tx.T("author changed")
	case identity.EvVersionSupport:
		return tx.T("supported versions changed")
	case identity.EvDependencies:
		return tx.T("dependencies changed")
	case identity.EvLoadRules:
		return tx.T("load order rules changed")
	case identity.EvPackageIDChanged:
		return tx.T("package id changed")
	}
	return tx.T("updated")
}

// Count is the number of mods on the page: the rows and new and gone, in range and older.
func (p *Page) Count() int {
	n := len(p.Rows) + len(groupRows(p.Arrivals))
	for _, d := range p.Older {
		n += len(d.Rows) + len(groupRows(d.Arrivals))
	}
	return n
}

func groupRows(g *Group) []Row {
	if g == nil {
		return nil
	}
	return g.Rows
}

// kindName is a file kind in words ("files.<kind>" in the translation table).
func kindName(tx Text, k string) string {
	if n := tx.T("files." + k); n != "files."+k {
		return n
	}
	return k
}
