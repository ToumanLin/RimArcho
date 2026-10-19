package app

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rimarcho/rimarcho/internal/analyze"
	"github.com/rimarcho/rimarcho/internal/content"
	"github.com/rimarcho/rimarcho/internal/env"
	"github.com/rimarcho/rimarcho/internal/finding"
	"github.com/rimarcho/rimarcho/internal/logs"
	"github.com/rimarcho/rimarcho/internal/store"
)

// Attribution roles (03 §6.3).
const (
	RoleSource     = "source"
	RolePatcher    = "patcher"
	RoleDependency = "dependency"
	RoleMentioned  = "mentioned"
)

var roleRank = map[string]int{RoleSource: 0, RolePatcher: 1, RoleDependency: 2, RoleMentioned: 3}

// Attribution is one candidate mod of a cluster.
type Attribution struct {
	Instance string
	Role     string
	Evidence []string // "log harmony:Verse.Pawn.Tick#prefix …" etc.
}

// LogCluster is an attributed error cluster.
type LogCluster struct {
	*logs.Cluster
	Attr       []*Attribution
	Confidence string
	Why        string // explanation of the confidence
	Next       []string
	Checklist  []string
	Details    []string // class-specific facts (patch reasons, cross-reference providers …)
	Finding    *finding.Finding
}

// LogReport is an analysed log.
type LogReport struct {
	Path    string
	Label   string // "Player.log", "Player-prev.log" or "<text>"
	ModTime time.Time
	Lines   int
	Version string
	// AnalysedAs is the major.minor the log was attributed against when it differs from
	// the environment's (EnvMM): the log's own version (decisions-m4 §7.3).
	AnalysedAs, EnvMM string
	Clusters          []*LogCluster
}

// LogPaths returns the current and previous Player.log for the environment's (or the
// overridden) game version: 1.7+ under Logs/, older in the user data root (02 §2.3). When
// the expected file is missing, the other location is used.
func (a *App) LogPaths(st *LoadoutState) (cur, prev string) {
	root := st.Ix.Env.UserDataRoot
	if root == "" {
		return "", ""
	}
	cur = env.PlayerLog(root, st.GameMM())
	if cur == "" {
		cur = env.PlayerLog(root, "")
	}
	if cur == "" {
		return "", ""
	}
	prev = filepath.Join(filepath.Dir(cur), "Player-prev.log")
	if _, err := os.Stat(prev); err != nil {
		prev = ""
	}
	return cur, prev
}

type logCacheEntry struct {
	key string
	rep *LogReport
}

