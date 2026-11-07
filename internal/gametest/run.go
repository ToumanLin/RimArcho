// Package gametest runs RimWorld with isolated data and captures startup logs (20 §2).
package gametest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
)

type Options struct {
	GameRoot, GameVersion string
	// The caller writes DataDir/Config/ModsConfig.xml before Run. Caches are reused.
	DataDir string
	// RunDir receives Player.log, combined stdout/stderr in stdout.log, and result.json.
	// These three files are replaced on each run.
	RunDir string
	Map    bool
	// Probe: run a test map (Map is implied) with -probequit until the probe's RESULT or
	// EXCEPTION line, then wait up to Settle for the game to quit by itself (20 §3).
	Probe bool
	// Zero durations use defaults of 10m, 5s (15s in a probe run) and 15s. Negative values are rejected.
	Timeout, Settle, MapQuiet time.Duration
	// ExtraArgs cannot override the managed data, log or quicktest arguments.
	ExtraArgs []string
	Now       func() time.Time
	// Start replaces cmd.Start and must start the supplied command on success.
	Start func(*exec.Cmd) error
}

type Result struct {
	Links        []*source.LinkInfo `json:"links,omitempty"`
	Status       string             // menu, map, probe, timeout, exited, start-failed or links-blocked
	ExitCode     *int
	LoadSeconds  float64 // launch to menu marker, confirmed map quiet period or the probe's last line
	TotalSeconds float64
	PeakMemoryMB int64
	LogPath      string
	Args         []string
	Exe          string
	Waylith      bool
}

// Run waits for the menu, a quiet test map or a probe's last line, then kills and reaps the process.
// Timeout and early exit return a result with no error; cancellation returns ctx.Err().
// Invalid options and setup failures return no result. Start failures return both.
func Run(ctx context.Context, o Options) (result *Result, runErr error) {
	if o.Timeout < 0 || o.Settle < 0 || o.MapQuiet < 0 {
		return nil, fmt.Errorf("gametest: durations must not be negative")
	}
	for _, arg := range o.ExtraArgs {
		key, _, _ := strings.Cut(strings.ToLower(strings.TrimLeft(arg, "-/")), "=")
		switch key {
		case "savedatafolder", "logfile", "nofilelog", "quicktest", "probequit":
			return nil, fmt.Errorf("gametest: extra argument %q overrides a managed option", arg)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	links, err := source.CheckModsLinks(filepath.Join(o.GameRoot, "Mods"))
	if err != nil {
		return nil, err
	}
	if len(links) > 0 {
		return &Result{Status: "links-blocked", Links: links}, nil
	}
	if err := preparePaths(&o); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Start == nil {
		o.Start = (*exec.Cmd).Start
	}
	if o.Timeout == 0 {
		o.Timeout = 10 * time.Minute
	}
	if o.Probe {
		o.Map = true
	}
	if o.Settle == 0 {
		o.Settle = 5 * time.Second
		if o.Probe {
			o.Settle = 15 * time.Second
		}
	}
	if o.MapQuiet == 0 {
		o.MapQuiet = 15 * time.Second
	}
	if o.GameVersion == "" {
		o.GameVersion = env.ReadGameVersion(o.GameRoot)
	}
	for _, dir := range []string{filepath.Join(o.DataDir, "Config"), o.RunDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("gametest: create %q: %w", dir, err)
		}
	}
	if err := resetPreferences(o.DataDir); err != nil {
		return nil, err
	}
	knowledge := filepath.Join(o.DataDir, "Config", "Knowledge.xml")
	if err := os.Remove(knowledge); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("gametest: remove old marker %q: %w", knowledge, err)
	}
	logPath := filepath.Join(o.RunDir, "Player.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("gametest: create log %q: %w", logPath, err)
	}
	defer func() {
		if logFile != nil {
			runErr = errors.Join(runErr, logFile.Close())
		}
	}()
	stdout, err := os.Create(filepath.Join(o.RunDir, "stdout.log"))
	if err != nil {
		return nil, fmt.Errorf("gametest: create stdout capture: %w", err)
	}
	defer func() { runErr = errors.Join(runErr, stdout.Close()) }()

	exe, waylith := executable(o)
	args := []string{"-savedatafolder=" + o.DataDir}
	if waylith {
		args = append(args, "-NoFileLog")
	} else {
		args = append(args, "-logfile", logPath, "-popupwindow")
	}
	if o.Map {
		args = append(args, "quicktest")
	}
	if o.Probe {
		args = append(args, ProbeQuit)
	}
	args = append(args, o.ExtraArgs...)
	cmd := exec.Command(exe, args...)
	cmd.Dir = o.GameRoot
	detach(cmd)
	// Bound output draining if a child inherits the capture pipe.
	cmd.WaitDelay = time.Second
	cmd.Stdout, cmd.Stderr = stdout, stdout
	launch := o.Now()
	result = &Result{Status: "start-failed", LogPath: logPath, Args: args, Exe: exe, Waylith: waylith}
	defer func() {
		result.TotalSeconds = o.Now().Sub(launch).Seconds()
		b, err := json.MarshalIndent(result, "", "  ")
		if err == nil {
			err = os.WriteFile(filepath.Join(o.RunDir, "result.json"), append(b, '\n'), 0o644)
		}
		if err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("gametest: save result: %w", err))
		}
	}()
	var console *consoleLog
	if waylith {
		console, err = newConsoleLog(logFile, o.GameVersion, launch)
		if err != nil {
			return result, err
		}
		output := io.MultiWriter(stdout, console)
		cmd.Stdout, cmd.Stderr = output, output
	} else {
		// Unity opens its own log handle; do not keep a competing writer open.
		err := logFile.Close()
		logFile = nil
		if err != nil {
			return result, err
		}
	}
	if err := o.Start(cmd); err != nil {
		return result, fmt.Errorf("gametest: start %q: %w", exe, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	sample := func() {
		result.PeakMemoryMB = max(result.PeakMemoryMB, peakMemoryMB(cmd.Process.Pid))
	}
	sample()
	finish := func(waitErr error) error {
		if console != nil {
			waitErr = errors.Join(waitErr, console.flush())
		}
		if cmd.ProcessState != nil {
			code := cmd.ProcessState.ExitCode()
			result.ExitCode = &code
		}
		return waitErr
	}
	stop := func() error {
		sample()
		killErr := cmd.Process.Kill()
		if errors.Is(killErr, os.ErrProcessDone) {
			killErr = nil
		}
		waitErr := <-done
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			waitErr = nil
		}
		return errors.Join(killErr, finish(waitErr))
	}
	result.Status = "timeout"
	deadline := time.NewTimer(o.Timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var menuAt, probeAt time.Time
	tail := logTail{probe: o.Probe}
	// probeEnded reads the log on and marks the run "probe" once the probe's last line is in it.
	probeEnded := func(now time.Time) (bool, error) {
		if !probeAt.IsZero() {
			return true, nil
		}
		if err := tail.read(logPath, now); err != nil {
			return false, err
		}
		if tail.probeEnd == "" {
			return false, nil
		}
		probeAt = now
		result.Status = "probe"
		result.LoadSeconds = now.Sub(launch).Seconds()
		return true, nil
	}
	for {
		select {
		case <-ctx.Done():
			return result, errors.Join(ctx.Err(), stop())
		case err := <-done:
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				err = nil
			}
			err = finish(err) // flushes the 1.7 console log before the last read
			result.Status = "exited"
			if o.Probe {
				// A probe quits right after its last line, often before the next sample.
				ended, readErr := probeEnded(o.Now())
				if ended {
					result.Status = "probe"
				}
				err = errors.Join(err, readErr)
			}
			return result, errors.Join(ctx.Err(), err)
		case <-deadline.C:
			return result, errors.Join(ctx.Err(), stop())
		case <-ticker.C:
			sample()
			now := o.Now()
			if o.Probe {
				ended, err := probeEnded(now)
				if err != nil {
					return result, errors.Join(err, stop())
				}
				if ended && now.Sub(probeAt) >= o.Settle {
					return result, errors.Join(ctx.Err(), stop())
				}
			} else if o.Map {
				if err := tail.read(logPath, now); err != nil {
					return result, errors.Join(err, stop())
				}
				if tail.marker && now.Sub(tail.lastWrite) >= o.MapQuiet {
					result.Status = "map"
					result.LoadSeconds = now.Sub(launch).Seconds()
					return result, errors.Join(ctx.Err(), stop())
				}
			} else {
				if menuAt.IsZero() {
					st, err := os.Stat(knowledge)
					if err != nil && !os.IsNotExist(err) {
						return result, errors.Join(err, stop())
					}
					if err == nil && !st.IsDir() && !st.ModTime().Before(launch) {
						menuAt = now
						result.LoadSeconds = now.Sub(launch).Seconds()
					}
				}
				if !menuAt.IsZero() && now.Sub(menuAt) >= o.Settle {
					result.Status = "menu"
					return result, errors.Join(ctx.Err(), stop())
				}
			}
		}
	}
}

