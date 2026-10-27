package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseWorkshopFinalStatus(t *testing.T) {
	for _, tc := range []struct {
		name, output, kind string
		confirmed          bool
	}{
		{"harmless failure", "Failed to load optional cache\nERROR! Failed to connect to Steam servers", "network", false},
		{"guard prompt", "Logging in user 'player'...\nEnter your Steam Guard code:\nERROR! Failed to connect to Steam servers", "network", false},
		{"earlier error", "ERROR! Failed to load optional cache\nERROR! Connection failed", "network", false},
		{"recovered authentication", "Logging in user 'player'...FAILED (Invalid Password)\nLogging in user 'player'...OK\nERROR! Connection reset", "network", false},
		{"final authentication", "ERROR! Connection failed\nLogging in user 'player'...FAILED (Invalid Password)", "password", false},
		{"anonymous login", "Connecting anonymously to Steam Public...\nFAILED (Failure)", "anonymous-login", false},
		{"login connection failure", "ERROR! Failed to log in (No Connection)", "network", false},
		{"named login", "Logging in user 'player'...\nFAILED (Failure)", "login", false},
		{"write denied", "ERROR! Unable to create directory (Permission denied)", "disk", false},
		{"file write denied", "ERROR! Download item 8100 failed (Write failure: Access denied)", "disk", false},
		{"private item", "ERROR! Download item 8100 failed (AccessDenied)", "permission", false},
		{"item overrides connection", "ERROR! Download item 8100 failed (AccessDenied)\nERROR! Connection failed", "permission", false},
		{"success overrides global", "Success. Downloaded item 8100\nERROR! Connection failed", "", true},
		{"last item success", "ERROR! Timeout downloading item 8100\nSuccess. Downloaded item 8100", "", true},
		{"last item failure", "Success. Downloaded item 8100\nERROR! Timeout downloading item 8100", "network", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := parseWorkshopOutput([]string{"8100"}, tc.output, nil)["8100"]
			if r.kind != tc.kind || r.confirmed != tc.confirmed {
				t.Fatalf("outcome %+v, want %s confirmed=%v", r, tc.kind, tc.confirmed)
			}
		})
	}
	if got := errorKind("unable to create directory: permission denied"); got != "disk" {
		t.Fatalf("write denial classified as %s", got)
	}
	if got := (&DownloadError{Kind: errorKind("access denied")}).Why(); got != "SteamCMD did not finish the download" {
		t.Fatalf("game download wording changed: %s", got)
	}
}

func scriptIDs(t *testing.T, exe string, args []string) []string {
	t.Helper()
	if len(args) != 6 || args[4] != "+runscript" || filepath.Base(args[5]) != args[5] {
		t.Fatalf("script must use an ASCII filename relative to exe: %v", args)
	}
	for _, r := range args[5] {
		if r > 127 {
			t.Fatalf("non-ASCII script argument: %s", args[5])
		}
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(exe), args[5]))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) == 3 && f[0] == "workshop_download_item" {
			ids = append(ids, f[2])
		}
	}
	return ids
}

type workshopLoginProc func() error

func (p workshopLoginProc) Wait() error { return p() }

