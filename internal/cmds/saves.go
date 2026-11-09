package cmds

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// Saves (13 §4.2, 12 §4): list colonies and their saves, show one save against its list
// and its playthrough, fix its list, rename, copy, back up and recycle, read saves in full.
// Lists use the cached full read (12 §1.7 L1) and never read; a save not read in full yet
// is "meta only". saves show reads its one save in full inline.

func init() {
	register(&Command{Name: "saves list", Summary: "colonies and their saves, newest first, each with its list and whether it loads with it", MaxArgs: 0,
		Flags: []Flag{{Name: "colony", Value: "<name>", Help: "only this colony"}, {Name: "mod", Value: "<ref>", Help: "only saves whose Mod list has this Mod"},
			{Name: "attention", Help: "only saves with warnings or errors"}, {Name: "all", Help: "no truncation"}}, Run: savesList})
	register(&Command{Name: "saves show", Args: "<save>", Summary: "one save: where it belongs, can it load (each issue with its fix), how its Mods differ from the list, the playthrough", MinArgs: 1, MaxArgs: 1,
		Flags: []Flag{{Name: "mods", Help: "the save's whole Mod list"}, {Name: "play", Help: "the whole playthrough: every colonist, event and stat"},
			{Name: "content", Help: "content references by Mod and category, with coverage limits"},
			{Name: "current", Help: "check against the current game's current list instead of the list the save matched"},
			{Name: "all", Help: "no truncation"}}, Run: savesShow})
	register(&Command{Name: "saves fix", Args: "<save>", Summary: "the save's Mod fixes as one undoable edit of its list, then Workshop subscriptions; --launch starts it", MinArgs: 1, MaxArgs: 1,
		Flags: []Flag{{Name: "mod", Value: "<packageId>", Multi: true, Help: "only this Mod's fix"}, {Name: "disable-added", Help: "also disable the list's Mods the save lacks"},
			{Name: "order", Help: "put the Mods both have in the save's order"}, {Name: "launch", Help: "then save the list to the game and start it (not while subscriptions download)"},
			{Name: "current", Help: "fix the current game's current list instead of the list the save matched"},
			{Name: "dry-run", Help: "show what would change; change nothing"}}, Run: savesFix})
	register(&Command{Name: "saves index", Args: "[<save…>]", Summary: "read new and changed saves in full into the cache, the named ones first (the Saves page runs it as a child process)", MaxArgs: -1,
		Flags: []Flag{{Name: "only", Help: "only the named saves"}, {Name: "progress", Help: "a progress line per save on stderr"}}, Run: savesIndex})
	register(&Command{Name: "saves rename", Args: "<save> <name>", Summary: "rename a save in its folder (keeps a RimSaves folder; refused while the game runs or when the name exists)", MinArgs: 2, MaxArgs: 2, Run: savesRename})
	register(&Command{Name: "saves rename-colony", Args: "<colony|save> [<name>]", Summary: "name a colony in RimArcho (without a name: back to its saves' name; save files and the game keep theirs)", MinArgs: 1, MaxArgs: 2, Run: savesRenameColony})
	register(&Command{Name: "saves copy", Args: "<save>", Summary: "copy a save beside itself as <name> (copy), never overwriting", MinArgs: 1, MaxArgs: 1, Run: savesCopy})
	yes := []Flag{{Name: "yes", Help: "do it (without it: only show the files and sizes)"}}
	register(&Command{Name: "saves backup", Args: "<save…>", Summary: "copy saves into RimArcho's backups folder (confirm with --yes)", MinArgs: 1, MaxArgs: -1, Flags: yes, Run: savesBackup})
	register(&Command{Name: "saves trash", Args: "<save…>", Summary: "move saves to the recycle bin (confirm with --yes; refused while the game runs)", MinArgs: 1, MaxArgs: -1, Flags: yes, Run: savesTrash})
}

// metaOnly marks a save without the full read: no in-game day or play facts (12 §1.7).
const metaOnly = "meta only"

// dayText is saves list's day column: the in-game day from the cached full read, else
// meta only.
func dayText(s *app.Save) string {
	switch {
	case s.Summary == nil:
		return metaOnly
	case s.Summary.Malformed:
		return "malformed"
	}
	return fmt.Sprintf("day %d", s.Summary.Day)
}

// ---- references ----

// findSaves resolves save references (13 §2). A reference that several saves match is the
// candidate list with exit code 3 (13 §3.3): the doc to return instead.
func findSaves(c *Ctx, refs []string) ([]*app.Save, *render.Doc, error) {
	l, err := c.App.ListSaves()
	if err != nil {
		return nil, nil, err
	}
	var out []*app.Save
	seen := map[string]bool{}
	for _, ref := range refs {
		s, err := l.Find(ref)
		if err == nil {
			if !seen[s.Path] {
				seen[s.Path] = true
				out = append(out, s)
			}
			continue
		}
		var amb *app.AmbiguousSave
		if !errors.As(err, &amb) {
			return nil, nil, err
		}
		hits := amb.Saves
		d, root := render.New(fmt.Sprintf("%s matches %d saves; pick one", ref, len(hits)))
		var rows [][]string
		for _, h := range hits {
			rows = append(rows, []string{h.Ref, h.Colony.Name + "/" + h.Name, h.Path})
		}
		root.AddColumns(rows)
		for _, h := range hits[:min(3, len(hits))] {
			d.AddNext("archo saves show " + h.Ref)
		}
		c.ExitCode = ExitAmbiguous
		return nil, d, nil
	}
	return out, nil, nil
}

