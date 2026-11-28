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
	LastLine string
	// StallSeconds: the longest time the log did not grow while the game loaded (launch to
	// the menu, new-game or load marker), or, in a timeout, until the end; 0 when under
	// StallMin. A paused window or a hang looks like this (20 §2).
	StallSeconds float64 `json:",omitempty"`
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
	if err := validateOptions(o); err != nil {
		return nil, err
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
	applyDefaults(&o)
	if err := prepareData(o); err != nil {
		return nil, err
	}
	g := &gameRun{o: o, knowledge: filepath.Join(o.DataDir, "Config", "Knowledge.xml"),
		saves: filepath.Join(o.DataDir, "Saves"), logPath: filepath.Join(o.RunDir, "Player.log")}
	g.logFile, err = os.Create(g.logPath)
	if err != nil {
		return nil, fmt.Errorf("gametest: create log %q: %w", g.logPath, err)
	}
	defer func() {
		if g.logFile != nil {
			runErr = errors.Join(runErr, g.logFile.Close())
		}
	}()
	g.stdout, err = os.Create(filepath.Join(o.RunDir, "stdout.log"))
	if err != nil {
		return nil, fmt.Errorf("gametest: create stdout capture: %w", err)
	}
	defer func() { runErr = errors.Join(runErr, g.stdout.Close()) }()
	if err := g.buildCommand(); err != nil {
		return nil, err
	}
	g.launch = o.Now()
	g.result = &Result{Status: "start-failed", LogPath: g.logPath, Args: g.args, Exe: g.exe, Waylith: g.waylith}
	g.tail = logTail{probe: o.Probe}
	defer func() {
		if err := g.saveResult(); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("gametest: save result: %w", err))
		}
	}()
	if err := g.attachLog(); err != nil {
		return g.result, err
	}
	if err := o.Start(g.cmd); err != nil {
		return g.result, fmt.Errorf("gametest: start %q: %w", g.exe, err)
	}
	g.done = make(chan error, 1)
	go func() { g.done <- g.cmd.Wait() }()
	g.sample()
	return g.result, g.wait(ctx)
}

// validateOptions rejects negative durations, contradictory modes and extra arguments that override managed ones.
func validateOptions(o Options) error {
	if o.Timeout < 0 || o.Settle < 0 || o.MapQuiet < 0 || o.Ticks < 0 {
		return fmt.Errorf("gametest: durations and ticks must not be negative")
	}
	if o.Save != "" && (o.Map || o.Probe) {
		return fmt.Errorf("gametest: a save run loads the save, not a test map")
	}
	if o.Ticks > 0 && o.Probe {
		return fmt.Errorf("gametest: a probe run ends at the probe's last line, not after ticks")
	}
	for _, arg := range o.ExtraArgs {
		key, _, _ := strings.Cut(strings.ToLower(strings.TrimLeft(arg, "-/")), "=")
		switch key {
		case "savedatafolder", "logfile", "nofilelog", "quicktest", "probequit":
			return fmt.Errorf("gametest: extra argument %q overrides a managed option", arg)
		}
	}
	return nil
}

// applyDefaults fills zero options with their defaults; a probe run implies a test map.
func applyDefaults(o *Options) {
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
}

// prepareData creates the data and run folders, resets preferences and saves, and removes the old menu marker.
func prepareData(o Options) error {
	for _, dir := range []string{filepath.Join(o.DataDir, "Config"), o.RunDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("gametest: create %q: %w", dir, err)
		}
	}
	if err := resetPreferences(o.DataDir); err != nil {
		return err
	}
	if err := prepareSaves(filepath.Join(o.DataDir, "Saves"), o.Save); err != nil {
		return err
	}
	// Every run writes its own Prefs.xml after the reset, so the game keeps running while its
	// window is unfocused. Dev mode loads Saves/autostart.rws at startup: only a save run has it.
	if err := writeRunPreferences(o.DataDir, o.Save != "", o.Ticks); err != nil {
		return err
	}
	knowledge := filepath.Join(o.DataDir, "Config", "Knowledge.xml")
	if err := os.Remove(knowledge); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("gametest: remove old marker %q: %w", knowledge, err)
	}
	return nil
}

