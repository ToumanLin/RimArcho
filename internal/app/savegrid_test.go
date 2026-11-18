package app

import "testing"

// The values are the hashes a real 1.6 save's grids hold for these Defs.
func TestDefShortHash(t *testing.T) {
	for name, want := range map[string]uint16{"Space": 41780, "MetalTile": 2806, "TemperateForest": 54643, "Soil": 34465} {
		if got := defShortHash(name); got != want {
			t.Errorf("%s: %d, want %d", name, got, want)
		}
	}
	// of two names with one hash, the later in name order takes the next free one
	got := shortHashes([]string{"MetalTile", "Space"})
	if got[2806] != "MetalTile" || got[41780] != "Space" || len(got) != 2 {
		t.Errorf("%v", got)
	}
}
