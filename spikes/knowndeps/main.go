// Spike knowndeps — observed prerequisites and known-bad combinations from the log corpus
// (docs/research/00-日志语料顶层设计.md §3, G10). Output: internal/known/known.json
// (aggregates only: packageIds, counts, templates; every entry from >= -minplayers players).
//
// Inputs:
//   - hugslogs parsed.jsonl: the mod list (with versions) and upload time of each log;
//   - corpusrun results (v4): clusters with RimArcho's attribution. A "dependency" role is an
//     installed-but-disabled mod that provides something an error says is missing (a type,
//     an abstract parent, a def, a texture);
//   - the research mod library (folders with About/About.xml): declared dependencies, and the
//     files a candidate mod uses;
//   - steamDB.json (Workshop "required items"), only to measure overlap;
//   - spikes/patterns/patterns-v0.json (G5), resolution evidence of mod-specific patterns.
//
// Method, prerequisites. For every missing item (provider B, what is missing) the mods enabled
// in >= 90% of the logs with that error are candidates; a candidate A is kept when RimArcho
// named it as the error's source, or A's own XML references the item without defining it.
// Per pair (A, B): logs with A enabled, B not, and such an error; different players (mod
// lists grouped at Jaccard >= 0.6); the rate among all logs with A enabled and B not.
//
// Method, combinations. Templates attributed to exactly one source mod A. A pair (A, B): B is
// enabled in >= 90% of the logs with the error, the error is in >= 30% of the logs with both,
// and at most a quarter of that rate in logs with A but not B (>= 30 such logs). A version: the
// same against A's other versions. A single mod: the error is in >= 50% of the logs that
// enable A (>= 20 logs) — a known error of that mod.
//
// Resolution: consecutive uploads of one player (Jaccard >= 0.9, <= 48 h, as G5). For each
// entry: uploads after one with the error, how many no longer had it, and what changed.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/bits"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/ToumanLin/RimArcho/internal/known"
	"github.com/ToumanLin/RimArcho/internal/scan"
)

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

var (
	minPlayers = flag.Int("minplayers", 5, "every entry: logs from at least this many players (design §8)")
	minRate    = flag.Float64("minrate", 0.25, "prerequisite: the error in at least this share of logs with A enabled and B not")
	cover      = flag.Float64("cover", 0.9, "candidate: enabled in at least this share of the logs with the error")
)

func main() {
	parsed := flag.String("parsed", "", "hugslogs parsed.jsonl")
	results := flag.String("results", "", "corpusrun results JSONL (v4)")
	patterns := flag.String("patterns", "spikes/patterns/patterns-v0.json", "G5 pattern dataset")
	steamdb := flag.String("steamdb", "", "RimSort steamDB.json (overlap only)")
	out := flag.String("out", "", "dataset to write (internal/known/known.json)")
	mm := flag.String("mm", "1.6", "game major.minor")
	var roots multi
	flag.Var(&roots, "mods", "mod root (folders with About/About.xml); repeatable")
	home := flag.String("verify", "", "verify mode: RIMARCHO_HOME of a corpusrun research environment")
	userData := flag.String("userdata", "", "verify mode: its user data root")
	perKind := flag.Int("lists", 20, "verify mode: mod lists per finding code")
	flag.Parse()
	if *home != "" {
		if err := verify(*home, *userData, *parsed, *mm, *perKind); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(*parsed, *results, *patterns, *steamdb, *out, *mm, roots); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func norm(p string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(p)), "_steam")
}

func official(p string) bool { return strings.HasPrefix(p, "ludeon.rimworld") }

type item struct{ kind, name string }

func (it item) String() string { return it.kind + " " + it.name }

type depHit struct {
	b     string
	it    item
	tpl   string
	class string
	srcs  []string
}

type srcHit struct {
	a, tpl, class string
	others        map[string]bool // other mods the cluster names (patcher, mentioned, dependency)
}

type logRec struct {
	id      string
	created time.Time
	set     map[string]bool
	ver     map[string]string
	version string   // game version the log was written by
	order   []string // the mod list as written (verify)
	deps    []depHit
	srcs    []srcHit
}

// ---- input ----

