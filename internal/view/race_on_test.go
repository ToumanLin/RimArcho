//go:build race

package view_test

import "time"

// timingLimit stretches a wall-clock bound under the race detector, which runs
// code several times slower; the bound still catches a lost batch or lock wait.
func timingLimit(d time.Duration) time.Duration { return 4 * d }
