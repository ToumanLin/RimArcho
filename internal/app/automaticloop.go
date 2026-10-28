package app

import (
	"context"
	"time"
)

// AutomaticLoop uses one timer for all maintenance. Surfaces supply a cycle
// that opens a short-lived connection, and cancel it when they close.
func AutomaticLoop(ctx context.Context, cycle func(context.Context)) {
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		cycle(ctx)
		timer.Reset(time.Minute)
	}
}
