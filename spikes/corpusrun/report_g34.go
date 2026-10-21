package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/logs"
)

// g3report: Harmony static recall (docs/research/00 §3 G3). Only logs written with G3 data count.
func g3report(all []*Result) {
	var rs []*Result
	for _, r := range all {
		if r.Harmony != nil {
			rs = append(rs, r)
		}
	}
	fmt.Printf("\n== G3 Harmony static recall (%d logs with a Harmony header and G3 data)\n", len(rs))
	if len(rs) == 0 {
		return
	}
	var runtime, owned, matched, loose, mAttr, mManual, mOther, missManualMod int
	var static, staticOnly int
	var perLog []float64
	type mod struct {
		logs, owned, matched, loose, mAttr, mManual, mOther, static, staticOnly int
		manual, noFacts                                                         bool
	}
	mods := map[string]*mod{}
	for _, r := range rs {
		h := r.Harmony
		runtime += h.Runtime
		owned += h.Owned
		matched += h.Matched
		loose += h.Loose
		if h.Owned >= 20 {
			perLog = append(perLog, float64(h.Matched)/float64(h.Owned))
		}
		for _, o := range h.Owners {
			mAttr += o.MissAttr
			mManual += o.MissManual
			mOther += o.MissOther
			if o.Manual {
				missManualMod += o.MissAttr + o.MissManual + o.MissOther
			}
			static += o.Static
			staticOnly += o.StaticOnly
			m := mods[o.Pkg]
			if m == nil {
				m = &mod{}
				mods[o.Pkg] = m
			}
			m.logs++
			m.owned += o.Owned
			m.matched += o.Matched
			m.loose += o.Loose
			m.mAttr += o.MissAttr
			m.mManual += o.MissManual
			m.mOther += o.MissOther
			m.static += o.Static
			m.staticOnly += o.StaticOnly
			m.manual = m.manual || o.Manual
			m.noFacts = m.noFacts || o.NoFacts
		}
	}
	missed := owned - matched - loose
	fmt.Printf("runtime patch methods %d; owned by a mod the library resolves %d (%.1f%%, the denominator)\n", runtime, owned, pct(owned, runtime))
	fmt.Printf("recall (same patch method, kind and target): %.1f%%; with the patch method found but another target or kind: +%.1f%% = %.1f%%\n",
		pct(matched, owned), pct(loose, owned), pct(matched+loose, owned))
	fmt.Printf("missed %d (%.1f%%): class is a static [HarmonyPatch] class %d (%.1f%%), assembly calls Harmony.Patch %d (%.1f%%), neither %d (%.1f%%)\n",
		missed, pct(missed, owned), mAttr, pct(mAttr, missed), mManual, pct(mManual, missed), mOther, pct(mOther, missed))
	fmt.Printf("missed patches owned by mods with harmony.manual: %d (%.1f%% of missed)\n", missManualMod, pct(missManualMod, missed))
	sort.Float64s(perLog)
	fmt.Printf("recall per log (logs with >= 20 owned patches, %d): p10 %.2f p25 %.2f median %.2f p75 %.2f p90 %.2f\n",
		len(perLog), q(perLog, .1), q(perLog, .25), q(perLog, .5), q(perLog, .75), q(perLog, .9))
	fmt.Printf("static facts of those owners with no runtime counterpart: %d of %d (%.1f%%; conditional patches, other versions, other overloads)\n",
		staticOnly, static, pct(staticOnly, static))

	// By mod: each mod once (its patches summed over the logs it is in).
	type kv struct {
		k string
		m *mod
	}
	var ms []kv
	full, none, manualMods, manualNone := 0, 0, 0, 0
	for k, m := range mods {
		ms = append(ms, kv{k, m})
		r := float64(m.matched+m.loose) / float64(m.owned)
		if r >= 0.9 {
			full++
		}
		if m.matched+m.loose == 0 {
			none++
			if m.manual {
				manualNone++
			}
		}
		if m.manual {
			manualMods++
		}
	}
	fmt.Printf("mods with runtime patches %d: recall >= 90%% %d (%.1f%%), nothing found %d (%.1f%%; %d of them call Harmony.Patch); mods calling Harmony.Patch %d\n",
		len(ms), full, pct(full, len(ms)), none, pct(none, len(ms)), manualNone, manualMods)
	sort.Slice(ms, func(a, b int) bool {
		ma, mb := ms[a].m.owned-ms[a].m.matched-ms[a].m.loose, ms[b].m.owned-ms[b].m.matched-ms[b].m.loose
		return ma > mb || ma == mb && ms[a].k < ms[b].k
	})
	fmt.Println("mods with the most missed patches (logs, patches, found, missed by: attr / manual / neither, manual flag):")
	for i := 0; i < len(ms) && i < 25; i++ {
		m := ms[i].m
		fl := ""
		if m.manual {
			fl = " manual"
		}
		if m.noFacts {
			fl += " no-facts"
		}
		fmt.Printf("  %4d logs %6d patches %5.1f%% found  %5d / %5d / %5d%s  %s\n", m.logs, m.owned, pct(m.matched+m.loose, m.owned),
			m.mAttr, m.mManual, m.mOther, fl, ms[i].k)
	}
}

