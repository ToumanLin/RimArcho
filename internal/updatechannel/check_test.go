package updatechannel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/lanzou"
	"github.com/ToumanLin/RimArcho/internal/selfupdate"
)

type wrappedTransport func(*http.Request) (*http.Response, error)

func (f wrappedTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCheckPreservesWrappedTransport(t *testing.T) {
	for _, useDefault := range []bool{false, true} {
		t.Run(fmt.Sprint(useDefault), func(t *testing.T) {
			calls := 0
			transport := wrappedTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != selfupdate.DefaultAPI+"/releases/latest" {
					t.Fatal("wrapper received wrong request", r.URL)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"tag_name":"v1.2.3"}`))}, nil
			})
			client := &http.Client{Transport: transport}
			if useDefault {
				old := http.DefaultTransport
				http.DefaultTransport = transport
				t.Cleanup(func() { http.DefaultTransport = old })
				client = nil
			}
			report, err := Check(context.Background(), Options{Source: "github", GitHub: selfupdate.Client{HTTP: client}})
			if err != nil || calls != 1 || report.Version != "1.2.3" || report.GitHub == nil {
				t.Fatalf("calls=%d report=%+v err=%v", calls, report, err)
			}
		})
	}
}

func TestChecksChooseHighestAndKeepFailures(t *testing.T) {
	for _, failGitHub := range []bool{false, true} {
		t.Run(fmt.Sprint(failGitHub), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/releases/latest":
					if failGitHub {
						http.Error(w, "offline", http.StatusServiceUnavailable)
						return
					}
					ioJSON(w, map[string]any{"tag_name": "v1.10.0"})
				case "/steam":
					if r.Method != "POST" || r.FormValue("itemcount") != "1" || r.FormValue("publishedfileids[0]") != "12345" {
						t.Error("not an anonymous single-item request")
					}
					if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
						t.Error("account data used in metadata check")
					}
					fmt.Fprint(w, `{"response":{"publishedfiledetails":[{"publishedfileid":"12345","result":1,"description":"Monitor\nRimArcho v1.9.0","tags":[{"tag":"v99.0.0"}]}]}}`)
				default:
					t.Errorf("unexpected check endpoint %s", r.URL.Path)
				}
			}))
			defer server.Close()
			options := Options{GitHub: selfupdate.Client{HTTP: server.Client(), API: server.URL}, HTTP: server.Client(), MonitorItem: "12345", SteamAPI: server.URL + "/steam", Lanzou: &lanzou.Client{ShareURL: "invalid"}}
			report, err := Check(context.Background(), options)
			want := "1.10.0"
			if failGitHub {
				want = "1.9.0"
			}
			if err != nil || report.Version != want || len(report.Sources) != 3 {
				t.Fatalf("report=%+v: %v", report, err)
			}
			if report.Sources[1].Source != "monitor" || report.Sources[1].Version != "1.9.0" || report.Sources[2].Status != "failed" {
				t.Fatal(report)
			}
			encoded, _ := json.Marshal(report)
			if strings.Contains(string(encoded), "ctot") || strings.Contains(string(encoded), server.URL) {
				t.Fatal("report exposed configuration or URLs")
			}
		})
	}
}

func ioJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(value)
}

func TestConcurrentDeadlinePreservesLanzouMaximum(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	defer close(release)
	var ready sync.WaitGroup
	ready.Add(3)
	makeTask := func(name, version string, blocked bool) checkTask {
		return checkTask{name, func(ctx context.Context) SourceResult {
			started <- struct{}{}
			ready.Done()
			ready.Wait()
			if blocked {
				<-release
			}
			return SourceResult{Status: "ok", Version: version}
		}}
	}
	begin := time.Now()
	report, err := collect(ctx, []checkTask{makeTask("github", "1.0.0", false), makeTask("monitor", "99.0.0", true), makeTask("lanzou", "1.0.10", false)})
	if err != nil || report.Version != "1.0.10" || report.Sources[1].Status != "timeout" || len(started) != 3 || time.Since(begin) > 2*time.Second {
		t.Fatalf("report=%+v: %v", report, err)
	}
}

func TestGitHubOnlyAndPrereleaseBypassOthers(t *testing.T) {
	for _, pre := range []bool{false, true} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if pre {
				if r.URL.RawQuery != "per_page=30" {
					t.Error("wrong prerelease query")
				}
				fmt.Fprint(w, `[{"tag_name":"v1.1.0-rc2","prerelease":true},{"tag_name":"v1.1.0-rc10","prerelease":true}]`)
			} else {
				fmt.Fprint(w, `{"tag_name":"v1.0.10"}`)
			}
		}))
		options := Options{Source: "github", Prerelease: pre, GitHub: selfupdate.Client{HTTP: server.Client(), API: server.URL}, MonitorItem: "12345", SteamAPI: server.URL + "/must-not-run", Lanzou: &lanzou.Client{ShareURL: "invalid"}}
		if pre {
			options.Source = "auto"
		}
		report, err := Check(context.Background(), options)
		server.Close()
		want := "1.0.10"
		if pre {
			want = "1.1.0-rc10"
		}
		if err != nil || report.Version != want || len(report.Sources) != 1 || calls != 1 {
			t.Fatalf("%+v calls=%d err=%v", report, calls, err)
		}
	}
}

func TestMonitorStampAndRetiredItem(t *testing.T) {
	t.Setenv(MonitorItemEnv, "")
	if r := checkMonitor(context.Background(), Options{}); r.Status != "unconfigured" {
		t.Fatal(r)
	}
	for _, id := range []string{"3835688284", "../123", "0"} {
		if r := checkMonitor(context.Background(), Options{MonitorItem: id}); r.Status != "failed" {
			t.Fatal(r)
		}
	}
	for _, body := range []string{
		`{"response":{"publishedfiledetails":[{"publishedfileid":"123","result":9,"description":"RimArcho v1.2.3"}]}}`,
		`{"response":{"publishedfiledetails":[{"publishedfileid":"999","result":1,"description":"RimArcho v1.2.3"}]}}`,
		`{"response":{"publishedfiledetails":[{"publishedfileid":"123","result":1,"description":"no stamp","tags":[{"tag":"v1.2.3"}]}]}}`,
		strings.Repeat("x", (1<<20)+1),
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		r := checkMonitor(context.Background(), Options{MonitorItem: "123", SteamAPI: server.URL, HTTP: server.Client()})
		server.Close()
		if r.Status != "failed" {
			t.Fatal(r)
		}
	}
}
