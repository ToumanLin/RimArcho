//go:build !windows

package env_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestSteamSignedIn(t *testing.T) {
	e := testkit.New(t)
	t.Setenv("HOME", e.Root)
	path := filepath.Join(e.Root, ".steam", "registry.vdf")
	for _, tc := range []struct {
		active string
		want   string
		ok     bool
	}{
		{fmt.Sprintf(`"ActiveUser" "79468545" "pid" "%d"`, os.Getpid()), "second", true},
		{fmt.Sprintf(`"ActiveUser" "7" "pid" "%d"`, os.Getpid()), "fallback", true},
		{`"ActiveUser" "79468545" "pid" "2147483647"`, "", false},
		{`"ActiveUser" "0" "pid" "1"`, "", false},
		{`"ActiveUser" "invalid"`, "", false},
	} {
		e.WriteFile(path, `"Registry" { "HKCU" { "Software" { "Valve" { "Steam" { "AutoLoginUser" "fallback" "ActiveProcess" { `+tc.active+` } } } } } }`)
		e.WriteFile(filepath.Join(e.Root, ".steam", "steam", "config", "loginusers.vdf"), `"users" { "76561198039734273" { "AccountName" "second" } }`)
		if name, ok := env.SteamSignedIn(); name != tc.want || ok != tc.ok {
			t.Fatalf("%s: %q %v", tc.active, name, ok)
		}
	}
	e.WriteFile(path, `"Registry" {`)
	if name, ok := env.SteamSignedIn(); ok || name != "" {
		t.Fatal("malformed registry signed in")
	}
}
