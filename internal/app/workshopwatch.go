package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/steamworks"
)

func downloadFailure(result string) error {
	switch result {
	case "failed:download-stalled":
		return errors.New("Steam did not download it; check Steam (no download progress for 2 minutes)")
	case "failed:download-limit":
		return errors.New("Steam did not finish the download within 6 hours; check Steam")
	case "ok":
		return nil
	default:
		return fmt.Errorf("Steam did not download it (%s); check Steam", result)
	}
}

func (a *App) watchWorkshop(op *WorkshopOp, ids []string, b BatchCalls) error {
	if b.Stopped != nil && b.Stopped() {
		return context.Canceled
	}
	ctx, cancel := context.WithTimeout(context.Background(), steamworks.DownloadLimit+time.Minute)
	defer cancel()
	if b.Stopped != nil {
		go func() {
			tick := time.NewTicker(100 * time.Millisecond)
			defer tick.Stop()
			for {
				if b.Stopped() {
					cancel()
					return
				}
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
				}
			}
		}()
	}
	byID := map[string]int{}
	for i := range op.Items {
		byID[op.Items[i].WorkshopID] = i
	}
	for _, id := range ids {
		op.Items[byID[id]].Result = "timeout"
	}
	_, err := a.runSteamStreaming(ctx, steamworks.Watch, ids, func(line string) {
		if p, ok := steamworks.ParseProgress(line); ok {
			if i, found := byID[strconv.FormatUint(p.ID, 10)]; found {
				op.Items[i].State = p.State
				if b.Progress != nil {
					b.Progress(i, p.Got, p.Total)
				}
			}
		}
		for _, item := range steamworks.Parse(line).Items {
			if i, found := byID[strconv.FormatUint(item.ID, 10)]; found {
				op.Items[i].Result, op.Items[i].State = item.Result, item.State
				if b.Done != nil {
					b.Done(i, downloadFailure(item.Result))
				}
			}
		}
	})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		for _, id := range ids {
			i := byID[id]
			if op.Items[i].Result == "timeout" {
				op.Items[i].Result = "failed:download worker: " + err.Error()
				if b.Done != nil {
					b.Done(i, downloadFailure(op.Items[i].Result))
				}
			}
		}
		return err
	}
	for _, id := range ids {
		i := byID[id]
		if err := downloadFailure(op.Items[i].Result); err != nil {
			// Preserve the useful, actionable reason in CLI and GUI results.
			op.Items[i].Result = err.Error()
		}
	}
	return nil
}

// runSteamStreaming consumes stdout as it arrives; it retains final results only.
// SteamWorker remains supported for older buffered fakes; live fakes use SteamWorkerStream.
func (a *App) runSteamStreaming(ctx context.Context, action string, ids []string, line func(string)) (*steamworks.Report, error) {
	dir, err := a.SteamRuntime()
	if err != nil {
		return nil, &SteamUnavailable{Why: err.Error()}
	}
	all := &steamworks.Report{}
	for i := 0; i < len(ids); i += 50 {
		if err := ctx.Err(); err != nil {
			return all, err
		}
		r := &steamworks.Report{}
		consume := func(s string) {
			parsed := steamworks.Parse(s)
			r.Items = append(r.Items, parsed.Items...)
			if parsed.Error != "" {
				r.Error = parsed.Error
			}
			r.Done = r.Done || parsed.Done
			if line != nil {
				line(s)
			}
		}
		args := append([]string{action}, ids[i:min(i+50, len(ids))]...)
		switch {
		case a.SteamWorkerStream != nil:
			err = a.SteamWorkerStream(ctx, dir, args, consume)
		case a.SteamWorker != nil:
			var out string
			out, err = a.SteamWorker(dir, args)
			for _, s := range strings.Split(out, "\n") {
				consume(s)
			}
		default:
			err = steamWorkerStreaming(ctx, dir, args, consume)
		}
		all.Items = append(all.Items, r.Items...)
		if ctx.Err() != nil {
			return all, ctx.Err()
		}
		if r.Error != "" {
			return all, &SteamUnavailable{Why: r.Error}
		}
		if err != nil {
			return all, fmt.Errorf("Steam download worker: %w; check Steam", err)
		}
		if !r.Done {
			return all, errors.New("Steam download worker did not finish; check Steam")
		}
	}
	all.Done = true
	return all, nil
}

func steamWorkerStreaming(ctx context.Context, dir string, args []string, line func(string)) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, exe, append([]string{"steam-worker"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "SteamAppId="+strconv.Itoa(steamworks.AppID), "SteamGameId="+strconv.Itoa(steamworks.AppID), "RIMARCHO_STEAM_WATCH_PIPE=1")
	input, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer input.Close()
	// EOF lets the worker run Shutdown; WaitDelay kills it if the DLL hangs.
	cmd.Cancel = input.Close
	hideWindow(cmd)
	return streamSteamCommand(cmd, line)
}

func streamSteamCommand(cmd *exec.Cmd, line func(string)) error {
	cmd.WaitDelay = time.Second
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	sc := bufio.NewScanner(pipe)
	// Bound memory even if the DLL writes unrelated diagnostics.
	sc.Buffer(make([]byte, 4096), 1024*1024)
	for sc.Scan() {
		line(sc.Text())
	}
	if sc.Err() != nil {
		_ = cmd.Process.Kill()
	}
	return errors.Join(sc.Err(), cmd.Wait())
}
