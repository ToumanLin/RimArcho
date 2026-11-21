package app_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// 03 §2 item 3 (B2): a game downloaded with SteamCMD gets the account's subscriptions in its
// own Workshop folder. While the Steam client's Workshop folder is registered, that folder
// is not registered, so no Mod gets a second copy; one registered before (an earlier
// version, or before the client's folder was known) is taken back on the next scan, and
// plans that used its copies use the client's. A folder the player adds stays.
func TestGameWorkshopMirrorNoDuplicates(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	shared := testkit.Sample("tester.shared", "Shared")
	other := testkit.Sample("tester.other", "Other")
	e.AddWorkshop("5100", shared)
	e.AddWorkshop("5101", other)
	e.AddEnv(false)
	scanNow(t, e)

	// A game whose folder holds SteamCMD's copies of the same items, plus one the client
	// no longer has (unsubscribed since).
	mirror := func(game string) string {
		own := filepath.Join(game, "steamapps", "workshop", "content", "294100")
		for id, m := range map[string]testkit.Mod{"5100": shared, "5101": other} {
			m.Workshop = id
			e.WriteMod(filepath.Join(own, id), m)
		}
		stale := testkit.Sample("tester.stale", "Stale")
		stale.Workshop = "5199"
		e.WriteMod(filepath.Join(own, "5199"), stale)
		return own
	}
	live := func(root string) *source.Source {
		for _, s := range sourcesOf(t, e) {
			if s.Root == root && s.Status != source.StatusRetired {
				return s
			}
		}
		return nil
	}
	copies := func(name string) int {
		n := 0
		for _, m := range named(t, e, name) {
			if m.TombstoneAt.IsZero() {
				n++
			}
		}
		return n
	}

	// Adding the downloaded game (what "download a version" does) registers nothing.
	g17 := secondGame(e, "Game17")
	own17 := mirror(g17)
	if _, err := e.App.AddDownloaded(app.DownloadSpec{Name: "1.7"}, g17); err != nil {
		t.Fatal(err)
	}
	scanNow(t, e)
	if s := live(own17); s != nil {
		t.Fatalf("the downloaded game's Workshop folder is registered: %+v", s)
	}
	if n := copies("Shared"); n != 1 {
		t.Fatalf("Shared has %d copies, want 1", n)
	}
	if out := e.MustRun("problems", "--all"); strings.Contains(out, "pkg.duplicate-installed") {
		t.Fatalf("duplicates after a download:\n%s", out)
	}

	// Registered while the client's folder was not known (as earlier versions did on every
	// download): the plan enables its copy; once the client's folder is back, the next
	// scan takes the folder back and the plan uses the client's copy.
	e.MustRun("env", "use", "main")
	wsAway := filepath.Join(e.Root, "WorkshopAway")
	e.Move(e.Workshop, wsAway)
	g16 := secondGame(e, "Game16")
	own16 := mirror(g16)
	e.MustRun("env", "add", "old", "--game", g16)
	e.MustRun("env", "use", "main")
	scanNow(t, e)
	if live(own16) == nil {
		t.Fatalf("without the client's Workshop folder the game's own is registered: %v", sourcesOf(t, e))
	}
	cmdRef := refOf(t, e, "Shared", source.SteamCMD)
	e.MustRun("enable", cmdRef)
	e.Move(wsAway, e.Workshop)
	scanNow(t, e)
	if s := live(own16); s != nil {
		t.Fatalf("the copies' folder stays registered next to the client's: %+v", s)
	}
	if n := copies("Shared"); n != 1 {
		t.Fatalf("Shared has %d copies after the scan, want 1", n)
	}
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	ws := refID(t, e, "Shared", source.Workshop)
	if st.List.Index(ws) < 0 {
		t.Fatalf("the plan uses the client's copy: %+v", st.List)
	}
	if out := e.MustRun("problems", "--all"); strings.Contains(out, "pkg.duplicate-installed") {
		t.Fatalf("duplicates after the scan:\n%s", out)
	}

	// The player adds it on purpose: it stays.
	e.MustRun("folder", "add", "steamcmd", own16)
	scanNow(t, e)
	if live(own16) == nil {
		t.Fatal("a folder the player added is taken back")
	}
}
