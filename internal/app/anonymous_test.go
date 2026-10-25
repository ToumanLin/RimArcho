package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestSteamCMDAnonymousAndAccountPrecedence(t *testing.T) {
	for _, tc := range []struct{ name, saved, override, want string }{
		{"default", "", "", "anonymous"},
		{"saved", "player", "", "player"},
		{"override", "player", "other", "other"},
		{"force anonymous", "player", "ANONYMOUS", "anonymous"},
		{"trim", "player", " other ", "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := testkit.New(t)
			if err := e.App.Store.SetMeta("cfg:steamcmd.account", tc.saved); err != nil {
				t.Fatal(err)
			}
			f := &fakeSteamCMD{lines: []string{"Connecting anonymously to Steam Public...OK", "ERROR! Failed to install app '294100' (No subscription)"}}
			f.install(t, e)
			_, err := e.App.FetchGame(app.DownloadSpec{Dir: filepath.Join(e.Root, "Game"), Account: tc.override}, nil)
			if tc.want != "anonymous" && (len(f.args) < 4 || f.args[3] != tc.want) {
				t.Fatalf("args %v, error %v", f.args, err)
			}
			var de *app.DownloadError
			if !errors.As(err, &de) {
				t.Fatal(err)
			}
			if tc.want == "anonymous" {
				if de.Kind != "anonymous-license" || !strings.Contains(de.Why(), "game folder") || len(f.args) != 0 {
					t.Fatalf("%+v", de)
				}
			} else if de.Kind != "ownership" {
				t.Fatalf("%+v", de)
			}
		})
	}
}

func TestSteamCMDBadSavedAccountNeverStarts(t *testing.T) {
	e := testkit.New(t)
	f := &fakeSteamCMD{}
	f.install(t, e)
	e.App.Store.SetMeta("cfg:steamcmd.account", "bad +quit")
	if _, err := e.App.FetchGame(app.DownloadSpec{Dir: filepath.Join(e.Root, "Game")}, nil); err == nil || f.args != nil {
		t.Fatalf("args %v, err %v", f.args, err)
	}
}

func TestAnonymousGameDownloadDoesNotStartOrCreateFolder(t *testing.T) {
	e := testkit.New(t)
	f := &fakeSteamCMD{lines: okRun, game: true}
	f.install(t, e)
	dir := filepath.Join(e.Root, "Game")
	_, err := e.App.FetchGame(app.DownloadSpec{Dir: dir}, nil)
	var de *app.DownloadError
	if !errors.As(err, &de) || de.Kind != "anonymous-license" || len(f.args) != 0 {
		t.Fatal(err, f.args)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("anonymous request created game folder")
	}
}

func TestWorkshopStateWithoutSteam(t *testing.T) {
	e := setup(t, false)
	e.AddWorkshop("7100", testkit.Sample("tester.local", "Local"))
	scanNow(t, e)
	ref := refID(t, e, "Local", source.Workshop)
	op, err := e.App.WorkshopState([]string{ref})
	if err != nil || !op.Local || len(op.Items) != 1 || !op.Items[0].Installed || op.Items[0].State != 0 || op.Note == "" {
		t.Fatalf("%+v %v", op, err)
	}
	out := e.MustRun("workshop", "state", ref)
	if !strings.Contains(out, "local folder present") || !strings.Contains(out, "state unknown") || strings.Contains(out, "not subscribed") {
		t.Fatal(out)
	}
	if _, err := e.App.Workshop("unsubscribe", []string{ref}, false); !errors.Is(err, app.ErrSteamUnavailable) {
		t.Fatalf("must not fake an unsubscribe: %v", err)
	}
}

func TestManualGameWithWorkshopDefaultsToDirect(t *testing.T) {
	e := testkit.New(t)
	game := filepath.Join(e.Root, "Standalone")
	e.WriteFile(filepath.Join(game, "RimWorldWin64.exe"), "")
	e.CopyTemplate("core", filepath.Join(game, "Data", "Core"))
	g, _, err := e.App.EnvAdd("manual", app.EnvSpec{GameRoot: game, WorkshopRoot: e.Workshop})
	if err != nil || g.Launch != "direct" {
		t.Fatalf("%+v %v", g, err)
	}
}
