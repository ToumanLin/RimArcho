package app_test

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/netcfg"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// 17 §2.2, §5.1: with network.github_mirror auto, a mirror may relay the update ZIP, but
// the release metadata and the checksum it is checked against come without one.
func TestSelfUpdateChecksumAvoidsAutoMirror(t *testing.T) {
	if !app.SelfUpdateSupported() {
		t.Skip("Windows replacement")
	}
	t.Setenv("RIMARCHO_UPDATE_MONITOR_ITEM", "")
	e := testkit.New(t)
	f := newSelfRelease(t)
	f.use(e.App)
	var mu sync.Mutex
	allowed := map[string]bool{}
	base := e.App.HTTP.Transport
	e.App.HTTP = &http.Client{Timeout: e.App.HTTP.Timeout, Transport: selfTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasPrefix(r.URL.String(), f.srv.URL) {
			mu.Lock()
			allowed[r.URL.Path] = allowed[r.URL.Path] || netcfg.AutoMirrorAllowed(r.Context())
			mu.Unlock()
		}
		return base.RoundTrip(r)
	})}
	if _, _, err := e.App.CheckSelfUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.InstallSelfUpdate(context.Background(), func(app.SelfUpdateStep) {}); err != nil {
		t.Fatal(err)
	}
	if !allowed["/dl/zip"] {
		t.Errorf("the ZIP was kept from mirrors: %v", allowed)
	}
	for path, ok := range allowed {
		if path != "/dl/zip" && ok {
			t.Errorf("%s may come through an automatic mirror", path)
		}
	}
	if len(allowed) < 3 {
		t.Fatalf("requests seen: %v", allowed)
	}
}
