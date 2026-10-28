// Package perftrace records opt-in startup spans to the captured process log.
package perftrace

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

var enabled = os.Getenv("RIMARCHO_PERF") != ""
var origin = time.Now()

func Mark(name string) {
	if enabled {
		fmt.Fprintf(os.Stderr, "F4 %s\n", mustJSON(map[string]any{"name": name, "at": time.Now().UnixMicro(), "sinceInitMs": float64(time.Since(origin).Microseconds()) / 1000}))
	}
}
func Span(name string) func() {
	if !enabled {
		return func() {}
	}
	start := time.Now()
	return func() {
		fmt.Fprintf(os.Stderr, "F4 %s\n", mustJSON(map[string]any{"name": name, "at": start.UnixMicro(), "ms": float64(time.Since(start).Microseconds()) / 1000}))
	}
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func init()                 { Mark("go-init") }
