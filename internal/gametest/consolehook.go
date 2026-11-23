package gametest

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// consoleHook is a .NET startup hook that sets the console encoding to UTF-8 (consolehook/).
//
//go:embed consolehook/ArchoConsoleUtf8.dll
var consoleHook []byte

const consoleHookName = "ArchoConsoleUtf8.dll"

// utf8Console makes a 1.7 game write its console in UTF-8 (20 §2). RimWorld.exe is a GUI
// program without a console, so .NET would encode stdout and stderr in the ANSI code page,
// which turns Chinese Mod names on an English system into '?'. The hook is copied into the
// run directory and named in DOTNET_STARTUP_HOOKS for this process only.
func utf8Console(cmd *exec.Cmd, runDir string) error {
	path := filepath.Join(runDir, consoleHookName)
	if err := os.WriteFile(path, consoleHook, 0o644); err != nil {
		return fmt.Errorf("gametest: write console hook: %w", err)
	}
	hooks := path
	if old := os.Getenv("DOTNET_STARTUP_HOOKS"); old != "" {
		hooks += string(os.PathListSeparator) + old
	}
	cmd.Env = append(cmd.Environ(), "DOTNET_STARTUP_HOOKS="+hooks)
	return nil
}
