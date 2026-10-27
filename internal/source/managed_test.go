package source

import "testing"

func TestManagedKinds(t *testing.T) {
	for _, tc := range []struct {
		kind              Kind
		managed, gameMods bool
	}{
		{Local, true, true}, {Elsewhere, true, true},
		{SteamCMD, true, false}, {Extra, true, false},
		{Official, false, false}, {Workshop, false, false}, {"unknown", false, false},
	} {
		if tc.kind.Managed() != tc.managed {
			t.Errorf("%s managed = %v, want %v", tc.kind, tc.kind.Managed(), tc.managed)
		}
		if tc.kind.GameMods() != tc.gameMods {
			t.Errorf("%s game Mods = %v, want %v", tc.kind, tc.kind.GameMods(), tc.gameMods)
		}
	}
}
