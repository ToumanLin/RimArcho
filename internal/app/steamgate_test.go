package app_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// countingSteam answers GetPublishedFileDetails through answer (nil: result 1 for every ID)
// and counts the requests and the most in flight at once.
type countingSteam struct {
	srv                *httptest.Server
	requests, inFlight atomic.Int32
	mostInFlight       atomic.Int32
	mu                 sync.Mutex
	answer             func(w http.ResponseWriter, ids []string) bool // true: answered
	delay              time.Duration
}

func newCountingSteam(t *testing.T) *countingSteam {
	s := &countingSteam{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		n := s.inFlight.Add(1)
		defer s.inFlight.Add(-1)
		for {
			m := s.mostInFlight.Load()
			if n <= m || s.mostInFlight.CompareAndSwap(m, n) {
				break
			}
		}
		time.Sleep(s.delay)
		r.ParseForm()
		count, _ := strconv.Atoi(r.Form.Get("itemcount"))
		ids := make([]string, count)
		for i := range ids {
			ids[i] = r.Form.Get(fmt.Sprintf("publishedfileids[%d]", i))
		}
		s.mu.Lock()
		answer := s.answer
		s.mu.Unlock()
		if answer != nil && answer(w, ids) {
			return
		}
		var out []string
		for _, id := range ids {
			out = append(out, fmt.Sprintf(`{"publishedfileid":%q,"result":1,"title":"T","description":"Workshop text of %s","time_updated":1790000000}`, id, id))
		}
		fmt.Fprintf(w, `{"response":{"publishedfiledetails":[%s]}}`, strings.Join(out, ","))
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *countingSteam) setAnswer(f func(w http.ResponseWriter, ids []string) bool) {
	s.mu.Lock()
	s.answer = f
	s.mu.Unlock()
}

func workshopHome(t *testing.T, ids ...string) *testkit.Env {
	e := testkit.New(t)
	for _, id := range ids {
		e.AddWorkshop(id, testkit.Sample("tester.m"+id, "Mod "+id))
	}
	e.AddEnv(false)
	e.MustRun("scan")
	return e
}

func describeFetch(t *testing.T, e *testkit.Env, id string) *app.Description {
	t.Helper()
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	m, err := ix.ResolveOne("ws:" + id)
	if err != nil {
		t.Fatal(err)
	}
	return e.App.Describe(ix, m, true, false)
}

// A 429 starts a cooldown: no further request reaches Steam, from this App, another App of
// the process, or a restarted one (11 §2.1).
func TestSteamCooldownAfter429(t *testing.T) {
	t.Parallel()
	e := workshopHome(t, "7001", "7002")
	s := newCountingSteam(t)
	s.setAnswer(func(w http.ResponseWriter, _ []string) bool {
		w.Header().Set("Retry-After", "120")
		http.Error(w, "slow down", http.StatusTooManyRequests)
		return true
	})
	e.App.SteamAPI = s.srv.URL
	if d := describeFetch(t, e, "7001"); !strings.Contains(d.WorkshopState, "429") {
		t.Fatalf("first: %q", d.WorkshopState)
	}
	for _, id := range []string{"7001", "7002", "7002"} {
		if d := describeFetch(t, e, id); !strings.Contains(d.WorkshopState, "Steam is limiting requests") {
			t.Fatalf("%s during cooldown: %q", id, d.WorkshopState)
		}
	}
	if err := e.App.CheckSubscriptionSource(app.LibWorkshop); err == nil {
		t.Fatal("check during cooldown succeeded")
	}
	if n := s.requests.Load(); n != 1 {
		t.Fatalf("requests during cooldown: %d", n)
	}
	// A restarted App (its own client, so its own gate) reads the stored cooldown.
	e.Open()
	e.App.SteamAPI = s.srv.URL
	if d := describeFetch(t, e, "7002"); !strings.Contains(d.WorkshopState, "Steam is limiting requests") || s.requests.Load() != 1 {
		t.Fatalf("after restart: %q, requests %d", d.WorkshopState, s.requests.Load())
	}
}

// Item results "rate limited" and three server errors in a row start a cooldown too; a
// single server error is retried once, as before (16 §2.16.1 S04).
func TestSteamCooldownOnLimitedResultAndServerErrors(t *testing.T) {
	t.Parallel()
	t.Run("result 84", func(t *testing.T) {
		t.Parallel()
		e := workshopHome(t, "7101", "7102")
		s := newCountingSteam(t)
		s.setAnswer(func(w http.ResponseWriter, ids []string) bool {
			var out []string
			for _, id := range ids {
				out = append(out, fmt.Sprintf(`{"publishedfileid":%q,"result":84}`, id))
			}
			fmt.Fprintf(w, `{"response":{"publishedfiledetails":[%s]}}`, strings.Join(out, ","))
			return true
		})
		e.App.SteamAPI = s.srv.URL
		err := e.App.CheckSubscriptionSource(app.LibWorkshop)
		c := e.App.SubscriptionSourceCheck(app.LibWorkshop)
		if err == nil || len(c.Failures) != 2 || s.requests.Load() != 1 {
			t.Fatalf("err %v failures %+v requests %d", err, c.Failures, s.requests.Load())
		}
		if d := describeFetch(t, e, "7101"); !strings.Contains(d.WorkshopState, "Steam is limiting requests") || s.requests.Load() != 1 {
			t.Fatalf("after 84: %q, requests %d", d.WorkshopState, s.requests.Load())
		}
	})
	t.Run("server errors", func(t *testing.T) {
		t.Parallel()
		e := workshopHome(t, "7201")
		s := newCountingSteam(t)
		s.setAnswer(func(w http.ResponseWriter, _ []string) bool {
			http.Error(w, "busy", http.StatusBadGateway)
			return true
		})
		e.App.SteamAPI = s.srv.URL
		// The check sends the batch and retries it once: two errors, no cooldown yet.
		e.App.CheckSubscriptionSource(app.LibWorkshop)
		if s.requests.Load() != 2 {
			t.Fatalf("requests %d", s.requests.Load())
		}
		if d := describeFetch(t, e, "7201"); !strings.Contains(d.WorkshopState, "502") {
			t.Fatalf("third: %q", d.WorkshopState)
		}
		if d := describeFetch(t, e, "7201"); !strings.Contains(d.WorkshopState, "Steam is limiting requests") || s.requests.Load() != 3 {
			t.Fatalf("after three: %q, requests %d", d.WorkshopState, s.requests.Load())
		}
	})
}

// Requests to one origin: at most 2 at once, 4 without waiting, then about 1 a second.
func TestSteamGatePacesRequests(t *testing.T) {
	t.Parallel()
	ids := []string{"7301", "7302", "7303", "7304", "7305", "7306"}
	e := workshopHome(t, ids...)
	s := newCountingSteam(t)
	s.delay = 50 * time.Millisecond
	e.App.SteamAPI = s.srv.URL
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	var wg sync.WaitGroup
	for _, id := range ids {
		m, err := ix.ResolveOne("ws:" + id)
		if err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if d := e.App.Describe(ix, m, true, false); d.WorkshopState != "fetched" {
				t.Errorf("%s: %q", id, d.WorkshopState)
			}
		}()
	}
	wg.Wait()
	if most := s.mostInFlight.Load(); most > 2 {
		t.Fatalf("%d requests at once", most)
	}
	// 4 at once, then the 5th and 6th a second apart.
	if took := time.Since(start); took < 1800*time.Millisecond {
		t.Fatalf("6 requests took %v", took)
	}
}

