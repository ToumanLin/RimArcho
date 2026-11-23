package view

import (
	"errors"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// Launching with a required mod not installed or not enabled (VM run B8), or without Core
// (VM 2026-11-23): the game does not load the mods that need it, or resets its mod list and
// loads Core alone, so the window asks once. The CLI and the TUI do not ask (09 §2: the pre-launch hint never blocks).

// RunAsking is Run, except that a launch without Core or with required mods missing comes back with
// Outcome.Confirm instead of starting the game; Run then launches anyway.
func RunAsking(a *app.App, what string) *Outcome {
	if what == ListLaunch {
		if title, lines := launchAsk(a); title != "" {
			// A launch refused anyway (an outside change, a running game …) says so first:
			// asking would be for nothing. Folders that can move are asked about after.
			if _, err := a.LaunchWith(true, false); err != nil {
				var de *app.DuplicatesError
				if !errors.As(err, &de) || !de.Movable() {
					return refused(a, &Outcome{What: what}, err)
				}
			}
			return &Outcome{What: what, Text: title, Confirm: lines}
		}
	}
	return Run(a, what)
}

// launchAsk is the question before a launch ("" when there is none): a list without Core
// (the game resets it and loads Core alone), then required mods that are missing, one line
// each.
func launchAsk(a *app.App) (title string, lines []string) {
	st, err := a.Loadout()
	if err != nil {
		return "", nil
	}
	ps, err := a.Problems(false, nil)
	if err != nil {
		return "", nil
	}
	var missing []string
	for _, it := range BuildNeeds(a, st, ps).Items {
		if !it.Counted || SectionOf(it.Group) != SectionBreak {
			continue
		}
		switch it.code {
		case app.CodeCoreMissing:
			return T("Core is not in the list"), []string{T("the game resets its mod list when it starts and loads Core alone; sort adds Core")}
		case "dep.missing", "dep.inactive":
			missing = append(missing, "  "+it.Text)
		}
	}
	if len(missing) == 0 {
		return "", nil
	}
	return T("required mods are missing"), append([]string{T("the game will not load the mods below and may reset its mod list to the base game:")}, missing...)
}
