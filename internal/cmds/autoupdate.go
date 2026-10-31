package cmds

import (
	"fmt"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// Shared by CLI status/data output and MCP's status command; no network or secrets.
func automaticStatus(c *Ctx, root *render.Node) {
	var pending []string
	var last time.Time
	for _, kind := range app.AutomaticKinds {
		if kind == "session" && c.App.Store.Meta("account:loverslab:enabled") != "true" {
			continue
		}
		s := c.App.AutomaticStatus(kind)
		if s.Checked.After(last) {
			last = s.Checked
		}
		if kind == "self" {
			// worked out against the running version: a record from before an update is stale
			if r := c.App.SelfUpdatePending(); r != nil {
				pending = append(pending, "RimArcho "+r.Version+" available")
			}
			continue
		}
		if s.Pending <= 0 {
			continue
		}
		switch kind {
		case "mods", "tools":
			name := strings.TrimSuffix(kind, "s") + " update"
			if s.Pending != 1 {
				name += "s"
			}
			pending = append(pending, fmt.Sprintf("%d %s waiting", s.Pending, name))
		case "session":
			message := "LoversLab needs login"
			if s.State == "session expires soon" {
				message = "LoversLab login expires soon"
			}
			pending = append(pending, message)
		}
	}
	value := Setting(c.App, "updates.auto") + "; "
	if len(pending) > 0 {
		value += strings.Join(pending, "; ")
	} else {
		value += "nothing waiting"
		if !last.IsZero() {
			value += " (last " + last.In(c.App.Loc).Format("01-02 15:04") + ")"
		} else {
			value += " (not checked yet)"
		}
	}
	root.KV("automatic checks", value)
}