// gameRun is the state of one started game: its command, captures, log tail and result.
type gameRun struct {
	o                         Options
	knowledge, saves, logPath string
	logFile, stdout           *os.File
	exe                       string
	args                      []string
	waylith                   bool
	cmd                       *exec.Cmd
	console                   *consoleLog
	done                      chan error
	launch                    time.Time
	menuAt, menuSeen, probeAt time.Time
	tail                      logTail
	played                    autosave
	stall                     time.Duration // the longest quiet log so far (noteStall)
	result                    *Result
}

// buildCommand builds the game command with the managed arguments and the stdout capture.
func (g *gameRun) buildCommand() error {
	o := g.o
	exe, waylith := executable(o)
	args := []string{"-savedatafolder=" + o.DataDir}
	if waylith {
		args = append(args, "-NoFileLog")
	} else {
		args = append(args, "-logfile", g.logPath, "-popupwindow")
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
	if waylith {
		if err := utf8Console(cmd, o.RunDir); err != nil {
			return err
		}
	}
	// Bound output draining if a child inherits the capture pipe.
	cmd.WaitDelay = time.Second
	cmd.Stdout, cmd.Stderr = g.stdout, g.stdout
	g.exe, g.args, g.waylith, g.cmd = exe, args, waylith, cmd
	return nil
}

// attachLog tees 1.7's console into Player.log, or closes the log file for Unity to write itself.
func (g *gameRun) attachLog() error {
	if g.waylith {
		var err error
		g.console, err = newConsoleLog(g.logFile, g.o.GameVersion, g.launch)
		if err != nil {
			return err
		}
		output := io.MultiWriter(g.stdout, g.console)
		g.cmd.Stdout, g.cmd.Stderr = output, output
		return nil
	}
	// Unity opens its own log handle; do not keep a competing writer open.
	err := g.logFile.Close()
	g.logFile = nil
	return err
}

// saveResult fills the timing, phase and last line and writes result.json.
func (g *gameRun) saveResult() error {
	result, tail := g.result, &g.tail
	end := g.o.Now()
	result.TotalSeconds = end.Sub(g.launch).Seconds()
	result.Phase = "loading"
	switch {
	case !tail.saveAt.IsZero():
		result.Phase = "save"
	case !tail.newAt.IsZero():
		result.Phase = "new"
	case !g.menuSeen.IsZero():
		result.Phase = "menu"
	}
	marker := tail.newAt
	if g.o.Save != "" {
		marker = tail.saveAt
	}
	if !marker.IsZero() {
		result.MapSeconds = end.Sub(marker).Seconds()
	}
	if result.Status == "timeout" || result.Status == "exited" {
		result.LastLine = tail.last
	}
	g.noteStall(end, result.Status == "timeout")
	if g.stall >= StallMin {
		result.StallSeconds = g.stall.Round(time.Second).Seconds()
	}
	b, err := json.MarshalIndent(result, "", "  ")
	if err == nil {
		err = os.WriteFile(filepath.Join(g.o.RunDir, "result.json"), append(b, '\n'), 0o644)
	}
	return err
}

// sample records the game's peak memory so far.
func (g *gameRun) sample() {
	g.result.PeakMemoryMB = max(g.result.PeakMemoryMB, peakMemoryMB(g.cmd.Process.Pid))
}

// finish flushes the console log and records the exit code of a reaped game.
func (g *gameRun) finish(waitErr error) error {
	if g.console != nil {
		waitErr = errors.Join(waitErr, g.console.flush())
	}
	if g.cmd.ProcessState != nil {
		code := g.cmd.ProcessState.ExitCode()
		g.result.ExitCode = &code
	}
	return waitErr
}

// stop kills and reaps the game.
func (g *gameRun) stop() error {
	g.sample()
	killErr := g.cmd.Process.Kill()
	if errors.Is(killErr, os.ErrProcessDone) {
		killErr = nil
	}
	waitErr := <-g.done
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		waitErr = nil
	}
	return errors.Join(killErr, g.finish(waitErr))
}

// seeMenu notes when this run wrote Knowledge.xml (in every mode, for Phase).
func (g *gameRun) seeMenu(now time.Time) error {
	if !g.menuSeen.IsZero() {
		return nil
	}
	st, err := os.Stat(g.knowledge)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && !st.IsDir() && !st.ModTime().Before(g.launch) {
		g.menuSeen = now
	}
	return nil
}

