package cmds

import (
	"fmt"
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// Shared by CLI status/data output and MCP's status command; no network or secrets.
func automaticStatus(c *Ctx, root *render.Node) {
	n := root.KV("automatic-checks", Setting(c.App, "updates.auto")+", interval "+Setting(c.App, "updates.interval"))
	for _, kind := range app.AutomaticKinds {
		s := c.App.AutomaticStatus(kind)
		checked := "never"
		if !s.Checked.IsZero() {
			checked = c.App.FormatTime(s.Checked)
		}
		n.KV(kind, fmt.Sprintf("last checked %s; %s; %d pending", checked, s.State, s.Pending))
	}
}
