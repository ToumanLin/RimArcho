package lanzou

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

const MaxArchive = 200 << 20
const MaxChecksum = 64 << 10

// A transfer is private until its caller verifies the checksum or reports a probe observation.
type transfer struct {
	file   *os.File
	SHA256 string
	Size   int64
	URL    string
}

func (a *transfer) Close() error { return errors.Join(a.file.Close(), os.Remove(a.file.Name())) }
func (r *runner) transfer(ctx context.Context, target, dir string, limit int64) (result *transfer, err error) {
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
	if resp.ContentLength > limit {
		return nil, errors.New("download exceeds byte limit")
	}
	file, err := os.CreateTemp(dir, "lanzou-*.part")
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
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return &transfer{file: file, SHA256: sum, Size: n, URL: resp.Request.URL.String()}, nil
}
