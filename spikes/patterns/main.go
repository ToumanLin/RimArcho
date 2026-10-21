// Spike: error pattern library v0 (docs/research/00 §3 G5, §5 L1 Pattern / Resolution, §8).
//
// Input, all local and private: spikes/hugslogs parsed.jsonl (upload time, mod list with versions)
// and a spikes/corpusrun results file (the clusters of each log with RimArcho's attribution).
// Output: aggregates only — a JSON dataset of patterns and a text report. No gist ids, no mod
// lists; a mod-specific pattern needs at least -minlogs logs from -minsessions sessions.
//
//	go run ./spikes/patterns -parsed <corpus>/hugslogs/parsed.jsonl -results <kb>/results-v3.jsonl \
//	    -out spikes/patterns/patterns-v0.json > report.txt
//
// Two layers (design §3 G5):
//   - generic patterns: hand-written recognition rules over the cluster template, each with a
//     diagnosis route, common causes and fix types; "fallback" rules only name the class;
//   - mod-specific patterns: a template whose clusters name one mod as source in enough logs.
//
// Resolution evidence: repeat uploads (sessions: same game version, package-set Jaccard >= 0.9,
// <= 48 h, as hugslogs stats). For consecutive uploads A -> B, the list change is classified
// (none, order only, 1-3 package or version changes, more = excluded) and every pattern present in
// A is checked in B. Pairs without any change are the control: a runtime error also disappears
// when the player simply did something else in the second session.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/logs"
)

// ---- input ----

type pmod struct {
	Pkg string `json:"pkg"`
	MV  string `json:"mv"`
	V   string `json:"v"`
	OV  string `json:"ov"`
}

// version: override, then About.xml, then Manifest (as hugslogs Mod.Version).
func (m pmod) version() string {
	switch {
	case m.OV != "":
		return m.OV
	case m.V != "":
		return m.V
	}
	return m.MV
}

type pcluster struct {
	Key   string `json:"key"`
	Tpl   string `json:"tpl"`
	Class string `json:"class"`
	Count int    `json:"count"`
}

type precord struct {
	ID       string     `json:"id"`
	Created  string     `json:"created"`
	Status   string     `json:"status"`
	MM       string     `json:"mm"`
	Mods     []pmod     `json:"mods"`
	Clusters []pcluster `json:"clusters"`
}

type rcluster struct {
	Key   string `json:"key"`
	Tpl   string `json:"tpl"`
	Class string `json:"class"`
	Count int    `json:"count"`
	Attr  []struct {
		Pkg  string `json:"pkg"`
		Role string `json:"role"`
	} `json:"attr"`
}

type rresult struct {
	ID       string     `json:"id"`
	Clusters []rcluster `json:"clusters"`
}

// cl is a problem cluster of one log as this spike uses it.
type cl struct {
	tpl, class string
	count      int
	src        []string // source mods (RimArcho attribution)
	gen        *Rule
}

type log1 struct {
	created time.Time
	mods    []pmod
	set     map[string]bool
	mv      map[string]string
	pos     map[string]int
	cls     []*cl
	session int
}

// ---- generic rules ----

// Rule is a generic pattern: a recognition rule and what an agent should do with it.
type Rule struct {
	ID       string   `json:"id"`
	Class    string   `json:"class"` // the cluster class it applies to ("" any)
	Match    string   `json:"match"` // regular expression over the cluster template
	Fallback bool     `json:"fallback,omitempty"`
	Stage    string   `json:"stage"` // load | save | runtime | noise
	Summary  string   `json:"summary"`
	Causes   []string `json:"causes"`
	Route    []string `json:"route"`    // MCP calls, in order
	Fixes    []string `json:"fixes"`    // disable | enable-dependency | reorder | update | local-fix | ignore | new-game
	Workflow string   `json:"workflow"` // covered by the current MCP Instructions: yes | partial | no
	re       *regexp.Regexp
}

