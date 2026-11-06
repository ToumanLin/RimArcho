package selfupdate

// WorkshopItem is set after the maintainer publishes the RimArcho item.
const WorkshopItem = ""

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
