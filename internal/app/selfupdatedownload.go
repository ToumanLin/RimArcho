package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/ToumanLin/RimArcho/internal/lanzou"
	"github.com/ToumanLin/RimArcho/internal/selfupdate"
	"github.com/ToumanLin/RimArcho/internal/updatechannel"
)

// ErrSelfDownloadManual means automatic sources could not supply the exact ZIP.
var ErrSelfDownloadManual = errors.New("automatic update download failed")

func mirrorSelfRelease(version string) *selfupdate.Release {
	return &selfupdate.Release{Version: version, Tag: "v" + version, Mirror: true,
		Page: "https://github.com/ToumanLin/RimArcho/releases/tag/v" + version,
		Zip:  selfupdate.Asset{Name: selfupdate.ZipName(version, "windows", "amd64")}}
}

func (a *App) selfLanzou() updatechannel.LanzouClient {
	if a.SelfLanzou != nil {
		return a.SelfLanzou
	}
	return &lanzou.Client{ShareURL: lanzou.DefaultShareURL, Password: lanzou.DefaultPassword}
}

// SelfUpdateManualInstructions is shared by CLI and surfaces offering a browser download.
func SelfUpdateManualInstructions(version string) string {
	return fmt.Sprintf("open %s (password %s) in a browser, download %s, then run `archo update --from <path>`",
		lanzou.DefaultShareURL, lanzou.DefaultPassword, selfupdate.ZipName(version, "windows", "amd64"))
}

func (a *App) downloadSelfUpdate(ctx context.Context, r *selfupdate.Release, dir string, step func(SelfUpdateStep)) (string, error) {
	// No overall deadline: a slow GitHub download (through a mirror) may take long and
	// can be stopped; Lanzou bounds its own requests (17 §5.1).
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	client := a.selfClient()
	release, githubErr := client.Version(ctx, r.Version)
	var sum []byte
	if githubErr == nil {
		// Retain an obtained checksum even when the ZIP request fails.
		sumCtx, stop := context.WithTimeout(ctx, 15*time.Second)
		var sumErr error
		sum, sumErr = client.Checksum(sumCtx, release)
		stop()
		githubErr = sumErr
		if githubErr == nil {
			step(SelfUpdateStep{Phase: "download", Total: release.Zip.Size})
			var path string
			path, githubErr = client.DownloadWithChecksum(ctx, release, dir, sum, func(done, total int64) {
				step(SelfUpdateStep{Phase: "download", Done: done, Total: total})
			})
			if githubErr == nil {
				return path, nil
			}
		}
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	lanzouErr := errors.New("Lanzou is skipped for GitHub-only updates")
	if a.Store.Meta("cfg:updates.source") != "github" && !selfPrerelease(a.SelfVersionNow()) {
		step(SelfUpdateStep{Phase: "download"})
		var archive *lanzou.Package
		archive, lanzouErr = a.selfLanzou().Download(ctx, r.Version, dir, sum)
		if lanzouErr == nil {
			// The resolver owns a subdirectory in download/, kept while waiting.
			return archive.ZipPath, nil
		}
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	s := a.SelfUpdateInfo()
	s.Manual = r.Version
	b, _ := json.Marshal(s)
	metaErr := a.Store.SetMeta("self-update", string(b))
	return "", errors.Join(fmt.Errorf("%w for RimArcho %s; GitHub: %w; Lanzou: %w; %s",
		ErrSelfDownloadManual, r.Version, githubErr, lanzouErr, SelfUpdateManualInstructions(r.Version)), metaErr)
}

var manualArchiveName = regexp.MustCompile(`^archo-((?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*))-windows-amd64(?: \([0-9]+\))?\.zip$`)

func manualSelfVersion(name string) string {
	if m := manualArchiveName.FindStringSubmatch(name); m != nil {
		return m[1]
	}
	return ""
}

// selfChecksum never requests a different version, and bounds metadata + sidecar
// together to fifteen seconds. Failure leaves extraction's CRC checks (17 §5.1).
func (a *App) selfChecksum(ctx context.Context, version string) []byte {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client := a.selfClient()
	r, err := client.Version(ctx, version)
	if err != nil {
		return nil
	}
	b, _ := client.Checksum(ctx, r)
	return b
}

func readSelfSidecar(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > lanzou.MaxChecksum {
		return nil, errors.New("checksum must be a regular file of at most 64 KiB")
	}
	b, err := io.ReadAll(io.LimitReader(f, lanzou.MaxChecksum+1))
	if len(b) > lanzou.MaxChecksum {
		return nil, errors.New("checksum exceeds 64 KiB")
	}
	return b, err
}

func (a *App) stageSelfUpdate(ctx context.Context, version, picked, dir string, step func(SelfUpdateStep)) (result string, err error) {
	var sum []byte
	name := selfupdate.ZipName(version, "windows", "amd64")
	for _, path := range []string{picked + ".sha256", filepath.Join(filepath.Dir(picked), name+".sha256")} {
		sum, err = readSelfSidecar(path)
		if !errors.Is(err, os.ErrNotExist) {
			if err != nil {
				return "", err
			}
			break
		}
	}
	if sum == nil {
		sum = a.selfChecksum(ctx, version)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	src, err := os.Open(picked)
	if err != nil {
		return "", err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > lanzou.MaxArchive {
		return "", errors.New("release ZIP must be a regular file of at most 200 MiB")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "manual-*.zip")
	if err != nil {
		return "", err
	}
	defer func() {
		if result == "" {
			_ = os.Remove(f.Name())
		}
	}()
	step(SelfUpdateStep{Phase: "download", Total: info.Size()})
	buf := make([]byte, 64<<10)
	var done int64
	for err == nil {
		if err = ctx.Err(); err != nil {
			break
		}
		var n int
		n, err = src.Read(buf)
		done += int64(n)
		if done > lanzou.MaxArchive {
			err = errors.New("release ZIP exceeds 200 MiB")
			break
		}
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				err = writeErr
				break
			}
			step(SelfUpdateStep{Phase: "download", Done: done, Total: info.Size()})
		}
	}
	if errors.Is(err, io.EOF) {
		err = nil
	}
	if err = errors.Join(err, f.Close(), ctx.Err()); err != nil {
		return "", err
	}
	if sum != nil {
		if err := selfupdate.VerifyLocal(ctx, f.Name(), name, sum, 0); err != nil {
			return "", err
		}
	}
	return f.Name(), nil
}
