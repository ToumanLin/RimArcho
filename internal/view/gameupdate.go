package view

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/vdf"
)

// GameUpdateLine is the Launch page's line about a game update (16 §2.3): the game folder's
// version changed since the last scan, or Steam has an update waiting for it. Its link,
// "back up <game> first", opens Add Game Config › copy a game folder (03 §3.5).
type GameUpdateLine struct {
	Label string `json:"label"` // "the game"
	Text  string `json:"text"`  // "updated to 1.6.4633" / "Steam has an update waiting"
	Env   string `json:"env"`   // the Game Config whose folder the copy starts from
	Link  string `json:"link"`  // "back up 1.6 first"
}

// GameUpdateHome says whether the current Game Config's game changed or is about to; nil
// when neither, or when the version is unknown. It reads Version.txt and Steam's
// appmanifest_294100.acf next to the game, two small files.
func GameUpdateHome(a *app.App, e *env.Environment) *GameUpdateLine {
	if e == nil || e.GameRoot == "" {
		return nil
	}
	line := &GameUpdateLine{Label: T("the game"), Env: e.Name, Link: Tf("back up %s first", e.Name)}
	if now := env.ReadGameVersion(e.GameRoot); now != "" && e.GameVersion != "" && now != e.GameVersion {
		line.Text = Tf("updated to %s since the last scan", now)
		return line
	}
	if steamUpdatePending(e.GameRoot) {
		line.Text = T("Steam has an update waiting")
		return line
	}
	return nil
}

// steamUpdatePending: the game's Steam manifest (…/steamapps/appmanifest_294100.acf for a
// game in …/steamapps/common/<folder>) flags an update (StateFlags' UpdateRequired bit) or
// names a target build other than the installed one. False when there is no manifest.
func steamUpdatePending(gameRoot string) bool {
	common := filepath.Dir(filepath.Clean(gameRoot))
	if !strings.EqualFold(filepath.Base(common), "common") {
		return false
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(common), "appmanifest_294100.acf"))
	if err != nil {
		return false
	}
	n, err := vdf.Parse(b)
	if err != nil {
		return false
	}
	st := n.Get("AppState")
	if st == nil {
		return false
	}
	if flags, err := strconv.Atoi(st.Str("StateFlags")); err == nil && flags&2 != 0 {
		return true
	}
	target, build := st.Str("TargetBuildID"), st.Str("buildid")
	return target != "" && target != "0" && build != "" && target != build
}
