// Spike 2: replay every PatchOperation of the active mod set against the merged Defs
// document using antchfx/xmlquery + xpath, then compare the set of failed operations
// with what the game reported in Player.log.
//
// Load semantics mirror the decompiled game code (Verse.LoadedModManager,
// ModContentPack.InitLoadFolders/LoadPatches, DirectXmlLoader.XmlAssetsInModFolder,
// Verse.PatchOperation*). Read-only.
//
//	go run ./spikes/xpathcompat -version 1.6 -out spikes/xpathcompat/RESULT.txt
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	"github.com/antchfx/xmlquery"
	"github.com/antchfx/xpath"
	"github.com/rimarcho/rimarcho/spikes/internal/procmem"
	"github.com/rimarcho/rimarcho/spikes/internal/rwenv"
)

var (
	gameVersion = flag.String("version", "1.6", "major.minor the Player.log was produced with")
	outPath     = flag.String("out", "", "write the diff list here")
	logPath     = flag.String("log", "", "Player.log (default: user data dir)")
	cpuProf     = flag.String("cpuprofile", "", "write a CPU profile")
	traceSub    = flag.String("trace", "", "print every operation whose xpath contains this")
)

type modCtx struct {
	m       *rwenv.Mod
	folders []string // foldersToLoadDescendingOrder
}

