//go:build linux

package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractSteamCMDTar(t *testing.T) {
	for _, bad := range []string{"", "../outside", "/absolute", "symlink", "no-exec", "missing-native"} {
		t.Run(bad, func(t *testing.T) {
			var b bytes.Buffer
			gz := gzip.NewWriter(&b)
			tw := tar.NewWriter(gz)
			for _, name := range []string{"steamcmd.sh", "linux32/steamcmd"} {
				if bad == "missing-native" && name == "linux32/steamcmd" {
					continue
				}
				mode := int64(0o755)
				if bad == "no-exec" {
					mode = 0o644
				}
				data := "#!/bin/sh\nexit 0\n"
				if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(data))}); err != nil {
					t.Fatal(err)
				}
				if _, err := io.WriteString(tw, data); err != nil {
					t.Fatal(err)
				}
			}
			if bad == "../outside" || bad == "/absolute" {
				tw.WriteHeader(&tar.Header{Name: bad, Mode: 0o644})
			}
			if bad == "symlink" {
				tw.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../outside"})
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(b.Bytes()) }))
			defer srv.Close()
			resp, err := srv.Client().Get(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			data, err := readToolDownload(resp, nil)
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(t.TempDir(), "tool")
			exe, err := extractSteamCMDTar(data, dir)
			if bad != "" {
				if err == nil {
					t.Fatal("unsafe or incomplete archive accepted")
				}
				return
			}
			if err != nil || exe != filepath.Join(dir, "steamcmd.sh") {
				t.Fatal(exe, err)
			}
			for _, name := range []string{"steamcmd.sh", "linux32/steamcmd"} {
				st, err := os.Stat(filepath.Join(dir, name))
				if err != nil || st.Mode().Perm() != 0o755 {
					t.Fatal(st, err)
				}
			}
		})
	}
}
