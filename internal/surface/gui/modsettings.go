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
// (WorkspaceSnapshot), as Mod library's do, off the window's own session.

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
	return view.ModSettingsRows(l), nil
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
