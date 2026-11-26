package store_test

import "time"

// timingLimit stretches a wall-clock bound under the race detector (raceBuild),
// which runs code several times slower; the bound still catches a lost lock wait.
func timingLimit(d time.Duration) time.Duration {
	if raceBuild {
		return 4 * d
	}
	return d
}
