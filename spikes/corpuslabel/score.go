package main

import (
	"fmt"
	"sort"
	"strings"
)

// sources are the packageIds a label blames (role source).
func sources(l *Label) map[string]bool {
	out := map[string]bool{}
	if l == nil || l.Verdict != "cause" {
		return out
	}
	for _, c := range l.Causes {
		if c.Role == "source" {
			out[strings.ToLower(c.Pkg)] = true
		}
	}
	return out
}

func allCauses(l *Label) map[string]bool {
	out := map[string]bool{}
	if l == nil || l.Verdict != "cause" {
		return out
	}
	for _, c := range l.Causes {
		out[strings.ToLower(c.Pkg)] = true
	}
	return out
}

func meets(a, b map[string]bool) bool {
	for k := range a {
		if b[k] {
			return true
		}
	}
	return false
}

// agree: both name no source, or the source sets share a mod.
func agree(a, b map[string]bool) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return meets(a, b)
}

func pct(n, d int) string {
	if d == 0 {
		return "   -  "
	}
	return fmt.Sprintf("%5.1f%%", 100*float64(n)/float64(d))
}

// score compares the labeller with the reviewer (disagreement rate), then RimArcho's
// attribution with the reviewed labels (the accuracy estimate), by class.
func score(samplePath, packetDir, labelDir, reviewDir string) error {
	items, ps, err := loadAll(samplePath, packetDir)
	if err != nil {
		return err
	}
	lab, _, err := readLabels(labelDir)
	if err != nil {
		return err
	}
	rev, _, err := readLabels(reviewDir)
	if err != nil {
		return err
	}
	var names []string
	for n := range items {
		names = append(names, n)
	}
	sort.Strings(names)

	// --- labeller vs reviewer
	invalid := 0
	var n, src, verdict, fix, stage, ctype int
	fmt.Println("== Labeller vs reviewer")
	for _, name := range names {
		l, r := lab[name], rev[name]
		if r == nil {
			continue
		}
		if l == nil || len(validate(l, items, ps)) > 0 {
			invalid++
		}
		if l == nil {
			fmt.Printf("  %s no label\n", name)
			n++
			continue
		}
		n++
		ok := agree(sources(l), sources(r))
		if ok {
			src++
		}
		if (l.Verdict == "cause") == (r.Verdict == "cause") {
			verdict++
		}
		if l.Fix.Kind == r.Fix.Kind {
			fix++
		}
		if l.Stage == r.Stage {
			stage++
		}
		if l.CauseType == r.CauseType {
			ctype++
		}
		if !ok {
			fmt.Printf("  %s %-15s labeller %-7s %v | reviewer %-7s %v\n", name, items[name].Class, l.Verdict, keysOf(sources(l)), r.Verdict, keysOf(sources(r)))
		}
	}
	fmt.Printf("  items %d, labeller labels failing validation %d\n", n, invalid)
	fmt.Printf("  agree on source mods %s (disagreement %s)\n", pct(src, n), pct(n-src, n))
	fmt.Printf("  agree on cause/no cause %s, fix kind %s, stage %s, cause type %s\n", pct(verdict, n), pct(fix, n), pct(stage, n), pct(ctype, n))

	// --- RimArcho vs reviewed labels
	type row struct{ n, gold, hit, wrong, silent, over, quiet, any, anyN int }
	by := map[string]*row{}
	tot := &row{}
	fmt.Println("\n== RimArcho vs reviewed labels")
	for _, name := range names {
		r, p := rev[name], ps[name]
		if r == nil || p == nil {
			continue
		}
		cls := p.Class
		if by[cls] == nil {
			by[cls] = &row{}
		}
		ra, rany := map[string]bool{}, map[string]bool{}
		for _, at := range p.RimArcho.Attr {
			rany[at.Pkg] = true
			if at.Role == "source" {
				ra[at.Pkg] = true
			}
		}
		gold := sources(r)
		var res string
		for _, x := range []*row{by[cls], tot} {
			x.n++
			switch {
			case len(gold) > 0 && meets(ra, gold):
				x.hit++
				res = "hit"
			case len(gold) > 0 && len(ra) > 0:
				x.wrong++
				res = "WRONG"
			case len(gold) > 0:
				x.silent++
				res = "silent"
			case len(ra) > 0:
				x.over++
				res = "OVERCLAIM"
			default:
				x.quiet++
				res = "both none"
			}
			if g := allCauses(r); len(g) > 0 {
				x.anyN++
				if meets(rany, g) {
					x.any++
				}
			}
			if len(gold) > 0 {
				x.gold++
			}
		}
		fmt.Printf("  %s %-15s %-9s rimarcho %v | reviewed %s %v\n", name, cls, res, keysOf(ra), r.Verdict, keysOf(allCauses(r)))
	}
	var classes []string
	for c := range by {
		classes = append(classes, c)
	}
	sort.Strings(classes)
	fmt.Println("\n  class             n  gold-src  hit wrong silent overclaim both-none  agree   src-precision  src-recall  any-role-recall")
	line := func(c string, x *row) {
		agreeN := x.hit + x.quiet
		fmt.Printf("  %-15s %3d %6d %7d %5d %6d %8d %9d   %s  %s        %s     %s\n", c, x.n, x.gold, x.hit, x.wrong, x.silent, x.over, x.quiet,
			pct(agreeN, x.n), pct(x.hit, x.hit+x.wrong+x.over), pct(x.hit, x.gold), pct(x.any, x.anyN))
	}
	for _, c := range classes {
		line(c, by[c])
	}
	line("all", tot)
	return nil
}

func keysOf(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
