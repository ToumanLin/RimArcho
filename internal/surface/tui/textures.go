package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Texture optimisation from the command palette (10 §2, 16 §2.14): the current plan's textures as a
// panel, with the commands that convert and take them back (run them on the : line). One
// copy at a time is Mod library's optimise textures (the shared op table).

// texturesText is the page as panel text.
func texturesText(p *view.TexturePage) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s\n", T("encoder"), p.Tool)
	if !p.ToolOK && p.Offer != "" {
		fmt.Fprintf(&b, "  %s → %s\n  : archo textures tool --install\n", p.Offer, p.OfferTo)
	}
	b.WriteString(T("the PNGs stay; RimArcho writes a .dds next to each and can take them all back") + "\n\n")
	fmt.Fprintf(&b, "%s  %s\n", T("what is there"), Tf("%d mods", p.Mods))
	fmt.Fprintf(&b, "  %s\n", p.PNG)
	if p.Ours != "" {
		fmt.Fprintf(&b, "  %s: %s\n", T("RimArcho's DDS"), p.Ours)
	}
	if p.StaleText != "" {
		fmt.Fprintf(&b, "  ⚠ %s\n", p.StaleText)
	}
	fmt.Fprintf(&b, "\n%s  %s\n", T("what converting does"), view.TextureScaleLabel(p.Scale))
	for _, f := range p.EstFacts {
		fmt.Fprintf(&b, "  %s\n", f)
	}
	if p.UpToDate > 0 {
		fmt.Fprintf(&b, "  %s\n", Tf("%d up to date", p.UpToDate))
	}
	if p.LeftAlone > 0 {
		fmt.Fprintf(&b, "  %s\n", Tf("%d left alone (a DDS of the mod's own, or another format of the same name)", p.LeftAlone))
	}
	b.WriteString("\n")
	for i, r := range p.Rows {
		if i == 20 {
			fmt.Fprintf(&b, "  … %d\n", len(p.Rows)-20)
			break
		}
		l := fmt.Sprintf("  %s  PNG %s", r.Name, r.PNG)
		if r.Convert > 0 {
			l += "  " + T("to convert") + " " + fmt.Sprint(r.Convert)
		}
		if r.DDS != "" {
			l += "  DDS " + r.DDS
		}
		if r.Stale > 0 {
			l += "  ⚠ " + T("stale") + " " + fmt.Sprint(r.Stale)
		}
		b.WriteString(l + "\n")
	}
	b.WriteString("\n: archo textures optimize --plan --dry-run\n: archo textures optimize --plan\n: archo textures optimize --plan --scale 50\n: archo textures clean --plan\n")
	return b.String()
}

func (m *model) texturesPanel() tea.Cmd {
	p, err := view.TexturePageOf(m.app, app.TexPlan, 100)
	if err != nil {
		m.say(sayFail, err.Error())
		return nil
	}
	m.openPanel(T("texture optimisation"), texturesText(p))
	return nil
}
