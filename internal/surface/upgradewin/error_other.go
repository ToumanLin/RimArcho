//go:build !windows && !linux

package upgradewin

// ShowError does nothing here (macOS).
func ShowError(title, text string) {}
