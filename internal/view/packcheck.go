package view

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
)

// PackCheckOf translates the report without changing the app's diagnostic text.
func PackCheckOf(check *app.PackCheck) *app.PackCheck {
	if check == nil {
		return nil
	}
	out := *check
	out.Items = append([]app.PackCheckItem(nil), check.Items...)
	for i := range out.Items {
		item := &out.Items[i]
		names := strings.Join(item.Names, ", ")
		switch item.Part {
		case "game":
			if item.Level == finding.Info {
				item.Text = T(item.Text)
			} else {
				item.Text = Tf("built for %s, this game is %s", check.PackGame, check.Game)
			}
		case "dlc":
			if len(item.Names) == 0 {
				item.Text = T("cannot read installed official content")
			} else if item.Level == finding.Error {
				item.Text = Tf("missing DLC: %s", names)
			} else {
				item.Text = Tf("installed DLC not used by the pack: %s", names)
			}
		case "mods":
			if item.Level == finding.Error {
				item.Text = Tf("copy could not be verified: %s", names)
			} else {
				item.Text = Tf("mods not installed: %s", names)
			}
		}
	}
	return &out
}
