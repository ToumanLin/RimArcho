package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseSum(t *testing.T) {
	hash := strings.Repeat("aB", 32)
	for _, text := range []string{
		hash + "  archive.zip\n", hash + " *archive.zip\n", hash + "  path/to/ARCHIVE.ZIP\r\n",
		hash + ` *C:\release\Archive.zip`, hash, "\n# ignored\n" + hash + "\n\n",
		strings.Repeat("0", 64) + "  other.zip\n" + hash + "  archive.zip\n",
	} {
		got, err := ParseSum([]byte(text), "archive.zip")
		if err != nil || got != strings.ToLower(hash) {
			t.Errorf("ParseSum(%q) = %q, %v", text, got, err)
		}
	}
	for _, text := range []string{
		hash + "  wrong.zip", strings.Repeat("g", 64) + "  archive.zip", "abc  archive.zip", "",
		hash + " archive.zip", hash + "\n" + hash + "  other.zip",
	} {
		if _, err := ParseSum([]byte(text), "archive.zip"); err == nil {
			t.Errorf("accepted %q", text)
		}
	}
}

func sumOf(b []byte) string {
	hash := sha256.Sum256(b)
	return hex.EncodeToString(hash[:])
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDownload(t *testing.T) {
	data := makeZip(t, []zipEntry{{name: Files[0], data: strings.Repeat("z", 140<<10)}})
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sum":
			_, _ = fmt.Fprintf(w, "%s  archive.zip\n", sumOf(data))
		case "/zip":
			hits.Add(1)
			_, _ = w.Write(data)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := server.Client()
	base := client.Transport
	client.Timeout = 15 * time.Second
	client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if _, deadline := r.Context().Deadline(); r.URL.Path == "/zip" && deadline {
			t.Error("zip request inherited metadata timeout")
		}
		return base.RoundTrip(r)
	})
	c := Client{HTTP: client}
	r := &Release{Zip: Asset{Name: "archive.zip", URL: server.URL + "/zip", Size: int64(len(data))}, Sum: Asset{Name: "archive.zip.sha256", URL: server.URL + "/sum"}}
	dir := t.TempDir()
	var calls [][2]int64
	path, err := c.Download(context.Background(), r, dir, func(done, total int64) { calls = append(calls, [2]int64{done, total}) })
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, r.Zip.Name) {
		t.Fatalf("wrong path %q", path)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, data) || sumOf(got) != sumOf(data) {
		t.Fatalf("wrong zip: %v", err)
	}
	if _, err := os.Stat(path + ".part"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("part still exists: %v", err)
	}
	if len(calls) == 0 || calls[len(calls)-1] != [2]int64{int64(len(data)), r.Zip.Size} {
		t.Fatalf("wrong final progress: %v", calls)
	}
	var previous int64
	for i, call := range calls {
		if call[1] != r.Zip.Size || call[0] < previous || (i < len(calls)-1 && call[0]-previous < 64<<10) {
			t.Fatalf("progress was not throttled: %v", calls)
		}
		previous = call[0]
	}
	if _, err := c.Download(context.Background(), r, dir, nil); err != nil || hits.Load() != 1 {
		t.Fatalf("cache reuse: %v; zip hits %d", err, hits.Load())
	}
	// A corrupt cached file must be replaced rather than reused.
	if err := os.WriteFile(path, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Download(context.Background(), r, dir, nil); err != nil || hits.Load() != 2 {
		t.Fatalf("corrupt cache: %v; zip hits %d", err, hits.Load())
	}
}

func TestDownloadFailures(t *testing.T) {
	data := makeZip(t, []zipEntry{{name: Files[0], data: strings.Repeat("z", 140<<10)}})
	for _, mode := range []string{"hash", "interrupted", "size", "sum http", "zip http", "sum too large", "zip too large", "declared too large", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/sum" {
					if mode == "sum http" {
						w.WriteHeader(403)
						return
					}
					if mode == "sum too large" {
						_, _ = w.Write(bytes.Repeat([]byte("x"), (64<<10)+1))
						return
					}
					hash := sumOf(data)
					if mode == "hash" {
						hash = strings.Repeat("0", 64)
					}
					_, _ = fmt.Fprintf(w, "%s *archive.zip\n", hash)
					return
				}
				if mode == "zip http" {
					w.WriteHeader(500)
					return
				}
				if mode == "zip too large" {
					w.Header().Set("Content-Length", strconv.Itoa(maxDownload+1))
					return
				}
				if mode == "interrupted" {
					w.Header().Set("Content-Length", strconv.Itoa(len(data)))
					_, _ = w.Write(data[:len(data)/2])
					w.(http.Flusher).Flush()
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					_ = conn.Close()
					return
				}
				_, _ = w.Write(data)
			}))
			defer server.Close()
			c := Client{HTTP: server.Client()}
			r := &Release{Zip: Asset{Name: "archive.zip", URL: server.URL + "/zip", Size: int64(len(data))}, Sum: Asset{Name: "archive.zip.sha256", URL: server.URL + "/sum"}}
			if mode == "size" {
				r.Zip.Size++
			}
			if mode == "declared too large" {
				r.Zip.Size = maxDownload + 1
			}
			dir := t.TempDir()
			// Failures must also remove stale partial and invalid final files.
			for _, suffix := range []string{"", ".part"} {
				if err := os.WriteFile(filepath.Join(dir, r.Zip.Name+suffix), []byte("stale"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := c.Download(ctx, r, dir, func(done, total int64) {
				if mode == "cancel" {
					cancel()
				}
			})
			if err == nil {
				t.Fatal("expected failure")
			}
			if mode == "hash" && !errors.Is(err, ErrChecksum) {
				t.Fatalf("got %v, want ErrChecksum", err)
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("got %v, want cancellation", err)
			}
			entries, readErr := os.ReadDir(dir)
			if readErr != nil || len(entries) != 0 {
				t.Fatalf("download left files: %v, %v", entries, readErr)
			}
		})
	}
}

func TestDownloadUnknownSize(t *testing.T) {
	data := []byte("an archive need not be parsed during download")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sum" {
			_, _ = io.WriteString(w, sumOf(data))
		} else {
			_, _ = w.Write(data)
		}
	}))
	defer server.Close()
	c := Client{HTTP: server.Client()}
	r := &Release{Zip: Asset{Name: "archive.zip", URL: server.URL + "/zip"}, Sum: Asset{Name: "archive.zip.sha256", URL: server.URL + "/sum"}}
	var calls int
	_, err := c.Download(context.Background(), r, t.TempDir(), func(done, total int64) {
		calls++
		if done != int64(len(data)) || total != 0 {
			t.Errorf("wrong progress %d/%d", done, total)
		}
	})
	if err != nil || calls != 1 {
		t.Fatalf("unknown size: %v; %d callbacks", err, calls)
	}
}
