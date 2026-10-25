package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func incomingFixture(t *testing.T) (*testkit.Env, string, string, []string) {
	t.Helper()
	e := libFixture(t)
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	id := refID(t, e, "Lockable", source.Local)
	target := filepath.Join(e.Game, "Mods", "Lockable Copy")
	var links []string
	for _, name := range []string{"IncomingOne", "IncomingTwo"} {
		game := secondGame(e, name)
		e.MustRun("env", "add", name, "--game", game)
		path := filepath.Join(game, "Mods", "_Archo_env_"+id)
		e.Junction(path, target)
		links = append(links, path)
	}
	return e, target, id, links
}

func TestIncomingLinksLockMove(t *testing.T) {
	t.Parallel()
	for _, cross := range []bool{false, true} {
		name := "rename"
		if cross {
			name = "cross-volume"
		}
		t.Run(name, func(t *testing.T) {
			e, from, id, links := incomingFixture(t)
			e.App.SameVolume = func(string, string) bool { return !cross }
			trashCalls := 0
			e.App.TrashFunc = func(path string) error {
				trashCalls++
				for _, l := range links {
					if source.IsLink(l) {
						t.Fatal("target recycled before incoming link detached")
					}
				}
				return os.RemoveAll(path)
			}
			plan, err := e.App.MoveToLocked("#"+id[:6], true)
			if err != nil || len(plan.Links.Links) != 3 {
				t.Fatalf("preview: %+v %v", plan, err)
			}
			rows, _ := e.App.Store.ManagedLinks()
			before := len(rows)
			for _, l := range links {
				if !source.IsLink(l) {
					t.Fatal("preview detached link")
				}
			}
			result, err := e.App.MoveToLocked("#"+id[:6], false)
			if err != nil {
				t.Fatal(err)
			}
			if result.Copied != cross || cross && trashCalls != 1 || !cross && trashCalls != 0 {
				t.Fatalf("copy/trash: %+v %d", result, trashCalls)
			}
			if _, err := os.Stat(from); !os.IsNotExist(err) {
				t.Fatal("original remains")
			}
			for _, path := range links {
				info, err := source.ReadLink(path)
				if err != nil || info.State != source.TargetPresent || mustReal(t, path) != mustReal(t, result.To) {
					t.Fatalf("link after move: %+v %v", info, err)
				}
				entries, _ := os.ReadDir(filepath.Dir(path))
				count := 0
				for _, entry := range entries {
					l, err := source.ReadLink(filepath.Join(filepath.Dir(path), entry.Name()))
					if err == nil && l.State == source.TargetPresent && (l.Kind == "junction" || l.Kind == "symlink") && mustReal(t, l.Target) == mustReal(t, result.To) {
						count++
					}
				}
				if count != 1 {
					t.Fatalf("game has %d entries for instance", count)
				}
			}
			rows, err = e.App.Store.ManagedLinks()
			if err != nil || len(rows) != before+3 {
				t.Fatalf("ledger %+v %v", rows, err)
			}
			for _, row := range rows[before:] {
				if row.Phase != "complete" || row.Instance != id {
					t.Fatalf("incomplete ledger: %+v", row)
				}
			}
			// A normal save must not add a locked alias beside a retained env alias.
			e.MustRun("env", "use", "IncomingOne")
			e.MustRun("swap", "ws:5100", "#"+id[:6])
			e.MustRun("save")
			incoming, err := e.App.IncomingLinks(result.To)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, l := range incoming {
				if mustReal(t, l.Mods) == mustReal(t, filepath.Dir(links[0])) {
					count++
				}
			}
			if count != 1 {
				t.Fatalf("save produced duplicate env/locked entries: %d", count)
			}
		})
	}
}

