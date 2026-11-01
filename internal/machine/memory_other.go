//go:build !windows

package machine

import "time"

func Read() Memory {
	return Memory{SampledAt: time.Now(), Error: "hardware memory probe is available on Windows only"}
}