func TestWorkshopBatchSessionFailureStopsQueue(t *testing.T) {
	for _, tc := range []struct {
		name, output, account, kind string
		calls                       int
	}{
		{"password", "Logging in user 'player'...FAILED (Invalid Password)", "player", "password", 1},
		{"guard", "ERROR! InvalidAuthCode", "player", "guard", 1},
		{"login", "Logging in user 'player'...\nFAILED (Failure)", "player", "login", 1},
		{"anonymous login", "Connecting anonymously to Steam Public...\nFAILED (Failure)", "anonymous", "anonymous-login", 1},
		{"network", "ERROR! Failed to connect to Steam servers", "anonymous", "network", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			exe := filepath.Join(base, "steamcmd.exe")
			a := &App{SteamCMDWait: func(time.Duration) {}}
			var calls [][]string
			run := func(args []string) (string, error) {
				calls = append(calls, scriptIDs(t, exe, args))
				return tc.output, nil
			}
			a.SteamCMDRun = func(_ string, args []string) (string, error) {
				if tc.account != "anonymous" {
					t.Fatal("named account ran hidden")
				}
				return run(args)
			}
			a.SteamCMDStart = func(_ string, args []string, console bool) (SteamCMDProc, error) {
				if !console {
					t.Fatal("named account lost native console")
				}
				return workshopLoginProc(func() error {
					out, _ := run(args)
					if err := os.MkdirAll(filepath.Join(base, "logs"), 0755); err != nil {
						return err
					}
					f, err := os.OpenFile(filepath.Join(base, "logs", "console_log.txt"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
					if err != nil {
						return err
					}
					_, err = fmt.Fprintln(f, out)
					return errors.Join(err, f.Close())
				}), nil
			}
			var ids []string
			for i := 0; i < 100; i++ {
				ids = append(ids, fmt.Sprint(8100+i))
			}
			outcomes, err := a.runWorkshopBatches(exe, base, tc.account, ids)
			if err != nil || len(calls) != tc.calls {
				t.Fatalf("starts=%d, error=%v", len(calls), err)
			}
			for _, batch := range calls {
				if !slices.Equal(batch, ids[:25]) {
					t.Fatalf("started later batch: %v", batch)
				}
			}
			for _, id := range ids {
				if r := outcomes[id]; r.confirmed || r.kind != tc.kind {
					t.Fatalf("ID %s: %+v", id, r)
				}
			}
			leftovers, err := filepath.Glob(filepath.Join(base, "RimArcho-workshop-*.txt"))
			if err != nil || len(leftovers) != 0 {
				t.Fatalf("script cleanup: %v %v", leftovers, err)
			}
		})
	}
}

func TestWorkshopInvalidIDDoesNotPoisonBatch(t *testing.T) {
	base := filepath.Join(t.TempDir(), "中文用户名")
	if err := os.Mkdir(base, 0755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(base, "steamcmd.exe")
	a := &App{}
	var calls [][]string
	a.SteamCMDRun = func(_ string, args []string) (string, error) {
		ids := scriptIDs(t, exe, args)
		calls = append(calls, ids)
		var out []string
		for _, id := range ids {
			out = append(out, "Success. Downloaded item "+id)
		}
		return strings.Join(out, "\n"), nil
	}
	bad := "8101\nquit"
	results, err := a.runWorkshopBatches(exe, base, "anonymous", []string{"8100", bad, "0", "8102", "8100"})
	if err != nil || len(calls) != 1 || !slices.Equal(calls[0], []string{"8100", "8102"}) {
		t.Fatalf("calls %v, error %v", calls, err)
	}
	if !results["8100"].confirmed || !results["8102"].confirmed || !strings.Contains(results[bad].why(), "invalid Workshop ID") || !strings.Contains(results["0"].why(), "invalid Workshop ID") {
		t.Fatalf("outcomes: %+v", results)
	}
	results, err = a.runWorkshopBatches(exe, base, "anonymous", []string{bad})
	if err != nil || len(calls) != 1 || results[bad].err == nil {
		t.Fatalf("invalid-only queue: %+v %v", results, err)
	}
}

func TestWorkshopPreparationErrorRetained(t *testing.T) {
	base := t.TempDir()
	exe := filepath.Join(base, "missing-tools", "steamcmd.exe")
	a := &App{SteamCMDRun: func(string, []string) (string, error) {
		t.Fatal("runner called after script creation failed")
		return "", nil
	}}
	results, err := a.runWorkshopBatches(exe, base, "anonymous", []string{"8100", "8101"})
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		var pathErr *os.PathError
		if !errors.As(result.err, &pathErr) || !strings.Contains(result.why(), "missing-tools") || result.why() != result.err.Error() {
			t.Fatalf("preparation reason lost: %+v", result)
		}
	}
	startErr := &os.PathError{Op: "fork/exec", Path: exe, Err: os.ErrPermission}
	r := parseWorkshopOutput([]string{"8100"}, "", startErr)["8100"]
	if !errors.Is(r.err, os.ErrPermission) || r.why() != startErr.Error() {
		t.Fatalf("start reason lost: %+v", r)
	}
}
