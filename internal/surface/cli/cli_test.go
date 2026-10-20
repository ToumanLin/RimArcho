package cli

import (
	"io"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// 06 §1.1: only a bare `archo` may start the WezTerm host.
func TestWantsHost(t *testing.T) {
	for _, c := range []struct {
		argv []string
		want bool
	}{
		{nil, true},
		{[]string{"--env", "main"}, true},
		{[]string{"--here"}, false},
		{[]string{"tui"}, false},
		{[]string{"mcp"}, false},
		{[]string{"status"}, false},
		{[]string{"list", "--here"}, false},
	} {
		_, here, rest, err := globalFlags(c.argv)
		if err != nil {
			t.Fatal(err)
		}
		if got := wantsHost(rest, here); got != c.want {
			t.Errorf("%q: wantsHost %v, want %v", c.argv, got, c.want)
		}
	}
	if _, _, rest, _ := globalFlags([]string{"list", "--here"}); strings.Join(rest, " ") != "list --here" {
		t.Errorf("--here after a command is the command's: %q", rest)
	}
}

// CLI commands run without ever reaching the host step (and never download anything).
func TestCommandsNeverStartHost(t *testing.T) {
	t.Setenv("RIMARCHO_HOME", t.TempDir())
	runHost = func(*app.App, string, io.Writer) (bool, error) { t.Fatal("host step reached"); return false, nil }
	for _, argv := range [][]string{{"version"}, {"config", "get"}, {"config", "set", "terminal.host", "none"}, {"status"}, {"--env", "x", "status"}} {
		Run(argv, io.Discard, io.Discard)
	}
}
