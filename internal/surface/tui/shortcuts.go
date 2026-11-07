package tui

import (
	"errors"
	"os"
	"os/exec"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// The editor's and Mod library's shortcuts for one mod (16 §3.5): ctrl+o its folder, ctrl+w its
// Workshop page, C the Updates page narrowed to its Lineage.

// openFolderOf shows an instance's folder in the file manager.
func (m *model) openFolderOf(id string) tea.Cmd {
	if id == "" || m.st == nil {
		m.say(sayInfo, T("RimArcho has not seen this copy: no folder"))
		return nil
	}
	mod := m.st.Ix.ByID[id]
	if mod == nil {
		return nil
	}
	dir := m.st.Ix.ModDir(mod)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		m.say(sayFail, T("the folder is not there"))
		return nil
	}
	open := m.opts.OpenDir
	if open == nil {
		open = openDir
	}
	if err := open(dir); err != nil {
		m.say(sayFail, err.Error())
	} else {
		m.say(sayOK, T("opened the folder"))
	}
	return nil
}

// openWorkshopID opens a Workshop page; without an ID the status line says so.
func (m *model) openWorkshopID(ws string) tea.Cmd {
	return m.openWorkshopPage(ws, false)
}

func (m *model) openWorkshopSteamID(ws string) tea.Cmd {
	return m.openWorkshopPage(ws, true)
}

func (m *model) openWorkshopPage(ws string, steam bool) tea.Cmd {
	if !view.IsNumber(ws) {
		m.say(sayInfo, T("no Workshop ID: not a Workshop mod"))
		return nil
	}
	open := m.opts.OpenURL
	if open == nil {
		open = openURL
	}
	u, done := view.WorkshopURL(ws), T("opened the Workshop page")
	if steam {
		u, done = view.WorkshopSteamURL(ws), T("opened the Steam page")
	}
	if err := open(u); err != nil {
		m.say(sayFail, err.Error())
	} else {
		m.say(sayOK, done)
	}
	return nil
}

// workshopOf is an instance's Workshop ID ("" for none).
func (m *model) workshopOf(id string) string {
	if m.st == nil {
		return ""
	}
	if mod := m.st.Ix.ByID[id]; mod != nil {
		return mod.WorkshopID()
	}
	return ""
}

// openChangesOf opens Updates with only the changes of the instance's Lineage; "" every mod.
func (m *model) openChangesOf(id, name string) tea.Cmd {
	m.chg.of, m.chg.ofName = id, name
	m.chg.cursor, m.chg.off = 0, 0
	m.tab = tabChanges
	m.loadChanges()
	if id != "" {
		for i, r := range m.chg.rows {
			if r.text == "3 "+T("already updated") {
				m.chg.cursor, m.chg.off = i, i
				break
			}
		}
	}
	return nil
}

// A variable so tests can never start the real program (noopen_test.go).
var openDir = func(dir string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer.exe", dir).Start()
	case "darwin":
		return exec.Command("open", dir).Start()
	case "linux", "freebsd", "openbsd", "netbsd":
		return exec.Command("xdg-open", dir).Start()
	}
	return errors.New("no file manager")
}