func eachLine(path string, f func([]byte) error) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	r := bufio.NewReaderSize(fh, 1<<20)
	for {
		b, err := r.ReadBytes('\n')
		if len(b) > 0 {
			if e := f(b); e != nil {
				return e
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

type pmod struct {
	Pkg string `json:"pkg"`
	MV  string `json:"mv"`
	V   string `json:"v"`
	OV  string `json:"ov"`
}

func (m pmod) version() string {
	switch {
	case m.OV != "":
		return m.OV
	case m.V != "":
		return m.V
	}
	return m.MV
}

var (
	reType     = regexp.MustCompile(`^provides type (\S+) but`)
	reAbstract = regexp.MustCompile(`^provides the abstract def (\S+) `)
	reDef      = regexp.MustCompile(`^defines (\w+) (\S+) at`)
	reFile     = regexp.MustCompile(`^ships (\S+) but`)
	// Types every mod gets from the game or from bundled libraries: they say nothing about B.
	reGeneric = regexp.MustCompile(`^(HarmonyLib|Harmony|System|Mono|UnityEngine|Verse|RimWorld|Microsoft|Unity)\.`)
)

func itemOf(ev string) (item, bool) {
	if m := reType.FindStringSubmatch(ev); m != nil {
		return item{"type", m[1]}, !reGeneric.MatchString(m[1])
	}
	if m := reAbstract.FindStringSubmatch(ev); m != nil {
		return item{"abstract", m[1]}, true
	}
	if m := reDef.FindStringSubmatch(ev); m != nil {
		return item{"def", m[1] + " " + m[2]}, true
	}
	if m := reFile.FindStringSubmatch(ev); m != nil {
		return item{"file", m[1]}, true
	}
	return item{}, false
}

func readCorpus(parsedPath, resultsPath, mm string) ([]*logRec, error) {
	byID := map[string]*logRec{}
	var ls []*logRec
	err := eachLine(parsedPath, func(b []byte) error {
		var r struct {
			ID      string `json:"id"`
			Created string `json:"created"`
			Status  string `json:"status"`
			MM      string `json:"mm"`
			Version string `json:"version"`
			Mods    []pmod `json:"mods"`
		}
		if json.Unmarshal(b, &r) != nil || r.Status != "ok" || r.MM != mm {
			return nil
		}
		t, _ := time.Parse(time.RFC3339, r.Created)
		l := &logRec{id: r.ID, created: t, set: map[string]bool{}, ver: map[string]string{}, version: r.Version}
		for _, m := range r.Mods {
			l.order = append(l.order, strings.ToLower(m.Pkg))
			p := norm(m.Pkg)
			l.set[p] = true
			l.ver[p] = m.version()
		}
		byID[r.ID] = l
		ls = append(ls, l)
		return nil
	})
	if err != nil || resultsPath == "" {
		return ls, err
	}
	type attr struct {
		Pkg  string   `json:"pkg"`
		Role string   `json:"role"`
		Ev   []string `json:"ev"`
	}
	type cluster struct {
		Tpl     string   `json:"tpl"`
		Class   string   `json:"class"`
		Attr    []attr   `json:"attr"`
		Details []string `json:"details"`
	}
	err = eachLine(resultsPath, func(b []byte) error {
		var r struct {
			ID       string    `json:"id"`
			Clusters []cluster `json:"clusters"`
		}
		if json.Unmarshal(b, &r) != nil {
			return nil
		}
		l := byID[r.ID]
		if l == nil {
			return nil
		}
		for _, c := range r.Clusters {
			if len(c.Attr) == 0 || c.Class == "save-load" {
				continue
			}
			save := false
			for _, d := range c.Details {
				save = save || strings.Contains(d, "raised while loading a save")
			}
			if save {
				continue // the save names a def, not a mod that needs it
			}
			var srcs []string
			others := map[string]bool{}
			for _, a := range c.Attr {
				if a.Role == "source" {
					srcs = append(srcs, norm(a.Pkg))
				} else {
					others[norm(a.Pkg)] = true
				}
			}
			for _, a := range c.Attr {
				if a.Role != "dependency" {
					continue
				}
				bp := norm(a.Pkg)
				if l.set[bp] {
					continue
				}
				it, ok := item{"error", c.Tpl}, true
				if len(a.Ev) > 0 {
					if x, good := itemOf(a.Ev[0]); x.kind != "" {
						it, ok = x, good
					}
				}
				if ok {
					l.deps = append(l.deps, depHit{b: bp, it: it, tpl: c.Tpl, class: c.Class, srcs: srcs})
				}
			}
			if len(srcs) == 1 && !official(srcs[0]) && l.set[srcs[0]] {
				l.srcs = append(l.srcs, srcHit{a: srcs[0], tpl: c.Tpl, class: c.Class, others: others})
			}
		}
		return nil
	})
	sort.SliceStable(ls, func(a, b int) bool { return ls[a].created.Before(ls[b].created) })
	return ls, err
}

// ---- mod library ----

type modInfo struct {
	dirs      []string
	deps      map[string]bool
	loadAfter map[string]bool
}

func readLibrary(roots []string, mm string) map[string]*modInfo {
	lib := map[string]*modInfo{}
	for _, r := range roots {
		ents, err := os.ReadDir(r)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(r, e.Name())
			d := scan.ReadDecl(dir, false)
			p := norm(d.PackageIDRaw)
			if p == "" {
				continue
			}
			m := lib[p]
			if m == nil {
				m = &modInfo{deps: map[string]bool{}, loadAfter: map[string]bool{}}
				lib[p] = m
			}
			m.dirs = append(m.dirs, dir)
			for _, x := range append(append([]scan.Dependency{}, d.Dependencies...), d.DependenciesByVersion["v"+mm]...) {
				m.deps[norm(x.PackageID)] = true
			}
			for _, x := range append(append(append([]string{}, d.LoadAfter...), d.ForceLoadAfter...), d.LoadAfterByVersion["v"+mm]...) {
				m.loadAfter[norm(x)] = true
			}
		}
	}
	return lib
}

// files: the XML and DLL bytes of a mod (textures, sounds and translations skipped).
type files struct{ xml, dll [][]byte }

type fileCache struct {
	lib   map[string]*modInfo
	m     map[string]*files
	order []string
}

func (c *fileCache) get(p string) *files {
	if f := c.m[p]; f != nil {
		return f
	}
	f := &files{}
	if mi := c.lib[p]; mi != nil {
		for _, d := range mi.dirs {
			filepath.WalkDir(d, func(path string, e os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				low := strings.ToLower(e.Name())
				if e.IsDir() {
					if low == "textures" || low == "sounds" || low == "languages" || low == ".git" {
						return filepath.SkipDir
					}
					return nil
				}
				switch {
				case strings.HasSuffix(low, ".xml"):
					if b, err := os.ReadFile(path); err == nil {
						f.xml = append(f.xml, b)
					}
				case strings.HasSuffix(low, ".dll"):
					if b, err := os.ReadFile(path); err == nil {
						f.dll = append(f.dll, b)
					}
				}
				return nil
			})
		}
	}
	c.m[p] = f
	c.order = append(c.order, p)
	if len(c.order) > 300 {
		delete(c.m, c.order[0])
		c.order = c.order[1:]
	}
	return f
}

func anyContains(bs [][]byte, subs ...[]byte) bool {
	for _, b := range bs {
		for _, s := range subs {
			if bytes.Contains(b, s) {
				return true
			}
		}
	}
	return false
}

func u16(s string) []byte {
	var out []byte
	for _, r := range utf16.Encode([]rune(s)) {
		out = append(out, byte(r), byte(r>>8))
	}
	return out
}

var reDirection = regexp.MustCompile(`_(north|south|east|west|side|front|back)$`)

// uses: A's own files reference the item and do not define it (a mod that defines the
// item itself is an alternative provider, not a dependent).
func uses(f *files, it item) bool {
	switch it.kind {
	case "type":
		return anyContains(f.xml, []byte(it.name))
	case "abstract":
		if anyContains(f.xml, []byte(`Name="`+it.name+`"`)) && !anyContains(f.xml, []byte(`ParentName="`+it.name+`"`)) {
			return false
		}
		return anyContains(f.xml, []byte(`ParentName="`+it.name+`"`))
	case "def":
		n := it.name[strings.IndexByte(it.name, ' ')+1:]
		if anyContains(f.xml, []byte("<defName>"+n+"</defName>")) {
			return false
		}
		return anyContains(f.xml, []byte(">"+n+"<"), []byte(`"`+n+`"`)) || anyContains(f.dll, u16(n))
	case "file":
		p := strings.TrimPrefix(it.name, "Textures/")
		if i := strings.LastIndexByte(p, '.'); i > 0 {
			p = p[:i]
		}
		p = reDirection.ReplaceAllString(p, "")
		return anyContains(f.xml, []byte(p)) || anyContains(f.dll, u16(p))
	}
	return false
}

// ---- helpers ----

type set map[int]bool

func players(ls []*logRec, idx set) int {
	var ids []int
	for i := range idx {
		ids = append(ids, i)
	}
	sort.Ints(ids) // logs are in upload order
	var groups []map[string]bool
	for _, i := range ids {
		e := ls[i].set
		found := false
		for _, g := range groups {
			if jaccard(e, g) >= 0.6 {
				for p := range e {
					g[p] = true
				}
				found = true
				break
			}
		}
		if !found {
			g := map[string]bool{}
			for p := range e {
				g[p] = true
			}
			groups = append(groups, g)
		}
	}
	return len(groups)
}

func jaccard(a, b map[string]bool) float64 {
	in := 0
	for k := range a {
		if b[k] {
			in++
		}
	}
	u := len(a) + len(b) - in
	if u == 0 {
		return 0
	}
	return float64(in) / float64(u)
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}

var privateRe = regexp.MustCompile(`(?i)users[\\/]|/home/|appdata|steamapps|[a-z]:[\\/]|@[a-z0-9-]+\.[a-z]`)

func top(m map[string]int, n int) []string {
	type kv struct {
		k string
		v int
	}
	var s []kv
	for k, v := range m {
		s = append(s, kv{k, v})
	}
	sort.Slice(s, func(a, b int) bool { return s[a].v > s[b].v || s[a].v == s[b].v && s[a].k < s[b].k })
	var out []string
	for _, x := range s {
		if len(out) == n {
			break
		}
		out = append(out, x.k)
	}
	return out
}

// sessions links each upload to the previous upload of the same player.
func sessions(ls []*logRec) [][2]int {
	var links [][2]int
	for j := range ls {
		best, bestJ := -1, 0.0
		for i := j - 1; i >= 0; i-- {
			if ls[j].created.Sub(ls[i].created) > 48*time.Hour {
				break
			}
			if jac := jaccard(ls[i].set, ls[j].set); jac >= 0.9 && jac > bestJ {
				best, bestJ = i, jac
			}
		}
		if best >= 0 {
			links = append(links, [2]int{best, j})
		}
	}
	return links
}

// fix: of the links whose first upload is in errs, how many second uploads lost the error,
// and what changed about the entry's mods (label → whether that change happened).
func fix(ls []*logRec, links [][2]int, errs set, changes func(a, b *logRec) []string) *known.Fix {
	f := &known.Fix{Changes: map[string]int{}, Source: "sessions"}
	for _, l := range links {
		if !errs[l[0]] {
			continue
		}
		a, b := ls[l[0]], ls[l[1]]
		f.Pairs++
		ch := changes(a, b)
		if len(ch) == 0 {
			f.Same++
		}
		if errs[l[1]] {
			continue
		}
		f.Gone++
		if len(ch) == 0 {
			f.SameGone++
			ch = []string{"no change to these mods"}
		}
		for _, c := range ch {
			f.Changes[c]++
		}
	}
	if f.Pairs == 0 {
		return nil
	}
	if len(f.Changes) == 0 {
		f.Changes = nil
	}
	return f
}

func modChanges(mods []string) func(a, b *logRec) []string {
	return func(a, b *logRec) []string {
		var out []string
		for i, m := range mods {
			switch {
			case a.set[m] && !b.set[m]:
				out = append(out, "disabled "+m)
			case !a.set[m] && b.set[m]:
				out = append(out, "enabled "+m)
			case i == 0 && a.set[m] && a.ver[m] != b.ver[m]:
				out = append(out, "updated "+m)
			}
		}
		return out
	}
}

// ---- run ----

func run(parsedPath, resultsPath, patternsPath, steamPath, outPath, mm string, roots []string) error {
	p := func(f string, a ...any) { fmt.Printf(f+"\n", a...) }
	t0 := time.Now()
	ls, err := readCorpus(parsedPath, resultsPath, mm)
	if err != nil {
		return err
	}
	lib := readLibrary(roots, mm)
	fc := &fileCache{lib: lib, m: map[string]*files{}}
	wsDeps := readSteamDB(steamPath)
	links := sessions(ls)
	nA := map[string]int{}
	for _, l := range ls {
		for m := range l.set {
			nA[m]++
		}
	}
	// Per-mod bitsets of the logs that enable it: pair counts are an AND and a popcount.
	words := (len(ls) + 63) / 64
	logBits := map[string][]uint64{}
	for i, l := range ls {
		for m := range l.set {
			b := logBits[m]
			if b == nil {
				b = make([]uint64, words)
				logBits[m] = b
			}
			b[i/64] |= 1 << (i % 64)
		}
	}
	type pairKey struct{ a, b string }
	bothCache := map[pairKey]int{}
	both := func(a, b string) int {
		if n, ok := bothCache[pairKey{a, b}]; ok {
			return n
		}
		x, y := logBits[a], logBits[b]
		n := 0
		if x != nil && y != nil {
			for i := range x {
				n += bitsOn(x[i] & y[i])
			}
		}
		bothCache[pairKey{a, b}] = n
		return n
	}
	p("Spike knowndeps — observed prerequisites and known-bad combinations (G10)")
	p("logs %d (game %s), research library %d packages, %d consecutive-upload links; read in %s",
		len(ls), mm, len(lib), len(links), time.Since(t0).Round(time.Second))
	p("thresholds: players >= %d, candidate cover >= %.0f%%, prerequisite rate >= %.0f%%", *minPlayers, 100**cover, 100**minRate)

	data := &known.Data{
		Source: "RimArcho log corpus: 6,734 shared HugsLib logs of RimWorld 1.6 (2025-07..2026-10), attribution by RimArcho (final run v4). Aggregate counts only; every entry from the logs of at least 5 different players. Logs are shared because something went wrong: rates are of shared logs, not of all players.",
		Game:   mm, Logs: len(ls),
	}

	// ---- prerequisites ----
	type ik struct {
		b  string
		it item
	}
	itemLogs := map[ik]set{}
	itemTpl := map[ik]map[string]int{}
	itemClass := map[ik]map[string]int{}
	itemSrc := map[ik]map[string]int{}
	for i, l := range ls {
		for _, d := range l.deps {
			k := ik{d.b, d.it}
			if itemLogs[k] == nil {
				itemLogs[k], itemTpl[k], itemClass[k], itemSrc[k] = set{}, map[string]int{}, map[string]int{}, map[string]int{}
			}
			if !itemLogs[k][i] {
				for _, s := range d.srcs {
					itemSrc[k][s]++
				}
			}
			itemLogs[k][i] = true
			itemTpl[k][d.tpl]++
			itemClass[k][d.class]++
		}
	}
	type pk struct{ a, b string }
	type pitem struct {
		k   ik
		how string
	}
	pairItems := map[pk][]pitem{}
	nItems, nConf := 0, 0
	for k, idx := range itemLogs {
		if len(idx) < 3 {
			continue
		}
		nItems++
		cnt := map[string]int{}
		for i := range idx {
			for m := range ls[i].set {
				cnt[m]++
			}
		}
		conf := false
		for a, c := range cnt {
			if float64(c) < *cover*float64(len(idx)) || a == k.b || official(a) || a == "brrainz.harmony" {
				continue
			}
			how := ""
			switch {
			case itemSrc[k][a] > 0:
				how = "source"
			case k.it.kind != "error" && lib[a] != nil && uses(fc.get(a), k.it):
				how = "files"
			}
			if how != "" {
				pairItems[pk{a, k.b}] = append(pairItems[pk{a, k.b}], pitem{k, how})
				conf = true
			}
		}
		if conf {
			nConf++
		}
	}
	p("\n== Prerequisites\n\nmissing items (provider, what) in >= 3 logs: %d; with a confirmed dependent mod: %d; candidate pairs: %d", nItems, nConf, len(pairItems))
	type prow struct {
		pr                   *known.Prereq
		declared, wsDeclared bool
		rate                 float64
		errs                 set
	}
	var rows []*prow
	var passPlayers, passRate int
	for k, its := range pairItems {
		errs := set{}
		tpl, class := map[string]int{}, map[string]int{}
		var items []string
		cosmetic := true
		for _, x := range its {
			for i := range itemLogs[x.k] {
				if ls[i].set[k.a] {
					errs[i] = true
				}
			}
			for t, n := range itemTpl[x.k] {
				tpl[t] += n
			}
			for c, n := range itemClass[x.k] {
				class[c] += n
			}
			items = append(items, x.k.it.String())
			if !(x.k.it.kind == "file" || x.k.it.kind == "def" && strings.HasPrefix(x.k.it.name, "SoundDef ")) {
				cosmetic = false
			}
		}
		if len(errs) < *minPlayers {
			continue
		}
		pl := players(ls, errs)
		if pl < *minPlayers {
			continue
		}
		passPlayers++
		without := nA[k.a] - both(k.a, k.b)
		r := float64(len(errs)) / float64(max(without, 1))
		if r < *minRate {
			continue
		}
		passRate++
		sort.Strings(items)
		if len(items) > 5 {
			items = items[:5]
		}
		ex := ""
		for _, t := range top(tpl, 5) {
			if !privateRe.MatchString(t) {
				ex = t
				break
			}
		}
		pr := &known.Prereq{Mod: k.a, Needs: k.b, Logs: len(errs), Players: pl, Without: without, Items: items,
			Class: top(class, 1)[0], Example: ex, Cosmetic: cosmetic}
		mi := lib[k.a]
		row := &prow{pr: pr, rate: r, errs: errs}
		if mi != nil {
			row.declared = mi.deps[k.b]
			pr.LoadAfter = mi.loadAfter[k.b]
		}
		row.wsDeclared = wsDeps[k.a][k.b]
		pr.Fix = fix(ls, links, errs, modChanges([]string{k.a, k.b}))
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i].pr, rows[j].pr
		return a.Players > b.Players || a.Players == b.Players && (a.Mod < b.Mod || a.Mod == b.Mod && a.Needs < b.Needs)
	})
	var decl, ws, after, dlc, cos int
	for _, r := range rows {
		switch {
		case r.declared:
			decl++
			continue
		case r.wsDeclared:
			ws++
		}
		if r.pr.LoadAfter {
			after++
		}
		if official(r.pr.Needs) {
			dlc++
		}
		if r.pr.Cosmetic {
			cos++
		}
		data.Prereqs = append(data.Prereqs, r.pr)
	}
	p("pairs from >= %d players: %d; error in >= %.0f%% of logs with A on and B off: %d", *minPlayers, passPlayers, 100**minRate, passRate)
	p("  declared in A's About.xml (already dep.inactive/dep.missing, dropped): %d (%.0f%%)", decl, pct(decl, passRate))
	p("  kept: %d — only on the Workshop page (steamDB required items; RimArcho does not read them): %d;", len(data.Prereqs), ws)
	p("        A lists B in loadAfter only: %d; B is a DLC: %d; only sounds/textures missing: %d", after, dlc, cos)
	p("\nplayers logs  rate  fix(gone/pairs: change)  A -> B  [what is missing]")
	for _, r := range rows {
		if r.declared {
			continue
		}
		x := r.pr
		p("%5d %5d %4.0f%%  %-26s %s -> %s  %s", x.Players, x.Logs, 100*r.rate, fixText(x.Fix), x.Mod, x.Needs, strings.Join(x.Items[:min(2, len(x.Items))], "; "))
	}

	// ---- combinations ----
	type tk struct{ a, tpl string }
	tlogs := map[tk]set{}
	tclass := map[tk]string{}
	tothers := map[tk]map[string]int{}
	for i, l := range ls {
		for _, s := range l.srcs {
			k := tk{s.a, s.tpl}
			if tlogs[k] == nil {
				tlogs[k], tothers[k] = set{}, map[string]int{}
			}
			if !tlogs[k][i] {
				for o := range s.others {
					tothers[k][o]++
				}
			}
			tlogs[k][i] = true
			tclass[k] = s.class
		}
	}
	g5 := readG5(patternsPath)
	type ck struct{ a, b, v string }
	type cacc struct {
		tpls  map[string]int
		class map[string]int
		g5    []*g5res
	}
	combos := map[ck]*cacc{}
	addC := func(k ck, t tk) {
		c := combos[k]
		if c == nil {
			c = &cacc{tpls: map[string]int{}, class: map[string]int{}}
			combos[k] = c
		}
		c.tpls[t.tpl] = len(tlogs[t])
		c.class[tclass[t]] += len(tlogs[t])
		if r := g5[t.a+"\x00"+t.tpl]; r != nil {
			c.g5 = append(c.g5, r)
		}
	}
	nT := 0
	inPair := map[tk]bool{}
	for t, idx := range tlogs {
		if reporters[t.a] || len([]rune(t.tpl))-len([]rune(tclass[t])) < 20 || len(idx) < *minPlayers || players(ls, idx) < *minPlayers {
			continue
		}
		nT++
		k := len(idx)
		// pair
		cnt := map[string]int{}
		for i := range idx {
			for m := range ls[i].set {
				cnt[m]++
			}
		}
		bestB, bestD, bestRole := "", 0.0, false
		for b, c := range cnt {
			if b == t.a || official(b) || b == "brrainz.harmony" || float64(c) < *cover*float64(k) {
				continue
			}
			if mi := lib[t.a]; mi != nil && mi.deps[b] {
				continue // A requires B: not a combination
			}
			nab := both(t.a, b)
			n0, k0 := nA[t.a]-nab, k-c
			if n0 < 30 || nab == 0 {
				continue
			}
			r1, r0 := float64(c)/float64(nab), float64(k0)/float64(n0)
			if r1 < 0.3 || r1 < 4*max(r0, 0.01) {
				continue
			}
			role := tothers[t][b] > 0
			if bestB == "" || role && !bestRole || role == bestRole && r1-r0 > bestD {
				bestB, bestD, bestRole = b, r1-r0, role
			}
		}
		if bestB != "" {
			addC(ck{t.a, bestB, ""}, t)
			inPair[t] = true
			continue
		}
		// version
		vc := map[string]int{}
		for i := range idx {
			vc[ls[i].ver[t.a]]++
		}
		if v := top(vc, 1)[0]; v != "" && float64(vc[v]) >= *cover*float64(k) {
			nv := 0
			for _, l := range ls {
				if l.set[t.a] && l.ver[t.a] == v {
					nv++
				}
			}
			n0, k0 := nA[t.a]-nv, k-vc[v]
			r1, r0 := float64(vc[v])/float64(nv), float64(k0)/float64(max(n0, 1))
			if n0 >= 30 && r1 >= 0.3 && r1 >= 4*max(r0, 0.01) {
				addC(ck{t.a, "", v}, t)
				inPair[t] = true
				continue
			}
		}
		// single mod
		if nA[t.a] >= 20 && float64(k) >= 0.5*float64(nA[t.a]) {
			addC(ck{t.a, "", ""}, t)
		}
	}
	p("\n== Combinations\n\ntemplates with one source mod, from >= %d players: %d; entries: %d", *minPlayers, nT, len(combos))
	var cks []ck
	for k := range combos {
		cks = append(cks, k)
	}
	sort.Slice(cks, func(i, j int) bool {
		a, b := cks[i], cks[j]
		return a.a < b.a || a.a == b.a && (a.b < b.b || a.b == b.b && a.v < b.v)
	})
	kinds := map[string]int{}
	for _, k := range cks {
		c := combos[k]
		mods := []string{k.a}
		if k.b != "" {
			mods = append(mods, k.b)
		}
		in := func(l *logRec) bool {
			if !l.set[k.a] {
				return false
			}
			if k.b != "" {
				return l.set[k.b]
			}
			if k.v != "" {
				return l.ver[k.a] == k.v
			}
			return true
		}
		errs := set{}
		withoutErr := 0
		for t := range c.tpls {
			for i := range tlogs[tk{k.a, t}] {
				if in(ls[i]) {
					errs[i] = true
				}
			}
		}
		with, without := 0, 0
		werr := set{}
		for i, l := range ls {
			if in(l) {
				with++
			} else if l.set[k.a] {
				without++
				for t := range c.tpls {
					if tlogs[tk{k.a, t}][i] {
						werr[i] = true
					}
				}
			}
		}
		withoutErr = len(werr)
		pl := players(ls, errs)
		if pl < *minPlayers {
			continue
		}
		var tpls []string
		for _, t := range top(c.tpls, 10) {
			if !privateRe.MatchString(t) && len(tpls) < 3 {
				tpls = append(tpls, t)
			}
		}
		if len(tpls) == 0 {
			continue
		}
		x := &known.Combo{Mods: mods, Version: k.v, Class: top(c.class, 1)[0], Templates: tpls, Logs: len(errs), Players: pl,
			With: with, Without: without, WithoutErr: withoutErr}
		chg := []string{k.a}
		if k.b != "" {
			chg = append(chg, k.b)
		}
		x.Fix = fix(ls, links, errs, modChanges(chg))
		x.G5 = mergeG5(c.g5)
		kind := x.Kind()
		kinds[kind]++
		data.Combos = append(data.Combos, x)
	}
	p("kept: %d pairs, %d versions, %d single mods (error in >= 50%% of logs that enable it)", kinds[known.KindPair], kinds[known.KindVersion], kinds[known.KindMod])
	sort.SliceStable(data.Combos, func(i, j int) bool { return data.Combos[i].Players > data.Combos[j].Players })
	p("\nplayers logs  with-rate  without-rate  fix  mods [version]  template")
	for _, x := range data.Combos {
		v := ""
		if x.Version != "" {
			v = " @" + x.Version
		}
		g := ""
		if x.G5 != nil {
			g = " G5:" + x.G5.Text()
		}
		p("%5d %5d  %4.0f%% (%d)  %4.0f%% (%d)  %-22s %s%s  %s%s", x.Players, x.Logs, 100*x.Rate(), x.With, 100*x.RateWithout(), x.Without,
			fixText(x.Fix), strings.Join(x.Mods, " + "), v, trunc(x.Templates[0], 90), g)
	}
	if outPath != "" {
		b, err := json.MarshalIndent(data, "", " ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(outPath, append(b, '\n'), 0o644); err != nil {
			return err
		}
		p("\nwrote %s: %d prerequisites, %d combinations", outPath, len(data.Prereqs), len(data.Combos))
	}
	return nil
}

