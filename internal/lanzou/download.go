package lanzou

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ToumanLin/RimArcho/internal/selfupdate"
)

// Package holds a release ZIP in private staging. Close removes staging.
type Package struct {
	ZipPath string
	Size    int64
	SHA256  string
	dir     string
}

func (p *Package) Close() error { return os.RemoveAll(p.dir) }

// Download resolves only the exact ZIP. An optional GitHub checksum verifies
// the staged copy. Extraction must check ZIP CRCs before installation (17 §5.1).
func (c Client) Download(ctx context.Context, version, dir string, sum []byte) (result *Package, err error) {
	name := selfupdate.ZipName(version, "windows", "amd64")
	if stableFilename(name) != version || version == "" {
		return nil, errors.New("Lanzou downloads require an exact stable version")
	}
	if len(sum) > MaxChecksum {
		return nil, errors.New("checksum exceeds 64 KiB")
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	r, share, err := c.runner()
	if err != nil {
		return nil, err
	}
	defer r.client.CloseIdleConnections()
	listing, err := r.list(ctx, share, c.Password)
	if err != nil {
		return nil, err
	}
	zipID, err := listing.find(name)
	if err != nil {
		return nil, err
	}
	stage, err := os.MkdirTemp(dir, "lanzou-release-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		if result == nil {
			err = errors.Join(err, os.RemoveAll(stage))
		}
	}()
	target, err := r.resolve(ctx, share, zipID)
	if err != nil {
		return nil, err
	}
	zip, err := r.transfer(ctx, target, stage, MaxArchive)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, zip.Close())
		if err != nil && result != nil {
			err = errors.Join(err, result.Close())
			result = nil
		}
	}()
	zipPath := filepath.Join(stage, name)
	out, err := os.OpenFile(zipPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	_, copyErr := io.Copy(out, zip.file)
	if err := errors.Join(copyErr, out.Close()); err != nil {
		return nil, err
	}
	if sum != nil {
		if err := selfupdate.VerifyLocal(ctx, zipPath, name, sum, zip.Size); err != nil {
			return nil, err
		}
	}
	return &Package{ZipPath: zipPath, Size: zip.Size, SHA256: zip.SHA256, dir: stage}, nil
}
