package cmds

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/buildinfo"
)

// featureOff reports whether a command name or settings key belongs to a feature this build
// leaves out: Archo Agent (`agent …`, `agent.*`) unless buildinfo.AgentOn (15 §7). Such
// commands stay registered and documented, but no surface lists, completes or runs them,
// and such keys are neither listed nor set.
func featureOff(name string) bool {
	if buildinfo.AgentOn() {
		return false
	}
	return name == "agent" || strings.HasPrefix(name, "agent ") || strings.HasPrefix(name, "agent.")
}
