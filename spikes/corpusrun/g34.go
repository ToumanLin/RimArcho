package main

// G3 (Harmony static recall) and G4 (severity calibration), docs/research/00 §3.
//
// G3 compares the "Active Harmony patches:" header of a log (what Harmony really applied) with the
// static harmony_fact rows of the same Loadout (what the scan of the mods' assemblies found). A
// runtime patch counts only when its patch class belongs to a mod of the Loadout that the research
// library resolves (coverage, design §5 L2); the library has the current Workshop versions, not the
// player's, so part of a miss can be a version difference.
//
// G4 records the static findings of the log's analysis (decl, def, patch, harmony layers) with the
// clusters of the same log that could be their visible effect: clusters attributed to a subject
// mod, and clusters whose text names the finding's def, patch file or Harmony target.

import (
	"path"
	"regexp"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// HPatch is one line of the log's "Active Harmony patches:" (parsed.jsonl).
type HPatch struct {
	Target  string   `json:"target"`
	Patches []string `json:"patches"`
}

// RHarmony is G3 for one log.
type RHarmony struct {
	Runtime int `json:"runtime"` // patch methods applied at runtime (one per target × patch)
	Owned   int `json:"owned"`   // of those, owned by a mod the library resolves (the denominator)
	Matched int `json:"matched"` // a static fact with the same patch method, kind and target
	Loose   int `json:"loose"`   // the patch method is a static fact, but with another target or kind (dynamic, TargetMethods …)
	// Owners: per owning mod. Missed = Owned - Matched - Loose.
	Owners []*RHOwner `json:"owners,omitempty"`
}

// RHOwner is one mod's share of G3.
type RHOwner struct {
	Pkg     string `json:"pkg"`
	Owned   int    `json:"owned"`
	Matched int    `json:"matched"`
	Loose   int    `json:"loose"`
	// Missed patches split by what the static scan knows about the patch class.
	MissAttr   int  `json:"miss_attr,omitempty"`   // the class is a static [HarmonyPatch] class, the method is not
	MissManual int  `json:"miss_manual,omitempty"` // the class's assembly calls Harmony.Patch (harmony.manual)
	MissOther  int  `json:"miss_other,omitempty"`  // neither: no attribute and no Harmony.Patch reference found
	Static     int  `json:"static"`                // static facts of the mod (non-dynamic)
	StaticOnly int  `json:"static_only"`           // static facts with no runtime counterpart
	Manual     bool `json:"manual,omitempty"`      // some assembly of the mod calls Harmony.Patch
	NoFacts    bool `json:"no_facts,omitempty"`    // the analysis stored no assembly rows for the mod
}

// RFinding is one static finding of the log's analysis (G4).
type RFinding struct {
	Code string   `json:"code"`
	Sev  string   `json:"sev"`
	Conf string   `json:"conf"`
	Subj []string `json:"subj,omitempty"` // subject mods (package ids), at most 8
	NS   int      `json:"ns"`             // number of subject mods
	// Cluster indexes (Result.Clusters) that could be this finding's effect.
	Attr []int `json:"a,omitempty"` // attributed (any role) to a subject mod
	Src  []int `json:"s,omitempty"` // with a subject mod as source
	Text []int `json:"t,omitempty"` // naming the finding's def / patch file / Harmony target
	// Lib: dep.missing whose dependency is in the log's own list but not in the research library.
	Lib bool `json:"lib,omitempty"`
}

// g3cache holds facts that do not change between logs: they are per instance, not per Loadout.
type g3cache struct {
	owners map[string][][2]string // patch class -> (instance, file)
	static map[string]*instHarmony
}

type instHarmony struct {
	rows    []*store.HarmonyRow
	byPatch map[string][]*store.HarmonyRow // normalised patch method -> facts
	types   map[string]bool                // normalised patch classes
	manual  map[string]bool                // assembly files that call Harmony.Patch
	anyAsm  bool
}

var cache = &g3cache{owners: map[string][][2]string{}, static: map[string]*instHarmony{}}

var kindOf = map[string]string{"PRE": "prefix", "post": "postfix", "TRANS": "transpiler", "FIN": "finalizer", "ILM": "ilmanipulator"}

var (
	prioRe  = regexp.MustCompile(`^\[-?\d+\]`)
	arityRe = regexp.MustCompile("`\\d+")
)

// normName: nested '+' and generic arity do not matter for matching.
func normName(s string) string {
	return arityRe.ReplaceAllString(strings.ReplaceAll(s, "+", "."), "")
}

// shortTarget reduces a method to "Type.Method" with the type's own name, as the HugsLib header
// prints it: "RimWorld.Ability.PreActivate(int)" -> "Ability.PreActivate".
func shortTarget(s string) string {
	if i := strings.IndexByte(s, '('); i >= 0 {
		s = s[:i]
	}
	s = normName(s)
	typ, m := s, ""
	switch {
	case strings.HasSuffix(s, "..ctor"):
		typ, m = s[:len(s)-6], ".ctor"
	case strings.HasSuffix(s, "..cctor"):
		typ, m = s[:len(s)-7], ".cctor"
	default:
		i := strings.LastIndexByte(s, '.')
		if i < 0 {
			return s
		}
		typ, m = s[:i], s[i+1:]
	}
	if i := strings.LastIndexByte(typ, '.'); i >= 0 {
		typ = typ[i+1:]
	}
	return typ + "." + m
}

// splitPatches splits "A.B.Prefix, [800]C.D.Prefix" on commas outside brackets.
func splitPatches(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, c := range s {
		switch c {
		case '(', '[', '<':
			depth++
		case ')', ']', '>':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}

func (c *g3cache) inst(a *app.App, id string) *instHarmony {
	if h := c.static[id]; h != nil {
		return h
	}
	h := &instHarmony{byPatch: map[string][]*store.HarmonyRow{}, types: map[string]bool{}, manual: map[string]bool{}}
	rows, _ := a.Store.HarmonyPatches(store.HarmonyFilter{Instance: id})
	h.rows = rows
	for _, r := range rows {
		k := normName(r.PatchType + "." + r.PatchMethod)
		h.byPatch[k] = append(h.byPatch[k], r)
		h.types[normName(r.PatchType)] = true
	}
	asms, _ := a.Store.Assemblies(id)
	for _, as := range asms {
		h.anyAsm = true
		if as.Manual {
			h.manual[as.File] = true
		}
	}
	c.static[id] = h
	return h
}

// g3 compares the runtime patches with the static facts of the Loadout's instances.
func g3(a *app.App, st *app.LoadoutState, runtime []HPatch) *RHarmony {
	if len(runtime) == 0 {
		return nil
	}
	active := map[string]string{} // instance -> package id
	for _, e := range st.List {
		if m := st.Ix.ByID[e.Instance]; e.Instance != "" && m != nil {
			active[e.Instance] = strings.ToLower(m.PackageID)
		}
	}
	res := &RHarmony{}
	owners := map[string]*RHOwner{} // instance -> tally
	seen := map[string]map[*store.HarmonyRow]bool{}
	for _, hp := range runtime {
		target := shortTarget(hp.Target)
		for _, grp := range hp.Patches {
			kw, list, ok := strings.Cut(grp, ": ")
			if !ok {
				continue
			}
			kind := kindOf[kw]
			for _, p := range splitPatches(list) {
				p = prioRe.ReplaceAllString(p, "")
				i := strings.LastIndexByte(p, '.')
				if i <= 0 {
					continue
				}
				res.Runtime++
				class := p[:i]
				os, ok := cache.owners[class]
				if !ok {
					os, _ = a.Store.TypeOwners(class)
					cache.owners[class] = os
				}
				inst, file := "", ""
				for _, o := range os {
					if active[o[0]] != "" {
						inst, file = o[0], o[1]
						break
					}
				}
				if inst == "" {
					continue // the owner is not in the library: outside coverage
				}
				res.Owned++
				ow := owners[inst]
				h := cache.inst(a, inst)
				if ow == nil {
					ow = &RHOwner{Pkg: active[inst], NoFacts: !h.anyAsm, Manual: len(h.manual) > 0}
					owners[inst] = ow
					seen[inst] = map[*store.HarmonyRow]bool{}
				}
				ow.Owned++
				facts := h.byPatch[normName(p)]
				hit := false
				for _, f := range facts {
					if f.Kind == kind && !f.Dynamic && shortTarget(f.Target) == target {
						hit = true
						seen[inst][f] = true
					}
				}
				switch {
				case hit:
					ow.Matched++
					res.Matched++
				case len(facts) > 0:
					ow.Loose++
					res.Loose++
					for _, f := range facts {
						seen[inst][f] = true
					}
				case h.types[normName(class)]:
					ow.MissAttr++
				case h.manual[file]:
					ow.MissManual++
				default:
					ow.MissOther++
				}
			}
		}
	}
	for inst, ow := range owners {
		for _, f := range cache.inst(a, inst).rows {
			if f.Dynamic || kindOf2[f.Kind] == "" {
				continue
			}
			ow.Static++
			if !seen[inst][f] {
				ow.StaticOnly++
			}
		}
		res.Owners = append(res.Owners, ow)
	}
	return res
}

var kindOf2 = map[string]string{"prefix": "PRE", "postfix": "post", "transpiler": "TRANS", "finalizer": "FIN", "ilmanipulator": "ILM"}

// g4 records the static findings with the clusters that could be their effect.
func g4(a *app.App, st *app.LoadoutState, as *app.AnalysisState, rep *app.LogReport, res *Result) {
	var fs []*finding.Finding
	if d, err := a.DeclFindings(st); err == nil {
		fs = append(fs, d...)
	}
	if as != nil && as.Run != nil {
		if s, err := a.Store.Findings(as.Run.ID); err == nil {
			fs = append(fs, s...)
		}
	}
	pkgOf := func(tok string) string {
		if id := finding.InstID(tok); id != "" {
			if m := st.Ix.ByID[id]; m != nil {
				return strings.ToLower(m.PackageID)
			}
		}
		return ""
	}
	// Per cluster: the attributed packages, the stack's methods, the searchable text.
	type cinfo struct {
		attr, src map[string]bool
		frames    map[string]bool
		text      string
	}
	libMissing := map[string]bool{}
	for _, p := range res.Missing {
		libMissing[strings.ToLower(p)] = true
	}
	cs := make([]cinfo, len(rep.Clusters))
	for i, lc := range rep.Clusters {
		ci := cinfo{attr: map[string]bool{}, src: map[string]bool{}, frames: map[string]bool{}}
		for _, at := range res.Clusters[i].Attr {
			ci.attr[at.Pkg] = true
			if at.Role == "source" {
				ci.src[at.Pkg] = true
			}
		}
		if lc.First != nil {
			for _, fr := range lc.First.Frames {
				ci.frames[shortTarget(fr.Type+"."+fr.Method)] = true
			}
			ci.text = lc.First.Message + "\n" + strings.Join(lc.First.Extra, "\n")
		}
		cs[i] = ci
	}
	for _, f := range fs {
		if libraryCodes[f.Code] {
			continue
		}
		rf := RFinding{Code: f.Code, Sev: f.Severity, Conf: f.Confidence}
		if f.Code == "dep.missing" && libMissing[strings.ToLower(f.Key)] {
			rf.Lib = true
		}
		subj := map[string]bool{}
		for _, s := range f.Subjects {
			if p := pkgOf(s); p != "" && !subj[p] {
				subj[p] = true
				rf.NS++
				if len(rf.Subj) < 8 {
					rf.Subj = append(rf.Subj, p)
				}
			}
		}
		// What text in a cluster would name this finding.
		var words []string
		var target string
		switch {
		case f.Code == "def.override":
			if _, n, ok := strings.Cut(f.Key, "/"); ok {
				words = append(words, n)
			}
		case f.Code == "def.abstract-collision":
			words = append(words, f.Key)
		case f.Code == "def.parent-missing":
			if l, parent, ok := strings.Cut(f.Key, "<"); ok {
				words = append(words, parent)
				if _, id, ok := strings.Cut(l, "/"); ok && !strings.HasPrefix(id, "@") && id != "" {
					words = append(words, id)
				}
			}
		case f.Code == "patch.no-match":
			file, _, _ := strings.Cut(f.Key, "|")
			if b := path.Base(strings.ReplaceAll(file, `\`, "/")); b != "." && b != "/" {
				words = append(words, b)
			}
		case strings.HasPrefix(f.Code, "harmony.") && f.Code != "harmony.manual":
			t, _, _ := strings.Cut(f.Key, "|")
			target = shortTarget(t)
		}
		for i, ci := range cs {
			if logs.Informational(res.Clusters[i].Class) {
				continue
			}
			a, s := false, false
			for p := range subj {
				a = a || ci.attr[p]
				s = s || ci.src[p]
			}
			if a {
				rf.Attr = append(rf.Attr, i)
			}
			if s {
				rf.Src = append(rf.Src, i)
			}
			t := target != "" && ci.frames[target]
			for _, w := range words {
				t = t || containsWord(ci.text, w)
			}
			if t {
				rf.Text = append(rf.Text, i)
			}
		}
		res.Findings = append(res.Findings, rf)
	}
}

// libraryCodes are findings about the research library, not about the player's list: mods it lacks,
// several downloaded versions of one package, hidden or pinned instances.
var libraryCodes = map[string]bool{"entry.missing": true, "pkg.duplicate-installed": true, "mod.hidden": true,
	"mod.hidden-residue": true, "mod.pinned": true}

// containsWord: w occurs in s not as part of a longer identifier.
func containsWord(s, w string) bool {
	if len(w) < 3 {
		return false
	}
	for off := 0; ; {
		i := strings.Index(s[off:], w)
		if i < 0 {
			return false
		}
		i += off
		j := i + len(w)
		if (i == 0 || !identByte(s[i-1])) && (j == len(s) || !identByte(s[j])) {
			return true
		}
		off = i + 1
	}
}

func identByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
