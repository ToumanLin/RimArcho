package updatefeed

import (
	"context"
	"fmt"

	"github.com/ToumanLin/RimArcho/internal/selfupdate"
)

// VerifyLocal checks a player-selected archive against authenticated metadata.
// The caller must re-parse the feed for freshness before calling this method and
// verify the private staged copy again before extraction (the input may change).
// This method neither modifies the selected file nor installs it.
func (f *Feed) VerifyLocal(ctx context.Context, path string) error {
	a := f.manifest.Artifact
	checksum := []byte(fmt.Sprintf("%s  %s\n", a.SHA256, a.Name))
	return selfupdate.VerifyLocal(ctx, path, a.Name, checksum, a.Size)
}