// AnalyzeLog parses and attributes a log file (cached by path, size and mtime).
func (a *App) AnalyzeLog(path string) (*LogReport, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("%s|%d|%d|%s|%s", path, fi.Size(), fi.ModTime().UnixNano(), a.Store.ChangeKey(), a.GameVersion)
	if a.logCache != nil && a.logCache.key == key {
		return a.logCache.rep, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	lf, err := logs.Parse(f)
	if err != nil {
		return nil, err
	}
	rep := &LogReport{Path: path, Label: filepath.Base(path), ModTime: fi.ModTime(), Lines: lf.Lines, Version: lf.Version}
	fails, _ := analyze.ParseLogFailures(path)
	st, restore, err := a.logState(st, rep)
	if err != nil {
		return nil, err
	}
	defer restore()
	if err := a.attribute(st, rep, lf, fails); err != nil {
		return nil, err
	}
	a.logCache = &logCacheEntry{key: key, rep: rep}
	return rep, nil
}

// AnalyzeLogText attributes a pasted stack trace or log excerpt.
func (a *App) AnalyzeLogText(text string) (*LogReport, error) {
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	lf, err := logs.Parse(strings.NewReader(text))
	if err != nil {
		return nil, err
	}
	rep := &LogReport{Label: "<text>", Lines: lf.Lines, Version: lf.Version}
	st, restore, err := a.logState(st, rep)
	if err != nil {
		return nil, err
	}
	defer restore()
	return rep, a.attribute(st, rep, lf, nil)
}

// LogMM is the game major.minor a log was written by ("" if it does not say).
func LogMM(version string) string { return env.MajorMinor(version) }

// logState switches attribution to the log's own game version when it differs from the
// environment's and no --game-version was given (decisions-m4 §7.3). restore undoes it.
func (a *App) logState(st *LoadoutState, rep *LogReport) (*LoadoutState, func(), error) {
	mm := LogMM(rep.Version)
	if a.GameVersion != "" || mm == "" || mm == st.GameMM() {
		return st, func() {}, nil
	}
	prev := a.GameVersion
	rep.AnalysedAs, rep.EnvMM = mm, st.GameMM()
	a.GameVersion = mm
	restore := func() { a.GameVersion = prev }
	s2, err := a.Loadout()
	if err != nil {
		restore()
		return nil, nil, err
	}
	return s2, restore, nil
}

// EnsureLogAnalysis brings the analysis for a log's own game version up to date when it
// differs from the environment's, so patch failures can be matched to the simulation.
func (a *App) EnsureLogAnalysis(st *LoadoutState, version string, progress func(string)) error {
	mm := LogMM(version)
	if a.GameVersion != "" || mm == "" || mm == st.GameMM() {
		return nil
	}
	a.GameVersion = mm
	defer func() { a.GameVersion = "" }()
	_, err := a.EnsureAnalysis(progress)
	return err
}

// TextLogVersion is LogVersionOf for pasted text.
func TextLogVersion(text string) string {
	if len(text) > 64<<10 {
		text = text[:64<<10]
	}
	lf, err := logs.Parse(strings.NewReader(text))
	if err != nil {
		return ""
	}
	return lf.Version
}

// LogVersionOf reads the game version a log file names in its first lines.
func LogVersionOf(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	lf, err := logs.Parse(io.LimitReader(f, 64<<10))
	if err != nil {
		return ""
	}
	return lf.Version
}

type attrCtx struct {
	a        *App
	st       *LoadoutState
	byName   map[string][]*store.Mod // display name (lower) → active mods
	byPkg    map[string][]*store.Mod
	dirs     []dirOwner
	typeMemo map[string][]string
	idMemo   map[string][]string
	run      *store.Analysis
	fails    map[string]*analyze.PatchFact // FailKey(game path, desc) → fact
	byXPath  map[string][]*analyze.PatchFact
}

type dirOwner struct {
	dir string // lower case, with trailing separator
	id  string
}

func (a *App) attribute(st *LoadoutState, rep *LogReport, lf *logs.File, fails []analyze.LogFailure) error {
	cx := &attrCtx{a: a, st: st, byName: map[string][]*store.Mod{}, byPkg: map[string][]*store.Mod{}, typeMemo: map[string][]string{}, idMemo: map[string][]string{}}
	for _, e := range st.List {
		m := st.Ix.ByID[e.Instance]
		if m == nil {
			continue
		}
		cx.byName[strings.ToLower(m.DisplayName())] = append(cx.byName[strings.ToLower(m.DisplayName())], m)
		cx.byPkg[strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam")] = append(cx.byPkg[strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam")], m)
	}
	for _, m := range st.Ix.Mods {
		if !m.TombstoneAt.IsZero() {
			continue
		}
		for _, d := range []string{st.Ix.ModDir(m), gamePath(st.Ix, m)} {
			if d != "" {
				cx.dirs = append(cx.dirs, dirOwner{strings.ToLower(filepath.Clean(d)) + string(os.PathSeparator), m.ID})
			}
		}
	}
	sort.Slice(cx.dirs, func(i, j int) bool { return len(cx.dirs[i].dir) > len(cx.dirs[j].dir) })
	if s, err := a.Analysis(st); err == nil && s.Run != nil {
		cx.run = s.Run
		if pf, err := a.Store.PatchFacts(s.Run.ID, store.PatchFilter{FailedOnly: true}); err == nil {
			cx.fails = map[string]*analyze.PatchFact{}
			cx.byXPath = map[string][]*analyze.PatchFact{}
			for _, p := range pf {
				if p.Leaf != "" {
					cx.byXPath[p.Leaf] = append(cx.byXPath[p.Leaf], p)
				}
				if m := st.Ix.ByID[p.Instance]; m != nil {
					for _, root := range []string{gamePath(st.Ix, m), st.Ix.ModDir(m)} {
						cx.fails[analyze.FailKey(filepath.Join(root, filepath.FromSlash(p.File)), p.Desc)] = p
					}
				}
			}
		}
	}
	// Map the multi-line patch-failure records (file + description) onto entries by line.
	failAt := map[string]analyze.LogFailure{}
	for _, f := range fails {
		failAt[f.Mod+"|"+strings.SplitN(f.Desc, "\n", 2)[0]] = f
	}
	var xrefs []*LogCluster
	for _, c := range logs.Clusters(lf) {
		lc := &LogCluster{Cluster: c}
		cx.attributeCluster(lc, failAt)
		if c.Class == logs.ClassXrefMissing {
			xrefs = append(xrefs, lc)
		}
		rep.Clusters = append(rep.Clusters, lc)
	}
	if len(xrefs) > 0 {
		cx.xrefReferrers(xrefs)
	}
	for _, lc := range rep.Clusters {
		// Source searches cover the cluster's candidates, not every enabled mod (decisions-m4 §7.4).
		in := ""
		for i, at := range lc.Attr {
			if i == 4 {
				break
			}
			in += " --in " + cx.st.Ix.Ref(at.Instance)
		}
		if in == "" {
			in = " --implicated"
		}
		for i, n := range lc.Next {
			lc.Next[i] = strings.Replace(n, " "+inPlaceholder, in, 1)
		}
		lc.Finding = cx.toFinding(lc)
	}
	return nil
}

