//go:build !race

package analyze

import "time"

// timingLimit is the bound itself without the race detector (race_on_test.go).
func timingLimit(d time.Duration) time.Duration { return d }
