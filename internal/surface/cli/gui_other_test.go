//go:build !windows

package cli

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestGUIUnavailable(t *testing.T) {
	t.Setenv("RIMARCHO_HOME", t.TempDir())
	t.Setenv("ARCHO_LANG", "en")
	t.Cleanup(func() { view.SetLang("en") })
	var out, errOut strings.Builder
	code := Run([]string{"gui"}, &out, &errOut)
	if code != cmds.ExitError || out.Len() != 0 || errOut.String() != "error: This build has no window; the text interface (archo) has the same features.\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out.String(), errOut.String())
	}
}
