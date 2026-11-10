//go:build !windows

package machine

// Wine is false outside Windows.
func Wine() bool { return false }

// TerminalDrawsTUI is true outside Windows: Unix terminals draw escape sequences.
func TerminalDrawsTUI() bool { return true }

// OwnConsole is false outside Windows.
func OwnConsole() bool { return false }
