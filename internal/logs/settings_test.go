package logs

import (
	"os"
	"path/filepath"
	"testing"
)

const settingsSession = `{"timestamp":"2026-11-03 10:00:00.000","level":"INFO","message":"[Archo Monitor] session 0123456789ab","details":"","count":1,"context":{"session":"0123456789ab","version":"0.10.0","game":"1.6.4871 rev590"},"source":"archo-monitor","schema":1,"kind":"session"}
`

const settingsLine = `{"timestamp":"2026-11-03 10:01:00.000","level":"INFO","message":"[Archo Monitor] settings 2","details":"","count":1,"context":{"settings":[{"packageId":"a.mod","folder":"!ArchoPack_x_1","class":"AMod","file":"Mod__ArchoPack_x_1_AMod.xml","title":"A","stored":true},{"packageId":"b.mod","folder":"2","class":"BMod","file":"Mod_2_BMod.xml","title":"","stored":true}]},"source":"archo-monitor","schema":1,"kind":"settings"}
`

const settingsError = `{"timestamp":"2026-11-03 10:00:30.000","level":"ERROR","message":"boom","details":"","count":1,"context":{},"source":"archo-monitor","schema":1,"id":"aaaaaaaa","total":1}
`

// 03 §6.7.1: the settings record is read, not taken for an entry; ReadMonitorSettings stops
// at it and has nothing before the main menu.
func TestMonitorSettingsRecord(t *testing.T) {
	c := NewCompanion("x")
	c.Feed([]byte(settingsSession+settingsError+settingsLine), true)
	if len(c.Entries) != 1 || c.BadLines != 0 || len(c.Settings) != 2 || c.Settings[0].Folder != "!ArchoPack_x_1" || !c.Settings[1].Stored {
		t.Fatalf("entries %d, bad %d, settings %+v", len(c.Entries), c.BadLines, c.Settings)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, MonitorLog)
	if err := os.WriteFile(path, []byte(settingsSession+settingsError), 0o644); err != nil {
		t.Fatal(err)
	}
	if ms, err := ReadMonitorSettings(path); err != nil || ms != nil {
		t.Fatalf("before the menu: %+v, %v", ms, err)
	}
	if err := os.WriteFile(path, []byte(settingsSession+settingsError+settingsLine+settingsError), 0o644); err != nil {
		t.Fatal(err)
	}
	ms, err := ReadMonitorSettings(path)
	if err != nil || ms == nil || ms.Session != "0123456789ab" || ms.Game != "1.6.4871 rev590" || ms.Started.IsZero() || len(ms.Files) != 2 {
		t.Fatalf("record %+v, %v", ms, err)
	}
	if f := ms.Files[0]; f.PackageID != "a.mod" || f.Class != "AMod" || f.File != "Mod__ArchoPack_x_1_AMod.xml" || f.Title != "A" {
		t.Fatalf("file %+v", f)
	}
	if err := os.WriteFile(path, []byte(settingsLine), 0o644); err != nil {
		t.Fatal(err)
	}
	if ms, _ := ReadMonitorSettings(path); ms != nil {
		t.Fatal("a log without a session line")
	}
}