var rules = []*Rule{
	// cross-references and def lookups
	{ID: "xref.sound", Class: logs.ClassXrefMissing, Match: `No Verse\.SoundDef named .* found to give to`, Stage: "load",
		Summary: "a def names a SoundDef that no active mod defines; the game uses no sound",
		Causes:  []string{"the sound comes from a DLC or mod that is not active", "the sound was renamed in an update"},
		Route:   []string{"analyze_log cluster=E<n>", "search_files <defName>", "list_problems code=dep"},
		Fixes:   []string{"ignore", "enable-dependency", "update"}, Workflow: "partial"},
	{ID: "xref.def", Class: logs.ClassXrefMissing, Match: `Could not resolve cross-reference`, Stage: "load",
		Summary: "a def references another def by name that no active mod defines",
		Causes:  []string{"the providing mod or DLC is not active (often an optional patch without MayRequire)", "the def was renamed or removed in an update", "a load-order problem when a patch adds the reference"},
		Route:   []string{"analyze_log cluster=E<n> (dependency candidates)", "get_def <name>", "get_patches failed=true", "list_problems code=dep"},
		Fixes:   []string{"enable-dependency", "update", "disable"}, Workflow: "yes"},
	{ID: "xref.getnamed", Class: logs.ClassXrefMissing, Match: `Failed to find \S+ named .* There are`, Stage: "load",
		Summary: "code asked DefDatabase for a def by name that does not exist",
		Causes:  []string{"a C# mod hard-codes a def from a mod or DLC that is not active", "renamed def after an update"},
		Route:   []string{"analyze_log cluster=E<n>", "read_source search=\"<defName>\"", "get_def <name>"},
		Fixes:   []string{"enable-dependency", "update", "disable"}, Workflow: "partial"},
	{ID: "save.defref", Class: logs.ClassXrefMissing, Match: `Could not load reference to \S+ named`, Stage: "save",
		Summary: "the save refers to a def that no longer exists",
		Causes:  []string{"a mod was removed or a def renamed after the save was made"},
		Route:   []string{"analyze_log cluster=E<n>", "find_mods <def prefix>"},
		Fixes:   []string{"ignore", "enable-dependency", "new-game"}, Workflow: "no"},
	// save loading
	{ID: "save.loadid", Class: logs.ClassSave, Match: `Could not resolve reference to object with loadID`, Stage: "save",
		Summary: "the save refers to an object that was not loaded",
		Causes:  []string{"a mod that owned the object was removed or its data changed", "a mod's save data is corrupt"},
		Route:   []string{"analyze_log cluster=E<n> (curParent names the owning type)", "find_mods <namespace of curParent>"},
		Fixes:   []string{"ignore", "new-game", "update"}, Workflow: "no"},
	{ID: "save.thinknode", Class: logs.ClassSave, Match: `Could not find think node with key`, Stage: "save",
		Summary: "the save's AI state refers to think-tree nodes that changed",
		Causes:  []string{"a think-tree patcher was added or removed since the save", "duplicate think-node keys"},
		Route:   []string{"analyze_log cluster=E<n>", "get_patches target=ThinkTreeDef"},
		Fixes:   []string{"ignore"}, Workflow: "no"},
	{ID: "save.not-deep-saved", Class: logs.ClassSave, Match: `is referenced .* but is not deep-saved`, Stage: "save",
		Summary: "a reference is saved without its object",
		Causes:  []string{"a mod saves a reference to a pawn or lord it does not own"},
		Route:   []string{"analyze_log cluster=E<n>"},
		Fixes:   []string{"ignore", "update"}, Workflow: "no"},
	{ID: "save.abstract-component", Match: `Can't load abstract class`, Stage: "save",
		Summary: "the save has a map/game/world component of a mod that is not loaded",
		Causes:  []string{"a mod was removed after the save was made"},
		Route:   []string{"analyze_log cluster=E<n>"},
		Fixes:   []string{"ignore"}, Workflow: "no"},
	{ID: "save.other", Class: logs.ClassSave, Match: `.`, Fallback: true, Stage: "save",
		Summary: "other save-loading error", Route: []string{"analyze_log cluster=E<n>"}, Fixes: []string{"ignore"}, Workflow: "no"},
	// XML patches
	{ID: "patch.findmod", Class: logs.ClassPatchFailed, Match: `PatchOperationFindMod|XmlExtensions\.(FindMod|OptionalPatch)`, Stage: "load",
		Summary: "a FindMod patch (optional-mod patch) failed inside",
		Causes:  []string{"the target mod's defs changed", "the patch is for another version of the target mod"},
		Route:   []string{"get_patches failed=true", "get_def <target>"},
		Fixes:   []string{"update", "disable", "local-fix"}, Workflow: "yes"},
	{ID: "patch.xpath", Class: logs.ClassPatchFailed, Match: `Failed to find a node with the given xpath`, Stage: "load",
		Summary: "a patch operation found no node to change",
		Causes:  []string{"the target mod or DLC is inactive (unguarded optional patch)", "the target was renamed in an update", "the target loads after the patch"},
		Route:   []string{"get_patches failed=true (reasons: target-inactive, target-missing, target-later, stale-name …)", "get_def <target>"},
		Fixes:   []string{"enable-dependency", "reorder", "update", "local-fix"}, Workflow: "yes"},
	{ID: "patch.sequence", Class: logs.ClassPatchFailed, Match: `PatchOperationSequence: Error in the operation at`, Stage: "load",
		Summary: "one operation of a patch sequence failed",
		Causes:  []string{"as patch.xpath, for the operation named"},
		Route:   []string{"get_patches failed=true"},
		Fixes:   []string{"enable-dependency", "update", "local-fix"}, Workflow: "yes"},
	{ID: "patch.op-failed", Class: logs.ClassPatchFailed, Match: `Patch operation .* failed`, Stage: "load",
		Summary: "a mod's patch operation failed (the game's summary line, names the mod and the xpath)",
		Causes:  []string{"as patch.xpath: target inactive, renamed or loading later", "a custom operation class whose condition failed"},
		Route:   []string{"get_patches failed=true", "get_def <target>"},
		Fixes:   []string{"enable-dependency", "reorder", "update", "local-fix"}, Workflow: "yes"},
	{ID: "patch.other", Class: logs.ClassPatchFailed, Match: `.`, Fallback: true, Stage: "load",
		Summary: "other patch failure", Route: []string{"get_patches failed=true"}, Fixes: []string{"update", "local-fix"}, Workflow: "yes"},
	// XML loading
	{ID: "xml.no-field", Match: `doesn't correspond to any field`, Stage: "load",
		Summary: "XML sets a field the def's class does not have",
		Causes:  []string{"the mod was written for another game version", "a patch adds a field meant for a class from an inactive mod", "a framework mod is missing or outdated"},
		Route:   []string{"analyze_log cluster=E<n>", "get_def <defName>", "get_patches"},
		Fixes:   []string{"update", "enable-dependency", "ignore"}, Workflow: "partial"},
	{ID: "xml.type-missing", Match: `Could not find (a )?type|Could not find type named|Could not find class`, Stage: "load",
		Summary: "XML names a C# class that no loaded assembly has",
		Causes:  []string{"the framework mod that defines the class is not active or loads later", "the class was renamed in an update"},
		Route:   []string{"analyze_log cluster=E<n> (dependency candidates)", "search_files <class>", "list_problems code=dep"},
		Fixes:   []string{"enable-dependency", "update"}, Workflow: "partial"},
	{ID: "xml.config-error", Match: `^Config error in`, Stage: "load",
		Summary: "a def fails the game's config checks", Causes: []string{"authoring mistake in the mod"},
		Route: []string{"get_def <defName>"}, Fixes: []string{"ignore", "local-fix"}, Workflow: "no"},
	{ID: "xml.asset-load", Match: `Exception loading UnityEngine\.(Texture|AudioClip)`, Stage: "load",
		Summary: "a texture or sound file could not be read", Causes: []string{"corrupt or unsupported file in the mod"},
		Route: []string{"analyze_log cluster=E<n>"}, Fixes: []string{"update", "ignore"}, Workflow: "no"},
	{ID: "xml.def-exception", Match: `Exception loading def from file`, Stage: "load",
		Summary: "a def file could not be loaded", Causes: []string{"a class named by the XML is missing", "malformed XML"},
		Route: []string{"analyze_log cluster=E<n>", "search_files"}, Fixes: []string{"enable-dependency", "update"}, Workflow: "yes"},
	{ID: "xml.other", Class: logs.ClassXMLLoad, Match: `.`, Fallback: true, Stage: "load",
		Summary: "other XML loading error", Route: []string{"analyze_log cluster=E<n>", "search_files"}, Fixes: []string{"update"}, Workflow: "yes"},
	// assets
	{ID: "asset.texture-missing", Class: logs.ClassTexture, Match: `.`, Stage: "load",
		Summary: "a texture or sound path names no file in any active mod",
		Causes:  []string{"the art comes from a mod or DLC that is not active", "the mod ships the file under another path or case"},
		Route:   []string{"analyze_log cluster=E<n>", "search_files <path>"},
		Fixes:   []string{"enable-dependency", "update", "ignore"}, Workflow: "no"},
	{ID: "asset.maintex", Class: logs.ClassMaterial, Match: `_MainTex`, Stage: "load",
		Summary: "a material without a texture (often an icon)",
		Causes:  []string{"a buildable def with uiIconColor but no icon path"},
		Route:   []string{"analyze_log cluster=E<n>", "search_files uiIconColor"},
		Fixes:   []string{"local-fix", "update"}, Workflow: "yes"},
	// assemblies
	{ID: "asm.dependency-missing", Match: `Cannot resolve dependency to assembly`, Stage: "load",
		Summary: "a mod's DLL references an assembly that is not loaded",
		Causes:  []string{"a required framework mod is missing or loads later", "the DLL is for another game version"},
		Route:   []string{"analyze_log cluster=E<n>", "get_mod <ref> (assemblies)", "list_problems code=dep"},
		Fixes:   []string{"enable-dependency", "reorder", "update"}, Workflow: "no"},
	{ID: "asm.harmony-target", Match: `HarmonyException: Patching exception|Undefined target method|Patching exception in method|Exception patching|Failed to patch|Cannot instrument|couldn't find target instruction|Ambiguous match for HarmonyMethod|^Transpiler: `, Stage: "load",
		Summary: "a Harmony patch could not be applied (target method missing or changed)",
		Causes:  []string{"the mod was built for another version of the game or of the target mod", "the target mod is not active"},
		Route:   []string{"analyze_log cluster=E<n>", "get_harmony target=<method>", "read_source type=<patch class>"},
		Fixes:   []string{"update", "enable-dependency", "disable"}, Workflow: "partial"},
	{ID: "asm.version-mismatch", Match: `MissingMethodException|MissingFieldException|Method not found|Field not found|TypeLoadException|Could not resolve type|Could not load type|invalid vtable|ReflectionTypeLoadException`, Stage: "load",
		Summary: "a DLL calls code that does not exist in the loaded game or dependency",
		Causes:  []string{"the mod or one of its dependencies is outdated", "two copies of a library in different versions"},
		Route:   []string{"analyze_log cluster=E<n>", "get_mod <ref> (assemblies, versions)", "find_mods <dependency>"},
		Fixes:   []string{"update", "disable"}, Workflow: "partial"},
	{ID: "asm.invalid-il", Match: `InvalidProgramException|Invalid IL`, Stage: "runtime",
		Summary: "a transpiler produced invalid code",
		Causes:  []string{"two transpilers on one method conflict", "a transpiler for another game version"},
		Route:   []string{"get_harmony target=<method>", "list_problems code=harmony.multi-transpiler"},
		Fixes:   []string{"disable", "update"}, Workflow: "partial"},
	{ID: "asm.static-ctor", Match: `Error in static constructor of|Error while instantiating a mod of type|TypeInitializationException`, Stage: "load",
		Summary: "a mod's start-up code threw",
		Causes:  []string{"the mod's Harmony patching failed", "a dependency is missing"},
		Route:   []string{"analyze_log cluster=E<n>", "read_source type=<type>"},
		Fixes:   []string{"update", "enable-dependency", "disable"}, Workflow: "partial"},
	// runtime
	{ID: "rt.tick", Match: `(?i)ticking|\bTick\b`, Stage: "runtime",
		Summary: "an exception while ticking a pawn, thing or the world",
		Causes:  []string{"a comp added or removed during tick", "a patch on the tick path"},
		Route:   []string{"analyze_log cluster=E<n>", "get_harmony target=<each patched frame>", "read_source search=\"AllComps.Remove(\""},
		Fixes:   []string{"disable", "update", "local-fix"}, Workflow: "yes"},
	{ID: "rt.ui", Match: `Exception filling window|OnGUI|UIRootUpdate|Root level exception in Update|DoWindowContents`, Stage: "runtime",
		Summary: "an exception in drawing the UI",
		Causes:  []string{"a UI patch from a mod", "a window of a mod with missing data"},
		Route:   []string{"analyze_log cluster=E<n>", "get_harmony target=<patched frame>"},
		Fixes:   []string{"disable", "update"}, Workflow: "partial"},
	{ID: "rt.pawn-generation", Match: `Error while generating pawn|generating pawn group|generating thing set`, Stage: "runtime",
		Summary: "pawn generation failed", Causes: []string{"a race, xenotype or apparel mod with incomplete defs", "a patch on PawnGenerator"},
		Route: []string{"analyze_log cluster=E<n>", "get_harmony target=RimWorld.PawnGenerator.GeneratePawn"}, Fixes: []string{"disable", "update"}, Workflow: "partial"},
	{ID: "rt.mapgen", Match: `Error in (World)?GenStep`, Stage: "runtime",
		Summary: "map or world generation step failed", Causes: []string{"a map-generation mod or a missing def in a GenStep"},
		Route: []string{"analyze_log cluster=E<n>"}, Fixes: []string{"disable", "update"}, Workflow: "partial"},
	{ID: "rt.post-load", Match: `post-long-event|post-load event|PostLoadInit|post load init|SaveableFromNode exception|ExposeData|Exception loading from System\.Xml|loaded object directory|Exception spawning loaded thing|FinalizeLoading`, Stage: "save",
		Summary: "an exception while loading a save or after a long event",
		Causes:  []string{"a mod's save data from another version", "a mod removed or added mid-save"},
		Route:   []string{"analyze_log cluster=E<n>"},
		Fixes:   []string{"update", "ignore", "new-game"}, Workflow: "no"},
	{ID: "rt.resolve-references", Match: `Error while resolving references|Could not resolve cross refs`, Stage: "load",
		Summary: "a def's ResolveReferences threw", Causes: []string{"a def with missing fields its class expects"},
		Route: []string{"analyze_log cluster=E<n>", "get_def <defName>"}, Fixes: []string{"update", "enable-dependency"}, Workflow: "partial"},
	{ID: "rt.duplicate-key", Match: `An item with the same key`, Stage: "runtime",
		Summary: "duplicate key in a dictionary", Causes: []string{"two mods register the same thing", "a duplicated def or ID"},
		Route: []string{"analyze_log cluster=E<n>", "list_problems code=def"}, Fixes: []string{"disable", "update"}, Workflow: "no"},
	{ID: "noise.hugslib-publish", Match: `\[HugsLib\].*(Bad Request|log publishing)`, Stage: "noise",
		Summary: "HugsLib's own log upload failed (not a mod problem)", Route: []string{}, Fixes: []string{"ignore"}, Workflow: "no"},
	{ID: "noise.analyzer", Match: `\[Analyzer\] Detected Visual Exceptions`, Stage: "noise",
		Summary: "Dubs Performance Analyzer reports its own status", Route: []string{}, Fixes: []string{"ignore"}, Workflow: "no"},
	{ID: "rt.ai-job", Match: `ThinkNode|TryIssueJobPackage|JobDriver|JobGiver|WorkGiver|float menu`, Stage: "runtime",
		Summary: "an exception in pawn AI (think tree, job giver, job driver)",
		Causes:  []string{"a job or work-giver of a mod with missing data", "a patch on the AI path"},
		Route:   []string{"analyze_log cluster=E<n>", "get_harmony target=<patched frame>", "read_source type=<job class>"},
		Fixes:   []string{"disable", "update"}, Workflow: "partial"},
	{ID: "rt.map-draw", Match: `Could not regenerate layer|SectionLayer`, Stage: "runtime",
		Summary: "a map drawing layer failed", Causes: []string{"a terrain or graphics mod", "a patch on map drawing"},
		Route: []string{"analyze_log cluster=E<n>", "get_harmony target=<patched frame>"}, Fixes: []string{"disable", "update"}, Workflow: "no"},
	{ID: "rt.comp-init", Match: `Could not instantiate or initialize a ThingComp|Could not instantiate inspector tab|Null method for`, Stage: "runtime",
		Summary: "a comp, tab or worker class could not be created", Causes: []string{"a class named by XML is missing or has no default constructor", "the providing mod is outdated"},
		Route: []string{"analyze_log cluster=E<n>", "search_files <class>"}, Fixes: []string{"update", "enable-dependency"}, Workflow: "partial"},
	{ID: "rt.collection-modified", Match: `Collection was modified`, Stage: "runtime",
		Summary: "a list was changed while iterating (often comps or a mod's cache)", Causes: []string{"AllComps.Add/Remove during tick", "a mod modifying a game list from a patch"},
		Route: []string{"analyze_log cluster=E<n>", "read_source search=\"AllComps.Remove(\""}, Fixes: []string{"disable", "update", "local-fix"}, Workflow: "partial"},
	{ID: "asset.texture-create", Match: `Failed to create texture because of invalid parameters`, Stage: "load",
		Summary: "Unity rejected a texture (size or format)", Causes: []string{"a texture-replacement or resizing mod", "corrupt image in a mod"},
		Route: []string{"analyze_log cluster=E<n>"}, Fixes: []string{"disable", "update"}, Workflow: "no"},
	{ID: "rt.translation", Match: `Failed to resolve text|injecting translations|AutoTranslation|translate failed`, Stage: "load",
		Summary: "a translation or text resolution failed", Causes: []string{"a translation mod for another version of the translated mod"},
		Route: []string{"analyze_log cluster=E<n>"}, Fixes: []string{"update", "ignore"}, Workflow: "no"},
	{ID: "noise.harmony-il-dump", Match: `\[EX_(Begin|End)Exception\]|^\[Ref #?\w+\]$|^\S+ Label#`, Stage: "noise",
		Summary: "a fragment of a Harmony IL dump or a stack reference line (belongs to the error above it)", Route: []string{}, Fixes: []string{"ignore"}, Workflow: "no"},
	{ID: "noise.mod-status", Match: `Loaded settings:|^Visual Exceptions$`, Stage: "noise",
		Summary: "a mod logs its status as an error", Route: []string{}, Fixes: []string{"ignore"}, Workflow: "no"},
	{ID: "rt.nre", Match: `NullReferenceException`, Fallback: true, Stage: "runtime",
		Summary: "a null reference in mod or game code", Route: []string{"analyze_log cluster=E<n>", "get_harmony target=<patched frame>", "read_source"},
		Fixes: []string{"disable", "update"}, Workflow: "partial"},
	{ID: "rt.other", Match: `.`, Fallback: true, Stage: "runtime",
		Summary: "other exception", Route: []string{"analyze_log cluster=E<n>"}, Fixes: []string{"disable", "update"}, Workflow: "partial"},
}

