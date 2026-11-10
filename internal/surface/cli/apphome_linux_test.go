package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
)

func TestStartupRejectsRelativeDataHome(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("RIMARCHO_HOME", "")
	t.Setenv("XDG_DATA_HOME", "relative-data")
	for _, home := range []string{"", "relative-home"} {
		t.Setenv("HOME", home)
		var out, errOut strings.Builder
		if code := Run([]string{"status"}, &out, &errOut); code != cmds.ExitError || out.Len() != 0 || !strings.Contains(errOut.String(), "absolute HOME or XDG_DATA_HOME") {
			t.Fatalf("HOME=%q: exit %d, stdout %q, stderr %q", home, code, out.String(), errOut.String())
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed startup created a relative data directory: %v %v", entries, err)
	}
}
