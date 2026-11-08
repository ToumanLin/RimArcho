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
	// Save: a save file to load instead of the main menu or a test map. It is copied into
	// DataDir/Saves as autostart.rws and Prefs.xml turns dev mode on, so the game loads it
	// at startup (20 §2).
	Save string
	// Ticks: play this many game ticks on the test map or the loaded save, then stop:
	// Prefs.xml sets the autosave interval to Ticks, and the first autosave marks them
	// played (20 §2). 0 stops once the map has been quiet for MapQuiet.
	Ticks int
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
	Status       string             // menu, map, save, probe, timeout, exited, start-failed or links-blocked
	ExitCode     *int
	LoadSeconds  float64 // launch to arrival: the menu marker, a quiet map or save, the first autosave, or the probe's last line
	TotalSeconds float64
	// Phase is the furthest boundary the run reached: loading, menu (Knowledge.xml written),
	// new ("Initializing new game") or save ("Loading game from file").
	Phase string
	// MapSeconds: from the new-game marker (the load marker in a save run) to the stop; 0
	// when the marker never appeared.
	MapSeconds float64
	// TicksReached: Ticks, when the first autosave showed they were played.
	TicksReached int
	// LastLine: the log's last line, for a run that did not arrive (timeout, exited).
	LastLine     string
	PeakMemoryMB int64
	LogPath      string
	Args         []string
	Exe          string
	Waylith      bool
}

