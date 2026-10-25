package view_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestSteamAccountSummaryHasOneStateAndReadableMetadata(t *testing.T) {
	e, _, _ := shellEnv(t)
	e.App.SteamCMDNoPath = true
	e.WriteFile(filepath.Join(e.App.LocalDir(), "steamcmd", "steamcmd.exe"), "offline fixture")
	item := view.AccountGroup(e.App).Items[0]
	text := item.Note + item.Value
	for _, want := range []string{"匿名", "来源：RimArcho 自带", "最后验证：从未", "到期：由 Steam 管理"} {
		if strings.Count(text, want) != 1 {
			t.Fatal(text)
		}
	}
	if strings.HasPrefix(item.Note, "；") || strings.HasSuffix(strings.Split(item.Note, "\n")[0], "；") || strings.Contains(text, "own") || strings.Contains(text, "未知") {
		t.Fatal(text)
	}
}
