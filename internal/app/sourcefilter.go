package app

import (
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/source"
)

// DirectoryRole describes where a copy is registered, independently of how it
// was downloaded. Existing source labels survive directory rebinds.
func (a *App) DirectoryRole(s *source.Source) string {
	if s == nil {
		return ""
	}
	if a.IsLockSource(s) {
		return LibLocked
	}
	switch s.Kind {
	case source.Official:
		return "official"
	case source.Workshop:
		return LibWorkshop
	case source.Local:
		return LibLocal
	case source.SteamCMD:
		return LibSteamCMD
	default:
		return "other"
	}
}

// DirectoryAlias is a stable, path-free name for an additional directory.
func DirectoryAlias(s *source.Source) string {
	alias := s.Label
	switch strings.ToLower(alias) {
	case "", "official", "workshop", "local", "steamcmd", "locked", "other", "extra", "git", "page", "copy", "monitor":
		if alias == "" {
			alias = "folder/" + s.ID
		} else {
			alias = "folder/" + alias
		}
	}
	return alias
}

func DirectoryName(s *source.Source) string {
	if s == nil {
		return ""
	}
	name := filepath.Base(filepath.Clean(s.Root))
	if name == "." || name == string(filepath.Separator) || name == "" {
		return s.Label
	}
	return name
}

func (a *App) DirectoryValues(s *source.Source) []string {
	role := a.DirectoryRole(s)
	if role == "other" {
		return []string{role, DirectoryAlias(s)}
	}
	if role == "" {
		return nil
	}
	return []string{role}
}
