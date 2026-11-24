//go:build !windows

package cli

import (
	"context"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// SteamCMD uses this terminal on Linux and macOS; the player types the password there.
type cliPrompter struct{}

func (*cliPrompter) Secret(context.Context, string) *app.SteamSecret { return nil }
func (*cliPrompter) Phone()                                          {}
func (*cliPrompter) ask(func())                                      {}
func (*cliPrompter) wipe()                                           {}
func newCLIPrompter() *cliPrompter                                   { return nil }
