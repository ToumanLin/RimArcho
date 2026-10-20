package cmds

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

func init() {
	register(&Command{Name: "scan", Summary: "scan sources and reconcile identities", MaxArgs: 0,
		Flags: []Flag{{Name: "full", Help: "re-hash every semantic file"}}, Run: scanCmd})
	register(&Command{Name: "status", Summary: "overview of the current environment", MaxArgs: 0, Run: statusCmd})
	register(&Command{Name: "list", Summary: "list mods", MaxArgs: 0,
		Flags: []Flag{{Name: "filter", Value: "<expr>", Help: "terms: source:<label|kind> status:live|tombstone|all residue unloadable copies pkg:<id> ws:<id> tag:<tag> or words"},
			{Name: "active", Help: "the workspace Loadout in load order"}, {Name: "all", Help: "no truncation"}}, Run: listCmd})
	register(&Command{Name: "find", Args: "<words…>", Summary: "search by name, package id, author, workshop id, folder", MinArgs: 1, MaxArgs: -1,
		Flags: []Flag{{Name: "all", Help: "no truncation"}}, Run: findCmd})
	register(&Command{Name: "mod", Args: "<ref>", Summary: "mod details", MinArgs: 1, MaxArgs: 1, Run: modCmd,
		Flags: []Flag{{Name: "diff", Value: "<ref>", Help: "list the semantic files that differ from another instance"}}})
	register(&Command{Name: "changes", Args: "[<ref>]", Summary: "change events (default: the last scan)", MaxArgs: 1,
		Flags: []Flag{{Name: "since", Value: "scan:<n>|<days>d|YYYY-MM-DD"}, {Name: "all", Help: "no truncation"}}, Run: changesCmd})
}

// eventOrder fixes the order kinds are summarised in.
var eventOrder = []string{identity.EvDiscovered, identity.EvUpdated, identity.EvMoved, identity.EvCopied, identity.EvReappeared,
	identity.EvDisappeared, identity.EvPackageIDChanged, identity.EvRenamed, identity.EvAuthorChanged, identity.EvVersionSupport,
	identity.EvDependencies, identity.EvLoadRules}