const inPlaceholder = "<in>"

// ImplicatedMods are the mods the newest log and the current problems point at: the
// candidates of the problem log's error clusters, else the subjects of Harmony warnings.
// MCP source searches default to them (decisions-m4 §7.4).
func (a *App) ImplicatedMods(st *LoadoutState) []string {
	seen := map[string]bool{}
	var out []string
	addID := func(id string) {
		if id != "" && !seen[id] && st.Ix.ByID[id] != nil {
			seen[id] = true
			out = append(out, id)
		}
	}
	if p := a.ProblemLog(st); p != "" {
		if rep, err := a.AnalyzeLog(p); err == nil {
			for _, lc := range rep.Clusters {
				if lc.Class == logs.ClassOther {
					continue
				}
				for _, at := range lc.Attr {
					addID(at.Instance)
				}
			}
		}
	}
	if len(out) == 0 {
		if ps, err := a.Problems(false, nil); err == nil {
			for _, p := range ps.List {
				if p.Layer == "harmony" && p.Severity != finding.Info {
					for _, s := range p.Subjects {
						addID(finding.InstID(s))
					}
				}
			}
		}
	}
	return out
}

// gamePath is where the game sees an instance (the source root + folder, links unresolved).
func gamePath(ix *Index, m *store.Mod) string {
	if s := ix.Source(m.SourceID); s != nil {
		return filepath.Join(s.Root, m.Rel)
	}
	return ""
}

func (lc *LogCluster) add(inst, role, ev string) {
	for _, x := range lc.Attr {
		if x.Instance == inst {
			if roleRank[role] < roleRank[x.Role] {
				x.Role = role
			}
			for _, e := range x.Evidence {
				if e == ev {
					return
				}
			}
			x.Evidence = append(x.Evidence, ev)
			return
		}
	}
	lc.Attr = append(lc.Attr, &Attribution{Instance: inst, Role: role, Evidence: []string{ev}})
}

func (lc *LogCluster) next(cmds ...string) {
	for _, c := range cmds {
		dup := false
		for _, n := range lc.Next {
			dup = dup || n == c
		}
		if !dup {
			lc.Next = append(lc.Next, c)
		}
	}
}

func (cx *attrCtx) types(t string) []string {
	if r, ok := cx.typeMemo[t]; ok {
		return r
	}
	var out []string
	if owners, err := cx.a.Store.TypeOwners(t); err == nil {
		for _, o := range owners {
			if cx.st.Position(o[0]) > 0 || cx.st.Ix.ByID[o[0]] != nil {
				out = append(out, o[0])
			}
		}
	}
	out = uniqStr(out)
	cx.typeMemo[t] = out
	return out
}

func (cx *attrCtx) harmonyOwners(p logs.Patch) ([]string, string) {
	if ids := cx.types(p.PatchType); len(ids) > 0 {
		return ids, "type " + p.PatchType
	}
	if r, ok := cx.idMemo[p.Owner]; ok {
		return r, "Harmony ID " + p.Owner
	}
	var out []string
	if ids, err := cx.a.Store.HarmonyIDOwners(p.Owner); err == nil {
		out = ids
	}
	for _, m := range cx.byPkg[strings.ToLower(p.Owner)] {
		out = append(out, m.ID)
	}
	out = uniqStr(out)
	cx.idMemo[p.Owner] = out
	return out, "Harmony ID " + p.Owner
}