func main() {
	flag.Parse()
	if *cpuProf != "" {
		f, err := os.Create(*cpuProf)
		must(err)
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	env, err := rwenv.Detect()
	must(err)
	mc, err := rwenv.ReadModsConfig(env.ModsConfigPath())
	must(err)
	reg := env.Scan()
	if *logPath == "" {
		*logPath = env.PlayerLogPath()
	}
	logInfo, err := os.Stat(*logPath)
	must(err)

	// Running mods in ModsConfig order.
	var mods []*modCtx
	activeNoSuffix := map[string]bool{}
	activeNames := map[string]bool{}
	for _, id := range mc.ActiveMods {
		m := reg.Resolve(id)
		if m == nil {
			fmt.Printf("skip unresolved active entry %s\n", id)
			continue
		}
		mods = append(mods, &modCtx{m: m})
		activeNoSuffix[m.PackageID] = true
		activeNames[m.Name] = true
	}
	for _, mc := range mods {
		mc.folders = loadFolders(mc.m.Dir, *gameVersion, activeNoSuffix)
	}

	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)
	t0 := time.Now()

	// 1. Defs → unified document.
	doc := &xmlquery.Node{Type: xmlquery.DocumentNode}
	root := &xmlquery.Node{Type: xmlquery.ElementNode, Data: "Defs"}
	xmlquery.AddChild(doc, root)
	defFiles, defNodes, parseErrs, staleDefFiles := 0, 0, 0, 0
	staleDefMods := map[string]bool{}
	for _, mc := range mods {
		for _, f := range xmlAssets(mc.folders, "Defs") {
			d, err := parseFile(f)
			if err != nil {
				parseErrs++
				continue
			}
			defFiles++
			if st, err := os.Stat(f); err == nil && st.ModTime().After(logInfo.ModTime()) {
				staleDefFiles++
				staleDefMods[mc.m.Name] = true
			}
			de := docElement(d)
			if de == nil {
				continue
			}
			for c := de.FirstChild; c != nil; {
				next := c.NextSibling
				detach(c)
				appendChild(root, c)
				defNodes++
				c = next
			}
		}
	}
	tDefs := time.Since(t0)

	// 2. Patches, in mod order, file order, document order.
	type topOp struct {
		mod   *modCtx
		op    *Op
		file  string
		stale bool
	}
	var ops []topOp
	patchFiles := 0
	for _, mc := range mods {
		for _, f := range xmlAssets(mc.folders, "Patches") {
			d, err := parseFile(f)
			if err != nil {
				parseErrs++
				continue
			}
			patchFiles++
			de := docElement(d)
			if de == nil || de.Data != "Patch" {
				continue
			}
			stale := false
			if st, err := os.Stat(f); err == nil && st.ModTime().After(logInfo.ModTime()) {
				stale = true
			}
			for c := de.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == xmlquery.ElementNode && c.Data == "Operation" {
					ops = append(ops, topOp{mod: mc, op: parseOp(c), file: f, stale: stale})
				}
			}
		}
	}
	tParse := time.Since(t0)

	indexRoot = root
	sim := &Sim{plans: map[string]*fastPlan{}, doc: doc, activeNoSuffix: activeNoSuffix, activeNames: activeNames, cache: map[string]*xpath.Expr{}}
	type failRec struct {
		key, mod, desc, file string
		stale, unknown      bool
	}
	var simFails []failRec
	opsTotal, opsUnknownTop := 0, 0
	for _, t := range ops {
		opsTotal++
		ok := sim.Apply(t.op)
		if t.op.unknown() {
			opsUnknownTop++
		}
		if !ok {
			simFails = append(simFails, failRec{
				key: failKey(t.file, t.op.String()), mod: t.mod.m.Name, desc: t.op.String(),
				file: t.file, stale: t.stale, unknown: t.op.unknown(),
			})
		}
	}
	tAll := time.Since(t0)
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)
	runtime.GC()
	var memLive runtime.MemStats
	runtime.ReadMemStats(&memLive)
	_, _, peakWS := procmem.MB()
	runtime.KeepAlive(doc)

	// 3. Player.log failures.
	logFails := parseLogFailures(*logPath)

	// 4. Compare as multisets keyed by file + operation description.
	staleFiles := map[string]bool{}
	unknownKeys := map[string]bool{}
	for _, t := range ops {
		if t.stale {
			staleFiles[normPath(t.file)] = true
		}
	}
	simCnt := map[string]int{}
	for _, f := range simFails {
		simCnt[f.key]++
		if f.unknown {
			unknownKeys[f.key] = true
		}
	}
	logCnt := map[string]int{}
	for _, l := range logFails {
		logCnt[l.key]++
	}
	var onlySim, onlyLog []string
	agreeFail := 0
	for _, f := range simFails {
		if logCnt[f.key] > 0 {
			logCnt[f.key]--
			agreeFail++
			continue
		}
		tag := ""
		if f.stale {
			tag = " [stale: file changed after log]"
		}
		if f.unknown {
			tag += " [unknown op class]"
		}
		onlySim = append(onlySim, fmt.Sprintf("[%s] %s\n      file: %s%s", f.mod, f.desc, f.file, tag))
	}
	for _, l := range logFails {
		if logCnt[l.key] > 0 {
			logCnt[l.key]--
			tag := ""
			if staleFiles[normPath(l.file)] {
				tag = " [stale: file changed after log]"
			}
			onlyLog = append(onlyLog, fmt.Sprintf("[%s] %s\n      file: %s%s", l.mod, l.desc, l.file, tag))
		}
	}
	sort.Strings(onlySim)
	sort.Strings(onlyLog)
	staleMismatch := 0
	for _, s := range append(append([]string{}, onlySim...), onlyLog...) {
		if strings.Contains(s, "[stale") {
			staleMismatch++
		}
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, "active mods: %d, load-folder version: %s, log: %s (%s)\n", len(mods), *gameVersion, *logPath, logInfo.ModTime().Format(time.RFC3339))
	fmt.Fprintf(&b, "def files: %d, top-level def nodes: %d, patch files: %d, xml parse errors: %d\n", defFiles, defNodes, patchFiles, parseErrs)
	fmt.Fprintf(&b, "def files modified after the log: %d (in %d mods) — patches targeting them may legitimately differ\n", staleDefFiles, len(staleDefMods))
	fmt.Fprintf(&b, "top-level operations: %d (unknown/mod-defined classes: %d), nested ops evaluated: %d, distinct xpaths: %d, xpath compile errors: %d\n",
		opsTotal, opsUnknownTop, sim.applied, len(sim.cache), sim.compileErrs)
	fmt.Fprintf(&b, "index fast path: %v, hits %d, rebuilds %d\n", *useIndex, sim.ix.hits, sim.ix.rebuilds)
	fmt.Fprintf(&b, "xpath evaluations: %d, runtime errors (attr-node parent etc.): %d\n", sim.evals, sim.runtimeErrs)
	fmt.Fprintf(&b, "time: defs %v, +patch parse %v, total %v; heap in use after: %d MB (Δ %d MB), total alloc %d MB\n",
		tDefs.Round(time.Millisecond), tParse.Round(time.Millisecond), tAll.Round(time.Millisecond),
		memAfter.HeapInuse>>20, (memAfter.HeapInuse-memBefore.HeapInuse)>>20, (memAfter.TotalAlloc-memBefore.TotalAlloc)>>20)
	fmt.Fprintf(&b, "live heap after GC (merged doc retained): %d MB, peak working set: %.0f MB\n", memLive.HeapInuse>>20, peakWS)
	nodes, attrs, textBytes := countNodes(doc)
	fmt.Fprintf(&b, "merged doc after patching: %d nodes, %d attributes, %d MB of text/names (≈ %d bytes heap per node)\n",
		nodes, attrs, textBytes>>20, memLive.HeapInuse/uint64(max(1, nodes)))
	fmt.Fprintf(&b, "\nsimulated failures: %d, Player.log failures: %d, agreeing: %d\n", len(simFails), len(logFails), agreeFail)
	mism := len(onlySim) + len(onlyLog)
	fmt.Fprintf(&b, "mismatches: %d (%d sim-only, %d log-only; %d involve files changed after the log)\n", mism, len(onlySim), len(onlyLog), staleMismatch)
	fmt.Fprintf(&b, "mismatch rate: %d / %d = %.3f%%   excluding stale files: %d / %d = %.3f%%\n",
		mism, opsTotal, 100*float64(mism)/float64(opsTotal), mism-staleMismatch, opsTotal, 100*float64(mism-staleMismatch)/float64(opsTotal))
	fmt.Fprintf(&b, "\n== failed in simulation, not in log ==\n%s\n", strings.Join(onlySim, "\n"))
	fmt.Fprintf(&b, "\n== failed in log, not in simulation ==\n%s\n", strings.Join(onlyLog, "\n"))
	os.Stdout.Write(b.Bytes())
	if *outPath != "" {
		must(os.WriteFile(*outPath, b.Bytes(), 0o644))
	}
}