func eventSummary(counts map[string]int) string {
	var parts []string
	for _, k := range eventOrder {
		if n := counts[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, k))
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func scanCmd(c *Ctx, a *Args) (*render.Doc, error) {
	r, err := c.App.Scan(a.Bool("full"))
	if err != nil {
		return nil, err
	}
	st := r.Stats
	line := fmt.Sprintf("scan:%d  %s  %s", r.ScanID, envRef(r.Env), app.Elapsed(st.DurationMS))
	if r.Full {
		line += "  [full]"
	}
	d, root := render.New(line)
	srcs := root.Section("sources")
	var rows [][]string
	for _, s := range st.Sources {
		rows = append(rows, []string{"src:" + s.Label, sourceSummary(s)})
	}
	srcs.AddColumns(rows)
	root.KV("instances", fmt.Sprintf("%d live, %d tombstones", st.Live, st.Tombstones))
	root.KV("files", fmt.Sprintf("%d (%s), semantic hashed %d, reused %d", st.Files, render.Size(st.Bytes), st.Hashed, st.Reused))
	root.KV("changes", eventSummary(st.Events))
	if st.Questions > 0 {
		root.KV("questions", fmt.Sprintf("%d new (archo identity list)", st.Questions))
	}
	for _, n := range r.Notices {
		root.KV("loadout", n)
	}
	if len(st.Events) > 0 {
		d.AddNext("archo changes")
	}
	if st.Questions > 0 {
		d.AddNext("archo identity list")
	}
	return d, nil
}

func sourceSummary(s app.SourceStat) string {
	switch s.Status {
	case source.StatusMissing:
		return "missing (root not found; instances kept, not scanned)"
	case source.StatusRebindPending:
		return "rebind pending (not scanned; archo env list)"
	case source.StatusRetired:
		return "retired"
	}
	if s.Err != "" {
		return "error: " + s.Err
	}
	text := render.Plural(s.Mods, "mod")
	var extra []string
	if s.Folders != s.Mods {
		extra = append(extra, render.Plural(s.Folders, "folder"))
	}
	if s.Residue > 0 {
		extra = append(extra, fmt.Sprintf("%d residue", s.Residue))
	}
	if s.Disabled > 0 {
		extra = append(extra, fmt.Sprintf("%d about-disabled", s.Disabled))
	}
	if n := s.NoAbout + s.ResidueNoAbout; n > 0 {
		extra = append(extra, fmt.Sprintf("%d without About.xml", n))
	}
	if s.Aliases > 0 {
		extra = append(extra, render.Plural(s.Aliases, "alias"))
	}
	if (s.Kind == string(source.Workshop) || s.Kind == string(source.SteamCMD)) && !s.HasACF {
		extra = append(extra, "no ACF")
	}
	if len(extra) > 0 {
		text += " (" + strings.Join(extra, ", ") + ")"
	}
	return text
}

func statusCmd(c *Ctx, a *Args) (*render.Doc, error) {
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	e := ix.Env
	d, root := render.New(envRef(e) + "  RimWorld " + versionText(e.GameVersion))
	live, tomb, residue := 0, 0, 0
	perSource := map[string]int{}
	for _, m := range ix.Mods {
		if m.Status == identity.Live {
			live++
			perSource[m.SourceLabel]++
			if m.Residue {
				residue++
			}
		} else {
			tomb++
		}
	}
	var parts []string
	for _, s := range ix.Sources {
		if n := perSource[s.Label]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", s.Label, n))
		}
	}
	mods := fmt.Sprintf("%d live", live)
	if len(parts) > 0 {
		mods += " (" + strings.Join(parts, ", ") + ")"
	}
	root.KV("mods", fmt.Sprintf("%s, %d tombstones", mods, tomb))
	if residue > 0 {
		root.KV("residue", fmt.Sprintf("%d (unsubscribed workshop folders still on disk; the game does not load them: archo list --filter residue)", residue))
	}
	withCopies := 0
	for _, l := range ix.ByLineage {
		if len(l) > 1 {
			withCopies++
		}
	}
	root.KV("lineages", fmt.Sprintf("%d (%d with several instances)", len(ix.ByLineage), withCopies))
	sc, st, err := c.App.LastScan(e.ID)
	if err != nil {
		return nil, err
	}
	if sc == nil {
		root.KV("last-scan", "never")
		d.AddNext("archo scan")
	} else {
		root.KV("last-scan", fmt.Sprintf("scan:%d %s, %s", sc.ID, c.App.FormatTime(sc.Started), app.Elapsed(st.DurationMS)))
		root.KV("changes", fmt.Sprintf("%s (archo changes)", eventSummary(st.Events)))
	}
	qs, err := c.App.Store.Questions(e.ID, false)
	if err != nil {
		return nil, err
	}
	if len(qs) > 0 {
		root.KV("questions", fmt.Sprintf("%d open (archo identity list)", len(qs)))
		d.AddNext("archo identity list")
	}
	for _, s := range ix.Sources {
		switch s.Status {
		case source.StatusMissing:
			root.KV("warning", fmt.Sprintf("%s root missing: %s (archo env set)", s.Ref(), s.Root))
		case source.StatusRebindPending:
			root.KV("warning", fmt.Sprintf("%s re-binding pending (archo env rebind %s yes|no)", s.Ref(), s.Ref()))
		}
	}
	loadoutStatus(c, root, d)
	// Status never parses the log: it uses the stored report (logs are a problems command).
	c.App.LogCacheOnly = true
	ps, err := c.App.Problems(false, nil)
	c.App.LogCacheOnly = false
	if err == nil && ps.State != nil {
		line := severityCounts(ps.List) + " (archo problems)"
		if ps.Analysis == nil || ps.Analysis.Run == nil {
			line = severityCounts(ps.List) + " in declarations; Defs and Patches not analysed yet (archo problems)"
		} else if !ps.Analysis.Fresh {
			line = severityCounts(ps.List) + "; Def/Patch analysis stale (archo problems)"
		}
		root.KV("problems", line)
		for _, r := range ps.Logs {
			text := logRunText(c, r)
			switch {
			case r.Pending:
				text += ": not analysed yet, so its errors are not in the counts (archo problems)"
			case r.Stale():
				var lp []*app.Problem
				for _, p := range ps.Stale {
					if p.Log == r {
						lp = append(lp, p)
					}
				}
				text += ": " + shortCounts(lp) + ", not counted (archo problems --code log --all)"
			}
			root.KV("log", text)
		}
	}
	return d, nil
}

