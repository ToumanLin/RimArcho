package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSteamLoginStepsAndDownloadProgress(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, account, text, kind string
		ready                     bool
	}{
		{name: "anonymous", account: "anonymous", ready: true},
		{name: "user", account: "fixture-player", ready: true},
		{name: "failed", account: "fixture-player", kind: "password"},
		{name: "one-line", account: "fixture-player", ready: true},
		{name: "wrong identity", account: "another-player", text: "Logging in user 'fixture-player'...OK\nWaiting for user info...OK"},
		{name: "incomplete login", text: "Connecting anonymously...OK\nWaiting for client config...OK"},
		{name: "unpaired OK", text: "Connecting anonymously...\nLoading Steam API...\nOK\nWaiting for user info...OK"},
		{name: "user info only", text: "Waiting for user info...\nOK"},
		{name: "failed user info", text: "Connecting anonymously...OK\nWaiting for user info...\nFAILED (No Connection)", kind: "network"},
		{name: "guard", text: "Logging in user 'fixture-player'...\nTwo-factor code mismatch", kind: "guard"},
		{name: "connection", text: "Connecting anonymously...\nNo Connection", kind: "network"},
		{name: "one-line failure", text: "Logging in user 'fixture-player'...FAILED (Invalid Password)", kind: "password"},
		{name: "failure then unrelated OK", text: "Connecting anonymously...\nFAILED (Invalid Password)\nWaiting for user info...\nOK", kind: "password"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := tc.text
			if text == "" {
				b, err := os.ReadFile(filepath.Join("testdata", "steam-login", tc.name+".log"))
				if err != nil {
					t.Fatal(err)
				}
				text = string(b)
			}
			login := steamLogin{account: tc.account}
			var download steamResult
			events := 0
			for _, line := range strings.Split(text, "\n") {
				login.read(line)
				if progress, ok := download.read(line); ok && progress.State == "login" {
					events++
				}
				if login.ready && !login.connected {
					t.Fatal("ready before login result")
				}
			}
			kind := ""
			if login.err != nil {
				kind = login.err.Kind
			}
			if login.ready != tc.ready || kind != tc.kind {
				t.Fatalf("ready=%v kind=%s, want %v %s", login.ready, kind, tc.ready, tc.kind)
			}
			if tc.name == "wrong identity" {
				return
			} // Downloads do not compare account names.
			kind = ""
			if download.err != nil {
				kind = download.err.Kind
			}
			if download.loggedIn != tc.ready || kind != tc.kind || events == 0 {
				t.Fatalf("download loggedIn=%v kind=%s events=%d", download.loggedIn, kind, events)
			}
			if tc.ready {
				progress, ok := download.read("Update state (0x61) downloading, progress: 12.50 (50 / 400)")
				if !ok || progress.State != "downloading" || progress.Percent != 12.5 {
					t.Fatal(progress, ok)
				}
			}
		})
	}
}

// A connection SteamCMD drops and retries mid-download is not a failure once it says the
// app is fully installed; one it never recovers from still is (VM run 2026-10-28).
func TestSteamDownloadRecoversFromDisconnect(t *testing.T) {
	t.Parallel()
	head := `[2026-10-28 15:47:59] Logging in user 'fixture-player' [U:1:1] to Steam Public...
[2026-10-28 15:48:00] OK
[2026-10-28 15:48:00] Waiting for client config...
[2026-10-28 15:48:00] OK
[2026-10-28 15:48:00] Waiting for user info...
[2026-10-28 15:48:00] OK
 Update state (0x61) downloading, progress: 62.01 (10152347554 / 16372745944)
[2026-10-28 16:07:02] 
steamcmd has been disconnected from steam with result 3 (No Connection)
[2026-10-28 16:07:02] Retrying... `
	for _, tc := range []struct {
		name, tail string
		ok         bool
	}{
		{"recovered", `
 Update state (0x61) downloading, progress: 99.00 (1 / 1)
[2026-10-28 16:24:55] Success! App '294100' fully installed.
[2026-10-28 16:24:55] Unloading Steam API...
[2026-10-28 16:24:55] OK`, true},
		{"never recovered", `
[2026-10-28 16:24:55] Unloading Steam API...
[2026-10-28 16:24:55] OK`, false},
	} {
		var r steamResult
		for _, line := range strings.Split(head+tc.tail, "\n") {
			r.read(line)
		}
		if tc.ok && (!r.success || r.err != nil) {
			t.Fatalf("%s: success=%v err=%v", tc.name, r.success, r.err)
		}
		if !tc.ok && (r.success || r.err == nil || r.err.Kind != "network") {
			t.Fatalf("%s: success=%v err=%v", tc.name, r.success, r.err)
		}
	}
}
