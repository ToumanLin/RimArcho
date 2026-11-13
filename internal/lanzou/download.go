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

// Package holds the existing release pair in private staging, verified with
// selfupdate.VerifyLocal. Close removes staging. Neither file is executed.
type Package struct {
	ZipPath, ChecksumPath string
	Size                  int64
	SHA256                string
	dir                   string
}

func (p *Package) Close() error { return os.RemoveAll(p.dir) }

// Download resolves both exact filenames afresh in one bounded session. A
// missing checksum, interactive verification, or mismatch fails this source.
func (c Client) Download(ctx context.Context, version, dir string) (result *Package, err error) {
	name := selfupdate.ZipName(version, "windows", "amd64")
	if stableFilename(name) != version || version == "" {
		return nil, errors.New("Lanzou downloads require an exact stable version")
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
	sumID, err := listing.find(name + ".sha256")
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
	sum, err := r.fetchChecksum(ctx, share, sumID, stage, name)
	if err != nil {
		return nil, err
	}
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
	if err := selfupdate.VerifyLocal(ctx, zip.file.Name(), name, sum, zip.Size); err != nil {
		return nil, err
	}
	zipPath := filepath.Join(stage, name)
	out, err := os.OpenFile(zipPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	_, copyErr := io.Copy(out, zip.file)
	if err := errors.Join(copyErr, out.Close()); err != nil {
		return nil, err
	}
	// Recheck the actual staged input, including its observed byte length.
	if err := selfupdate.VerifyLocal(ctx, zipPath, name, sum, zip.Size); err != nil {
		return nil, err
	}
	sumPath := filepath.Join(stage, name+".sha256")
	if err := os.WriteFile(sumPath, sum, 0600); err != nil {
		return nil, err
	}
	return &Package{ZipPath: zipPath, ChecksumPath: sumPath, Size: zip.Size, SHA256: zip.SHA256, dir: stage}, nil
}

// Checksum supports manual ZIP selection when the sidecar was not downloaded
// alongside it. It uses the same resolver and stops at any human check.
func (c Client) Checksum(ctx context.Context, version string) (sum []byte, err error) {
	name := selfupdate.ZipName(version, "windows", "amd64")
	if stableFilename(name) != version || version == "" {
		return nil, errors.New("checksum requires an exact stable version")
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
	id, err := listing.find(name + ".sha256")
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "rimarcho-lanzou-checksum-*")
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()
	return r.fetchChecksum(ctx, share, id, dir, name)
}

func (r *runner) fetchChecksum(ctx context.Context, share, id, dir, name string) (sum []byte, err error) {
	target, err := r.resolve(ctx, share, id)
	if err != nil {
		return nil, err
	}
	download, err := r.transfer(ctx, target, dir, MaxChecksum)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, download.Close()) }()
	sum, err = limited(download.file, MaxChecksum)
	if err != nil {
		return nil, err
	}
	if _, err := selfupdate.ParseSum(sum, name); err != nil {
		return nil, errors.New("Lanzou checksum has no valid entry for the checked ZIP")
	}
	return sum, nil
}
