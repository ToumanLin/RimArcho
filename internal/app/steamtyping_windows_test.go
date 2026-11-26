package app_test

import (
	"bytes"
	"context"
	testhide "github.com/ToumanLin/RimArcho/internal/testkit/hide"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"unicode/utf16"

	"golang.org/x/sys/windows"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// A fake steamcmd.exe: SteamCMD's prompts in its own console, read from console input (11 §3).
// The right password and code are compiled in; nothing reaches it any other way.
const typingStub = `package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	go func() { time.Sleep(30 * time.Second); os.Exit(3) }()
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	os.WriteFile(filepath.Join(dir, "args.txt"), []byte(strings.Join(os.Args, " ")), 0o644)
	os.MkdirAll(filepath.Join(dir, "logs"), 0o755)
	log, _ := os.OpenFile(filepath.Join(dir, "logs", "console_log.txt"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	defer log.Close()
	args := strings.Join(os.Args[1:], " ")
	if strings.Contains(args, "+licenses_print") {
		if b, _ := os.ReadFile(filepath.Join(dir, "signed-in")); len(b) > 0 {
			fmt.Fprintln(log, "Logging in user 'fixture-player' to Steam Public...OK")
			fmt.Fprintln(log, "Waiting for user info...OK")
		} else {
			fmt.Fprintln(log, "Cached credentials not found.")
		}
		return
	}
	in := bufio.NewReader(os.Stdin)
	fmt.Println("Steam Console Client (c) Valve Corporation - version 0 (test stub)")
	fmt.Println("Cached credentials not found.")
	fmt.Print("password: ")
	pw, _ := in.ReadString('\n')
	if strings.TrimRight(pw, "\r\n") != "Fixture-Pässwort 7731!" {
		fmt.Println("Logging in user 'fixture-player' [U:1:0] to Steam Public...FAILED (Invalid Password)")
		time.Sleep(500 * time.Millisecond)
		return
	}
	fmt.Print("Steam Guard code: ")
	code, _ := in.ReadString('\n')
	if strings.TrimRight(code, "\r\n") != "ABC12" {
		fmt.Println("Logging in user 'fixture-player' [U:1:0] to Steam Public...FAILED (Two-factor code mismatch)")
		time.Sleep(500 * time.Millisecond)
		return
	}
	fmt.Println("Logging in user 'fixture-player' [U:1:0] to Steam Public...OK")
	os.WriteFile(filepath.Join(dir, "signed-in"), []byte("1"), 0o644)
}
`

const fixturePassword = "Fixture-Pässwort 7731!"

type typingPrompter struct {
	mu       sync.Mutex
	password string
	asked    []string
}

func (p *typingPrompter) Secret(_ context.Context, kind string) *app.SteamSecret {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.asked = append(p.asked, kind)
	if kind == "guard" {
		return app.NewSteamSecret("ABC12")
	}
	return app.NewSteamSecret(p.password)
}

func (p *typingPrompter) Phone() {}

func buildTypingStub(t *testing.T, dir string) string {
	t.Helper()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte(typingStub), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte("module stub\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, deps.SteamCMDExecutable())
	os.MkdirAll(dir, 0o755)
	cmd := testhide.Cmd(exec.Command("go", "build", "-o", exe, "."))
	cmd.Dir = src
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the stub: %v\n%s", err, out)
	}
	return exe
}

// leaks lists the files under roots that hold the password, as UTF-8 or UTF-16.
func leaks(t *testing.T, password string, roots ...string) []string {
	t.Helper()
	u8 := []byte(password)
	var u16 []byte
	for _, c := range utf16.Encode([]rune(password)) {
		u16 = append(u16, byte(c), byte(c>>8))
	}
	var out []string
	for _, root := range roots {
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(p) == ".exe" { // the stub has it compiled in
				return nil
			}
			b, err := os.ReadFile(p)
			if err == nil && (bytes.Contains(b, u8) || bytes.Contains(b, u16)) {
				out = append(out, p)
			}
			return nil
		})
	}
	return out
}

// 11 §3 "Password prompt" (#19): RimArcho types the password and the Steam Guard code into
// SteamCMD's own console when it asks, and the password ends up nowhere: not in the data
// folder (database, logs, operation log, tasks), the local folder, SteamCMD's command line or
// a feedback report with every attachment.
func TestSteamCMDTypedPasswordLeavesNoTrace(t *testing.T) {
	// The window has no console of its own; a test binary may have one.
	windows.NewLazySystemDLL("kernel32.dll").NewProc("FreeConsole").Call()
	for _, tc := range []struct {
		name, password, want string
		asked                []string
	}{
		{"accepted", fixturePassword, "", []string{"password", "guard"}},
		{"refused", fixturePassword + "x", app.SteamPasswordRefused, []string{"password"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := testkit.New(t)
			e.App.SteamCMDNoPath = true
			dir := filepath.Join(e.App.LocalDir(), "steamcmd")
			buildTypingStub(t, dir)
			e.MustRun("config", "set", "steamcmd.account", "fixture-player")
			p := &typingPrompter{password: tc.password}
			e.App.SteamCMDPrompter = p
			e.App.SteamCMDPoll = 50e6
			err := e.App.SteamAccountAction("login")
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("sign-in failed: %v", err)
			case tc.want != "" && (err == nil || err.Error() != tc.want):
				t.Fatalf("got %v, want %q", err, tc.want)
			}
			if tc.want == "" && !e.App.SteamAccountStatus().LoggedIn {
				t.Fatal("not signed in")
			}
			if got := p.asked; len(got) != len(tc.asked) || got[0] != tc.asked[0] {
				t.Fatalf("asked %v, want %v", got, tc.asked)
			}
			args, _ := os.ReadFile(filepath.Join(dir, "args.txt"))
			if len(args) == 0 || bytes.Contains(args, []byte("Pässwort")) || bytes.Contains(args, []byte("ABC12")) {
				t.Fatalf("SteamCMD's command line: %q", args)
			}
			// A report with every attachment, as Feedback sends it.
			st, _ := e.App.Loadout()
			at := view.FeedbackAttachments(e.App, st, view.FeedbackOptions{Mods: true, Paths: true, Log: true, Ops: true, Problems: true}, nil)
			report := view.ComposeFeedback("SteamCMD sign-in", "it asked for the password", at)
			if bytes.Contains([]byte(report.Body+report.URL), []byte("Pässwort")) {
				t.Fatal("the password is in a feedback report")
			}
			e.App.Close()
			if got := leaks(t, tc.password, e.Home, e.App.LocalDir()); len(got) > 0 {
				t.Fatalf("the password was written to %v", got)
			}
		})
	}
}

// The secret never prints: every fmt verb and JSON say nothing of it.
func TestSteamSecretNeverPrints(t *testing.T) {
	s := app.NewSteamSecret(fixturePassword)
	for _, got := range []string{fmtAll(s)} {
		if bytes.Contains([]byte(got), []byte("Pässwort")) {
			t.Fatalf("printed: %s", got)
		}
	}
	if _, err := jsonMarshal(s); err == nil {
		t.Fatal("a secret was marshalled")
	}
	s.Wipe()
	if !s.Empty() {
		t.Fatal("not wiped")
	}
}
