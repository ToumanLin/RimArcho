//go:build windows || cgo

package gui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// TestCatchUpLargeCollectionTiming: catching up with a followed collection of about 900
// items, about 450 enabled (16 §2.17), reads nothing over the network and returns quickly.
func TestCatchUpLargeCollectionTiming(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	const n = 900
	var items []string
	active := []string{"ludeon.rimworld"}
	for i := 0; i < n; i++ {
		id := fmt.Sprint(4000000 + i)
		items = append(items, id)
		if i%18 == 17 { // 50 not installed
			continue
		}
		pid := fmt.Sprintf("big.mod%03d", i)
		e.AddWorkshop(id, testkit.Sample(pid, fmt.Sprintf("Mod %03d", i)))
		if i%2 == 0 {
			active = append(active, pid)
		}
	}
	for i := 0; i < 30; i++ {
		e.AddWorkshop(fmt.Sprint(5000000+i), testkit.Sample(fmt.Sprintf("big.new%02d", i), fmt.Sprintf("New %02d", i)))
	}
	e.WriteModsConfig("1.6.4633 rev1261", active...)
	e.AddEnv(false)
	e.MustRun("scan")
	var reads atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		time.Sleep(300 * time.Millisecond) // the Steam Web API from far away
		r.ParseForm()
		var b strings.Builder
		switch r.URL.Path {
		case "/collection":
			b.WriteString(`{"response":{"collectiondetails":[{"publishedfileid":"3000000077","result":1,"children":[`)
			for i, id := range items {
				if i > 0 {
					b.WriteString(",")
				}
				fmt.Fprintf(&b, `{"publishedfileid":"%s","sortorder":%d,"filetype":0}`, id, i+1)
			}
			b.WriteString(`]}]}}`)
		case "/api":
			b.WriteString(`{"response":{"publishedfiledetails":[`)
			for i := 0; ; i++ {
				id := r.Form.Get(fmt.Sprintf("publishedfileids[%d]", i))
				if id == "" {
					break
				}
				if i > 0 {
					b.WriteString(",")
				}
				fmt.Fprintf(&b, `{"publishedfileid":"%s","result":1,"title":"T%s"}`, id, id)
			}
			b.WriteString(`]}}`)
		}
		w.Write([]byte(b.String()))
	}))
	t.Cleanup(srv.Close)
	e.App.SteamAPI, e.App.SteamCollectionAPI = srv.URL+"/api", srv.URL+"/collection"
	api := NewAPI(e.App, func(string, any) {})
	t.Cleanup(api.Close)
	id := api.Shell().Plan.ID

	start := time.Now()
	if _, err := api.PlanFollow(id, "3000000077", "follow"); err != nil {
		t.Fatal(err)
	}
	t.Logf("follow (network, %d requests): %v", reads.Load(), time.Since(start))
	// The collection gains 30 installed mods and drops 10 the list holds.
	for i := 0; i < 30; i++ {
		at := i*20 + 1
		items = append(items[:at], append([]string{fmt.Sprint(5000000 + i)}, items[at:]...)...)
	}
	var kept []string
	for i, id := range items {
		if i >= 100 && i < 120 && i%2 == 0 {
			continue
		}
		kept = append(kept, id)
	}
	items = kept
	// the card was read more than ten minutes ago
	now := time.Now().Add(11 * time.Minute)
	e.App.Now = func() time.Time { return now }
	start = time.Now()
	reads.Store(0)
	if _, err := api.PlanCheck(id); err != nil {
		t.Fatal(err)
	}
	t.Logf("check (network, %d requests): %v", reads.Load(), time.Since(start))
	// the titles it had are not asked again: the collection and the 30 new ones
	if n := reads.Load(); n != 2 {
		t.Fatalf("a check of known items sent %d requests", n)
	}
	now = now.Add(11 * time.Minute)
	d, err := api.PlanManage(id)
	if err != nil || len(d.Sel.Follows) != 1 || d.Sel.Follows[0].Changes == "" {
		t.Fatalf("%+v %v", d, err)
	}

	reads.Store(0)
	start = time.Now()
	text, err := api.PlanSync(id, "3000000077")
	sync := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	start = time.Now()
	res, _ := api.workspaceRead(1, 0)
	ws := time.Since(start)
	start = time.Now()
	d, err = api.PlanManage(id)
	pm := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("sync %v (%d network reads), workspace %v, page %v: %s", sync, reads.Load(), ws, pm, text)
	if reads.Load() != 0 {
		t.Fatal("catching up read the collection again")
	}
	if !strings.Contains(text, "30 enabled, 9 disabled") {
		t.Fatal(text)
	}
	if res == nil || res.Board == nil {
		t.Fatal("no board after catching up")
	}
	// the card after catching up: nothing changed since, the uninstalled ones still counted
	if f := d.Sel.Follows[0]; f.Changes != "" || f.NotInstalled != 50 {
		b, _ := json.Marshal(f)
		t.Fatalf("after catching up: %s", b)
	}
	if total := sync + ws + pm; total > timingLimit(time.Second) {
		t.Fatalf("catching up and reading the page again took %v", total)
	}
}
