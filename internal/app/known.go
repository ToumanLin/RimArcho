package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/known"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Findings from what the log corpus observed (internal/known, docs/decisions-corpus.md §2):
//
//   - dep.observed-missing: A is enabled, B is not, A's About.xml does not declare B, and in the
//     shared logs with A enabled and B not, an error regularly named B as the provider of
//     something A uses. Warning when it is in at least half of those logs and more than
//     sounds or textures are missing; information otherwise.
//   - combo.known-bad: an error attributed to A that the corpus sees mostly when B is enabled
//     too (or with A at this version) and at most a quarter as often without. Warning when it
//     is in at least half of the logs with the combination and is not a missing sound or
//     texture; information otherwise.
//   - mod.known-error: an error A logs in at least half of the shared logs that enable it,
//     whatever else is enabled. Information: it says the error is the mod's, not the list's.
//
// Severities follow the numbers: a declared dependency left disabled (dep.inactive) made a
// subject mod an error source in 18% of logs against 7.5% by chance; these entries, by
// construction, come with their error in at least 25–30% of the matching logs.

// knownFindings adds the corpus findings for the enabled mods.
func (a *App) knownFindings(active []*store.Mod, activeByPkg, installed map[string][]*store.Mod, add func(*finding.Finding)) {
	data := known.Get()
	if len(data.Prereqs) == 0 && len(data.Combos) == 0 {
		return
	}
	inst := finding.Inst
	pkgOf := func(m *store.Mod) string { return strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam") }
	for _, m := range active {
		p := pkgOf(m)
		d := m.Decl()
		declared := map[string]bool{}
		for _, dep := range append(append([]scanDep{}, toDeps(d.Dependencies)...), toDepsAll(d.DependenciesByVersion)...) {
			declared[strings.TrimSuffix(strings.ToLower(strings.TrimSpace(dep.pkg)), "_steam")] = true
		}
		for _, pr := range data.PrereqsOf(p) {
			if len(activeByPkg[pr.Needs]) > 0 || declared[pr.Needs] {
				continue
			}
			add(observedMissing(a, m, pr, installed[pr.Needs]))
		}
		for _, c := range data.CombosOf(p) {
			switch c.Kind() {
			case known.KindPair:
				for _, o := range activeByPkg[c.Mods[1]] {
					add(comboFinding(m, o, c))
					break
				}
			case known.KindVersion:
				if modVersionIs(m, d.ModVersion, c.Version) {
					add(comboFinding(m, nil, c))
				}
			default:
				add(&finding.Finding{Code: "mod.known-error", Severity: finding.Info, Confidence: finding.Likely, Key: c.Templates[0],
					Subjects: []string{inst(m.ID)},
					Claim: fmt.Sprintf("%s logs a known error in %.0f%% of the shared logs that enable it: %s",
						inst(m.ID), 100*c.Rate(), short(c.Templates[0])),
					Evidence: comboEvidence(c, m, nil)})
			}
		}
	}
}

func toDepsAll(m map[string][]scan.Dependency) []scanDep {
	var out []scanDep
	for _, l := range m {
		out = append(out, toDeps(l)...)
	}
	return out
}

func observedMissing(a *App, m *store.Mod, pr *known.Prereq, cands []*store.Mod) *finding.Finding {
	inst := finding.Inst
	sev := finding.Info
	if !pr.Cosmetic && pr.Rate() >= 0.5 {
		sev = finding.Warning
	}
	what := "content"
	if pr.Cosmetic {
		what = "sounds or textures"
	}
	f := &finding.Finding{Code: "dep.observed-missing", Severity: sev, Confidence: finding.Likely, Key: pr.Needs,
		Subjects: []string{inst(m.ID), "pkg:" + pr.Needs}}
	needs := "pkg:" + pr.Needs
	switch {
	case len(cands) > 0:
		needs = inst(cands[0].ID)
		f.Subjects[1] = needs
		f.Claim = fmt.Sprintf("%s uses %s from %s, which is installed but not enabled (not declared; seen in shared logs)", inst(m.ID), what, needs)
		for _, c := range cands {
			f.Suggestions = append(f.Suggestions, "archo enable "+inst(c.ID))
		}
	case strings.HasPrefix(pr.Needs, "ludeon.rimworld."):
		f.Claim = fmt.Sprintf("%s uses %s from the DLC %s, which is not installed (not declared; seen in shared logs)", inst(m.ID), what, needs)
	default:
		f.Claim = fmt.Sprintf("%s uses %s from %s, which is not installed (not declared; seen in shared logs)", inst(m.ID), what, needs)
		if w, ok := a.WorkshopFor(pr.Needs); ok {
			f.Suggestions = append(f.Suggestions, "subscribe: https://steamcommunity.com/sharedfiles/filedetails/?id="+w.WorkshopID)
		}
	}
	ev := []finding.Evidence{{Source: finding.Community, Ref: "log corpus", Text: fmt.Sprintf(
		"log corpus: %d of %d shared logs with pkg:%s enabled and pkg:%s not (%.0f%%, %d players) had an error naming pkg:%s as the provider of what was missing",
		pr.Logs, pr.Without, pr.Mod, pr.Needs, 100*pr.Rate(), pr.Players, pr.Needs)}}
	if len(pr.Items) > 0 {
		ev = append(ev, finding.Evidence{Source: finding.Community, Ref: "log corpus", Text: "missing: " + strings.Join(pr.Items, "; ")})
	}
	if pr.Example != "" {
		ev = append(ev, finding.Evidence{Source: finding.Community, Ref: "log corpus", Text: "typical error (" + pr.Class + "): " + short(pr.Example)})
	}
	if pr.LoadAfter {
		ev = append(ev, finding.Evidence{Source: finding.Declared, Ref: inst(m.ID), Text: "About.xml loadAfter lists pkg:" + pr.Needs + " (an optional dependency, without MayRequire on everything that uses it)"})
	}
	if t := fixText(pr.Fix, "enabled "+pr.Needs); t != "" {
		ev = append(ev, finding.Evidence{Source: finding.Community, Ref: "log corpus", Text: t})
	}
	f.Evidence = ev
	return f
}

func comboFinding(m, o *store.Mod, c *known.Combo) *finding.Finding {
	inst := finding.Inst
	sev := finding.Info
	if c.Rate() >= 0.5 && !cosmeticTemplate(c.Class, c.Templates[0]) {
		sev = finding.Warning
	}
	f := &finding.Finding{Code: "combo.known-bad", Severity: sev, Confidence: finding.Possible, Subjects: []string{inst(m.ID)}}
	if o != nil {
		f.Subjects = append(f.Subjects, inst(o.ID))
		f.Key = c.Mods[1] + "|" + c.Templates[0]
		f.Claim = fmt.Sprintf("%s together with %s: %.0f%% of shared logs with both show an error from %s, %.0f%% without %s",
			inst(m.ID), inst(o.ID), 100*c.Rate(), inst(m.ID), 100*c.RateWithout(), inst(o.ID))
		f.Suggestions = []string{"archo disable " + inst(o.ID), "archo disable " + inst(m.ID)}
	} else {
		f.Key = "v" + c.Version + "|" + c.Templates[0]
		f.Claim = fmt.Sprintf("%s version %s: %.0f%% of shared logs with this version show an error from it, %.0f%% with other versions",
			inst(m.ID), c.Version, 100*c.Rate(), 100*c.RateWithout())
		f.Suggestions = []string{"archo mod " + inst(m.ID)}
	}
	f.Evidence = comboEvidence(c, m, o)
	return f
}

func comboEvidence(c *known.Combo, m, o *store.Mod) []finding.Evidence {
	src := finding.Community
	var ev []finding.Evidence
	switch c.Kind() {
	case known.KindPair:
		ev = append(ev, finding.Evidence{Source: src, Ref: "log corpus", Text: fmt.Sprintf(
			"log corpus: with pkg:%s and pkg:%s %d of %d shared logs (%d players); with pkg:%s alone %d of %d",
			c.Mods[0], c.Mods[1], c.Logs, c.With, c.Players, c.Mods[0], c.WithoutErr, c.Without)})
	case known.KindVersion:
		ev = append(ev, finding.Evidence{Source: src, Ref: "log corpus", Text: fmt.Sprintf(
			"log corpus: pkg:%s %s in %d of %d shared logs (%d players); other versions %d of %d",
			c.Mods[0], c.Version, c.Logs, c.With, c.Players, c.WithoutErr, c.Without)})
	default:
		ev = append(ev, finding.Evidence{Source: src, Ref: "log corpus", Text: fmt.Sprintf(
			"log corpus: %d of %d shared logs that enable pkg:%s (%d players)", c.Logs, c.With, c.Mods[0], c.Players)})
	}
	for _, t := range c.Templates {
		ev = append(ev, finding.Evidence{Source: src, Ref: "log corpus", Text: "error (" + c.Class + "): " + short(t)})
	}
	want := "disabled " + c.Mods[0]
	if c.Kind() == known.KindPair {
		want = "disabled " + c.Mods[1]
	} else if c.Kind() == known.KindVersion {
		want = "updated " + c.Mods[0]
	}
	if t := fixText(c.Fix, want); t != "" {
		ev = append(ev, finding.Evidence{Source: src, Ref: "log corpus", Text: t})
	}
	if c.G5 != nil {
		ev = append(ev, finding.Evidence{Source: src, Ref: "pattern library G5", Text: "pattern library: " + c.G5.Text()})
	}
	return ev
}

// modVersionIs: HugsLib logs a mod's version from About.xml <modVersion> or, more often, from
// About/Manifest.xml <version> (the corpus entries so far are all the latter).
func modVersionIs(m *store.Mod, modVersion, want string) bool {
	if strings.TrimSpace(modVersion) == want {
		return true
	}
	if m.RealPath == "" {
		return false
	}
	b, err := os.ReadFile(filepath.Join(m.RealPath, "About", "Manifest.xml"))
	if err != nil {
		return false
	}
	mt := manifestVersionRe.FindSubmatch(b)
	return mt != nil && strings.TrimSpace(string(mt[1])) == want
}

var manifestVersionRe = regexp.MustCompile(`(?is)<version>\s*([^<]*?)\s*</version>`)

// fixText describes the session evidence: how often the error was gone in the player's next
// upload, after the change that matters, and without any change to these mods (the control).
func fixText(f *known.Fix, change string) string {
	if f == nil || f.Pairs == 0 {
		return ""
	}
	s := fmt.Sprintf("next upload by the same player: error gone in %d of %d", f.Gone, f.Pairs)
	if n := f.Changes[change]; n > 0 {
		s += fmt.Sprintf(", %d of them after %s", n, strings.Replace(change, " ", " pkg:", 1))
	}
	if f.Same > 0 {
		s += fmt.Sprintf("; with these mods unchanged gone in %d of %d", f.SameGone, f.Same)
	}
	return s
}

func cosmeticTemplate(class, tpl string) bool {
	return class == "texture-missing" || strings.Contains(tpl, "SoundDef")
}

// short keeps a template to one readable line.
func short(s string) string {
	if _, rest, ok := strings.Cut(s, "|"); ok {
		s = rest
	}
	r := []rune(s)
	if len(r) > 160 {
		return string(r[:160]) + "…"
	}
	return s
}
