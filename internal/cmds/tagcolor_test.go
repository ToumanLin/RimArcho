package cmds_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// tag color and tag order (16 §2.9 Tag manager): a colour is a preset name or #rrggbb and
// follows a rename; a Mod's tags keep the order the player sets.
func TestTagColorAndOrder(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("6100", testkit.Sample("tester.a", "Alpha"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("tag", "add", "pkg:tester.a", "Medieval", "framework", "Gravship")

	if out := e.MustRun("tag", "color", "gravship", "#3AF"); !strings.Contains(out, "colour set for tag Gravship") || !strings.Contains(out, "#33aaff") {
		t.Fatalf("tag color:\n%s", out)
	}
	if _, code := e.Run("tag", "color", "Gravship", "red"); code == 0 {
		t.Fatal("red is a problem colour, not a tag colour")
	}
	if _, code := e.Run("tag", "color", "nosuch", "blue"); code == 0 {
		t.Fatal("a colour for a tag that does not exist")
	}
	e.MustRun("tag", "color", "Medieval", "orange")
	if out := e.MustRun("tag", "list"); !strings.Contains(out, "colour: orange") || !strings.Contains(out, "colour: #33aaff") {
		t.Fatalf("tag list:\n%s", out)
	}

	if out := e.MustRun("tag", "order", "pkg:tester.a", "Gravship"); !strings.Contains(out, "tags: Gravship, Medieval, framework") {
		t.Fatalf("tag order:\n%s", out)
	}
	if _, code := e.Run("tag", "order", "pkg:tester.a", "visuals"); code == 0 {
		t.Fatal("ordering a tag the mod does not carry")
	}

	e.MustRun("tag", "rename", "Gravship", "Space")
	list, err := cmds.TagList(e.App)
	if err != nil {
		t.Fatal(err)
	}
	colors := map[string]string{}
	for _, x := range list {
		colors[x.Tag] = x.Color
	}
	if colors["Space"] != "#33aaff" || colors["Medieval"] != "orange" {
		t.Fatalf("colours after the rename: %v", colors)
	}
	e.MustRun("tag", "color", "Space", "--clear")
	if list, _ := cmds.TagList(e.App); list[0].Color != "" && list[0].Tag == "Space" {
		t.Fatalf("cleared colour kept: %+v", list[0])
	}
}
