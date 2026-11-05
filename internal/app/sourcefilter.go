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
	case source.Local, source.Elsewhere: // a game's own Mods/, this one's or another's
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
	if role == LibLocal {
		return []string{a.LocalValue(s.EnvID)}
	}
	return []string{role}
}

// LocalValue is the source: value of a game's own Mods/ folder, "local-<game>" (local-main,
// local-1.6; 2026-10-25): every game's Mods/ is in the library and the editor (07 §1.7), so
// the value says which. source:local still matches all of them (LocalMatches).
func (a *App) LocalValue(envID string) string {
	if e, err := a.Store.EnvByID(envID); err == nil && e != nil && e.Name != "" {
		return LibLocal + "-" + e.Name
	}
	return LibLocal
}

// LocalMatches reports whether a source: filter value want takes the value have: equal
// (case-insensitive), or "local" for any game's "local-<game>".
func LocalMatches(want, have string) bool {
	return strings.EqualFold(want, have) || strings.EqualFold(want, LibLocal) && len(have) > len(LibLocal) && strings.EqualFold(have[:len(LibLocal)+1], LibLocal+"-")
}
