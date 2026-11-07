//go:build !windows

package upgradewin

// ShowError does nothing outside Windows.
func ShowError(title, text string) {}
