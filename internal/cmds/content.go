package cmds

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/rimarcho/rimarcho/internal/analyze"
	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/content"
	"github.com/rimarcho/rimarcho/internal/finding"
	"github.com/rimarcho/rimarcho/internal/order"
	"github.com/rimarcho/rimarcho/internal/render"
	"github.com/rimarcho/rimarcho/internal/scan"
	"github.com/rimarcho/rimarcho/internal/store"
)

func init() {
	register(&Command{Name: "def", Args: "[<Type/defName>]", Summary: "who defines and patches a Def; or the Defs of a mod", MaxArgs: 1, Run: defCmd,
		Flags: []Flag{{Name: "mod", Value: "<ref>"}, {Name: "all"}, gameVersionFlag}})
	register(&Command{Name: "patches", Summary: "Patch operations and their simulated outcome", MaxArgs: 0, Run: patchesCmd,
		Flags: []Flag{{Name: "mod", Value: "<ref>"}, {Name: "target", Value: "<xpath part | Type/defName>"}, {Name: "failed"}, {Name: "all"}, gameVersionFlag}})
	register(&Command{Name: "why", Args: "<A> <B>", Summary: "how two mods relate: order, dependencies, conflicts, overrides, shared patches", MinArgs: 2, MaxArgs: 2, Run: whyCmd, Flags: []Flag{gameVersionFlag}})
	register(&Command{Name: "files search", Args: "<pattern>", Summary: "search the effective content of enabled mods", MinArgs: 1, MaxArgs: 1, Run: filesSearchCmd,
		Flags: []Flag{{Name: "mod", Value: "<ref>"}, {Name: "regex"}, {Name: "languages", Help: "also search Languages/ (translations; excluded by default)"}, {Name: "all"}}})
}

// analysed brings the analysis up to date and returns the run (an error if none exists).
func analysed(c *Ctx) (*app.LoadoutState, *app.AnalysisState, error) {
	s, err := c.App.EnsureAnalysis(progress())
	if err != nil && (s == nil || s.Run == nil) {
		return nil, nil, err
	}
	st, err2 := c.App.Loadout()
	if err2 != nil {
		return nil, nil, err2
	}
	if s == nil || s.Run == nil {
		return nil, nil, fmt.Errorf("no analysis available (archo analyze)")
	}
	return st, s, nil
}

func modLabel(ix *app.Index, id string) string {
	m := ix.ByID[id]
	if m == nil {
		return "#?"
	}
	return ix.Ref(id) + " " + m.DisplayName()
}

// parseDefRef accepts "def:ThingDef/Gun_A", "ThingDef/Gun_A", "ThingDef@BaseGun" or "Gun_A".
func parseDefRef(s string) (typ, name string, byName bool) {
	s = strings.TrimPrefix(s, "def:")
	if i := strings.IndexByte(s, '@'); i >= 0 {
		return s[:i], s[i+1:], true
	}
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return s[:i], s[i+1:], false
	}
	return "", s, false
}