func (cx *attrCtx) dirOwner(path string) (string, string) {
	p := strings.ToLower(filepath.Clean(strings.ReplaceAll(path, "/", string(os.PathSeparator))))
	for _, d := range cx.dirs {
		if strings.HasPrefix(p, d.dir) {
			return d.id, filepath.ToSlash(path[len(d.dir):])
		}
	}
	return "", ""
}

var (
	bracketRe = regexp.MustCompile(`^(?:<color=[^>]*>)?\[([^\]]{2,80})\](?:</color>)?`)
	xmlExtRe  = regexp.MustCompile(`^Verse\.PatchOperation\w*\(xpath="(.*)"\): Failed`)
	winPathRe = regexp.MustCompile(`[A-Za-z]:[\\/][^"'<>|\r\n]*?\.(?:xml|dll|png|txt)`)
	xrefRe    = regexp.MustCompile(`No ([\w.]+) named (\S+) found to give to (\S+)|cross-reference to ([\w.]+) named (\S+)|reference to ([\w.]+) named (\S+)`)
	typeTokRe = regexp.MustCompile(`\b([A-Za-z_]\w*(?:\.[A-Za-z_]\w*)+)\b`)
)

var hotSpots = map[string]bool{"Verse.Pawn.Tick": true, "Verse.ThingWithComps.Tick": true, "Verse.Thing.DoTick": true,
	"RimWorld.Planet.WorldPawns.WorldPawnsTick": true, "Verse.Pawn.TickRare": true, "Verse.ThingWithComps.TickRare": true,
	"Verse.TickList.Tick": true, "Verse.ThingWithComps.CompTick": true}

// tickChecklist condenses the inspector workflow for tick crashes (assembly-inspector skill).
var tickChecklist = []string{
	"ThingWithComps.Tick caches comps.Count and indexes AllComps in a for loop: removing or reordering comps during a tick throws ArgumentOutOfRangeException in List`1.get_Item",
	"red flags: parent.AllComps.Remove(this) inside CompTick(); pawn.AllComps.Add/Remove for runtime comps without deferral; Swap or list surgery on AllComps; runtime ThingComps added without InitializeComps() invariants",
	"a PREFIX/POSTFIX line only shows who touched the method last; it is not the cause without other evidence",
	"also check ExposeData/PostExposeData, SpawnSetup, DeSpawn and Destroy of the mods that patch the method",
}

