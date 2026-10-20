package tui

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// 需要处理 (06 §7): what the player can act on, grouped by where the problem is, each item one
// sentence in plain words with the keys that fix it. No problem codes and no #refs appear
// here: they stay in the technical information, the CLI and MCP. The top bar's number is
// the number of counted items (06 §4.2).

// Groups, in screen order (06 §7.1).
const (
	gList = iota
	gDeps
	gConflict
	gLastRun
	gConfirm
)

var groupTitles = []string{"this list", "missing dependencies", "conflicts", "last run", "needs your answer"}

type needKey struct {
	key, label string
	run        func(m *model) tea.Cmd
}

// need is one item of the screen.
type need struct {
	group   int
	text    string
	keys    []needKey
	counted bool     // in the top bar's number; false for the folded stale log
	mods    []string // instance ids: the first carries the row icon
	status  int      // statusErr, statusWarn or statusLog for the row icon
	dim     bool
	rank    int // order within the group (06 §7.2's order)
	// A folded item stands for several findings one key resolves (06 §7.2): enter shows
	// them; every mod in mods gets the row icon.
	parts []string
	fold  bool
}

// buildNeeds turns the problem set and the environment's open questions into items.
func (m *model) buildNeeds(ps *app.ProblemSet) []need {
	if m.st == nil {
		return nil
	}
	ix := m.st.Ix
	nm := func(tok string) string { return m.tokName(tok) }
	var out []need
	add := func(n need) { out = append(out, n) }
	var versions []*app.Problem
	var order []need                    // order.violated: folded into one item when there are several
	logs := map[string][]*app.Problem{} // current log problems by subject mod ("" unattributed)
	for _, p := range ps.List {
		if p.Log != nil {
			if !p.Log.Stale() && (p.Severity == finding.Error || p.Severity == finding.Warning) {
				logs[p.SubjectInstance()] = append(logs[p.SubjectInstance()], p)
			}
			continue
		}
		if p.Code == "version.unsupported" {
			versions = append(versions, p)
			continue
		}
		if p.Severity != finding.Error && p.Severity != finding.Warning {
			continue // info: technical information only (06 §7.3)
		}
		subj := subjects(p)
		a, b := "", ""
		if len(subj) > 0 {
			a = subj[0]
		}
		if len(subj) > 1 {
			b = subj[1]
		}
		ids := instIDs(subj)
		st := statusWarn
		if p.Severity == finding.Error {
			st = statusErr
		}
		switch p.Code {
		case "official.order":
			add(need{rank: 0, group: gList, text: T("Core and the DLCs must come first in the list"), keys: []needKey{m.sortKey()}, counted: true, status: st})
		case "order.violated":
			order = append(order, need{rank: 1, group: gList, text: Tf("%s should load after %s", nm(a), nm(b)), keys: []needKey{m.sortKey()}, counted: true, mods: ids, status: st})
		case "order.cycle":
			add(need{rank: 2, group: gList, text: Tf("the order requirements of %d mods contradict each other", len(subj)),
				keys: []needKey{m.cycleViewKey(p), m.cycleIgnoreKey(p)}, counted: true, mods: ids, status: st})
		case "pkg.duplicate-active":
			add(need{rank: 3, group: gList, text: Tf("%s is enabled twice", nm(a)), keys: []needKey{m.keepOneKey(ids)}, counted: true, mods: ids, status: st})
		case "entry.missing":
			pkg := strings.TrimPrefix(a, "pkg:")
			add(need{rank: 4, group: gList, text: Tf("the list has %s, but it is not installed", pkg),
				keys: []needKey{m.searchKey(pkg), m.disableKey("pkg:"+pkg, T("remove it from the list"))}, counted: true, status: st})
		case "mod.unloadable":
			add(need{rank: 4, group: gList, text: Tf("%s is enabled, but the game cannot load it", nm(a)), keys: []needKey{m.disableKey(a, T("disable it"))}, counted: true, mods: ids, status: st})
		case "dep.missing":
			dep := depName(p.Claim, strings.TrimPrefix(b, "pkg:"))
			add(need{group: gDeps, text: Tf("%s needs %s, but %s is not installed", nm(a), dep, dep), keys: []needKey{m.workshopKey(p, dep)}, counted: true, mods: ids, status: st})
		case "dep.inactive":
			add(need{group: gDeps, text: Tf("%s needs %s, but %s is not enabled", nm(a), nm(b), nm(b)), keys: []needKey{m.enableKey(b)}, counted: true, mods: ids, status: st})
		case "patch.no-match":
			if b == "" {
				continue // no inactive target: nothing the player can do (technical information)
			}
			add(need{group: gDeps, text: Tf("part of %s does not take effect: %s, which it targets, is not enabled", nm(a), nm(b)),
				keys: []needKey{m.enableKey(b), m.viewKey(T("see what"), "patches", "--mod", m.tokRef(a))}, counted: true, mods: ids, status: st})
		case "incompat.active":
			add(need{group: gConflict, text: Tf("%s is incompatible with %s", nm(a), nm(b)), keys: []needKey{m.disableOneKey(a, b)}, counted: true, mods: ids, status: st})
		case "def.parent-missing":
			add(need{group: gDeps, text: Tf("part of %s builds on a definition no enabled mod provides; a dependency may be missing", nm(a)),
				keys: []needKey{m.viewKey(T("see what"), "mod", m.tokRef(a))}, counted: true, mods: ids, status: st})
		default:
			// Harmony warnings without log evidence, overlapping Defs and patches whose target
			// is simply absent stay in the technical information (06 §7.3).
		}
	}
	switch len(order) {
	case 0:
	case 1:
		add(order[0])
	default:
		f := need{rank: 1, group: gList, counted: true, status: statusWarn, fold: true}
		seen := map[string]bool{}
		for _, o := range order {
			f.parts = append(f.parts, o.text)
			if len(o.mods) > 0 && !seen[o.mods[0]] {
				seen[o.mods[0]] = true
				f.mods = append(f.mods, o.mods[0])
			}
		}
		f.text = Tf("%d mods are out of order", len(seen))
		f.keys = []needKey{m.sortKey(), {key: "enter", label: T("show which"), run: func(m *model) tea.Cmd {
			p := &m.prob
			if p.open == nil {
				p.open = map[string]bool{}
			}
			p.open[f.text] = !p.open[f.text]
			return nil
		}}}
		add(f)
	}
	if n := len(versions); n > 0 {
		warn := 0
		for _, v := range versions {
			if v.Severity == finding.Warning {
				warn++
			}
		}
		if warn > 0 {
			mm := m.st.GameMM()
			text := Tf("%d mods do not list %s as supported", n, mm)
			if prev := app.PrevMinor(mm); prev != "" {
				text = Tf("%d mods do not list %s as supported (%d of them list %s)", n, mm, n-warn, prev)
			}
			names := make([]string, 0, n)
			for _, v := range versions {
				names = append(names, nm(v.Subjects[0]))
			}
			sort.Strings(names)
			add(need{rank: 5, group: gList, text: text, counted: true, status: statusWarn,
				keys: []needKey{{key: "enter", label: T("see the list"), run: func(m *model) tea.Cmd {
					m.openPanel(text, strings.Join(names, "\n"), nil)
					return nil
				}}}})
		}
	}
	if v := env.MajorMinor(m.st.FileVersionMismatch()); v != "" {
		add(need{rank: 6, group: gList, text: Tf("the list was saved for %s; the game resets it when it starts — saving once fixes it", v),
			keys: []needKey{{key: "w", label: T("save"), run: func(m *model) tea.Cmd { return m.savePreview() }}}, counted: true, status: statusWarn})
	}
	// Last run: the current log's errors, one item per mod (06 §7.1).
	var subjectsOrder []string
	for id := range logs {
		subjectsOrder = append(subjectsOrder, id)
	}
	sort.Slice(subjectsOrder, func(i, j int) bool { return nm(finding.Inst(subjectsOrder[i])) < nm(finding.Inst(subjectsOrder[j])) })
	for _, id := range subjectsOrder {
		ps := logs[id]
		errs, warns := 0, 0
		for _, p := range ps {
			if p.Severity == finding.Error {
				errs += logCount(p)
			} else {
				warns += logCount(p)
			}
		}
		var text string
		switch {
		case id == "" && errs > 0:
			text = Tf("last run, %d errors could not be tied to a mod", errs)
		case id == "":
			text = Tf("last run, %d warnings could not be tied to a mod", warns)
		case errs > 0:
			text = Tf("last run, %s reported %d errors", nm(finding.Inst(id)), errs)
		default:
			text = Tf("last run, %s reported %d warnings", nm(finding.Inst(id)), warns)
		}
		argv := []string{"problems", "--code", "log"}
		var ids []string
		if id != "" {
			argv = append(argv, "--subject", ix.Ref(id))
			ids = []string{id}
		}
		add(need{group: gLastRun, text: text, keys: []needKey{m.viewKey(T("see the log and evidence"), argv...)}, counted: true, mods: ids, status: statusLog})
	}
	for _, r := range ps.Logs {
		if r.Stale() {
			when := shortDate(r.ModTime)
			add(need{group: gLastRun, text: Tf("earlier log · %s · %s", when, r.MM), dim: true,
				keys: []needKey{m.viewKey(T("see it"), "problems", "--code", "log", "--all")}})
		}
	}
	// Needs your answer: identity questions and an external change of the list.
	if qs, err := m.app.Store.Questions(ix.Env.ID, false); err == nil {
		for _, q := range qs {
			out = append(out, m.questionNeed(q))
		}
	}
	if v := m.st.WS.Conflict; v != 0 {
		add(need{group: gConfirm, text: T("ModsConfig.xml was changed outside RimArcho"), counted: true, status: statusWarn, keys: []needKey{
			{key: "L", label: T("load it into the list"), run: func(m *model) tea.Cmd {
				m.openPanel(T("load the outside change into the list?"), T("your unsaved edits are replaced by the list in ModsConfig.xml"), []string{"loadout", "checkout", fmt.Sprintf("v%d", v)})
				return nil
			}},
			{key: "I", label: T("ignore it"), run: func(m *model) tea.Cmd {
				if err := m.app.DismissConflict(); err != nil {
					m.say(sayFail, err.Error())
				} else {
					m.say(sayOK, T("the outside change is ignored (it stays in the history)"))
					m.reload()
				}
				return nil
			}}}})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].group != out[j].group {
			return out[i].group < out[j].group
		}
		return out[i].rank < out[j].rank
	})
	return out
}