func findSave(c *Ctx, ref string) (*app.Save, *render.Doc, error) {
	ss, d, err := findSaves(c, []string{ref})
	if err != nil || d != nil {
		return nil, d, err
	}
	return ss[0], nil, nil
}

// ---- text ----

func saveTime(c *Ctx, s *app.Save) string {
	if s.ModTime.IsZero() {
		return "unknown time"
	}
	return s.ModTime.In(c.App.Loc).Format("2006-01-02 15:04")
}

// saveListText names what a save matched: "plan:<name> (<rule>)", "pack <name>" or "no list".
func saveListText(s *app.Save) string {
	switch {
	case s.Plan != nil:
		return app.PlanRef(s.Plan) + " (" + s.Match + ")"
	case s.Pack != "":
		return "pack " + s.Pack
	}
	return "no list"
}

// matchText says how a save matched its list (12 §4.2).
func matchText(s *app.Save) string {
	switch s.Match {
	case app.MatchPack:
		return "pack " + s.Pack + " (the save is in the pack's folder)"
	case app.MatchLaunch:
		return app.PlanRef(s.Plan) + " (saved after a launch of it)"
	case app.MatchMods:
		return app.PlanRef(s.Plan) + " (same Mods as one of its versions)"
	}
	return "none (matches no list or pack)"
}

// fixLine is a fix with the archo command that performs it.
func fixLine(f app.SaveFix) string {
	return strings.TrimSuffix(f.Label, "…") + " (archo " + f.Command + ")"
}

// consequence is what an issue means for loading the save (12 §4.3). With the full read a
// missing Mod's row says what it owns in the save (12 §4.6); without it, only that what
// the Mod added is lost.
func consequence(is app.SaveIssue) string {
	owns := func(lost string) string {
		o := is.Owns
		if o == nil {
			return lost
		}
		var b strings.Builder
		if o.Empty() {
			b.WriteString("nothing in the save is known to need it; what else it stored is lost on load")
		} else {
			b.WriteString("loading without it loses " + strings.Join(o.Parts(), ", "))
			if o.Inferred {
				b.WriteString(" (inferred: nothing installed provides them")
				if o.Among > 0 {
					fmt.Fprintf(&b, "; they may belong to the other %s instead", render.Plural(o.Among, "Mod")+" without facts")
				}
				b.WriteString(")")
			}
		}
		if o.DefsUnchecked {
			b.WriteString("; Defs, pawns and factions not checked: the list is not analysed for this game version (archo problems)")
		}
		if o.ClassesUnchecked {
			b.WriteString("; classes not checked: the list's or the game's assemblies are not indexed (archo scan, archo problems)")
		}
		return b.String()
	}
	switch is.Kind {
	case app.IssueUnreadable:
		return "cannot be read as a save"
	case app.IssueCutOff:
		return "can never load: the file stops before its end (written during a crash?)"
	case app.IssueMalformed:
		return "can never load: its XML breaks off where reading stopped (written during a crash, or edited by hand?)"
	case app.IssueClassMissing:
		if len(is.Mods) > 0 {
			m := is.Mods[0]
			return "the game drops these objects with errors: only " + m.Name + " (pkg:" + m.PackageID + ") defines them, and the list does not load it"
		}
		return "the game drops these objects with errors: no installed Mod nor the game defines them (inferred: a Mod update removed them, or their Mod is gone)"
	case app.IssueNoList:
		return "nothing to compare its Mods with"
	case app.IssueGameVersion:
		return "the game converts it on load"
	case app.IssueNotInstalled:
		return owns("what the Mod added to the save is lost on load")
	case app.IssueDisabled:
		return "installed but not in the list: " + owns("what it added to the save is lost on load")
	case app.IssueAdded:
		return "usually safe: they start fresh in an existing save"
	case app.IssueReordered:
		return "usually safe; patches apply in another order"
	case app.IssueLastLoad:
		t := "it may load with missing or broken content"
		if l := is.Load; l != nil && l.First != "" {
			first, _, _ := strings.Cut(l.First, "\n")
			t += "; first: log:" + l.Cluster + " " + strings.TrimSpace(first)
		}
		return t
	}
	return ""
}

// saveModLine names one Mod of a save or its list: the instance reference when the save's
// game has it installed, else its packageId.
func saveModLine(ix *app.Index, s *app.Save, m app.SaveMod) string {
	l := "pkg:" + m.PackageID + " " + m.Name
	if ix != nil && s.Plan != nil && m.Instance != "" && ix.ByID[m.Instance] != nil {
		l = ix.Ref(m.Instance) + " " + m.Name
	}
	if m.Instance == "" && m.SteamID != "" {
		l += " (ws:" + m.SteamID + ")"
	}
	return l
}

