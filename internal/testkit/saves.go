package testkit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SaveSpec describes a synthetic save (10 §1.3): only <meta> and a token game block.
type SaveSpec struct {
	Game     string   // gameVersion; "" is the fixture's 1.6.4633 rev1261
	Mods     []string // packageIds in load order
	Names    []string // one per Mod; "" uses the packageId
	SteamIDs []string // one per Mod; "" writes 0
	CutOff   bool     // the file stops inside <game>, as after a crash
	ModTime  time.Time
}

// SaveXML is the save's text.
func SaveXML(s SaveSpec) string {
	game := s.Game
	if game == "" {
		game = "1.6.4633 rev1261"
	}
	var ids, steam, names strings.Builder
	for i, m := range s.Mods {
		fmt.Fprintf(&ids, "<li>%s</li>", m)
		sid, name := "0", m
		if i < len(s.SteamIDs) && s.SteamIDs[i] != "" {
			sid = s.SteamIDs[i]
		}
		if i < len(s.Names) && s.Names[i] != "" {
			name = s.Names[i]
		}
		fmt.Fprintf(&steam, "<li>%s</li>", sid)
		fmt.Fprintf(&names, "<li>%s</li>", name)
	}
	x := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<savegame>
	<meta>
		<gameVersion>%s</gameVersion>
		<modIds>%s</modIds>
		<modSteamIds>%s</modSteamIds>
		<modNames>%s</modNames>
	</meta>
	<game>
		<tickManager><ticksGame>60000</ticksGame></tickManager>
		<world><info><name>Synthetic</name></info></world>
	</game>
</savegame>
`, game, ids.String(), steam.String(), names.String())
	if s.CutOff {
		x = x[:strings.Index(x, "<world>")+len("<world><in")]
	}
	return x
}

// SavesDir is the fixture's game Saves folder.
func (e *Env) SavesDir() string { return filepath.Join(e.UserData, "Saves") }

// WriteSave writes a save into the fixture's Saves folder and returns its path. Its
// modification time is s.ModTime, else the fake clock minus a minute (so it is not being
// written).
func (e *Env) WriteSave(file string, s SaveSpec) string {
	e.T.Helper()
	p := filepath.Join(e.SavesDir(), file)
	e.WriteFile(p, SaveXML(s))
	at := s.ModTime
	if at.IsZero() {
		at = e.now.Add(-time.Minute)
	}
	if err := os.Chtimes(p, at, at); err != nil {
		e.T.Fatal(err)
	}
	return p
}