func defCmd(c *Ctx, a *Args) (*render.Doc, error) {
	if a.Str("mod") != "" {
		return defsOfModCmd(c, a)
	}
	if len(a.Pos) == 0 {
		return nil, &UsageError{Msg: "give a def (Type/defName) or --mod <ref>"}
	}
	st, s, err := analysed(c)
	if err != nil {
		return nil, err
	}
	ix := st.Ix
	typ, name, byName := parseDefRef(a.Pos[0])
	facts, err := c.App.Store.DefsNamed(name, byName)
	if err != nil {
		return nil, err
	}
	type loc struct {
		f   analyze.DefFact
		pos int
	}
	var active, inactive []loc
	types := map[string]bool{}
	for _, f := range facts {
		if typ != "" && f.Type != typ {
			continue
		}
		if ix.ByID[f.Instance] == nil {
			continue
		}
		types[f.Type] = true
		if p := st.Position(f.Instance); p > 0 {
			active = append(active, loc{f, p})
		} else {
			inactive = append(inactive, loc{f, 0})
		}
	}
	sort.SliceStable(active, func(i, j int) bool { return active[i].pos < active[j].pos })
	sep := "/"
	if byName {
		sep = "@"
	}
	if typ == "" && len(types) == 1 {
		for t := range types {
			typ = t
		}
	}
	subject := "def:" + typ + sep + name
	if typ == "" {
		subject = "def:*" + sep + name
	}
	d, root := render.New(subject)
	if len(active)+len(inactive) == 0 {
		root.Add("not defined by any installed mod")
	}
	if len(active) > 0 {
		sec := root.Section("defined-by (load order)")
		for i, l := range active {
			line := fmt.Sprintf("%d. %s  %s:%d", i+1, modLabel(ix, l.f.Instance), l.f.File, l.f.Line)
			if typ == "" {
				line += "  " + l.f.Type
			}
			if l.f.Parent != "" {
				line += "  parent " + l.f.Parent
			}
			if l.f.Abstract {
				line += "  abstract"
			}
			if i == len(active)-1 && !byName {
				line += "  ← effective"
			}
			sec.Add(line)
		}
	}
	if len(inactive) > 0 {
		sec := root.Section("also-in (not enabled)")
		for _, l := range inactive {
			sec.Addf("%s  %s:%d", modLabel(ix, l.f.Instance), l.f.File, l.f.Line)
		}
		sec.Truncate(10, "")
	}
	target := typ + sep + name
	pf := store.PatchFilter{Target: target}
	if typ == "" {
		pf = store.PatchFilter{XPathPart: `"` + name + `"`}
	}
	pfs, err := c.App.Store.PatchFacts(s.Run.ID, pf)
	if err != nil {
		return nil, err
	}
	if len(pfs) > 0 {
		sec := root.Section("patched-by")
		for _, p := range pfs {
			sec.Add(patchLine(ix, p, true))
		}
		sec.Truncate(boolLimit(a, 20), "archo patches --target "+target+" --all")
	}
	if !s.Fresh {
		root.KV("analysis", "stale (archo analyze)")
	}
	return d, nil
}

func boolLimit(a *Args, n int) int {
	if a.Bool("all") {
		return 0
	}
	return n
}

func patchLine(ix *app.Index, p *analyze.PatchFact, withMod bool) string {
	l := fmt.Sprintf("%s:%d  %s", p.File, p.Line, p.Desc)
	if withMod {
		l = modLabel(ix, p.Instance) + "  " + l
	}
	switch p.Result {
	case analyze.ResultFailed:
		l += fmt.Sprintf("  hit %d  (patch.no-match: %s)", p.Hits, strings.Join(p.Reasons, ", "))
	case analyze.ResultUnknown:
		l += "  (mod-defined class: not simulated)"
	default:
		l += fmt.Sprintf("  hit %d", p.Hits)
	}
	return l
}

func defsOfModCmd(c *Ctx, a *Args) (*render.Doc, error) {
	st, _, err := analysed(c)
	if err != nil {
		return nil, err
	}
	m, err := st.Ix.ResolveOne(a.Str("mod"))
	if err != nil {
		return nil, err
	}
	facts, err := c.App.Store.DefsOf(m.ID)
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("%s  defs: %d", modLabel(st.Ix, m.ID), len(facts)))
	byType := map[string][]analyze.DefFact{}
	var types []string
	for _, f := range facts {
		if byType[f.Type] == nil {
			types = append(types, f.Type)
		}
		byType[f.Type] = append(byType[f.Type], f)
	}
	sort.Strings(types)
	for _, t := range types {
		sec := root.Section(fmt.Sprintf("%s (%d)", t, len(byType[t])))
		for _, f := range byType[t] {
			id := f.DefName
			if id == "" {
				id = "@" + f.Name
			}
			sec.Addf("%s  %s:%d", id, f.File, f.Line)
		}
		sec.Truncate(boolLimit(a, 5), "archo def --mod "+a.Str("mod")+" --all")
	}
	root.Truncate(boolLimit(a, 25), "archo def --mod "+a.Str("mod")+" --all")
	return d, nil
}