// modMark is how a save's Mod differs from the list ("" for no difference).
func modMark(chk *app.SaveCheck, m app.SaveMod) string {
	hasList := chk.Save.Plan != nil || chk.Save.Pack != ""
	switch {
	case m.Instance == "":
		return "not installed"
	case hasList && !m.InList:
		return "not in the list"
	case m.Moved:
		return "moved"
	}
	return ""
}

func normPkgID(p string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(p)), "_steam")
}

// ---- saves list ----

// modFilter is --mod: the packageIds and Workshop IDs a save's Mod list must include. A
// packageId or ws: of a Mod that is not installed still matches saves that have it.
func modFilter(c *Ctx, ref string) (map[string]bool, map[string]bool, error) {
	pkgs, ws := map[string]bool{}, map[string]bool{}
	if p, ok := app.PackageRef(ref); ok && p != "" {
		pkgs[normPkgID(p)] = true
	}
	if id, ok := strings.CutPrefix(strings.TrimSpace(ref), "ws:"); ok && id != "" {
		ws[id] = true
	}
	ix, err := c.App.Index()
	if err != nil {
		return nil, nil, err
	}
	mods, rerr := ix.Resolve(ref)
	var amb *app.Ambiguous
	if errors.As(rerr, &amb) {
		mods, rerr = amb.Mods, nil
	}
	for _, m := range mods {
		pkgs[normPkgID(m.PackageID)] = true
		if id := m.WorkshopID(); id != "" {
			ws[id] = true
		}
	}
	if len(pkgs) == 0 && len(ws) == 0 {
		if rerr == nil {
			rerr = fmt.Errorf("%s matches no mod", ref)
		}
		return nil, nil, rerr
	}
	return pkgs, ws, nil
}

func saveHasMod(s *app.Save, pkgs, ws map[string]bool) bool {
	if s.Head == nil {
		return false
	}
	for i, id := range s.Head.ModIDs {
		if pkgs[normPkgID(id)] || ws[s.Head.SteamID(i)] {
			return true
		}
	}
	return false
}

func inEnv(s *app.Save, name string) bool {
	for _, e := range s.Folder.Envs {
		if strings.EqualFold(e.Name, name) {
			return true
		}
	}
	return false
}

// listSaveBudget is the default number of lines for colonies and saves (13 §3.1 rule 5),
// and listColonySaves the saves shown per colony when every colony is listed.
const (
	listSaveBudget  = 34
	listColonySaves = 5
)

func savesList(c *Ctx, a *Args) (*render.Doc, error) {
	envName := c.App.EnvName
	if envName != "" {
		if _, err := c.App.Env(); err != nil {
			return nil, err
		}
	}
	var pkgs, ws map[string]bool
	if ref := a.Str("mod"); ref != "" {
		var err error
		if pkgs, ws, err = modFilter(c, ref); err != nil {
			return nil, err
		}
	}
	l, err := c.App.ListSaves()
	if err != nil {
		return nil, err
	}
	colony, attention, all := a.Str("colony"), a.Bool("attention"), a.Bool("all")
	type row struct {
		s   *app.Save
		chk *app.SaveCheck
	}
	type group struct {
		c    *app.Colony
		rows []row
	}
	var groups []group
	total, colonyFound := 0, colony == ""
	for _, col := range l.Colonies {
		if colony != "" && !strings.EqualFold(col.Name, colony) {
			continue
		}
		colonyFound = true
		g := group{c: col}
		for _, s := range col.Saves {
			if envName != "" && !inEnv(s, envName) || pkgs != nil && !saveHasMod(s, pkgs, ws) {
				continue
			}
			chk, err := c.App.CheckSave(s)
			if err != nil {
				return nil, err
			}
			if attention && chk.Level != app.LevelWarning && chk.Level != app.LevelError {
				continue
			}
			g.rows = append(g.rows, row{s, chk})
		}
		if len(g.rows) > 0 {
			groups = append(groups, g)
			total += len(g.rows)
		}
	}
	if !colonyFound {
		return nil, fmt.Errorf("no colony named %s (archo saves list)", colony)
	}

	head := fmt.Sprintf("saves: %s in %s", render.Plural(total, "save"), render.Plural(len(groups), "colony"))
	var filters []string
	if envName != "" {
		filters = append(filters, "env:"+envName)
	}
	if colony != "" {
		filters = append(filters, "colony "+colony)
	}
	if pkgs != nil {
		filters = append(filters, "mod "+a.Str("mod"))
	}
	if attention {
		filters = append(filters, "attention")
	}
	if len(filters) > 0 {
		head += " (" + strings.Join(filters, ", ") + ")"
	}
	d, root := render.New(head)
	for _, e := range l.Errors {
		root.KV("error", e)
	}
	if total == 0 {
		if len(filters) == 0 {
			fs := root.Section("folders")
			for _, f := range l.Folders {
				fs.Add(f.Dir)
			}
		}
		return d, nil
	}

	// One column layout for every shown save, so the colonies line up.
	perColony := listColonySaves
	if all || colony != "" {
		perColony = 0
	}
	var cells [][]string
	type shown struct {
		g    group
		n    int // rows shown
		more int
	}
	var plan []shown
	used := 0
	for i, g := range groups {
		n := len(g.rows)
		if perColony > 0 && n > perColony {
			n = perColony
		}
		more := len(g.rows) - n
		lines := 1 + n
		if more > 0 {
			lines++
		}
		if !all && used+lines > listSaveBudget && len(plan) > 0 {
			rest := 0
			for _, r := range groups[i:] {
				rest += len(r.rows)
			}
			plan = append(plan, shown{n: -1, more: rest})
			break
		}
		used += lines
		plan = append(plan, shown{g: g, n: n, more: more})
		for _, r := range g.rows[:n] {
			cells = append(cells, []string{r.s.Ref, r.s.Name, saveTime(c, r.s), dayText(r.s), saveListText(r.s), r.chk.Verdict()})
		}
	}
	unread := 0
	for _, g := range groups {
		for _, r := range g.rows {
			if r.s.Summary == nil {
				unread++
			}
		}
	}
	lines := render.Columns(cells)
	k := 0
	var firstAttention, first *app.Save
	for _, p := range plan {
		if p.n < 0 {
			hidden := len(groups) - (len(plan) - 1)
			root.Addf("… %s more in %s (archo saves list%s --all)", render.Plural(p.more, "save"), render.Plural(hidden, "colony"), listFlags(a))
			break
		}
		col := p.g.c
		line := col.Name + "  " + render.Plural(len(col.Saves), "save") + ", latest " + saveTime(c, col.Latest())
		if col.Orphan() {
			line += "  [matches no list]"
		}
		n := root.Add(line)
		for _, r := range p.g.rows[:p.n] {
			n.Add(lines[k])
			k++
			if first == nil {
				first = r.s
			}
			if firstAttention == nil && (r.chk.Level == app.LevelError || r.chk.Level == app.LevelWarning) {
				firstAttention = r.s
			}
		}
		if p.more > 0 {
			n.Addf("… %d more (archo saves list --colony %s)", p.more, quote(col.Name))
		}
	}
	if unread > 0 {
		what := "1 save is"
		if unread > 1 {
			what = fmt.Sprintf("%d saves are", unread)
		}
		root.KV("note", what+" "+metaOnly+": only the head is read, so no in-game day yet; archo saves index reads them in full")
	}
	if firstAttention != nil {
		first = firstAttention
	}
	d.AddNext("archo saves show " + first.Ref)
	return d, nil
}

