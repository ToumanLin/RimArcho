package view

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// VM run B11: the window says the Steam client is offline and why, in its language.
func TestSteamOfflineWords(t *testing.T) {
	SetLang("zh")
	defer SetLang("en")
	if got := Humanize(nil, "note: "+fmt.Sprintf(app.SteamSignOutNote, "player")); !strings.HasPrefix(got, "注意：SteamCMD 会用 player 登录") {
		t.Fatalf("%q", got)
	}
	for _, it := range []app.TaskItem{
		{Ref: "ws:1", FailureKind: "offline", Why: "the Steam client is offline"},
		{Ref: "ws:1", Why: "Steam request failed (code 3)"}, // tasks from before
	} {
		if got := subscriptionReason(it, true); !strings.HasPrefix(got, "Steam 客户端离线：重启 Steam") {
			t.Fatalf("%+v: %q", it, got)
		}
	}
	if got := subscriptionReason(app.TaskItem{Ref: "ws:1", Why: "Steam request timed out"}, false); got == T("download was not confirmed") {
		t.Fatalf("Steam's own reason hidden: %q", got)
	}
}