func (cx *attrCtx) attributeCluster(lc *LogCluster, failAt map[string]analyze.LogFailure) {
	e := lc.First
	ix := cx.st.Ix
	// 1. "[Mod Name] …" prefix: the mod logs its own problem.
	if m := bracketRe.FindStringSubmatch(e.Message); m != nil {
		for _, mod := range cx.byName[strings.ToLower(m[1])] {
			lc.add(mod.ID, RoleSource, fmt.Sprintf("log message prefix [%s]", m[1]))
		}
	}
	// 2. File paths in the message or its continuation lines.
	texts := append([]string{e.Message}, e.Extra...)
	for _, t := range texts {
		for _, p := range winPathRe.FindAllString(t, -1) {
			if id, rel := cx.dirOwner(p); id != "" {
				lc.add(id, RoleSource, "log file "+rel)
			}
		}
	}
	switch lc.Class {
	case logs.ClassPatchFailed:
		cx.patchFailure(lc, failAt)
	case logs.ClassXrefMissing:
		if m := xrefRe.FindStringSubmatch(e.Message); m != nil {
			typ, name := m[1]+m[4]+m[6], m[2]+m[5]+m[7]
			short := typ[strings.LastIndexByte(typ, '.')+1:]
			lc.Details = append(lc.Details, "missing: def:"+short+"/"+name)
			if strings.HasPrefix(e.Message, "Could not load reference") {
				lc.Details = append(lc.Details, "raised while loading a save: the save references a def no active mod provides")
			}
			defs, _ := cx.a.Store.DefsNamed(name, false)
			var prov []string
			for _, d := range defs {
				if d.Type != short && !strings.HasSuffix(typ, "."+d.Type) {
					continue
				}
				if cx.st.Position(d.Instance) > 0 {
					prov = append(prov, ix.Ref(d.Instance)+" (active)")
				} else if ix.ByID[d.Instance] != nil {
					prov = append(prov, ix.Ref(d.Instance)+" (not enabled)")
					lc.add(d.Instance, RoleDependency, fmt.Sprintf("defines %s %s at %s:%d but is not enabled", short, name, d.File, d.Line))
					lc.next("archo enable " + ix.Ref(d.Instance))
				}
			}
			if len(prov) == 0 {
				lc.Details = append(lc.Details, "providers: none among installed mods")
			} else {
				lc.Details = append(lc.Details, "providers: "+strings.Join(uniqStr(prov), ", "))
			}
			lc.next("archo def "+short+"/"+name, fmt.Sprintf("archo files search %q", name))
		}
	case logs.ClassMaterial:
		lc.next(`archo files search "uiIconColor"`, `archo files search "uiIconPath"`)
		lc.Checklist = append(lc.Checklist, "a _MainTex/ResolveIcon error is XML-side first: look for buildable defs with uiIconColor but no uiIconPath, uiIconPathsStuff or graphicData/texPath, including abstract bases")
	}
	// 3. Stack frames and Harmony lines.
	firstMod := true
	hot := false
	for i, fr := range e.Frames {
		target := fr.Type + "." + fr.Method
		if hotSpots[strings.ReplaceAll(target, "+", ".")] {
			hot = true
		}
		for _, p := range fr.Harmony {
			ids, via := cx.harmonyOwners(p)
			for _, id := range ids {
				lc.add(id, RolePatcher, fmt.Sprintf("log harmony:%s#%s %q (%s)", target, p.Kind, "- "+strings.ToUpper(p.Kind)+" "+p.Owner, via))
			}
			if len(ids) == 0 {
				lc.Details = append(lc.Details, fmt.Sprintf("harmony owner %q on %s: no installed mod matches", p.Owner, target))
			}
			lc.next("archo harmony --target " + strings.ReplaceAll(target, "+", "."))
		}
		if logs.GameNamespace(fr.Type) {
			continue
		}
		ids := cx.types(fr.Type)
		for _, id := range ids {
			if firstMod {
				lc.add(id, RoleSource, fmt.Sprintf("log frame %d %s (topmost mod code in the stack)", i+1, target))
				lc.next(fmt.Sprintf("archo source %s --type %s", ix.Ref(id), outerType(fr.Type)))
			} else {
				lc.add(id, RoleMentioned, fmt.Sprintf("log frame %d %s", i+1, target))
			}
		}
		if len(ids) > 0 {
			firstMod = false
		}
	}
	if hot {
		lc.next(`archo source search "AllComps.Remove(" `+inPlaceholder, `archo source search "AllComps.Add(" `+inPlaceholder)
		lc.Checklist = append(lc.Checklist, tickChecklist...)
	}
	// 4. Type names in the message ("Error while instantiating a mod of type X").
	for _, t := range typeTokRe.FindAllString(e.Message, -1) {
		if logs.GameNamespace(t) || !strings.Contains(t, ".") {
			continue
		}
		role := RoleMentioned
		if strings.Contains(e.Message, "instantiating a mod of type "+t) {
			role = RoleSource
		}
		ids := cx.types(t)
		if len(ids) == 0 {
			// Type.Method: try the declaring type.
			if i := strings.LastIndexByte(t, '.'); i > 0 {
				ids = cx.types(t[:i])
			}
		}
		for _, id := range ids {
			lc.add(id, role, "log message names type "+t)
			if role == RoleSource {
				lc.next(fmt.Sprintf("archo source %s --type %s", ix.Ref(id), outerType(t)))
			}
		}
	}
	sort.SliceStable(lc.Attr, func(i, j int) bool { return roleRank[lc.Attr[i].Role] < roleRank[lc.Attr[j].Role] })
	switch {
	case len(lc.Attr) > 0 && lc.Attr[0].Role == RoleSource:
		lc.Confidence, lc.Why = finding.Likely, ""
		for _, ev := range lc.Attr[0].Evidence {
			if strings.HasPrefix(ev, "log file ") || strings.HasPrefix(ev, "patch ") {
				lc.Confidence = finding.Certain
			}
		}
	case len(lc.Attr) > 0:
		lc.Confidence, lc.Why = finding.Possible, "no source identified"
	default:
		lc.Confidence, lc.Why = finding.Possible, "no mod identified"
	}
}

func outerType(t string) string {
	if i := strings.IndexByte(t, '+'); i >= 0 {
		return t[:i]
	}
	return t
}

