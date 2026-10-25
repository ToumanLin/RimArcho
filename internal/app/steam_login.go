package app

import "strings"

// steamLogin pairs SteamCMD steps with their results, which can be on the
// same line or on the next nonempty line (including timestamp-only lines).
// account is empty for download progress, which accepts any selected identity.
type steamLogin struct {
	account   string
	pending   string
	identity  bool
	connected bool
	ready     bool
	err       *DownloadError
}

func (s *steamLogin) read(line string) bool {
	line = strings.TrimSpace(stampRe.ReplaceAllString(line, ""))
	if line == "" {
		return false
	}
	low := strings.ToLower(line)
	step := ""
	switch {
	case strings.Contains(low, "logging in user") || strings.Contains(low, "connecting anonymously"):
		step = "login"
		s.connected, s.ready = false, false
		s.identity = s.account == "" ||
			s.account == "anonymous" && strings.Contains(low, "connecting anonymously") ||
			s.account != "anonymous" && strings.Contains(low, "logging in user '"+strings.ToLower(s.account)+"'")
	case strings.Contains(low, "waiting for user info"):
		step = "user"
	case strings.Contains(low, "waiting for client config"):
		step = "config"
	}
	if step != "" {
		s.pending = step
	}
	kind := ""
	switch {
	case strings.Contains(low, "invalid password") || strings.Contains(low, "invalidpassword"):
		kind = "password"
	case strings.Contains(low, "two-factor") || strings.Contains(low, "logon denied") || strings.Contains(low, "invalidauthcode") || strings.Contains(low, "steam guard"):
		kind = "guard"
	case strings.Contains(low, "no connection") || strings.Contains(low, "timeout") || strings.Contains(low, "rate limit"):
		kind = "network"
	case strings.Contains(low, "failed") && (s.pending != "" || low == "failed" || strings.HasPrefix(low, "failed (")):
		kind = "login"
	}
	if kind != "" {
		s.err = &DownloadError{Kind: kind, Line: line}
		s.pending, s.ready = "", false
		return true
	}
	if low == "ok" || step != "" && strings.HasSuffix(low, "ok") {
		pending := s.pending
		s.pending = ""
		switch pending {
		case "login":
			s.connected = s.identity
		case "user":
			s.ready = s.connected && s.err == nil
		}
		return pending != ""
	}
	if step == "" {
		s.pending = ""
	}
	return step != ""
}
