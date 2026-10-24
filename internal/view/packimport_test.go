package view

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
)

// The import result says whether the files were checked and what differs from this
// game; info items stay on the pack's page (06 §2.7, 08 §5.5).
func TestPackImportTextShowsCheck(t *testing.T) {
	check := &app.PackCheck{PackGame: "1.6.4633 rev1261", Game: "1.6.4871 rev590", Items: []app.PackCheckItem{
		{Part: "game", Level: finding.Warning, Text: "built for 1.6.4633 rev1261, this game is 1.6.4871 rev590"},
		{Part: "dlc", Level: finding.Info, Text: "installed DLC not used by the pack: Royalty", Names: []string{"Royalty"}},
	}}
	got := PackImportText(&app.PackMatch{Pack: &app.Pack{Name: "p"}, Verified: true, Check: check})
	for _, want := range []string{T("every mod's files match the pack"), Tf("built for %s, this game is %s", "1.6.4633 rev1261", "1.6.4871 rev590")} {
		if !strings.Contains(got, want) {
			t.Errorf("import text %q lacks %q", got, want)
		}
	}
	if strings.Contains(got, "Royalty") {
		t.Errorf("info item shown in the import text: %q", got)
	}
	if old := PackImportText(&app.PackMatch{Pack: &app.Pack{Name: "p"}}); !strings.Contains(old, T("files not checked (a pack from an older RimArcho)")) {
		t.Errorf("format-1 import text %q does not say the files were not checked", old)
	}
}
