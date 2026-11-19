// Package buildinfo carries the release version, set by scripts/release.ps1 through
// -ldflags "-X github.com/ToumanLin/RimArcho/internal/buildinfo.Version=…".
package buildinfo

import "runtime/debug"

var (
	Version = "dev"
	Commit  = ""
	Date    = ""
	// GUI is "1" in RimArcho.exe, the Windows-GUI-subsystem link of the same program
	// (16 §8.1): with no command it opens the GUI instead of the TUI.
	GUI = ""
	// Agent is "1" in a build that offers Archo Agent (15 §7): its settings group, `archo agent`
	// commands and agent.* settings keys. 1.1 ships without it; the code stays so a later release
	// can turn it on (-X github.com/ToumanLin/RimArcho/internal/buildinfo.Agent=1).
	Agent = ""
)

// AgentOn reports whether this build offers Archo Agent (Agent).
func AgentOn() bool { return Agent == "1" }

// String is "0.5.0 (abc1234, 2026-10-19)" or, for a plain go build, "dev (<vcs revision>)".
func String() string {
	c, d := Commit, Date
	if c == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				switch s.Key {
				case "vcs.revision":
					c = s.Value
				case "vcs.time":
					if d == "" {
						d = s.Value
					}
				}
			}
		}
	}
	if Commit == "" && len(c) > 7 {
		c = c[:7] // a VCS revision; a stamped commit is printed as given (it may end in -dirty)
	}
	out := Version
	switch {
	case c != "" && d != "":
		out += " (" + c + ", " + d + ")"
	case c != "":
		out += " (" + c + ")"
	}
	return out
}