// filterMods applies a list --filter expression; all terms must match.
func filterMods(ix *app.Index, expr string) ([]*store.Mod, error) {
	status := identity.Live
	var preds []func(*store.Mod) bool
	for _, t := range strings.Fields(expr) {
		k, v, hasKV := strings.Cut(t, ":")
		k = strings.ToLower(k)
		switch {
		case hasKV && k == "status":
			switch v {
			case "live", "tombstone", "all":
				status = v
			default:
				return nil, &UsageError{Msg: "status: must be live, tombstone or all"}
			}
		case hasKV && k == "source":
			v := strings.ToLower(v)
			preds = append(preds, func(m *store.Mod) bool { return m.SourceLabel == v || string(m.Kind) == v })
		case hasKV && k == "pkg":
			v := strings.ToLower(v)
			preds = append(preds, func(m *store.Mod) bool { return m.PackageID == v })
		case hasKV && k == "ws":
			preds = append(preds, func(m *store.Mod) bool { return m.WorkshopID() == v })
		case !hasKV && strings.ToLower(t) == "residue":
			preds = append(preds, func(m *store.Mod) bool { return m.Residue })
		case !hasKV && strings.ToLower(t) == "unloadable":
			preds = append(preds, func(m *store.Mod) bool { return m.Unloadable != "" })
		case !hasKV && strings.ToLower(t) == "copies":
			preds = append(preds, func(m *store.Mod) bool { return len(ix.ByLineage[m.LineageID]) > 1 })
		case hasKV && k == "tag":
			preds = append(preds, func(m *store.Mod) bool {
				return slices.ContainsFunc(m.Tags, func(t string) bool { return strings.EqualFold(t, v) })
			})
		case hasKV && k == "lineage":
			lin, err := ix.ResolveLineage("L#" + strings.TrimPrefix(strings.ToLower(v), "l#"))
			if err != nil {
				return nil, err
			}
			preds = append(preds, func(m *store.Mod) bool { return m.LineageID == lin })
		default:
			w := strings.ToLower(t)
			preds = append(preds, func(m *store.Mod) bool { return matchWord(m, w) })
		}
	}
	var out []*store.Mod
	for _, m := range ix.Mods {
		if status != "all" && m.Status != status {
			continue
		}
		ok := true
		for _, p := range preds {
			if !p(m) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, m)
		}
	}
	return out, nil
}

func matchWord(m *store.Mod, w string) bool {
	for _, f := range []string{m.Name, m.Alias, m.PackageID, m.Authors, m.WorkshopID(), m.Rel} {
		if strings.Contains(strings.ToLower(f), w) {
			return true
		}
	}
	return false
}

func listCmd(c *Ctx, a *Args) (*render.Doc, error) {
	if a.Bool("active") {
		return activeList(c, a)
	}
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	expr := a.Str("filter")
	ms, err := filterMods(ix, expr)
	if err != nil {
		return nil, err
	}
	head := fmt.Sprintf("mods: %d (%s", len(ms), envRef(ix.Env))
	if expr != "" {
		head += ", filter: " + expr
	}
	d, root := render.New(head + ")")
	limit := render.DefaultLimit
	if a.Bool("all") {
		limit = 0
	}
	expand := "archo list --all"
	if expr != "" {
		expand = "archo list --filter " + quote(expr) + " --all"
	}
	addMods(c, root, ix, ms, limit, expand)
	if len(ms) == 0 && ix.Env != nil && len(ix.Mods) == 0 {
		d.AddNext("archo scan")
	}
	return d, nil
}

