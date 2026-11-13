package lanzou

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const testName = "archo-1.2.3-windows-amd64.zip"
const folderHTML = `url:'/filemoreajax.php'; 'lx':2,'fid':1,'uid':2,'puid':3,'rep':0,'up':1,'ls':1; var t='1234567890'; var k='0123456789abcdef0123456789abcdef';`
const challengeHTML = `<script>var arg1='0123456789ABCDEF0123456789ABCDEF01234567';'acw_sc__v2'</script>`

func syntheticZip(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, err := w.Create("archo.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("synthetic; never executed")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func testClient(t *testing.T, handler http.HandlerFunc) (Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	return Client{HTTP: server.Client(), ShareURL: server.URL + "/share", Password: "private +&=", testOrigin: server.URL}, server
}

// fullPage is a folder page of folderPageSize entries: entries first, then filler files.
func fullPage(prefix string, entries ...string) string {
	for i := len(entries); i < folderPageSize; i++ {
		entries = append(entries, fmt.Sprintf(`{"name_all":"notes-%s-%d.txt","id":"%s-%d"}`, prefix, i, prefix, i))
	}
	return `{"zt":1,"text":[` + strings.Join(entries, ",") + `]}`
}

func TestListingOnlyAndPagination(t *testing.T) {
	for _, tc := range []struct {
		name     string
		pages    []string
		version  string
		checksum bool
		wantErr  string
		served   int // pages requested when the listing succeeds; 0 means all of pages
	}{
		{"later numeric maximum", []string{fullPage("p1", `{"name_all":"archo-1.9.0-windows-amd64.zip","id":"old"}`, `{"name_all":"archo-99.0.0-rc1-windows-amd64.zip","id":"rc"}`), `{"zt":1,"text":[{"name_all":"archo-1.10.0-windows-amd64.zip","id":"new"},{"name_all":"archo-1.10.0-windows-amd64.zip.sha256","id":"sum"}]}`}, "1.10.0", true, "", 0},
		{"zip only", []string{fullPage("p1", `{"name_all":"`+testName+`","id":"zip?secret=token"}`), `{"zt":"2"}`}, "1.2.3", false, "", 0},
		{"full page then empty", []string{fullPage("p1", `{"name_all":"`+testName+`","id":"zip"}`), `{"zt":1,"text":[]}`}, "1.2.3", false, "", 0},
		// Some nodes answer the request after the last page as expired instead of zt=2:
		// a short page is the last, so that request is never made (RimCrow's PAGE_SIZE rule).
		{"short page ends", []string{`{"zt":1,"text":[{"name_all":"` + testName + `","id":"zip"}]}`, `{"zt":3,"info":"expired"}`}, "1.2.3", false, "", 1},
		{"stable exact only", []string{`{"zt":1,"text":[{"name_all":"archo-01.2.3-windows-amd64.zip","id":"a"},{"name_all":"archo-9.9.9-linux-amd64.zip","id":"b"},{"name_all":"archo-1.2.3-windows-amd64.zip.bak","id":"c"}]}`}, "", false, "", 0},
		{"ambiguous", []string{`{"zt":1,"text":[{"name_all":"` + testName + `","id":"a"},{"name_all":"` + testName + `","id":"b"}]}`}, "", false, "ambiguous", 0},
		{"later failure", []string{fullPage("p1", `{"name_all":"`+testName+`","id":"a"}`), `{"zt":0,"info":"node busy\nretry later"}`}, "", false, `page 2: zt=0 info="node busy retry later"`, 0},
		{"repeated", []string{fullPage("p1", `{"name_all":"`+testName+`","id":"a"}`), fullPage("p1", `{"name_all":"`+testName+`","id":"a"}`)}, "", false, "no progress", 0},
		{"malformed", []string{`{"zt":1,"text":[{"name_all":"x"}]}`}, "", false, "invalid", 0},
		{"page cap", nil, "", false, "16-page", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pages, other := 0, 0
			c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/share" {
					io.WriteString(w, folderHTML)
					return
				}
				if r.URL.Path != "/filemoreajax.php" {
					other++
					http.Error(w, "no file/node in checks", 500)
					return
				}
				pages++
				if r.Method != "POST" || r.FormValue("pg") != strconv.Itoa(pages) || r.FormValue("pwd") != "private +&=" {
					t.Error("incorrect listing form")
				}
				if tc.pages == nil {
					io.WriteString(w, fullPage("p"+strconv.Itoa(pages)))
					return
				}
				if pages > len(tc.pages) {
					t.Error("extra page request")
					return
				}
				io.WriteString(w, tc.pages[pages-1])
			})
			listing, err := c.List(context.Background())
			if other != 0 {
				t.Fatal("version check reached file/download stages")
			}
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("wanted %q: %v", tc.wantErr, err)
				}
				return
			}
			served := tc.served
			if served == 0 {
				served = len(tc.pages)
			}
			if err != nil || listing.Version != tc.version || listing.HasChecksum != tc.checksum || pages != served {
				t.Fatalf("listing=%+v pages=%d err=%v", listing, pages, err)
			}
		})
	}
}

