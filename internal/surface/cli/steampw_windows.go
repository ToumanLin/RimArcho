package cli

import (
	"context"
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// The CLI's SteamCMD password (11 §3 "Password prompt"): read from this console with echo
// off before SteamCMD starts in the same console, kept as UTF-16, typed into the console
// when SteamCMD asks, then wiped. Only when stdin and stdout are this console.

var procReadConsoleW = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReadConsoleW")

type cliPrompter struct {
	mu       sync.Mutex
	password *app.SteamSecret
}

func (p *cliPrompter) Secret(_ context.Context, kind string) *app.SteamSecret {
	if kind != "password" {
		return nil // a Steam Guard code is typed in this terminal by the player
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.password
	p.password = nil
	return s
}

func (p *cliPrompter) Phone() {}

// ask reads the password before a signed-in run; Enter alone leaves it to SteamCMD's prompt.
func (p *cliPrompter) ask(prompt func()) {
	in := windows.Handle(os.Stdin.Fd())
	var mode uint32
	if windows.GetConsoleMode(in, &mode) != nil {
		return
	}
	prompt()
	windows.SetConsoleMode(in, (mode|windows.ENABLE_LINE_INPUT|windows.ENABLE_PROCESSED_INPUT)&^windows.ENABLE_ECHO_INPUT)
	defer windows.SetConsoleMode(in, mode)
	buf := make([]uint16, 512)
	var n uint32
	r, _, _ := procReadConsoleW.Call(uintptr(in), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&n)), 0)
	os.Stderr.WriteString("\n")
	if r == 0 {
		clear(buf)
		return
	}
	end := int(n)
	for end > 0 && (buf[end-1] == '\r' || buf[end-1] == '\n') {
		end--
	}
	text := make([]uint16, end)
	copy(text, buf[:end])
	clear(buf)
	p.mu.Lock()
	p.password.Wipe()
	p.password = app.NewSteamSecretUTF16(text)
	p.mu.Unlock()
}

// wipe drops a password SteamCMD never asked for.
func (p *cliPrompter) wipe() {
	p.mu.Lock()
	p.password.Wipe()
	p.password = nil
	p.mu.Unlock()
}

// newCLIPrompter: the CLI types the password only when SteamCMD shares its console.
func newCLIPrompter() *cliPrompter {
	var mode uint32
	if windows.GetConsoleMode(windows.Handle(os.Stdin.Fd()), &mode) != nil || windows.GetConsoleMode(windows.Handle(os.Stdout.Fd()), &mode) != nil {
		return nil
	}
	return &cliPrompter{}
}
