package app

import (
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/analyze"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Attribution roles (06 §3).
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
	// Near: the log messages just before and after the first occurrence ("Player.log:<line>
	// <message>"), the context agents otherwise grep the raw log for (G7 traces).
	Near    []string
	Finding *finding.Finding
	// From the companion log (06 §7): its file name, the first occurrence's time and
	// level; OnlyCompanion when Player.log does not have the entry at all.
	Companion     string    `json:",omitempty"`
	At            time.Time `json:",omitzero"`
	Level         string    `json:",omitempty"`
	OnlyCompanion bool      `json:",omitempty"`
	// ConsequenceOf: the ID of the cluster whose failed def file this one follows from;
	// Consequences and ConsequentLines count them on that cluster (06 §3).
	ConsequenceOf                 string `json:",omitempty"`
	Consequences, ConsequentLines int    `json:",omitempty"`
	// providerOff: a missing reference whose def an installed mod defines but is not
	// enabled. That mod is the reason, not a patcher on the stack (06 §3).
	providerOff bool
	// missingTypes: types the log reports missing that no installed mod defines; the XML
	// naming them is searched for (refQuery.class, 06 §2).
	missingTypes []string
}

// LogReport is an analysed log.
type LogReport struct {
	HasPhases bool          `json:",omitempty"`
	Phases    []*logs.Phase `json:",omitempty"`
	Path      string
	Label     string // "Player.log", "Player-prev.log" or "<text>"
	ModTime   time.Time
	Lines     int
	Version   string
	// AnalysedAs is the major.minor the log was attributed against when it differs from
	// the environment's (EnvMM): the log's own version (06 §4).
	AnalysedAs, EnvMM string
	Clusters          []*LogCluster
	// LoadedSaves are the saves the run loaded ("Loading game from file <name> with mods"),
	// in order, each once (06 §3): the output says which are not in Saves/.
	LoadedSaves []string `json:",omitempty"`
	// Unanalysed counts the enabled mods (Core and DLC aside) whose content and assemblies
	// had not been analysed for the attributed version: their types and Harmony IDs could
	// not name them (06 §2).
	Unanalysed int `json:",omitempty"`
	// Companion is the companion log read with it (06 §7); CompanionBad its bad lines.
	Companion    string `json:",omitempty"`
	CompanionBad int    `json:",omitempty"`
	// Cursor marks how much of the log and its companion this report covers: get_log
	// since=<cursor> lists what is new after it (06 §6). Empty for pasted text.
	Cursor string `json:",omitempty"`
}

// LogPaths returns usable current and previous logs (06 §4). Check both locations,
// preferring this game's layout, independently: a foreign Player.log must not hide an
// owned Player-prev.log. --game-version changes analysis, never game ownership.
func (a *App) LogPaths(st *LoadoutState) (cur, prev string) {
	root := st.Ix.Env.UserDataRoot
	if root == "" {
		return "", ""
	}
	dirs := []string{root, filepath.Join(root, "Logs")}
	if mm := env.MajorMinor(st.Ix.Env.GameVersion); mm != "" && !env.VersionLess(mm, "1.7") {
		dirs[0], dirs[1] = dirs[1], dirs[0]
	}
	pick := func(name string) string {
		for _, dir := range dirs {
			p := filepath.Join(dir, name)
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() && a.CheckLogOwner(st, p) == nil {
				return p
			}
		}
		return ""
	}
	cur, prev = pick("Player.log"), pick("Player-prev.log")
	return cur, prev
}

type logCacheEntry struct {
	key string
	rep *LogReport
}

// DropLogCaches forgets the parsed log report and the problem set built with it; the next
// read takes the stored report from the database again (the GUI calls it when the player
// leaves Can it run or Game log, 17 §7.3).
func (a *App) DropLogCaches() {
	a.logCache = nil
	a.prCache, a.prKey = nil, ""
}

// DropPlanProblems forgets every plan's problem set (PlanProblems, the home page's counts).
func (a *App) DropPlanProblems() { a.planPS = nil }

// ErrLogNotCached: a cache-only read found no stored report for the log as it is now.
var ErrLogNotCached = errors.New("log not analysed yet")

// logCacheVersion invalidates stored reports when attribution changes.
// 7: both the companion log (dev, 4) and the G1/G7 attribution rules (master, 6); 8: no
// nearby messages for entries only the companion log has; 9: observed phase boundaries
// and counts from both logs; 10: a left-out provider of a missing def before patchers;
// 11: texture and assembly providers in the shared mod folders (Store.InstanceFiles); 12: known
// noise (logs.ClassNoise) and its note; "assemblies not analysed yet" instead of "no enabled mod
// defines it" (Unanalysed); 13: a missing type's XML referrer, nearest types, a same-name DLL that
// loads first; consequences of a def file that failed to load (06 §2, §3); stack-less
// "Tried to" errors, the game-error class and the lord rule; 14: Unity-format stack frames,
// HugsLib upload headers skipped (06 §1); 15: XML Extensions blocks in 1.7 logs, any
// "[Mod] Ns.PatchOperation…(" block line a patch failure; 16: a pawn's name in "Exception
// ticking …" errors normalised, one cluster for all pawns; 17: a texture-missing cluster says
// "rendering only", the saves the run loaded are recorded (LoadedSaves), and a mentioned mod is
// no subject of the cluster's finding (06 §3).
const logCacheVersion = 17

// AnalyzeLog parses and attributes a log file. The report is cached in memory and in the
// database, keyed by path, size and mtime plus the attribution inputs (logInputKey), so a
// new process does not parse and attribute an unchanged log again.
func (a *App) AnalyzeLog(path string) (*LogReport, error) { return a.analyzeLog(path, false) }

// CachedLog returns the stored report of a log without parsing it (ErrLogNotCached if the
// log or the attribution inputs changed since it was stored).
func (a *App) CachedLog(path string) (*LogReport, error) { return a.analyzeLog(path, true) }

