package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// A pack build in progress (10 §8.2). The copy takes minutes, so the window runs it as an
// `archo pack build` child process: it goes on when the window closes, and its state is a
// file in the folder it assembles the pack in, which any process can read.

// TmpSuffix names the folder a pack is assembled in, next to the packs.
const TmpSuffix = ".archo-tmp"

// BuildFile is the build's state inside that folder; StopFile asks the build to stop.
const (
	BuildFile = "build.json"
	StopFile  = "stop"
)

// The steps of a build, in order.
const (
	StepCheck   = "check"   // the list: nothing left to fix (需要处理)
	StepMeasure = "measure" // sizes, free space
	StepCopy    = "copy"    // the mods, without other game versions' folders
	StepWrite   = "write"   // pack.json
	StepDone    = "done"
)

// beatEvery is how often a running build rewrites its state; one not rewritten for
// beatStale has stopped (its process ended).
const (
	beatEvery = 2 * time.Second
	beatStale = 15 * time.Second
)

// BuildProgress is a build's state (build.json).
type BuildProgress struct {
	Name       string    `json:"name"`
	Env        string    `json:"env,omitempty"`
	Step       string    `json:"step"`
	Mods       int       `json:"mods"`       // copied
	ModsTotal  int       `json:"mods_total"` // to copy
	Bytes      int64     `json:"bytes"`
	BytesTotal int64     `json:"bytes_total"`
	Mod        string    `json:"mod,omitempty"` // being copied
	PID        int       `json:"pid,omitempty"`
	Started    time.Time `json:"started"`
	Beat       time.Time `json:"beat"` // last written
}

// Running reports whether the build's process is still writing its state.
func (p *BuildProgress) Running() bool {
	return p != nil && p.Step != StepDone && time.Since(p.Beat) < beatStale
}

// ReadBuildProgress reads the state in a build folder (nil: none, or unreadable).
func ReadBuildProgress(tmp string) *BuildProgress {
	b, err := os.ReadFile(filepath.Join(tmp, BuildFile))
	if err != nil {
		return nil
	}
	var p BuildProgress
	if json.Unmarshal(b, &p) != nil {
		return nil
	}
	return &p
}

// buildState keeps a build's progress, writes build.json (when it has a folder) and
// passes the progress on (throttled).
type buildState struct {
	mu         sync.Mutex
	dir        string // "" for a dry run: nothing is written
	p          BuildProgress
	report     func(BuildProgress)
	wrote      time.Time
	reported   time.Time
	lastStep   string
	stopAsked  time.Time
	stopErr    error
	quit, done chan struct{}
}

func (b *buildState) start(dir string, p BuildProgress) {
	b.dir, b.p = dir, p
	b.p.PID = os.Getpid()
	b.p.Started = time.Now().UTC()
	b.quit, b.done = make(chan struct{}), make(chan struct{})
	b.mu.Lock()
	b.writeLocked()
	b.mu.Unlock()
	go func() {
		defer close(b.done)
		t := time.NewTicker(beatEvery)
		defer t.Stop()
		for {
			select {
			case <-b.quit:
				return
			case <-t.C:
				b.mu.Lock()
				b.writeLocked()
				b.mu.Unlock()
			}
		}
	}()
}

// stop ends the heartbeat (the state file stays until the folder goes).
func (b *buildState) stop() {
	if b.quit == nil {
		return
	}
	select {
	case <-b.quit:
	default:
		close(b.quit)
		<-b.done
	}
}

func (b *buildState) update(f func(*BuildProgress)) {
	b.mu.Lock()
	f(&b.p)
	now := time.Now()
	stepChanged := b.p.Step != b.lastStep
	if b.dir != "" && (stepChanged || now.Sub(b.wrote) >= 250*time.Millisecond) && b.p.Step != StepDone {
		b.writeLocked()
	}
	var send *BuildProgress
	if b.report != nil && (stepChanged || now.Sub(b.reported) >= 500*time.Millisecond) {
		p := b.p
		send = &p
		b.reported = now
	}
	b.lastStep = b.p.Step
	b.mu.Unlock()
	if send != nil {
		b.report(*send)
	}
}

func (b *buildState) writeLocked() {
	if b.dir == "" {
		return
	}
	b.p.Beat = time.Now().UTC()
	b.wrote = time.Now()
	data, _ := json.Marshal(&b.p)
	part := filepath.Join(b.dir, BuildFile+".part")
	if os.WriteFile(part, data, 0o644) == nil {
		os.Rename(part, filepath.Join(b.dir, BuildFile)) // a reader never sees half a file
	}
}

var errStopped = errors.New("the build was stopped")

