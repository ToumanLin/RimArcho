package view_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/netcfg"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 16 §2.14, 17 §2.2: Settings › network holds the proxy and the GitHub mirror; config set
// checks them and the process's requests follow at once.
func TestNetworkSettings(t *testing.T) {
	e := testkit.New(t)
	t.Cleanup(func() { netcfg.Configure(netcfg.Settings{}) })
	// Never set: system proxy with fallback, automatic mirrors (17 §2.2).
	if out := e.MustRun("config", "get", "network.github_mirror"); !strings.Contains(out, "auto") {
		t.Fatalf("default mirror: %s", out)
	}
	if m := networkGroup(t, e).Items[1]; m.Value != "automatic" || *m.EditValue != "auto" || !strings.Contains(m.Hover, "gh-proxy.com") {
		t.Fatalf("default mirror row: %+v", m)
	}
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
	net := networkGroup(t, e)
	if len(net.Items) != 3 || net.Items[2].Key != "feedback.replies" {
		t.Fatalf("network group: %+v", net)
	}
	if p := net.Items[0]; p.Value != "socks5://127.0.0.1:7891" || !strings.Contains(p.ValueNote, "through 127.0.0.1:7891") {
		t.Errorf("proxy row: %+v", p)
	}
	if m := net.Items[1]; m.Value != "https://gh-proxy.org/" || strings.Join(m.Argv, " ") != "config set network.github_mirror {}" {
		t.Errorf("mirror row: %+v", m)
	}
	// off never uses one; empty goes back to the default, auto.
	e.MustRun("config", "set", "network.github_mirror", "off")
	if m := networkGroup(t, e).Items[1]; m.Value != "off" {
		t.Fatalf("off: %+v", m)
	}
	e.MustRun("config", "set", "network.github_mirror", "")
	if out := e.MustRun("config", "get", "network.github_mirror"); strings.Contains(out, "gh-proxy") || !strings.Contains(out, "auto") {
		t.Fatalf("mirror not cleared: %s", out)
	}
	e.MustRun("config", "set", "network.proxy", "off")
	if got := netcfg.Describe("https://github.com/"); got != "direct" {
		t.Fatalf("off: %q", got)
	}
}

func networkGroup(t *testing.T, e *testkit.Env) *view.SettingGroup {
	t.Helper()
	for _, g := range view.SettingSections(view.SettingsPage(e.App, nil)) {
		if g.Title == "network" {
			return &g
		}
	}
	t.Fatal("no network group")
	return nil
}

// 17 §2.2: the route names in the player's words.
func TestRouteText(t *testing.T) {
	for in, want := range map[string]string{"direct": "direct", "proxy 127.0.0.1:7890": "through proxy 127.0.0.1:7890", "mirror gh-proxy.com": "through mirror gh-proxy.com"} {
		if got := view.RouteText(in); got != want {
			t.Errorf("%s: %s", in, got)
		}
	}
	got := view.Humanize(nil, `Get "https://steamcommunity.com/x": dial tcp: i/o timeout (tried direct, proxy 127.0.0.1:7890; `+netcfg.SteamHint+`; or set a proxy)`)
	if !strings.HasPrefix(got, "Steam could not be reached.") || !strings.HasSuffix(got, "dial tcp: i/o timeout") {
		t.Errorf("Steam hint: %s", got)
	}
}
