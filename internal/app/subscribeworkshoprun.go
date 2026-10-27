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
	workshopCurrent = regexp.MustCompile(`(?i)Downloading (?:workshop )?item\s+(\d+)\b`)
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
		return "access was denied; check account access"
	}
	if o.kind == "missing" {
		return workshopMissing
	}
	if o.kind == "crash" {
		return "SteamCMD exited before confirming the download"
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
	if (globalKind == "" || globalKind == "other") && runErr != nil {
		var download *DownloadError
		var network net.Error
		switch {
		case errors.As(runErr, &download):
			globalKind = download.Kind
		case errors.As(runErr, &network):
			globalKind = "network"
		default:
			globalKind = "crash"
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
			if !result.confirmed && result.kind == "other" && globalKind == "crash" {
				result.kind = "crash"
			}
			requested[id] = result
		} else {
			requested[id] = workshopOutcome{kind: globalKind, err: localErr}
		}
	}
	return requested
}

func (a *App) runWorkshopBatches(exe, base, account string, ids []string, transfer ...*workshopTransfer) (map[string]workshopOutcome, error) {
	w := workshopObserver(transfer)
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
	queue := validIDs
	for attempt := 0; attempt < 3 && len(queue) > 0; attempt++ {
		if attempt > 0 {
			delay := []time.Duration{5 * time.Second, 15 * time.Second}[attempt-1]
			if a.SteamCMDWait != nil {
				a.SteamCMDWait(delay)
			} else {
				timer := time.NewTimer(delay)
				select {
				case <-timer.C:
				case <-w.ctx.Done():
				}
				timer.Stop()
			}
		}
		var retry []string
		for start := 0; start < len(queue); start += workshopBatchSize {
			if w.ctx.Err() != nil {
				return results, w.ctx.Err()
			}
			end := min(start+workshopBatchSize, len(queue))
			pending := queue[start:end]
			w.line("Downloading item " + pending[0])
			out, ran, err := a.runWorkshopScript(exe, base, account, pending, w)
			if !ran {
				for _, id := range queue[start:] {
					results[id] = workshopOutcome{err: err}
				}
				return results, errors.Join(processErrors...)
			}
			parsed := parseWorkshopOutput(pending, out, err)
			confirmed := 0
			stopKind := ""
			for _, id := range pending {
				r := parsed[id]
				results[id] = r
				if w.ctx.Err() == nil || r.confirmed || workshopPermanent(r.kind) {
					w.outcome(id, r)
				}
				if r.confirmed {
					confirmed++
				}
				switch r.kind {
				case "password", "guard", "login", "anonymous-login":
					stopKind = r.kind
				}
				if !r.confirmed && r.err == nil && (r.kind == "network" || r.kind == "crash" || r.kind == "") && attempt < 2 {
					retry = append(retry, id)
				}
			}
			if err != nil && confirmed == len(pending) {
				processErrors = append(processErrors, errors.New("SteamCMD did not exit successfully; confirmed items were retained"))
			}
			if w.ctx.Err() != nil {
				return results, w.ctx.Err()
			}
			if confirmed == 0 && stopKind != "" {
				for _, id := range validIDs {
					if results[id].confirmed || workshopPermanent(results[id].kind) {
						continue
					}
					results[id] = workshopOutcome{kind: stopKind}
				}
				return results, errors.Join(processErrors...)
			}
		}
		queue = retry
	}
	return results, errors.Join(processErrors...)
}

func (a *App) runWorkshopScript(exe, base, account string, ids []string, transfer ...*workshopTransfer) (string, bool, error) {
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
	out, err := a.runWorkshopSteamCMD(exe, args, account != "anonymous", transfer...)
	return out, true, err
}

// Workshop access errors need item context; game download diagnostics keep their
// existing wording. Local write errors must never be described as private content.
func workshopErrorKind(low string) string {
	if errorKind(low) == "disk" {
		return "disk"
	}
	if strings.Contains(low, "file not found") || strings.Contains(low, "filenotfound") || strings.Contains(low, "private") || strings.Contains(low, "not visible") {
		return "missing"
	}
	// SteamCMD can report bare Failure for the item interrupted by a crashed
	// process. It supplies no permanent cause; retry it as unconfirmed.
	if strings.Contains(low, "failed (failure)") && workshopFailure.MatchString(low) {
		return ""
	}
	if kind := errorKind(low); kind != "other" {
		return kind
	}
	if strings.Contains(low, "access denied") || strings.Contains(low, "accessdenied") || strings.Contains(low, "permission") {
		return "permission"
	}
	return errorKind(low)
}
