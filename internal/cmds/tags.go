package cmds

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// Whole-tag edits (07 §6.1, 16 §2.9 Tag manager): rename, merge, delete and a note per
// tag. Tags are global, like the lineages, so these apply in every game. The pinned tags
// (config tags.pinned, 16 §3.8) follow: a renamed or merged tag keeps its place, a deleted
// one leaves the list. The CLI's tag subcommands, MCP set_player_data and the window all
// call these.

// TagInfo is one tag: how many mods (lineages) carry it, its note, whether it is pinned.
type TagInfo struct {
	Tag    string `json:"tag"`
	N      int    `json:"n"`
	Note   string `json:"note,omitempty"`
	Pinned bool   `json:"pinned,omitempty"`
}

// TagEdit says what a merge or a delete changes: the tags removed, the tag they become
// ("" for a delete), the mods that carry one of them, and of those the ones that already
// carry Into.
type TagEdit struct {
	From     []string `json:"from"`
	Into     string   `json:"into,omitempty"`
	Lineages int      `json:"lineages"`
	Both     int      `json:"both,omitempty"`
}

// PinnedTagList is config tags.pinned, split on ASCII and fullwidth commas.
func PinnedTagList(a *app.App) []string {
	var out []string
	for _, t := range strings.FieldsFunc(Setting(a, "tags.pinned"), func(r rune) bool { return r == ',' || r == 0xff0c }) {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// TagList lists the tags: the pinned ones first in their order (with no mods too), then
// the rest by how many mods carry them, then by name.
func TagList(a *app.App) ([]TagInfo, error) {
	counts, err := a.Store.TagCounts("")
	if err != nil {
		return nil, err
	}
	notes, err := a.Store.TagNotes()
	if err != nil {
		return nil, err
	}
	var out []TagInfo
	seen := map[string]bool{}
	byKey := map[string]TagInfo{}
	for _, c := range counts {
		n := 0
		fmt.Sscan(c[1], &n)
		byKey[strings.ToLower(c[0])] = TagInfo{Tag: c[0], N: n, Note: notes[strings.ToLower(c[0])]}
	}
	for _, p := range PinnedTagList(a) {
		k := strings.ToLower(p)
		if seen[k] {
			continue
		}
		seen[k] = true
		t, ok := byKey[k]
		if !ok {
			t = TagInfo{Tag: p, Note: notes[k]}
		}
		t.Pinned = true
		out = append(out, t)
	}
	var rest []TagInfo
	for k, t := range byKey {
		if !seen[k] {
			rest = append(rest, t)
		}
	}
	slices.SortFunc(rest, func(x, y TagInfo) int {
		if c := cmp.Compare(y.N, x.N); c != 0 {
			return c
		}
		return cmp.Compare(strings.ToLower(x.Tag), strings.ToLower(y.Tag))
	})
	return append(out, rest...), nil
}

// findTag returns the tag as listed (its stored spelling), if there is one.
func findTag(list []TagInfo, tag string) (TagInfo, bool) {
	for _, t := range list {
		if strings.EqualFold(t.Tag, strings.TrimSpace(tag)) {
			return t, true
		}
	}
	return TagInfo{}, false
}

// cleanTags trims the tags, drops empty ones and repeats (ignoring case).
func cleanTags(tags []string) []string {
	var out []string
	for _, t := range tags {
		if t = strings.TrimSpace(t); t != "" && !slices.ContainsFunc(out, func(o string) bool { return strings.EqualFold(o, t) }) {
			out = append(out, t)
		}
	}
	return out
}

// RenameTag gives a tag another name, which may differ only in case. A name another tag
// already has is refused: that is a merge.
func RenameTag(a *app.App, from, to string, dryRun bool) (TagEdit, error) {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if to == "" {
		return TagEdit{}, &UsageError{Msg: "give the new name"}
	}
	list, err := TagList(a)
	if err != nil {
		return TagEdit{}, err
	}
	if t, ok := findTag(list, to); ok && !strings.EqualFold(from, to) {
		return TagEdit{}, fmt.Errorf("tag %q already exists: archo tag merge %s --into %s", t.Tag, quoteArg(from), quoteArg(t.Tag))
	}
	return MergeTags(a, []string{from}, to, dryRun)
}

// MergeTags replaces the tags from with into on every mod; into may be new or one of from
// (that one stays). Their notes join into's, pinned places go to into.
func MergeTags(a *app.App, from []string, into string, dryRun bool) (TagEdit, error) {
	into = strings.TrimSpace(into)
	if into == "" {
		return TagEdit{}, &UsageError{Msg: "give the tag to merge into (--into)"}
	}
	list, err := TagList(a)
	if err != nil {
		return TagEdit{}, err
	}
	if t, ok := findTag(list, into); ok && t.Tag != into && !slices.ContainsFunc(from, func(f string) bool { return strings.EqualFold(f, into) }) {
		into = t.Tag // an existing tag keeps its spelling unless it is the one renamed
	}
	var src []string
	for _, f := range cleanTags(from) {
		t, ok := findTag(list, f)
		if !ok {
			return TagEdit{}, fmt.Errorf("no tag %q (archo tag list)", f)
		}
		if t.Tag == into {
			continue // merging a tag into itself
		}
		src = append(src, t.Tag)
	}
	if len(src) == 0 {
		return TagEdit{}, &UsageError{Msg: "give at least one tag other than the target"}
	}
	e, err := previewTagEdit(a, src, into)
	if err != nil || dryRun {
		return e, err
	}
	if _, err := a.Store.MergeTags(src, into); err != nil {
		return e, err
	}
	return e, repin(a, src, into)
}

// DeleteTags removes the tags from every mod, with their notes and pinned places.
func DeleteTags(a *app.App, tags []string, dryRun bool) (TagEdit, error) {
	list, err := TagList(a)
	if err != nil {
		return TagEdit{}, err
	}
	var src []string
	for _, f := range cleanTags(tags) {
		t, ok := findTag(list, f)
		if !ok {
			return TagEdit{}, fmt.Errorf("no tag %q (archo tag list)", f)
		}
		src = append(src, t.Tag)
	}
	if len(src) == 0 {
		return TagEdit{}, &UsageError{Msg: "give the tags to delete"}
	}
	e, err := previewTagEdit(a, src, "")
	if err != nil || dryRun {
		return e, err
	}
	if _, err := a.Store.DeleteTags(src); err != nil {
		return e, err
	}
	return e, repin(a, src, "")
}

// SetTagNote sets (or with "" clears) a tag's note.
func SetTagNote(a *app.App, tag, note string) (string, error) {
	list, err := TagList(a)
	if err != nil {
		return "", err
	}
	t, ok := findTag(list, tag)
	if !ok {
		return "", fmt.Errorf("no tag %q (archo tag list)", strings.TrimSpace(tag))
	}
	return t.Tag, a.Store.SetTagNote(t.Tag, note)
}

// SetTagPinned pins a tag (at the end of tags.pinned) or unpins it.
func SetTagPinned(a *app.App, tag string, on bool) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return &UsageError{Msg: "give the tag"}
	}
	old := PinnedTagList(a)
	is := func(p string) bool { return strings.EqualFold(p, tag) }
	if slices.ContainsFunc(old, is) == on {
		return nil
	}
	out := slices.DeleteFunc(slices.Clone(old), is)
	if on {
		out = append(out, tag)
	}
	v := strings.Join(out, ",")
	if v == "" {
		v = "," // as in repin
	}
	return a.Store.SetMeta("cfg:tags.pinned", v)
}

