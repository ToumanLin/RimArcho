package app

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
)

// SteamCMD handles commands serially; limit each process's queue and lifetime.
const workshopBatchSize = 25

var (
	workshopSuccess = regexp.MustCompile(`(?i)Success[.!]\s+Downloaded (?:workshop )?item\s+(\d+)\b`)
	workshopFailure = regexp.MustCompile(`(?i)ERROR!\s+(?:Download(?:ing)? (?:workshop )?item|Timeout downloading (?:workshop )?item)\s+(\d+)\b[^\r\n]*`)
)

type workshopOutcome struct {
	confirmed bool
	kind      string
	err       error // local preparation errors retain their path and OS reason
}

func (o workshopOutcome) why() string {
	if o.err != nil {
		return o.err.Error()
	}
	if o.kind == "permission" {
		return "this content is private, unavailable, or access was denied; check account access"
	}
	if o.kind == "" {
		return "download was not confirmed"
	}
	return (&DownloadError{Kind: o.kind}).Why()
}

func parseWorkshopOutput(ids []string, out string, runErr error) map[string]workshopOutcome {
	results := map[string]workshopOutcome{}
	globalKind := ""
	loginPending, anonymous := false, false
	for _, line := range strings.Split(out, "\n") {
		if m := workshopFailure.FindStringSubmatch(line); m != nil {
			results[m[1]] = workshopOutcome{kind: workshopErrorKind(strings.ToLower(m[0]))}
			continue
		}
		if m := workshopSuccess.FindStringSubmatch(line); m != nil {
			results[m[1]] = workshopOutcome{confirmed: true}
			continue
		}
		low := strings.ToLower(strings.TrimSpace(stampRe.ReplaceAllString(line, "")))
		if low == "" {
			continue
		}
		if strings.Contains(low, "logging in user") || strings.Contains(low, "connecting anonymously") || strings.Contains(low, "waiting for user info") || strings.Contains(low, "waiting for client config") {
			loginPending = true
			if strings.Contains(low, "connecting anonymously") {
				anonymous = true
			}
		}
		if loginPending && (low == "ok" || strings.HasSuffix(low, "...ok")) {
			globalKind, loginPending = "", false
			continue
		}
		// Prompts (including Steam Guard) and incidental failed lookups are not
		// terminal failures. The last explicit failure describes unconfirmed IDs.
		failed := strings.Contains(low, "error!") || strings.Contains(low, "failed") || strings.Contains(low, "unable to") || strings.Contains(low, "not connected")
		kind := workshopErrorKind(low)
		if kind == "password" || strings.Contains(low, "invalidauthcode") || strings.Contains(low, "invalidloginauthcode") {
			failed = true
		}
		if !failed {
			continue
		}
		if strings.Contains(low, "rate limit") {
			kind = "other"
		} else if kind == "other" && loginPending {
			kind = "login"
			if anonymous {
				kind = "anonymous-login"
			}
		} else if kind == "other" && !strings.Contains(low, "error!") {
			continue
		}
		globalKind, loginPending = kind, false
	}
	if globalKind == "" && runErr != nil {
		var download *DownloadError
		var network net.Error
		switch {
		case errors.As(runErr, &download):
			globalKind = download.Kind
		case errors.As(runErr, &network):
			globalKind = "network"
		default:
			globalKind = "other"
		}
	}
	var localErr error
	if out == "" && runErr != nil {
		var pathErr *os.PathError
		var execErr *exec.Error
		if errors.As(runErr, &pathErr) || errors.As(runErr, &execErr) {
			localErr = runErr
		}
	}
	requested := make(map[string]workshopOutcome, len(ids))
	for _, id := range ids {
		if result, ok := results[id]; ok {
			requested[id] = result
		} else {
			requested[id] = workshopOutcome{kind: globalKind, err: localErr}
		}
	}
	return requested
}

