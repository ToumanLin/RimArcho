package view

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// MigrationText is what one part of a move from another manager came to, for its line
// in the task (16 §2.18).
func MigrationText(p app.MigrationPick, out *app.MigrationOutcome) string {
	switch {
	case out == nil:
		return ""
	case out.Plan != nil:
		t := Tf("a new list %s", out.Plan.Name)
		if out.Missing > 0 {
			t += "  " + Tf("%d not installed", out.Missing)
		}
		return t
	case strings.HasPrefix(p.Part, "profile:") && out.Report != nil:
		for _, pr := range out.Report.Profiles {
			if "profile:"+pr.ID == p.Part {
				if strings.HasPrefix(pr.Status, "already imported") {
					return Tf("already the Game Config %s", pr.Environment)
				}
				return Tf("a Game Config %s", pr.Environment)
			}
		}
	}
	t := Tf("%d added, %d already there", out.Count.New, out.Count.Same)
	prefix := map[string]string{app.PartAliases: "alias:", app.PartNotes: "note:"}[p.Part]
	kept := 0
	if prefix != "" && out.Report != nil {
		for _, c := range out.Report.Conflicts {
			if strings.HasPrefix(c.Reason, prefix) {
				kept++
			}
		}
	}
	if kept > 0 {
		t += "  " + Tf("%d kept yours", kept)
	}
	return t
}
