//go:build race

package analyze

import "time"

// timingLimit stretches a wall-clock bound under the race detector, which runs code
// several times slower (17 §4 "Concurrency"); the bound still catches a lost budget.
func timingLimit(d time.Duration) time.Duration { return 4 * d }