func TestIncomingTrashAndForeignRefusal(t *testing.T) {
	t.Parallel()
	e, target, id, links := incomingFixture(t)
	foreign := filepath.Join(filepath.Dir(links[1]), "PlayerManager")
	e.Junction(foreign, target)
	if _, err := e.App.MoveToLocked("#"+id[:6], true); err == nil || !strings.Contains(err.Error(), "foreign link") {
		t.Fatalf("foreign move: %v", err)
	}
	called := false
	e.App.TrashFunc = func(path string) error {
		called = true
		for _, l := range links {
			if source.IsLink(l) {
				t.Fatal("trash before detach")
			}
		}
		return os.RemoveAll(path)
	}
	if err := e.App.ToTrash(target); err == nil || called {
		t.Fatalf("foreign trash: %v called=%v", err, called)
	}
	if err := source.RemoveLink(foreign); err != nil {
		t.Fatal(err)
	}
	if err := e.App.ToTrash(target); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("trash not called")
	}
	for _, l := range links {
		if source.IsLink(l) {
			t.Fatal("trash left incoming link")
		}
	}
}

func TestIncomingRunningAndRollback(t *testing.T) {
	t.Parallel()
	e, target, id, links := incomingFixture(t)
	e.App.GameProbe = func(game string) *app.GameRun {
		if strings.Contains(game, "IncomingTwo") {
			return &app.GameRun{PID: 42}
		}
		return nil
	}
	if _, err := e.App.MoveToLocked("#"+id[:6], false); err == nil || !strings.Contains(err.Error(), "running") {
		t.Fatalf("running: %v", err)
	}
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	e.App.TrashFunc = func(string) error { return errors.New("synthetic recycle failure") }
	if err := e.App.ToTrash(target); err == nil {
		t.Fatal("trash failure lost")
	}
	for _, l := range links {
		if !source.IsLink(l) || mustReal(t, l) != mustReal(t, target) {
			t.Fatal("original surviving target was not reattached")
		}
	}
	rows, _ := e.App.Store.ManagedLinks()
	for _, r := range rows {
		if !store.LinkFinished(r.Phase) {
			t.Fatalf("successful rollback left recovery: %+v", r)
		}
	}
}

func TestIncomingPackRemoveAcrossGames(t *testing.T) {
	t.Parallel()
	e := packEnv(t)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("pack", "build", "incoming-pack")
	pack, err := e.App.ReadPack("incoming-pack")
	if err != nil {
		t.Fatal(err)
	}
	var folder string
	for _, entry := range pack.Entries {
		if !entry.Official && entry.Folder != "" {
			folder = entry.Folder
			break
		}
	}
	if folder == "" {
		t.Fatal("fixture pack has no mod")
	}
	target := filepath.Join(e.App.PacksDir(), "incoming-pack", "mods", folder)
	var links []string
	for _, game := range []string{e.Game, secondGame(e, "PackSecond")} {
		if game != e.Game {
			e.MustRun("env", "add", "pack-second", "--game", game)
		}
		link := filepath.Join(game, "Mods", "!ArchoPack_incoming-pack_"+folder)
		e.Junction(link, target)
		links = append(links, link)
	}
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	e.App.TrashFunc = func(path string) error {
		for _, l := range links {
			if source.IsLink(l) {
				t.Fatal("pack target recycled before detach")
			}
		}
		return os.RemoveAll(path)
	}
	if _, err := e.App.RemovePack("incoming-pack", true); err != nil {
		t.Fatal(err)
	}
	for _, l := range links {
		if !source.IsLink(l) {
			t.Fatal("preview changed link")
		}
	}
	if _, err := e.App.RemovePack("incoming-pack", false); err != nil {
		t.Fatal(err)
	}
	for _, l := range links {
		if source.IsLink(l) {
			t.Fatal("pack removal left cross-game link")
		}
	}
}

