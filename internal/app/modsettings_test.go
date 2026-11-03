package app_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// 08 §3: a Mod played from its Workshop folder and then from a SteamCMD copy (an _Archo_
// link) keeps its settings; going back never replaces the Workshop file.
func TestSettingsFollowDeployedCopy(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	e.WriteMod(filepath.Join(e.SteamCMD, "1001"), testkit.Sample("tester.a", "A"))
	scanNow(t, e)
	cfg := filepath.Join(e.UserData, "Config")
	e.WriteFile(filepath.Join(cfg, "Mod_1001_AMod.xml"), "<SettingsBlock>workshop</SettingsBlock>")
	carried := filepath.Join(cfg, "Mod__Archo_steamcmd_1001_AMod.xml")

	e.MustRun("swap", refOf(t, e, "A", source.Workshop), refOf(t, e, "A", source.SteamCMD))
	out := e.MustRun("save", "--dry-run")
	if !strings.Contains(out, "would copy 1 settings file") || !strings.Contains(out, "A: Mod_1001_AMod.xml → Mod__Archo_steamcmd_1001_AMod.xml") {
		t.Fatalf("dry run:\n%s", out)
	}
	if _, err := os.Stat(carried); !os.IsNotExist(err) {
		t.Fatal("the dry run copied")
	}
	if out := e.MustRun("save"); !strings.Contains(out, "copied 1 settings file") {
		t.Fatalf("save:\n%s", out)
	}
	if got := readFile(t, carried); got != "<SettingsBlock>workshop</SettingsBlock>" {
		t.Fatalf("carried %q", got)
	}

	// Played as the copy, its settings change; back on the Workshop folder, its own file stays.
	e.WriteFile(carried, "<SettingsBlock>copy</SettingsBlock>")
	e.MustRun("swap", refOf(t, e, "A", source.SteamCMD), refOf(t, e, "A", source.Workshop))
	if out := e.MustRun("save"); strings.Contains(out, "Mod settings") {
		t.Fatalf("copied over an existing file:\n%s", out)
	}
	if got := readFile(t, filepath.Join(cfg, "Mod_1001_AMod.xml")); got != "<SettingsBlock>workshop</SettingsBlock>" {
		t.Fatalf("the Workshop file changed: %q", got)
	}
	e.MustRun("env", "set", "main", "--launch", "direct")
	if out := e.MustRun("launch", "--dry-run"); strings.Contains(out, "Mod settings") {
		t.Fatalf("launch preview:\n%s", out)
	}
}

// 08 §5.3 step 4: a pack's links read their own names; the first play copies each Mod's
// settings to them, and HugsLib's; later plays keep the pack's own.
func TestPackSettingsFollowTheMod(t *testing.T) {
	t.Parallel()
	e := packEnv(t)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("enable", "pkg:tester.versions")
	e.MustRun("save")
	e.MustRun("pack", "build", "first")
	dir := filepath.Join(e.Home, "packs", "first")
	var p app.Pack
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "pack.json"))), &p); err != nil {
		t.Fatal(err)
	}
	folder := ""
	for _, pe := range p.Entries {
		if pe.PackageID == "tester.a" {
			folder = pe.Folder
		}
	}
	cfg := filepath.Join(e.UserData, "Config")
	e.WriteFile(filepath.Join(cfg, "Mod_1001_AMod.xml"), "<SettingsBlock>env</SettingsBlock>")
	e.WriteFile(filepath.Join(cfg, "Mod_Versions_VMod.xml"), "<SettingsBlock>versions</SettingsBlock>")
	e.WriteFile(filepath.Join(e.UserData, "HugsLib", "ModSettings.xml"), "<settings/>")
	to := app.SettingsFile("!ArchoPack_first_"+folder, "AMod")
	packCfg := filepath.Join(dir, "userdata", "Config")

	out := e.MustRun("pack", "launch", "first", "--dry-run")
	if !strings.Contains(out, "would copy 1 settings file") || !strings.Contains(out, "Mod_1001_AMod.xml → "+to) {
		t.Fatalf("dry run (%s):\n%s", to, out)
	}
	out = e.MustRun("pack", "launch", "first")
	if !strings.Contains(out, "copied 1 settings file") || !strings.Contains(out, "HugsLib/ModSettings.xml") {
		t.Fatalf("launch:\n%s", out)
	}
	if got := readFile(t, filepath.Join(packCfg, to)); got != "<SettingsBlock>env</SettingsBlock>" {
		t.Fatalf("pack copy %q", got)
	}
	// Versions is used from Mods/ (the same content): its file keeps its name.
	if got := readFile(t, filepath.Join(packCfg, "Mod_Versions_VMod.xml")); got != "<SettingsBlock>versions</SettingsBlock>" {
		t.Fatalf("reused copy's settings %q", got)
	}
	if got := readFile(t, filepath.Join(dir, "userdata", "HugsLib", "ModSettings.xml")); got != "<settings/>" {
		t.Fatalf("HugsLib %q", got)
	}

	e.WriteFile(filepath.Join(packCfg, to), "<SettingsBlock>pack</SettingsBlock>")
	e.WriteFile(filepath.Join(cfg, "Mod_1001_AMod.xml"), "<SettingsBlock>env later</SettingsBlock>")
	if out := e.MustRun("pack", "launch", "first"); strings.Contains(out, "Mod settings") || strings.Contains(out, "HugsLib") {
		t.Fatalf("second launch copied again:\n%s", out)
	}
	if got := readFile(t, filepath.Join(packCfg, to)); got != "<SettingsBlock>pack</SettingsBlock>" {
		t.Fatalf("the pack's settings were replaced: %q", got)
	}
}

