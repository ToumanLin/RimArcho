package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestPlayerAccountForgetAndDownloadProvenance(t *testing.T) {
	e := testkit.New(t)
	api := newAPI(e.App)
	t.Cleanup(api.Close)
	if _, err := api.AccountAction("loverslab", "enable"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(e.App.AccountsDir(), "downloads", "fixture.zip")
	e.WriteFile(path, "offline-archive-fixture")
	e.WriteFile(filepath.Join(e.App.AccountsDir(), "loverslab.session"), "offline-cookie-sentinel")
	e.WriteFile(filepath.Join(e.App.AccountsDir(), "webview-profile", "Cookies"), "offline-profile-sentinel")
	page := "https://www.loverslab.com/files/file/123-fixture/"
	api.llFiles = map[string]loverDownload{"opaque-fixture-selection": {page: page, path: path}}
	other := "https://www.loverslab.com/files/file/456-other/"
	if _, err := api.LoverInstallPreview(other, "opaque-fixture-selection", ""); err == nil {
		t.Fatal("preview accepted another page's download")
	}
	if _, err := api.LoverInstall(other, "opaque-fixture-selection", ""); err == nil {
		t.Fatal("install accepted another page's download")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("invalid request consumed player's file")
	}
	if _, err := api.AccountAction("loverslab", "forget"); err != nil {
		t.Fatal(err)
	}
	_ = endedTask(t, api, view.TaskAccount)
	if _, err := os.Stat(e.App.AccountsDir()); !os.IsNotExist(err) {
		t.Fatal("session/profile/download remains")
	}
	if len(api.llFiles) != 0 || e.App.Store.Meta("account:loverslab:enabled") != "" {
		t.Fatal("forgotten session enabled")
	}
	b, err := os.ReadFile(filepath.Join(e.Home, "tasks.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"offline-cookie-sentinel", "offline-profile-sentinel", "opaque-fixture-selection", page, "fixture.zip"} {
		if strings.Contains(string(b), secret) {
			t.Fatal("account task contains private data")
		}
	}
	if _, err := api.AccountAction("loverslab", "import-cookies"); err == nil {
		t.Fatal("unexpected credential action")
	}
}