func patchesCmd(c *Ctx, a *Args) (*render.Doc, error) {
	st, s, err := analysed(c)
	if err != nil {
		return nil, err
	}
	ix := st.Ix
	f := store.PatchFilter{FailedOnly: a.Bool("failed")}
	if r := a.Str("mod"); r != "" {
		m, err := ix.ResolveOne(r)
		if err != nil {
			return nil, err
		}
		f.Instance = m.ID
	}
	if t := a.Str("target"); t != "" {
		if typ, name, byName := parseDefRef(t); typ != "" && !strings.ContainsAny(name, "/[") {
			sep := "/"
			if byName {
				sep = "@"
			}
			f.Target = typ + sep + name
		} else {
			f.XPathPart = strings.TrimPrefix(t, "def:")
		}
	}
	pfs, err := c.App.Store.PatchFacts(s.Run.ID, f)
	if err != nil {
		return nil, err
	}
	failed, unknown := 0, 0
	for _, p := range pfs {
		switch p.Result {
		case analyze.ResultFailed:
			failed++
		case analyze.ResultUnknown:
			unknown++
		}
	}
	d, root := render.New(fmt.Sprintf("patches: %d operations, %d failed, %d not simulated", len(pfs), failed, unknown))
	if !s.Fresh {
		root.KV("analysis", "stale (archo analyze)")
	}
	lst := root.Section("list")
	if len(pfs) == 0 {
		lst.Add("(none)")
	}
	for _, p := range pfs {
		n := lst.Add(patchLine(ix, p, f.Instance == ""))
		if p.Result == analyze.ResultFailed && p.Detail != "" && (a.Bool("failed") || len(pfs) <= 10) {
			n.Add("detail: " + expand(ix, p.Detail, ""))
		}
	}
	lst.Truncate(boolLimit(a, render.DefaultLimit), "add --all")
	if failed > 0 && !a.Bool("failed") {
		d.AddNext("archo patches --failed")
	}
	return d, nil
}

