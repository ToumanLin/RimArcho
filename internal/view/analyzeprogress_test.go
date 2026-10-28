package view

import "testing"

func TestAnalyzeProgressInChinese(t *testing.T) {
	SetLang("zh")
	defer SetLang("en")
	for in, want := range map[string]string{
		"defs: 1000/1559 files":                           "Def 文件 1000/1559",
		"defs: 1559 files, 9000 defs from 12 active mods": "12 个启用的 Mod：1559 个文件，9000 个 Def",
		"done: 0 failed operations, 1 findings, 421ms":    "完成：0 个操作失败，1 个发现，用时 421ms",
		"patches: 3/40 operations":                        "Patch 操作 3/40",
		"something new":                                   "something new",
	} {
		if got := AnalyzeProgress(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	SetLang("en")
	if got := AnalyzeProgress("defs: 1000/1559 files"); got != "defs: 1000/1559 files" {
		t.Errorf("english changed: %q", got)
	}
}
