//go:build !windows || !amd64

package steamworks

import (
	"fmt"
	"io"
)

// Worker: the Steam client calls are Windows-only for now.
func Worker(action string, ids []uint64, w io.Writer) int {
	fmt.Fprintln(w, ErrorLine("subscribing through Steam is only done on Windows"))
	return 3
}