// Answers are cached so a view does not ask again: the update check's descriptions, and a
// removed item for a day (16 §4.3, §2.16.1 C01).
func TestWorkshopDescriptionCacheSavesRequests(t *testing.T) {
	t.Parallel()
	e := workshopHome(t, "7401", "7402")
	e.SetTimeUpdated("7401", 1790000000)
	e.MustRun("scan")
	s := newCountingSteam(t)
	s.setAnswer(func(w http.ResponseWriter, ids []string) bool {
		var out []string
		for _, id := range ids {
			if id == "7402" {
				out = append(out, fmt.Sprintf(`{"publishedfileid":%q,"result":9}`, id))
			} else {
				out = append(out, fmt.Sprintf(`{"publishedfileid":%q,"result":1,"description":"checked text","time_updated":1790000000}`, id))
			}
		}
		fmt.Fprintf(w, `{"response":{"publishedfiledetails":[%s]}}`, strings.Join(out, ","))
		return true
	})
	e.App.SteamAPI = s.srv.URL
	e.App.CheckSubscriptionSource(app.LibWorkshop)
	if s.requests.Load() != 1 {
		t.Fatalf("check requests %d", s.requests.Load())
	}
	if d := describeFetch(t, e, "7401"); d.WorkshopState != "cached" || d.Workshop != "checked text" {
		t.Fatalf("checked Mod: %+v", d)
	}
	for range 3 {
		if d := describeFetch(t, e, "7402"); d.WorkshopState != "cached" || d.Workshop != "" {
			t.Fatalf("removed Mod: %+v", d)
		}
	}
	if s.requests.Load() != 1 {
		t.Fatalf("views sent %d requests", s.requests.Load()-1)
	}
	e.Advance(25 * time.Hour)
	if d := describeFetch(t, e, "7402"); d.WorkshopState != "fetched" || s.requests.Load() != 2 {
		t.Fatalf("a day later: %+v, requests %d", d, s.requests.Load())
	}
}
