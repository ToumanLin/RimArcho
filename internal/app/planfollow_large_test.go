package app_test

// Catching up with a large followed collection (07 §7.6, C5): about 900 items, about 450
// enabled, must take well under a second.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/oplog"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// bigCollection serves collection 3000000077 with the children *items holds at each read.
func bigCollection(t *testing.T, e *testkit.Env, items *[]string) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		switch r.URL.Path {
		case "/collection":
			var b strings.Builder
			b.WriteString(`{"response":{"result":1,"resultcount":1,"collectiondetails":[{"publishedfileid":"3000000077","result":1,"children":[`)
			for i, id := range *items {
				if i > 0 {
					b.WriteString(",")
				}
				fmt.Fprintf(&b, `{"publishedfileid":"%s","sortorder":%d,"filetype":0}`, id, i+1)
			}
			b.WriteString(`]}]}}`)
			w.Write([]byte(b.String()))
		case "/api":
			var b strings.Builder
			b.WriteString(`{"response":{"publishedfiledetails":[`)
			for i := 0; ; i++ {
				id := r.Form.Get(fmt.Sprintf("publishedfileids[%d]", i))
				if id == "" {
					break
				}
				if i > 0 {
					b.WriteString(",")
				}
				title := "Big Pack"
				if id != "3000000077" {
					title = "Mod " + id
				}
				fmt.Fprintf(&b, `{"publishedfileid":"%s","result":1,"title":"%s"}`, id, title)
			}
			b.WriteString(`]}}`)
			w.Write([]byte(b.String()))
		}
	}))
	t.Cleanup(srv.Close)
	e.App.SteamAPI, e.App.SteamCollectionAPI = srv.URL+"/api", srv.URL+"/collection"
}

func TestSyncLargeCollection(t *testing.T) {
	t.Parallel()
	e := setup(t, true)
	const n = 900
	var items, active []string
	active = append(active, "ludeon.rimworld")
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
	e.WriteModsConfig("1.6.4633 rev1261", active...)
	scanNow(t, e)
	bigCollection(t, e, &items)
	e.MustRun("plan", "follow", "default", "3000000077", "--mode", "follow")

	// The collection gains 30 installed mods it had not had and drops 10 the plan holds.
	for i := 0; i < 30; i++ {
		id := fmt.Sprint(5000000 + i)
		e.AddWorkshop(id, testkit.Sample(fmt.Sprintf("big.new%02d", i), fmt.Sprintf("New %02d", i)))
		items = append(items[:i*20+1], append([]string{id}, items[i*20+1:]...)...)
	}
	scanNow(t, e)
	var kept []string
	dropped := 0
	for _, id := range items {
		if dropped < 10 && id >= "4000100" && id < "4000200" && id[len(id)-1]%2 == 0 {
			dropped++
			continue
		}
		kept = append(kept, id)
	}
	items = kept
	e.MustRun("plan", "sync", "default", "--check")

	// The window catches up with what the card shows: no network, even when it is old.
	e.App.SteamAPI, e.App.SteamCollectionAPI = "http://127.0.0.1:1/api", "http://127.0.0.1:1/collection"
	later := time.Now().Add(time.Hour)
	e.App.Now = func() time.Time { return later }
	start := time.Now()
	r, err := e.App.SyncCollectionShown("default", "")
	took := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Enabled) != 30 || len(r.Disabled) != 10 || len(r.NotInstalled) != 0 {
		t.Fatalf("enabled %d, disabled %d, not installed %d", len(r.Enabled), len(r.Disabled), len(r.NotInstalled))
	}
	t.Logf("sync of %d items: %v", len(items), took)
	if took > time.Second {
		t.Fatalf("catching up took %v", took)
	}

	// The operation log has one row with the counts and the names (13 §1.3).
	rows, err := e.App.Operations(oplog.Query{Op: oplog.PlanSync})
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows %+v %v", rows, err)
	}
	row := rows[0]
	if row.Result != oplog.OK || row.Subject != "default" || row.Target != "workshop:3000000077" ||
		!strings.HasPrefix(row.Detail, "Big Pack: 30 enabled, 10 disabled, 0 moved; enabled: New 00, New 01") || !strings.Contains(row.Detail, "; disabled: Mod 1") {
		t.Fatalf("row %+v", row)
	}
	if oplog.Where(row) == "" {
		t.Fatal("the row does not say where the edit can be undone")
	}

	// C5: the card's not-installed count stays after catching up; "no change" is about the
	// collection since the snapshot, not about the list matching it.
	out := e.MustRun("plan", "sync", "default", "--check")
	if !strings.Contains(out, "0 changed") || !strings.Contains(out, "no change since the last catch-up") || !strings.Contains(out, "not installed: 50") {
		t.Fatal(out)
	}
}