func whyCmd(c *Ctx, a *Args) (*render.Doc, error) {
	st, err := c.App.Loadout()
	if err != nil {
		return nil, err
	}
	ix := st.Ix
	ma, err := ix.ResolveOne(a.Pos[0])
	if err != nil {
		return nil, err
	}
	mb, err := ix.ResolveOne(a.Pos[1])
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("%s ↔ %s", modLabel(ix, ma.ID), modLabel(ix, mb.ID)))
	pa, pb := st.Position(ma.ID), st.Position(mb.ID)
	posText := func(p int) string {
		if p == 0 {
			return "not enabled"
		}
		return fmt.Sprintf("enabled at %d", p)
	}
	root.KV("positions", posText(pa)+" / "+posText(pb))
	if ma.LineageID == mb.LineageID {
		root.KV("lineage", "same lineage "+ix.LRef(ma.LineageID)+" (copies of one mod)")
	}
	if err := c.App.Store.LoadDecls([]*store.Mod{ma, mb}); err != nil {
		return nil, err
	}
	mm := st.GameMM()
	rel := root.Section("declared")
	declRel := func(x, y *store.Mod) {
		dx := x.Decl()
		pkgY := strings.TrimSuffix(strings.ToLower(y.PackageID), "_steam")
		for _, dep := range append(append([]string{}, depPkgs(dx.Dependencies)...), depPkgs(dx.DependenciesByVersion["v"+mm])...) {
			if dep == pkgY {
				rel.Addf("%s depends on %s (About.xml modDependencies)", ix.Ref(x.ID), ix.Ref(y.ID))
			}
		}
		for _, f := range []struct {
			name string
			l    []string
		}{
			{"loadAfter", forVersion(dx.LoadAfter, dx.LoadAfterByVersion, mm)}, {"loadBefore", forVersion(dx.LoadBefore, dx.LoadBeforeByVersion, mm)},
			{"forceLoadAfter", dx.ForceLoadAfter}, {"forceLoadBefore", dx.ForceLoadBefore},
			{"incompatibleWith", forVersion(dx.IncompatibleWith, dx.IncompatibleByVersion, mm)},
		} {
			for _, p := range f.l {
				if strings.EqualFold(strings.TrimSpace(p), pkgY) {
					rel.Addf("%s %s %s (About.xml)", ix.Ref(x.ID), f.name, ix.Ref(y.ID))
				}
			}
		}
	}
	declRel(ma, mb)
	declRel(mb, ma)
	if len(rel.Children) == 0 {
		rel.Add("(nothing between them)")
	}
	// Sort constraints between the two, as the sorter sees them (community and user rules too).
	if pa > 0 && pb > 0 {
		in, err := c.App.SortInput(st, st.List)
		if err != nil {
			return nil, err
		}
		r := order.Sort(in)
		cs := root.Section("order-constraints")
		for _, e := range r.Edges {
			if (e.From == pa-1 && e.To == pb-1) || (e.From == pb-1 && e.To == pa-1) {
				ev, _ := constraintText(st, in, e)
				state := "satisfied"
				if e.From > e.To {
					state = "VIOLATED"
				}
				cs.Addf("%s before %s: %s  (%s)", ix.Ref(in.Nodes[e.From].Instance), ix.Ref(in.Nodes[e.To].Instance), ev, state)
			}
		}
		if len(cs.Children) == 0 {
			cs.Add("(none)")
		}
	}
	// Content: shared defs and patches of one onto the other's defs.
	if s, err := c.App.Analysis(st); err == nil && s.Run != nil {
		da, _ := c.App.Store.DefsOf(ma.ID)
		db, _ := c.App.Store.DefsOf(mb.ID)
		inB := map[string]bool{}
		for _, f := range db {
			if f.DefName != "" {
				inB[f.Type+"/"+f.DefName] = true
			}
		}
		var shared []string
		for _, f := range da {
			if k := f.Type + "/" + f.DefName; f.DefName != "" && inB[k] {
				shared = append(shared, k)
			}
		}
		if len(shared) > 0 {
			sec := root.Section(fmt.Sprintf("both-define (%d defs; the later one wins)", len(shared)))
			for _, k := range shared {
				sec.Add("def:" + k)
			}
			sec.Truncate(8, "")
		}
		onto := func(x *store.Mod, ydefs []analyze.DefFact) {
			own := map[string]bool{}
			for _, f := range ydefs {
				if f.DefName != "" {
					own[f.Type+"/"+f.DefName] = true
				}
				if f.Name != "" {
					own[f.Type+"@"+f.Name] = true
				}
			}
			pfs, _ := c.App.Store.PatchFacts(s.Run.ID, store.PatchFilter{Instance: x.ID})
			var lines []string
			for _, p := range pfs {
				for _, t := range p.Targets {
					if own[t] {
						lines = append(lines, patchLine(ix, p, false))
						break
					}
				}
			}
			if len(lines) > 0 {
				other := mb
				if x == mb {
					other = ma
				}
				sec := root.Section(fmt.Sprintf("%s patches defs of %s", ix.Ref(x.ID), ix.Ref(other.ID)))
				for _, l := range lines {
					sec.Add(l)
				}
				sec.Truncate(8, "archo patches --mod "+ix.Ref(x.ID))
			}
		}
		onto(ma, db)
		onto(mb, da)
		// Methods both mods patch with Harmony (05 §3: 共同补丁).
		ha, _ := c.App.Store.HarmonyPatches(store.HarmonyFilter{Instance: ma.ID})
		hb, _ := c.App.Store.HarmonyPatches(store.HarmonyFilter{Instance: mb.ID})
		kinds := map[string][2][]string{}
		for i, rows := range [][]*store.HarmonyRow{ha, hb} {
			for _, r := range rows {
				k := kinds[r.Target]
				if !slices.Contains(k[i], r.Kind) {
					k[i] = append(k[i], r.Kind)
				}
				kinds[r.Target] = k
			}
		}
		var hshared []string
		for t, k := range kinds {
			if len(k[0]) > 0 && len(k[1]) > 0 {
				hshared = append(hshared, fmt.Sprintf("%s  %s / %s", t, strings.Join(k[0], "+"), strings.Join(k[1], "+")))
			}
		}
		if len(hshared) > 0 {
			sort.Strings(hshared)
			sec := root.Section("shared-harmony-targets")
			for _, l := range hshared {
				sec.Add(l)
			}
			sec.Truncate(8, "archo harmony --mod "+ix.Ref(ma.ID)+" --shared")
		}
		if fs, err := c.App.Store.Findings(s.Run.ID); err == nil {
			ta, tb := finding.Inst(ma.ID), finding.Inst(mb.ID)
			var both []*finding.Finding
			for _, f := range fs {
				if contains(f.Subjects, ta) && contains(f.Subjects, tb) {
					both = append(both, f)
				}
			}
			if len(both) > 0 {
				sec := root.Section("shared-problems")
				for _, f := range both {
					sec.Addf("%s %s  %s", f.Severity, f.Code, expand(ix, f.Claim, ""))
				}
				sec.Truncate(8, fmt.Sprintf("archo problems --subject %s", ix.Ref(ma.ID)))
			}
		}
		if !s.Fresh {
			root.KV("analysis", "stale (archo analyze)")
		}
	} else {
		root.KV("content", "not analysed (archo analyze)")
	}
	return d, nil
}

