package env

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/ToumanLin/RimArcho/internal/vdf"
)

// steamID64Base turns a Steam account ID into its 64-bit Steam ID.
const steamID64Base = 76561197960265728

// SteamSignedIn is the account name the running Steam client is signed in with. ok is false
// when Steam is not running or nobody is signed in; account is "" when Steam is signed in
// but its account name cannot be read.
func SteamSignedIn() (account string, ok bool) {
	id, autoLogin := steamActiveUser()
	if id == 0 {
		return "", false
	}
	if root := registrySteamPath(); root != "" {
		if b, err := os.ReadFile(filepath.Join(root, "config", "loginusers.vdf")); err == nil {
			if name := LoginUserName(b, id); name != "" {
				return name, true
			}
		}
	}
	return autoLogin, true
}

// LoginUserName is the AccountName loginusers.vdf gives the account ID, or "".
func LoginUserName(b []byte, id uint32) string {
	root, err := vdf.Parse(b)
	if err != nil {
		return ""
	}
	return root.Str("users", strconv.FormatUint(steamID64Base+uint64(id), 10), "AccountName")
}