func findCmd(c *Ctx, a *Args) (*render.Doc, error) {
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	var words []string
	for _, w := range a.Pos {
		words = append(words, strings.ToLower(w))
	}
	var ms []*store.Mod
	for _, m := range ix.Mods {
		ok := true
		for _, w := range words {
			if !matchWord(m, w) {
				ok = false
				break
			}
		}
		if ok {
			ms = append(ms, m)
		}
	}
	q := strings.Join(a.Pos, " ")
	d, root := render.New(fmt.Sprintf("found: %d for %q", len(ms), q))
	limit := render.DefaultLimit
	if a.Bool("all") {
		limit = 0
	}
	addMods(c, root, ix, ms, limit, "archo find "+quote(q)+" --all")
	if len(ms) == 1 {
		d.AddNext("archo mod " + ix.Ref(ms[0].ID))
	}
	return d, nil
}

func modCmd(c *Ctx, a *Args) (*render.Doc, error) {
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	m, err := ix.ResolveOne(a.Pos[0])
	if err != nil {
		return nil, err
	}
	if ref := a.Str("diff"); ref != "" {
		o, err := ix.ResolveOne(ref)
		if err != nil {
			return nil, err
		}
		lines, err := c.App.DiffInstances(m, o)
		if err != nil {
			return nil, err
		}
		d, root := render.New(fmt.Sprintf("%s → %s: %d semantic files differ", ix.Ref(m.ID), ix.Ref(o.ID), len(lines)))
		for _, l := range lines {
			root.Add(l)
		}
		root.Truncate(render.DefaultLimit, "")
		return d, nil
	}
	return modDoc(c, ix, m)
}

