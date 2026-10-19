// Baseline: process memory of an otherwise empty Go program, with and without the sqlite import.
package main

import (
	"fmt"

	"github.com/rimarcho/rimarcho/spikes/internal/procmem"
)

func main() {
	ws, priv, _ := procmem.MB()
	fmt.Printf("empty go program: working set %.1f MB, private %.1f MB\n", ws, priv)
}
