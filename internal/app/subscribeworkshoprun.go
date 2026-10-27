package app

import (
	"errors"
	"fmt"
	"net"
	"os"
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
}

func (o workshopOutcome) why() string {
	if o.kind == "" {
		return "download was not confirmed"
	}
	return (&DownloadError{Kind: o.kind}).Why()
}

func parseWorkshopOutput(ids []string, out string, runErr error) map[string]workshopOutcome {
	results := map[string]workshopOutcome{}
	for _, m := range workshopSuccess.FindAllStringSubmatch(out, -1) {
		results[m[1]] = workshopOutcome{confirmed: true}
	}
	for _, m := range workshopFailure.FindAllStringSubmatch(out, -1) {
		// A failure wins over a success in the same run, including stale log lines.
		results[m[1]] = workshopOutcome{kind: errorKind(strings.ToLower(m[0]))}
	}
	globalKind := ""
	var login steamLogin
	for _, line := range strings.Split(out, "\n") {
		if workshopFailure.MatchString(line) || workshopSuccess.MatchString(line) {
			continue
		}
		low := strings.ToLower(line)
		kind := ""
		if login.read(line) && login.err != nil {
			kind = login.err.Kind
		}
		if strings.Contains(low, "error!") || strings.Contains(low, "failed") || strings.Contains(low, "unable to") || strings.Contains(low, "not connected") {
			if k := errorKind(low); k != "other" || kind == "" {
				kind = k
			}
		}
		if strings.Contains(low, "rate limit") {
			kind = "other"
		}
		if kind != "" && (globalKind == "" || globalKind == "network") {
			globalKind = kind
		}
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
	requested := make(map[string]workshopOutcome, len(ids))
	for _, id := range ids {
		if result, ok := results[id]; ok {
			requested[id] = result
		} else {
			requested[id] = workshopOutcome{kind: globalKind}
		}
	}
	return requested
}

func (a *App) runWorkshopBatches(exe, base, account string, ids []string) (map[string]workshopOutcome, error) {
	var processErrors []error
	results := map[string]workshopOutcome{}
	// Only failed IDs are requeued. Successful IDs are never downloaded again.
	pending := append([]string(nil), ids...)
	for attempt := 0; len(pending) > 0; attempt++ {
		if attempt > 0 {
			wait := time.Sleep
			if a.SteamCMDWait != nil {
				wait = a.SteamCMDWait
			}
			wait([]time.Duration{5 * time.Second, 15 * time.Second}[attempt-1])
		}
		var retry []string
		for start := 0; start < len(pending); start += workshopBatchSize {
			end := min(start+workshopBatchSize, len(pending))
			batch := pending[start:end]
			out, err := a.runWorkshopScript(exe, base, account, batch)
			for id, result := range parseWorkshopOutput(batch, out, err) {
				results[id] = result
			}
			allConfirmed := true
			for _, id := range batch {
				allConfirmed = allConfirmed && results[id].confirmed
				if r := results[id]; !r.confirmed && r.kind == "network" && attempt < 2 {
					retry = append(retry, id)
				}
			}
			if err != nil && allConfirmed {
				// Keep confirmed items, but do not hide a failed process or expose its log.
				processErrors = append(processErrors, errors.New("SteamCMD did not exit successfully; confirmed items were retained"))
			}
		}
		pending = retry
	}
	return results, errors.Join(processErrors...)
}

func (a *App) runWorkshopScript(exe, base, account string, ids []string) (string, error) {
	f, err := os.CreateTemp("", "RimArcho-workshop-*.txt")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	// Do not stop on one item failure: SteamCMD often exits 0 even for failed items.
	var script strings.Builder
	script.WriteString("@ShutdownOnFailedCommand 0\n@NoPromptForPassword 0\n")
	for _, id := range ids {
		// IDs can also come from source.json during updates; never treat them as code.
		valid, err := workshopIDs([]string{id})
		if err != nil || len(valid) != 1 || valid[0] != id {
			f.Close()
			return "", fmt.Errorf("invalid Workshop ID %q", id)
		}
		fmt.Fprintf(&script, "workshop_download_item %s %s\n", env.WorkshopAppID, id)
	}
	script.WriteString("quit\n")
	_, writeErr := f.WriteString(script.String())
	closeErr := f.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return "", err
	}
	args := []string{"+force_install_dir", base, "+login", account, "+runscript", f.Name()}
	return a.runWorkshopSteamCMD(exe, args, account != "anonymous")
}
