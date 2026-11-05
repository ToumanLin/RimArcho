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

	// The full read (10 §1.7 L1). Unset fields keep the token game block as it is.
	Ticks   int64  // ticksGame; 0 writes 60000 (day 1)
	World   string // world name; "" writes Synthetic
	Seed    string // world seed; with it, persistentRandomValue PRV is written too
	PRV     int
	Faction string // the player faction's name (a PlayerColony faction)
	Body    string // raw XML added at the end of <game>: pawns, things, Class="…" values
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
	ticks, world := s.Ticks, s.World
	if ticks == 0 {
		ticks = 60000
	}
	if world == "" {
		world = "Synthetic"
	}
	info := "<name>" + world + "</name>"
	if s.Seed != "" {
		info += fmt.Sprintf("<seedString>%s</seedString><persistentRandomValue>%d</persistentRandomValue>", s.Seed, s.PRV)
	}
	factions := ""
	if s.Faction != "" {
		factions = "<factionManager><allFactions><li><def>PlayerColony</def><name>" + s.Faction + "</name><loadID>1</loadID></li></allFactions></factionManager>"
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
		<tickManager><ticksGame>%d</ticksGame></tickManager>
		<world><info>%s</info>%s</world>%s
	</game>
</savegame>
`, game, ids.String(), steam.String(), names.String(), ticks, info, factions, s.Body)
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
