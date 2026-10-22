package cmds

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// addLaunchChecks lists the enabled list's problems that most often cause errors in the game
// (app.LaunchChecks, 04 §8), one row per code with why and what to do. It never blocks the
// launch: a failure to compute them is a line of its own.
func addLaunchChecks(c *Ctx, root *render.Node, ix *app.Index) {
	ws, as, err := c.App.LaunchWarnings()
	if err != nil {
		root.KV("checks", "not available: "+err.Error())
		return
	}
	switch {
	case as == nil || as.Run == nil:
		root.KV("checks", "declarations only: the def and patch layers were never analysed (archo analyze)")
	case !as.Fresh:
		root.KV("checks", "the def and patch layers are from a stale analysis (archo analyze)")
	}
	if len(ws) == 0 {
		return
	}
	n := 0
	for _, w := range ws {
		n += len(w.Problems)
	}
	sec := root.Add(fmt.Sprintf("before you play: %d problems that often cause errors in the game (they do not block the launch)", n))
	for _, w := range ws {
		sev := finding.Info
		for _, p := range w.Problems {
			if sevRank(p.Severity) < sevRank(sev) {
				sev = p.Severity
			}
		}
		var text, fix string
		if len(w.Problems) == 1 {
			p := w.Problems[0]
			text = expand(ix, p.Claim, p.SubjectInstance())
			if id := p.SubjectInstance(); id != "" && !strings.Contains(p.Claim, finding.Inst(id)) {
				text = modName(ix, id) + ": " + text
			}
			fix = "archo problems show " + p.Ref
			if len(p.Suggestions) > 0 {
				fix = expand(ix, p.Suggestions[0], "")
			}
		} else {
			text = w.Check.Many(len(w.Problems), max(1, len(w.Mods)))
			var names []string
			for i, id := range w.Mods {
				if i == 3 {
					names = append(names, "…")
					break
				}
				names = append(names, modName(ix, id))
			}
			if len(names) > 0 {
				text += ": " + strings.Join(names, ", ")
			}
			fix = "archo problems --code " + w.Check.Code
			if s := commonFix(w.Problems); s != "" {
				fix = expand(ix, s, "")
			}
		}
		row := sec.Add(fmt.Sprintf("%s %s  %s", sev, w.Check.Code, text))
		row.Add("why: " + w.Check.Why)
		row.Add("fix: " + fix)
	}
}

// modName is "#ref Name".
func modName(ix *app.Index, id string) string {
	if m := ix.ByID[id]; m != nil {
		return ix.Ref(id) + " " + m.DisplayName()
	}
	return "#?"
}

// commonFix is the first suggestion when every problem has the same one ("" otherwise).
func commonFix(ps []*app.Problem) string {
	s := ""
	for _, p := range ps {
		if len(p.Suggestions) == 0 || s != "" && p.Suggestions[0] != s {
			return ""
		}
		s = p.Suggestions[0]
	}
	return s
}
