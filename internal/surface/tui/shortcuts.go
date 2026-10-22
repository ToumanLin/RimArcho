package tui

import (
	"errors"
	"os"
	"os/exec"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// The editor's and Mod 库存's shortcuts for one mod (06 §3.5): ctrl+o its folder, ctrl+w its
// Workshop page, C the 变化 page narrowed to its Lineage.

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
	if !view.IsNumber(ws) {
		m.say(sayInfo, T("no Workshop ID: not a Workshop mod"))
		return nil
	}
	open := m.opts.OpenURL
	if open == nil {
		open = openURL
	}
	if err := open(view.WorkshopURL(ws)); err != nil {
		m.say(sayFail, err.Error())
	} else {
		m.say(sayOK, T("opened the Workshop page"))
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

// openChangesOf opens 变化 with only the changes of the instance's Lineage; "" every mod.
func (m *model) openChangesOf(id, name string) tea.Cmd {
	m.chg.of, m.chg.ofName = id, name
	m.chg.cursor, m.chg.off = 0, 0
	m.tab = tabChanges
	m.loadChanges()
	return nil
}

func openDir(dir string) error {
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