// ---- load folders (ModContentPack.InitLoadFolders) -------------------------

type lfEntry struct {
	folder                    string
	anyOf, allOf, notAnyOf    []string
}

func loadFolders(root, version string, active map[string]bool) []string {
	var out []string
	lf := readLoadFolders(root)
	if len(lf) > 0 {
		pick := lf[version]
		if len(pick) == 0 {
			best := ""
			for k := range lf {
				if k == "default" || !strings.Contains(k, ".") || !versionLE(k, version) {
					continue
				}
				if best == "" || k > best { // the game also compares the strings
					best = k
				}
			}
			if best != "" {
				pick = lf[best]
			} else {
				pick = lf["default"]
			}
		}
		if pick != nil {
			for i := len(pick) - 1; i >= 0; i-- {
				e := pick[i]
				if shouldLoad(e, active) {
					out = append(out, filepath.Join(root, e.folder))
				}
			}
			return out
		}
	}
	if st, err := os.Stat(filepath.Join(root, version)); err == nil && st.IsDir() {
		out = append(out, filepath.Join(root, version))
	} else {
		best := ""
		ents, _ := os.ReadDir(root)
		var vs []string
		for _, e := range ents {
			if isVersion(e.Name()) {
				vs = append(vs, e.Name())
			}
		}
		sort.Slice(vs, func(i, j int) bool { return versionLE(vs[i], vs[j]) && vs[i] != vs[j] })
		for _, v := range vs {
			if versionLE(v, version) {
				best = v
			} else if best == "" {
				best = v
			}
		}
		if best != "" {
			out = append(out, filepath.Join(root, best))
		}
	}
	if st, err := os.Stat(filepath.Join(root, "Common")); err == nil && st.IsDir() {
		out = append(out, filepath.Join(root, "Common"))
	}
	return append(out, root)
}

func shouldLoad(e lfEntry, active map[string]bool) bool {
	any := func(ids []string) bool {
		for _, id := range ids {
			if active[strings.ToLower(id)] {
				return true
			}
		}
		return false
	}
	if len(e.anyOf) > 0 && !any(e.anyOf) {
		return false
	}
	for _, id := range e.allOf {
		if !active[strings.ToLower(id)] {
			return false
		}
	}
	return len(e.notAnyOf) == 0 || !any(e.notAnyOf)
}