func (m *model) questionNeed(q *store.Question) need {
	nm := func(id string) string { return m.tokName(finding.Inst(id)) }
	answer := func(key, label, ans, done string) needKey {
		return needKey{key: key, label: label, run: func(m *model) tea.Cmd {
			m.act(done, "identity", "resolve", fmt.Sprintf("q:%d", q.ID), ans)
			m.loadProblems()
			return nil
		}}
	}
	a, b := nm(q.InstanceID), nm(q.CandidateID)
	return need{group: gConfirm, text: Tf("is %s the same mod as %s?", a, b), counted: true, status: statusWarn,
		mods: []string{q.InstanceID, q.CandidateID},
		keys: []needKey{
			answer("s", T("same"), "same", Tf("%s and %s are one mod", a, b)),
			answer("c", T("a copy"), "copy", Tf("%s is a copy of %s", a, b)),
			answer("u", T("unrelated"), "unrelated", Tf("%s and %s are unrelated", a, b)),
		}}
}

// ---- names and refs ----

// subjects: the finding's subjects that are mods or packages.
func subjects(p *app.Problem) []string {
	var out []string
	for _, s := range p.Subjects {
		if finding.InstID(s) != "" || strings.HasPrefix(s, "pkg:") {
			out = append(out, s)
		}
	}
	return out
}

