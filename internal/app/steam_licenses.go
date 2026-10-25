package app

import (
	"regexp"
	"strings"
)

var licenseApps = regexp.MustCompile(`(?i)^(?:-\s*)?(?:apps?|app\s*ids?|appids)\s*[:=]\s*(.*)$`)
var licenseNumbers = regexp.MustCompile(`\b[0-9]+\b`)

// Only app lists inside a license count. Package/depot IDs and log timestamps
// can contain 294100 too. SteamCMD versions use both AppID and Apps lists.
func steamOwnsRimWorld(output string) bool {
	inLicense, inApps := false, false
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(stampRe.ReplaceAllString(line, ""))
		low := strings.ToLower(line)
		if strings.HasPrefix(low, "license ") || strings.HasPrefix(low, "license:") {
			inLicense, inApps = true, false
			continue
		}
		if !inLicense {
			continue
		}
		if m := licenseApps.FindStringSubmatch(line); m != nil {
			inApps = true
			line = m[1]
		} else if inApps {
			// Wrapped app lists contain only numbers, commas, brackets and whitespace.
			if strings.Trim(line, "0123456789 ,\t[]()") != "" {
				inApps = false
			}
		}
		if inApps {
			for _, id := range licenseNumbers.FindAllString(line, -1) {
				if id == "294100" {
					return true
				}
			}
		}
	}
	return false
}
