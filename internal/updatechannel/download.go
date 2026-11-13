package updatechannel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ToumanLin/RimArcho/internal/lanzou"
	"github.com/ToumanLin/RimArcho/internal/selfupdate"
)

type Downloaded struct {
	Path, Source string
	GitHubError  string // Safe fallback reason, with no URLs/tokens.
	dir          string
}

func (d *Downloaded) Close() error { return os.RemoveAll(d.dir) }

// Download tries the existing GitHub pair, then the Lanzou pair, for precisely
// the checked version. Exhaustion leaves the browser/local-pair fallback to callers.
// It never extracts, replaces or executes anything.
func Download(ctx context.Context, options Options, version, dir string) (result *Downloaded, err error) {
	if !validVersion(version, options.Prerelease) {
		return nil, errors.New("invalid checked update version")
	}
	if options.Source != "" && options.Source != "auto" && options.Source != "github" {
		return nil, errors.New("unknown update source setting")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	stage, err := os.MkdirTemp(dir, "rimarcho-update-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		if result == nil {
			err = errors.Join(err, os.RemoveAll(stage))
		}
	}()
	github := options.GitHub
	if github.GOOS == "" {
		github.GOOS = "windows"
	}
	if github.GOARCH == "" {
		github.GOARCH = "amd64"
	}
	client, githubErr := shortHTTP(github.HTTP)
	if githubErr == nil {
		defer client.CloseIdleConnections()
		github.HTTP = client
		var release *selfupdate.Release
		if options.Prerelease {
			release, githubErr = github.Latest(ctx, true)
		} else {
			release, githubErr = github.Version(ctx, version)
		}
		if githubErr == nil && (release == nil || release.Version != version) {
			githubErr = errors.New("GitHub no longer offers the checked version")
		}
		if githubErr == nil {
			var path string
			path, githubErr = github.Download(ctx, release, stage, nil)
			if githubErr == nil {
				return &Downloaded{Path: path, Source: "github", dir: stage}, nil
			}
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if options.Source == "github" || options.Prerelease {
		return nil, errors.New("GitHub could not download and verify the checked release pair")
	}
	c := options.Lanzou
	if c == nil {
		c = &lanzou.Client{ShareURL: lanzou.DefaultShareURL, Password: lanzou.DefaultPassword}
	}
	pair, lanzouErr := c.Download(ctx, version, stage)
	if lanzouErr != nil {
		return nil, fmt.Errorf("GitHub release pair failed; Lanzou: %w; download the exact ZIP and its .sha256 in your browser, then select them", lanzouErr)
	}
	return &Downloaded{Path: pair.ZipPath, Source: "lanzou", GitHubError: "GitHub could not download and verify the checked release pair", dir: stage}, nil
}

// VerifyLocal accepts an explicit browser-downloaded sidecar or looks beside the
// selected ZIP. The filename inside the checksum must name the checked release;
// the ZIP itself may have been renamed by the browser. No input file is changed.
func VerifyLocal(ctx context.Context, version, zipPath, checksumPath string, size int64) error {
	if !validVersion(version, true) {
		return errors.New("invalid checked update version")
	}
	name := selfupdate.ZipName(version, "windows", "amd64")
	if checksumPath == "" {
		checksumPath = zipPath + ".sha256"
		if _, err := os.Lstat(checksumPath); errors.Is(err, os.ErrNotExist) {
			checksumPath = filepath.Join(filepath.Dir(zipPath), name+".sha256")
		}
	}
	info, err := os.Lstat(checksumPath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > lanzou.MaxChecksum {
		return errors.New("checksum must be a regular file of at most 64 KiB")
	}
	f, err := os.Open(checksumPath)
	if err != nil {
		return err
	}
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > lanzou.MaxChecksum {
		f.Close()
		return errors.New("checksum must be a regular file of at most 64 KiB")
	}
	sum, readErr := readChecksum(f)
	if err := errors.Join(readErr, f.Close()); err != nil {
		return err
	}
	return VerifyWithChecksum(ctx, version, zipPath, sum, size)
}

// VerifyWithChecksum also accepts a sidecar fetched via Lanzou.Client.Checksum.
// size may be zero when only the same-source checksum supplies integrity, as in
// the existing Workshop path. Reverify a private staged copy before extraction.
func VerifyWithChecksum(ctx context.Context, version, path string, sum []byte, size int64) error {
	if !validVersion(version, true) || len(sum) > lanzou.MaxChecksum || size < 0 || size > lanzou.MaxArchive {
		return errors.New("invalid local release metadata")
	}
	return selfupdate.VerifyLocal(ctx, path, selfupdate.ZipName(version, "windows", "amd64"), sum, size)
}
