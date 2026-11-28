package machine

import (
	"strings"
	"sync"
)

// CPUName is the processor's model string ("AMD Ryzen 9 7950X 16-Core Processor"), or ""
// when the system does not say. The slow-check report sends it (16 §2.14).
func CPUName() string { return cpuOnce() }

var cpuOnce = sync.OnceValue(func() string { return strings.Join(strings.Fields(cpuName()), " ") })