// listFlags repeats saves list's filters for an expansion command.
func listFlags(a *Args) string {
	s := ""
	if v := a.Str("colony"); v != "" {
		s += " --colony " + quote(v)
	}
	if v := a.Str("mod"); v != "" {
		s += " --mod " + quote(v)
	}
	if a.Bool("attention") {
		s += " --attention"
	}
	return s
}

// ---- saves show ----

func savesShow(c *Ctx, a *Args) (*render.Doc, error) {
	s, amb, err := findSave(c, a.Pos[0])
	if err != nil || amb != nil {
		return amb, err
	}
	// The full read first (12 §1.7: inline for the one save), so the load check and the
	// colony see it: a save read now may join its world's colony (12 §4.1).
	had := s.Summary != nil
	facts, ferr := c.App.SaveFacts(s, true)
	if facts != nil && !had {
		if l, err := c.App.ListSaves(); err == nil {
			if again, err := l.Find(s.Path); err == nil {
				s = again
			}
		}
	}
	own := s // where it belongs; with --current s is checked against the current list
	if a.Bool("current") {
		if s, err = c.App.SaveAgainstCurrent(s, ""); err != nil {
			return nil, err
		}
	}
	chk, err := c.App.CheckSave(s)
	if err != nil {
		return nil, err
	}
	ix, err := c.App.SaveIndex(s)
	if err != nil {
		return nil, err
	}
	all, play := a.Bool("all"), a.Bool("play")
	limit := render.DefaultLimit
	if all {
		limit = 0
	}
	d, root := render.New(s.Ref + " " + s.Name + "  [" + chk.Verdict() + "]")
	col := s.Colony
	cl := col.Name + " (" + render.Plural(len(col.Saves), "save")
	if l := col.Latest(); l != s {
		cl += ", latest " + l.Name + " " + l.Ref
	}
	root.KV("colony", cl+")")
	if s.Head != nil {
		g := s.Head.GameVersion
		if s.Env != nil {
			g += ", env:" + s.Env.Name + " runs " + versionText(s.Env.GameVersion)
		}
		root.KV("game", g)
	} else if s.Env != nil {
		root.KV("game", "env:"+s.Env.Name+" runs "+versionText(s.Env.GameVersion))
	}
	root.KV("list", matchText(own))
	if s.Current {
		against := "no current list"
		if s.Plan != nil {
			against = "the current list " + app.PlanRef(s.Plan)
		}
		root.KV("checked against", against+" of env:"+s.Env.Name+" (--current)")
	}
	root.KV("file", s.Path)
	root.KV("saved", saveTime(c, s)+", "+render.Size(s.Size))
	root.KV("read", readText(facts, s, ferr))
	if a.Bool("content") {
		content, err := c.App.SaveContent(own)
		if err != nil {
			return nil, err
		}
		addSaveContent(root, content, all, showRef(own), showLines-docLines(d))
		return d, nil
	}

	if chk.Level == "writing" {
		root.KV("can it load", "no verdict: the file is being written; show it again when the game has saved")
		return d, nil
	}
	if facts != nil {
		var prev *playPrev
		if p := previousSave(s); p != nil {
			pf, err := c.App.SaveFacts(p, false)
			if err != nil {
				return nil, err
			}
			prev = &playPrev{save: p, facts: pf}
		}
		addPlay(root, s, facts, prev, play, limit)
	}
	// The rest shares the call's 40 lines (13 §3.1 rule 5): the load check's items first,
	// then the Mod rows, keeping room for the quick fix, a few Mod rows and next:.
	budget := 0
	if !all {
		budget = showLines
	}
	showMods, mods := s.Head != nil && (!play || facts == nil || a.Bool("mods")), a.Bool("mods")
	modRows := len(chk.ListOnly)
	for _, m := range chk.Mods {
		if mods || modMark(chk, m) != "" {
			modRows++
		}
	}
	tail := 0
	if chk.Quick != nil {
		tail += 1 + 2 // quick fix:, next: and its command
	}
	minRows := min(modRows, 5)
	if mods {
		minRows = min(modRows, 15)
	}
	if showMods {
		tail += 1 + minRows
		if modRows > minRows {
			tail++
		}
		if !mods && len(chk.Mods) > 0 {
			tail++
		}
	}
	switch {
	case play && facts != nil:
		// The playthrough in full: the load check shrinks to its verdict.
		line := "yes, with the list as it is now"
		if len(chk.Issues) > 0 {
			line = chk.Verdict() + ": " + render.Plural(len(chk.Issues), "item")
		}
		root.KV("can it load", line+" (archo saves show "+showRef(s)+")")
	case len(chk.Issues) == 0:
		root.KV("can it load", "yes, with the list as it is now")
	default:
		cn := root.KV("can it load", render.Plural(len(chk.Issues), "item"))
		for i, is := range chk.Issues {
			line := is.Level + " " + is.Kind
			switch {
			case is.Text != "":
				line += "  " + is.Text
			case len(is.Mods) == 1:
				line += "  " + saveModLine(ix, s, is.Mods[0])
			case len(is.Mods) > 1:
				line += "  " + render.Plural(len(is.Mods), "Mod")
			}
			n := &render.Node{Line: line}
			if len(is.Mods) > 1 {
				for _, m := range is.Mods {
					n.Add(saveModLine(ix, s, m))
				}
				n.Truncate(min(limit, 10), "archo saves show "+showRef(s)+" --all")
			}
			n.KV("consequence", consequence(is))
			for _, f := range is.Fixes {
				n.KV("fix", fixLine(f))
			}
			more := 0
			if i < len(chk.Issues)-1 {
				more = 1
			}
			if budget > 0 && i > 0 && docLines(d)+nodeLines(n)+more+tail > budget {
				cn.Addf("… %d more (archo saves show %s --all)", len(chk.Issues)-i, showRef(s))
				break
			}
			cn.Children = append(cn.Children, n)
		}
	}
	if chk.Quick != nil {
		root.KV("quick fix", fixLine(*chk.Quick))
	}

	if showMods {
		rows := limit
		if budget > 0 {
			left := budget - docLines(d) - 1 // the mods: line
			if chk.Quick != nil {
				left -= 2
			}
			if !mods && len(chk.Mods) > 0 {
				left-- // the whole list hint
			}
			if modRows > left {
				left-- // … N more
			}
			rows = min(limit, max(left, minRows))
		}
		addSaveMods(root, ix, chk, mods, rows, s)
	}
	if q := chk.Quick; q != nil && strings.HasPrefix(q.Command, "saves fix ") {
		d.AddNext("archo " + q.Command + " --dry-run")
	} else if q != nil {
		d.AddNext("archo " + q.Command)
	}
	return d, nil
}

