//go:build !windows || !amd64

package steamworks

import (
	"fmt"
	"io"
)

func PublishWorker(r PublishRequest, w io.Writer) int {
	fmt.Fprintln(w, ErrorLine("Workshop publishing requires Windows amd64"))
	return 3
}
