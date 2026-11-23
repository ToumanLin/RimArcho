package selfupdate

import "strings"

// stampPrefix starts the description's last line, which names the item's release. The
// item's tags are the game's (Mod and the game versions); the anonymous details request
// returns the description but not key-value tags or metadata.
const stampPrefix = "RimArcho v"

// Stamp ends a Workshop description with the release it publishes, replacing an
// earlier stamp at its end so a description file that already carries one is not
// stamped twice.
func Stamp(description, version string) string {
	lines := strings.Split(strings.TrimRight(description, " \t\r\n"), "\n")
	for len(lines) > 0 && stampVersion(lines[len(lines)-1]) != "" {
		lines = lines[:len(lines)-1]
	}
	body := strings.TrimRight(strings.Join(lines, "\n"), " \t\r\n")
	if body == "" {
		return stampPrefix + version
	}
	return body + "\n\n" + stampPrefix + version
}

// DescriptionVersion is the stable release a Workshop description's stamp names: its
// last stamp line, or "" when it has none (an item published before 1.0.6, or a
// description edited on the web).
func DescriptionVersion(description string) string {
	lines := strings.Split(description, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if v := stampVersion(lines[i]); v != "" {
			return v
		}
	}
	return ""
}

func stampVersion(line string) string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, stampPrefix) {
		return ""
	}
	v := strings.TrimPrefix(line, "RimArcho ")
	if p, ok := parseVersion(v); !ok || len(p.pre) > 0 || strings.Contains(v, "+") {
		return ""
	}
	return v[1:]
}
