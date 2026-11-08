package selfupdate

import (
	"os"
	"path/filepath"
	"strings"
)

// WorkshopItem is the RimArcho item on the Steam Workshop (17 §5.1), created 2026-11-07.
const WorkshopItem = "3835688284"

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

// WorkshopVersion selects the highest stable v<semver> tag, ignoring Mod tags. Items
// published by 1.0.5 carry one; RimArcho 1.0.5 reads only this (17 §5.1).
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