// 03 §6.7.1, 08 §3: a folder only Archo Monitor saw (a copy since deleted) still leads to the
// Mod's settings, also after its log has rotated away.
func TestSettingsFromMonitorRecord(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.WriteFile(filepath.Join(e.UserData, "ArchoMonitor-prev.log"),
		`{"timestamp":"2026-10-01 10:00:00.000","level":"INFO","message":"[Archo Monitor] session 0123456789ab","details":"","count":1,"context":{"session":"0123456789ab","version":"0.10.0","game":"1.6.4633 rev1261"},"source":"archo-monitor","schema":1,"kind":"session"}`+"\n"+
			`{"timestamp":"2026-10-01 10:01:00.000","level":"INFO","message":"[Archo Monitor] settings 1","details":"","count":1,"context":{"settings":[{"packageId":"tester.a","folder":"A old copy","class":"AMod","file":"Mod_A old copy_AMod.xml","title":"A","stored":true}]},"source":"archo-monitor","schema":1,"kind":"settings"}`+"\n")
	cfg := filepath.Join(e.UserData, "Config")
	e.WriteFile(filepath.Join(cfg, "Mod_A old copy_AMod.xml"), "<SettingsBlock>old copy</SettingsBlock>")

	if out := e.MustRun("save"); !strings.Contains(out, "A: Mod_A old copy_AMod.xml → Mod_1001_AMod.xml") {
		t.Fatalf("save:\n%s", out)
	}
	seen := e.App.MonitorSettings(e.UserData)
	if len(seen) != 1 || seen[0].Folder != "A old copy" || seen[0].Title != "A" || seen[0].Game != "1.6.4633 rev1261" {
		t.Fatalf("kept record %+v", seen)
	}

	os.Remove(filepath.Join(e.UserData, "ArchoMonitor-prev.log"))
	os.Remove(filepath.Join(cfg, "Mod_1001_AMod.xml"))
	e.MustRun("disable", "pkg:tester.a")
	e.MustRun("save")
	e.MustRun("enable", "pkg:tester.a")
	if out := e.MustRun("save"); !strings.Contains(out, "Mod_A old copy_AMod.xml → Mod_1001_AMod.xml") {
		t.Fatalf("after the log rotated away:\n%s", out)
	}
	if got := readFile(t, filepath.Join(cfg, "Mod_1001_AMod.xml")); got != "<SettingsBlock>old copy</SettingsBlock>" {
		t.Fatalf("copy %q", got)
	}
}

