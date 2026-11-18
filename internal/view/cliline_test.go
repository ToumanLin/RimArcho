package view_test

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// The subscribe preview's dry-run lines read in Chinese (VM round 2026-11-05: they were
// English in the Chinese interface). Not parallel: it sets the interface language.
func TestCLILineZh(t *testing.T) {
	defer view.SetLang("en")
	view.SetLang("zh")
	for in, want := range map[string]string{
		"dry run, nothing changed: workshop: 1 subscriptions":                             "试运行，没有改动：工坊：1 个订阅",
		"result: would download with SteamCMD":                                            "结果：将用 SteamCMD 下载",
		"note: Would subscribe through Steam; use SteamCMD only if Steam is unavailable.": "备注：将通过 Steam 订阅；只在 Steam 不可用时用 SteamCMD。",
		"Workshop 1578693166": "Workshop 1578693166",
		// the Mod library's remove dialog (VM round 2026-11-10: its lines were English)
		`affected link: C:\G: C:\G\Mods\_Archo_x -> C:\t; detach link`: "\u53d7\u5f71\u54cd\u7684\u94fe\u63a5\uff1a" + `C:\G: C:\G\Mods\_Archo_x -> C:\t` + "\uff0c\u65ad\u5f00\u94fe\u63a5",
		"dry run, nothing changed: remove: 1 subscription":             "试运行，没有改动：移除：1 个订阅",
	} {
		if got := view.CLILine(nil, in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	view.SetLang("en")
	if got := view.CLILine(nil, "result: would download with SteamCMD"); got != "result: would download with SteamCMD" {
		t.Errorf("English changed: %q", got)
	}
}
