package mcp_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestAccountBoundaryNoSecretsInMCP(t *testing.T) {
	e := testkit.New(t)
	e.MustRun("config", "set", "steamcmd.account", "fixture-player")
	e.WriteFile(filepath.Join(e.App.AccountsDir(), "loverslab.session"), "offline-cookie-sentinel")
	e.WriteFile(filepath.Join(e.App.LocalDir(), "steamcmd", "config", "config.vdf"), "offline-token-sentinel")
	cs := connect(t, e)
	out, bad := call(t, cs, "status", map[string]any{"show": "settings"})
	if bad || !strings.Contains(out, "player login required") {
		t.Fatal(out)
	}
	for _, secret := range []string{"fixture-player", "offline-cookie-sentinel", "offline-token-sentinel"} {
		if strings.Contains(out, secret) {
			t.Fatal("MCP leaked account data")
		}
	}
	catalog, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(catalog)
	for _, field := range []string{`"cookie"`, `"password"`, `"credential"`, `"csrfKey"`, `"AccountAction"`, `"LoverFollow"`} {
		if strings.Contains(string(raw), field) {
			t.Fatal("credential capability exposed", field)
		}
	}
	if out, code := mcp.New(e.App, mcp.All).Execute("", []string{"account", "loverslab", "login"}); code == 0 {
		t.Fatal("MCP executed player account action", out)
	}
}