// 06 §2.9 "Mod settings", 05 §4: the current list's Mods with settings (a file, or a window
// Archo Monitor saw), one Mod's stored values, and the same in archo mod.
func TestModSettingsCommand(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("enable", "pkg:tester.b")
	e.WriteFile(filepath.Join(e.UserData, "ArchoMonitor.log"),
		`{"timestamp":"2026-11-03 10:00:00.000","level":"INFO","message":"[Archo Monitor] session 0123456789ab","details":"","count":1,"context":{"session":"0123456789ab","version":"0.10.0","game":"1.6.4633 rev1261"},"source":"archo-monitor","schema":1,"kind":"session"}`+"\n"+
			`{"timestamp":"2026-11-03 10:01:00.000","level":"INFO","message":"[Archo Monitor] settings 2","details":"","count":1,"context":{"settings":[{"packageId":"tester.a","folder":"1001","class":"AMod","file":"Mod_1001_AMod.xml","title":"A settings","stored":true},{"packageId":"tester.b","folder":"B","class":"BMod","file":"Mod_B_BMod.xml","title":"B options","stored":true}]},"source":"archo-monitor","schema":1,"kind":"settings"}`+"\n")
	e.WriteFile(filepath.Join(e.UserData, "Config", "Mod_1001_AMod.xml"),
		`<SettingsBlock><ModSettings Class="A.Settings"><fast>True</fast><speed>2.5</speed><things><li>x</li></things></ModSettings></SettingsBlock>`)

	out := e.MustRun("mod-settings")
	if !strings.Contains(out, "mod settings: 2 Mods in plan:") || !strings.Contains(out, "A settings") || !strings.Contains(out, "Mod_1001_AMod.xml") ||
		!strings.Contains(out, "B options") || !strings.Contains(out, "defaults (no file yet)") || strings.Contains(out, "Harmony") {
		t.Fatalf("list:\n%s", out)
	}
	out = e.MustRun("mod-settings", "pkg:tester.a")
	for _, want := range []string{`"A settings"  Mod_1001_AMod.xml:`, "stored values (3;", "fast: True (on/off)", "speed: 2.5", "things: 1 item (list)", "read only: change them in the game"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if out := e.MustRun("mod-settings", "pkg:tester.b"); !strings.Contains(out, "the Mod uses its defaults") {
		t.Fatalf("B:\n%s", out)
	}
	if out := e.MustRun("mod", "pkg:tester.a"); !strings.Contains(out, "settings:") || !strings.Contains(out, "speed: 2.5") {
		t.Fatalf("archo mod:\n%s", out)
	}
	if out := e.MustRun("mod", "pkg:brrainz.harmony"); strings.Contains(out, "settings:") {
		t.Fatalf("Harmony has no settings:\n%s", out)
	}
}

// 03 §6.7.1, 06 §2.9: a settings window Archo Monitor saw shows in archo mod-settings, read
// from where the last read of the live log stopped; it stays after its log is gone, and a
// window alone puts its Mod in the list.
func TestModSettingsWindow(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("enable", "pkg:tester.b")
	session := `{"timestamp":"2026-11-03 10:00:00.000","level":"INFO","message":"[Archo Monitor] session 0123456789ab","details":"","count":1,"context":{"session":"0123456789ab","version":"0.11.0","game":"1.6.4633 rev1261"},"source":"archo-monitor","schema":1,"kind":"session"}` + "\n" +
		`{"timestamp":"2026-11-03 10:01:00.000","level":"INFO","message":"[Archo Monitor] settings 1","details":"","count":1,"context":{"settings":[{"packageId":"tester.a","folder":"1001","class":"AMod","file":"Mod_1001_AMod.xml","title":"A settings","stored":true}]},"source":"archo-monitor","schema":1,"kind":"settings"}` + "\n"
	log := filepath.Join(e.UserData, "ArchoMonitor.log")
	e.WriteFile(log, session)
	if out := e.MustRun("mod-settings"); strings.Contains(out, "B window") {
		t.Fatalf("before any window:\n%s", out)
	}
	windows := `{"timestamp":"2026-11-03 10:05:00.000","level":"INFO","message":"[Archo Monitor] settings window A settings","details":"","count":1,"context":{"window":{"packageId":"tester.a","folder":"1001","class":"AMod","file":"Mod_1001_AMod.xml","title":"A settings","full":true,"controls":[{"kind":"label","label":"General"},{"kind":"checkbox","label":"Raids","value":"False"},{"kind":"slider","label":"Scale","value":"0.75","min":"0","max":"2"},{"kind":"button","label":"Reset"}]}},"source":"archo-monitor","schema":1,"kind":"settings-window"}` + "\n" +
		`{"timestamp":"2026-11-03 10:06:00.000","level":"INFO","message":"[Archo Monitor] settings window B window","details":"","count":1,"context":{"window":{"packageId":"tester.b","folder":"B","class":"BMod","file":"Mod_B_BMod.xml","title":"B window","full":false,"controls":[{"kind":"text","label":"Name","value":"Bob"}]}},"source":"archo-monitor","schema":1,"kind":"settings-window"}` + "\n"
	e.WriteFile(log, session+windows)
	out := e.MustRun("mod-settings", "pkg:tester.a")
	for _, want := range []string{"the game's window (seen 2026-11-03", "General:", "Raids: False (checkbox)", "Scale: 0.75 (slider 0 to 2)", "[Reset] (button)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if out := e.MustRun("mod-settings"); !strings.Contains(out, "B window") {
		t.Fatalf("a window alone lists its Mod:\n%s", out)
	}
	// the next run rotates the log away: the window stays
	e.WriteFile(filepath.Join(e.UserData, "ArchoMonitor-prev.log"), "")
	e.WriteFile(log, strings.ReplaceAll(session, "0123456789ab", "ba9876543210"))
	out = e.MustRun("mod-settings", "pkg:tester.b")
	for _, want := range []string{`Name: "Bob" (text)`, "the window draws more than Archo Monitor keeps", "the Mod uses its defaults"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q after the log rotated:\n%s", want, out)
		}
	}
}

