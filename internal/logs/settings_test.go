package logs

import (
	"os"
	"path/filepath"
	"testing"
)

const settingsSession = `{"timestamp":"2026-11-03 10:00:00.000","level":"INFO","message":"[Archo Monitor] session 0123456789ab","details":"","count":1,"context":{"session":"0123456789ab","version":"0.11.0","game":"1.6.4871 rev590"},"source":"archo-monitor","schema":1,"kind":"session"}
`

const settingsLine = `{"timestamp":"2026-11-03 10:01:00.000","level":"INFO","message":"[Archo Monitor] settings 2","details":"","count":1,"context":{"settings":[{"packageId":"a.mod","folder":"!ArchoPack_x_1","class":"AMod","file":"Mod__ArchoPack_x_1_AMod.xml","title":"A","stored":true},{"packageId":"b.mod","folder":"2","class":"BMod","file":"Mod_2_BMod.xml","title":"","stored":true}]},"source":"archo-monitor","schema":1,"kind":"settings"}
`

const settingsError = `{"timestamp":"2026-11-03 10:00:30.000","level":"ERROR","message":"boom","details":"","count":1,"context":{},"source":"archo-monitor","schema":1,"id":"aaaaaaaa","total":1}
`

const settingsWindowLine = `{"timestamp":"2026-11-03 10:05:00.000","level":"INFO","message":"[Archo Monitor] settings window A","details":"","count":1,"context":{"window":{"packageId":"a.mod","folder":"A","class":"AMod","file":"Mod_A_AMod.xml","title":"A","full":true,"controls":[{"kind":"label","label":"General"},{"kind":"checkbox","label":"Raids","value":"False"},{"kind":"slider","label":"Scale","value":"0.75","min":"0","max":"2"}]}},"source":"archo-monitor","schema":1,"kind":"settings-window"}
`

const hugsLibLine = `{"timestamp":"2026-11-03 10:01:01.000","level":"INFO","message":"[Archo Monitor] hugslib 1","details":"","count":1,"context":{"hugslib":[{"id":"APack","packageId":"a.mod","folder":"A","title":"A options","full":true,"settings":[{"name":"raids","title":"Raids","value":"True","default":"False","type":"Boolean"},{"name":"seen","value":"3","default":"0","type":"Int32","hidden":true}]}]},"source":"archo-monitor","schema":1,"kind":"hugslib"}
`

// 19 §2: the settings records are read, not taken for entries. ReadMonitorRecords has no
// settings record before the main menu, and reads a live log again only past its last read.
func TestMonitorSettingsRecord(t *testing.T) {
	c := NewCompanion("x")
	c.Feed([]byte(settingsSession+settingsError+settingsLine+hugsLibLine+settingsWindowLine), true)
	if len(c.Entries) != 1 || c.BadLines != 0 || len(c.Settings) != 2 || c.Settings[0].Folder != "!ArchoPack_x_1" || !c.Settings[1].Stored {
		t.Fatalf("entries %d, bad %d, settings %+v", len(c.Entries), c.BadLines, c.Settings)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, MonitorLog)
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	read := map[string]int64{}
	write(settingsSession + settingsError)
	mr, err := ReadMonitorRecords(path, read)
	if err != nil || mr == nil || mr.Files != nil || mr.Session != "0123456789ab" || mr.End != int64(len(settingsSession+settingsError)) {
		t.Fatalf("before the menu: %+v, %v", mr, err)
	}
	read[mr.Session] = mr.End
	// the menu, then a window; its last line is still being written
	write(settingsSession + settingsError + settingsLine + settingsError + settingsWindowLine[:40])
	mr, err = ReadMonitorRecords(path, read)
	if err != nil || mr == nil || mr.Game != "1.6.4871 rev590" || mr.Started.IsZero() || len(mr.Files) != 2 || len(mr.Windows) != 0 {
		t.Fatalf("record %+v, %v", mr, err)
	}
	if f := mr.Files[0]; f.PackageID != "a.mod" || f.Class != "AMod" || f.File != "Mod__ArchoPack_x_1_AMod.xml" || f.Title != "A" {
		t.Fatalf("file %+v", f)
	}
	if mr.End != int64(len(settingsSession+settingsError+settingsLine+settingsError)) {
		t.Fatalf("a half-written line is not read: end %d", mr.End)
	}
	if mr.HugsLib != nil {
		t.Fatalf("no HugsLib record yet: %+v", mr.HugsLib)
	}
	read[mr.Session] = mr.End
	write(settingsSession + settingsError + settingsLine + settingsError + hugsLibLine + settingsWindowLine)
	mr, err = ReadMonitorRecords(path, read)
	if err != nil || mr == nil || mr.Files != nil || len(mr.Windows) != 1 || len(mr.HugsLib) != 1 {
		t.Fatalf("past the record: %+v, %v", mr, err)
	}
	if p := mr.HugsLib[0]; p.ID != "APack" || p.PackageID != "a.mod" || p.Title != "A options" || !p.Full || len(p.Settings) != 2 ||
		p.Settings[0].Default != "False" || p.Settings[0].Title != "Raids" || !p.Settings[1].Hidden || p.Settings[1].Type != "Int32" {
		t.Fatalf("HugsLib pack %+v", p)
	}
	w := mr.Windows[0]
	if w.Class != "AMod" || !w.Full || len(w.Controls) != 3 || w.Controls[2].Kind != "slider" || w.Controls[2].Max != "2" || w.Controls[0].Value != "" || w.At.IsZero() {
		t.Fatalf("window %+v", w)
	}
	// another session's offset does not apply
	if mr, _ := ReadMonitorRecords(path, map[string]int64{"other": 1 << 20}); mr == nil || len(mr.Files) != 2 || len(mr.Windows) != 1 {
		t.Fatalf("another session: %+v", mr)
	}
	write(settingsLine)
	if mr, _ := ReadMonitorRecords(path, nil); mr != nil {
		t.Fatal("a log without a session line")
	}
}
