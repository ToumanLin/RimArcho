package app

import "golang.org/x/sys/windows"

// Tests start SteamCMD stubs in a console with no window (see newConsole): typing into it
// works the same, and nothing appears on the screen of whoever runs the tests.
func init() { newConsole = windows.CREATE_NO_WINDOW }
