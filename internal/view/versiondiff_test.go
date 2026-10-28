package view_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// An outside change over unsaved edits: "see the difference" says what differs in the
// window's words, and launching is refused in them too, not with archo commands (VM run B9).
func TestOutsideChangeDifferenceAndRefusal(t *testing.T) {
	e := exchangeEnv(t)
	e.MustRun("save")
	e.MustRun("enable", "pkg:tester.b") // unsaved edit
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "brrainz.harmony", "tester.a")
	e.MustRun("scan")
	st, err := e.App.Loadout()
	if err != nil || st.WS.Conflict == 0 {
		t.Fatalf("no outside change pending: %v", err)
	}
	v := "v" + itoa(st.WS.Conflict)
	view.SetLang("zh")
	defer view.SetLang("en")
	lines, err := view.VersionDiff(e.App, v, "workspace")
	text := strings.Join(lines, "\n")
	if err != nil || !strings.HasPrefix(lines[0], v+" → 你的列表：2 处不同") || !strings.Contains(text, "− Harmony（只在"+v+"里）") || !strings.Contains(text, "+ Beta（只在你的列表里") {
		t.Fatalf("difference %v:\n%s", err, text)
	}
	o := view.Run(e.App, view.ListSave)
	if len(o.Refused) != 2 || strings.Contains(strings.Join(o.Refused, " "), "archo ") || !strings.Contains(o.Refused[1], "能不能跑") {
		t.Fatalf("refusal %+v", o.Refused)
	}
	if _, err := e.App.Loadout(); err != nil {
		t.Fatal(err)
	}
}