func (a *App) analyzeLog(path string, cacheOnly bool) (*LogReport, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	if err := a.CheckLogOwner(st, path); err != nil {
		return nil, err
	}
	compPath := CompanionFor(path)
	key := fmt.Sprintf("%s|%d|%d|%s|%s|%s", path, fi.Size(), fi.ModTime().UnixNano(), a.Store.ChangeKey(), a.GameVersion, companionKey(compPath))
	if a.logCache != nil && a.logCache.key == key {
		return a.logCache.rep, nil
	}
	rep := &LogReport{Path: path, Label: filepath.Base(path), ModTime: fi.ModTime()}
	var lf *logs.File
	var mark logMark
	row, _ := a.Store.LogCache(path)
	fresh := row != nil && row.Size == fi.Size() && row.MTime == fi.ModTime().UnixNano()
	switch {
	case fresh:
		rep.Version = row.Version
	case cacheOnly:
		return nil, ErrLogNotCached
	default:
		if lf, mark, err = readLog(path, fi.Size()); err != nil {
			return nil, err
		}
		rep.Version = lf.Version
	}
	st, restore, err := a.logState(st, rep)
	if err != nil {
		return nil, err
	}
	defer restore()
	ik := a.logInputKey(st) + "|" + companionKey(compPath)
	if fresh && row.InputKey == ik {
		var stored LogReport
		if err := json.Unmarshal(row.Report, &stored); err == nil {
			stored.Path, stored.Label, stored.ModTime = rep.Path, rep.Label, rep.ModTime
			a.logCache = &logCacheEntry{key: key, rep: &stored}
			return &stored, nil
		}
	}
	if cacheOnly {
		return nil, ErrLogNotCached
	}
	if lf == nil {
		if lf, mark, err = readLog(path, fi.Size()); err != nil {
			return nil, err
		}
	}
	rep.Lines = lf.Lines
	fails, _ := analyze.ParseLogFailures(path)
	var comp *logs.Companion
	cur := LogCursor{Size: mark.size, Sum: mark.sum}
	if compPath != "" {
		var cm logMark
		comp, cm = readCompanion(compPath, -1)
		if comp != nil {
			rep.Companion, rep.CompanionBad = filepath.Base(compPath), comp.BadLines
			cur.CompSize, cur.CompSum = cm.size, cm.sum
		}
	}
	rep.Cursor = cur.String()
	only := companionOnly(lf, comp)
	if err := a.attribute(st, rep, lf, fails, comp); err != nil {
		return nil, err
	}
	for _, lc := range rep.Clusters {
		if only[lc.First] {
			lc.OnlyCompanion = true
			lc.Details = append(lc.Details, "only in "+filepath.Base(compPath)+": Player.log does not have it (the game may have stopped logging)")
		}
	}
	a.logCache = &logCacheEntry{key: key, rep: rep}
	if b, err := json.Marshal(rep); err == nil {
		a.Store.PutLogCache(&store.LogCache{Path: path, Size: fi.Size(), MTime: fi.ModTime().UnixNano(), Version: rep.Version, InputKey: ik, Report: b})
	}
	return rep, nil
}

