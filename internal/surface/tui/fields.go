package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// fieldLines draws the detail's one field list (06 §4.1), the GUI's fields.js in text: a
// label column, then the value; a row's button becomes the key that does it.
func fieldLines(fs []view.Field, w int) []string {
	lw := 0
	for _, f := range fs {
		lw = max(lw, runewidth.StringWidth(f.Label))
	}
	lw += 2
	var out []string
	for _, f := range fs {
		var vals []string
		switch f.Kind {
		case view.FieldDeps, view.FieldConflicts:
			for _, x := range f.Items {
				vals = append(vals, depLine(x, f.Kind == view.FieldConflicts))
			}
		case view.FieldTags:
			v := strings.Join(f.List, " "+markDot+" ")
			if len(f.List) == 0 {
				v = sDimS.Render(T("none · g adds"))
			}
			vals = []string{v}
		case view.FieldWorkshop:
			vals = []string{f.Value + sDimS.Render(" "+markDot+" ctrl+w "+T("web page")+" · ctrl+s "+T("steam page"))}
		case view.FieldAlias:
			vals = []string{f.Value + sDimS.Render("  ("+f.Extra+")")}
		default:
			vals = []string{f.Value}
		}
		for i, v := range vals {
			label := strings.Repeat(" ", lw)
			if i == 0 {
				label = sDimS.Render(fit(f.Label, lw))
			}
			if lipgloss.Width(v) <= w-lw {
				out = append(out, label+clip(v, w-lw))
				continue
			}
			for j, x := range wrap(stripANSIWidth(v), w-lw) {
				if j > 0 {
					label = strings.Repeat(" ", lw)
				}
				out = append(out, label+sText.Render(fit(x, w-lw)))
			}
		}
	}
	return out
}

// depLine: a dependency or a conflict with the named mod's state. A conflict with an
// enabled mod is the problem; one that is not here is fine.
func depLine(x view.Dep, conflict bool) string {
	switch x.State {
	case view.DepEnabled:
		if conflict {
			return sErrS.Render(ic.err) + " " + sText.Render(x.Name)
		}
		return sOK.Render(markOK) + " " + sText.Render(x.Name)
	case view.DepDisabled:
		return sWarnS.Render(ic.warn) + " " + sText.Render(x.Name) + sDimS.Render(T(" (not enabled)"))
	}
	if conflict {
		return sOK.Render(markOK) + " " + sText.Render(x.Name) + sDimS.Render(T(" (not installed)"))
	}
	return sErrS.Render(ic.err) + " " + sText.Render(x.Name) + sDimS.Render(T(" (not installed)"))
}
