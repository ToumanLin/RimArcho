package main

import (
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/known"
)

// verify loads corpus mod lists into a research environment (corpusrun setup) and checks
// that RimArcho raises the embedded dataset's findings for them. Per entry kind: lists where
// the dataset predicts the finding (its mods enabled as the entry says, and present in the
// research library), and how many of them got it. Prints counts and packageIds only.
func verify(home, userData, parsedPath, mm string, perKind int) error {
	os.Setenv("RIMARCHO_HOME", home)
	a, err := app.Open(home)
	if err != nil {
		return err
	}
	defer a.Close()
	ls, err := readCorpus(parsedPath, "", mm)
	if err != nil {
		return err
	}
	data := known.Get()
	type want struct {
		code, key, subj string
	}
	expect := func(l *logRec) []want {
		var out []want
		for _, p := range data.Prereqs {
			if l.set[p.Mod] && !l.set[p.Needs] {
				out = append(out, want{"dep.observed-missing", p.Needs, p.Mod})
			}
		}
		for _, c := range data.Combos {
			switch c.Kind() {
			case known.KindPair:
				if l.set[c.Mods[0]] && l.set[c.Mods[1]] {
					out = append(out, want{"combo.known-bad", c.Mods[1] + "|" + c.Templates[0], c.Mods[0]})
				}
			case known.KindMod:
				if l.set[c.Mods[0]] {
					out = append(out, want{"mod.known-error", c.Templates[0], c.Mods[0]})
				}
			}
		}
		return out
	}
	r := rand.New(rand.NewSource(10))
	idx := r.Perm(len(ls))
	picked := map[string]int{}
	tried, got := map[string]int{}, map[string]int{}
	notInLib := map[string]int{}
	examples := map[string][]string{}
	for _, i := range idx {
		l := ls[i]
		ws := expect(l)
		need := false
		for _, w := range ws {
			if picked[w.code] < perKind {
				need = true
			}
		}
		if !need {
			continue
		}
		if err := writeModsConfig(userData, l.version, l.order); err != nil {
			return err
		}
		st, err := a.Loadout()
		if err != nil {
			return err
		}
		fs, err := a.DeclFindings(st)
		if err != nil {
			return err
		}
		have := map[string]bool{}
		for _, f := range fs {
			if m := st.Ix.ByID[f.SubjectInstance()]; m != nil {
				have[f.Code+"\x00"+f.Key+"\x00"+strings.ToLower(m.PackageID)] = true
				if strings.Contains(f.Code, "known") || f.Code == "dep.observed-missing" {
					if len(examples[f.Code]) < 3 {
						examples[f.Code] = append(examples[f.Code], fmt.Sprintf("[%s] %s", f.Severity, finding.Expand(f.Claim, func(id string) string {
							if m := st.Ix.ByID[id]; m != nil {
								return m.PackageID
							}
							return id
						})))
					}
				}
			}
		}
		installed := map[string]bool{}
		for _, e := range st.List {
			if e.Instance != "" {
				if m := st.Ix.ByID[e.Instance]; m != nil {
					installed[strings.TrimSuffix(strings.ToLower(m.PackageID), "_steam")] = true
				}
			}
		}
		picked0 := map[string]bool{}
		for _, w := range ws {
			if !installed[w.subj] || w.code == "combo.known-bad" && !installed[strings.SplitN(w.key, "|", 2)[0]] {
				notInLib[w.code]++
				continue
			}
			tried[w.code]++
			picked0[w.code] = true
			if have[w.code+"\x00"+w.key+"\x00"+w.subj] {
				got[w.code]++
			}
		}
		for c := range picked0 {
			picked[c]++
		}
		done := true
		for _, c := range []string{"dep.observed-missing", "combo.known-bad", "mod.known-error"} {
			done = done && picked[c] >= perKind
		}
		if done {
			break
		}
	}
	var codes []string
	for c := range tried {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	fmt.Println("verify: corpus mod lists in the research environment, expected findings raised")
	for _, c := range codes {
		fmt.Printf("  %-22s lists %3d  expected %4d  raised %4d (%.0f%%)  not in the research library %d\n", c, picked[c], tried[c], got[c], pct(got[c], tried[c]), notInLib[c])
		for _, e := range examples[c] {
			fmt.Println("      e.g. " + e)
		}
	}
	return nil
}

func writeModsConfig(userData, version string, active []string) error {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<ModsConfigData>\n  <version>" + version + "</version>\n  <activeMods>\n")
	for _, p := range active {
		b.WriteString("    <li>" + p + "</li>\n")
	}
	b.WriteString("  </activeMods>\n</ModsConfigData>\n")
	return os.WriteFile(env.ModsConfigPath(userData), []byte(b.String()), 0o644)
}
