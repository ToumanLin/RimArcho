package gui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Mod library › Mod settings (06 §2.9 "Mod settings"): the current list's Mods that have
// settings and what one file stores. Reads use their own session over the persisted list
// (WorkspaceSnapshot), as Mod library's do, off the window's own session; a change uses the
// window's session.

func (s *API) modSettingsReader() (*app.App, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.a.Uninstalled {
		return nil, errors.New("RimArcho was uninstalled; account operations are disabled")
	}
	return s.readInventorySession()
}

// ModSettings is the page's table: one row per settings file of the current list's Mods.
func (s *API) ModSettings() (*view.ModSettingsPage, error) {
	r, err := s.modSettingsReader()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	defer s.free.soon()
	l, err := r.ModSettings()
	if err != nil {
		return nil, err
	}
	p := view.ModSettingsRows(l)
	if g, _ := r.RunningGame(); g != nil {
		p.Locked = view.T("the game is running: values change here once it is closed")
	}
	return p, nil
}

// ModSettingChange is SetModSetting's reply: what was written, and the file read again.
type ModSettingChange struct {
	Message string                  `json:"message"`
	Backup  string                  `json:"backup,omitempty"`
	Detail  *view.ModSettingsDetail `json:"detail"`
}

// SetModSetting changes one value of a Mod's settings file or HugsLib pack (class: its key)
// to value, or resets it (06 §2.9): only while no game runs, after backing the file up. old
// is the stored text the page showed ("" not stored); a file changed since is refused.
func (s *API) SetModSetting(instance, class, name, value, old string, reset bool) (*ModSettingChange, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.a.Uninstalled {
		return nil, errors.New("RimArcho was uninstalled; account operations are disabled")
	}
	ix, err := s.a.Index()
	if err != nil {
		return nil, err
	}
	m := ix.ByID[instance]
	if m == nil {
		return nil, fmt.Errorf("no mod %s", instance)
	}
	w, err := s.a.EditModSettings(m, class, []app.SettingEdit{{Name: name, Value: value, Reset: reset, Old: &old}}, false)
	if err != nil {
		var ie *app.SettingInputError
		if errors.As(err, &ie) {
			return nil, errors.New(view.SettingErrorText(ie))
		}
		return nil, err
	}
	sm, err := s.a.ModSettingsOf(m)
	if err != nil {
		return nil, err
	}
	d := view.ModSettingsFileDetail(sm, class)
	label, now := name, value
	for _, v := range d.Values {
		if v.Key == name || v.Name == name {
			label, now = v.Key, v.Text
		}
	}
	return &ModSettingChange{Message: view.SettingsWriteText(w, label, now), Backup: w.Backup, Detail: d}, nil
}

// ModSettingsFile is what one of a Mod's settings files stores (class: its Mod class). The
// page passes the instance and class, never a path.
func (s *API) ModSettingsFile(instance, class string) (*view.ModSettingsDetail, error) {
	r, err := s.modSettingsReader()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	ix, err := r.Index()
	if err != nil {
		return nil, err
	}
	m := ix.ByID[instance]
	if m == nil {
		return nil, fmt.Errorf("no mod %s", instance)
	}
	sm, err := r.ModSettingsOf(m)
	if err != nil {
		return nil, err
	}
	return view.ModSettingsFileDetail(sm, class), nil
}

// OpenModSettingsFolder shows the environment's Config folder, where settings files are.
func (s *API) OpenModSettingsFolder() error {
	s.mu.Lock()
	ix, err := s.a.Index()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if ix.Env.UserDataRoot == "" {
		return errors.New(view.T("the folder is not there"))
	}
	dir := filepath.Join(ix.Env.UserDataRoot, "Config")
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return errors.New(view.T("the folder is not there"))
	}
	if s.openDir != nil {
		return s.openDir(dir)
	}
	return openDir(dir)
}
