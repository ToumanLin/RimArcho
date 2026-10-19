package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/cmds"
	"github.com/rimarcho/rimarcho/internal/identity"
	"github.com/rimarcho/rimarcho/internal/order"
	"github.com/rimarcho/rimarcho/internal/store"
)

// The full-screen detail's sub-sections (06 §6.3): Def, patches, Harmony, rules and the
// full history, loaded when the full screen opens for a mod. 1–5 jump to them.

type fullSec struct {
	title string
	count int // -1: no count
	lines []string
}

type fullSecs struct {
	id   string
	secs []fullSec
}

type fullSecsMsg struct{ s *fullSecs }

// secCap: lines shown per sub-section; the technical pages (tab) have all of them.
const secCap = 60

var countRe = regexp.MustCompile(`(?:defs|harmony|patches): (\d+)`)

// loadFullSecs runs the commands behind the sub-sections off the key path.
func (m *model) loadFullSecs() tea.Cmd {
	it := m.current()
	if it == nil || it.id == "" || m.st == nil || (m.fsecs != nil && m.fsecs.id == it.id) {
		return nil
	}
	mod := m.st.Ix.ByID[it.id]
	if mod == nil {
		return nil
	}
	a, ix, ref, id := m.app, m.st.Ix, it.ref, it.id
	m.fsecs = &fullSecs{id: id} // loading
	return func() tea.Msg {
		s := &fullSecs{id: id}
		run := func(title string, argv ...string) {
			out, _ := cmds.Execute(a, argv)
			ls := strings.Split(strings.TrimRight(out, "\n"), "\n")
			sec := fullSec{title: title, count: -1}
			if len(ls) > 0 {
				if mt := countRe.FindStringSubmatch(ls[0]); mt != nil {
					fmt.Sscan(mt[1], &sec.count)
				}
				if strings.HasPrefix(ls[0], "#") { // "#ref Name  defs: N": the title says it
					ls = ls[1:]
				}
			}
			for i, l := range ls {
				t := strings.TrimSpace(l)
				if t == "list:" || t == "next:" || strings.HasPrefix(t, "archo ") {
					continue
				}
				if strings.HasSuffix(t, ":") && (i+1 == len(ls) || indentOf(ls[i+1]) <= indentOf(l)) {
					continue // a heading with nothing under it
				}
				sec.lines = append(sec.lines, l)
			}
			s.secs = append(s.secs, sec)
		}
		run(T("Defs"), "def", "--mod", ref)
		run(T("patches"), "patches", "--mod", ref)
		run("Harmony", "harmony", "--mod", ref)
		s.secs = append(s.secs, rulesSec(a, ix, mod))
		s.secs = append(s.secs, historySec(a, ix, id))
		return fullSecsMsg{s}
	}
}

func indentOf(s string) int { return len(s) - len(strings.TrimLeft(s, " ")) }

// rulesSec: the order the mod declares in About.xml and the player's rules that name it.
func rulesSec(a *app.App, ix *app.Index, mod *store.Mod) fullSec {
	sec := fullSec{title: T("rules"), count: -1}
	d := mod.Decl()
	name := func(pkg string) string {
		for _, x := range ix.Mods {
			if x.PackageID == strings.ToLower(pkg) && x.Status == identity.Live {
				return x.DisplayName()
			}
		}
		return pkg
	}
	for _, g := range []struct {
		f    string
		pkgs []string
	}{
		{"loads after %s (About.xml)", append(append([]string{}, d.LoadAfter...), d.ForceLoadAfter...)},
		{"loads before %s (About.xml)", append(append([]string{}, d.LoadBefore...), d.ForceLoadBefore...)},
		{"incompatible with %s (About.xml)", d.IncompatibleWith},
	} {
		for _, p := range g.pkgs {
			sec.lines = append(sec.lines, "  "+Tf(g.f, name(p)))
		}
	}
	lin := func(l string) string {
		if p := ix.Lineages[l]; p != nil && ix.ByID[p.PrimaryID] != nil {
			return ix.ByID[p.PrimaryID].DisplayName()
		}
		if ms := ix.ByLineage[l]; len(ms) > 0 {
			return ms[0].DisplayName()
		}
		return "?"
	}
	rs, _ := a.Store.Rules(ix.Env.ID)
	for _, r := range rs {
		if r.A != mod.LineageID && r.B != mod.LineageID {
			continue
		}
		var l string
		switch r.Kind {
		case order.RuleAfter:
			l = Tf("%s loads after %s (your rule)", lin(r.A), lin(r.B))
		case order.RuleBefore:
			l = Tf("%s loads before %s (your rule)", lin(r.A), lin(r.B))
		case order.RuleIncompatible:
			l = Tf("%s is incompatible with %s (your rule)", lin(r.A), lin(r.B))
		case order.RuleAttach:
			l = Tf("%s follows %s (your rule)", lin(r.A), lin(r.B))
		default:
			continue
		}
		if r.Note != "" {
			l += "  " + markDot + "  " + r.Note
		}
		sec.lines = append(sec.lines, "  "+l)
	}
	if len(sec.lines) == 0 {
		sec.lines = []string{"  " + T("none")}
	}
	return sec
}

