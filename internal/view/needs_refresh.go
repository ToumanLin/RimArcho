package view

import "github.com/ToumanLin/RimArcho/internal/app"

// RefreshDirectories replaces live filesystem observations without rebuilding findings.
func RefreshDirectories(a *app.App, old *Needs) *Needs {
	n := *old
	n.Items = nil
	for _, it := range old.Items {
		directory := false
		for _, act := range it.Actions {
			if act.Kind == ActLocateDirectory {
				directory = true
				break
			}
		}
		if !directory {
			n.Items = append(n.Items, it)
		}
	}
	for _, it := range directoryNeeds(a) {
		it.Section = SectionOf(it.Group)
		identifyNeed(&it)
		n.Items = append(n.Items, it)
	}
	indexNeeds(&n)
	return &n
}

// RebindNeedsLinks gives a detached reader's actions identities registered on the
// receiving App. Only server-owned snapshots cross this boundary, never GUI JSON.
func RebindNeedsLinks(a *app.App, n *Needs, links []*app.GameLink) *Needs {
	for i, it := range n.Items {
		if it.code != "mods.link-health" {
			continue
		}
		copy := *n
		copy.Items = append([]Need{}, n.Items[:i]...)
		for _, fresh := range linkNeeds(a, links) {
			fresh.Section = SectionOf(fresh.Group)
			identifyNeed(&fresh)
			copy.Items = append(copy.Items, fresh)
		}
		copy.Items = append(copy.Items, n.Items[i+1:]...)
		indexNeeds(&copy)
		return &copy
	}
	return n
}