func TestReleaseZipAndVerificationBoundary(t *testing.T) {
	for _, mode := range []string{"ok", "missing checksum", "wrong hash", "human check", "repeated challenge", "oversized checksum", "no GitHub sum", "newer also listed"} {
		t.Run(mode, func(t *testing.T) {
			data := syntheticZip(t)
			sum := sha256.Sum256(data)
			checksum := hex.EncodeToString(sum[:]) + "  " + testName + "\n"
			if mode == "wrong hash" {
				checksum = strings.Repeat("0", 64) + "  " + testName
			}
			var origin string
			nodeGets, verificationPosts := 0, 0
			var events strings.Builder
			c, server := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/share":
					io.WriteString(w, folderHTML)
				case "/filemoreajax.php":
					if r.FormValue("pg") != "1" {
						io.WriteString(w, `{"zt":2}`)
						return
					}
					entries := []map[string]string{{"name_all": testName, "id": "entry?kind=zip&token=private-secret"}}
					if mode != "missing checksum" {
						entries = append(entries, map[string]string{"name_all": testName + ".sha256", "id": "entry?kind=sum&token=private-secret"})
					}
					if mode == "newer also listed" {
						entries = append(entries, map[string]string{"name_all": "archo-9.0.0-windows-amd64.zip", "id": "never"})
					}
					json.NewEncoder(w).Encode(map[string]any{"zt": 1, "text": entries})
				case "/entry":
					if r.URL.Query().Get("kind") == "sum" {
						t.Error("download fetched a Lanzou checksum")
					}
					fmt.Fprintf(w, `<iframe src="/fn?kind=%s&token=private-secret"></iframe>`, r.URL.Query().Get("kind"))
				case "/fn":
					fmt.Fprintf(w, "var domain1='%s/ajaxfile.php?kind=%s';var ajaxdata='private-secret';var wp_sign='private-secret';", origin, r.URL.Query().Get("kind"))
				case "/ajaxfile.php":
					if r.FormValue("sign") != "private-secret" {
						t.Error("missing API token")
					}
					fmt.Fprintf(w, `{"zt":1,"dom":%q,"url":"ticket?kind=%s&token=private-secret"}`, origin, r.URL.Query().Get("kind"))
				case "/file/ticket":
					nodeGets++
					if mode == "human check" || mode == "repeated challenge" {
						w.Header().Set("Content-Type", "text/html")
						if nodeGets == 1 || mode == "repeated challenge" {
							io.WriteString(w, challengeHTML)
						} else {
							io.WriteString(w, `<script>function down_r(el){}</script>`)
						}
						return
					}
					if r.URL.Query().Get("kind") == "sum" {
						w.Header().Set("Content-Type", "text/plain")
						if mode == "oversized checksum" {
							io.WriteString(w, strings.Repeat("x", MaxChecksum+1))
						} else {
							io.WriteString(w, checksum)
						}
						return
					}
					w.Header().Set("Location", "/bytes")
					w.WriteHeader(302)
				case "/bytes":
					w.Header().Set("Content-Type", "application/zip")
					w.Write(data)
				default:
					verificationPosts++
					http.Error(w, "unexpected verification request", 500)
				}
			})
			origin = server.URL
			c.Observe = func(e ProbeEvent) { fmt.Fprintf(&events, "%s %s %d\n", e.Stage, e.Host, e.Status) }
			dir := t.TempDir()
			supplied := []byte(checksum)
			if mode == "no GitHub sum" {
				supplied = nil
			}
			if mode == "oversized checksum" {
				supplied = []byte(strings.Repeat("x", MaxChecksum+1))
			}
			pair, err := c.Download(context.Background(), "1.2.3", dir, supplied)
			if mode == "ok" || mode == "newer also listed" || mode == "missing checksum" || mode == "no GitHub sum" {
				if err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(pair.ZipPath)
				if err != nil || !bytes.Equal(got, data) || filepath.Base(pair.ZipPath) != testName {
					t.Fatal("wrong staged ZIP")
				}
				if pair.SHA256 != hex.EncodeToString(sum[:]) || pair.Size != int64(len(data)) {
					t.Fatal("wrong measured bytes")
				}
				if err := pair.Close(); err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				pair.Close()
				t.Fatal("bad pair accepted")
			}
			if mode == "human check" && !strings.Contains(err.Error(), "human verification") {
				t.Fatal(err)
			}
			if (mode == "human check" || mode == "repeated challenge") && nodeGets != 2 {
				t.Fatalf("retried node %d times", nodeGets)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatal("temporary release files left behind")
			}
			if verificationPosts != 0 || strings.Contains(events.String(), "private-secret") || strings.Contains(events.String(), "/fn") {
				t.Fatal("verification automation or token disclosure")
			}
		})
	}
}

func TestDestinationAndResourceLimits(t *testing.T) {
	r, err := newRunner(nil, []string{"example.com"}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer r.client.CloseIdleConnections()
	for _, raw := range []string{"http://example.com/x", "https://example.com:661/x", "https://127.0.0.1/x", "https://example.com.evil.test/x", "https://user:secret@example.com/x", "https://example.com/#x", "file:///tmp/x"} {
		if _, err := r.checkURL(raw); err == nil {
			t.Errorf("unsafe URL accepted: %s", raw)
		}
	}
	for _, mode := range []string{"redirect", "requests", "body", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			c, server := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "redirect":
					w.Header().Set("Location", "/loop")
					w.WriteHeader(302)
				case "body":
					io.WriteString(w, strings.Repeat("x", maxBody+1))
				default:
					io.WriteString(w, "ok")
				}
			})
			r, _, err := c.runner()
			if err != nil {
				t.Fatal(err)
			}
			defer r.client.CloseIdleConnections()
			ctx := context.Background()
			if mode == "cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			calls := 1
			if mode == "requests" {
				calls = maxRequests + 1
			}
			for i := 0; i < calls; i++ {
				_, _, err = r.page(ctx, server.URL, "GET", nil, nil)
				if err != nil {
					break
				}
			}
			if err == nil {
				t.Fatal("resource bound was not enforced")
			}
		})
	}
}