func TestLinkLedgerRecoveryConfirmedOnly(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"intent", "detached", "records-updated"} {
		t.Run(phase, func(t *testing.T) {
			e, target, id := linkHealthEnv(t)
			path := filepath.Join(e.Game, "Mods", "_Archo_env_"+id)
			e.Junction(path, target)
			next := target
			if phase == "records-updated" {
				next = filepath.Join(e.Root, "recovered-target")
				e.MkdirAll(next)
			}
			row := store.ManagedLink{Operation: "interrupted", Mods: mustReal(t, filepath.Dir(path)), Name: filepath.Base(path), Kind: "junction", Instance: id, Target: target, NextPath: path, NextTarget: next}
			// Platform-independent tests use the actual kind, real junctions on Windows.
			info, _ := source.ReadLink(path)
			row.Kind = info.Kind
			if err := e.App.Store.RecordLinkIntent([]store.ManagedLink{row}); err != nil {
				t.Fatal(err)
			}
			if err := e.App.Store.LinkPhase(row.Operation, phase); err != nil {
				t.Fatal(err)
			}
			if phase != "intent" {
				if err := source.RemoveLink(path); err != nil {
					t.Fatal(err)
				}
			}
			if err := e.App.Close(); err != nil {
				t.Fatal(err)
			}
			e.Open()
			if len(e.App.StartupLinkRecovery) != 1 {
				t.Fatalf("startup recovery: %+v", e.App.StartupLinkRecovery)
			}
			ps, err := e.App.Problems(false, nil)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, p := range ps.List {
				if p.Code == app.CodeLinkRecovery {
					found = true
				}
			}
			if !found {
				t.Fatal("no recovery problem")
			}
			out, _ := e.Run("links", "repair", "--all", "--dry-run")
			if !strings.Contains(out, app.CodeLinkRecovery) {
				t.Fatal(out)
			}
			if phase != "intent" && source.IsLink(path) {
				t.Fatal("startup or dry run silently recovered")
			}
			e.MustRun("links", "repair", "--all", "--yes")
			want := target
			if phase == "records-updated" {
				want = next
			}
			if mustReal(t, path) != mustReal(t, want) {
				t.Fatal("incorrect recovery target")
			}
			all, err := e.App.InspectGameLinks()
			if err != nil {
				t.Fatal(err)
			}
			for _, l := range all {
				if l.Code == app.CodeLinkRecovery {
					t.Fatal("recovery remains")
				}
			}
		})
	}
}

func TestLinksCheckOmitsRemoved(t *testing.T) {
	t.Parallel()
	e, _, _ := linkHealthEnv(t)
	out := e.MustRun("links", "check")
	if strings.Contains(out, "removed:") {
		t.Fatal(out)
	}
	out = e.MustRun("links", "repair", "--dry-run")
	if !strings.Contains(out, "removed: 0") {
		t.Fatal(out)
	}
}

// A worker can stop after removing just one item. Recover each journal row in
// its own direction while requiring confirmation of the whole batch.
func TestLinkRecoveryMixedBatch(t *testing.T) {
	t.Parallel()
	e, target, id := linkHealthEnv(t)
	mods := filepath.Join(e.Game, "Mods")
	removed := filepath.Join(mods, "_Archo_env_"+id)
	survivor := filepath.Join(mods, "_Archo_env_survivor")
	survivingTarget := filepath.Join(e.Root, "surviving-target")
	e.MkdirAll(survivingTarget)
	var rows []store.ManagedLink
	for _, pair := range [][2]string{{removed, target}, {survivor, survivingTarget}} {
		e.Junction(pair[0], pair[1])
		info, err := source.ReadLink(pair[0])
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, store.ManagedLink{Operation: "mixed-batch", Mods: mustReal(t, mods), Name: filepath.Base(pair[0]), Kind: info.Kind, Target: pair[1], From: pair[1]})
	}
	if err := e.App.Store.RecordLinkIntent(rows); err != nil {
		t.Fatal(err)
	}
	if err := source.RemoveLink(removed); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(target); err != nil {
		t.Fatal(err)
	}
	e.MustRun("links", "repair", "--all", "--dry-run")
	if !source.IsLink(survivor) || source.IsLink(removed) {
		t.Fatal("preview changed the batch")
	}
	e.MustRun("links", "repair", "--all", "--yes")
	rows, err := e.App.Store.ManagedLinks()
	if err != nil || len(rows) != 2 || rows[0].Phase != "complete" || rows[1].Phase != "rolled-back" {
		t.Fatalf("mixed recovery: %+v %v", rows, err)
	}
	if !source.IsLink(survivor) || mustReal(t, survivor) != mustReal(t, survivingTarget) || source.IsLink(removed) {
		t.Fatal("mixed recovery changed surviving entry or recreated a dangling link")
	}
}

