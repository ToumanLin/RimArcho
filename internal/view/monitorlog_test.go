package view_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// realRoot is the save data root the fixtures were written under (sanitized, 03 §6.7.3).
const realRoot = `C:\Users\p\AppData\LocalLow\Ludeon Studios\RimWorld by Ludeon Studios`

// realRun puts the 1.7 run of 2026-10-21 (Archo Monitor 0.9.0-dev: 12 patch errors, one
// assembly conflict, 4 warnings) into a 1.7 environment whose mods carry the packageIds
// the logs name. The Player.log's marker names the copied ArchoMonitor.log.
func realRun(t *testing.T) (*testkit.Env, string, string) {
	t.Helper()
	view.SetLang("en")
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.Game, "Version.txt"), "1.7.4985 rev1269\n")
	e.AddWorkshop("2009463077", testkit.Mod{PackageID: "brrainz.harmony", Name: "Harmony"})
	for _, m := range [][2]string{
		{"oskarpotocki.vanillafactionsexpanded.core", "Vanilla Expanded Framework"},
		{"rimcrow.companion", "RimCrow Companion"},
		{"co.uk.epicguru.meleeanimation", "Melee Animation"},
	} {
		e.WriteMod(filepath.Join(e.Game, "Mods", strings.ReplaceAll(m[1], " ", "")), testkit.Sample(m[0], m[1]))
	}
	e.WriteModsConfig("1.7.4985 rev1269", "ludeon.rimworld", "brrainz.harmony", "oskarpotocki.vanillafactionsexpanded.core", "rimcrow.companion", "co.uk.epicguru.meleeanimation")
	e.AddEnv(false)
	e.MustRun("scan")
	fix := filepath.Join("..", "..", "testdata", "companion", "real17")
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(fix, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	esc := func(s string) string { return strings.ReplaceAll(s, `\`, `\\`) }
	player := filepath.Join(e.UserData, "Logs", "Player.log")
	monitor := filepath.Join(e.UserData, "ArchoMonitor.log")
	e.WriteFile(player, strings.ReplaceAll(read(filepath.Join("Logs", "Player.log")), realRoot, e.UserData))
	e.WriteFile(monitor, strings.ReplaceAll(read("ArchoMonitor.log"), esc(realRoot), esc(e.UserData)))
	return e, player, monitor
}

// 06 §2.10: the log page shows the run's ArchoMonitor.log as its own section and lists it
// in the file picker under its Player.log.
func TestLogPageMonitor(t *testing.T) {
	e, player, monitor := realRun(t)
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	files := view.LogFiles(e.App, st)
	if len(files) < 2 || files[0].Path != player || files[1].Label != "ArchoMonitor.log" || files[1].Of != player || files[1].Path != monitor {
		t.Fatalf("files %+v", files)
	}
	// Picking the companion log shows its Player.log.
	p, err := view.BuildLogPage(e.App, st, monitor, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Path != player || p.Monitor == nil {
		t.Fatalf("page %s, monitor %v", p.Path, p.Monitor)
	}
	m := p.Monitor
	clock := func(s string) string {
		at, _ := time.ParseInLocation("2006-01-02 15:04:05.000", s, time.Local)
		return at.In(e.App.Loc).Format("15:04")
	}
	if m.Title != "Archo Monitor" || m.Label != "ArchoMonitor.log" ||
		m.Facts != clock("2026-10-21 17:51:43.164")+"–"+clock("2026-10-21 17:55:55.418") {
		t.Fatalf("head %q / %q", m.Title, m.Facts)
	}
	if m.Errors+m.Warnings != len(m.Entries) || m.Warnings != 4 || m.Errors < 10 {
		t.Fatalf("%d errors, %d warnings, %d rows", m.Errors, m.Warnings, len(m.Entries))
	}
	first := m.Entries[0]
	if first.Kind != "assembly conflict" || first.Status != view.StatusErr || strings.Join(first.Mods, ",") != "RimCrow Companion" ||
		!strings.HasPrefix(first.Text, "Error while instantiating a mod of type RimCrowCompanion") || strings.Contains(first.Text, "\n") ||
		!strings.Contains(first.Details, "RimCrowCompanionMod..ctor") || first.Time == "" {
		t.Fatalf("first row %+v", first)
	}
	rows := map[string]bool{}
	for _, r := range p.Rows {
		rows[r.Cluster] = true
	}
	patch, linked, repeat := 0, 0, false
	for _, r := range m.Entries {
		if r.Kind == "patch error" {
			patch++
			if strings.Join(r.Mods, ",") != "Vanilla Expanded Framework" {
				t.Fatalf("patch error mods %+v", r)
			}
		}
		if r.Cluster != "" {
			if !rows[r.Cluster] {
				t.Fatalf("row links to %s, not a cluster of the page", r.Cluster)
			}
			linked++
		}
		repeat = repeat || r.Count >= 10
		if strings.Contains(r.Text, "[MeleeAnim]") && strings.Contains(r.Text, "DrawEquip") && strings.Join(r.Mods, ",") != "Melee Animation,palpha.godhands,costel.layeredapparel" {
			t.Fatalf("names, then the ids of mods not installed: %v", r.Mods)
		}
	}
	if patch < 10 || linked == 0 || !repeat {
		t.Fatalf("%d patch errors, %d linked, a repeated entry %v", patch, linked, repeat)
	}
	// The run had the monitor: no line saying it is missing, whatever the plan has now.
	if p.NoMonitor != "" || p.AddMonitor {
		t.Fatalf("no monitor: %q %v", p.NoMonitor, p.AddMonitor)
	}
	view.SetLang("zh")
	defer view.SetLang("en")
	if view.MonitorKind("PatchError") != "补丁错误" || view.MonitorKind("AssemblyConflict") != "程序集冲突" || view.MonitorKind("Other") != "" {
		t.Fatal("zh kinds")
	}
}

// 06 §2.10: while the game runs, the section comes from the live tracker's companion log.
func TestLiveMonitor(t *testing.T) {
	t.Parallel()
	e, player, _ := realRun(t)
	e.App.GameProbe = func(root string) *app.GameRun {
		if root == e.Game {
			return &app.GameRun{PID: 7, Started: e.Now()}
		}
		return nil
	}
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	var tr view.LiveTracker
	for i := 0; i < 3; i++ { // the marker switches to ArchoMonitor.log; the next poll reads it
		if _, err := tr.Update(e.App, st); err != nil {
			t.Fatal(err)
		}
	}
	m := view.LiveMonitor(e.App, st, &tr, player, nil)
	if m == nil || !m.Running || len(m.Entries) == 0 || !strings.Contains(m.Facts, "since") && !strings.Contains(m.Facts, "–") {
		t.Fatalf("live %+v", m)
	}
	if view.LiveMonitor(e.App, st, &tr, filepath.Join(e.UserData, "Logs", "Player-prev.log"), nil) != nil {
		t.Fatal("a section for a log the game does not write")
	}
	// The files rotated at the same paths, but the tracker has not polled the new session.
	monitor := tr.Companion(player).Path
	e.WriteFile(player, "[Archo Monitor] 0.9.0 session abcdefabcdef: "+monitor+"\n")
	e.WriteFile(monitor, `{"kind":"session","message":"session","context":{"session":"abcdefabcdef"},"source":"archo-monitor"}`+"\n"+
		`{"message":"new session error","level":"ERROR","source":"archo-monitor","count":1}`+"\n")
	m = view.LiveMonitor(e.App, st, &tr, player, nil)
	if m == nil || m.Title != "Archo Monitor" || len(m.Entries) != 1 || m.Entries[0].Text != "new session error" {
		t.Fatalf("rotated monitor: %+v", m)
	}
}

// A run without the monitor's log, and no monitor installed: one line says so, no action.
func TestLogPageNoMonitor(t *testing.T) {
	t.Parallel()
	e, player, monitor := realRun(t)
	if err := os.Remove(monitor); err != nil {
		t.Fatal(err)
	}
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	p, err := view.BuildLogPage(e.App, st, player, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Monitor != nil || !strings.Contains(p.NoMonitor, "not installed") || p.AddMonitor {
		t.Fatalf("monitor %v, note %q %v", p.Monitor != nil, p.NoMonitor, p.AddMonitor)
	}
}

// A new monitor must replace the tracker's old RimCrow section even before its next poll.
func TestLogPagePrefersNewMonitor(t *testing.T) {
	t.Parallel()
	e, player, monitor := realRun(t)
	content, err := os.ReadFile(monitor)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(monitor); err != nil {
		t.Fatal(err)
	}
	crow := filepath.Join(e.UserData, "RimCrow_Realtime.log")
	e.WriteFile(crow, `{"message":"ready","level":"INFO"}`+"\n")
	fi, err := os.Stat(player)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(crow, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	e.App.GameProbe = func(root string) *app.GameRun {
		if root == e.Game {
			return &app.GameRun{PID: 7, Started: e.Now()}
		}
		return nil
	}
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	var tr view.LiveTracker
	if _, err := tr.Update(e.App, st); err != nil {
		t.Fatal(err)
	}
	if c := tr.Companion(player); c == nil || c.Path != crow {
		t.Fatalf("initial companion: %+v", c)
	}
	files := view.LogFiles(e.App, st)
	if len(files) < 2 || files[1].Path != crow {
		t.Fatalf("initial files: %+v", files)
	}
	if err := os.WriteFile(monitor, content, 0o644); err != nil {
		t.Fatal(err)
	}
	files = view.LogFiles(e.App, st)
	if len(files) < 2 || files[1].Path != monitor {
		t.Fatalf("monitor appeared: %+v", files)
	}
	m := view.LiveMonitor(e.App, st, &tr, player, nil)
	if m == nil || m.Path != monitor || m.Title != "Archo Monitor" {
		t.Fatalf("live monitor: %+v", m)
	}
	p, err := view.BuildLogPage(e.App, st, player, &tr)
	if err != nil {
		t.Fatal(err)
	}
	if p.Monitor == nil || p.Monitor.Path != monitor {
		t.Fatalf("page monitor: %+v", p.Monitor)
	}
}

func TestLogPageRejectsStaleCrow(t *testing.T) {
	t.Parallel()
	e, player, monitor := realRun(t)
	prev := filepath.Join(filepath.Dir(player), "Player-prev.log")
	if err := os.Rename(player, prev); err != nil {
		t.Fatal(err)
	}
	monitorPrev := filepath.Join(e.UserData, "ArchoMonitor-prev.log")
	if err := os.Rename(monitor, monitorPrev); err != nil {
		t.Fatal(err)
	}
	old := time.Date(2026, 10, 21, 17, 55, 0, 0, time.Local)
	for _, p := range []string{prev, monitorPrev} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	crow := filepath.Join(e.UserData, "RimCrow_Realtime.log")
	e.WriteFile(crow, "")
	if err := os.Chtimes(crow, old, old); err != nil {
		t.Fatal(err)
	}
	e.WriteFile(player, "=== Waylith log ===\nStarted:   2026-10-24 03:30:00.000 -07:00\nVersion:   1.7.4985.1269\n")
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	files := view.LogFiles(e.App, st)
	if len(files) != 3 || files[0].Path != player || files[1].Path != prev || files[2].Path != monitorPrev || files[2].Of != prev {
		t.Fatalf("files: %+v", files)
	}
	p, err := view.BuildLogPage(e.App, st, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Path != player || p.Monitor != nil {
		t.Fatalf("current path %q monitor %+v", p.Path, p.Monitor)
	}
}
