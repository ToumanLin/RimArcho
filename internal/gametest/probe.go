package gametest

import (
	"regexp"
	"strings"
)

// A probe is a test Mod that logs "[Probe] …" messages and ends with one "[Probe] RESULT …"
// or "[Probe] EXCEPTION …" line, then quits the game when -probequit is given (09 §7.1).

// ProbeQuit is the command-line switch that tells a probe to quit the game after its last line.
const ProbeQuit = "-probequit"

const probePrefix = "[Probe] "

// The converted 1.7 console format: "2026-11-05 03:36:12.000  INFO message" (console.go).
var waylithMessage = regexp.MustCompile(`^\d{4}-\d\d-\d\d \d\d:\d\d:\d\d\.\d+ +[A-Z]+ `)

// ProbeLine returns the text after "[Probe] " when a Player.log line is a probe message: the
// whole line in a Unity log, the message after the time and level in a converted 1.7 log.
func ProbeLine(line string) (string, bool) {
	line = strings.TrimRight(line, "\r")
	if loc := waylithMessage.FindStringIndex(line); loc != nil {
		line = line[loc[1]:]
	}
	return strings.CutPrefix(line, probePrefix)
}

// ProbeEnd reports whether a probe message is the probe's last line: "result", "exception" or "".
func ProbeEnd(text string) string {
	for _, end := range []string{"RESULT", "EXCEPTION"} {
		if rest, ok := strings.CutPrefix(text, end); ok && (rest == "" || rest[0] == ' ') {
			return strings.ToLower(end)
		}
	}
	return ""
}
