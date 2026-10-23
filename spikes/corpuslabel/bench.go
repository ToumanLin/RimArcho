package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Benchmark v2 (G6): labelled clusters in two tiers, reusable as agent tasks (G7).
//
//	tier 1  patch-failed, xref-missing, texture-missing, xml-load: the evidence packet decides
//	tier 2  exception, harmony-stack, assembly, static-ctor: code-level research
//
// assembly and static-ctor are not RimArcho classes: they are exception / harmony-stack
// clusters whose key matches the patterns below (static constructor first).

var (
	staticRe   = regexp.MustCompile(`(?i)static constructor|TypeInitializationException`)
	assemblyRe = regexp.MustCompile(`(?i)TypeLoadException|ReflectionTypeLoadException|Could not resolve type .{0,80}token|Could not load type|Could not load file or assembly|Cannot resolve dependency to assembly|MissingMethodException|MissingFieldException`)
)

// benchClasses in order, with the item prefix and tier.
var benchClasses = []struct {
	Name, Prefix string
	Tier         int
}{
	{"patch-failed", "PF", 1}, {"xref-missing", "XR", 1}, {"texture-missing", "TX", 1}, {"xml-load", "XL", 1},
	{"exception", "EX", 2}, {"harmony-stack", "HS", 2}, {"assembly", "AS", 2}, {"static-ctor", "SC", 2},
}

// benchClass maps a RimArcho cluster class and key to the benchmark class ("" = not sampled).
func benchClass(class, key string) string {
	switch class {
	case "patch-failed", "xref-missing", "texture-missing", "xml-load":
		return class
	case "exception", "harmony-stack":
		if staticRe.MatchString(key) {
			return "static-ctor"
		}
		if assemblyRe.MatchString(key) {
			return "assembly"
		}
		return class
	}
	return ""
}

// family groups near-identical templates so one mod's many patch lines do not fill a stratum:
// the "[Mod name]" prefix, else the first 40 characters after the class.
func family(tpl string) string {
	_, t, _ := strings.Cut(tpl, "|")
	if strings.HasPrefix(t, "[") {
		if i := strings.Index(t, "]"); i > 0 {
			return t[:i+1]
		}
	}
	if len(t) > 40 {
		t = t[:40]
	}
	return t
}

type v4Cluster struct {
	Key   string `json:"key"`
	Tpl   string `json:"tpl"`
	Class string `json:"class"`
	Count int    `json:"count"`
	Conf  string `json:"conf"`
	Attr  []struct {
		Pkg  string   `json:"pkg"`
		Role string   `json:"role"`
		Ev   []string `json:"ev"`
	} `json:"attr"`
}

// RimArchoV4 is RimArcho's verdict on a sampled cluster in the final corpus run (kept apart
// from the sample and the packets: labellers and agents never see it).
type RimArchoV4 struct {
	Item  string  `json:"item"`
	Conf  string  `json:"conf"`
	Attr  []RAttr `json:"attr"`
	Found bool    `json:"found"`
}

