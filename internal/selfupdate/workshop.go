package selfupdate

import (
	"os"
	"path/filepath"
	"strings"
)

// WorkshopItem is the RimArcho item on the Steam Workshop (17 §5.1), created 2026-11-07.
const WorkshopItem = "3835688284"

// WorkshopVersion selects the highest stable v<semver> tag, ignoring Mod tags.
func WorkshopVersion(tags []string) string {
	var latest string
	for _, tag := range tags {
		if len(tag) == 0 || tag[0] != 'v' {
			continue
		}
		v, ok := parseVersion(tag)
		if ok && len(v.pre) == 0 && (latest == "" || Newer(latest, tag)) {
			latest = tag
		}
	}
	if latest == "" {
		return ""
	}
	return latest[1:]
}

// SubscribedVersion is the highest stable release a Workshop item folder holds, read
// from its zip names (each with its checksum). The anonymous Web API does not return
// unlisted items, so a subscriber's folder is the other place the version shows.
func SubscribedVersion(dir, goos, goarch string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	suffix := "-" + goos + "-" + goarch + ".zip"
	var tags []string
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() || !strings.HasPrefix(name, "archo-") || !strings.HasSuffix(name, suffix) {
			continue
		}
		if st, err := os.Stat(filepath.Join(dir, name+".sha256")); err != nil || !st.Mode().IsRegular() {
			continue
		}
		tags = append(tags, "v"+strings.TrimSuffix(strings.TrimPrefix(name, "archo-"), suffix))
	}
	return WorkshopVersion(tags)
}