// pkgRefs renders package ids with the instances that provide them.
func pkgRefs(ix *app.Index, pkgs []string) string {
	var parts []string
	for _, p := range pkgs {
		p = strings.ToLower(p)
		var refs []string
		for _, m := range ix.Mods {
			if m.PackageID == p && m.Status == identity.Live {
				refs = append(refs, ix.Ref(m.ID))
			}
		}
		s := "pkg:" + p
		if len(refs) == 0 {
			s += " (not installed)"
		} else {
			s += " (" + strings.Join(refs, ", ") + ")"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ", ")
}

// forVersion merges a base list with the entry for the game's version ("v1.6").
func forVersion(base []string, byVersion map[string][]string, mm string) []string {
	out := append([]string(nil), base...)
	for _, x := range byVersion["v"+mm] {
		dup := false
		for _, y := range out {
			if strings.EqualFold(x, y) {
				dup = true
			}
		}
		if !dup {
			out = append(out, x)
		}
	}
	return out
}

func modDoc(c *Ctx, ix *app.Index, m *store.Mod) (*render.Doc, error) {
	ref := ix.Ref(m.ID)
	d, root := render.New(fmt.Sprintf("%s %s  %s", ref, m.DisplayName(), tags(c, ix, m)))
	decl := m.Decl()
	switch {
	case m.Status == identity.Tombstone:
		until := m.TombstoneAt.AddDate(0, 0, c.App.Config.TombstoneDays)
		root.KV("status", fmt.Sprintf("tombstone since %s (kept until %s)", c.App.FormatDate(m.TombstoneAt), c.App.FormatDate(until)))
	case m.Residue:
		root.KV("status", "residue: not subscribed in Steam; the game will not load it")
	default:
		root.KV("status", "live")
	}
	pkg := m.PackageID
	if decl.PackageIDDerived {
		pkg += " (inferred: About.xml has no <packageId>)"
	} else if m.PackageIDRaw != "" && m.PackageIDRaw != m.PackageID {
		pkg += " (declared " + m.PackageIDRaw + ")"
	}
	root.KV("package", pkg)
	if m.Alias != "" {
		root.KV("alias", fmt.Sprintf("%s (About.xml name: %s)", m.Alias, m.Name))
	}
	if len(m.Tags) > 0 {
		root.KV("tags", strings.Join(m.Tags, ", "))
	}
	if m.Note != "" {
		root.KV("note", m.Note)
	}
	lin := ix.LRef(m.LineageID)
	if l := ix.Lineages[m.LineageID]; l != nil && l.PrimaryID != "" && l.PrimaryID != m.ID {
		lin += ", primary " + ix.Ref(l.PrimaryID)
	}
	if b := app.Basis(m.LineageBasis); b != "" {
		lin += ", membership " + b
	}
	ln := root.KV("lineage", lin)
	var others []*store.Mod
	for _, o := range ix.ByLineage[m.LineageID] {
		if o.ID != m.ID {
			others = append(others, o)
		}
	}
	if len(others) > 0 {
		addMods(c, ln, ix, others, 10, "archo list --filter lineage:"+ix.LRef(m.LineageID)[2:])
	}
	root.KV("authors", strings.Join(decl.Authors, ", "))
	root.KV("version", decl.ModVersion)
	root.KV("supports", strings.Join(decl.SupportedVersions, " "))
	mm := env.MajorMinor(ix.Env.GameVersion)
	var deps []string
	for _, x := range decl.Dependencies {
		deps = append(deps, x.PackageID)
	}
	byV := map[string][]string{}
	for v, l := range decl.DependenciesByVersion {
		for _, x := range l {
			byV[v] = append(byV[v], x.PackageID)
		}
	}
	if dl := forVersion(deps, byV, mm); len(dl) > 0 {
		root.KV("depends", pkgRefs(ix, dl))
	}
	for _, r := range []struct {
		key  string
		base []string
		byV  map[string][]string
	}{
		{"load-before", decl.LoadBefore, decl.LoadBeforeByVersion},
		{"load-after", decl.LoadAfter, decl.LoadAfterByVersion},
		{"force-load-before", decl.ForceLoadBefore, nil},
		{"force-load-after", decl.ForceLoadAfter, nil},
		{"incompatible", decl.IncompatibleWith, decl.IncompatibleByVersion},
	} {
		if l := forVersion(r.base, r.byV, mm); len(l) > 0 {
			var ps []string
			for _, p := range l {
				ps = append(ps, "pkg:"+strings.ToLower(p))
			}
			root.KV(r.key, strings.Join(ps, " "))
		}
	}
	if err := addModProblems(c, ix, m, root); err != nil {
		return nil, err
	}
	loc := root.KV("location", fmt.Sprintf("src:%s %s", m.SourceLabel, m.Rel))
	if s := ix.Source(m.SourceID); s != nil {
		path := s.Root + string(pathSep) + m.Rel
		loc.KV("path", path)
		realRoot, err := source.RealPath(s.Root)
		if err != nil {
			realRoot = s.Root
		}
		if m.RealPath != "" && !strings.EqualFold(m.RealPath, realRoot+string(pathSep)+m.Rel) {
			loc.KV("links-to", m.RealPath)
		}
	}
	aliases, err := c.App.Store.Aliases(m.ID)
	if err != nil {
		return nil, err
	}
	if len(aliases) > 0 {
		an := root.Section("aliases")
		for _, al := range aliases {
			an.Addf("src:%s %s", al.SourceLabel, al.Rel)
		}
	}
	if ws := m.WorkshopID(); ws != "" {
		w := "ws:" + ws
		if m.ACFManifest != "" {
			w += fmt.Sprintf(", manifest %s, updated %s", m.ACFManifest, c.App.FormatDate(unix(m.ACFTimeUpdated)))
		}
		root.KV("workshop", w)
	}
	root.KV("git", m.GitHead)
	if m.Origin != "" {
		root.KV("origin", ".rimarcho/origin.json")
	}
	root.KV("files", fmt.Sprintf("%d (%s), %d semantic", m.FileCount, render.Size(m.TotalSize), m.SemanticCount))
	root.KV("seen", fmt.Sprintf("first %s, last %s", c.App.FormatTime(m.FirstSeen), c.App.FormatTime(m.LastSeen)))
	evs, err := c.App.Store.Events(store.EventFilter{EnvID: ix.Env.ID, InstanceIDs: []string{m.ID}})
	if err != nil {
		return nil, err
	}
	if len(evs) > 0 {
		last := evs[len(evs)-1]
		root.KV("changes", fmt.Sprintf("%d, latest %s %s (archo changes %s)", len(evs), last.Kind, c.App.FormatDate(last.ScanTime), ref))
	}
	qs, err := c.App.Store.Questions(ix.Env.ID, false)
	if err != nil {
		return nil, err
	}
	for _, q := range qs {
		switch m.ID {
		case q.InstanceID:
			root.KV("question", fmt.Sprintf("q:%d may be %s back (archo identity list)", q.ID, ix.Ref(q.CandidateID)))
		case q.CandidateID:
			root.KV("question", fmt.Sprintf("q:%d %s may be this mod back (archo identity list)", q.ID, ix.Ref(q.InstanceID)))
		}
	}
	if decl.ParseError != "" {
		root.KV("warning", "About.xml: "+decl.ParseError)
	}
	if len(evs) > 0 {
		d.AddNext("archo changes " + ref)
	}
	return d, nil
}

func changesCmd(c *Ctx, a *Args) (*render.Doc, error) {
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	f := store.EventFilter{EnvID: ix.Env.ID}
	scope := ""
	if len(a.Pos) == 1 {
		ms, err := ix.Resolve(a.Pos[0])
		if err != nil {
			return nil, err
		}
		for _, m := range ms {
			f.InstanceIDs = append(f.InstanceIDs, m.ID)
		}
		scope = a.Pos[0]
	}
	if since := a.Str("since"); since != "" {
		if err := parseSince(c, since, &f); err != nil {
			return nil, err
		}
		scope = strings.TrimSpace(scope + " since " + since)
	} else if len(a.Pos) == 0 {
		scans, err := c.App.Store.Scans(ix.Env.ID, 1)
		if err != nil {
			return nil, err
		}
		if len(scans) == 0 {
			d, _ := render.New("changes: 0 (no scan yet)")
			d.AddNext("archo scan")
			return d, nil
		}
		f.MinScan, f.MaxScan = scans[0].ID, scans[0].ID
		scope = fmt.Sprintf("scan:%d", scans[0].ID)
	}
	evs, err := c.App.Store.Events(f)
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("changes: %d (%s)", len(evs), scope))
	limit := render.DefaultLimit
	if a.Bool("all") {
		limit = 0
	}
	shown := 0
	var group *render.Node
	var rows [][]string
	counts := map[string]int{}
	flush := func() {
		if group != nil {
			group.Line += "  " + eventSummary(counts)
			group.AddColumns(rows)
		}
		rows, counts = nil, map[string]int{}
	}
	var cur int64 = -1
	for _, ev := range evs {
		if ev.ScanID != cur {
			flush()
			cur = ev.ScanID
			group = root.Addf("scan:%d  %s", ev.ScanID, c.App.FormatTime(ev.ScanTime))
		}
		counts[ev.Kind]++
		if limit > 0 && shown >= limit {
			continue
		}
		shown++
		name := ""
		if m := ix.ByID[ev.InstanceID]; m != nil {
			name = m.DisplayName()
		}
		rows = append(rows, []string{ev.Kind, ix.Ref(ev.InstanceID), name, shortRefs(ix, ev.Detail)})
	}
	flush()
	if shown < len(evs) {
		expand := "archo changes"
		if len(a.Pos) == 1 {
			expand += " " + a.Pos[0]
		}
		if s := a.Str("since"); s != "" {
			expand += " --since " + s
		}
		root.Addf("… %d more (%s --all)", len(evs)-shown, expand)
	}
	if len(evs) == 0 && len(a.Pos) == 0 && a.Str("since") == "" {
		d.AddNext("archo changes --since 30d")
	}
	return d, nil
}