// showLines is saves show's line budget without --all (13 §3.1 rule 5).
const showLines = 40

// nodeLines counts the lines a node renders; docLines a whole document's, next: included.
func nodeLines(n *render.Node) int {
	c := 1
	for _, k := range n.Children {
		c += nodeLines(k)
	}
	return c
}

func docLines(d *render.Doc) int {
	c := 0
	for _, n := range d.Nodes {
		c += nodeLines(n)
	}
	if len(d.Next) > 0 {
		c += 1 + len(d.Next)
	}
	return c
}

// addSaveMods is the save's Mod list against the list (12 §4.3 "Mod list"): with full the
// whole list in save order, else only the differences.
func addSaveMods(root *render.Node, ix *app.Index, chk *app.SaveCheck, full bool, limit int, s *app.Save) {
	counts := map[string]int{}
	for _, m := range chk.Mods {
		if mk := modMark(chk, m); mk != "" {
			counts[mk]++
		}
	}
	line := render.Plural(len(chk.Mods), "Mod") + " in the save"
	var parts []string
	for _, mk := range []string{"not installed", "not in the list", "moved"} {
		if counts[mk] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[mk], mk))
		}
	}
	if len(chk.ListOnly) > 0 {
		parts = append(parts, fmt.Sprintf("%d only in the list", len(chk.ListOnly)))
	}
	switch {
	case len(parts) > 0:
		line += "; " + strings.Join(parts, ", ")
	case s.Plan != nil || s.Pack != "":
		line += ", the same as the list"
	}
	n := root.KV("mods", line)
	var rows [][]string
	for i, m := range chk.Mods {
		mk := modMark(chk, m)
		if full || mk != "" {
			rows = append(rows, []string{fmt.Sprintf("%d.", i+1), saveModLine(ix, s, m), mk})
		}
	}
	for _, m := range chk.ListOnly {
		rows = append(rows, []string{"-", saveModLine(ix, s, m), "only in the list"})
	}
	n.AddColumns(rows)
	n.Truncate(limit, "archo saves show "+showRef(s)+" --mods --all")
	if !full && len(chk.Mods) > 0 {
		n.Add("(the whole list: archo saves show " + showRef(s) + " --mods)")
	}
}

