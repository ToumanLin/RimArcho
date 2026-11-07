package app_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

type preflightTransport func(*http.Request) (*http.Response, error)

func (f preflightTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSubscribeWorkshopPreflightBothRoutesAndCredit(t *testing.T) {
	t.Parallel()
	for _, steamcmd := range []bool{true, false} {
		t.Run(fmt.Sprint(steamcmd), func(t *testing.T) {
			e := setup(t, false)
			e.App.HTTP = &http.Client{Transport: preflightTransport(func(*http.Request) (*http.Response, error) {
				body := `{"response":{"publishedfiledetails":[{"publishedfileid":"8100","result":9},{"publishedfileid":"8101","result":1,"consumer_app_id":480,"title":"Another game"},{"publishedfileid":"8102","result":1,"consumer_app_id":294100,"title":"Actual Mod","file_size":"900"}]}}`
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			var sent []string
			if steamcmd {
				e.App.FindTool = func(string) (string, bool) { return filepath.Join(e.Root, "steamcmd.exe"), true }
				e.App.SteamCMDRun = func(exe string, args []string) (string, error) {
					sent = workshopScriptIDs(t, exe, args)
					e.WriteMod(filepath.Join(args[1], "steamapps", "workshop", "content", "294100", "8102"), testkit.Sample("preflight.mod", "Actual Mod"))
					return "Success. Downloaded item 8102", nil
				}
			} else {
				steamDLL(e)
				e.App.SteamWorker = func(_ string, args []string) (string, error) {
					sent = args[1:]
					return steamworks.ItemLine(8102, "ok", 0) + "\n" + steamworks.DoneLine("subscribe", 1), nil
				}
			}
			var last []app.TaskItem
			op, err := e.App.SubscribeWorkshopBatch([]string{"8100", "8101", "8102"}, steamcmd, true, false, app.WorkshopCalls{Changed: func(items []app.TaskItem) { last = items }})
			if err == nil || fmt.Sprint(sent) != "[8102]" || len(op.Items) != 3 || op.Items[2].Name != "Actual Mod" {
				t.Fatalf("op=%+v sent=%v err=%v", op, sent, err)
			}
			for _, i := range []int{0, 1} {
				if !op.Items[i].Permanent || op.Items[i].FailureKind != "missing" || !last[i].Permanent {
					t.Fatalf("invalid item %+v", op.Items[i])
				}
			}
			if last[2].Size != 900 || last[2].Name != "Actual Mod" || last[2].State != app.ItemOK {
				t.Fatalf("task items %+v", last)
			}
			batches, e2 := e.App.Store.CreditBatches("")
			if e2 != nil {
				t.Fatal(e2)
			}
			if steamcmd {
				if len(batches) != 1 {
					t.Fatalf("batches %+v", batches)
				}
				items, e2 := e.App.Store.CreditItems(batches[0].ID)
				if e2 != nil || len(items) != 1 || items[0].WorkshopID != "8102" {
					t.Fatalf("credit items %+v err=%v", items, e2)
				}
			} else if len(batches) != 0 {
				t.Fatal("Steam client created credit batch")
			}
		})
	}
}

func TestSteamSubscribeCancellationKeepsCompletedCalls(t *testing.T) {
	t.Parallel()
	e := setup(t, false)
	steamDLL(e)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var ids []string
	for i := 0; i < 55; i++ {
		ids = append(ids, fmt.Sprint(8100+i))
	}
	calls := 0
	e.App.FindTool = func(string) (string, bool) {
		t.Fatal("cancelled Steam subscription fell back to SteamCMD")
		return "", false
	}
	e.App.SteamWorker = func(_ string, args []string) (string, error) {
		calls++
		var out []string
		for _, id := range args[1:] {
			var n uint64
			fmt.Sscan(id, &n)
			out = append(out, steamworks.ItemLine(n, "ok", 0))
		}
		out = append(out, steamworks.DoneLine("subscribe", len(args)-1))
		cancel()
		return strings.Join(out, "\n"), nil
	}
	var last []app.TaskItem
	op, err := e.App.SubscribeWorkshop(ids, false, false, app.WorkshopCalls{Context: ctx, Changed: func(items []app.TaskItem) { last = items }})
	if err == nil || calls != 1 || len(op.Items) != 55 {
		t.Fatalf("calls=%d op=%+v err=%v", calls, op, err)
	}
	for i, it := range last {
		want := app.ItemOK
		if i >= 50 {
			want = app.ItemSkip
		}
		if it.State != want {
			t.Fatalf("item %d = %+v, want %s", i, it, want)
		}
	}
}
