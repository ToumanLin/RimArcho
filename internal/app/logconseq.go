package app

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/logs"
)

// Consequences of a Def file that failed to load (03 §6.3). "Exception loading def from file
// F" means every def F defines is missing from the game, so each later "Could not resolve
// cross-reference … named X" for one of them, and the config checks that then read a null
// stat ("defines the stat base  more than once", the stat's name empty), are the same fault.
// They are marked as consequences of that cluster and listed under it, instead of standing
// as dozens of clusters whose referrers look like suspects (MCP eval h1: one misspelled
// class in a patch to Core's Stats_Basics_General.xml gave 64 clusters, and the 1,000-line
// one named five mods that merely use the Beauty stat).

var (
	defFileExcRe = regexp.MustCompile(`^Exception loading def from file ([^\s:]+\.xml)`)
	// The game prints the stat's label; a stat that did not load prints nothing.
	nullStatRe = regexp.MustCompile(`^Config error in \S+: defines the stat base  more than once`)
)

func (cx *attrCtx) consequences(rep *LogReport) {
	var only map[string]bool
	for _, root := range rep.Clusters {
		m := defFileExcRe.FindStringSubmatch(root.First.Message)
		if m == nil || root.ConsequenceOf != "" {
			continue
		}
		if only == nil {
			only = map[string]bool{}
			for _, e := range cx.st.List {
				if e.Instance != "" {
					only[e.Instance] = true
				}
			}
		}
		ds, err := cx.a.Store.DefsInFile(m[1], only)
		if err != nil || len(ds) == 0 {
			continue
		}
		lost := map[string]string{} // defName → def type
		var names []string
		stats := false
		for _, d := range ds {
			if _, ok := lost[d.DefName]; !ok {
				names = append(names, d.Type+"/"+d.DefName)
			}
			lost[d.DefName] = d.Type
			stats = stats || d.Type == "StatDef"
		}
		var ids []string
		lines := 0
		for _, lc := range rep.Clusters {
			if lc == root || lc.ConsequenceOf != "" || lc.Consequences > 0 {
				continue
			}
			why := ""
			if lc.Class == logs.ClassXrefMissing {
				if typ, name, ok := xrefName(lc.First.Message); ok && lost[name] != "" && lost[name] == typ[strings.LastIndexByte(typ, '.')+1:] {
					why = lost[name] + " " + name + " does not exist"
				}
			} else if stats && nullStatRe.MatchString(lc.First.Message) {
				why = "a stat it defines is null here (the stat's name is empty)"
			}
			if why == "" {
				continue
			}
			lc.ConsequenceOf = root.ID
			lc.Details = append([]string{fmt.Sprintf("consequence of log:%s: %s failed to load, so %s", root.ID, m[1], why)}, lc.Details...)
			lc.Confidence, lc.Why = finding.Possible, "consequence of log:"+root.ID
			ids = append(ids, "log:"+lc.ID)
			lines += lc.Count
		}
		if len(ids) == 0 {
			continue
		}
		root.Consequences, root.ConsequentLines = len(ids), lines
		sort.Strings(names)
		root.Details = append(root.Details, fmt.Sprintf("consequences: %d lines in %d clusters (%s): %s failed to load, so its defs are missing (%s)",
			lines, len(ids), sampleList(ids, 6), m[1], sampleList(names, 8)))
	}
}

// sampleList is up to n items joined, with "+k more".
func sampleList(ss []string, n int) string {
	if len(ss) <= n {
		return strings.Join(ss, ", ")
	}
	return strings.Join(ss[:n], ", ") + fmt.Sprintf(" +%d more", len(ss)-n)
}

// DefFileFailed names the cluster of the player's newest log that says the def file base
// failed to load ("log:E3"), from the cached report only (as a mod page reads the log); ""
// when there is none or the log was not analysed yet. get_def uses it: the definition is
// there, but the game did not load it.
func (a *App) DefFileFailed(st *LoadoutState, base string) string {
	cur, prev := a.LogPaths(st)
	p := cur
	if p == "" {
		p = prev
	}
	if p == "" {
		return ""
	}
	old := a.LogCacheOnly
	a.LogCacheOnly = true
	defer func() { a.LogCacheOnly = old }()
	rep, err := a.AnalyzeLog(p)
	if err != nil {
		return ""
	}
	for _, lc := range rep.Clusters {
		if m := defFileExcRe.FindStringSubmatch(lc.First.Message); m != nil && strings.EqualFold(m[1], base) {
			return "log:" + lc.ID
		}
	}
	return ""
}