// showRef is what saves show names to show this check again: the save, with --current
// when it is checked against the current list.
func showRef(s *app.Save) string {
	if s.Current {
		return s.Ref + " --current"
	}
	return s.Ref
}

// ---- saves fix ----

func savesFix(c *Ctx, a *Args) (*render.Doc, error) {
	s, amb, err := findSave(c, a.Pos[0])
	if err != nil || amb != nil {
		return amb, err
	}
	if a.Bool("current") {
		if s, err = c.App.SaveAgainstCurrent(s, ""); err != nil {
			return nil, err
		}
	}
	dry, launch := a.Bool("dry-run"), a.Bool("launch")
	r, err := c.App.FixSave(s, app.SaveFixOptions{Mods: a.List("mod"), DisableAdded: a.Bool("disable-added"), Order: a.Bool("order")}, dry)
	if err != nil {
		return nil, err
	}
	ix, err := c.App.SaveIndex(s)
	if err != nil {
		return nil, err
	}
	plan := app.PlanRef(s.Plan)
	changed := r.Changed()
	var head string
	switch {
	case !changed && len(r.Waiting) > 0:
		head = s.Ref + " " + s.Name + ": " + plan + " already has the save's Mods; " + render.Plural(len(r.Waiting), "Mod") + " still to download"
	case !changed:
		head = s.Ref + " " + s.Name + ": nothing to fix in " + plan
	case dry:
		head = "dry run, nothing changed: would fix " + plan + " for " + s.Ref + " " + s.Name
	default:
		head = "fixed " + plan + " for " + s.Ref + " " + s.Name
	}
	d, root := render.New(head)
	verb := func(done, would string) string {
		if dry {
			return would
		}
		return done
	}
	if len(r.Enable) > 0 {
		n := root.Section(verb("enabled", "would enable"))
		for _, m := range r.Enable {
			n.Add(saveModLine(ix, s, m))
		}
	}
	if len(r.Subscribe) > 0 {
		n := root.Section(verb("added (missing until downloaded)", "would add (missing until downloaded)"))
		for _, m := range r.Subscribe {
			n.Add(saveModLine(ix, s, m))
		}
	}
	if len(r.Waiting) > 0 {
		n := root.Section(verb("in the list, not installed: subscribed again", "in the list, not installed: would subscribe again"))
		for _, m := range r.Waiting {
			n.Add(saveModLine(ix, s, m))
		}
	}
	if len(r.Unresolved) > 0 {
		n := root.Section("left out (not installed, no Workshop ID)")
		for _, m := range r.Unresolved {
			n.Add(saveModLine(ix, s, m))
		}
		n.Add("(a list from the save keeps them: archo loadout import " + quote(s.Path) + " --new " + quote(s.Colony.Name) + ")")
	}
	if len(r.Disable) > 0 {
		n := root.Section(verb("disabled", "would disable"))
		for _, m := range r.Disable {
			n.Add(saveModLine(ix, s, m))
		}
	}
	if r.Reordered > 0 {
		root.KV("order", fmt.Sprintf("%s %s to the save's order", render.Plural(r.Reordered, "Mod"), verb("moved", "would move")))
	}
	current := false
	if st := c.Loadout(); st != nil && st.Plan != nil && st.Plan.ID == s.Plan.ID {
		current = true
	}
	if changed && !dry {
		if current {
			root.KV("history", "one undoable step (archo undo)")
		} else {
			root.KV("history", "one undoable step in "+plan+", undone from that plan (archo plan use "+quote(s.Plan.Name)+")")
		}
	}

	// Subscriptions through the existing Workshop operation, in the save's game.
	ids := r.WorkshopIDs()
	var subErr error
	if len(ids) > 0 {
		var op *app.SubscribeOp
		subErr = c.App.InSaveList(s, func() error {
			var err error
			op, err = c.App.SubscribeWorkshop(ids, false, dry)
			return err
		})
		n := root.Section("subscriptions")
		names := map[string]string{}
		for _, m := range append(append([]app.SaveMod{}, r.Subscribe...), r.Waiting...) {
			names[m.SteamID] = m.Name
		}
		if op != nil {
			for _, it := range op.Items {
				id := ""
				if it.Record != nil {
					id = it.Record.WorkshopID
				}
				name := it.Name
				if names[id] != "" {
					name = names[id]
				}
				line := "ws:" + id + " " + name
				if it.Result != "" {
					line += "  " + it.Result
				}
				n.Add(line)
			}
			for _, note := range op.Notes {
				n.KV("note", note)
			}
		}
		if subErr != nil {
			n.KV("error", steamErr(subErr).Error())
			c.ExitCode = ExitError
		}
	}

	// The list after the fix: a save that cannot load is not launched, nor a list whose
	// Mods from the save are still missing entries (subscribed, not downloaded and scanned).
	after, err := c.App.CheckSave(s)
	if err != nil {
		return nil, err
	}
	broken := ""
	var waiting []app.SaveMod
	for _, is := range after.Issues {
		if is.Kind == app.IssueCutOff || is.Kind == app.IssueUnreadable || is.Kind == app.IssueMalformed {
			broken = is.Kind
		}
	}
	for _, m := range after.Mods {
		if m.InList && m.Instance == "" {
			waiting = append(waiting, m)
		}
	}
	if broken != "" && !changed {
		root.KV("note", "the file itself cannot load ("+broken+"); no list fix changes that (archo saves show "+showRef(s)+")")
	}
	if launch {
		switch {
		case subErr != nil:
			root.KV("launch", "not started: a subscription failed")
		case broken != "":
			root.KV("launch", "not started: the save cannot load ("+broken+")")
		case len(ids) > 0 && dry:
			root.KV("launch", "would wait: the new subscriptions download first")
		case len(waiting) > 0:
			what := "1 Mod of the save is"
			if len(waiting) > 1 {
				what = fmt.Sprintf("%d Mods of the save are", len(waiting))
			}
			n := root.KV("launch", "not started: "+what+" not installed yet; scan when Steam has downloaded it, then fix again with --launch")
			for _, m := range waiting {
				n.Add(saveModLine(ix, s, m))
			}
			n.Truncate(10, "")
			d.AddNext("archo scan", "archo saves fix "+s.Ref+" --launch")
		case dry:
			root.KV("launch", "would save "+plan+" to the game and start it")
		default:
			var lr *app.SaveResult
			err := c.App.InSaveList(s, func() error {
				var err error
				lr, err = c.App.LaunchWith(false, false)
				return err
			})
			if err != nil {
				root.KV("launch", "not started: "+err.Error())
				c.ExitCode = ExitError
				break
			}
			root.KV("launch", "started RimWorld via "+lr.Launch.Method+" with "+plan)
			root.KV("command", lr.Launch.Command())
			if !current {
				root.KV("note", plan+" is now the current plan")
			}
		}
	}
	if dry && (changed || len(ids) > 0) {
		d.AddNext("archo " + strings.Join(fixArgv(s, a), " "))
	} else if !dry && !launch && changed && len(ids) == 0 {
		d.AddNext("archo saves fix " + s.Ref + " --launch --dry-run")
	}
	return d, nil
}

