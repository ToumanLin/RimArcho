package updatechannel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/lanzou"
	"github.com/ToumanLin/RimArcho/internal/selfupdate"
)

func TestDownloadExactGitHubPairAndNoVersionSubstitution(t *testing.T) {
	for _, mode := range []string{"ok", "wrong hash", "different version", "missing checksum", "fallback failure"} {
		t.Run(mode, func(t *testing.T) {
			data := []byte("synthetic ZIP bytes; never extracted or executed")
			sum := sha256.Sum256(data)
			const name = "archo-1.2.3-windows-amd64.zip"
			digest := hex.EncodeToString(sum[:])
			if mode == "wrong hash" {
				digest = strings.Repeat("0", 64)
			}
			var origin string
			zipHits := 0
			metadataHits := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/releases/tags/v1.2.3":
					metadataHits++
					if mode == "fallback failure" {
						http.Error(w, "offline", 503)
						return
					}
					version := "1.2.3"
					if mode == "different version" {
						version = "9.9.9"
					}
					assets := []map[string]any{{"name": name, "browser_download_url": origin + "/zip", "size": len(data)}}
					if mode != "missing checksum" {
						assets = append(assets, map[string]any{"name": name + ".sha256", "browser_download_url": origin + "/sum", "size": len(digest) + len(name) + 3})
					}
					ioJSON(w, map[string]any{"tag_name": "v" + version, "assets": assets})
				case "/zip":
					zipHits++
					w.Write(data)
				case "/sum":
					fmt.Fprintf(w, "%s  %s\n", digest, name)
				default:
					t.Errorf("unexpected endpoint: %s", r.URL.Path)
				}
			}))
			defer server.Close()
			origin = server.URL
			options := Options{Source: "github", GitHub: selfupdate.Client{HTTP: server.Client(), API: origin, GOOS: "windows", GOARCH: "amd64"}, Lanzou: &lanzou.Client{ShareURL: "invalid"}}
			if mode == "fallback failure" {
				options.Source = "auto"
			}
			dir := t.TempDir()
			download, err := Download(context.Background(), options, "1.2.3", dir)
			if mode == "ok" {
				if err != nil || download.Source != "github" {
					t.Fatal(err)
				}
				got, _ := os.ReadFile(download.Path)
				if string(got) != string(data) {
					t.Fatal("wrong bytes")
				}
				if err := download.Close(); err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				download.Close()
				t.Fatal("invalid pair installed")
			}
			if mode == "fallback failure" && !strings.Contains(err.Error(), "Lanzou:") {
				t.Fatal("Lanzou fallback was not attempted")
			}
			if metadataHits != 1 || (mode == "different version" && zipHits != 0) {
				t.Fatal("download changed the checked version")
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 0 {
				t.Fatal("failed download or Close left staging files")
			}
		})
	}
}

func TestManualPairVerification(t *testing.T) {
	dir := t.TempDir()
	name := "archo-1.2.3-windows-amd64.zip"
	zipPath := filepath.Join(dir, "browser (1).zip")
	data := []byte("synthetic bytes")
	sum := sha256.Sum256(data)
	checksum := []byte(hex.EncodeToString(sum[:]) + "  " + name + "\n")
	if err := os.WriteFile(zipPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".sha256"), checksum, 0600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyLocal(context.Background(), "1.2.3", zipPath, "", int64(len(data))); err != nil {
		t.Fatal(err)
	}
	if err := VerifyWithChecksum(context.Background(), "1.2.3", zipPath, checksum, 0); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		version string
		sum     []byte
		size    int64
	}{
		{"1.2.4", checksum, 0}, {"1.2.3", checksum, int64(len(data) + 1)}, {"1.2.3", []byte(strings.Repeat("0", 64)), 0}, {"1.2.3", []byte(strings.Repeat("x", lanzou.MaxChecksum+1)), 0},
	} {
		if err := VerifyWithChecksum(context.Background(), test.version, zipPath, test.sum, test.size); err == nil {
			t.Fatal("invalid local pair accepted")
		}
	}
	if err := VerifyLocal(context.Background(), "1.2.3", zipPath, dir, 0); err == nil {
		t.Fatal("checksum directory accepted")
	}
	got, _ := os.ReadFile(zipPath)
	if string(got) != string(data) {
		t.Fatal("manual verification changed selected file")
	}
}