func (a *App) runWorkshopBatches(exe, base, account string, ids []string) (map[string]workshopOutcome, error) {
	var processErrors []error
	results := map[string]workshopOutcome{}
	var validIDs []string
	for _, id := range ids {
		if _, seen := results[id]; seen {
			continue
		}
		valid, err := workshopIDs([]string{id})
		if err != nil || len(valid) != 1 || valid[0] != id {
			results[id] = workshopOutcome{err: fmt.Errorf("invalid Workshop ID %q", id)}
			continue
		}
		results[id] = workshopOutcome{}
		validIDs = append(validIDs, id)
	}
	for start := 0; start < len(validIDs); start += workshopBatchSize {
		end := min(start+workshopBatchSize, len(validIDs))
		pending := validIDs[start:end]
		for attempt := 0; len(pending) > 0; attempt++ {
			if attempt > 0 {
				wait := time.Sleep
				if a.SteamCMDWait != nil {
					wait = a.SteamCMDWait
				}
				wait([]time.Duration{5 * time.Second, 15 * time.Second}[attempt-1])
			}
			out, ran, err := a.runWorkshopScript(exe, base, account, pending)
			if !ran {
				for _, id := range append(append([]string(nil), pending...), validIDs[end:]...) {
					results[id] = workshopOutcome{err: err}
				}
				return results, errors.Join(processErrors...)
			}
			parsed := parseWorkshopOutput(pending, out, err)
			confirmed := 0
			var retry []string
			stopKind := ""
			allNetwork := true
			for _, id := range pending {
				r := parsed[id]
				results[id] = r
				if r.confirmed {
					confirmed++
				}
				switch r.kind {
				case "password", "guard", "login", "anonymous-login":
					stopKind = r.kind
				}
				allNetwork = allNetwork && r.kind == "network"
				if !r.confirmed && r.kind == "network" && attempt < 2 {
					retry = append(retry, id)
				}
			}
			if err != nil && confirmed == len(pending) {
				processErrors = append(processErrors, errors.New("SteamCMD did not exit successfully; confirmed items were retained"))
			}
			if confirmed == 0 && (stopKind != "" || allNetwork && attempt == 2) {
				if stopKind == "" {
					stopKind = "network"
				}
				for _, id := range validIDs[end:] {
					results[id] = workshopOutcome{kind: stopKind}
				}
				return results, errors.Join(processErrors...)
			}
			pending = retry
		}
	}
	return results, errors.Join(processErrors...)
}

func (a *App) runWorkshopScript(exe, base, account string, ids []string) (string, bool, error) {
	f, err := os.CreateTemp(filepath.Dir(exe), "RimArcho-workshop-*.txt")
	if err != nil {
		return "", false, err
	}
	defer os.Remove(f.Name())
	// Do not stop on one item failure: SteamCMD often exits 0 even for failed items.
	var script strings.Builder
	script.WriteString("@ShutdownOnFailedCommand 0\n@NoPromptForPassword 0\n")
	for _, id := range ids {
		fmt.Fprintf(&script, "workshop_download_item %s %s\n", env.WorkshopAppID, id)
	}
	script.WriteString("quit\n")
	_, writeErr := f.WriteString(script.String())
	closeErr := f.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return "", false, err
	}
	// Pass only an ASCII filename: SteamCMD resolves it against its working
	// directory, so non-ASCII profile/base paths never enter the script argument.
	args := []string{"+force_install_dir", base, "+login", account, "+runscript", filepath.Base(f.Name())}
	out, err := a.runWorkshopSteamCMD(exe, args, account != "anonymous")
	return out, true, err
}

// Workshop access errors need item context; game download diagnostics keep their
// existing wording. Local write errors must never be described as private content.
func workshopErrorKind(low string) string {
	if kind := errorKind(low); kind != "other" {
		return kind
	}
	if strings.Contains(low, "access denied") || strings.Contains(low, "accessdenied") || strings.Contains(low, "permission") || strings.Contains(low, "private") || strings.Contains(low, "not visible") || strings.Contains(low, "file not found") || strings.Contains(low, "filenotfound") {
		return "permission"
	}
	return errorKind(low)
}
