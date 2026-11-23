package cmds

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/savefile"
)

// A new colony as the game saves it: three colonists on the map, the five starting pawns
// the player did not choose kept as world pawns of the player's faction, and a research key
// for every project, most at 0.
func TestPlayCountsNewColony(t *testing.T) {
	f := &savefile.Facts{Player: savefile.Faction{Def: "PlayerColony"}}
	for i := 0; i < 3; i++ {
		f.Colonists = append(f.Colonists, savefile.Pawn{ID: "Human" + string(rune('a'+i)), Where: savefile.WhereMap})
	}
	for i := 0; i < 5; i++ {
		f.Colonists = append(f.Colonists, savefile.Pawn{ID: "Human" + string(rune('k'+i)), Where: savefile.WhereWorld})
	}
	f.Research.Progress = map[string]float64{"Electricity": 120, "Batteries": 0, "Smithing": 0}

	c := countsOf(f)
	if c.alive != 3 || c.world != 5 {
		t.Fatalf("alive %d, world %d; want 3 and 5", c.alive, c.world)
	}
	if got := peopleText(f, c); !strings.HasPrefix(got, "3, 5 world pawns;") {
		t.Errorf("colonists line %q", got)
	}

	doc, root := render.New("save")
	addPlay(root, &app.Save{Ref: "sv:test"}, f, nil, true, 0)
	if out := doc.String(); !strings.Contains(out, "nothing now; 1 project with progress") {
		t.Errorf("research line missing or wrong:\n%s", out)
	}
}