// benchSample draws n1 clusters per tier-1 class and n2 per tier-2 class from logs with
// library coverage >= minCov: about 60% "head" (the most frequent templates by number of
// logs, one cluster each, one per family) and the rest "tail" (clusters drawn uniformly from
// the other templates, again one per template and family). At most one cluster per log over
// the whole sample; logs of the exclude samples (the pilot) are skipped.
//
// Benchmark v3 (G12): items continue the numbering of the exclude samples (PF21 after v2's
// PF20), candidates with higher library coverage come first (coverage in steps of 5%, then the
// hash), and with balance each class is split about evenly between clusters RimArcho names a
// source for and clusters it is silent on (a proxy for difficulty, which is known only after
// labelling: v2's silent clusters were mostly L2/L3).
func benchSample(results, out string, n1, n2 int, minCov float64, seed string, exclude []string, balance bool) error {
	skip := map[string]bool{}
	start := map[string]int{} // item prefix -> highest number in the exclude samples
	for _, x := range exclude {
		if x == "" {
			continue
		}
		its, err := readSample(x)
		if err != nil {
			return err
		}
		for _, it := range its {
			skip[it.Gist] = true
			if len(it.Item) > 2 {
				if k, err := strconv.Atoi(it.Item[2:]); err == nil && k > start[it.Item[:2]] {
					start[it.Item[:2]] = k
				}
			}
		}
	}
	f, err := os.Open(results)
	if err != nil {
		return err
	}
	defer f.Close()
	byClass := map[string]map[string][]cand{} // bclass -> tpl -> clusters
	tplLogs := map[string]int{}
	tplClass := map[string]string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<30)
	nLogs := 0
	for sc.Scan() {
		var r struct {
			ID         string      `json:"id"`
			Mods       int         `json:"mods"`
			Resolved   int         `json:"resolved"`
			AnalysisOK bool        `json:"analysis_ok"`
			Err        string      `json:"err"`
			Clusters   []v4Cluster `json:"clusters"`
		}
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.Mods == 0 || r.Err != "" || skip[r.ID] {
			continue
		}
		cov := float64(r.Resolved) / float64(r.Mods)
		if cov < minCov {
			continue
		}
		nLogs++
		seen := map[string]bool{}
		for _, c := range r.Clusters {
			bc := benchClass(c.Class, c.Key)
			if bc == "" {
				continue
			}
			if !seen[c.Tpl] {
				seen[c.Tpl] = true
				tplLogs[c.Tpl]++
			}
			tplClass[c.Tpl] = bc
			if byClass[bc] == nil {
				byClass[bc] = map[string][]cand{}
			}
			byClass[bc][c.Tpl] = append(byClass[bc][c.Tpl], cand{r.ID, c.Key, c.Tpl, pickHash(seed, r.ID, c.Key), c.Count, cov, c})
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	usedLog := map[string]bool{}
	var items []Item
	var ra []RimArchoV4
	for _, bcl := range benchClasses {
		n := n1
		if bcl.Tier == 2 {
			n = n2
		}
		nHead := int(math.Ceil(0.6 * float64(n)))
		tpls := byClass[bcl.Name]
		var order []string
		for t := range tpls {
			order = append(order, t)
		}
		sort.Slice(order, func(i, j int) bool {
			if tplLogs[order[i]] != tplLogs[order[j]] {
				return tplLogs[order[i]] > tplLogs[order[j]]
			}
			return order[i] < order[j]
		})
		usedTpl, usedFam := map[string]bool{}, map[string]bool{}
		k := 0
		quota := (n + 1) / 2
		nGroup := map[bool]int{} // named by RimArcho -> items taken
		full := func(x cand) bool { return balance && nGroup[x.named()] >= quota }
		take := func(x cand, stratum string) {
			usedLog[x.gist], usedTpl[x.tpl], usedFam[family(x.tpl)] = true, true, true
			k++
			nGroup[x.named()]++
			id := fmt.Sprintf("%s%02d", bcl.Prefix, start[bcl.Prefix]+k)
			items = append(items, Item{Item: id, Gist: x.gist, Key: x.key, Tpl: x.tpl, Class: x.cl.Class, Count: x.count, Cov: x.cov,
				TplLogs: tplLogs[x.tpl], Tier: bcl.Tier, Bench: bcl.Name, Stratum: stratum})
			r := RimArchoV4{Item: id, Conf: x.cl.Conf, Found: true}
			for _, a := range x.cl.Attr {
				r.Attr = append(r.Attr, RAttr{Pkg: a.Pkg, Role: a.Role, Evidence: a.Ev})
			}
			ra = append(ra, r)
		}
		pick := func(cs []cand) (cand, bool) {
			sort.Slice(cs, func(i, j int) bool { return better(cs[i], cs[j]) })
			for pass := 0; pass < 2; pass++ {
				for _, x := range cs {
					if !usedLog[x.gist] && (pass == 1 || !full(x)) {
						return x, true
					}
				}
			}
			return cand{}, false
		}
		for _, t := range order {
			if k == nHead {
				break
			}
			if usedFam[family(t)] {
				continue
			}
			if x, ok := pick(tpls[t]); ok {
				take(x, "head")
			}
		}
		// tail: uniform over (log, template) pairs, at most tailCap items per template and per
		// family (classes such as texture-missing have only a few templates).
		var rest []cand
		for _, t := range order {
			seenLog := map[string]bool{}
			for _, x := range tpls[t] {
				if !seenLog[x.gist] {
					seenLog[x.gist] = true
					rest = append(rest, x)
				}
			}
		}
		sort.Slice(rest, func(i, j int) bool { return better(rest[i], rest[j]) })
		tailCap := 4
		if bcl.Tier == 2 {
			tailCap = 2
		}
		nTpl, nFam := map[string]int{}, map[string]int{}
		for _, it := range items {
			if it.Bench == bcl.Name {
				nTpl[it.Tpl]++
				nFam[family(it.Tpl)]++
			}
		}
		// pass 0 keeps the balance, pass 1 drops it, pass 2 (a class still short, such as
		// texture-missing, whose templates mostly share one family) drops the family cap.
		for pass := 0; pass < 3; pass++ {
			for _, x := range rest {
				if k == n {
					break
				}
				if usedLog[x.gist] || nTpl[x.tpl] >= tailCap || (pass < 2 && nFam[family(x.tpl)] >= tailCap) || (pass == 0 && full(x)) {
					continue
				}
				nTpl[x.tpl]++
				nFam[family(x.tpl)]++
				take(x, "tail")
			}
		}
		fmt.Printf("%-16s %3d items (%d templates in %d covered logs; RimArcho names a source in %d)\n", bcl.Name, k, len(tpls), nLogs, nGroup[true])
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	if err := writeJSONL(out, items); err != nil {
		return err
	}
	return writeJSONL(filepath.Join(filepath.Dir(out), "rimarcho-v4.jsonl"), ra)
}

type cand struct {
	gist, key, tpl, hash string
	count                int
	cov                  float64
	cl                   v4Cluster
}

// named: RimArcho's final corpus run names a source for this cluster.
func (c cand) named() bool {
	for _, a := range c.cl.Attr {
		if a.Role == "source" {
			return true
		}
	}
	return false
}

// better orders candidates: higher library coverage (in 5% steps) first, then the hash.
func better(a, b cand) bool {
	ba, bb := int(a.cov*20+1e-9), int(b.cov*20+1e-9)
	if ba != bb {
		return ba > bb
	}
	return a.hash < b.hash
}

func writeJSONL[T any](path string, xs []T) error {
	o, err := os.Create(path)
	if err != nil {
		return err
	}
	defer o.Close()
	w := bufio.NewWriter(o)
	for _, x := range xs {
		j, _ := json.Marshal(x)
		w.Write(append(j, '\n'))
	}
	return w.Flush()
}

func readRimArchoV4(path string) (map[string]*RimArchoV4, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]*RimArchoV4{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var r RimArchoV4
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, err
		}
		out[r.Item] = &r
	}
	return out, sc.Err()
}
