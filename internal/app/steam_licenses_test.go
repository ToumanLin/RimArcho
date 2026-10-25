package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSteamLicenseApps(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, output string
		owns         bool
	}{
		{"apps", "License packageID 123:\n - State : Active\n - Apps : 123, 294100, 567\n - Depots : 123", true},
		{"app ID", "License packageID 456:\n AppID: 294100 (RimWorld)", true},
		{"wrapped", "License packageID 1:\n Apps: 123,\n 294100, 456\n Depots: 77", true},
		{"another license", "License packageID 1:\n Apps: 10\nLicense packageID 2:\n Apps: 294100", true},
		{"timestamped", "[2026-10-24 18:10:34] License packageID 123:\n[2026-10-24 18:10:34] - Apps : 294100", true},
		{"package not app", "License packageID 294100:\n Apps: 123\n Depots: 294100", false},
		{"depot on wrapped list", "License packageID 1:\n Apps: 123,\n Depots: 294100", false},
		{"substring", "License packageID 1:\n Apps: 1294100, 2941000", false},
		{"untrusted log text", "Workshop appID 294100\n Apps: 294100", false},
		{"anonymous", "No licenses found.", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := steamOwnsRimWorld(tc.output); got != tc.owns {
				t.Fatalf("owns=%v, want %v", got, tc.owns)
			}
		})
	}
}

func TestSteamLicenseVMFixtures(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"real-purchased.log", "borrowed-and-truncated.log"} {
		b, err := os.ReadFile(filepath.Join("testdata", "steam-licenses", name))
		if err != nil {
			t.Fatal(err)
		}
		if !steamOwnsRimWorld(string(b)) {
			t.Fatalf("RimWorld missing in %s", name)
		}
		withoutRimWorld := strings.NewReplacer("Apps\t: 294100", "Apps\t: 10", "Apps : 294100", "Apps : 10").Replace(string(b))
		if steamOwnsRimWorld(withoutRimWorld) {
			t.Fatal("depot, package or truncated count counted as app")
		}
	}
}
