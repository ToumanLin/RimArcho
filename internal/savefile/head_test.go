package savefile

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestReadHead(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name, s string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	full := `<?xml version="1.0" encoding="utf-8"?>
<savegame><meta><gameVersion>1.6.4633 rev1261</gameVersion>
<modIds><li>Ludeon.RimWorld</li><li>tester.local</li><li>tester.ws</li></modIds>
<modSteamIds><li>0</li><li/><li>3000000002</li></modSteamIds>
<modNames><li>Core</li><li></li><li>Workshop Mod</li></modNames></meta>
<game><x>…</x></game></savegame>` + "\r\n"
	h, f, changed, err := ReadHead(write("ok.rws", full))
	if err != nil || changed || f.Size != int64(len(full)) {
		t.Fatalf("read: %v changed=%v size=%d", err, changed, f.Size)
	}
	if !h.Complete || h.GameVersion != "1.6.4633 rev1261" || !slices.Equal(h.ModIDs, []string{"ludeon.rimworld", "tester.local", "tester.ws"}) {
		t.Fatalf("head %+v", h)
	}
	if h.Name(1) != "tester.local" || h.Name(2) != "Workshop Mod" || h.SteamID(1) != "" || h.SteamID(2) != "3000000002" {
		t.Fatalf("names %q steam %q", h.ModNames, h.SteamIDs)
	}

	cut := full[:strings.Index(full, "<x>")+2]
	if h, _, _, err := ReadHead(write("cut.rws", cut)); err != nil || h.Complete {
		t.Fatalf("cut off: %+v %v", h, err)
	}
	if _, _, _, err := ReadHead(write("empty.rws", `<savegame><meta><gameVersion>1.6</gameVersion></meta></savegame>`)); !errors.Is(err, ErrNoMeta) {
		t.Fatalf("no mods: %v", err)
	}
}