// stopRequested is errStopped once the stop file appeared (looked for twice a second).
func (b *buildState) stopRequested() error {
	if b.dir == "" || b.stopErr != nil {
		return b.stopErr
	}
	if time.Since(b.stopAsked) < 500*time.Millisecond {
		return nil
	}
	b.stopAsked = time.Now()
	if _, err := os.Stat(filepath.Join(b.dir, StopFile)); err == nil {
		b.stopErr = errStopped
	}
	return b.stopErr
}

// ---- unfinished builds ----

// UnfinishedPack is a build folder in the packs folder: a build running (in any process)
// or one cut short.
type UnfinishedPack struct {
	Name     string
	Dir      string
	Progress *BuildProgress // nil: no state (an older build, or an import cut short)
	Running  bool
}

// UnfinishedPacks lists the build folders in the packs folder.
func (a *App) UnfinishedPacks() ([]*UnfinishedPack, error) {
	ents, err := os.ReadDir(a.PacksDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*UnfinishedPack
	for _, de := range ents {
		if !de.IsDir() || !strings.HasSuffix(de.Name(), TmpSuffix) {
			continue
		}
		u := &UnfinishedPack{Name: strings.TrimSuffix(de.Name(), TmpSuffix), Dir: filepath.Join(a.PacksDir(), de.Name())}
		u.Progress = ReadBuildProgress(u.Dir)
		u.Running = u.Progress.Running()
		out = append(out, u)
	}
	return out, nil
}

// RemoveUnfinished deletes what an unfinished build copied (its folder in the packs
// folder; nothing else). A running build is stopped with StopBuild instead.
func (a *App) RemoveUnfinished(name string) error {
	tmp, err := a.UnfinishedDir(name)
	if err != nil {
		return err
	}
	return os.RemoveAll(tmp)
}

// UnfinishedDir is the folder of an unfinished build that may be removed (not running,
// inside the packs folder). The window removes it without holding the app: deleting
// gigabytes takes a while.
func (a *App) UnfinishedDir(name string) (string, error) {
	if err := checkPackName(name); err != nil {
		return "", err
	}
	tmp := a.packPath(name) + TmpSuffix
	if !within(a.PacksDir(), tmp) {
		return "", fmt.Errorf("%s is not in the packs folder", tmp)
	}
	fi, err := os.Stat(tmp)
	if err != nil || !fi.IsDir() {
		return "", fmt.Errorf("no unfinished build of %q", name)
	}
	if ReadBuildProgress(tmp).Running() {
		return "", fmt.Errorf("%w (%s): stop it first", ErrBuildRunning, name)
	}
	return tmp, nil
}

// StopBuild asks a running build to stop; it removes what it copied.
func (a *App) StopBuild(name string) error {
	if err := checkPackName(name); err != nil {
		return err
	}
	tmp := a.packPath(name) + TmpSuffix
	if !ReadBuildProgress(tmp).Running() {
		return fmt.Errorf("no build of %q is running", name)
	}
	return os.WriteFile(filepath.Join(tmp, StopFile), nil, 0o644)
}

// DiskFree is the space free (for this user) on the drive of dir, or of its nearest
// existing parent; -1 when it cannot be told.
func DiskFree(dir string) int64 {
	for d := filepath.Clean(dir); ; {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			return diskFree(d)
		}
		p := filepath.Dir(d)
		if p == d {
			return -1
		}
		d = p
	}
}

// ---- the child process ----

// ChildPackBuild is ChildPackBuildExe with this executable.
func ChildPackBuild(home, envName, name, logPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if b := strings.ToLower(filepath.Base(exe)); strings.HasSuffix(b, ".test") || strings.HasSuffix(b, ".test.exe") {
		return errors.New("child pack build from a test binary: set the runner")
	}
	return ChildPackBuildExe(exe, home, envName, name, logPath)
}

// ChildPackBuildExe runs `<exe> pack build <name>` with the same data directory. Its
// output goes to logPath (not a pipe: the build goes on when this process ends); the
// progress is in the build folder (ReadBuildProgress). It returns when the child ends.
func ChildPackBuildExe(exe, home, envName, name, logPath string) error {
	args := []string{"pack", "build", name}
	if envName != "" {
		args = append(args, "--env", envName)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	logf, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer logf.Close()
	cmd, err := startDetached(func() *exec.Cmd {
		c := exec.Command(exe, args...)
		c.Env = append(os.Environ(), "RIMARCHO_HOME="+home)
		c.Stdout, c.Stderr = logf, logf
		return c
	})
	if err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%s", lastLines(logPath, err))
	}
	return nil
}

// lastLines is what a failed child said: its output without the progress lines.
func lastLines(path string, err error) string {
	f, ferr := os.Open(path)
	if ferr != nil {
		return err.Error()
	}
	defer f.Close()
	var keep []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l != "" && !strings.HasPrefix(l, "progress:") {
			keep = append(keep, l)
		}
	}
	if len(keep) == 0 {
		return err.Error()
	}
	if len(keep) > 12 {
		keep = keep[:12]
	}
	return strings.Join(keep, "\n")
}