func instIDs(toks []string) []string {
	var out []string
	for _, t := range toks {
		if id := finding.InstID(t); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// tokName: a mod token's display name; a package id names an installed mod when it can.
func (m *model) tokName(tok string) string {
	ix := m.st.Ix
	if id := finding.InstID(tok); id != "" {
		if mod := ix.ByID[id]; mod != nil {
			return mod.DisplayName()
		}
		return T("a removed mod")
	}
	pkg := strings.ToLower(strings.TrimPrefix(tok, "pkg:"))
	for _, mod := range ix.Mods {
		if mod.PackageID == pkg && mod.Status == identity.Live {
			return mod.DisplayName()
		}
	}
	return pkg
}

// tokRef: the CLI reference of a token (for the commands the keys run; never shown).
func (m *model) tokRef(tok string) string {
	if id := finding.InstID(tok); id != "" {
		return m.st.Ix.Ref(id)
	}
	return tok
}

var depNameRe = regexp.MustCompile(`requires (?:the )?(.+?)(?: DLC)? \(pkg:`)

func depName(claim, pkg string) string {
	if mt := depNameRe.FindStringSubmatch(claim); mt != nil && !strings.Contains(mt[1], "@{") {
		return mt[1]
	}
	return pkg
}

var logCountRe = regexp.MustCompile(` x(\d+): `)

func logCount(p *app.Problem) int {
	n := 1
	if mt := logCountRe.FindStringSubmatch(p.Claim); mt != nil {
		fmt.Sscan(mt[1], &n)
	}
	return n
}

// ---- keys ----

func (m *model) sortKey() needKey {
	return needKey{key: "S", label: T("sort"), run: func(m *model) tea.Cmd { return m.sortPreview() }}
}

func (m *model) enableKey(tok string) needKey {
	name := m.tokName(tok)
	return needKey{key: "space", label: Tf("enable %s", name), run: func(m *model) tea.Cmd {
		m.act(Tf("enabled %s", name), "enable", m.tokRef(tok))
		m.loadProblems()
		return nil
	}}
}

func (m *model) disableKey(tok, label string) needKey {
	name := m.tokName(tok)
	return needKey{key: "space", label: label, run: func(m *model) tea.Cmd {
		m.act(Tf("disabled %s", name), "disable", m.tokRef(tok))
		m.loadProblems()
		return nil
	}}
}

// disableOneKey: choose which of two to disable (incompatible mods).
func (m *model) disableOneKey(a, b string) needKey {
	na, nb := m.tokName(a), m.tokName(b)
	return needKey{key: "space", label: T("choose which to disable"), run: func(m *model) tea.Cmd {
		p := &panel{title: T("disable which one?"), choices: []string{Tf("disable %s", na), Tf("disable %s", nb)}}
		p.onPick = func(m *model, v string) tea.Cmd {
			tok, name := a, na
			if v == Tf("disable %s", nb) {
				tok, name = b, nb
			}
			m.act(Tf("disabled %s", name), "disable", m.tokRef(tok))
			m.loadProblems()
			return nil
		}
		m.panel = p
		return nil
	}}
}

// keepOneKey: choose which copy to keep (the same mod enabled twice).
func (m *model) keepOneKey(ids []string) needKey {
	return needKey{key: "space", label: T("choose which to keep"), run: func(m *model) tea.Cmd {
		ix := m.st.Ix
		var labels []string
		for _, id := range ids {
			mod := ix.ByID[id]
			if mod == nil {
				continue
			}
			labels = append(labels, Tf("keep the one from %s", sourceText(mod)+" "+markDot+" "+mod.Rel))
		}
		p := &panel{title: T("keep which copy?"), choices: labels}
		p.onPick = func(m *model, v string) tea.Cmd {
			for i, l := range labels {
				if l != v {
					continue
				}
				var off []string
				for j, id := range ids {
					if j != i {
						off = append(off, m.st.Ix.Ref(id))
					}
				}
				m.act(T("kept one copy"), append([]string{"disable"}, off...)...)
			}
			m.loadProblems()
			return nil
		}
		m.panel = p
		return nil
	}}
}

// workshopKey: open the Workshop page of a missing dependency (its id when the
// declaration names it, else a Workshop search for its name).
func (m *model) workshopKey(p *app.Problem, name string) needKey {
	u := ""
	for _, s := range p.Suggestions {
		if strings.HasPrefix(s, "subscribe: ") {
			u = strings.TrimPrefix(s, "subscribe: ")
		}
	}
	if u == "" {
		u = workshopSearch(name)
	}
	return needKey{key: "o", label: Tf("open %s in the Workshop", name), run: func(m *model) tea.Cmd { return m.open(u) }}
}

func (m *model) searchKey(pkg string) needKey {
	return needKey{key: "o", label: T("look for it in the Workshop"), run: func(m *model) tea.Cmd { return m.open(workshopSearch(pkg)) }}
}

func workshopSearch(q string) string {
	return "https://steamcommunity.com/workshop/browse/?appid=294100&searchtext=" + url.QueryEscape(q)
}

func (m *model) open(u string) tea.Cmd {
	open := m.opts.OpenURL
	if open == nil {
		open = openURL
	}
	if err := open(u); err != nil {
		m.say(sayFail, err.Error())
	} else {
		m.say(sayOK, T("opened the Workshop page"))
	}
	return nil
}

func (m *model) viewKey(label string, argv ...string) needKey {
	return needKey{key: "enter", label: label, run: func(m *model) tea.Cmd { return m.openInspect(argv) }}
}

// cycleViewKey: the requirements that contradict each other, in words.
func (m *model) cycleViewKey(p *app.Problem) needKey {
	return needKey{key: "enter", label: T("see which"), run: func(m *model) tea.Cmd {
		var lines []string
		for _, e := range p.Evidence {
			lines = append(lines, m.edgeWords(e.Text))
		}
		m.openPanel(T("order requirements that contradict each other"), strings.Join(lines, "\n"), nil)
		return nil
	}}
}

// cycleIgnoreKey: ignore the requirement the sort suggests dropping (its first suggestion).
func (m *model) cycleIgnoreKey(p *app.Problem) needKey {
	return needKey{key: "x", label: T("ignore the suggested one"), run: func(m *model) tea.Cmd {
		for _, s := range p.Suggestions {
			if rest, ok := strings.CutPrefix(s, "archo rule add ignore "); ok {
				m.act(T("that requirement is ignored from now on"), "rule", "add", "ignore", finding.Expand(rest, m.st.Ix.Ref))
				m.loadProblems()
				return nil
			}
		}
		m.say(sayInfo, T("nothing to ignore here"))
		return nil
	}}
}

var edgeRe = regexp.MustCompile(`^(?:decl|community|rule r:\d+|builtin) (\S+)(?: About\.xml)? (\S+) (pkg:\S+)`)

// edgeWords: "decl @{a} About.xml loadAfter pkg:b  [suggested to ignore]" → "A wants to load after B (About.xml) · suggested to ignore".
func (m *model) edgeWords(t string) string {
	sug := strings.Contains(t, "[suggested to ignore]")
	mt := edgeRe.FindStringSubmatch(t)
	if mt == nil {
		return m.humanize(finding.Expand(t, m.st.Ix.Ref))
	}
	a, field, b := m.tokName(mt[1]), strings.ToLower(mt[2]), m.tokName(mt[3])
	if strings.HasPrefix(mt[1], "pkg:") {
		a = m.tokName(mt[1])
	}
	var s string
	switch {
	case strings.Contains(field, "before"):
		s = Tf("%s wants to load before %s", a, b)
	default:
		s = Tf("%s wants to load after %s", a, b)
	}
	if sug {
		s += "  " + markDot + "  " + T("suggested to ignore (x)")
	}
	return s
}

// sortPreview and savePreview are the S and w of the main screen.
func (m *model) sortPreview() tea.Cmd {
	out, code := m.run("sort", "--dry-run")
	var confirm []string
	if code == cmds.ExitOK && !strings.HasPrefix(out, "sort: 0 moved") {
		confirm = []string{"sort"}
	}
	m.openPanel(T("sort — preview"), out, confirm)
	return nil
}

func (m *model) savePreview() tea.Cmd {
	out, code := m.run("save", "--dry-run")
	var confirm []string
	if code == cmds.ExitOK {
		confirm = []string{"save"}
	}
	m.openPanel(T("save to game — preview"), out, confirm)
	return nil
}

// ---- the screen ----

// needLines draws 需要处理: a count line, then the groups with their items; the selected
// item shows its keys.
func (m *model) needLines(n int) []string {
	p := &m.prob
	w := m.w
	items := p.items
	head := Tf("%d to handle", p.need)
	if p.need == 0 {
		head = T("nothing to handle")
	}
	switch {
	case m.analyzing:
		head += "   " + T("checking the mods' files…")
	case p.never:
		head += "   " + T("the mods' files are not checked yet · A checks them")
	case p.stale:
		head += "   " + T("the files changed since the last check · A checks again")
	}
	out := []string{"  " + sText.Render(fit(head, w-2)), ""}
	if len(items) == 0 {
		return append(out, "  "+sOK.Render(markOK)+" "+sSecond.Render(fit(T("nothing needs your attention"), w-4)))
	}
	p.cursor = max(0, min(p.cursor, len(items)-1))
	type row struct {
		text string
		item int
	}
	var rows []row
	grp := -1
	for i, it := range items {
		if it.group != grp {
			if grp >= 0 {
				rows = append(rows, row{"", -1})
			}
			grp = it.group
			rows = append(rows, row{"  " + sDimS.Render(fit(T(groupTitles[grp]), w-2)), -1})
		}
		icon := statusIcon(it.status)
		if it.dim || it.status == statusNone {
			icon = " "
		}
		st := sText
		if it.dim {
			st = sDimS
		}
		sel := i == p.cursor
		var keys []string
		for _, k := range it.keys {
			keys = append(keys, k.key+" "+k.label)
		}
		hint := strings.Join(keys, "  "+markDot+"  ")
		tw := w - 6
		lines := wrap(it.text, tw)
		for j, l := range lines {
			lead := "    "
			if j == 0 {
				lead = "  " + icon + " "
				if sel {
					lead = "  " + sAccent.Render(markSel) + " "
				}
			}
			s := st
			if sel {
				s = sSel
			}
			rows = append(rows, row{lead + s.Render(fit(l, tw)), i})
		}
		if it.fold && p.open[it.text] {
			for _, part := range it.parts {
				for j, l := range wrap(part, tw-2) {
					lead := "    " + sDimS.Render(markDot) + " "
					if j > 0 {
						lead = "      "
					}
					rows = append(rows, row{lead + sSecond.Render(fit(l, tw-2)), i})
				}
			}
		}
		if sel && hint != "" {
			for _, l := range wrap(hint, tw) {
				rows = append(rows, row{"    " + sSecond.Render(fit(l, tw)), i})
			}
		}
	}
	// keep the selected item in view
	first, last := -1, 0
	for i, r := range rows {
		if r.item == p.cursor {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	first = max(first, 0)
	page := max(n-len(out), 1)
	if first < p.off {
		p.off = max(first-1, 0)
	}
	if last >= p.off+page {
		p.off = last - page + 1
	}
	for i := p.off; i < len(rows) && len(out) < n; i++ {
		out = append(out, rows[i].text)
	}
	return out
}

func (m *model) needsKey(s string) tea.Cmd {
	p := &m.prob
	switch s {
	case "up", "k":
		p.cursor = max(p.cursor-1, 0)
		return nil
	case "down", "j":
		p.cursor = min(p.cursor+1, max(len(p.items)-1, 0))
		return nil
	case "pgup":
		p.cursor = max(p.cursor-10, 0)
		return nil
	case "pgdown":
		p.cursor = min(p.cursor+10, max(len(p.items)-1, 0))
		return nil
	case "home", "g":
		p.cursor = 0
		return nil
	case "end", "G":
		p.cursor = max(len(p.items)-1, 0)
		return nil
	case "A":
		return m.startAnalyze()
	case "l":
		return m.chooseLog()
	}
	if p.cursor >= len(p.items) {
		return nil
	}
	if s == " " {
		s = "space"
	}
	for _, k := range p.items[p.cursor].keys {
		if k.key == s {
			return k.run(m)
		}
	}
	return nil
}
