package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const maxDownload = 200 << 20

var ErrChecksum = errors.New("the download does not match its checksum")

// ParseSum finds a sha256sum entry by basename, or accepts one bare hash.
func ParseSum(b []byte, name string) (string, error) {
	var lines []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	for _, line := range lines {
		if len(line) < 64 {
			continue
		}
		hash := line[:64]
		if _, err := hex.DecodeString(hash); err != nil {
			continue
		}
		if len(lines) == 1 && len(line) == 64 {
			return strings.ToLower(hash), nil
		}
		if len(line) < 67 || (line[64:66] != "  " && line[64:66] != " *") {
			continue
		}
		entry := path.Base(strings.ReplaceAll(line[66:], `\`, "/"))
		if strings.EqualFold(entry, name) {
			return strings.ToLower(hash), nil
		}
	}
	return "", fmt.Errorf("checksum file has no valid sha256 for %q", name)
}

// Download verifies a cached zip or streams a new one into an atomic .part file.
func (c *Client) Download(ctx context.Context, r *Release, dir string, progress func(done, total int64)) (result string, err error) {
	if r == nil || !safeAssetName(r.Zip.Name) {
		return "", errors.New("release has an invalid zip name")
	}
	zipPath := filepath.Join(dir, r.Zip.Name)
	part := zipPath + ".part"
	defer func() {
		if err != nil {
			for _, name := range []string{part, zipPath} {
				if e := os.Remove(name); e != nil && !errors.Is(e, os.ErrNotExist) {
					err = errors.Join(err, e)
				}
			}
		}
	}()
	if err := r.Installable(); err != nil {
		return "", err
	}
	if r.Zip.Size > maxDownload {
		return "", errors.New("release zip exceeds 200 mb")
	}
	resp, err := c.get(ctx, c.metadataClient(), r.Sum.URL)
	if err != nil {
		return "", fmt.Errorf("checksum download: %w", err)
	}
	b, readErr := readLimited(resp.Body, 64<<10)
	closeErr := resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksum download: HTTP %d", resp.StatusCode)
	}
	if err := errors.Join(readErr, closeErr); err != nil {
		return "", fmt.Errorf("checksum download: %w", err)
	}
	want, err := ParseSum(b, r.Zip.Name)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if info, err := os.Lstat(zipPath); err == nil && info.Mode().IsRegular() && info.Size() <= maxDownload {
		hash, size, err := hashFile(ctx, zipPath)
		if err != nil {
			return "", err
		}
		if hash == want && (r.Zip.Size <= 0 || size == r.Zip.Size) {
			if err := removeIfExists(part); err != nil {
				return "", err
			}
			if progress != nil {
				progress(size, max(r.Zip.Size, 0))
			}
			return zipPath, nil
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := removeIfExists(zipPath); err != nil {
		return "", err
	}
	if err := removeIfExists(part); err != nil {
		return "", err
	}
	// Preserve the caller's transport, cookies and redirects, but not its metadata timeout.
	client := http.Client{}
	if c.HTTP != nil {
		client = *c.HTTP
	}
	client.Timeout = 0
	resp, err = c.get(ctx, &client, r.Zip.URL)
	if err != nil {
		return "", fmt.Errorf("zip download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("zip download: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxDownload {
		return "", errors.New("release zip exceeds 200 mb")
	}
	f, err := os.OpenFile(part, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	done, copyErr := copyDownload(ctx, io.MultiWriter(f, h), resp.Body, max(r.Zip.Size, 0), progress)
	closeErr = f.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return "", err
	}
	if r.Zip.Size > 0 && done != r.Zip.Size {
		return "", fmt.Errorf("zip size mismatch: got %d, want %d", done, r.Zip.Size)
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return "", fmt.Errorf("zip verification: %w", ErrChecksum)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.Rename(part, zipPath); err != nil {
		return "", err
	}
	return zipPath, nil
}

func safeAssetName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\:`) &&
		!strings.HasSuffix(name, ".") && !strings.HasSuffix(name, " ")
}

func removeIfExists(name string) error {
	err := os.Remove(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func hashFile(ctx context.Context, name string) (string, int64, error) {
	f, err := os.Open(name)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := copyDownload(ctx, h, f, 0, nil)
	return hex.EncodeToString(h.Sum(nil)), n, err
}

func copyDownload(ctx context.Context, w io.Writer, r io.Reader, total int64, progress func(int64, int64)) (int64, error) {
	buf := make([]byte, 64<<10)
	var done, reported int64
	for {
		if err := ctx.Err(); err != nil {
			return done, err
		}
		n, readErr := r.Read(buf)
		if done+int64(n) > maxDownload {
			return done, errors.New("release zip exceeds 200 mb")
		}
		if n > 0 {
			written, err := w.Write(buf[:n])
			done += int64(written)
			if err != nil {
				return done, err
			}
			if written != n {
				return done, io.ErrShortWrite
			}
			if progress != nil && done-reported >= 64<<10 {
				progress(done, total)
				reported = done
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				return done, readErr
			}
			if err := ctx.Err(); err != nil {
				return done, err
			}
			if progress != nil {
				progress(done, total)
			}
			return done, nil
		}
	}
}