func depPkgs(l []scan.Dependency) []string {
	out := make([]string, 0, len(l))
	for _, d := range l {
		out = append(out, strings.TrimSuffix(strings.ToLower(strings.TrimSpace(d.PackageID)), "_steam"))
	}
	return out
}

func filesSearchCmd(c *Ctx, a *Args) (*render.Doc, error) {
	st, err := c.App.Loadout()
	if err != nil {
		return nil, err
	}
	ix := st.Ix
	var mods []*store.Mod
	if r := a.Str("mod"); r != "" {
		m, err := ix.ResolveOne(r)
		if err != nil {
			return nil, err
		}
		mods = []*store.Mod{m}
	} else {
		for _, e := range st.List {
			if m := ix.ByID[e.Instance]; m != nil {
				mods = append(mods, m)
			}
		}
	}
	if err := c.App.Store.LoadDecls(mods); err != nil {
		return nil, err
	}
	activePkg := map[string]bool{}
	for _, e := range st.List {
		if m := ix.ByID[e.Instance]; m != nil {
			activePkg[strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam")] = true
		}
	}
	pat := a.Pos[0]
	var re *regexp.Regexp
	if a.Bool("regex") {
		re, err = regexp.Compile("(?i)" + pat)
		if err != nil {
			return nil, &UsageError{Msg: "bad --regex pattern: " + err.Error()}
		}
	}
	lower := strings.ToLower(pat)
	match := func(line string) bool {
		if re != nil {
			return re.MatchString(line)
		}
		return strings.Contains(strings.ToLower(line), lower)
	}
	limit := boolLimit(a, render.DefaultLimit)
	langs := a.Bool("languages")
	var hits []string
	files, total := 0, 0
	// Walk the mods in parallel; each keeps its own file list, joined in mod order.
	perMod := make([][]fileJob, len(mods))
	var wwg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for mi, m := range mods {
		wwg.Add(1)
		sem <- struct{}{}
		go func(mi int, m *store.Mod) {
			defer func() { <-sem; wwg.Done() }()
			perMod[mi] = walkMod(ix, st, m, activePkg, langs)
		}(mi, m)
	}
	wwg.Wait()
	var jobs []fileJob
	for _, l := range perMod {
		jobs = append(jobs, l...)
	}
	files = len(jobs)
	// Read and match in parallel (the walk is cheap; reading 4–5k files is not), then
	// report in walk order.
	type result struct {
		n     int
		lines []string
	}
	results := make([]result, len(jobs))
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				j := jobs[i]
				b, err := os.ReadFile(j.path)
				if err != nil || (re == nil && !bytes.Contains(bytes.ToLower(b), []byte(lower))) {
					continue
				}
				sc := bufio.NewScanner(bytes.NewReader(b))
				sc.Buffer(make([]byte, 64<<10), 4<<20)
				n := 0
				for sc.Scan() {
					n++
					if line := sc.Text(); match(line) {
						results[i].n++
						if limit == 0 || len(results[i].lines) < limit {
							relMod, _ := filepath.Rel(j.dir, j.path)
							snippet := strings.TrimSpace(line)
							if len(snippet) > 120 {
								snippet = snippet[:117] + "..."
							}
							results[i].lines = append(results[i].lines, fmt.Sprintf("%s  %s:%d  %s", j.ref, filepath.ToSlash(relMod), n, snippet))
						}
					}
				}
			}
		}()
	}
	for i := range jobs {
		next <- i
	}
	close(next)
	wg.Wait()
	for _, r := range results {
		total += r.n
		for _, l := range r.lines {
			if limit == 0 || len(hits) < limit {
				hits = append(hits, l)
			}
		}
	}
	d, root := render.New(fmt.Sprintf("files search %q: %d matches in %d files of %d mods", pat, total, files, len(mods)))
	lst := root.Section("matches")
	for _, h := range hits {
		lst.Add(h)
	}
	if len(hits) == 0 {
		lst.Add("(none)")
	}
	if total > len(hits) {
		lst.Addf("… %d more (add --all)", total-len(hits))
	}
	return d, nil
}

