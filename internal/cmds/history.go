package cmds

import (
	"fmt"
	"strconv"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// The history (08 §14, 10 §6): unsaved edits around "now", then the saved versions.
func init() {
	register(&Command{Name: "history", Summary: "the workspace's unsaved edits (undo/redo) and the saved versions", MaxArgs: 0, Run: historyCmd,
		Flags: []Flag{{Name: "all", Help: "every version (default: the newest 10)"}}})
}

func historyCmd(c *Ctx, a *Args) (*render.Doc, error) {
	h, err := c.App.History()
	if err != nil {
		return nil, err
	}
	st, err := c.App.Loadout()
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("history: %d unsaved edits, %d redoable, %d versions", len(h.Undo), len(h.Redo), len(h.Versions)))
	step := func(s app.HistoryStep) string {
		what := fmt.Sprintf("%d changes", len(s.Changes))
		if rows := changeRows(c, st, s.Changes); len(rows) == 1 {
			what = fmt.Sprintf("%s %s %s %s", rows[0][0], rows[0][1], rows[0][2], rows[0][3])
		}
		return fmt.Sprintf("%-4s %-10s %s", strconv.Itoa(abs(s.Steps)), s.Op, what)
	}
	if len(h.Redo) > 0 {
		n := root.KV("redo", "archo redo <n>")
		for i := len(h.Redo) - 1; i >= 0; i-- {
			n.Add(step(h.Redo[i]))
		}
	}
	root.KV("now", workspaceLine(st))
	if len(h.Undo) > 0 {
		n := root.KV("undo", "archo undo <n> returns to before step n")
		for _, s := range h.Undo {
			n.Add(step(s))
		}
		n.Truncate(render.DefaultLimit, "")
	}
	if len(h.Versions) > 0 {
		n := root.KV("versions", "archo loadout checkout v<n>")
		for i, v := range h.Versions {
			if i == 10 && !a.Bool("all") {
				n.Addf("… %d more (archo history --all)", len(h.Versions)-10)
				break
			}
			line := fmt.Sprintf("v%-3d %s  %-8s %d mods", v.N, c.App.FormatDate(v.Created), v.Source, len(loadout.Decode(v.Entries)))
			if v.Note != "" {
				line += "  " + v.Note
			}
			n.Add(line)
		}
	}
	return d, nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
