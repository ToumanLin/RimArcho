package updatefeed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

func TestLanzouRecipeAndObservedTransfer(t *testing.T) {
	data := syntheticZip(t)
	var origin string
	var events strings.Builder
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/share":
			io.WriteString(w, `url:'/filemoreajax.php'; 'lx':2,'fid':1,'uid':2,'puid':3,'rep':0,'up':1,'ls':1; var t='1234567890'; var k='0123456789abcdef0123456789abcdef';`)
		case "/filemoreajax.php":
			if req.FormValue("pwd") != "private-password" {
				t.Error("missing password")
			}
			switch req.FormValue("pg") {
			case "1":
				io.WriteString(w, `{"zt":1,"text":[{"name_all":"other.zip","id":"other"}]}`)
			case "2":
				io.WriteString(w, `{"zt":1,"text":[{"name_all":"test.zip","id":"entry?webpage=bearer-secret"}]}`)
			case "3":
				io.WriteString(w, `{"zt":1,"text":[]}`)
			default:
				t.Error("unexpected folder page")
			}
		case "/entry":
			if req.URL.Query().Get("webpage") != "bearer-secret" {
				t.Error("file query lost")
			}
			io.WriteString(w, `<iframe class="ifr2" src="/fn?token=bearer-secret"></iframe>`)
		case "/fn":
			fmt.Fprintf(w, "var domain1='%s/ajaxfile.php'; var ajaxdata='bearer-secret'; var wp_sign='bearer-secret';", origin)
		case "/ajaxfile.php":
			if req.FormValue("sign") != "bearer-secret" {
				t.Error("missing signature")
			}
			fmt.Fprintf(w, `{"zt":1,"dom":%q,"url":"ticket?token=bearer-secret"}`, origin)
		case "/file/ticket":
			w.Header().Set("Location", "/bytes?token=bearer-secret")
			w.WriteHeader(302)
		case "/bytes":
			w.Header().Set("Content-Type", "application/zip")
			w.Write(data)
		default:
			t.Errorf("unexpected request path %s", req.URL.Path)
			http.NotFound(w, req)
		}
	}))
	defer server.Close()
	origin = server.URL
	source, err := LanzouSource(origin+"/share", "private-password")
	if err != nil {
		t.Fatal(err)
	}
	r, err := newRunner(server.Client(), source, origin)
	if err != nil {
		t.Fatal(err)
	}
	defer r.client.CloseIdleConnections()
	r.observe = func(e ProbeEvent) { fmt.Fprintf(&events, "%s %s %d\n", e.Stage, e.Host, e.Status) }
	target, err := r.resolve(context.Background(), "test.zip")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	archive, err := r.transfer(context.Background(), target, nil, dir)
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(data)
	if archive.Size != int64(len(data)) || archive.SHA256 != hex.EncodeToString(want[:]) || !strings.Contains(archive.URL, "/bytes?") {
		t.Fatal("wrong observed bytes or final host")
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("probe transfer file not deleted")
	}
	if strings.Contains(events.String(), "bearer-secret") || strings.Contains(events.String(), "private-password") || strings.Contains(events.String(), "/fn") {
		t.Fatal("events exposed request secrets")
	}
}

