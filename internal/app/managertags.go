package app

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/l10ndata"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// ManagerTagNames name the tags a migration makes for what another manager keeps per Mod
// and RimArcho keeps as tags (07 §6.2, §6.4): a colour mark, and "don't warn me about
// this Mod". Empty fields use the English ones.
type ManagerTagNames struct {
	Color  string // fmt: the manager, then the colour ("RimSort colour #e06c75")
	Ignore string // fmt: the manager ("RimSort: ignored warnings")
}

// ManagerTagNamesIn are the names in a language (ui.lang resolved), from the locale's
// supplemental table, so the window and the CLI name the same tags the same way.
func ManagerTagNamesIn(lang string) ManagerTagNames {
	get := func(key string) string {
		if v := l10ndata.Extra(lang, key); v != key {
			return v
		}
		return ""
	}
	return ManagerTagNames{Color: get("@tag.manager-colour"), Ignore: get("@tag.manager-ignore")}
}

func (n ManagerTagNames) color(manager, hex string) string {
	f := n.Color
	if f == "" {
		f = "%s colour %s"
	}
	return fmt.Sprintf(f, manager, hex)
}

func (n ManagerTagNames) ignore(manager string) string {
	f := n.Ignore
	if f == "" {
		f = "%s: ignored warnings"
	}
	return fmt.Sprintf(f, manager)
}

// managerHex is a colour another manager stored, as #rrggbb ("" when it is not one):
// RimSort writes QColor names (#rrggbb, or #aarrggbb with alpha), RimCrow CSS hex.
func managerHex(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if !strings.HasPrefix(v, "#") {
		return ""
	}
	hex := v[1:]
	switch len(hex) {
	case 8:
		hex = hex[2:] // #aarrggbb
	case 3:
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	case 6:
	default:
		return ""
	}
	for _, r := range hex {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return ""
		}
	}
	return "#" + hex
}

// tagImport adds tags to lineages and colours to tags for one migration, counting them in
// the report. A tag that already has a colour keeps it.
type tagImport struct {
	a      *App
	env    string
	dry    bool
	rep    *RimCrowReport
	colors map[string]string // lower-cased tag → colour, the store's plus this import's
}

func (a *App) newTagImport(env string, dry bool, rep *RimCrowReport) (*tagImport, error) {
	colors, err := a.Store.TagColors()
	if err != nil {
		return nil, err
	}
	return &tagImport{a: a, env: env, dry: dry, rep: rep, colors: colors}, nil
}

// add counts the tag under part, the part of the migration it comes from.
func (t *tagImport) add(part, lin string, cur *store.PlayerData, tag string) error {
	if tag = strings.TrimSpace(tag); tag == "" {
		return nil
	}
	if containsFold(cur.Tags, tag) {
		t.rep.TagSame++
		t.rep.part(part).Same++
		return nil
	}
	cur.Tags = append(cur.Tags, tag)
	t.rep.TagNew++
	t.rep.part(part).New++
	if t.dry {
		return nil
	}
	_, err := t.a.Store.AddTag(t.env, lin, tag)
	return err
}

func (t *tagImport) color(tag, hex string) error {
	tag, hex = strings.TrimSpace(tag), managerHex(hex)
	if tag == "" || hex == "" || t.colors[strings.ToLower(tag)] != "" {
		return nil
	}
	t.colors[strings.ToLower(tag)] = hex
	t.rep.TagColors++
	if t.dry {
		return nil
	}
	return t.a.Store.SetTagColor(tag, hex)
}
