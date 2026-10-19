package cmds

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/identity"
	"github.com/rimarcho/rimarcho/internal/render"
	"github.com/rimarcho/rimarcho/internal/store"
)

// tags is the bracketed status of an instance: source and location, then flags.
func tags(c *Ctx, ix *app.Index, m *store.Mod) string {
	parts := []string{location(m)}
	if m.Status == identity.Tombstone {
		parts = append(parts, "tombstone since "+c.App.FormatDate(m.TombstoneAt))
	}
	if m.Residue {
		parts = append(parts, "residue")
	}
	if m.Unloadable != "" {
		parts = append(parts, "unloadable: "+m.Unloadable)
	}
	if b := app.Basis(m.LineageBasis); b != "" && b != "manual" && len(ix.ByLineage[m.LineageID]) > 1 {
		parts = append(parts, "copy "+b)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// location is "<source label> ws:<id>" for workshop-like sources, else "<label> <folder>".
func location(m *store.Mod) string {
	if m.Kind.WorkshopLike() {
		return m.SourceLabel + " ws:" + m.Rel
	}
	return m.SourceLabel + " " + m.Rel
}

// modRow is one aligned list row: ref, name, tags.
func modRow(c *Ctx, ix *app.Index, m *store.Mod) []string {
	return []string{ix.Ref(m.ID), m.DisplayName(), tags(c, ix, m)}
}

// addMods appends aligned instance rows, truncated with an expand hint.
func addMods(c *Ctx, n *render.Node, ix *app.Index, ms []*store.Mod, limit int, expand string) {
	shown := ms
	if limit > 0 && len(ms) > limit {
		shown = ms[:limit]
	}
	rows := make([][]string, 0, len(shown))
	for _, m := range shown {
		rows = append(rows, modRow(c, ix, m))
	}
	n.AddColumns(rows)
	if len(shown) < len(ms) {
		n.Addf("… %d more (%s)", len(ms)-len(shown), expand)
	}
}

var fullIDRef = regexp.MustCompile(`#([a-z2-7]{26})\b`)

// shortRefs rewrites full instance IDs stored in event details ("#<26 chars>") to short refs.
func shortRefs(ix *app.Index, s string) string {
	return fullIDRef.ReplaceAllStringFunc(s, func(m string) string { return ix.Ref(m[1:]) })
}

// ambiguousDoc lists the candidates of a reference that matched several instances (05 I3).
func ambiguousDoc(ap *app.App, amb *app.Ambiguous, cmd *Command) *render.Doc {
	c := &Ctx{App: ap}
	ix, err := ap.Index()
	if err != nil {
		return &render.Doc{Nodes: []*render.Node{{Line: "error: " + err.Error()}}}
	}
	d, root := render.New(fmt.Sprintf("%s matches %d instances; pick one", amb.Ref, len(amb.Mods)))
	addMods(c, root, ix, amb.Mods, 0, "")
	for _, m := range amb.Mods {
		if len(d.Next) >= 3 {
			break
		}
		d.AddNext("archo " + cmd.Name + " " + ix.Ref(m.ID))
	}
	return d
}

// quote wraps a value containing spaces for a suggested command line.
func quote(s string) string {
	if strings.ContainsAny(s, " \t\"") {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}