func readLoadFolders(root string) map[string][]lfEntry {
	ents, _ := os.ReadDir(root)
	var path string
	for _, e := range ents {
		if strings.EqualFold(e.Name(), "LoadFolders.xml") {
			path = filepath.Join(root, e.Name())
		}
	}
	if path == "" {
		return nil
	}
	d, err := parseFile(path)
	if err != nil {
		return nil
	}
	de := docElement(d)
	if de == nil {
		return nil
	}
	out := map[string][]lfEntry{}
	split := func(s string) []string {
		var r []string
		for _, p := range strings.Split(s, ",") {
			r = append(r, strings.TrimSpace(p))
		}
		return r
	}
	for v := de.FirstChild; v != nil; v = v.NextSibling {
		if v.Type != xmlquery.ElementNode {
			continue
		}
		key := strings.TrimPrefix(strings.ToLower(v.Data), "v")
		if _, ok := out[key]; !ok {
			out[key] = []lfEntry{}
		}
		for li := v.FirstChild; li != nil; li = li.NextSibling {
			if li.Type != xmlquery.ElementNode {
				continue
			}
			e := lfEntry{folder: li.InnerText()}
			if e.folder == "/" || e.folder == `\` {
				e.folder = ""
			}
			if a := li.SelectAttr("IfModActive"); a != "" {
				e.anyOf = split(a)
			}
			if a := li.SelectAttr("IfModActiveAll"); a != "" {
				e.allOf = split(a)
			}
			if a := li.SelectAttr("IfModNotActive"); a != "" {
				e.notAnyOf = split(a)
			}
			out[key] = append(out[key], e)
		}
	}
	return out
}

var verRe = regexp.MustCompile(`^\d+\.\d+$`)

func isVersion(s string) bool { return verRe.MatchString(s) }

func versionLE(a, b string) bool {
	var a1, a2, b1, b2 int
	fmt.Sscanf(a, "%d.%d", &a1, &a2)
	fmt.Sscanf(b, "%d.%d", &b1, &b2)
	return a1 < b1 || a1 == b1 && a2 <= b2
}

// ---- asset enumeration (DirectXmlLoader.XmlAssetsInModFolder) --------------

// xmlAssets lists *.xml under <folder>/<sub> for each load folder (highest priority
// first), de-duplicated by path relative to the load folder. Enumeration order
// approximates .NET GetFiles(AllDirectories): files of a directory in name order,
// then subdirectories breadth-first.
func xmlAssets(folders []string, sub string) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range folders {
		base := filepath.Join(f, sub)
		queue := []string{base}
		for len(queue) > 0 {
			dir := queue[0]
			queue = queue[1:]
			ents, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			sort.Slice(ents, func(i, j int) bool { return strings.ToLower(ents[i].Name()) < strings.ToLower(ents[j].Name()) })
			for _, e := range ents {
				p := filepath.Join(dir, e.Name())
				if e.IsDir() {
					queue = append(queue, p)
					continue
				}
				n := e.Name()
				if !strings.EqualFold(filepath.Ext(n), ".xml") || strings.HasPrefix(n, ".") {
					continue
				}
				key := strings.ToLower(p[len(f)+1:])
				if !seen[key] {
					seen[key] = true
					out = append(out, p)
				}
			}
		}
	}
	return out
}

// ---- XML helpers -----------------------------------------------------------

// parseFile parses like LoadableXmlAsset: IgnoreComments + IgnoreWhitespace.
func parseFile(path string) (*xmlquery.Node, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	d, err := xmlquery.Parse(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	clean(d)
	return d, nil
}

func clean(n *xmlquery.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		switch c.Type {
		case xmlquery.CommentNode, xmlquery.DeclarationNode, xmlquery.NotationNode:
			detach(c)
		case xmlquery.TextNode:
			if strings.TrimSpace(c.Data) == "" {
				detach(c)
			}
		case xmlquery.ElementNode:
			clean(c)
		}
		c = next
	}
}

func docElement(d *xmlquery.Node) *xmlquery.Node {
	for c := d.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == xmlquery.ElementNode {
			return c
		}
	}
	return nil
}

func detach(n *xmlquery.Node) {
	if n.Parent == nil {
		return
	}
	touch(n.Parent, n)
	if n.PrevSibling != nil {
		n.PrevSibling.NextSibling = n.NextSibling
	} else {
		n.Parent.FirstChild = n.NextSibling
	}
	if n.NextSibling != nil {
		n.NextSibling.PrevSibling = n.PrevSibling
	} else {
		n.Parent.LastChild = n.PrevSibling
	}
	n.Parent, n.PrevSibling, n.NextSibling = nil, nil, nil
}

func appendChild(p, n *xmlquery.Node) {
	touch(p, n)
	n.Parent = p
	n.NextSibling = nil
	n.PrevSibling = p.LastChild
	if p.LastChild != nil {
		p.LastChild.NextSibling = n
	} else {
		p.FirstChild = n
	}
	p.LastChild = n
}

func insertBefore(ref, n *xmlquery.Node) {
	p := ref.Parent
	touch(p, n)
	n.Parent = p
	n.NextSibling = ref
	n.PrevSibling = ref.PrevSibling
	if ref.PrevSibling != nil {
		ref.PrevSibling.NextSibling = n
	} else {
		p.FirstChild = n
	}
	ref.PrevSibling = n
}

func insertAfter(ref, n *xmlquery.Node) {
	if ref.NextSibling == nil {
		appendChild(ref.Parent, n)
		return
	}
	insertBefore(ref.NextSibling, n)
}

func prependChild(p, n *xmlquery.Node) {
	if p.FirstChild == nil {
		appendChild(p, n)
		return
	}
	insertBefore(p.FirstChild, n)
}

func clone(n *xmlquery.Node) *xmlquery.Node {
	c := &xmlquery.Node{Type: n.Type, Data: n.Data, Prefix: n.Prefix, NamespaceURI: n.NamespaceURI}
	if len(n.Attr) > 0 {
		c.Attr = append([]xmlquery.Attr(nil), n.Attr...)
	}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		appendChild(c, clone(ch))
	}
	return c
}

// normalize merges adjacent text nodes (XmlNode.Normalize).
func normalize(p *xmlquery.Node) {
	for c := p.FirstChild; c != nil && c.NextSibling != nil; {
		if c.Type == xmlquery.TextNode && c.NextSibling.Type == xmlquery.TextNode {
			c.Data += c.NextSibling.Data
			detach(c.NextSibling)
			continue
		}
		c = c.NextSibling
	}
}

// ---- Player.log ------------------------------------------------------------

type logFail struct{ key, mod, desc, file string }

var logHead = regexp.MustCompile(`^\[(.*?)\] Patch operation (.*)$`)

func parseLogFailures(path string) []logFail {
	f, err := os.Open(path)
	must(err)
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	var out []logFail
	var cur *logFail
	var desc strings.Builder
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if cur != nil {
			if desc.Len() > 0 { // still collecting a multi-line description
				desc.WriteString("\n" + line)
				if strings.HasSuffix(line, " failed") {
					cur.desc = strings.TrimSuffix(desc.String(), " failed")
					desc.Reset()
				}
				continue
			}
			if strings.HasPrefix(line, "file: ") {
				cur.file = strings.TrimPrefix(line, "file: ")
			}
			cur.key = failKey(cur.file, cur.desc)
			out = append(out, *cur)
			cur = nil
			if strings.HasPrefix(line, "file: ") {
				continue
			}
		}
		if m := logHead.FindStringSubmatch(line); m != nil {
			cur = &logFail{mod: m[1]}
			if strings.HasSuffix(m[2], " failed") {
				cur.desc = strings.TrimSuffix(m[2], " failed")
			} else {
				desc.WriteString(m[2])
			}
		}
	}
	if cur != nil {
		cur.key = failKey(cur.file, cur.desc)
		out = append(out, *cur)
	}
	return out
}

var ws = regexp.MustCompile(`\s+`)

func failKey(file, desc string) string {
	return normPath(file) + "|" + ws.ReplaceAllString(strings.TrimSpace(desc), " ")
}

func normPath(p string) string { return strings.ToLower(filepath.Clean(strings.ReplaceAll(p, "/", `\`))) }

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func countNodes(n *xmlquery.Node) (nodes, attrs, bytes int) {
	nodes = 1
	attrs = len(n.Attr)
	bytes = len(n.Data)
	for _, a := range n.Attr {
		bytes += len(a.Name.Local) + len(a.Value)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		cn, ca, cb := countNodes(c)
		nodes, attrs, bytes = nodes+cn, attrs+ca, bytes+cb
	}
	return
}
