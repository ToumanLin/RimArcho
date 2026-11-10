package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Mod settings from the command palette (16 §2.9 "Mod settings"): the current list's Mods
// that have settings as a panel, with up to settingsShown values of each file. The panel
// only reads: values change on the GUI page, with archo mod-settings set, or in the game.

const settingsShown = 10

func (m *model) modSettingsPanel() tea.Cmd {
	l, err := m.app.ModSettings()
	if err != nil {
		m.say(sayFail, err.Error())
		return nil
	}
	m.openPanel(T("mod settings"), modSettingsText(l))
	m.panel.nav, m.panel.navSub = 6, view.PageModSettings
	return nil
}

// modSettingsText is the page as panel text.
func modSettingsText(l *app.ModSettingsList) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s\n", T("folder"), l.Config)
	b.WriteString(T("change simple values on the GUI's Mod settings page, with archo mod-settings set, or in the game") + "\n")
	if len(l.Mods) == 0 {
		b.WriteString("\n" + T("no Mod of this list has a settings file, and Archo Monitor recorded no settings window yet") + "\n")
		return b.String()
	}
	for i := range l.Mods {
		sm := &l.Mods[i]
		for _, f := range sm.Files {
			d := view.ModSettingsFileDetail(sm, f.Key())
			head := sm.Name
			switch {
			case f.Kind == app.SettingsKindHugsLib && f.Title != "":
				head += "  HugsLib “" + f.Title + "”"
			case f.Kind == app.SettingsKindHugsLib:
				head += "  HugsLib"
			case f.Title != "":
				head += "  “" + f.Title + "”"
			}
			fmt.Fprintf(&b, "\n%s  %s\n", head, f.File)
			if w := d.Window; w != nil {
				fmt.Fprintf(&b, "  %s\n", Tf("in the game's window (%s)", w.Seen))
				for j, c := range w.Controls {
					if j == settingsShown {
						fmt.Fprintf(&b, "    … %d\n", len(w.Controls)-settingsShown)
						break
					}
					switch c.Kind {
					case "label":
						fmt.Fprintf(&b, "    %s\n", c.Label)
					case "button":
						fmt.Fprintf(&b, "    [%s]\n", c.Label)
					default:
						fmt.Fprintf(&b, "    %s: %s\n", c.Label, c.Text)
					}
				}
			}
			switch {
			case d.Error != "":
				fmt.Fprintf(&b, "  %s\n", d.Error)
			case !d.Exists && d.Kind == "":
				fmt.Fprintf(&b, "  %s\n", T("defaults (no file yet)"))
			case len(d.Values) == 0:
				fmt.Fprintf(&b, "  %s\n", T("the file stores no value: every value is at its default"))
			}
			for j, v := range d.Values {
				if j == settingsShown {
					fmt.Fprintf(&b, "  … %d\n", len(d.Values)-settingsShown)
					break
				}
				switch {
				case v.Default != "":
					fmt.Fprintf(&b, "  %s: %s  (%s)\n", v.Key, v.Text, Tf("default %s", v.Default))
				case v.AtDefault:
					fmt.Fprintf(&b, "  %s: %s  (%s)\n", v.Key, v.Text, T("default"))
				default:
					fmt.Fprintf(&b, "  %s: %s\n", v.Key, v.Text)
				}
			}
		}
	}
	b.WriteString("\n" + T("a value missing from the file is at its default") + "\n: archo mod-settings <ref>\n")
	return b.String()
}