var versionDirRe = regexp.MustCompile(`^[vV]?\d+\.\d+$`)

func isVersionDir(s string) bool { return versionDirRe.MatchString(s) }

type fileJob struct{ ref, dir, path string }

// walkMod lists the effective .xml/.txt files of one mod (higher-priority load folders
// shadow lower ones; Languages/ only with langs).
func walkMod(ix *app.Index, st *app.LoadoutState, m *store.Mod, activePkg map[string]bool, langs bool) []fileJob {
	dir := ix.ModDir(m)
	r := content.Resolve(dir, m.Decl().LoadFolders, st.GameMM(), func(p string) bool { return activePkg[strings.TrimSuffix(strings.ToLower(p), "_steam")] })
	seen := map[string]bool{}
	ref := ix.Ref(m.ID)
	var out []fileJob
	for _, folder := range r.Folders {
		filepath.WalkDir(folder, func(p string, de os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if de.IsDir() {
				// Translations repeat every key and drown the Defs/Patches evidence.
				if strings.EqualFold(de.Name(), "Languages") && !langs {
					return filepath.SkipDir
				}
				// The mod root is a load folder too; its version folders are handled by Resolve.
				if p != folder && folder == dir && (isVersionDir(de.Name()) || strings.EqualFold(de.Name(), "Common")) {
					return filepath.SkipDir
				}
				return nil
			}
			ext := strings.ToLower(filepath.Ext(p))
			if ext != ".xml" && ext != ".txt" {
				return nil
			}
			rel, _ := filepath.Rel(folder, p)
			key := strings.ToLower(rel)
			if seen[key] {
				return nil // overridden by a higher-priority load folder
			}
			seen[key] = true
			out = append(out, fileJob{ref: ref, dir: dir, path: p})
			return nil
		})
	}
	return out
}
