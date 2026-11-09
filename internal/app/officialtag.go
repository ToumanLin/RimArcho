package app

import "strings"

// OfficialTag is the tag Core and the DLCs get by default, kept on top (16 §3.8). It is the
// game's name, the same in every language.
const OfficialTag = "RimWorld"

// tagOfficial gives each of the game's own Mods (Core, the DLCs) the RimWorld tag once,
// after a scan: one the player untags, or a tag they delete, does not come back; a DLC
// installed later gets it on the scan that first sees it. A pinned-tags list the player
// stored before the tag existed gets it in front, once.
func (a *App) tagOfficial(envID string) error {
	ls, err := a.Store.OfficialLineages(envID)
	if err != nil {
		return err
	}
	for _, l := range ls {
		key := "deftag:" + l
		if a.Store.Meta(key) != "" {
			continue
		}
		if _, err := a.Store.AddTag(envID, l, OfficialTag); err != nil {
			return err
		}
		if err := a.Store.SetMeta(key, "1"); err != nil {
			return err
		}
	}
	if a.Store.Meta("deftag:pinned") != "" {
		return nil
	}
	if v := a.Store.Meta("cfg:tags.pinned"); v != "" && !pinnedHas(v, OfficialTag) {
		if err := a.Store.SetMeta("cfg:tags.pinned", OfficialTag+","+v); err != nil {
			return err
		}
	}
	return a.Store.SetMeta("deftag:pinned", "1")
}

// pinnedHas: whether a tags.pinned list (ASCII or fullwidth commas) names tag.
func pinnedHas(list, tag string) bool {
	for _, t := range strings.FieldsFunc(list, func(r rune) bool { return r == ',' || r == 0xff0c }) {
		if strings.EqualFold(strings.TrimSpace(t), tag) {
			return true
		}
	}
	return false
}
