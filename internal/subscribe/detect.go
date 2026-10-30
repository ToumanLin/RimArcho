package subscribe

import (
	"net/url"
	"regexp"
	"strings"
)

// Detect says what a pasted text is, the way the 添加订阅 paste box recognises it (06 §2.9):
// "git", "loverslab", "page", "workshop", "bad", or "" for nothing. Repositories are
// recognised on known Git hosts (owner/name, nothing more) or by a .git ending; any other
// https page is a download page that is read first. The window's copy is detect in
// surface/gui/assets/js/subscribe.js; both are tested against testdata/detect.json.
func Detect(text string) string {
	t := strings.TrimSpace(text)
	if t == "" {
		return ""
	}
	if workshopIDs.MatchString(t) || workshopLink.MatchString(t) {
		return "workshop"
	}
	if strings.ContainsAny(t, " \t\r\n\v\f") {
		return "bad"
	}
	if loversLabLink.MatchString(t) {
		return "loverslab"
	}
	u, err := url.Parse(t)
	if err != nil || u.Host == "" || u.Opaque != "" {
		return "bad"
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && scheme != "http" {
		return "bad"
	}
	var parts []string
	for _, p := range strings.Split(u.Path, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if strings.HasSuffix(u.Path, ".git") || gitHosts[host] && len(parts) == 2 {
		return "git"
	}
	if scheme == "https" {
		return "page"
	}
	return "bad"
}

var (
	workshopIDs   = regexp.MustCompile(`^\d{5,}(\s+\d{5,})*$`)
	workshopLink  = regexp.MustCompile(`^https://steamcommunity\.com/(sharedfiles|workshop)/filedetails/\?(.*&)?id=\d+`)
	loversLabLink = regexp.MustCompile(`^https://(www\.)?loverslab\.com/files/file/[^?#]+$`)
	gitHosts      = map[string]bool{"github.com": true, "gitlab.com": true, "codeberg.org": true, "gitgud.io": true, "bitbucket.org": true, "gitee.com": true}
)
