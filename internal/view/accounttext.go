package view

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/l10ndata"
)

func init() {
	enText["environment create failed"] = "WebView2 setup failed"
	enText["environment interface not available"] = "WebView2 interface not available"
}

type interfaceFormat struct {
	key string
	re  *regexp.Regexp
}

var interfaceFormats = []interfaceFormat{}

func init() {
	for _, key := range []string{"cannot tell which linked repository is the mod (%d: %s); subscribe to it with archo subscribe git <repository>", "cannot rule out an incoming link at %s: %s; leave it alone and resolve the check failure first", "moving or removing %s would break foreign link %s -> %s (%s); detach it yourself first", "unfinished link operation: %s; review archo links repair --all --dry-run first", "the game is running (pid %d); close it before updating subscriptions", "%s is not a page subscription; --from is only for page subscriptions", "included link is not a prefixed entry in the selected game scope: %s", "old folder is in the recycle bin; replacement remains at %s: %w", "repository root has no About/About.xml; mod subdirectories: %s", "Workshop %s: Steam did not confirm subscription (%s); %s", "installed at %s but scan did not find a live instance", "game is running: %s; stop it before repairing links", "install SteamCMD in Settings; Workshop pages: %s", "SteamCMD download directory overlaps src:%s (%s)", "%s is not directly inside a subscription source", "SteamCMD item %s has no About/About.xml at %s", "new link path is occupied or inaccessible: %s", "%s: updating %s requires --from <zip|folder>", "%s: update check unavailable offline for %s", "incoming link changed since preview: %s", "%s (%s %q) is subscribed already at %s", "%s Workshop items could not be checked", "cannot check incoming links in %s: %w", "scan did not find subscription at %s", "link recovery remains pending: %s", "target changed since preview: %s", "page is subscribed already: %s", "link changed since preview: %s", "Steam request failed (code %s)", "affected game is running: %s", "%s is not a subscription", "SteamCMD item %s: %s; %s", "invalid repair entry: %s", "not a Workshop URL: %s", "invalid Workshop ID %q", "cannot reattach %s: %s", "unknown instance: %s", "subscription %s: %w", "game is running: %s", "check failed: %s", "Repository: %s", "Version: %s", "failed: %s", "Page: %s"} {
		interfaceFormats = append(interfaceFormats, interfaceFormatFor(key))
	}
}

// interfaceFormatFor matches a whole line written with the format key.
func interfaceFormatFor(key string) interfaceFormat {
	pattern := regexp.QuoteMeta(key)
	for _, verb := range []string{"%s", "%d", "%w"} {
		pattern = strings.ReplaceAll(pattern, verb, "(.+?)")
	}
	pattern = strings.ReplaceAll(pattern, "%q", `"([^"]*)"`)
	return interfaceFormat{key, regexp.MustCompile("^" + pattern + "$")}
}

// Text translates fixed diagnostics and joined reasons, retaining data such as
// names, paths, site agreements and HRESULTs verbatim.
func Text(s string) string {
	if Lang() == "en" {
		s = l10ndata.LegacyEnglish(s)
	}

	if v := T(s); v != s {
		return v
	}
	for _, f := range interfaceFormats {
		if match := f.re.FindStringSubmatch(s); match != nil {
			if Lang() == "en" {
				return s
			}
			args := make([]any, len(match)-1)
			for i, v := range match[1:] {
				args[i] = v
			}
			format := strings.NewReplacer("%d", "%s", "%w", "%s").Replace(T(f.key))
			return fmt.Sprintf(format, args...)
		}
	}
	for _, sep := range []string{"\n", "; ", "\uff1b"} {
		if strings.Contains(s, sep) {
			parts := strings.Split(s, sep)
			changed := false
			for i := range parts {
				original := parts[i]
				parts[i] = Text(parts[i])
				changed = changed || parts[i] != original
			}
			if !changed {
				return s
			}
			if sep == "\n" {
				return strings.Join(parts, "\n")
			}
			return strings.Join(parts, T("; "))
		}
	}
	if i := strings.LastIndex(s, " 0x"); i >= 0 {
		return T(s[:i]) + s[i:]
	}
	return s
}
