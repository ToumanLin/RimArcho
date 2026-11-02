package view

import "github.com/ToumanLin/RimArcho/internal/app"

// Removing an environment (06 §2.14): the dry run says what goes and what stays, then one
// confirmation removes it.

// EnvRemovalTitle is the confirmation's title.
func EnvRemovalTitle(r *app.EnvRemoval) string { return Tf("remove game %s", r.Env.Name) }

// EnvRemovalLines says what removing the environment forgets and what stays.
func EnvRemovalLines(r *app.EnvRemoval) []string {
	out := []string{
		Tf("RimArcho forgets: %d launch records, %d mods read from its Data/ and Mods/, its history; lists are shared by every game and stay", r.Launches, r.Mods),
		T("stays: the game folder, every mod and save; the shared mod folders and what you noted on mods"),
	}
	if r.Current {
		if r.Next != "" {
			out = append(out, Tf("then %s is in use", r.Next))
		} else {
			out = append(out, T("no game is left"))
		}
	}
	return out
}