// fixArgv is the saves fix command line without --dry-run.
func fixArgv(s *app.Save, a *Args) []string {
	out := []string{"saves", "fix", s.Ref}
	for _, m := range a.List("mod") {
		out = append(out, "--mod", m)
	}
	for _, f := range []string{"disable-added", "order", "launch"} {
		if a.Bool(f) {
			out = append(out, "--"+f)
		}
	}
	return out
}

// ---- saves index ----

func savesIndex(c *Ctx, a *Args) (*render.Doc, error) {
	if a.Bool("only") && len(a.Pos) == 0 {
		return nil, errors.New("--only needs the saves to read (archo saves index <save…> --only)")
	}
	var first []*app.Save
	if len(a.Pos) > 0 {
		ss, amb, err := findSaves(c, a.Pos)
		if err != nil || amb != nil {
			return amb, err
		}
		first = ss
	}
	l, err := c.App.ListSaves()
	if err != nil {
		return nil, err
	}
	order := l.IndexOrder(first)
	if a.Bool("only") {
		order = order[:len(first)]
	}
	var progress func(app.SaveIndexProgress)
	if a.Bool("progress") && Progress != nil {
		progress = func(p app.SaveIndexProgress) { Progress(app.SavesIndexLine(p)) }
	}
	r, err := c.App.IndexSaves(order, progress)
	if err != nil {
		return nil, err
	}
	pruned := 0
	if len(a.Pos) == 0 {
		if pruned, err = c.App.PruneSaveCache(l); err != nil {
			return nil, err
		}
	}
	head := fmt.Sprintf("saves index: read %s", render.Plural(r.Read, "save"))
	if r.Read > 0 {
		head += fmt.Sprintf(" (%s) in %s", render.Size(r.Bytes), app.Elapsed(r.Elapsed.Milliseconds()))
	}
	if r.Cached > 0 {
		head += fmt.Sprintf("; %d already read", r.Cached)
	}
	d, root := render.New(head)
	if r.Writing > 0 {
		root.KV("being written", fmt.Sprintf("%d (read on a later run)", r.Writing))
	}
	for _, e := range r.Errs {
		root.KV("error", e)
	}
	if len(r.Errs) > 0 {
		c.ExitCode = ExitError
	}
	if pruned > 0 {
		root.KV("removed", render.Plural(pruned, "cached read")+" of saves that are gone")
	}
	d.AddNext("archo saves list")
	return d, nil
}