// historySec: every change event of the mod, newest first.
func historySec(a *app.App, ix *app.Index, id string) fullSec {
	sec := fullSec{title: T("full history"), count: -1}
	evs, _ := a.Store.Events(store.EventFilter{EnvID: ix.Env.ID, InstanceIDs: []string{id}})
	for i := len(evs) - 1; i >= 0; i-- {
		e := evs[i]
		l := "  " + e.ScanTime.Local().Format("2006-01-02 15:04") + "  " + eventText(e.Kind)
		if d := strings.TrimSpace(e.Detail); d != "" && e.Kind != identity.EvDiscovered {
			l += "  " + d
		}
		sec.lines = append(sec.lines, l)
	}
	sec.count = len(evs)
	if len(evs) == 0 {
		sec.lines = []string{"  " + T("none")}
	}
	return sec
}

// eventText is a change event in words.
func eventText(kind string) string {
	switch kind {
	case identity.EvDiscovered:
		return T("first seen")
	case identity.EvUpdated:
		return T("updated")
	case identity.EvMoved:
		return T("moved")
	case identity.EvCopied:
		return T("copied")
	case identity.EvDisappeared:
		return T("gone")
	case identity.EvReappeared:
		return T("reappeared")
	case identity.EvPackageIDChanged:
		return T("package id changed")
	case identity.EvRenamed:
		return T("renamed")
	case identity.EvAuthorChanged:
		return T("author changed")
	case identity.EvVersionSupport:
		return T("supported versions changed")
	case identity.EvDependencies:
		return T("dependencies changed")
	case identity.EvLoadRules:
		return T("load order rules changed")
	}
	return kind
}

// shortDate: "Oct 17" / "10 月 17 日".
func shortDate(t time.Time) string {
	t = t.Local()
	if lang == "zh" {
		return fmt.Sprintf("%d 月 %d 日", int(t.Month()), t.Day())
	}
	return t.Format("Jan 2")
}

// historyLine: the column's one-line history, the three newest events (06 §6.1).
func historyLine(evs []*store.Event) string {
	var parts []string
	for i := len(evs) - 1; i >= 0 && len(parts) < 3; i-- {
		parts = append(parts, shortDate(evs[i].ScanTime)+" "+eventText(evs[i].Kind))
	}
	return strings.Join(parts, " "+markDot+" ")
}

// secLines lays out the sub-sections at width w; each header line carries its number.
func (m *model) secLines(w int) []dline {
	it := m.current()
	if m.fsecs == nil || it == nil || m.fsecs.id != it.id {
		return nil
	}
	var out []dline
	if m.fsecs.secs == nil {
		return []dline{{text: sDimS.Render(fit(T("loading…"), w))}}
	}
	for i, s := range m.fsecs.secs {
		title := fmt.Sprintf("%d  %s", i+1, s.title)
		if s.count >= 0 {
			title += " " + markDot + " " + fmt.Sprint(s.count)
		}
		out = append(out, dline{text: sBold.Render(fit(title, w)), sec: i + 1})
		for j, l := range s.lines {
			if j == secCap {
				out = append(out, dline{text: "  " + sDimS.Render(fit(Tf("… %d more · tab shows all", len(s.lines)-secCap), w-2))})
				break
			}
			ind := min(len(l)-len(strings.TrimLeft(l, " ")), w/2)
			for k, x := range wrap(m.humanize(l), max(w-ind-2, 8)) {
				pad := ind
				if k > 0 {
					pad = ind + 2 // continuation lines hang under the first
				}
				out = append(out, dline{text: strings.Repeat(" ", pad) + sText.Render(fit(x, w-pad))})
			}
		}
		out = append(out, dline{})
	}
	return out
}
