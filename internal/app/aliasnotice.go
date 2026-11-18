package app

// The one-time notice that Game Configs migrated from separate legacy installations use one
// game folder (LegacyInstallationAliases). The GUI's Game Config page and the TUI's Game
// Config picker show it until the player acknowledges it in either; showing it is not
// acknowledging it.

const aliasNoticeKey = "notice.shared-game-folder"

// AliasNotice is the aliased Game Configs, one comma-separated group per game folder, while
// the player has not acknowledged the notice; nil once acknowledged or without aliases.
func (a *App) AliasNotice() []string {
	if a.Store.Meta(aliasNoticeKey) == "seen" {
		return nil
	}
	return a.LegacyInstallationAliases()
}

// AcknowledgeAliasNotice records that the player dismissed the notice; it does not come back.
func (a *App) AcknowledgeAliasNotice() error {
	return a.Store.SetMeta(aliasNoticeKey, "seen")
}