// ---- rename, copy, backup, trash ----

func savesRename(c *Ctx, a *Args) (*render.Doc, error) {
	s, amb, err := findSave(c, a.Pos[0])
	if err != nil || amb != nil {
		return amb, err
	}
	to, err := c.App.RenameSave(s, a.Pos[1])
	if err != nil {
		return nil, err
	}
	ref := app.SaveRef(to)
	_, name := app.SplitSaveName(fileBase(to))
	d, root := render.New("renamed " + s.Ref + " " + s.Name + " → " + ref + " " + name)
	root.KV("file", to)
	d.AddNext("archo saves show " + ref)
	return d, nil
}

func savesRenameColony(c *Ctx, a *Args) (*render.Doc, error) {
	l, err := c.App.ListSaves()
	if err != nil {
		return nil, err
	}
	col, err := l.FindColony(a.Pos[0])
	if err != nil {
		s, serr := l.Find(a.Pos[0]) // a save names its colony
		if serr != nil || s.Colony == nil {
			return nil, err
		}
		col = s.Colony
	}
	name := ""
	if len(a.Pos) > 1 {
		name = a.Pos[1]
	}
	game := col.GameName
	if game == "" {
		game = col.Name
	}
	if err := c.App.RenameColony(col, name); err != nil {
		return nil, err
	}
	var d *render.Doc
	if name = strings.TrimSpace(name); name == "" || name == game {
		name = game
		d, _ = render.New("colony " + col.Name + " has its saves' name again: " + game)
	} else {
		d, _ = render.New("renamed colony " + col.Name + " → " + name + " (its saves name it " + game + ")")
	}
	d.AddNext("archo saves list --colony " + quote(name))
	return d, nil
}

func savesCopy(c *Ctx, a *Args) (*render.Doc, error) {
	s, amb, err := findSave(c, a.Pos[0])
	if err != nil || amb != nil {
		return amb, err
	}
	to, err := c.App.CopySave(s)
	if err != nil {
		return nil, err
	}
	ref := app.SaveRef(to)
	_, name := app.SplitSaveName(fileBase(to))
	d, root := render.New("copied " + s.Ref + " " + s.Name + " → " + ref + " " + name)
	root.KV("file", to)
	d.AddNext("archo saves show " + ref)
	return d, nil
}

// fileBase is a path's last element for both separators.
func fileBase(p string) string {
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func savesBackup(c *Ctx, a *Args) (*render.Doc, error) {
	ss, amb, err := findSaves(c, a.Pos)
	if err != nil || amb != nil {
		return amb, err
	}
	r, err := c.App.BackupSaves(ss, a.Bool("yes"))
	if err != nil {
		return nil, err
	}
	what := render.Plural(len(r.Saves), "save") + ", " + render.Size(r.Bytes)
	head := "backed up " + what
	if !r.Done {
		head = "backup preview, nothing written: " + what
	}
	d := saveFilesDoc(c, head, r)
	d.Nodes[0].KV("to", r.Dir)
	if !r.Done {
		d.AddNext(confirmCmd("backup", r.Saves))
	}
	return d, nil
}

func savesTrash(c *Ctx, a *Args) (*render.Doc, error) {
	ss, amb, err := findSaves(c, a.Pos)
	if err != nil || amb != nil {
		return amb, err
	}
	r, err := c.App.TrashSaves(ss, a.Bool("yes"))
	if err != nil {
		return nil, err
	}
	what := render.Plural(len(r.Saves), "save") + ", " + render.Size(r.Bytes)
	head := "moved to the recycle bin: " + what
	if !r.Done {
		head = "recycle preview, nothing moved: " + what
	}
	d := saveFilesDoc(c, head, r)
	if !r.Done {
		d.AddNext(confirmCmd("trash", r.Saves))
	}
	return d, nil
}

// saveFilesDoc lists the saves of a backup or recycle with their sizes and any failures.
func saveFilesDoc(c *Ctx, head string, r *app.SaveFilesResult) *render.Doc {
	d, root := render.New(head)
	var rows [][]string
	for _, s := range r.Saves {
		rows = append(rows, []string{s.Ref, s.Colony.Name + "/" + s.Name, render.Size(s.Size), s.Path})
	}
	root.AddColumns(rows)
	for _, e := range r.Errs {
		root.KV("error", e)
	}
	if len(r.Errs) > 0 {
		c.ExitCode = ExitError
	}
	if !r.Done {
		root.KV("confirm", "ask the player, then run it again with --yes")
	}
	return d
}

func confirmCmd(verb string, ss []*app.Save) string {
	refs := make([]string, len(ss))
	for i, s := range ss {
		refs[i] = s.Ref
	}
	return "archo saves " + verb + " " + strings.Join(refs, " ") + " --yes"
}
