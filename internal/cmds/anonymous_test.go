package cmds_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

type anonymousProc struct{}

func (anonymousProc) Wait() error { return nil }

func TestAnonymousConfigAndCLI(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	for _, value := range []string{"player", "anonymous", ""} {
		e.MustRun("config", "set", "steamcmd.account", value)
		if got := cmds.Setting(e.App, "steamcmd.account"); got != value {
			t.Fatal(got)
		}
	}
	if _, code := e.Run("config", "set", "steamcmd.account", "player password"); code == cmds.ExitOK {
		t.Fatal("accepted credentials")
	}
	e.WriteFile(filepath.Join(e.App.SteamCMDDir(), "steamcmd.exe"), "fake")
	e.App.SteamCMDNoPath = true
	var account string
	e.App.SteamCMDStart = func(_ string, args []string, _ bool) (app.SteamCMDProc, error) {
		account = args[3]
		return anonymousProc{}, nil
	}
	out, _ := e.Run("env", "download", "public", "--to", filepath.Join(e.Root, "Games", "New"))
	if account != "" || !strings.Contains(out, "cannot download RimWorld") {
		t.Fatalf("account %q, %s", account, out)
	}
}
