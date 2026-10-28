package steamworks

import "time"

// Starting the Steam API counts as starting the game, and the Steam client then asks the
// server for the account's subscription list. The answer can be older than a subscribe or
// unsubscribe made in the same second, and the client applies it anyway: the change holds
// on the server but is undone on this computer until the next refresh (the folder neither
// downloads nor goes). Seen in the VM run of 2026-10-27, Steam's workshop_log.txt:
//
//	21:57:21 Requesting updated subscription list
//	21:57:22 Unsubscribed from item 2572368517
//	21:57:22 Detected workshop change : added subscribed item 2572368517
//
// So the worker lets that refresh land before it acts (settle), and afterwards watches each
// accepted item for a while, asking once more for any the refresh turned back (holdChanges).
const (
	settleTime = 1500 * time.Millisecond
	holdTime   = 2500 * time.Millisecond
)

// settle runs Steam's callbacks for d, so the subscription refresh that starting the API
// set off is applied before the worker reads or changes anything.
func settle(callbacks func(), clock watchClock, d time.Duration) {
	for end := clock.now().Add(d); clock.now().Before(end); {
		callbacks()
		clock.sleep(100 * time.Millisecond)
	}
	callbacks()
}

// holdAPI is what holdChanges needs from the Steam API.
type holdAPI struct {
	state     func(id uint64) uint32
	again     func(id uint64) // the same call once more
	callbacks func()
}

// holdChanges watches the items Steam accepted (ok) for d and repeats the call once for an
// item whose subscribed state went back; it reports which items were asked again.
func holdChanges(ids []uint64, ok []bool, subscribe bool, api holdAPI, clock watchClock, d time.Duration) []bool {
	asked := make([]bool, len(ids))
	for end := clock.now().Add(d); ; {
		api.callbacks()
		for k, id := range ids {
			if !ok[k] || asked[k] {
				continue
			}
			if (api.state(id)&ItemSubscribed != 0) != subscribe {
				api.again(id)
				asked[k] = true
				// Steam answers the repeated call before the process goes
				if e := clock.now().Add(time.Second); e.After(end) {
					end = e
				}
			}
		}
		if !clock.now().Before(end) {
			return asked
		}
		clock.sleep(100 * time.Millisecond)
	}
}