func TestLinkRecoveryBetweenMoveAndRecords(t *testing.T) {
	t.Parallel()
	e, from, id, links := incomingFixture(t)
	to := filepath.Join(e.App.LockDir(), "Lockable Copy")
	e.MkdirAll(filepath.Dir(to))
	if err := e.App.Store.InsertSource(&source.Source{ID: "recovery-lock", Kind: source.Extra, Label: app.LockLabel, Root: filepath.Dir(to), Status: source.StatusOK}); err != nil {
		t.Fatal(err)
	}
	var rows []store.ManagedLink
	for _, path := range links {
		info, err := source.ReadLink(path)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, store.ManagedLink{Operation: "move-interrupted", Mods: mustReal(t, filepath.Dir(path)), Name: filepath.Base(path), Kind: info.Kind, Instance: id, Target: info.Target, NextPath: path, NextTarget: to, From: mustReal(t, from), To: to})
	}
	if err := e.App.Store.RecordLinkIntent(rows); err != nil {
		t.Fatal(err)
	}
	for _, path := range links {
		if err := source.RemoveLink(path); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.App.Store.LinkPhase("move-interrupted", "detached"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
	if err := e.App.Close(); err != nil {
		t.Fatal(err)
	}
	e.Open()
	m, err := e.App.Store.Mod(id)
	if err != nil {
		t.Fatal(err)
	}
	if source.Within(m.RealPath, to) {
		t.Fatal("startup silently updated instance")
	}
	e.MustRun("links", "repair", "--all", "--dry-run")
	for _, path := range links {
		if source.IsLink(path) {
			t.Fatal("preview reattached link")
		}
	}
	e.MustRun("links", "repair", "--all", "--yes")
	m, err = e.App.Store.Mod(id)
	if err != nil || mustReal(t, m.RealPath) != mustReal(t, to) || m.SourceID != "recovery-lock" {
		t.Fatalf("records not recovered: %+v %v", m, err)
	}
	for _, path := range links {
		if mustReal(t, path) != mustReal(t, to) {
			t.Fatal("moved target not recovered")
		}
	}
}

func TestWorkshopUnsubscribeIncomingLinks(t *testing.T) {
	t.Parallel()
	for _, partial := range []bool{false, true} {
		name := "success"
		if partial {
			name = "partial-failure"
		}
		t.Run(name, func(t *testing.T) { testWorkshopUnsubscribeIncomingLinks(t, partial) })
	}
}

func testWorkshopUnsubscribeIncomingLinks(t *testing.T, partial bool) {
	e := libFixture(t)
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	e.AddWorkshop("5200", testkit.Sample("tester.second.unsubscribe", "Second Unsubscribe"))
	scanNow(t, e)
	id := refID(t, e, "Lockable", source.Workshop)
	secondID := refID(t, e, "Second Unsubscribe", source.Workshop)
	game := secondGame(e, "UnsubscribeIncoming")
	e.MustRun("env", "add", "unsubscribe-incoming", "--game", game)
	path := filepath.Join(game, "Mods", "_Archo_env_"+id)
	target := filepath.Join(e.Workshop, "5100")
	e.Junction(path, target)
	secondPath := filepath.Join(game, "Mods", "_Archo_env_"+secondID)
	secondTarget := filepath.Join(e.Workshop, "5200")
	e.Junction(secondPath, secondTarget)
	refs := []string{"#" + id[:6], "#" + secondID[:6]}
	steamDLL(e)
	var calls [][]string
	e.App.SteamWorker = func(dir string, args []string) (string, error) {
		if source.IsLink(path) || source.IsLink(secondPath) {
			t.Fatal("Steam called before detach")
		}
		if err := os.RemoveAll(target); err != nil {
			t.Fatal(err)
		}
		if partial {
			calls = append(calls, append([]string{dir}, args...))
			return steamworks.ItemLine(5100, "ok", 0) + "\n" + steamworks.ItemLine(5200, "failed:15", 0) + "\n" + steamworks.DoneLine(args[0], 2), nil
		}
		if err := os.RemoveAll(secondTarget); err != nil {
			t.Fatal(err)
		}
		return fakeSteamWorker(&calls)(dir, args)
	}
	preview, err := e.App.Workshop(steamworks.Unsubscribe, refs, true)
	if err != nil || len(preview.Items) != 2 || len(preview.Items[0].Links.Links) != 1 || len(preview.Items[1].Links.Links) != 1 {
		t.Fatalf("unsubscribe preview %+v %v", preview, err)
	}
	if !source.IsLink(path) || !source.IsLink(secondPath) {
		t.Fatal("unsubscribe preview changed link")
	}
	_, err = e.App.Workshop(steamworks.Unsubscribe, refs, false)
	if partial {
		if err == nil || !strings.Contains(err.Error(), "failed:15") {
			t.Fatalf("partial failure: %v", err)
		}
		info, err := source.ReadLink(secondPath)
		if err != nil || info.State != source.TargetPresent {
			t.Fatalf("surviving target not reattached: %+v %v", info, err)
		}
		recovery, err := e.App.PlanLinkRepair(true, nil)
		if err != nil {
			t.Fatal(err)
		}
		pending := 0
		for _, l := range recovery.Links {
			if l.Code == app.CodeLinkRecovery {
				pending++
			}
		}
		if pending != 1 {
			t.Fatalf("pending recoveries: %d", pending)
		}
		if err := e.App.ApplyLinkRepair(recovery); err != nil {
			t.Fatal(err)
		}
		rows, err := e.App.Store.ManagedLinks()
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if !store.LinkFinished(row.Phase) || row.Instance == secondID && row.Phase != "rolled-back" {
				t.Fatalf("recovery changed a restored row: %+v", row)
			}
		}
	} else if err != nil {
		t.Fatal(err)
	} else if source.IsLink(secondPath) {
		t.Fatal("unsubscribe left second incoming link")
	}
	if source.IsLink(path) || len(calls) != 1 {
		t.Fatal("unsubscribe left link or skipped worker")
	}
}

