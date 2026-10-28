package app_test

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
)

// VM run B11: SteamCMD signing in with the account the Steam client uses signs the client
// out; the dry run says so, and Steam's "offline" (code 3) says what it is.
func TestSteamCMDSameAccountAndOffline(t *testing.T) {
	e := setup(t, false)
	e.App.FindTool = func(string) (string, bool) { return filepath.Join(e.Root, "steamcmd.exe"), true }
	note := fmt.Sprintf(app.SteamSignOutNote, "player")
	for _, tc := range []struct {
		account, client string
		signedIn, want  bool
	}{
		{"player", "Player", true, true},
		{"player", "someone", true, false},
		{"player", "", false, false},
		{"", "player", true, false}, // anonymous
	} {
		if err := e.App.Store.SetMeta("cfg:steamcmd.account", tc.account); err != nil {
			t.Fatal(err)
		}
		e.App.SteamSignedIn = func() (string, bool) { return tc.client, tc.signedIn }
		op, err := e.App.SubscribeWorkshop([]string{"8102"}, true, true)
		if err != nil {
			t.Fatal(err)
		}
		if got := slices.Contains(op.Notes, note); got != tc.want {
			t.Fatalf("%+v: notes %q", tc, op.Notes)
		}
	}

	steamDLL(e)
	e.App.HTTP = &http.Client{Transport: preflightTransport(func(*http.Request) (*http.Response, error) {
		body := `{"response":{"publishedfiledetails":[{"publishedfileid":"8102","result":1,"consumer_app_id":294100,"title":"Actual Mod","file_size":"900"}]}}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	e.App.SteamWorker = func(_ string, args []string) (string, error) {
		return steamworks.ItemLine(8102, "failed:3", 0) + "\n" + steamworks.DoneLine("subscribe", 1), nil
	}
	var last []app.TaskItem
	op, err := e.App.SubscribeWorkshop([]string{"8102"}, false, false, app.WorkshopCalls{Changed: func(items []app.TaskItem) { last = items }})
	if err == nil || !strings.Contains(err.Error(), "the Steam client is offline") {
		t.Fatalf("err %v", err)
	}
	if it := op.Items[0]; it.FailureKind != "offline" || it.Permanent || it.Result != "failed: the Steam client is offline" {
		t.Fatalf("item %+v", it)
	}
	if len(last) != 1 || last[0].FailureKind != "offline" || last[0].State != app.ItemFail {
		t.Fatalf("task items %+v", last)
	}
}