func (cx *attrCtx) patchFailure(lc *LogCluster, failAt map[string]analyze.LogFailure) {
	e := lc.First
	m := bracketRe.FindStringSubmatch(e.Message)
	desc := e.Message
	if i := strings.Index(desc, "Patch operation "); i >= 0 {
		desc = strings.TrimSuffix(desc[i+len("Patch operation "):], " failed")
	}
	var lf analyze.LogFailure
	ok := false
	if m != nil {
		lf, ok = failAt[m[1]+"|"+desc]
	}
	if !ok || lf.File == "" {
		for _, x := range e.Extra {
			if strings.HasPrefix(x, "file: ") {
				lf.File, ok = strings.TrimPrefix(x, "file: "), true
				lf.Desc = desc
			}
		}
	}
	if !ok || lf.File == "" {
		// XML Extensions logs each failure as Verse.PatchOperationX(xpath="…"): Failed …
		if m := xmlExtRe.FindStringSubmatch(e.Message); m != nil {
			for _, p := range cx.byXPath[m[1]] {
				lc.add(p.Instance, RoleSource, fmt.Sprintf("patch %s:%d %s (matched by XPath)", p.File, p.Line, p.Class))
				lc.Details = append(lc.Details, fmt.Sprintf("simulation: %s:%d fails (patch.no-match: %s)", p.File, p.Line, strings.Join(p.Reasons, ", ")))
				lc.next("archo patches --mod " + cx.st.Ix.Ref(p.Instance) + " --failed")
			}
		}
		return
	}
	id, rel := cx.dirOwner(lf.File)
	if id == "" {
		return
	}
	if p := cx.fails[analyze.FailKey(lf.File, lf.Desc)]; p != nil {
		lc.add(id, RoleSource, fmt.Sprintf("patch %s:%d %s", p.File, p.Line, p.Class))
		lc.Details = append(lc.Details, fmt.Sprintf("simulation: %s:%d fails (patch.no-match: %s)", p.File, p.Line, strings.Join(p.Reasons, ", ")))
		if p.Detail != "" {
			lc.Details = append(lc.Details, finding.Expand(p.Detail, cx.st.Ix.Ref))
		}
		for _, tok := range regexp.MustCompile(`@\{[a-z0-9]+\}`).FindAllString(p.Detail, -1) {
			if inst := finding.InstID(tok); inst != "" && cx.st.Position(inst) == 0 {
				lc.add(inst, RoleDependency, "the patch targets a def it provides")
				lc.next("archo enable " + cx.st.Ix.Ref(inst))
			}
		}
	} else {
		lc.add(id, RoleSource, "patch file "+rel)
		if cx.run != nil {
			lc.Details = append(lc.Details, "simulation: this operation does not fail now (the mod or the Loadout changed since the log)")
		}
	}
	lc.next("archo patches --mod "+cx.st.Ix.Ref(id)+" --failed", fmt.Sprintf("archo files search %q --mod %s", lastToken(lf.Desc), cx.st.Ix.Ref(id)))
}

func lastToken(desc string) string {
	if m := regexp.MustCompile(`(?:defName|@Name)\s*=\s*["']([^"']+)`).FindStringSubmatch(desc); m != nil {
		return m[1]
	}
	return desc
}