func init() {
	for _, r := range rules {
		r.re = regexp.MustCompile(r.Match)
	}
}

func classify(class, tpl string) *Rule {
	msg := tpl
	if i := strings.IndexByte(tpl, '|'); i >= 0 {
		msg = tpl[i+1:]
	}
	for _, r := range rules {
		if r.Class != "" && r.Class != class {
			continue
		}
		if r.re.MatchString(msg) {
			return r
		}
	}
	return nil
}

// ---- output ----

// Pattern is one entry of the dataset (design §5 L1 Pattern).
type Pattern struct {
	ID       string      `json:"id"`
	Layer    string      `json:"layer"` // generic | mod
	Rule     *Rule       `json:"rule,omitempty"`
	Template string      `json:"template,omitempty"` // mod layer: the cluster template
	Class    string      `json:"class"`
	Mod      string      `json:"mod,omitempty"` // mod layer: the source package
	Versions []string    `json:"mod_versions,omitempty"`
	First    string      `json:"first_seen,omitempty"` // month
	Last     string      `json:"last_seen,omitempty"`
	Logs     int         `json:"logs"`
	Sessions int         `json:"sessions"`
	Clusters int         `json:"clusters"`
	Count    int         `json:"occurrences"`
	WithSrc  int         `json:"clusters_with_source"`
	TopMods  []NameCount `json:"top_source_mods,omitempty"` // generic layer: mods most often attributed as source (>= minlogs logs)
	Res      *Res        `json:"resolution"`
}