// logInputKey fingerprints what attribution reads besides the log: the game version, the
// analysis run, the enabled list and every installed mod (identity, snapshot, names, folder).
func (a *App) logInputKey(st *LoadoutState) string {
	h := sha1.New()
	fmt.Fprintf(h, "v%d|%s|%s|", logCacheVersion, a.GameVersion, st.GameMM())
	if s, err := a.Analysis(st); err == nil && s.Run != nil {
		fmt.Fprintf(h, "a%d|", s.Run.ID)
	}
	for _, e := range st.List {
		fmt.Fprintf(h, "%s;", e.Instance)
	}
	var mods []string
	for _, m := range st.Ix.Mods {
		if m.TombstoneAt.IsZero() {
			mods = append(mods, fmt.Sprintf("%s %d %s %s %s %s", m.ID, m.SnapID, m.Name, m.Alias, m.PackageID, gamePath(st.Ix, m)))
		}
	}
	sort.Strings(mods)
	for _, m := range mods {
		fmt.Fprintf(h, "|%s", m)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
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
	return rep, a.attribute(st, rep, lf, nil, nil)
}

// LogMM is the game major.minor a log was written by ("" if it does not say).
func LogMM(version string) string { return env.MajorMinor(version) }

// logState switches attribution to the log's own game version when it differs from the
// environment's and no --game-version was given (06 §4). restore undoes it.
func (a *App) logState(st *LoadoutState, rep *LogReport) (*LoadoutState, func(), error) {
	mm := LogMM(rep.Version)
	if a.GameVersion != "" || mm == "" || mm == st.GameMM() {
		return st, func() {}, nil
	}
	prev := a.GameVersion
	rep.AnalysedAs, rep.EnvMM = mm, st.GameMM()
	a.GameVersion = mm
	restore := func() { a.GameVersion = prev; a.Store.MM = st.GameMM() }
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
	defer func() { a.GameVersion = ""; a.Store.MM = st.GameMM() }()
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
	defMemo  map[string][]analyze.DefFact // "n|" / "N|" + name → DefsNamed
	run      *store.Analysis
	fails    map[string]*analyze.PatchFact // FailKey(game path, desc) → fact
	byXPath  map[string][]*analyze.PatchFact
	comp     *logs.Companion     // nil without a companion log
	roMemo   map[string]roUse    // reflectionOnly, once per log
	libMemo  map[string]string   // type → the one DLL file name its owners ship it in
	asmMemo  map[string][]string // assembly base name (lower) → enabled mods loading it
	lf       *logs.File          // the whole log (neighbouring lines)
	// unanalysed: enabled non-official mods without analysed content (LogReport.Unanalysed).
	unanalysed int

	// dllMemo: DLL file name (lower) → the enabled mods' copies (shadowing).
	dllMemo map[string][]*store.AssemblyRow
}

// notAnalysed qualifies a "nothing defines it" detail: when some enabled mods were never
// analysed, their types are unknown, so the absence says nothing about them (06 §2).
func (cx *attrCtx) notAnalysed() string {
	if cx.unanalysed == 0 {
		return ""
	}
	return "assemblies not analysed yet (" + EnabledMods(cx.unanalysed) + ")"
}

// EnabledMods is "1 enabled mod" or "N enabled mods".
func EnabledMods(n int) string {
	if n == 1 {
		return "1 enabled mod"
	}
	return fmt.Sprintf("%d enabled mods", n)
}

// unanalysedMods counts the enabled mods, Core and DLC aside, that have no analysed
// content for the version attributed against.
func (a *App) unanalysedMods(st *LoadoutState) int {
	keys, err := a.Store.ContentKeys()
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range st.List {
		m := st.Ix.ByID[e.Instance]
		if m == nil || strings.HasPrefix(strings.ToLower(m.PackageID), "ludeon.rimworld") {
			continue
		}
		if keys[m.ID] == "" {
			n++
		}
	}
	return n
}

type dirOwner struct {
	dir string // lower case, with trailing separator
	id  string
}

func (a *App) attribute(st *LoadoutState, rep *LogReport, lf *logs.File, fails []analyze.LogFailure, comp *logs.Companion) error {
	rep.HasPhases = lf.HasPhases || comp != nil && comp.HasPhases
	rep.Phases = lf.Phases
	if comp != nil && comp.HasPhases {
		rep.Phases = comp.Phases
		for _, p := range lf.Phases {
			if p.Index <= len(comp.Phases) {
				cp := comp.Phases[p.Index-1]
				if cp.Name == p.Name {
					p.At, p.Save = cp.At, cp.Save
				}
			}
		}
	}
	cx := &attrCtx{a: a, st: st, byName: map[string][]*store.Mod{}, byPkg: map[string][]*store.Mod{}, typeMemo: map[string][]string{}, idMemo: map[string][]string{}, defMemo: map[string][]analyze.DefFact{}, libMemo: map[string]string{}, lf: lf, comp: comp}
	for _, e := range st.List {
		m := st.Ix.ByID[e.Instance]
		if m == nil {
			continue
		}
		// The game prints the About.xml name; an alias is the player's own name for it.
		for _, n := range uniq(strings.ToLower(m.Name), strings.ToLower(m.Alias)) {
			cx.byName[n] = append(cx.byName[n], m)
		}
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
	a.Store.MM = st.GameMM()
	cx.unanalysed = a.unanalysedMods(st)
	rep.Unanalysed = cx.unanalysed
	// Map the multi-line patch-failure records (file + description) onto entries by line.
	failAt := map[string]analyze.LogFailure{}
	for _, f := range fails {
		failAt[f.Mod+"|"+strings.SplitN(f.Desc, "\n", 2)[0]] = f
	}
	var qs []*refQuery
	for _, c := range logs.ClustersHint(lf, logs.HintFrom(comp)) {
		lc := &LogCluster{Cluster: c}
		lc.Near = nearMessages(lf, c.First)
		cx.attributeCluster(lc, failAt)
		if q := referrerQuery(lc); q != nil {
			qs = append(qs, q)
		}
		rep.Clusters = append(rep.Clusters, lc)
	}
	if len(qs) > 0 {
		cx.referrers(qs)
		cx.textureProviders(qs)
	}
	cx.consequences(rep)
	rep.LoadedSaves = logLoadedSaves(lf)
	for _, lc := range rep.Clusters {
		// A missing texture affects drawing only; say so on every such cluster, not only on the
		// one whose own mod's code asks for it (ownArtNote says it in its own words).
		if lc.Class == logs.ClassTexture && !slices.Contains(lc.Details, ownArtNote) {
			lc.Details = append(lc.Details, renderingOnlyNote)
		}
		// Source searches cover the cluster's candidates, not every enabled mod (05 §5.4).
		if rep.HasPhases {
			lc.Phases = logs.CompletePhaseCounts(lc.Phases, lc.Count)
			if lc.Phase == nil {
				lc.Phase = logs.UnknownPhase
			}
		}
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
// MCP source searches default to them (05 §5.4).
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
				if logs.Informational(lc.Class) {
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

// Owners are the candidates that may carry the cluster as their own problem: every role
// but mentioned. A mod only named in the message or a lower frame is not blamed by the
// problem list, Mod details or a test run's mods (06 §3).
func (lc *LogCluster) Owners() []*Attribution {
	var out []*Attribution
	for _, at := range lc.Attr {
		if at.Role != RoleMentioned {
			out = append(out, at)
		}
	}
	return out
}

// ListRank orders the cluster list by how much a row matters (06 §3): errors that stop code
// (exception, harmony-stack, xml-load) first, then the other warnings, then a missing
// texture, which affects drawing only, then informational clusters. Cluster IDs keep
// their order of first appearance; only the list is sorted.
func (lc *LogCluster) ListRank() int {
	switch {
	case logSeverity[lc.Class] == finding.Error:
		return 0
	case lc.Class == logs.ClassTexture:
		return 2
	case logSeverity[lc.Class] == finding.Warning:
		return 1
	}
	return 3
}

// ListOrder is the clusters in list order: ListRank, then order of first appearance.
func ListOrder(cs []*LogCluster) []*LogCluster {
	out := slices.Clone(cs)
	sort.SliceStable(out, func(i, j int) bool { return out[i].ListRank() < out[j].ListRank() })
	return out
}

// whyProviderOff explains a likely cluster led by a mod left out of the list.
const whyProviderOff = "a mod that defines it is installed but not enabled"

// order sorts the candidates by role (06 §3). A save or def that names a def of a mod
// left out of the list: that mod comes before whoever only patched the code that read it
// (VM run 2026-10-28: Vanilla Expanded Framework's Pawn.ExposeData patch was blamed for
// missing hair).
func (lc *LogCluster) order() {
	rank := func(role string) int {
		if lc.providerOff && role == RoleDependency {
			return roleRank[RolePatcher]
		}
		if lc.providerOff && role == RolePatcher {
			return roleRank[RoleDependency]
		}
		return roleRank[role]
	}
	sort.SliceStable(lc.Attr, func(i, j int) bool { return rank(lc.Attr[i].Role) < rank(lc.Attr[j].Role) })
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

// typeOrDeclaring is types, falling back to the declaring type of a Type.Method name.
func (cx *attrCtx) typeOrDeclaring(t string) []string {
	ids := cx.types(t)
	if len(ids) == 0 {
		if i := strings.LastIndexByte(t, '.'); i > 0 {
			ids = cx.types(t[:i])
		}
	}
	return ids
}

func (cx *attrCtx) types(t string) []string {
	if r, ok := cx.typeMemo[t]; ok {
		return r
	}
	var out []string
	files := map[string]bool{}
	if owners, err := cx.a.Store.TypeOwners(t); err == nil {
		for _, o := range owners {
			if cx.st.Position(o[0]) > 0 || cx.st.Ix.ByID[o[0]] != nil {
				out = append(out, o[0])
				files[strings.ToLower(path.Base(filepath.ToSlash(o[1])))] = true
			}
		}
	}
	out = uniqStr(out)
	cx.typeMemo[t] = out
	if len(files) == 1 {
		for f := range files {
			cx.libMemo[t] = f
		}
	}
	return out
}

// library is the DLL file name when a type comes from a library that several enabled mods
// ship under the same name (LunarFramework.dll in every Lunar mod): a frame in it does not
// say which of them is at fault (log corpus).
func (cx *attrCtx) library(t string) string {
	n := 0
	for _, id := range cx.types(t) {
		if cx.st.Position(id) > 0 {
			n++
		}
	}
	if n < 2 {
		return ""
	}
	return cx.libMemo[t]
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

// folderOwner maps a path from another machine (a shared log) to an active mod by its folder:
// …\workshop\content\294100\<id>\… or …\Mods\<folder>\… ([Rimworld_dir]\Mods\… after HugsLib's
// redaction).
func (cx *attrCtx) folderOwner(path string) (string, string) {
	m := modFolderRe.FindStringSubmatch(path)
	if m == nil {
		return "", ""
	}
	for _, e := range cx.st.List {
		if mod := cx.st.Ix.ByID[e.Instance]; mod != nil && strings.EqualFold(filepath.Base(filepath.FromSlash(mod.Rel)), m[1]) {
			return mod.ID, filepath.ToSlash(m[2])
		}
	}
	return "", ""
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
	bracketRe    = regexp.MustCompile(`^(?:<color=[^>]*>)?\[([^\]]{2,80})\](?:</color>)?`)
	xmlExtRe     = regexp.MustCompile(`^Verse\.PatchOperation\w*\(xpath="(.*)"\): Failed`)
	winPathRe    = regexp.MustCompile(`[A-Za-z]:[\\/][^"'<>|\r\n]*?\.(?:xml|dll|png|txt)`)
	xrefRe       = regexp.MustCompile(`No ([\w.]+) named (\S+) found to give to (\S+)|cross-reference to ([\w.]+) named (\S+)|reference to ([\w.]+) named (\S+)`)
	forDefRe     = regexp.MustCompile(` for def '([^']+)'`)
	ctxDefNameRe = regexp.MustCompile(`(?:Context|Full node): [\s\S]*?<defName>([^<\s]+)</defName>`)
	parentRe     = regexp.MustCompile(`Could not find parent node named "([^"]+)"`)
	thinkKeyRe   = regexp.MustCompile(`^Config error in (\S+): Two ThinkNodes have the same unique save key`)
	ctxNameRe    = regexp.MustCompile(`Context: <\w+[^>]*\bName="([^"]+)"`)
	// A bare name counts only before " from node": the game's wording for a Class="…" it cannot
	// resolve (VM round 2026-11-05: Class="PatchOperationConditionl" in a patch).
	missingTypeRe = regexp.MustCompile(`Could not find (?:a )?type (?:named )?(?:([A-Za-z_][\w.+]*\.[\w+]+)|([A-Za-z_]\w*) from node)|expected class '([^']+)'`)
	blockRe       = regexp.MustCompile(`^\[(.+?)\] Verse\.PatchOperation`)
	modFolderRe   = regexp.MustCompile(`(?i)[\\/](?:294100|Mods)[\\/]([^\\/]+)[\\/](.+)$`)
	wanterRe      = regexp.MustCompile(`found to give to ([\w.]+Def) ([^\s(]+)`)
	// "Tried to add pawn X to lord with lord jobA but this pawn is already a member of lord with lord jobB."
	lordAddRe = regexp.MustCompile(`^Tried to add pawn .* to lord with lord job ?\S+ but this pawn is already a member of lord`)
	typeTokRe = regexp.MustCompile(`\b([A-Za-z_]\w*(?:\.[A-Za-z_]\w*)+)\b`)
	// DefDatabase.GetNamed: "Failed to find Verse.ThingDef named X. There are N defs of this type loaded."
	getNamedRe = regexp.MustCompile(`^Failed to find ([\w.]+Def) named ([^\s.]+(?:\.[^\s.]+)*)\. There are`)
	texPathRe  = regexp.MustCompile(`^Could not load Texture2D at '([^']+)'|^Failed to find any textures at (\S+) while constructing`)
	multiRe    = regexp.MustCompile(`^(.+)_(?:north|south|east|west)m?$`)
	asmDepRe   = regexp.MustCompile(`Cannot resolve dependency to assembly '([^',]+)`)
	// "Could not resolve reference to object with loadID X of type T. … curParent=NS.Holder …"
	curParentRe = regexp.MustCompile(`curParent=([A-Za-z_][\w+]*(?:\.[A-Za-z_][\w+]*)+)`)
	loadIDRe    = regexp.MustCompile(`loadID (\S+) of type (\S+?)\.? Was`)
	shaderRe    = regexp.MustCompile(`with Shader '([^']+)' doesn't have a texture property '_MainTex'`)
	fieldErrRe  = regexp.MustCompile(`^XML error: <([\w.]+)>([^<]{0,200})</[\w.]+> doesn't correspond to any field in type`)
)

// xrefName reads the missing def's type and name from a cross-reference, save-load reference
// or DefDatabase.GetNamed message.
func xrefName(msg string) (typ, name string, ok bool) {
	if m := xrefRe.FindStringSubmatch(msg); m != nil {
		return m[1] + m[4] + m[6], m[2] + m[5] + m[7], true
	}
	if m := getNamedRe.FindStringSubmatch(msg); m != nil {
		return m[1], m[2], true
	}
	return "", "", false
}

// texturePath is the path of a texture-missing message ("" if it names none).
func texturePath(msg string) string {
	if m := texPathRe.FindStringSubmatch(msg); m != nil {
		return m[1] + m[2]
	}
	return ""
}

// gameAssembly reports assemblies that ship with the game (Unity, Mono/.NET, RimWorld itself).
func gameAssembly(name string) bool {
	n := strings.ToLower(name)
	for _, p := range []string{"system", "unityengine", "unity.", "mono.", "microsoft.", "assembly-csharp"} {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return n == "mscorlib" || n == "netstandard"
}

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
	// 1. "[Mod Name] …" prefix: the mod logs its own problem. An XML Extensions block names
	// the mod in full ("[[FSF] Vanilla Bionics Expansion] Verse.PatchOperation…").
	if m := blockRe.FindStringSubmatch(e.Message); m != nil {
		for _, mod := range cx.byName[strings.ToLower(m[1])] {
			lc.add(mod.ID, RoleSource, fmt.Sprintf("log message prefix [%s]", m[1]))
		}
	} else if m := bracketRe.FindStringSubmatch(e.Message); m != nil {
		for _, mod := range cx.byName[strings.ToLower(m[1])] {
			lc.add(mod.ID, RoleSource, fmt.Sprintf("log message prefix [%s]", m[1]))
		}
	}
	// 2. File paths in the message or its continuation lines.
	texts := append([]string{e.Message}, e.Extra...)
	for _, t := range texts {
		// "absFilePath: [Rimworld_dir]\Mods\<folder>\Sounds\x.mp3" after a failed content load.
		if p, ok := strings.CutPrefix(t, "absFilePath: "); ok {
			if id, rel := cx.dirOwner(p); id != "" {
				lc.add(id, RoleSource, "log file "+rel+" (the file the game failed to load)")
			} else if id, rel := cx.folderOwner(p); id != "" {
				lc.add(id, RoleSource, "log file "+rel+" (by mod folder; the file the game failed to load)")
			}
			continue
		}
		for _, p := range winPathRe.FindAllString(t, -1) {
			if id, rel := cx.dirOwner(p); id != "" {
				lc.add(id, RoleSource, "log file "+rel)
			} else if id, rel := cx.folderOwner(p); id != "" {
				lc.add(id, RoleSource, "log file "+rel+" (by mod folder)")
			}
		}
	}
	switch lc.Class {
	case logs.ClassPatchFailed:
		cx.patchFailure(lc, failAt)
	case logs.ClassXrefMissing:
		if typ, name, ok := xrefName(e.Message); ok {
			short := typ[strings.LastIndexByte(typ, '.')+1:]
			save := strings.HasPrefix(e.Message, "Could not load reference")
			lc.Details = append(lc.Details, "missing: def:"+short+"/"+name)
			if save {
				lc.Details = append(lc.Details, "raised while loading a save: the save references a def no active mod provides")
			} else if strings.HasPrefix(e.Message, "Failed to find ") {
				lc.Details = append(lc.Details, "asked for by name at run time (DefDatabase.GetNamed), usually from a mod's code: the def is not loaded")
			}
			defs, _ := cx.defsNamed(name, false)
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
					lc.providerOff = true
				}
			}
			if len(prov) == 0 {
				lc.Details = append(lc.Details, "providers: none among installed mods")
				if save {
					// The save names the def, not the mod that defined it: nothing to attribute.
					lc.Details = append(lc.Details, "the mod that defined it was removed, or it renamed or removed the def; the game drops the reference (usually harmless once the game is saved again)")
				}
			} else {
				lc.Details = append(lc.Details, "providers: "+strings.Join(uniqStr(prov), ", "))
			}
			lc.next("archo def "+short+"/"+name, fmt.Sprintf("archo files search %q", name))
			// "… found to give to Verse.ThingDef RKU_Hammer": the def that wanted the missing one.
			// Whoever defines it holds the reference (log corpus: a direct rule beats searching
			// every file for the missing name).
			if w := wanterRe.FindStringSubmatch(e.Message); w != nil {
				wshort := w[1][strings.LastIndexByte(w[1], '.')+1:]
				ws, _ := cx.defsNamed(w[2], false)
				for _, d := range ws {
					if d.Type == wshort && cx.st.Position(d.Instance) > 0 && !cx.official(d.Instance) {
						lc.add(d.Instance, RoleSource, fmt.Sprintf("defines %s %s, which wants %s %s (%s:%d)", wshort, w[2], short, name, d.File, d.Line))
					}
				}
			}
		}
	case logs.ClassTexture:
		// "Could not load Texture2D at '…' for def 'X'": the mod defining X points at it.
		if m := forDefRe.FindStringSubmatch(e.Message); m != nil {
			cx.definedBy(lc, m[1], false, "names the missing file")
		}
		if p := texturePath(e.Message); p != "" {
			lc.Details = append(lc.Details, "missing: texture "+p)
			lc.next(fmt.Sprintf("archo files search %q", p))
		}
	case logs.ClassSave:
		cx.saveLoad(lc)
	case logs.ClassXMLLoad:
		// "XML error: … Context: <ThingDef …><defName>X</defName>…": the def's XML is at fault.
		texts := strings.Join(append([]string{e.Message}, e.Extra...), "\n")
		// A missing parent provided by an installed, disabled mod is a missing dependency.
		if m := parentRe.FindStringSubmatch(e.Message); m != nil {
			ds, _ := cx.defsNamed(m[1], true)
			for _, d := range ds {
				if cx.st.Position(d.Instance) == 0 && ix.ByID[d.Instance] != nil {
					lc.add(d.Instance, RoleDependency, fmt.Sprintf("provides the abstract def %s (%s:%d) but is not enabled", m[1], d.File, d.Line))
					lc.next("archo enable " + ix.Ref(d.Instance))
				}
			}
		}
		// Duplicate ThinkNode keys come from patches to the think tree, not from the tree itself.
		if m := thinkKeyRe.FindStringSubmatch(e.Message); m != nil && cx.run != nil {
			pf, _ := cx.a.Store.PatchFacts(cx.run.ID, store.PatchFilter{Target: "ThinkTreeDef/" + m[1]})
			for _, p := range pf {
				if cx.st.Position(p.Instance) > 0 && !cx.official(p.Instance) {
					lc.add(p.Instance, RolePatcher, fmt.Sprintf("patches ThinkTreeDef %s at %s:%d", m[1], p.File, p.Line))
				}
			}
		}
		if m := ctxDefNameRe.FindStringSubmatch(texts); m != nil {
			cx.definedBy(lc, m[1], false, "is the def the XML error is in")
		} else if m := ctxNameRe.FindStringSubmatch(texts); m != nil {
			cx.definedBy(lc, m[1], true, "is the abstract def the XML error is in")
		}
	case logs.ClassNoise:
		if n := logs.KnownNoise(e.Message); n != nil {
			lc.Details = append(lc.Details, n.Note)
		}
	case logs.ClassMaterial:
		if m := shaderRe.FindStringSubmatch(e.Message); m != nil {
			// The shader lives in a mod's asset bundle (compressed) or in the game: its name is not
			// in any file RimArcho reads, so the message names no mod (log corpus).
			lc.Details = append(lc.Details, "shader "+m[1]+" has no _MainTex: a texture was assigned to a material made with it. The shader comes from the game or a mod's asset bundle, which file contents do not reveal; usually harmless unless something renders wrong")
			break
		}
		lc.next(`archo files search "uiIconColor"`, `archo files search "uiIconPath"`)
		lc.Checklist = append(lc.Checklist, "a _MainTex/ResolveIcon error is XML-side first: look for buildable defs with uiIconColor but no uiIconPath, uiIconPathsStuff or graphicData/texPath, including abstract bases")
	}
	// "Cannot resolve dependency to assembly 'X'" (FileNotFoundException, ReflectionOnly APIs).
	if m := asmDepRe.FindStringSubmatch(e.Message + "\n" + strings.Join(e.Extra, "\n")); m != nil {
		cx.missingAssembly(lc, m[1])
	}
	// Assemblies whose types could not be listed; type references into Harmony 1.x.
	skipProviders := cx.assemblyRules(lc)
	// Missing types ("Could not find type named NS.T", "expected class 'NS.T' in assembly 'A'"):
	// an installed mod that provides the type but is not enabled is a missing dependency.
	for _, m := range missingTypeRe.FindAllStringSubmatch(e.Message+"\n"+strings.Join(e.Extra, "\n"), -1) {
		if skipProviders {
			break
		}
		t := m[1] + m[2] + m[3]
		ids := cx.types(t)
		if len(ids) == 0 && !slices.Contains(lc.missingTypes, t) {
			lc.missingTypes = append(lc.missingTypes, t)
			if near := cx.nearTypes(t); len(near) > 0 {
				lc.Details = append(lc.Details, "did you mean: "+strings.Join(near, ", "))
			}
		}
		for _, id := range ids {
			if cx.st.Position(id) == 0 {
				lc.add(id, RoleDependency, "provides type "+t+" but is not enabled")
				lc.next("archo enable " + ix.Ref(id))
			} else if sh, why, prov := cx.shadowing(t, id); sh != "" {
				lc.add(sh, RoleSource, why)
				lc.add(id, RoleMentioned, prov)
				lc.next("archo mod " + ix.Ref(sh))
			} else {
				lc.add(id, RoleMentioned, "provides type "+t+" and is enabled (a version mismatch?)")
			}
		}
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
				d := fmt.Sprintf("harmony owner %q on %s: no installed mod matches", p.Owner, target)
				if na := cx.notAnalysed(); na != "" {
					d += " (" + na + ")"
				}
				lc.Details = append(lc.Details, d)
			}
			lc.next("archo harmony --target " + strings.ReplaceAll(target, "+", "."))
		}
		if logs.GameNamespace(fr.Type) {
			continue
		}
		ids := cx.types(fr.Type)
		if lib := cx.library(fr.Type); lib != "" {
			for _, id := range ids {
				lc.add(id, RoleMentioned, fmt.Sprintf("log frame %d %s (library %s, shipped by %d enabled mods)", i+1, target, lib, len(ids)))
			}
			continue
		}
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
		ids := cx.typeOrDeclaring(t)
		if u := logs.Unglue(t); len(ids) == 0 && u != t {
			// "lord jobNs.LordJob_X": the game glued a word to the name.
			if logs.GameNamespace(u) {
				continue
			}
			t, ids = u, cx.typeOrDeclaring(u)
		}
		role := RoleMentioned
		if strings.Contains(e.Message, "instantiating a mod of type "+t) || strings.Contains(e.Message, "static constructor of "+t) ||
			// Lord.AddPawn: the code adding the pawn is the lord's own (its LordJob or LordToil).
			// The other lord, the one the pawn is already in, did nothing wrong (Progression:
			// Education's class lord in 9 corpus logs, against five different other lords).
			lordAddRe.MatchString(e.Message) && (strings.Contains(e.Message, "to lord with lord job"+t+" ") || strings.Contains(e.Message, "to lord with lord job "+t+" ")) {
			role = RoleSource
		}
		for _, id := range ids {
			lc.add(id, role, "log message names type "+t)
			if role == RoleSource {
				lc.next(fmt.Sprintf("archo source %s --type %s", ix.Ref(id), outerType(t)))
			}
		}
	}
	// 5. Callers of failing code, HugsLib's log upload (logcallers.go).
	cx.callerRules(lc)
	if cx.comp != nil {
		cx.companion(lc)
	}
	lc.order()
	switch {
	case len(lc.Attr) > 0 && lc.Attr[0].Role == RoleDependency && lc.providerOff:
		lc.Confidence, lc.Why = finding.Likely, whyProviderOff
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

// definedBy makes the enabled mods (not Core or a DLC) that define a def the source of a
// cluster; byName looks the name up as an abstract Name="…".
func (cx *attrCtx) definedBy(lc *LogCluster, name string, byName bool, why string) {
	ds, _ := cx.defsNamed(name, byName)
	for _, d := range ds {
		if cx.st.Position(d.Instance) > 0 && !cx.official(d.Instance) {
			lc.add(d.Instance, RoleSource, fmt.Sprintf("defines %s %s (%s:%d), which %s", d.Type, name, d.File, d.Line, why))
		}
	}
}

// defsNamed is Store.DefsNamed, once per name and attribution: clusters of one log name
// the same defs again and again.
func (cx *attrCtx) defsNamed(name string, byName bool) ([]analyze.DefFact, error) {
	k := "n|" + name
	if byName {
		k = "N|" + name
	}
	if ds, ok := cx.defMemo[k]; ok {
		return ds, nil
	}
	ds, err := cx.a.Store.DefsNamed(name, byName)
	if err == nil {
		cx.defMemo[k] = ds
	}
	return ds, err
}

// official reports Core and the DLCs.
func (cx *attrCtx) official(inst string) bool {
	m := cx.st.Ix.ByID[inst]
	return m != nil && strings.HasPrefix(strings.ToLower(m.PackageID), "ludeon.rimworld")
}

func hasSource(lc *LogCluster) bool {
	for _, at := range lc.Attr {
		if at.Role == RoleSource {
			return true
		}
	}
	return false
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
	byXPath := func() {
		// XML Extensions logs each failure as Verse.PatchOperationX(xpath="…"): Failed …, in a
		// block after "[Mod] " (logs.Parser).
		body := e.Message
		if i := strings.Index(body, "] Verse.PatchOperation"); i >= 0 {
			body = body[i+2:]
		}
		if m := xmlExtRe.FindStringSubmatch(body); m != nil {
			for _, p := range cx.byXPath[m[1]] {
				lc.add(p.Instance, RoleSource, fmt.Sprintf("patch %s:%d %s (matched by XPath)", p.File, p.Line, p.Class))
				lc.Details = append(lc.Details, fmt.Sprintf("simulation: %s:%d fails (patch.no-match: %s)", p.File, p.Line, strings.Join(p.Reasons, ", ")))
				lc.next("archo patches --mod " + cx.st.Ix.Ref(p.Instance) + " --failed")
			}
		}
	}
	if !ok || lf.File == "" {
		byXPath()
		return
	}
	id, rel := cx.dirOwner(lf.File)
	if id == "" {
		// A shared log's path: match the mod folder instead (the simulation's failure keys are
		// local paths, so only the file is evidence here).
		if fid, frel := cx.folderOwner(lf.File); fid != "" {
			lc.add(fid, RoleSource, "patch file "+frel+" (by mod folder)")
			lc.Details = append(lc.Details, "the failed operation is in this mod's file: the mod stays the source even when the simulation of the installed copy does not fail (the player's copy, list or order may differ); the reason is what remains to find")
			lc.next("archo patches --mod " + cx.st.Ix.Ref(fid) + " --failed")
		} else {
			byXPath()
		}
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
			lc.Details = append(lc.Details, "simulation: this operation does not fail now (the mod or the Loadout changed since the log); the failed operation is still this mod's, so it stays the source")
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

var logSeverity = map[string]string{
	logs.ClassException: finding.Error, logs.ClassHarmony: finding.Error, logs.ClassXMLLoad: finding.Error,
	logs.ClassPatchFailed: finding.Warning, logs.ClassXrefMissing: finding.Warning, logs.ClassMaterial: finding.Warning,
	logs.ClassTexture: finding.Warning, logs.ClassSave: finding.Warning, logs.ClassGameError: finding.Warning,
	logs.ClassModReport: finding.Info, logs.ClassOther: finding.Info, logs.ClassNoise: finding.Info,
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
		Claim: fmt.Sprintf("log:%s x%d: %s%s", lc.ID, lc.Count, msg, logs.PhaseSummary(lc.Phases))}
	for _, at := range lc.Attr {
		// A mod only named by the message or a lower frame stays in the evidence, not among the
		// subjects: the cluster is not its problem (Mod details, test runs).
		if at.Role != RoleMentioned {
			f.Subjects = append(f.Subjects, finding.Inst(at.Instance))
		}
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

// SelectedLog is the log whose clusters `problems` includes ("" if none) (16 §2).
func (a *App) SelectedLog(st *LoadoutState) string {
	return a.Store.Meta("log.selected:" + st.Ix.Env.ID)
}

// SelectLog makes problems include a log's clusters and records its mtime as seen.
func (a *App) SelectLog(st *LoadoutState, path string) error {
	if err := a.CheckLogOwner(st, path); err != nil {
		return err
	}
	if err := a.Store.SetMeta("log.selected:"+st.Ix.Env.ID, path); err != nil {
		return err
	}
	if fi, err := os.Stat(path); err == nil {
		return a.Store.SetMeta("log.seen:"+st.Ix.Env.ID, fmt.Sprint(fi.ModTime().Unix()))
	}
	return nil
}

// NewLog reports whether the current Player.log is newer than the last launch and the
// last log the player looked at (16 §5).
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

// LiveLog is the log the current environment's running game writes ("" when none runs):
// this run, which is not a last run until the game exits (06 §4).
func (a *App) LiveLog(st *LoadoutState) string {
	if st == nil || st.Ix == nil || st.Ix.Env == nil {
		return ""
	}
	r := a.probe()(st.Ix.Env.GameRoot)
	if r == nil {
		return ""
	}
	return a.RunLog(st, a.WhatRuns(st.Ix.Env, r))
}

// sameLog: two log paths name one file ("" names none).
func sameLog(a, b string) bool { return a != "" && b != "" && env.PathKey(a) == env.PathKey(b) }

// ProblemLog is the log whose clusters join the problems (06 §4): the
// selected one, or the current Player.log when that is newer. While the game writes
// Player.log, the last run is the one it moved to Player-prev.log: the run in progress is
// listed as this run, not again as the last (VM run 2026-10-28).
func (a *App) ProblemLog(st *LoadoutState) string {
	sel := a.SelectedLog(st)
	foreign := sel != "" && a.CheckLogOwner(st, sel) != nil
	if foreign {
		sel = ""
	}
	live := ""
	if !a.LazyLog || sel != "" || foreign {
		live = a.LiveLog(st)
	}
	if a.LazyLog {
		if foreign {
			cur, prev := a.LogPaths(st)
			if cur != "" && !sameLog(cur, live) {
				return cur
			}
			return prev
		}
		if !sameLog(sel, live) {
			return sel
		}
		// the game writes the selected log: the last run is the one it moved aside, whose
		// report the refresh during this run stored (the sidebar's count reads it)
		_, prev := a.LogPaths(st)
		return prev
	}
	if sameLog(sel, live) {
		sel = ""
	}
	cur, prev := a.LogPaths(st)
	if sameLog(cur, live) {
		cur = prev
	}
	if cur == "" {
		cur = prev
	}
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

// LogRun is a log whose clusters join the problems, the run it belongs to, and whether it
// still describes the analysed plan (06 §4). A stale log's problems are listed apart and
// not counted.
type LogRun struct {
	Phases map[string][]logs.PhaseCount `json:",omitempty"` // finding key → observed counts
	// Marks are the run's phase boundaries in order (06 §7), nil without markers.
	Marks       []*logs.Phase `json:"-"`
	Path, Label string
	Version     string
	ModTime     time.Time
	MM, EnvMM   string // the log's and the environment's major.minor
	// Source is the launch (or, without one, the ModsConfig.xml write) the log maps to.
	Source *RunSource
	// The stale reasons (06 §4 new rule):
	OtherGame bool   // written by another game version
	OtherRun  bool   // its run is not the analysed plan: another plan, environment or a pack
	OtherEnv  string // the other environment's name, when that is the reason
	Changed   bool   // the plan's enabled list changed since that run
	Unknown   bool   // no launch or write record matches: the source is unknown
	// UnknownGame: with Unknown, the game folder the log's header names when no
	// environment has it (UnregisteredLogGame).
	UnknownGame string `json:",omitempty"`
	// Pending: a cache-only problem list (status, mod) found no stored report; the log's
	// problems are not in the list until a log or problems command analyses it.
	Pending bool
}

// Stale reports whether the log's problems describe another run or an earlier state. A log
// of this game whose run no record matches (a Steam launch before RimArcho ever wrote the
// list) is not stale: it is the only evidence there is, and leaving it out made status and
// list_problems say "0 error" over a log with errors (MCP review round 2 P25). A log whose
// header names a game folder no environment has stays stale.
func (r *LogRun) Stale() bool {
	return r.OtherGame || r.OtherRun || r.Changed || r.Unknown && r.UnknownGame != ""
}

func (a *App) logRun(st *LoadoutState, path, version string, mod time.Time) *LogRun {
	r := &LogRun{Path: path, Label: filepath.Base(path), Version: version, ModTime: mod, MM: LogMM(version), EnvMM: st.GameMM()}
	a.judge(st, r)
	return r
}

// LogRunOf judges a log file against st's plan without analysing it (the log page's
// headers and the per-plan rows).
func (a *App) LogRunOf(st *LoadoutState, path string) *LogRun {
	fi, err := os.Stat(path)
	if err != nil {
		return nil
	}
	return a.logRun(st, path, LogVersionOf(path), fi.ModTime())
}

// logFindings returns the problem log's clusters as findings (informational ones left out),
// the methods its error clusters pass through, and the log run. With LogCacheOnly the log
// is not parsed: a log without a stored report comes back Pending.
func (a *App) logFindings(st *LoadoutState) ([]*finding.Finding, map[string]hotMethod, *LogRun) {
	path := a.ProblemLog(st)
	if path == "" {
		return nil, nil, nil
	}
	var rep *LogReport
	var err error
	if a.LogCacheOnly {
		rep, err = a.CachedLog(path)
	} else {
		rep, err = a.AnalyzeLog(path)
	}
	if err != nil {
		fi, serr := os.Stat(path)
		if serr != nil || !errors.Is(err, ErrLogNotCached) {
			return nil, nil, nil
		}
		r := a.logRun(st, path, LogVersionOf(path), fi.ModTime())
		r.Pending = true
		return nil, nil, r
	}
	run := a.logRun(st, path, rep.Version, rep.ModTime)
	if rep.HasPhases {
		run.Phases = map[string][]logs.PhaseCount{}
		run.Marks = rep.Phases
	}
	var out []*finding.Finding
	hot := map[string]hotMethod{}
	for _, lc := range rep.Clusters {
		if logs.Informational(lc.Class) {
			continue
		}
		if lc.Finding != nil {
			if rep.HasPhases {
				run.Phases[lc.Finding.Key] = lc.Phases
			}
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
	return out, hot, run
}

// escalateSkips raises harmony.skip-prefix and harmony.multi-transpiler to warning when their
// method is in a log cluster (05 §4.5): a bool prefix or two transpilers on one method are
// ordinary until a stack trace passes through it.
func escalateSkips(fs []*finding.Finding, hot map[string]hotMethod) []*finding.Finding {
	if len(hot) == 0 {
		return fs
	}
	for i, f := range fs {
		if f.Code != "harmony.skip-prefix" && f.Code != "harmony.multi-transpiler" {
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

func uniq(xs ...string) []string {
	var out []string
	for _, x := range xs {
		if x != "" && !slices.Contains(out, x) {
			out = append(out, x)
		}
	}
	return out
}
