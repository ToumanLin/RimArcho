package view

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// The report's basic lines say how the list's last Mod check went (05 §3): none yet, then
// when it finished, how many operations and the slowest one, so a report about a check
// that stops tells a slow operation from one that never ends.
func TestFeedbackCheckLine(t *testing.T) {
	SetLang("en")
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Mod{PackageID: "a.b", Name: "AB"})
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "a.b")
	e.AddEnv(false)
	e.MustRun("scan")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	line := func() string {
		for _, l := range FeedbackAttachments(e.App, st, FeedbackOptions{}, nil).Basic {
			if strings.HasPrefix(l, "Mod check: ") {
				return l
			}
		}
		return ""
	}
	if l := line(); l != "Mod check: none finished yet" {
		t.Fatalf("before a check: %q", l)
	}
	if _, err := e.App.EnsureAnalysis(nil); err != nil {
		t.Fatal(err)
	}
	if st, err = e.App.Loadout(); err != nil {
		t.Fatal(err)
	}
	if l := line(); !strings.HasPrefix(l, "Mod check: finished ") || !strings.Contains(l, " operations in ") || strings.Contains(l, "changed since") {
		t.Fatalf("after a check: %q", l)
	}
}
