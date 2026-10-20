package tui

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// Import and export of the mod list (10 §4), the TUI's short form of the window's popup:
// import reads a path, share code or link, shows what it would do and offers replace or
// add; export picks a format, then a file.

// importPrompt asks where the list is.
func (m *model) importPrompt() tea.Cmd {
	m.tab = tabList
	m.prompt = &prompt{label: T("import from (a file, a share code, a Rentry or collection link): "), submit: func(m *model, text string) tea.Cmd {
		imp, err := view.ReadImport(m.app, view.ImportText, text)
		if err != nil {
			m.say(sayFail, m.humanize(err.Error()))
			return nil
		}
		p, err := view.PreviewImport(m.app, imp)
		if err != nil {
			m.say(sayFail, m.humanize(err.Error()))
			return nil
		}
		m.importChoice(imp, p)
		return nil
	}}
	return nil
}

// importChoice offers the two ways to apply the list, and the whole preview.
func (m *model) importChoice(imp *view.Import, p *view.ImportPreview) {
	replace := T("replace the enabled list") + " — " + p.Replace.Text
	merge := T("add the ones you lack") + " — " + p.Merge.Text
	show := T("show the whole preview")
	pn := &panel{title: p.Summary, choices: []string{replace, merge, show}}
	pn.onPick = func(m *model, v string) tea.Cmd {
		if v == show {
			m.openPanel(p.Summary, strings.Join(p.Lines(), "\n"), nil)
			m.afterPanel = nil
			return nil
		}
		t, err := view.ApplyImport(m.app, imp, v == merge)
		if !m.applied(err) {
			return nil
		}
		m.say(sayOK, t)
		m.clearSel()
		for k := range m.cols {
			m.refilterCol(colKind(k), "")
		}
		m.prob.loaded = false
		m.detailRef = ""
		return m.requestDetail()
	}
	m.panel = pn
}

// exportChoice picks a format, then asks for the file (a full path is suggested).
func (m *model) exportChoice() tea.Cmd {
	fs := view.ExportFormats(m.app)
	pn := &panel{title: T("export the mod list")}
	for _, f := range fs {
		pn.choices = append(pn.choices, f.Name+" — "+f.Desc)
	}
	pn.onPick = func(m *model, v string) tea.Cmd {
		for _, f := range fs {
			if f.Name+" — "+f.Desc != v {
				continue
			}
			id := f.ID
			m.prompt = &prompt{label: T("write to: "), text: filepath.Join(view.DefaultExportDir(), f.File), submit: func(m *model, text string) tea.Cmd {
				t, err := view.ExportTo(m.app, id, text, false)
				if err != nil {
					m.say(sayFail, m.humanize(err.Error()))
					return nil
				}
				m.say(sayOK, t)
				return nil
			}}
		}
		return nil
	}
	m.panel = pn
	return nil
}