// Run waits for the menu, a quiet test map or save, the first autosave or a probe's last
// line, then kills and reaps the process. Timeout and early exit return a result with no
// error; cancellation returns ctx.Err(). Invalid options and setup failures return no
// result. Start failures return both.
func Run(ctx context.Context, o Options) (result *Result, runErr error) {
	if o.Timeout < 0 || o.Settle < 0 || o.MapQuiet < 0 || o.Ticks < 0 {
		return nil, fmt.Errorf("gametest: durations and ticks must not be negative")
	}
	if o.Save != "" && (o.Map || o.Probe) {
		return nil, fmt.Errorf("gametest: a save run loads the save, not a test map")
	}
	if o.Ticks > 0 && o.Probe {
		return nil, fmt.Errorf("gametest: a probe run ends at the probe's last line, not after ticks")
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
	saves := filepath.Join(o.DataDir, "Saves")
	if err := prepareSaves(saves, o.Save); err != nil {
		return nil, err
	}
	if o.Save != "" || o.Ticks > 0 {
		// Dev mode loads Saves/autostart.rws at startup; a test map needs no dev mode.
		if err := writeRunPreferences(o.DataDir, o.Save != "", o.Ticks); err != nil {
			return nil, err
		}
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
	var menuAt, menuSeen, probeAt time.Time
	tail := logTail{probe: o.Probe}
	defer func() {
		end := o.Now()
		result.TotalSeconds = end.Sub(launch).Seconds()
		result.Phase = "loading"
		switch {
		case !tail.saveAt.IsZero():
			result.Phase = "save"
		case !tail.newAt.IsZero():
			result.Phase = "new"
		case !menuSeen.IsZero():
			result.Phase = "menu"
		}
		marker := tail.newAt
		if o.Save != "" {
			marker = tail.saveAt
		}
		if !marker.IsZero() {
			result.MapSeconds = end.Sub(marker).Seconds()
		}
		if result.Status == "timeout" || result.Status == "exited" {
			result.LastLine = tail.last
		}
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
	// seeMenu notes when this run wrote Knowledge.xml (in every mode, for Phase).
	seeMenu := func(now time.Time) error {
		if !menuSeen.IsZero() {
			return nil
		}
		st, err := os.Stat(knowledge)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && !st.IsDir() && !st.ModTime().Before(launch) {
			menuSeen = now
		}
		return nil
	}
	// probeEnded marks the run "probe" once the probe's last line is in the log.
	probeEnded := func(now time.Time) bool {
		if probeAt.IsZero() && tail.probeEnd != "" {
			probeAt = now
			result.Status = "probe"
			result.LoadSeconds = now.Sub(launch).Seconds()
		}
		return !probeAt.IsZero()
	}
	// The status a quiet map or save, or the first autosave, arrives at.
	arrival := "map"
	if o.Save != "" {
		arrival = "save"
	}
	var played autosave
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
			now := o.Now()
			err = errors.Join(err, tail.read(logPath, now), seeMenu(now))
			result.Status = "exited"
			// A probe quits right after its last line, often before the next sample.
			if o.Probe && probeEnded(now) {
				result.Status = "probe"
			}
			return result, errors.Join(ctx.Err(), err)
		case <-deadline.C:
			return result, errors.Join(ctx.Err(), stop())
		case <-ticker.C:
			sample()
			now := o.Now()
			if err := errors.Join(tail.read(logPath, now), seeMenu(now)); err != nil {
				return result, errors.Join(err, stop())
			}
			switch {
			case o.Probe:
				if probeEnded(now) && now.Sub(probeAt) >= o.Settle {
					return result, errors.Join(ctx.Err(), stop())
				}
			case o.Ticks > 0:
				done, err := played.check(saves, launch, now)
				if err != nil {
					return result, errors.Join(err, stop())
				}
				if done {
					result.Status, result.TicksReached = arrival, o.Ticks
					result.LoadSeconds = now.Sub(launch).Seconds()
					return result, errors.Join(ctx.Err(), stop())
				}
			case o.Map || o.Save != "":
				marker := tail.newAt
				if o.Save != "" {
					marker = tail.saveAt
				}
				if !marker.IsZero() && now.Sub(tail.lastWrite) >= o.MapQuiet {
					result.Status = arrival
					result.LoadSeconds = now.Sub(launch).Seconds()
					return result, errors.Join(ctx.Err(), stop())
				}
			default:
				if menuAt.IsZero() && !menuSeen.IsZero() {
					menuAt = now
					result.LoadSeconds = now.Sub(launch).Seconds()
				}
				if !menuAt.IsZero() && now.Sub(menuAt) >= o.Settle {
					result.Status = "menu"
					return result, errors.Join(ctx.Err(), stop())
				}
			}
		}
	}
}

const (
	mapMarker  = "Initializing new game with mods"
	saveMarker = "Loading game from file "
)

// logTail reads the run's log as it grows, line by line: the game's new-game and load
// markers, the last line, and in a probe run the probe's last line (20 §3).
type logTail struct {
	offset    int64
	lastWrite time.Time
	// newAt, saveAt: when the new-game and save-load markers were first read.
	newAt, saveAt time.Time
	last          string // the last non-empty line, cut to 300 characters
	line          []byte // the unfinished line
	probe         bool
	probeEnd      string // "result" or "exception" once the probe's last line is read
}

// maxLine bounds the unfinished line kept between reads; a longer line is dropped.
const maxLine = 64 << 10

func (t *logTail) marks(l string, now time.Time) {
	if t.newAt.IsZero() && strings.Contains(l, mapMarker) {
		t.newAt = now
	}
	if t.saveAt.IsZero() && strings.Contains(l, saveMarker) && strings.Contains(l, " with mods") {
		t.saveAt = now
	}
}

func (t *logTail) lines(chunk []byte, now time.Time) {
	t.line = append(t.line, chunk...)
	for {
		i := bytes.IndexByte(t.line, '\n')
		if i < 0 {
			break
		}
		l := strings.TrimRight(string(t.line[:i]), "\r")
		t.line = t.line[i+1:]
		t.marks(l, now)
		if s := strings.TrimSpace(l); s != "" && !strings.HasPrefix(s, "(Filename: ") {
			if r := []rune(s); len(r) > 300 {
				s = string(r[:297]) + "..."
			}
			t.last = s
		}
		if t.probe && t.probeEnd == "" {
			if text, ok := ProbeLine(l); ok {
				t.probeEnd = ProbeEnd(text)
			}
		}
	}
	if len(t.line) > maxLine {
		t.line = nil
	}
	t.marks(string(t.line), now) // a marker the game has not ended with a newline yet
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
		*t = logTail{probe: t.probe}
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
			t.lines(buf[:n], now)
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// autosave watches the run's Saves folder for the game's first autosave (Autosave-N.rws),
// written once the autosave interval's ticks were played.
type autosave struct {
	size  int64
	since time.Time
}

// autosaveSettle: the autosave's size has not changed for this long, so the game wrote it.
const autosaveSettle = time.Second

func (a *autosave) check(dir string, launch, now time.Time) (bool, error) {
	files, err := filepath.Glob(filepath.Join(dir, "Autosave*.rws"))
	if err != nil {
		return false, err
	}
	var size int64
	for _, f := range files {
		if st, err := os.Stat(f); err == nil && !st.IsDir() && !st.ModTime().Before(launch) && st.Size() > size {
			size = st.Size()
		}
	}
	if size == 0 {
		return false, nil
	}
	if size != a.size {
		a.size, a.since = size, now
		return false, nil
	}
	return now.Sub(a.since) >= autosaveSettle, nil
}
