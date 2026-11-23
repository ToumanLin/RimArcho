package cmds

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// saves review (12 §3): what the installed Mods add and how much of it the saves show.
// Without --read it reads no save: saves without a current full read are counted as not read.

const (
	reviewBudget = 34 // default lines of Mods (13 §3.1 rule 5)
	reviewNames  = 12 // names per category with --mod, without --all
)

func savesReview(c *Ctx, a *Args) (*render.Doc, error) {
	envName := c.App.EnvName
	if envName != "" {
		if _, err := c.App.Env(); err != nil {
			return nil, err
		}
	}
	var pkgs, ws map[string]bool
	modRef := a.Str("mod")
	if modRef != "" {
		var err error
		if pkgs, ws, err = modFilter(c, modRef); err != nil {
			return nil, err
		}
	}
	l, err := c.App.ListSaves()
	if err != nil {
		return nil, err
	}
	all := a.Bool("all")
	cols, err := reviewColonies(l, saveListFilter{envName: envName, colony: a.Str("colony"), pkgs: pkgs, ws: ws})
	if err != nil {
		return nil, err
	}
	var progress func(int, int)
	if a.Bool("progress") && Progress != nil {
		progress = func(done, total int) { Progress(fmt.Sprintf("review %d/%d", done, total)) }
	}
	r, err := c.App.SaveReview(cols, app.ReviewOpts{Read: a.Bool("read"), Progress: progress})
	if err != nil {
		return nil, err
	}
	head := "playthrough review: " + render.Plural(r.Colonies, "colony") + ", " + render.Plural(r.Saves, "save")
	if r.Colonies != 1 {
		head = "playthrough review: " + fmt.Sprintf("%d colonies", r.Colonies) + ", " + render.Plural(r.Saves, "save")
	}
	d, root := render.New(head)
	if r.Unread > 0 {
		root.KV("not counted", render.Plural(r.Unread, "save")+" without a current full read (archo saves review --read)")
	}
	root.KV("seen means", "the Def is in a save: a thing, pawn, terrain or biome that is there, research with progress, an event that fired. Not seen is not proof it was never met; recipes are not counted")
	root.KV("only set up means", "a save names it only in a bill, a quest part or research not begun: chosen or offered, not yet there")
	root.KV("never seen means", "not in these saves: content may come only late, in raids or quests, or with another storyteller or scenario; a Mod update that renamed things looks the same")
	if modRef != "" {
		n := 0
		for _, m := range r.Mods {
			if !pkgs[normPkgID(m.PackageID)] {
				continue
			}
			n++
			reviewMod(root, m, all)
		}
		if n == 0 {
			root.Add("no counted save has this Mod installed and analysed")
		}
		return d, nil
	}
	var never, config, part, whole, functional []app.ReviewMod
	for _, m := range r.Mods {
		switch m.Group() {
		case app.ReviewFunctional:
			functional = append(functional, m)
		case app.ReviewNeverSeen:
			never = append(never, m)
		case app.ReviewConfigured:
			config = append(config, m)
		default:
			if m.Seen < m.Adds {
				part = append(part, m)
			} else {
				whole = append(whole, m)
			}
		}
	}
	budget := reviewBudget
	if all {
		budget = len(r.Mods)
	}
	more := 0
	group := func(title string, ms []app.ReviewMod, line func(app.ReviewMod) string) {
		if len(ms) == 0 {
			return
		}
		n := root.Add(fmt.Sprintf("%s (%d):", title, len(ms)))
		k := min(len(ms), max(0, budget))
		if !all && len(part) > 0 && title == "never seen" {
			k = min(k, reviewBudget/2)
		}
		for _, m := range ms[:k] {
			n.KV(m.Name, line(m))
		}
		budget -= k
		more += len(ms) - k
	}
	group("never seen", never, func(m app.ReviewMod) string { return "adds " + reviewAdds(m) + reviewWhere(m) })
	group("only set up", config, func(m app.ReviewMod) string {
		return fmt.Sprintf("adds %s; %d only set up: %s%s", reviewAdds(m), len(m.Configured), reviewFew(m.Configured), reviewWhere(m))
	})
	group("partly seen", part, func(m app.ReviewMod) string {
		return fmt.Sprintf("%d of %d seen: %s%s", m.Seen, m.Adds, reviewShares(m), reviewWhere(m))
	})
	if len(whole) > 0 {
		root.KV("all seen", render.Plural(len(whole), "Mod"))
	}
	if len(functional) > 0 {
		text := " add nothing a save can show; saves cannot tell whether they were used"
		if len(functional) == 1 {
			text = " adds nothing a save can show; saves cannot tell whether it was used"
		}
		root.KV("functional", render.Plural(len(functional), "Mod")+text+reviewMarked(functional))
	}
	if more > 0 {
		root.Addf("… %d more Mods (archo saves review --all)", more)
	}
	d.AddNext("archo saves review --mod <mod>")
	d.AddNext("archo functional <mod>")
	return d, nil
}