func TestIncomingStaleIDLinkMove(t *testing.T) {
	t.Parallel()
	e, from, id, links := incomingFixture(t)
	old := links[0]
	if err := source.RemoveLink(old); err != nil {
		t.Fatal(err)
	}
	adopted := filepath.Join(mustReal(t, filepath.Dir(old)), "_Archo_env_deleted-instance")
	e.Junction(adopted, from)
	incoming, err := e.App.IncomingInstanceLinks(id)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range incoming {
		if l.Path == adopted {
			found = l.Owned && l.Instance == id
		}
	}
	if !found {
		t.Fatal("adopted link missing from incoming links")
	}
	moved, err := e.App.MoveToLocked("#"+id[:6], false)
	if err != nil {
		t.Fatal(err)
	}
	info, err := source.ReadLink(adopted)
	if err != nil || info.State != source.TargetPresent || mustReal(t, adopted) != mustReal(t, moved.To) {
		t.Fatalf("adopted link after move: %+v %v", info, err)
	}
	if _, err := os.Lstat(old); !os.IsNotExist(err) {
		t.Fatal("move renamed the adopted link")
	}
	l := healthByName(t, e)[filepath.Base(adopted)]
	if !l.Owned || l.Code != "" || l.Instance != id {
		t.Fatalf("moved link health: %+v", l)
	}
}