func TestLanzouPagination(t *testing.T) {
	const name = "archo-1.2.3-windows-amd64.zip"
	for _, tc := range []struct {
		name  string
		pages []string
		want  string
		err   string
	}{
		{"later page", []string{`{"zt":1,"text":[{"name_all":"other.zip","id":"a"}]}`, `{"zt":"1","text":[{"name_all":"` + name + `","id":"chosen?webpage=secret"}]}`, `{"zt":1,"text":[]}`}, "chosen?webpage=secret", ""},
		{"status end", []string{`{"zt":1,"text":[{"name_all":"` + name + `","id":"a"}]}`, `{"zt":"2"}`}, "a", ""},
		{"missing", []string{`{"zt":1,"text":[]}`}, "", "not found"},
		{"later failure", []string{`{"zt":1,"text":[{"name_all":"` + name + `","id":"a"}]}`, `{"zt":0,"info":"secret"}`}, "", "listing failed"},
		{"duplicate name", []string{`{"zt":1,"text":[{"name_all":"` + name + `","id":"a"}]}`, `{"zt":1,"text":[{"name_all":"` + name + `","id":"b"}]}`}, "", "ambiguous"},
		{"repeated page", []string{`{"zt":1,"text":[{"name_all":"other.zip","id":"a"}]}`, `{"zt":1,"text":[{"name_all":"other.zip","id":"a"}]}`}, "", "no progress"},
		{"invalid entry", []string{`{"zt":1,"text":[{"name_all":"other.zip"}]}`}, "", "invalid"},
		{"page cap", nil, "", "16-page"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				calls++
				if req.Method != "POST" || req.FormValue("pg") != strconv.Itoa(calls) || req.FormValue("pwd") != "synthetic +&=" {
					t.Error("pagination form changed")
				}
				if tc.pages == nil {
					fmt.Fprintf(w, `{"zt":1,"text":[{"name_all":"other.zip","id":"%d"}]}`, calls)
					return
				}
				if calls > len(tc.pages) {
					t.Error("unexpected extra request")
					return
				}
				io.WriteString(w, tc.pages[calls-1])
			}))
			defer server.Close()
			step := Step{Op: "lanzou-list", Into: "id", Method: "POST", URL: server.URL, Form: map[string]string{"pg": "1", "pwd": "synthetic +&="}}
			source := Source{Hosts: []string{"example.com"}, Steps: []Step{step}, Download: server.URL + "/${id}"}
			if err := validateSource(source); err != nil {
				t.Fatal(err)
			}
			r, err := newRunner(server.Client(), source, server.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer r.client.CloseIdleConnections()
			got, err := r.resolve(context.Background(), name)
			if tc.err == "" {
				if err != nil || got != server.URL+"/"+tc.want || calls != len(tc.pages) {
					t.Fatalf("got %q, calls %d: %v", got, calls, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("wanted %q: %v", tc.err, err)
			}
			if step.Form["pg"] != "1" {
				t.Fatal("authenticated recipe was mutated")
			}
		})
	}
}

func TestDownloadStopsAtVerification(t *testing.T) {
	for _, body := range []string{`<html><script>function down_r(el){}</script></html>`, `<html>unexpected</html>`, `<script>var arg1='0123456789ABCDEF0123456789ABCDEF01234567';'acw_sc__v2'</script>`} {
		t.Run(body, func(t *testing.T) {
			gets, posts := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.Method == "POST" {
					posts++
				}
				gets++
				w.Header().Set("Content-Type", "text/html")
				if gets == 1 {
					io.WriteString(w, `<script>var arg1='0123456789ABCDEF0123456789ABCDEF01234567';'acw_sc__v2'</script>`)
				} else {
					io.WriteString(w, body)
				}
			}))
			defer server.Close()
			r, err := newRunner(server.Client(), Source{Challenge: "acw-v2"}, server.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer r.client.CloseIdleConnections()
			dir := t.TempDir()
			_, err = r.transfer(context.Background(), server.URL, nil, dir)
			if err == nil || posts != 0 || gets != 2 {
				t.Fatalf("verification boundary: gets=%d posts=%d err=%v", gets, posts, err)
			}
			if strings.Contains(body, "down_r") && !strings.Contains(err.Error(), "human verification") {
				t.Fatal(err)
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 0 {
				t.Fatal("verification left download files")
			}
		})
	}
}

func TestVerifySelectedLocalArchive(t *testing.T) {
	data := syntheticZip(t)
	f := parseManifest(t, manifest(data, "https://example.com/unused"))
	path := filepath.Join(t.TempDir(), "browser-renamed.zip")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := f.VerifyLocal(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{append(append([]byte(nil), data...), 0), append([]byte{0}, data[1:]...)} {
		if err := os.WriteFile(path, bad, 0600); err != nil {
			t.Fatal(err)
		}
		if err := f.VerifyLocal(context.Background(), path); err == nil {
			t.Fatal("untrusted local bytes accepted")
		}
	}
	if err := f.VerifyLocal(context.Background(), t.TempDir()); err == nil {
		t.Fatal("directory accepted")
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.VerifyLocal(ctx, path); err == nil {
		t.Fatal("cancellation ignored")
	}
}