// expected classes per finding code: the log errors the finding predicts.
var expectClasses = map[string][]string{
	"patch.no-match":           {logs.ClassPatchFailed},
	"patch.unknown-op":         {logs.ClassPatchFailed, logs.ClassXMLLoad},
	"patch.contested":          {logs.ClassPatchFailed, logs.ClassXMLLoad, logs.ClassXrefMissing},
	"def.parent-missing":       {logs.ClassXMLLoad, logs.ClassXrefMissing},
	"def.abstract-collision":   {logs.ClassXMLLoad, logs.ClassXrefMissing},
	"def.override":             {logs.ClassXMLLoad, logs.ClassXrefMissing, logs.ClassException},
	"dep.missing":              {logs.ClassXMLLoad, logs.ClassXrefMissing, logs.ClassException, logs.ClassPatchFailed},
	"dep.inactive":             {logs.ClassXMLLoad, logs.ClassXrefMissing, logs.ClassException, logs.ClassPatchFailed},
	"harmony.skip-prefix":      {logs.ClassException, logs.ClassHarmony},
	"harmony.multi-transpiler": {logs.ClassException, logs.ClassHarmony},
	"harmony.shared-target":    {logs.ClassException, logs.ClassHarmony},
	"harmony.manual":           {logs.ClassException, logs.ClassHarmony},
}

// g4report: severity calibration (docs/research/00 §3 G4). For each finding code, how often the
// same log has a problem cluster that could be its effect, next to what chance gives: a finding
// with k subject mods in a log where a share b of the resolved mods has an attributed cluster
// would match with 1-(1-b)^k if findings were unrelated to errors.
func g4report(all []*Result) {
	var rs []*Result
	for _, r := range all {
		if r.Findings != nil {
			rs = append(rs, r)
		}
	}
	fmt.Printf("\n== G4 severity calibration (%d logs with G4 data)\n", len(rs))
	if len(rs) == 0 {
		return
	}
	type row struct {
		sev                        map[string]int
		n, logs, subj              int
		attr, src, text, cls, clsT int
		expA, expS, expC           float64
	}
	rows := map[string]*row{}
	for _, r := range rs {
		if r.Resolved == 0 {
			continue
		}
		// per log: the share of resolved mods with an attributed / source / per-class cluster
		attr, src := map[string]bool{}, map[string]bool{}
		byClass := map[string]map[string]bool{}
		for _, c := range r.Clusters {
			if logs.Informational(c.Class) {
				continue
			}
			for _, a := range c.Attr {
				attr[a.Pkg] = true
				if a.Role == "source" {
					src[a.Pkg] = true
				}
				if byClass[c.Class] == nil {
					byClass[c.Class] = map[string]bool{}
				}
				byClass[c.Class][a.Pkg] = true
			}
		}
		share := func(m map[string]bool) float64 { return min(1, float64(len(m))/float64(r.Resolved)) }
		chance := func(b float64, k int) float64 {
			p := 1.0
			for i := 0; i < k; i++ {
				p *= 1 - b
			}
			return 1 - p
		}
		seen := map[string]bool{}
		for _, f := range r.Findings {
			code := f.Code
			if f.Lib {
				code += " (lib)" // the player had the dependency; the library does not
			}
			w := rows[code]
			if w == nil {
				w = &row{sev: map[string]int{}}
				rows[code] = w
			}
			w.n++
			w.sev[f.Sev]++
			w.subj += f.NS
			if !seen[code] {
				seen[code] = true
				w.logs++
			}
			if len(f.Attr) > 0 {
				w.attr++
			}
			if len(f.Src) > 0 {
				w.src++
			}
			if len(f.Text) > 0 {
				w.text++
			}
			w.expA += chance(share(attr), f.NS)
			w.expS += chance(share(src), f.NS)
			ec := expectClasses[f.Code]
			inC := func(idx []int) bool {
				for _, i := range idx {
					for _, k := range ec {
						if r.Clusters[i].Class == k {
							return true
						}
					}
				}
				return false
			}
			if inC(f.Attr) {
				w.cls++
			}
			if inC(f.Text) {
				w.clsT++
			}
			cm := map[string]bool{}
			for _, k := range ec {
				for p := range byClass[k] {
					cm[p] = true
				}
			}
			w.expC += chance(share(cm), f.NS)
		}
	}
	var codes []string
	for k := range rows {
		codes = append(codes, k)
	}
	sort.Slice(codes, func(a, b int) bool {
		return rows[codes[a]].n > rows[codes[b]].n || rows[codes[a]].n == rows[codes[b]].n && codes[a] < codes[b]
	})
	fmt.Println("per code: findings, logs, mean subject mods, severity; share of findings whose log has a problem cluster")
	fmt.Println("  attributed to a subject mod (observed / by chance), with a subject as source (observed / chance),")
	fmt.Println("  of the expected classes attributed to a subject (observed / chance), naming its def / patch file /")
	fmt.Println("  Harmony target in message or stack (any class / expected classes)")
	for _, k := range codes {
		w := rows[k]
		d := float64(w.n)
		fmt.Printf("  %-24s %7d %5d logs %4.1f subj %-28s attr %5.1f%% / %5.1f%%  src %5.1f%% / %5.1f%%  class %5.1f%% / %5.1f%% [%s]  text %5.1f%% / %5.1f%%\n",
			k, w.n, w.logs, float64(w.subj)/d, fmtTop(w.sev, 0),
			100*float64(w.attr)/d, 100*w.expA/d, 100*float64(w.src)/d, 100*w.expS/d,
			100*float64(w.cls)/d, 100*w.expC/d, strings.Join(expectClasses[k], ","),
			100*float64(w.text)/d, 100*float64(w.clsT)/d)
	}
}