// xrefReferrers finds which enabled mods' effective XML mentions each missing def (B2),
// reading every file once.
func (cx *attrCtx) xrefReferrers(cs []*LogCluster) {
	type tok struct {
		name string
		lc   *LogCluster
	}
	var toks []tok
	for _, lc := range cs {
		if m := xrefRe.FindStringSubmatch(lc.First.Message); m != nil {
			toks = append(toks, tok{m[2] + m[5] + m[7], lc})
		}
	}
	if len(toks) == 0 {
		return
	}
	var mods []*store.Mod
	for _, e := range cx.st.List {
		if m := cx.st.Ix.ByID[e.Instance]; m != nil {
			mods = append(mods, m)
		}
	}
	if err := cx.a.Store.LoadDecls(mods); err != nil {
		return
	}
	active := map[string]bool{}
	for _, m := range mods {
		active[strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam")] = true
	}
	type job struct {
		m  *store.Mod
		as content.Asset
	}
	var jobs []job
	for _, m := range mods {
		dir := cx.st.Ix.ModDir(m)
		r := content.Resolve(dir, m.Decl().LoadFolders, cx.st.GameMM(), func(p string) bool { return active[strings.TrimSuffix(strings.ToLower(p), "_steam")] })
		for _, sub := range []string{"Defs", "Patches"} {
			for _, as := range content.Assets(dir, r.Folders, sub) {
				jobs = append(jobs, job{m, as})
			}
		}
	}
	// A def is referenced as element text (<thingDef>X</thingDef>, <li>X</li>) or, in
	// ThingDefCountClass lists such as costList and products, as an element name (<X>1</X>).
	needles := make([][2][]byte, len(toks))
	for i, t := range toks {
		needles[i] = [2][]byte{[]byte(">" + t.name + "<"), []byte("<" + t.name + ">")}
	}
	type hit struct {
		t    int
		inst string
		file string
		line int
	}
	var mu sync.Mutex
	var hits []hit
	var wg sync.WaitGroup
	next := make(chan job)
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range next {
				b, err := os.ReadFile(j.as.Path)
				if err != nil {
					continue
				}
				for i, n := range needles {
					k := bytes.Index(b, n[0])
					if k2 := bytes.Index(b, n[1]); k2 >= 0 && (k < 0 || k2 < k) {
						k = k2
					}
					if k >= 0 {
						mu.Lock()
						hits = append(hits, hit{i, j.m.ID, j.as.Rel, bytes.Count(b[:k], []byte{'\n'}) + 1})
						mu.Unlock()
					}
				}
			}
		}()
	}
	for _, j := range jobs {
		next <- j
	}
	close(next)
	wg.Wait()
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].t != hits[j].t {
			return hits[i].t < hits[j].t
		}
		return cx.st.Position(hits[i].inst) < cx.st.Position(hits[j].inst)
	})
	for _, h := range hits {
		t := toks[h.t]
		t.lc.add(h.inst, RoleSource, fmt.Sprintf("references %s at %s:%d", t.name, h.file, h.line))
	}
	for _, t := range toks {
		sort.SliceStable(t.lc.Attr, func(i, j int) bool { return roleRank[t.lc.Attr[i].Role] < roleRank[t.lc.Attr[j].Role] })
		if len(t.lc.Attr) > 0 && t.lc.Attr[0].Role == RoleSource {
			t.lc.Confidence, t.lc.Why = finding.Likely, ""
		}
	}
}

var logSeverity = map[string]string{
	logs.ClassException: finding.Error, logs.ClassHarmony: finding.Error, logs.ClassXMLLoad: finding.Error,
	logs.ClassPatchFailed: finding.Warning, logs.ClassXrefMissing: finding.Warning, logs.ClassMaterial: finding.Warning,
	logs.ClassOther: finding.Info,
}

// findingKey keeps a log problem's ref (and an ignore of it) stable across logs and
// install locations: the cluster key with each absolute path cut to its last three parts.
func findingKey(key string) string {
	return winPathRe.ReplaceAllStringFunc(key, func(p string) string {
		parts := strings.FieldsFunc(p, func(r rune) bool { return r == '\\' || r == '/' })
		if len(parts) > 3 {
			parts = parts[len(parts)-3:]
		}
		return strings.ToLower(strings.Join(parts, "/"))
	})
}

func (cx *attrCtx) toFinding(lc *LogCluster) *finding.Finding {
	msg := lc.First.Message
	if len(msg) > 160 {
		msg = msg[:157] + "..."
	}
	f := &finding.Finding{Code: "log." + lc.Class, Severity: logSeverity[lc.Class], Layer: "log", Key: findingKey(lc.Key), Confidence: lc.Confidence,
		Claim: fmt.Sprintf("log:%s x%d: %s", lc.ID, lc.Count, msg)}
	for _, at := range lc.Attr {
		f.Subjects = append(f.Subjects, finding.Inst(at.Instance))
		for _, ev := range at.Evidence {
			f.Evidence = append(f.Evidence, finding.Evidence{Source: finding.Log, Ref: finding.Inst(at.Instance), Text: at.Role + ": " + ev})
		}
	}
	if len(f.Subjects) == 0 {
		f.Subjects = []string{"log:" + lc.ID}
	}
	for _, d := range lc.Details {
		f.Evidence = append(f.Evidence, finding.Evidence{Source: finding.Inferred, Text: d})
	}
	f.Suggestions = append([]string{"archo log --cluster " + lc.ID}, lc.Next...)
	return f
}