// StallMin: a quiet log this long counts as a stall. Loading logs every few seconds; an
// unfocused window without runInBackground stops it until someone clicks the window.
const StallMin = 60 * time.Second

// noteStall keeps the longest time the log has not grown while the game loads (until the
// menu, new-game or load marker); final counts the quiet up to now whatever the phase.
func (g *gameRun) noteStall(now time.Time, final bool) {
	t := &g.tail
	if !final && (!t.newAt.IsZero() || !t.saveAt.IsZero() || !g.menuSeen.IsZero()) {
		return
	}
	since := t.lastWrite
	if since.Before(g.launch) {
		since = g.launch
	}
	if q := now.Sub(since); q > g.stall {
		g.stall = q
	}
}

// probeEnded marks the run "probe" once the probe's last line is in the log.
func (g *gameRun) probeEnded(now time.Time) bool {
	if g.probeAt.IsZero() && g.tail.probeEnd != "" {
		g.probeAt = now
		g.result.Status = "probe"
		g.result.LoadSeconds = now.Sub(g.launch).Seconds()
	}
	return !g.probeAt.IsZero()
}

// arrival is the status a quiet map or save, or the first autosave, arrives at.
func (g *gameRun) arrival() string {
	if g.o.Save != "" {
		return "save"
	}
	return "map"
}

// wait polls the log until the run arrives, times out, is cancelled or the game exits.
func (g *gameRun) wait(ctx context.Context) error {
	g.result.Status = "timeout"
	deadline := time.NewTimer(g.o.Timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), g.stop())
		case err := <-g.done:
			err = g.exited(err)
			return errors.Join(ctx.Err(), err)
		case <-deadline.C:
			return errors.Join(ctx.Err(), g.stop())
		case <-ticker.C:
			g.sample()
			now := g.o.Now()
			if err := errors.Join(g.tail.read(g.logPath, now), g.seeMenu(now)); err != nil {
				return errors.Join(err, g.stop())
			}
			g.noteStall(now, false)
			arrived, err := g.arrived(now)
			if err != nil {
				return errors.Join(err, g.stop())
			}
			if arrived {
				return errors.Join(ctx.Err(), g.stop())
			}
		}
	}
}

// exited reaps a game that ended by itself, reads its last log lines and sets the status.
func (g *gameRun) exited(err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		err = nil
	}
	err = g.finish(err) // flushes the 1.7 console log before the last read
	now := g.o.Now()
	err = errors.Join(err, g.tail.read(g.logPath, now), g.seeMenu(now))
	g.result.Status = "exited"
	// A probe quits right after its last line, often before the next sample.
	if g.o.Probe && g.probeEnded(now) {
		g.result.Status = "probe"
	}
	return err
}

// arrived reports whether the run reached its goal at now, setting the status and load time.
func (g *gameRun) arrived(now time.Time) (bool, error) {
	o, result := g.o, g.result
	switch {
	case o.Probe:
		return g.probeEnded(now) && now.Sub(g.probeAt) >= o.Settle, nil
	case o.Ticks > 0:
		done, err := g.played.check(g.saves, g.launch, now)
		if err != nil || !done {
			return false, err
		}
		result.Status, result.TicksReached = g.arrival(), o.Ticks
		result.LoadSeconds = now.Sub(g.launch).Seconds()
		return true, nil
	case o.Map || o.Save != "":
		marker := g.tail.newAt
		if o.Save != "" {
			marker = g.tail.saveAt
		}
		if !marker.IsZero() && now.Sub(g.tail.lastWrite) >= o.MapQuiet {
			result.Status = g.arrival()
			result.LoadSeconds = now.Sub(g.launch).Seconds()
			return true, nil
		}
	default:
		if g.menuAt.IsZero() && !g.menuSeen.IsZero() {
			g.menuAt = now
			result.LoadSeconds = now.Sub(g.launch).Seconds()
		}
		if !g.menuAt.IsZero() && now.Sub(g.menuAt) >= o.Settle {
			result.Status = "menu"
			return true, nil
		}
	}
	return false, nil
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
