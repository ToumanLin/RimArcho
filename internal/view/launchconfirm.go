package view

import (
	"errors"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// Launching with a required mod not installed or not enabled (VM run B8): the game does not
// load the mods that need it and may reset its mod list to the base game, so the window
// asks once. The CLI and the TUI do not ask (09 §2: the pre-launch hint never blocks).

// RunAsking is Run, except that a launch with required mods missing comes back with
// Outcome.Confirm instead of starting the game; Run then launches anyway.
func RunAsking(a *app.App, what string) *Outcome {
	if what == ListLaunch {
		if missing := missingRequired(a); len(missing) > 0 {
			// A launch refused anyway (an outside change, a running game …) says so first:
			// asking would be for nothing. Folders that can move are asked about after.
			if _, err := a.LaunchWith(true, false); err != nil {
				var de *app.DuplicatesError
				if !errors.As(err, &de) || !de.Movable() {
					return refused(a, &Outcome{What: what}, err)
				}
			}
			return &Outcome{What: what, Text: T("required mods are missing"),
				Confirm: append([]string{T("the game will not load the mods below and may reset its mod list to the base game:")}, missing...)}
		}
	}
	return Run(a, what)
}

// missingRequired: one line per required mod that is not installed or not enabled.
func missingRequired(a *app.App) []string {
	st, err := a.Loadout()
	if err != nil {
		return nil
	}
	ps, err := a.Problems(false, nil)
	if err != nil {
		return nil
	}
	var out []string
	for _, it := range BuildNeeds(a, st, ps).Items {
		if (it.code == "dep.missing" || it.code == "dep.inactive") && it.Counted && SectionOf(it.Group) == SectionBreak {
			out = append(out, "  "+it.Text)
		}
	}
	return out
}