type NameCount struct {
	Name string `json:"name"`
	N    int    `json:"logs"`
}

// Res is the session evidence (design §5 L1 Resolution, source "session").
type Res struct {
	// Pairs: consecutive uploads where A has the pattern, by list change; Gone: of those, B no longer has it.
	Pairs map[string]int `json:"pairs"`
	Gone  map[string]int `json:"gone"`
	// Fixes: "small" pairs where it disappeared, by what changed. A removed source mod is the
	// strongest form; the others only co-occur.
	Fixes map[string]int `json:"fixes,omitempty"`
}

func newRes() *Res {
	return &Res{Pairs: map[string]int{}, Gone: map[string]int{}, Fixes: map[string]int{}}
}

func main() {
	parsed := flag.String("parsed", "", "spikes/hugslogs parsed.jsonl")
	results := flag.String("results", "", "spikes/corpusrun results (clusters with attribution)")
	out := flag.String("out", "", "pattern dataset JSON to write")
	mm := flag.String("mm", "1.6", "game major.minor")
	minLogs := flag.Int("minlogs", 10, "mod-specific pattern: at least this many logs (design §8)")
	minSess := flag.Int("minsessions", 5, "mod-specific pattern: from at least this many sessions")
	maxChange := flag.Int("maxchange", 3, "resolution: at most this many package/version changes between two uploads")
	flag.StringVar(&dump, "dump", "", "print the most common templates of this rule id (local exploration; not for RESULT)")
	flag.Parse()
	if err := run(*parsed, *results, *out, *mm, *minLogs, *minSess, *maxChange); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

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

var dump string

func run(parsedPath, resultsPath, outPath, mm string, minLogs, minSess, maxChange int) error {
	attr := map[string][]rcluster{}
	if resultsPath != "" {
		if err := eachLine(resultsPath, func(b []byte) error {
			var r rresult
			if json.Unmarshal(b, &r) == nil {
				attr[r.ID] = r.Clusters
			}
			return nil
		}); err != nil {
			return err
		}
	}
	var ls []*log1
	fromResults := 0
	if err := eachLine(parsedPath, func(b []byte) error {
		var r precord
		if json.Unmarshal(b, &r) != nil || r.Status != "ok" || r.MM != mm {
			return nil
		}
		t, _ := time.Parse(time.RFC3339, r.Created)
		l := &log1{created: t, mods: r.Mods, set: map[string]bool{}, mv: map[string]string{}, pos: map[string]int{}}
		for i, m := range r.Mods {
			l.set[m.Pkg] = true
			l.mv[m.Pkg] = m.version()
			if _, ok := l.pos[m.Pkg]; !ok {
				l.pos[m.Pkg] = i
			}
		}
		if rc, ok := attr[r.ID]; ok {
			fromResults++
			for _, c := range rc {
				if logs.Informational(c.Class) {
					continue
				}
				x := &cl{tpl: c.Tpl, class: c.Class, count: c.Count}
				for _, a := range c.Attr {
					if a.Role == "source" {
						x.src = append(x.src, a.Pkg)
					}
				}
				l.cls = append(l.cls, x)
			}
		} else {
			for _, c := range r.Clusters {
				if !logs.Informational(c.Class) {
					l.cls = append(l.cls, &cl{tpl: c.Tpl, class: c.Class, count: c.Count})
				}
			}
		}
		for _, c := range l.cls {
			c.gen = classify(c.class, c.tpl)
		}
		ls = append(ls, l)
		return nil
	}); err != nil {
		return err
	}
	attr = nil
	if dump != "" {
		n := map[string]int{}
		for _, l := range ls {
			seen := map[string]bool{}
			for _, c := range l.cls {
				id := "none"
				if c.gen != nil {
					id = c.gen.ID
				}
				if id == dump && !seen[c.tpl] {
					seen[c.tpl] = true
					n[c.tpl]++
				}
			}
		}
		type kv struct {
			k string
			v int
		}
		var s []kv
		for k, v := range n {
			s = append(s, kv{k, v})
		}
		sort.Slice(s, func(a, b int) bool { return s[a].v > s[b].v })
		for i := 0; i < len(s) && i < 60; i++ {
			fmt.Printf("%5d %s\n", s[i].v, trunc(s[i].k, 220))
		}
		return nil
	}
	sort.SliceStable(ls, func(a, b int) bool { return ls[a].created.Before(ls[b].created) })
	p := func(f string, a ...any) { fmt.Printf(f+"\n", a...) }
	p("Spike patterns — error pattern library v0 (docs/research/00 §3 G5)")
	p("logs %d (game %s), clusters with RimArcho attribution from the results file: %d logs", len(ls), mm, fromResults)

	// Sessions, as hugslogs stats.
	parent := make([]int, len(ls))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	type link struct{ a, b int }
	var links []link
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
			links = append(links, link{best, j})
			parent[find(j)] = find(best)
		}
	}
	for i, l := range ls {
		l.session = find(i)
	}

	// Generic layer.
	type agg struct {
		logs, clusters, count, withSrc int
		sess                           map[int]bool
		srcLogs                        map[string]int
		first, last                    string
		versions                       map[string]bool
		res                            *Res
	}
	newAgg := func() *agg {
		return &agg{sess: map[int]bool{}, srcLogs: map[string]int{}, versions: map[string]bool{}, res: newRes()}
	}
	gen := map[string]*agg{}
	type mk struct{ tpl, mod string }
	mods := map[mk]*agg{}
	modClass := map[mk]string{}
	var totC, totN, matchedC, matchedN int
	var perLog []float64
	wfC := map[string]int{}    // clusters by the current workflow's coverage
	wfL := map[string]int{}    // logs with at least one cluster of that coverage
	stageC := map[string]int{} // clusters by failure stage
	for _, l := range ls {
		seenWF := map[string]bool{}
		seenG := map[string]bool{}
		seenSrc := map[string]map[string]bool{}
		seenM := map[mk]bool{}
		lc, lm := 0, 0
		for _, c := range l.cls {
			totC++
			totN += c.count
			lc++
			r := c.gen
			if r != nil && !r.Fallback {
				matchedC++
				matchedN += c.count
				lm++
			}
			id, wf, stage := "none", "no", "?"
			if r != nil {
				id, wf, stage = r.ID, r.Workflow, r.Stage
			}
			if stage != "noise" {
				wfC[wf]++
				if !seenWF[wf] {
					seenWF[wf] = true
					wfL[wf]++
				}
			}
			stageC[stage]++
			g := gen[id]
			if g == nil {
				g = newAgg()
				gen[id] = g
			}
			g.clusters++
			g.count += c.count
			if len(c.src) > 0 {
				g.withSrc++
			}
			if !seenG[id] {
				seenG[id] = true
				g.logs++
				g.sess[l.session] = true
				seenSrc[id] = map[string]bool{}
			}
			for _, s := range c.src {
				if !seenSrc[id][s] {
					seenSrc[id][s] = true
					g.srcLogs[s]++
				}
			}
			// Mod layer: a template with exactly one source mod.
			if len(c.src) == 1 {
				k := mk{c.tpl, c.src[0]}
				m := mods[k]
				if m == nil {
					m = newAgg()
					mods[k] = m
					modClass[k] = c.class
				}
				m.clusters++
				m.count += c.count
				m.withSrc++
				if !seenM[k] {
					seenM[k] = true
					m.logs++
					m.sess[l.session] = true
					month := l.created.Format("2006-01")
					if m.first == "" || month < m.first {
						m.first = month
					}
					if month > m.last {
						m.last = month
					}
					if v := l.mv[c.src[0]]; v != "" {
						m.versions[v] = true
					}
				}
			}
		}
		if lc > 0 {
			perLog = append(perLog, float64(lm)/float64(lc))
		}
	}
	mean := 0.0
	for _, v := range perLog {
		mean += v
	}
	if len(perLog) > 0 {
		mean /= float64(len(perLog))
	}
	p("\n== Coverage of the generic rules (fallback rules = class only, not counted)")
	p("problem clusters %d: matched %.1f%%; weighted by occurrences %.1f%%; mean per log %.1f%% (%d logs with problem clusters)",
		totC, pct(matchedC, totC), pct(matchedN, totN), 100*mean, len(perLog))
	p("clusters by failure stage: %s", fmtMap(stageC))
	nn := wfC["yes"] + wfC["partial"] + wfC["no"]
	p("current MCP diagnosis workflow (Instructions), non-noise clusters: covered %.1f%%, partly %.1f%%, not covered %.1f%%; logs with a cluster of each: %.1f%% / %.1f%% / %.1f%%",
		pct(wfC["yes"], nn), pct(wfC["partial"], nn), pct(wfC["no"], nn), pct(wfL["yes"], len(perLog)), pct(wfL["partial"], len(perLog)), pct(wfL["no"], len(perLog)))

	// Resolution evidence over the session links.
	kinds := map[string]int{}
	type chg struct {
		kind                string
		removed, added, upd []string
		moved               int
	}
	diff := func(a, b *log1) chg {
		var c chg
		for k := range a.set {
			if !b.set[k] {
				c.removed = append(c.removed, k)
			} else if a.mv[k] != b.mv[k] && a.mv[k] != "" && b.mv[k] != "" {
				c.upd = append(c.upd, k)
			}
		}
		for k := range b.set {
			if !a.set[k] {
				c.added = append(c.added, k)
			}
		}
		// order: common packages whose relative order changed (mods not on the longest kept sequence)
		var seq []int
		for _, m := range b.mods {
			if i, ok := a.pos[m.Pkg]; ok && a.set[m.Pkg] {
				seq = append(seq, i)
			}
		}
		c.moved = len(seq) - lis(seq)
		n := len(c.removed) + len(c.added) + len(c.upd)
		switch {
		case n == 0 && c.moved == 0:
			c.kind = "none"
		case n == 0:
			c.kind = "order"
		case n <= maxChange:
			c.kind = "small"
		default:
			c.kind = "large"
		}
		return c
	}
	for _, lk := range links {
		a, b := ls[lk.a], ls[lk.b]
		c := diff(a, b)
		kinds[c.kind]++
		if c.kind == "large" {
			continue
		}
		bG := map[string]bool{}
		bM := map[mk]bool{}
		bT := map[string]bool{}
		for _, x := range b.cls {
			if x.gen != nil {
				bG[x.gen.ID] = true
			}
			bT[x.tpl] = true
			if len(x.src) == 1 {
				bM[mk{x.tpl, x.src[0]}] = true
			}
		}
		removed := map[string]bool{}
		for _, r := range c.removed {
			removed[r] = true
		}
		upd := map[string]bool{}
		for _, r := range c.upd {
			upd[r] = true
		}
		// what kinds of change a disappearance coincides with
		fixKinds := func(srcs map[string]bool) []string {
			var out []string
			srcRemoved, srcUpd := false, false
			for s := range srcs {
				srcRemoved = srcRemoved || removed[s]
				srcUpd = srcUpd || upd[s]
			}
			if srcRemoved {
				out = append(out, "disable-source")
			} else if len(c.removed) > 0 {
				out = append(out, "disable-other")
			}
			if len(c.added) > 0 {
				out = append(out, "enable")
			}
			if srcUpd {
				out = append(out, "update-source")
			} else if len(c.upd) > 0 {
				out = append(out, "update-other")
			}
			if c.moved > 0 {
				out = append(out, "reorder")
			}
			return out
		}
		doneG := map[string]bool{}
		srcG := map[string]map[string]bool{}
		for _, x := range a.cls {
			if x.gen == nil {
				continue
			}
			if srcG[x.gen.ID] == nil {
				srcG[x.gen.ID] = map[string]bool{}
			}
			for _, s := range x.src {
				srcG[x.gen.ID][s] = true
			}
		}
		for _, x := range a.cls {
			if x.gen != nil && !doneG[x.gen.ID] {
				doneG[x.gen.ID] = true
				r := gen[x.gen.ID].res
				r.Pairs[c.kind]++
				if !bG[x.gen.ID] {
					r.Gone[c.kind]++
					if c.kind == "small" || c.kind == "order" {
						for _, f := range fixKinds(srcG[x.gen.ID]) {
							r.Fixes[f]++
						}
					}
				}
			}
		}
		doneM := map[mk]bool{}
		for _, x := range a.cls {
			if len(x.src) != 1 {
				continue
			}
			k := mk{x.tpl, x.src[0]}
			if doneM[k] {
				continue
			}
			doneM[k] = true
			r := mods[k].res
			r.Pairs[c.kind]++
			if !bM[k] && !bT[x.tpl] {
				r.Gone[c.kind]++
				if c.kind == "small" || c.kind == "order" {
					for _, f := range fixKinds(map[string]bool{x.src[0]: true}) {
						r.Fixes[f]++
					}
				}
			}
		}
	}
	p("\n== Sessions")
	nSess := map[int]int{}
	for _, l := range ls {
		nSess[l.session]++
	}
	multi, inMulti := 0, 0
	for _, n := range nSess {
		if n > 1 {
			multi++
			inMulti += n
		}
	}
	p("sessions with 2+ uploads %d, covering %d logs (%.0f%%); consecutive pairs %d: no change %d, order only %d, 1-%d package/version changes %d, more (excluded) %d",
		multi, inMulti, pct(inMulti, len(ls)), len(links), kinds["none"], kinds["order"], maxChange, kinds["small"], kinds["large"])

	// Dataset.
	var pats []*Pattern
	p("\n== Generic patterns (logs, sessions, clusters, with a source; pairs where A has it: gone in B for no change / order / small change; fixes in small/order pairs)")
	ids := make([]string, 0, len(gen))
	for id := range gen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(a, b int) bool {
		return gen[ids[a]].logs > gen[ids[b]].logs || gen[ids[a]].logs == gen[ids[b]].logs && ids[a] < ids[b]
	})
	ruleByID := map[string]*Rule{}
	for _, r := range rules {
		ruleByID[r.ID] = r
	}
	for _, id := range ids {
		g := gen[id]
		pt := &Pattern{ID: "g:" + id, Layer: "generic", Rule: ruleByID[id], Logs: g.logs, Sessions: len(g.sess), Clusters: g.clusters,
			Count: g.count, WithSrc: g.withSrc, Res: g.res}
		if r := ruleByID[id]; r != nil {
			pt.Class = r.Class
		}
		for s, n := range g.srcLogs {
			if n >= minLogs {
				pt.TopMods = append(pt.TopMods, NameCount{s, n})
			}
		}
		sort.Slice(pt.TopMods, func(a, b int) bool {
			return pt.TopMods[a].N > pt.TopMods[b].N || pt.TopMods[a].N == pt.TopMods[b].N && pt.TopMods[a].Name < pt.TopMods[b].Name
		})
		if len(pt.TopMods) > 10 {
			pt.TopMods = pt.TopMods[:10]
		}
		pats = append(pats, pt)
		fb := ""
		if r := ruleByID[id]; r != nil && r.Fallback {
			fb = " (fallback)"
		}
		r := g.res
		p("  %-26s %5d logs %5d sess %7d cl %5.1f%% src | gone none %s order %s small %s | %s%s", id, g.logs, len(g.sess), g.clusters, pct(g.withSrc, g.clusters),
			rate(r, "none"), rate(r, "order"), rate(r, "small"), fmtMap(r.Fixes), fb)
	}
	// Mod layer.
	type mkv struct {
		k mk
		a *agg
	}
	var ms []mkv
	dropped := 0
	for k, a := range mods {
		if a.logs < minLogs || len(a.sess) < minSess || private(k.tpl) {
			dropped++
			continue
		}
		ms = append(ms, mkv{k, a})
	}
	sort.Slice(ms, func(a, b int) bool {
		return ms[a].a.logs > ms[b].a.logs || ms[a].a.logs == ms[b].a.logs && ms[a].k.tpl < ms[b].k.tpl
	})
	modsN := map[string]bool{}
	var mClusters, mLogsSum int
	for _, m := range ms {
		modsN[m.k.mod] = true
		mClusters += m.a.clusters
		mLogsSum += m.a.logs
		var vs []string
		for v := range m.a.versions {
			vs = append(vs, v)
		}
		sort.Strings(vs)
		if len(vs) > 6 {
			vs = append(vs[:3], vs[len(vs)-3:]...)
		}
		g := classify(modClass[m.k], m.k.tpl)
		gid := ""
		if g != nil {
			gid = g.ID
		}
		pats = append(pats, &Pattern{ID: fmt.Sprintf("m:%s:%s", m.k.mod, shortHash(m.k.tpl)), Layer: "mod", Template: m.k.tpl, Class: modClass[m.k],
			Mod: m.k.mod, Versions: vs, First: m.a.first, Last: m.a.last, Logs: m.a.logs, Sessions: len(m.a.sess), Clusters: m.a.clusters,
			Count: m.a.count, WithSrc: m.a.withSrc, Res: m.a.res, Rule: &Rule{ID: gid}})
	}
	p("\n== Mod-specific patterns (one source mod; >= %d logs from >= %d sessions; %d candidates below the threshold dropped)", minLogs, minSess, dropped)
	p("kept %d patterns over %d mods, %d clusters", len(ms), len(modsN), mClusters)
	p("top 40 (logs, sessions, versions seen, pairs with it: gone after no change / small change; fixes):")
	for i := 0; i < len(ms) && i < 40; i++ {
		m := ms[i]
		r := m.a.res
		p("  %4d logs %4d sess %2d vers | gone none %s small %s | %-40s %s  %s", m.a.logs, len(m.a.sess), len(m.a.versions), rate(r, "none"), rate(r, "small"),
			fmtMap(r.Fixes), m.k.mod, trunc(m.k.tpl, 110))
	}
	// Strongest fix evidence: mod patterns whose source mod was removed and the error went.
	p("\nmod patterns with fix evidence 'disable-source' (removing the source mod; the error went):")
	var ev []mkv
	for _, m := range ms {
		if m.a.res.Fixes["disable-source"] > 0 {
			ev = append(ev, m)
		}
	}
	sort.Slice(ev, func(a, b int) bool { return ev[a].a.res.Fixes["disable-source"] > ev[b].a.res.Fixes["disable-source"] })
	p("  %d patterns", len(ev))
	for i := 0; i < len(ev) && i < 15; i++ {
		p("  %2d  %s  %s", ev[i].a.res.Fixes["disable-source"], ev[i].k.mod, trunc(ev[i].k.tpl, 110))
	}

	if outPath != "" {
		ds := struct {
			Version   string     `json:"version"`
			Game      string     `json:"game"`
			Logs      int        `json:"logs"`
			Generated string     `json:"generated"`
			Note      string     `json:"note"`
			Patterns  []*Pattern `json:"patterns"`
		}{"v0", mm, len(ls), time.Now().UTC().Format("2006-01-02"),
			"Aggregates from public HugsLib logs (docs/research/00). Counts are logs, sessions and clusters; no log ids. " +
				"Resolution: consecutive uploads of one player; pairs = A has the pattern, gone = B does not; fixes only co-occur with the change (not causal).",
			pats}
		// One pattern per line: readable diffs between versions of the dataset.
		head := ds
		head.Patterns = nil
		hb, err := json.Marshal(head)
		if err != nil {
			return err
		}
		var sb strings.Builder
		sb.Write(hb[:len(hb)-len(`,"patterns":null}`)])
		sb.WriteString(",\n\"patterns\": [\n")
		for i, pt := range pats {
			b, err := json.Marshal(pt)
			if err != nil {
				return err
			}
			sb.Write(b)
			if i < len(pats)-1 {
				sb.WriteString(",")
			}
			sb.WriteString("\n")
		}
		sb.WriteString("]}\n")
		if err := os.WriteFile(outPath, []byte(sb.String()), 0o644); err != nil {
			return err
		}
		p("\nwrote %s: %d patterns", outPath, len(pats))
	}
	return nil
}

// private: a template that could carry a player's own data (paths, user names) is never kept.
var privateRe = regexp.MustCompile(`(?i)users[\\/]|/home/|appdata|steamapps|[a-z]:[\\/]|@[a-z0-9-]+\.[a-z]`)

func private(s string) bool { return privateRe.MatchString(s) }

func rate(r *Res, k string) string {
	if r.Pairs[k] == 0 {
		return "   -     "
	}
	return fmt.Sprintf("%3.0f%%/%-4d", pct(r.Gone[k], r.Pairs[k]), r.Pairs[k])
}

func fmtMap(m map[string]int) string {
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
		out = append(out, fmt.Sprintf("%s %d", x.k, x.v))
	}
	return strings.Join(out, ", ")
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

func lis(a []int) int {
	var tails []int
	for _, x := range a {
		i := sort.SearchInts(tails, x)
		if i == len(tails) {
			tails = append(tails, x)
		} else {
			tails[i] = x
		}
	}
	return len(tails)
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func shortHash(s string) string {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return fmt.Sprintf("%08x", h)
}
