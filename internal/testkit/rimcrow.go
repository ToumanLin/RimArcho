package testkit

import (
	"database/sql"
	"encoding/json"
	"testing"
)

// CrowProfile is a synthetic GameProfile; paths must belong to the calling test's fixture.
type CrowProfile struct {
	ID, Name, Game, UserData string
	Args                     []string
	Steam                    bool
}

// RimCrowProfiles builds only the GameProfile table; no real manager data is used.
func RimCrowProfiles(t testing.TB, path string, profiles ...CrowProfile) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE gameprofile (id TEXT PRIMARY KEY,name TEXT NOT NULL,game_install_path TEXT NOT NULL,user_data_path TEXT NOT NULL,run_commands TEXT NOT NULL,prefer_steam_launch INTEGER NOT NULL,use_workshop_mods INTEGER NOT NULL,use_self_mods INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, p := range profiles {
		args, _ := json.Marshal(p.Args)
		if p.Args == nil {
			args = []byte("[]")
		}
		if _, err := db.Exec(`INSERT INTO gameprofile VALUES(?,?,?,?,?,?,1,0)`, p.ID, p.Name, p.Game, p.UserData, string(args), p.Steam); err != nil {
			t.Fatal(err)
		}
	}
}
