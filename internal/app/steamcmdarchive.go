package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/deps"
)

// Valve's Linux bootstrap has nested linux32/ files. OpenRoot confines writes,
// including through pre-existing symlinks, to the managed installation directory.
func extractSteamCMDTar(b []byte, dir string) (string, error) {
	gz, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("SteamCMD download is not gzip: %w", err)
	}
	defer gz.Close()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	tr := tar.NewReader(gz)
	var total int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("SteamCMD tar: %w", err)
		}
		name := filepath.FromSlash(h.Name)
		if !filepath.IsLocal(name) || strings.ContainsAny(h.Name, `\:`) {
			return "", fmt.Errorf("invalid SteamCMD archive path %q", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(name, 0o755); err != nil {
				return "", err
			}
		case tar.TypeReg, tar.TypeRegA:
			total += h.Size
			if h.Size < 0 || total > 200<<20 {
				return "", errors.New("SteamCMD archive exceeds 200 MB")
			}
			if err := root.MkdirAll(filepath.Dir(name), 0o755); err != nil {
				return "", err
			}
			w, err := root.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
			if err != nil {
				return "", err
			}
			_, copyErr := io.Copy(w, tr)
			modeErr := w.Chmod(os.FileMode(h.Mode) & 0o777)
			closeErr := w.Close()
			if err := errors.Join(copyErr, modeErr, closeErr); err != nil {
				return "", err
			}
		default:
			return "", fmt.Errorf("unsupported SteamCMD archive entry %q", h.Name)
		}
	}
	for _, name := range []string{deps.SteamCMDExecutable(), "linux32/steamcmd"} {
		f, err := root.Open(name)
		if err != nil {
			return "", fmt.Errorf("SteamCMD archive missing %s: %w", name, err)
		}
		st, err := f.Stat()
		f.Close()
		if err != nil {
			return "", err
		}
		if !st.Mode().IsRegular() || st.Mode().Perm()&0o111 == 0 {
			return "", fmt.Errorf("SteamCMD archive has no executable %s", name)
		}
	}
	return filepath.Join(dir, deps.SteamCMDExecutable()), nil
}