// activeList is `list --active`: the workspace in load order, missing entries included.
func activeList(c *Ctx, a *Args) (*render.Doc, error) {
	st, err := c.App.Loadout()
	if err != nil {
		return nil, err
	}
	c.st, c.stDone = st, true
	expr := a.Str("filter")
	var keep map[string]bool
	if expr != "" {
		ms, err := filterMods(st.Ix, "status:all "+expr)
		if err != nil {
			return nil, err
		}
		keep = map[string]bool{}
		for _, m := range ms {
			keep[m.ID] = true
		}
	}
	d, root := render.New(workspaceLine(st))
	var rows [][]string
	for i, e := range st.List {
		pos := fmt.Sprint(i + 1)
		if e.Instance == "" {
			if keep == nil {
				rows = append(rows, []string{pos, "pkg:" + e.Missing, "(missing: no installed mod matches this entry)"})
			}
			continue
		}
		if keep != nil && !keep[e.Instance] {
			continue
		}
		m := st.Ix.ByID[e.Instance]
		if m == nil {
			rows = append(rows, []string{pos, st.Ix.Ref(e.Instance), "(gone)"})
			continue
		}
		rows = append(rows, []string{pos, st.Ix.Ref(m.ID), m.DisplayName(), tags(c, st.Ix, m)})
	}
	limit := render.DefaultLimit
	if a.Bool("all") {
		limit = 0
	}
	shown := rows
	if limit > 0 && len(rows) > limit {
		shown = rows[:limit]
	}
	root.AddColumns(shown)
	if len(shown) < len(rows) {
		root.Addf("… %d more (archo list --active --all)", len(rows)-len(shown))
	}
	if len(st.Changes()) > 0 {
		d.AddNext("archo loadout diff", "archo save")
	}
	return d, nil
}

