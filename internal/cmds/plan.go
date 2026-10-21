package cmds

import (
	"fmt"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// Plan management (05 §4.3, 04 §1.3). Every other editing and history command acts on the
// environment's current plan.

func init() {
	register(&Command{Name: "plan list", Summary: "list the environment's plans", MaxArgs: 0, Run: planList})
	register(&Command{Name: "plan new", Args: "<name>", Summary: "create a plan: empty, or from the current plan, one of its versions, or a list file",
		MinArgs: 1, MaxArgs: 1, Flags: []Flag{{Name: "from", Value: "current|v<n>|<file>", Help: "default: an empty list"}}, Run: planNew})
	register(&Command{Name: "plan use", Args: "<name>", Summary: "make a plan current (writes no game file)", MinArgs: 1, MaxArgs: 1, Run: planUse})
	register(&Command{Name: "plan rename", Args: "<old> <new>", Summary: "rename a plan (its history stays)", MinArgs: 2, MaxArgs: 2, Run: planRename})
	register(&Command{Name: "plan remove", Args: "<name>", Summary: "delete a plan and its history (not the last one)", MinArgs: 1, MaxArgs: 1,
		Flags: []Flag{{Name: "yes", Help: "confirm"}}, Run: planRemove})
}

func planList(c *Ctx, a *Args) (*render.Doc, error) {
	ps, err := c.App.Plans()
	if err != nil {
		return nil, err
	}
	st := c.Loadout()
	d, root := render.New(fmt.Sprintf("plans: %d (%s)", len(ps), envRef(st.Ix.Env)))
	var rows [][]string
	for _, p := range ps {
		played := "never played"
		if !p.LastPlayed.IsZero() {
			played = "played " + c.App.FormatTime(p.LastPlayed)
		}
		cur := ""
		if p.Current {
			cur = "[current]"
		}
		rows = append(rows, []string{app.PlanRef(p.Plan), fmt.Sprintf("%d active", p.Entries), render.Plural(p.Versions, "version"), played, cur})
	}
	root.AddColumns(rows)
	if len(ps) > 1 {
		d.AddNext("archo plan use <name>")
	} else {
		d.AddNext("archo plan new <name> --from current")
	}
	return d, nil
}

func planNew(c *Ctx, a *Args) (*render.Doc, error) {
	r, err := c.App.NewPlan(a.Pos[0], a.Str("from"))
	if err != nil {
		return nil, err
	}
	return newPlanDoc(c, r), nil
}

// newPlanDoc reports a created plan (plan new, loadout import --new).
func newPlanDoc(c *Ctx, r *app.NewPlanResult) *render.Doc {
	d, root := render.New(fmt.Sprintf("%s created: %d active", app.PlanRef(r.Plan), len(r.List)))
	root.KV("v1", "import, "+r.Version.Note)
	if il := r.Imported; il != nil {
		if il.Title != "" {
			root.KV("list", il.Title)
		}
		if il.Game != "" {
			root.KV("game", il.Game)
		}
		missing := 0
		for _, e := range r.List {
			if e.Instance == "" {
				missing++
			}
		}
		if missing > 0 {
			root.KV("not installed", fmt.Sprint(missing))
		}
		if len(r.Unresolved) > 0 {
			root.KV("skipped", fmt.Sprintf("%d workshop IDs no installed mod has", len(r.Unresolved)))
		}
	}
	root.KV("current", app.PlanRef(r.State.Plan)+" (unchanged)")
	d.AddNext("archo plan use " + quoteArg(r.Plan.Name))
	return d
}

func planUse(c *Ctx, a *Args) (*render.Doc, error) {
	p, err := c.App.UsePlan(a.Pos[0])
	if err != nil {
		return nil, err
	}
	st, err := c.App.Loadout()
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("current plan: %s (%s)", app.PlanRef(p), envRef(st.Ix.Env)))
	root.Add(workspaceLine(st))
	root.KV("game", "ModsConfig.xml unchanged (archo save or archo launch writes this plan)")
	return d, nil
}

func planRename(c *Ctx, a *Args) (*render.Doc, error) {
	p, old, err := c.App.RenamePlan(a.Pos[0], a.Pos[1])
	if err != nil {
		return nil, err
	}
	d, _ := render.New(fmt.Sprintf("plan:%s renamed to %s", old, app.PlanRef(p)))
	return d, nil
}

func planRemove(c *Ctx, a *Args) (*render.Doc, error) {
	if !a.Bool("yes") {
		return nil, &UsageError{Msg: fmt.Sprintf("this deletes plan:%s with its versions, undo steps and staging list; add --yes to confirm", a.Pos[0])}
	}
	p, cur, err := c.App.RemovePlan(a.Pos[0])
	if err != nil {
		return nil, err
	}
	d, root := render.New(app.PlanRef(p) + "  removed")
	if cur != nil {
		root.KV("current", app.PlanRef(cur))
	}
	return d, nil
}
