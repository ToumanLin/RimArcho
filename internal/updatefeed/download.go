package updatefeed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Archive becomes readable only after the entire signed size and digest match.
// Close deletes its temporary file. It is not an installation or ZIP-layout check.
type Archive struct {
	file   *os.File
	SHA256 string
	Size   int64
	URL    string // Ephemeral bearer URL: do not persist or log it.
}

func (a *Archive) Read(p []byte) (int, error)                   { return a.file.Read(p) }
func (a *Archive) Seek(offset int64, whence int) (int64, error) { return a.file.Seek(offset, whence) }
func (a *Archive) Close() error                                 { return errors.Join(a.file.Close(), os.Remove(a.file.Name())) }

// Download runs one authenticated source by index. The caller controls fallback order
// and provides a private quarantine directory. Custom RoundTrippers are rejected;
// standard transports are cloned, with fresh cookies and direct, pinned public DNS.
func (f *Feed) Download(ctx context.Context, client *http.Client, index int, dir string) (*Archive, error) {
	return f.download(ctx, client, index, dir, "")
}
func (f *Feed) download(ctx context.Context, client *http.Client, index int, dir, testOrigin string) (*Archive, error) {
	if index < 0 || index >= len(f.manifest.Sources) {
		return nil, errors.New("source index is out of range")
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	r, err := newRunner(client, f.manifest.Sources[index], testOrigin)
	if err != nil {
		return nil, err
	}
	defer r.client.CloseIdleConnections()
	target, err := r.resolve(ctx, f.manifest.Artifact.Name)
	if err != nil {
		return nil, err
	}
	return r.download(ctx, target, f.manifest.Artifact, dir)
}

func (r *runner) download(ctx context.Context, target string, a Artifact, dir string) (result *Archive, err error) {
	return r.transfer(ctx, target, &a, dir)
}

// A nil artifact is used only by the diagnostic probe to measure untrusted bytes.
// Feed.Download always supplies authenticated metadata.
func (r *runner) transfer(ctx context.Context, target string, a *Artifact, dir string) (result *Archive, err error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	// At most one fixed challenge retry per download host, within the shared request budget.
	challenged := map[string]bool{}
	var resp *http.Response
	for {
		resp, err = r.send(ctx, target, "GET", nil, http.Header{})
		if err != nil {
			return nil, err
		}
		if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
			break
		}
		b, e := r.metadata(resp)
		if e != nil {
			return nil, e
		}
		host := resp.Request.URL.Hostname()
		if strings.Contains(string(b), "down_r") {
			r.report("human-verification-required", host, resp.StatusCode)
			return nil, errors.New("Lanzou requires human verification in a browser; this source failed")
		}
		if challenged[host] || !r.challenge(resp, b) {
			return nil, errors.New("download node verification failed: HTML instead of an archive")
		}
		challenged[host] = true
		target = resp.Request.URL.String()
	}
	defer resp.Body.Close()
	if encoding := resp.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return nil, errors.New("archive content encoding is unsupported")
	}
	if a != nil && resp.ContentLength >= 0 && resp.ContentLength != a.Size {
		return nil, errors.New("archive Content-Length differs from signed size")
	}
	limit := int64(MaxArchive)
	if a != nil {
		limit = a.Size
	}
	file, err := os.CreateTemp(dir, "updatefeed-*.part")
	if err != nil {
		return nil, err
	}
	defer func() {
		if result == nil {
			err = errors.Join(err, file.Close(), os.Remove(file.Name()))
		}
	}()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	sum := hex.EncodeToString(hash.Sum(nil))
	if n <= 0 || n > limit {
		return nil, errors.New("archive is empty or exceeds byte limit")
	}
	if a != nil && (n != a.Size || sum != a.SHA256) {
		return nil, errors.New("archive differs from signed size or SHA-256")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return &Archive{file: file, SHA256: sum, Size: n, URL: resp.Request.URL.String()}, nil
}