const mapMarker = "Initializing new game with mods"

type logTail struct {
	offset    int64
	lastWrite time.Time
	marker    bool
	pending   string
	// probe: look for the probe's last line (20 §3) in whole lines; line holds the
	// unfinished one, probeEnd is "result" or "exception" once it is found.
	probe    bool
	line     []byte
	probeEnd string
}

// maxProbeLine bounds the unfinished line kept between reads; a longer line is dropped.
const maxProbeLine = 64 << 10

func (t *logTail) probeLines(chunk []byte) {
	if t.probeEnd != "" {
		return
	}
	t.line = append(t.line, chunk...)
	for {
		i := bytes.IndexByte(t.line, '\n')
		if i < 0 {
			break
		}
		if text, ok := ProbeLine(string(t.line[:i])); ok {
			if end := ProbeEnd(text); end != "" {
				t.probeEnd, t.line = end, nil
				return
			}
		}
		t.line = t.line[i+1:]
	}
	if len(t.line) > maxProbeLine {
		t.line = nil
	}
}

func (t *logTail) read(path string, now time.Time) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if st.Size() < t.offset {
		t.offset, t.pending, t.marker, t.line = 0, "", false, nil
	}
	if _, err := f.Seek(t.offset, io.SeekStart); err != nil {
		return err
	}
	buf := make([]byte, 32*1024)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			t.offset += int64(n)
			t.lastWrite = now
			text := t.pending + string(buf[:n])
			t.marker = t.marker || strings.Contains(text, mapMarker)
			t.pending = text[max(0, len(text)-len(mapMarker)+1):]
			if t.probe {
				t.probeLines(buf[:n])
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
