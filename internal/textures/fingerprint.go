package textures

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
)

// Fingerprint checks the mod's texture paths, sizes and mtimes without opening PNGs
// (10 §2). Additions, deletions and overwrites invalidate page summaries, even before
// the next library scan. Conversion always collects and validates the individual files.
func Fingerprint(ctx context.Context, root string) ([32]byte, error) {
	set, err := collect(ctx, root, AllFolders(root), func(string, os.FileInfo) (PNGHeader, bool) { return PNGHeader{}, false })
	if err != nil {
		return [32]byte{}, err
	}
	h := sha256.New()
	for _, f := range set.Others {
		fmt.Fprintf(h, "%q %q %d %d %d\n", f.Rel, f.Ext, f.Prio, f.Size, f.MTime)
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}