func uniqStr(l []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range l {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// SelectedLog is the log whose clusters `problems` includes ("" if none) (06 §4).
func (a *App) SelectedLog(st *LoadoutState) string {
	return a.Store.Meta("log.selected:" + st.Ix.Env.ID)
}

// SelectLog makes problems include a log's clusters and records its mtime as seen.
func (a *App) SelectLog(st *LoadoutState, path string) error {
	if err := a.Store.SetMeta("log.selected:"+st.Ix.Env.ID, path); err != nil {
		return err
	}
	if fi, err := os.Stat(path); err == nil {
		return a.Store.SetMeta("log.seen:"+st.Ix.Env.ID, fmt.Sprint(fi.ModTime().Unix()))
	}
	return nil
}

// NewLog reports whether the current Player.log is newer than the last launch and the
// last log the player looked at (06 §7.3).
func (a *App) NewLog(st *LoadoutState) (string, bool) {
	cur, _ := a.LogPaths(st)
	if cur == "" {
		return "", false
	}
	fi, err := os.Stat(cur)
	if err != nil {
		return "", false
	}
	var seen int64
	fmt.Sscan(a.Store.Meta("log.seen:"+st.Ix.Env.ID), &seen)
	if fi.ModTime().Unix() <= seen {
		return cur, false
	}
	if l := a.Store.LastLaunch(st.Ix.Env.ID); !l.IsZero() && fi.ModTime().Before(l) {
		return cur, false
	}
	return cur, true
}

// ProblemLog is the log whose clusters join the problems (decisions-m4 §7.2): the
// selected one, or the current Player.log when that is newer.
func (a *App) ProblemLog(st *LoadoutState) string {
	sel := a.SelectedLog(st)
	cur, _ := a.LogPaths(st)
	switch {
	case cur == "":
		return sel
	case sel == "":
		return cur
	}
	fc, err1 := os.Stat(cur)
	fs, err2 := os.Stat(sel)
	if err2 != nil || (err1 == nil && fc.ModTime().After(fs.ModTime())) {
		return cur
	}
	return sel
}

// hotMethod is a method that appears in a log cluster (a frame, or the frame a Harmony
// patch line belongs to), with the cluster that shows it.
type hotMethod struct {
	cluster string
	count   int
}

// logFindings returns the problem log's clusters as findings (informational ones left out)
// and the methods its error clusters pass through.
func (a *App) logFindings(st *LoadoutState) ([]*finding.Finding, map[string]hotMethod) {
	path := a.ProblemLog(st)
	if path == "" {
		return nil, nil
	}
	rep, err := a.AnalyzeLog(path)
	if err != nil {
		return nil, nil
	}
	var out []*finding.Finding
	hot := map[string]hotMethod{}
	for _, lc := range rep.Clusters {
		if lc.Class == logs.ClassOther {
			continue
		}
		if lc.Finding != nil {
			out = append(out, lc.Finding)
		}
		for _, fr := range lc.First.Frames {
			if fr.Type == "" || fr.Method == "" {
				continue
			}
			m := strings.ReplaceAll(fr.Type, "+", ".") + "." + fr.Method
			if _, ok := hot[m]; !ok {
				hot[m] = hotMethod{"log:" + lc.ID, lc.Count}
			}
		}
	}
	return out, hot
}

// escalateSkips raises harmony.skip-prefix to warning when its method is in a log cluster
// (decisions-m4 §7.1): a bool prefix is ordinary until a stack trace passes through it.
func escalateSkips(fs []*finding.Finding, hot map[string]hotMethod) []*finding.Finding {
	if len(hot) == 0 {
		return fs
	}
	for i, f := range fs {
		if f.Code != "harmony.skip-prefix" {
			continue
		}
		t, _, _ := strings.Cut(f.Key, "|")
		if j := strings.IndexByte(t, '('); j >= 0 {
			t = t[:j]
		}
		h, ok := hot[t]
		if !ok {
			continue
		}
		c := *f
		c.Severity = finding.Warning
		c.Evidence = append(append([]finding.Evidence{}, f.Evidence...), finding.Evidence{Source: finding.Log, Ref: h.cluster,
			Text: fmt.Sprintf("%s is in the stack of %s (x%d)", t, h.cluster, h.count)})
		c.Suggestions = append([]string{"archo log --cluster " + strings.TrimPrefix(h.cluster, "log:")}, f.Suggestions...)
		fs[i] = &c
	}
	return fs
}

func logStamp(path string) string {
	if fi, err := os.Stat(path); err == nil {
		return fmt.Sprintf("%d.%d", fi.Size(), fi.ModTime().UnixNano())
	}
	return ""
}
