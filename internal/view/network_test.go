package view_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/netcfg"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 06 §2.14, 07 §2.2: Settings › network holds the proxy and the GitHub mirror; config set
// checks them and the process's requests follow at once.
func TestNetworkSettings(t *testing.T) {
	e := testkit.New(t)
	t.Cleanup(func() { netcfg.Configure(netcfg.Settings{}) })
	for _, bad := range [][]string{{"network.proxy", "on"}, {"network.proxy", "ftp://x:1"}, {"network.github_mirror", "gh-proxy.org"}} {
		if _, code := e.Run(append([]string{"config", "set"}, bad...)...); code != 2 {
			t.Errorf("config set %v: exit %d", bad, code)
		}
	}
	e.MustRun("config", "set", "network.proxy", "socks5://127.0.0.1:7891")
	e.MustRun("config", "set", "network.github_mirror", "https://gh-proxy.org/")
	if got := netcfg.Describe("https://github.com/"); got != "127.0.0.1:7891" {
		t.Fatalf("the proxy is not in use: %q", got)
	}
	var net *view.SettingGroup
	for _, g := range view.SettingsPage(e.App, nil) {
		if g.Title == "network" {
			net = &g
		}
	}
	if net == nil || len(net.Items) != 2 {
		t.Fatalf("network group: %+v", net)
	}
	if p := net.Items[0]; p.Value != "socks5://127.0.0.1:7891" || !strings.Contains(p.ValueNote, "through 127.0.0.1:7891") {
		t.Errorf("proxy row: %+v", p)
	}
	if m := net.Items[1]; m.Value != "https://gh-proxy.org/" || strings.Join(m.Argv, " ") != "config set network.github_mirror {}" {
		t.Errorf("mirror row: %+v", m)
	}
	// Empty goes back to GitHub directly.
	e.MustRun("config", "set", "network.github_mirror", "")
	if out := e.MustRun("config", "get", "network.github_mirror"); strings.Contains(out, "gh-proxy") {
		t.Fatalf("mirror not cleared: %s", out)
	}
	e.MustRun("config", "set", "network.proxy", "off")
	if got := netcfg.Describe("https://github.com/"); got != "direct" {
		t.Fatalf("off: %q", got)
	}
}
