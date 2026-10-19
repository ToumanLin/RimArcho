// Package cli is the command-line surface: it opens the data directory, strips global
// flags and hands the rest to internal/cmds.
package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/cmds"
	"github.com/rimarcho/rimarcho/internal/env"
)

// Run executes argv (without the program name) and returns the exit code.
func Run(argv []string, stdout, stderr io.Writer) int {
	envName, rest, err := globalFlags(argv)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return cmds.ExitUsage
	}
	ap, err := app.Open(env.AppHome())
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return cmds.ExitError
	}
	defer ap.Close()
	ap.EnvName = envName
	out, code := cmds.Execute(ap, rest)
	if code == cmds.ExitError || code == cmds.ExitUsage {
		io.WriteString(stderr, out)
	} else {
		io.WriteString(stdout, out)
	}
	return code
}

// globalFlags removes --env <name> / --env=<name> from argv.
func globalFlags(argv []string) (envName string, rest []string, err error) {
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--":
			return envName, append(rest, argv[i:]...), nil
		case a == "--env":
			if i+1 >= len(argv) {
				return "", nil, fmt.Errorf("--env needs a name")
			}
			envName = argv[i+1]
			i++
		case strings.HasPrefix(a, "--env="):
			envName = strings.TrimPrefix(a, "--env=")
		default:
			rest = append(rest, a)
		}
	}
	return envName, rest, nil
}
