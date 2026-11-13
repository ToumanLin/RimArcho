package app

import (
	"os"
	"path/filepath"

	"github.com/ToumanLin/RimArcho/internal/accounts"
)

// cleanSelfWorkshop removes the private SteamCMD directory left by older updaters.
// Workshop relocation and release-only item cleanup are independent (17 §5.1).
func (a *App) cleanSelfWorkshop() {
	base := filepath.Join(a.LocalDir(), "steamcmd-self")
	unlock := accounts.Lock(base)
	defer unlock()
	_ = os.RemoveAll(base)
}