// reviewWhere is how much play the numbers rest on.
func reviewWhere(m app.ReviewMod) string {
	if m.Colonies == 1 {
		return " · 1 colony, " + render.Plural(m.Saves, "save")
	}
	return fmt.Sprintf(" · %d colonies, %s", m.Colonies, render.Plural(m.Saves, "save"))
}

// reviewAdds is "403 terrain types, 2 buildings".
func reviewAdds(m app.ReviewMod) string {
	var out []string
	for _, cat := range m.Cats {
		out = append(out, fmt.Sprintf("%d %s", cat.Adds, contentTextLabel(cat.Key, cat.Adds)))
	}
	return strings.Join(out, ", ")
}

// reviewShares is "weapons 3/40, apparel 0/5": seen of added, by category.
func reviewShares(m app.ReviewMod) string {
	var out []string
	for _, cat := range m.Cats {
		out = append(out, fmt.Sprintf("%s %d/%d", cat.Key, cat.Seen, cat.Adds))
	}
	return strings.Join(out, ", ")
}

// reviewMod is one Mod in full: each category with the names not seen.
func reviewMod(root *render.Node, m app.ReviewMod, all bool) {
	text := "adds nothing a save can show; saves cannot tell whether it was used"
	if m.Marked == app.MarkFunctional {
		text = "marked functional; saves cannot tell whether it was used"
	}
	if !m.Functional {
		text = fmt.Sprintf("%d of %d seen", m.Seen, m.Adds)
		if m.Marked == app.MarkNotFunctional {
			text += " (marked not functional)"
		}
	}
	n := root.Add(m.Name + ": " + text + reviewWhere(m))
	if len(m.Configured) > 0 {
		names := m.Configured
		if !all && len(names) > reviewNames {
			names = append(names[:reviewNames:reviewNames], fmt.Sprintf("and %d more (--all)", len(m.Configured)-reviewNames))
		}
		n.KV("only set up", strings.Join(names, ", "))
	}
	for _, cat := range m.Cats {
		line := fmt.Sprintf("%d of %d seen", cat.Seen, cat.Adds)
		if cat.Under {
			line += " (saves undercount these)"
		}
		if k := len(cat.NotSeen); k > 0 {
			names := cat.NotSeen
			if !all && k > reviewNames {
				names = append(names[:reviewNames:reviewNames], fmt.Sprintf("and %d more (--all)", k-reviewNames))
			}
			line += "; not seen: " + strings.Join(names, ", ")
		}
		n.KV(cat.Key, line)
	}
}

// reviewFew is up to three names, then how many more.
func reviewFew(names []string) string {
	if len(names) <= 3 {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:3], ", ") + fmt.Sprintf(" and %d more", len(names)-3)
}

// reviewMarked says how many of the functional Mods the player marked.
func reviewMarked(ms []app.ReviewMod) string {
	n := 0
	for _, m := range ms {
		if m.Marked == app.MarkFunctional {
			n++
		}
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(" (%d marked by you)", n)
}

// reviewColonies copies the colonies the filters keep, each with only its kept saves.
func reviewColonies(l *app.SaveList, f saveListFilter) ([]*app.Colony, error) {
	var cols []*app.Colony
	found := f.colony == ""
	for _, col := range l.Colonies {
		if f.colony != "" && !strings.EqualFold(col.Name, f.colony) && (col.GameName == "" || !strings.EqualFold(col.GameName, f.colony)) {
			continue
		}
		found = true
		cc := *col
		cc.Saves = nil
		for _, s := range col.Saves {
			if f.envName != "" && !inEnv(s, f.envName) || f.pkgs != nil && !saveHasMod(s, f.pkgs, f.ws) {
				continue
			}
			cc.Saves = append(cc.Saves, s)
		}
		if len(cc.Saves) > 0 {
			cols = append(cols, &cc)
		}
	}
	if !found {
		return nil, fmt.Errorf("no colony named %s (archo saves list)", f.colony)
	}
	return cols, nil
}