// 06 §2.9: HugsLib's settings packs. Without Archo Monitor's HugsLib record a pack is a
// Mod's only under its package id and shows what the file stores; the record maps a pack
// under another name and adds titles and defaults, and stays after its log is gone.
func TestModSettingsHugsLib(t *testing.T) {
	t.Parallel()
	e := loadoutEnv(t)
	scanNow(t, e)
	e.MustRun("enable", "pkg:tester.b")
	e.WriteFile(filepath.Join(e.UserData, "HugsLib", "ModSettings.xml"), `<?xml version="1.0" encoding="utf-8"?>
<settings>
  <tester.a><raids>True</raids></tester.a>
  <BPack><scale>2</scale><seen>5</seen><gone>x</gone></BPack>
</settings>`)
	out := e.MustRun("mod-settings")
	if !strings.Contains(out, "HugsLib (tester.a)") || strings.Contains(out, "BPack") {
		t.Fatalf("before the record:\n%s", out)
	}
	if out := e.MustRun("mod-settings", "pkg:tester.a"); !strings.Contains(out, "HugsLib settings tester.a") || !strings.Contains(out, "raids: True (on/off)") {
		t.Fatalf("a pack under the package id:\n%s", out)
	}

	log := filepath.Join(e.UserData, "ArchoMonitor.log")
	session := `{"timestamp":"2026-11-03 10:00:00.000","level":"INFO","message":"[Archo Monitor] session 0123456789ab","details":"","count":1,"context":{"session":"0123456789ab","version":"0.11.0","game":"1.6.4633 rev1261"},"source":"archo-monitor","schema":1,"kind":"session"}` + "\n"
	record := `{"timestamp":"2026-11-03 10:01:00.000","level":"INFO","message":"[Archo Monitor] hugslib 1","details":"","count":1,"context":{"hugslib":[{"id":"BPack","packageId":"tester.b","folder":"B","title":"B options","full":true,"settings":[{"name":"scale","title":"Scale","value":"2","default":"1","type":"Single"},{"name":"mode","title":"Mode","description":"How B works","value":"Normal","default":"Normal","type":"BMode"},{"name":"seen","value":"5","default":"0","type":"Int32","hidden":true}]}]},"source":"archo-monitor","schema":1,"kind":"hugslib"}` + "\n"
	e.WriteFile(log, session+record)
	if out := e.MustRun("mod-settings"); !strings.Contains(out, "HugsLib: B options") {
		t.Fatalf("the record maps a pack:\n%s", out)
	}
	// the next run rotates the log away: the record stays
	e.WriteFile(filepath.Join(e.UserData, "ArchoMonitor-prev.log"), "")
	e.WriteFile(log, strings.ReplaceAll(session, "0123456789ab", "ba9876543210"))
	out = e.MustRun("mod-settings", "pkg:tester.b")
	for _, want := range []string{`HugsLib "B options" (BPack)`, "2 changed", "Scale (scale): 2  [default 1]", `Mode (mode): "Normal"  [default]`, "gone: \"x\"", "1 setting the Mod keeps out of its window not shown"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "seen") {
		t.Fatalf("a hidden setting is shown:\n%s", out)
	}
}
