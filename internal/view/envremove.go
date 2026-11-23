package view

import "github.com/ToumanLin/RimArcho/internal/app"

// Removing an environment (16 §2.14): the dry run says what goes and what stays, then one
// confirmation removes it.

// EnvRemovalTitle is the confirmation's title.
func EnvRemovalTitle(r *app.EnvRemoval) string { return Tf("remove Game Config %s", r.Env.Name) }

// EnvRemovalLines says what removing the environment forgets and what stays.
func EnvRemovalLines(r *app.EnvRemoval) []string {
	out := []string{Tf("RimArcho forgets this Game Config's %d launch records, settings and finding ignores; lists stay", r.Launches)}
	if r.LastSibling {
		out = append(out, Tf("this is the last Game Config using this folder: its records and %d Mod inventory records are removed", r.Mods))
	} else {
		out = append(out, T("other Game Configs use this game folder: its inventory and analysis stay"))
	}
	out = append(out, T("no files are deleted: game, Mods, saves and user data stay"))

	if r.Current {
		if r.Next != "" {
			out = append(out, Tf("then %s is in use", r.Next))
		} else {
			out = append(out, T("no Game Config is left"))
		}
	}
	return out
}
