package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"
)

// Typing the password into SteamCMD's window (11 §3 "Password prompt"): the player gives
// RimArcho the password or a Steam Guard code when SteamCMD's own window asks for it, and
// RimArcho types it into that window as console key input. It never goes on a command line,
// into a pipe, the environment, a file, a log, the database, a task or a report.

// SteamSecret is a password or Steam Guard code on its way into SteamCMD's window, kept as
// UTF-16 so it can be overwritten (Wipe) once typed. It formats as "[redacted]" and refuses
// to be marshalled, so a stray log or JSON call cannot print it.
type SteamSecret struct{ b []uint16 }

// NewSteamSecret takes text a surface just read; the caller drops its own copy.
func NewSteamSecret(text string) *SteamSecret {
	if text == "" {
		return nil
	}
	return &SteamSecret{b: utf16.Encode([]rune(text))}
}

// NewSteamSecretUTF16 takes ownership of b (the CLI's console read).
func NewSteamSecretUTF16(b []uint16) *SteamSecret {
	if len(b) == 0 {
		return nil
	}
	return &SteamSecret{b: b}
}

// Wipe overwrites the secret; it is empty afterwards.
func (s *SteamSecret) Wipe() {
	if s != nil {
		clear(s.b)
		s.b = nil
	}
}

// Empty reports whether nothing (is left) to type.
func (s *SteamSecret) Empty() bool { return s == nil || len(s.b) == 0 }

// Format keeps the secret out of every fmt verb.
func (s *SteamSecret) Format(f fmt.State, _ rune) { f.Write([]byte("[redacted]")) }

// MarshalJSON refuses: a secret is never serialised.
func (s *SteamSecret) MarshalJSON() ([]byte, error) {
	return nil, errors.New("a SteamCMD secret is never serialised")
}

// SteamCMDPrompter answers SteamCMD's window when it asks the player something (App.
// SteamCMDPrompter). The window surfaces ask for the value at that moment; the CLI asked
// before SteamCMD started.
type SteamCMDPrompter interface {
	// Secret answers the prompt kind, "password" or "guard"; nil leaves it to the player,
	// who types it in the window. ctx ends when the run does.
	Secret(ctx context.Context, kind string) *SteamSecret
	// Phone says SteamCMD waits for the sign-in to be approved in the Steam Mobile app.
	Phone()
}

// steamPromptKind names the prompt on SteamCMD's cursor line, or "".
func steamPromptKind(line string) string {
	low := strings.ToLower(strings.TrimSpace(line))
	if strings.HasSuffix(low, "password:") {
		return "password"
	}
	for _, p := range []string{"steam guard code:", "two-factor code:", "two factor code:", "auth code:", "authenticator code:"} {
		if strings.HasSuffix(low, p) {
			return "guard"
		}
	}
	return ""
}

// steamPhoneLine: SteamCMD waits for the mobile app's approval.
func steamPhoneLine(line string) bool {
	low := strings.ToLower(line)
	return strings.Contains(low, "steam mobile app") || strings.Contains(low, "confirm the login") || strings.Contains(low, "confirm this login")
}