// loadoutStatus adds the workspace lines to status (04 §2.2, §2.3).
func loadoutStatus(c *Ctx, root *render.Node, d *render.Doc) {
	st := c.Loadout()
	if st == nil {
		if c.stErr != nil {
			root.KV("loadout", "unavailable: "+c.stErr.Error())
		}
		return
	}
	missing := 0
	for _, e := range st.List {
		if e.Instance == "" {
			missing++
		}
	}
	line := fmt.Sprintf("%d active", len(st.List))
	if missing > 0 {
		line += fmt.Sprintf(" (%d missing entries)", missing)
	}
	if n := len(st.Changes()); n > 0 {
		line += fmt.Sprintf(", %s unsaved (archo loadout diff)", render.Plural(n, "change"))
		d.AddNext("archo save")
	} else if st.Latest != nil {
		line += fmt.Sprintf(", same as v%d (%s)", st.Latest.N, st.Latest.Source)
	}
	root.KV("loadout", line)
	if run, _ := c.App.GameRunning(); run != nil {
		what := "the list"
		if w := c.App.WhatRuns(st.Ix.Env, run); w.Pack != "" {
			what = "pack " + w.Pack
			if w.Guess {
				what += " (from the launch records)"
			}
		}
		root.KV("game", fmt.Sprintf("running %s for %s (pid %d; archo log --follow)", what, c.App.Now().Sub(run.Started).Round(time.Minute), run.PID))
	}
	for _, n := range st.Notices {
		root.KV("notice", n)
	}
	if st.WS.Conflict != 0 {
		root.KV("pending", fmt.Sprintf("ModsConfig.xml changed outside RimArcho (v%d) while the workspace had unsaved edits: archo loadout diff v%d, archo loadout checkout v%d", st.WS.Conflict, st.WS.Conflict, st.WS.Conflict))
	}
	if v := st.FileVersionMismatch(); v != "" {
		root.KV("warning", fmt.Sprintf("ModsConfig.xml was saved for %s; the game (%s) resets it on start. Saving once rewrites it for %s (archo save)",
			env.MajorMinor(v), st.GameMM(), st.GameMM()))
	}
	if st.FileErr != nil {
		root.KV("warning", "ModsConfig.xml unreadable: "+st.FileErr.Error())
	}
}
