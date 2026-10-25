package gui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

type anonymousProc struct{}

func (anonymousProc) Wait() error { return nil }

func TestGuideAnonymousAccount(t *testing.T) {
	e := testkit.New(t)
	e.WriteFile(filepath.Join(e.App.SteamCMDDir(), "steamcmd.exe"), "fake")
	e.App.SteamCMDNoPath = true
	e.App.SteamCMDRun = func(_ string, args []string) (string, error) {
		if args[1] != "anonymous" {
			t.Errorf("public metadata uses %v", args)
		}
		return `"294100" { "depots" { "branches" { "public" { "buildid" "1" } } } }`, nil
	}
	called := make(chan string, 1)
	finished := make(chan bool, 1)
	e.App.SteamCMDStart = func(_ string, args []string, _ bool) (app.SteamCMDProc, error) {
		called <- args[3]
		return anonymousProc{}, nil
	}
	api := NewAPI(e.App, func(name string, data any) {
		if name == "download" {
			if end, ok := data.(map[string]any); ok && end["end"] == true {
				finished <- true
			}
		}
	})
	t.Cleanup(api.Close)
	if err := e.App.Store.SetMeta("cfg:steamcmd.account", "player"); err != nil {
		t.Fatal(err)
	}
	if info := api.GuideDownloadInfo(); info.Account != "player" || len(info.Branches) != 1 {
		t.Fatalf("%+v", info)
	}
	for i, tc := range []struct{ override, want string }{{"", "player"}, {"anonymous", ""}} {
		if ok, err := api.GuideDownload("public", filepath.Join(e.Root, "Games", string(rune('A'+i))), tc.override, ""); !ok || err != nil {
			t.Fatalf("%v %v", ok, err)
		}
		if tc.want != "" {
			select {
			case account := <-called:
				if account != tc.want {
					t.Fatalf("%q", account)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("SteamCMD was not started")
			}
		}
		select {
		case <-finished:
		case <-time.After(10 * time.Second):
			t.Fatal("no end event")
		}
		if tc.want == "" {
			select {
			case account := <-called:
				t.Fatalf("anonymous download started SteamCMD: %q", account)
			default:
			}
			if _, err := os.Stat(filepath.Join(e.Root, "Games", string(rune('A'+i)))); !os.IsNotExist(err) {
				t.Fatalf("anonymous download created a folder: %v", err)
			}
		}
	}
	if ok, err := api.GuideDownload("public", filepath.Join(e.Root, "Games", "Bad"), "bad +quit", ""); ok || err == nil {
		t.Fatalf("%v %v", ok, err)
	}
}