func previewTagEdit(a *app.App, from []string, into string) (TagEdit, error) {
	e := TagEdit{From: from, Into: into}
	lins, err := a.Store.TagLineages(from)
	if err != nil {
		return e, err
	}
	e.Lineages = len(lins)
	if into != "" {
		have, err := a.Store.TagLineages([]string{into})
		if err != nil {
			return e, err
		}
		for lin := range lins {
			if _, ok := have[lin]; ok {
				e.Both++
			}
		}
	}
	return e, nil
}

// repin keeps tags.pinned in step: the first pinned tag of from becomes into (unless into
// is pinned already), the rest leave. An unchanged list is not written, so the preset
// stays a preset.
func repin(a *app.App, from []string, into string) error {
	old := PinnedTagList(a)
	var out []string
	has := func(t string) bool {
		return slices.ContainsFunc(out, func(o string) bool { return strings.EqualFold(o, t) })
	}
	intoPinned := into != "" && slices.ContainsFunc(old, func(o string) bool { return strings.EqualFold(o, into) })
	changed := false
	for _, p := range old {
		switch {
		case into != "" && strings.EqualFold(p, into):
			if !has(into) {
				out = append(out, into)
			}
			changed = changed || p != into // a rename that changed only the case
		case slices.ContainsFunc(from, func(f string) bool { return strings.EqualFold(f, p) }):
			if into != "" && !intoPinned && !has(into) {
				out = append(out, into)
			}
			changed = true
		case !has(p):
			out = append(out, p)
		}
	}
	if !changed {
		return nil
	}
	v := strings.Join(out, ",")
	if v == "" {
		v = "," // no pinned tags; an empty value would bring the preset back
	}
	return a.Store.SetMeta("cfg:tags.pinned", v)
}