// reporters log other mods' problems or their own status under their name: their lines
// are not errors of the reporter.
var reporters = map[string]bool{"taranchuk.moderrorchecker": true, "fluxxfield.defloadcache": true, "tyler.rimdoctor": true}

func fixText(f *known.Fix) string {
	if f == nil {
		return "-"
	}
	s := fmt.Sprintf("%d/%d", f.Gone, f.Pairs)
	if c := top(f.Changes, 1); len(c) > 0 {
		s += ": " + c[0]
	}
	return s
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// ---- steamDB ----

var dlcApps = map[string]string{"1149640": "ludeon.rimworld.royalty", "1392840": "ludeon.rimworld.ideology",
	"1826140": "ludeon.rimworld.biotech", "2380740": "ludeon.rimworld.anomaly", "3022790": "ludeon.rimworld.odyssey"}

func readSteamDB(path string) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	if path == "" {
		return out
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var v struct {
		Database map[string]struct {
			PackageID    string                     `json:"packageId"`
			Dependencies map[string]json.RawMessage `json:"dependencies"`
		} `json:"database"`
	}
	if json.Unmarshal(b, &v) != nil {
		return out
	}
	pkgOf := map[string]string{}
	for id, e := range v.Database {
		pkgOf[id] = norm(e.PackageID)
	}
	for id, p := range dlcApps {
		pkgOf[id] = p
	}
	for _, e := range v.Database {
		p := norm(e.PackageID)
		if p == "" {
			continue
		}
		for id := range e.Dependencies {
			if d := pkgOf[id]; d != "" {
				if out[p] == nil {
					out[p] = map[string]bool{}
				}
				out[p][d] = true
			}
		}
	}
	return out
}

// ---- G5 ----

type g5res = known.G5

func readG5(path string) map[string]*g5res {
	out := map[string]*g5res{}
	b, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var v struct {
		Patterns []struct {
			Layer    string `json:"layer"`
			Mod      string `json:"mod"`
			Template string `json:"template"`
			Res      *struct {
				Pairs map[string]int `json:"pairs"`
				Gone  map[string]int `json:"gone"`
				Fixes map[string]int `json:"fixes"`
			} `json:"resolution"`
		} `json:"patterns"`
	}
	if json.Unmarshal(b, &v) != nil {
		return out
	}
	for _, x := range v.Patterns {
		if x.Layer != "mod" || x.Res == nil {
			continue
		}
		out[norm(x.Mod)+"\x00"+x.Template] = &g5res{SmallPairs: x.Res.Pairs["small"], SmallGone: x.Res.Gone["small"],
			SamePairs: x.Res.Pairs["none"], SameGone: x.Res.Gone["none"], Fixes: x.Res.Fixes}
	}
	return out
}

func mergeG5(rs []*g5res) *g5res {
	if len(rs) == 0 {
		return nil
	}
	// The template with the most evidence speaks for the entry (they share the logs).
	best := rs[0]
	for _, r := range rs[1:] {
		if r.SmallPairs+r.SamePairs > best.SmallPairs+best.SamePairs {
			best = r
		}
	}
	if best.SmallPairs+best.SamePairs == 0 {
		return nil
	}
	c := *best
	if len(c.Fixes) == 0 {
		c.Fixes = nil
	}
	return &c
}

func bitsOn(x uint64) int { return bits.OnesCount64(x) }
