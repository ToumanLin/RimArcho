package app

// RimArcho updating itself (17 §5.1): the "self" automatic check reads GitHub Releases;
// installing waits for the player's confirmation, downloads and verifies the release zip,
// waits until no other RimArcho runs from the program folder, swaps the files and checks
// the new program before the surface restarts it. The data folder is never touched.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/selfupdate"
)

// UpdateDirName is the folder inside the program folder that holds the download, the
// unpacked new files and the old files moved aside; on the program's volume so the swap
// is renames only (17 §5.1, 02 §1).
const UpdateDirName = ".archo-update"

// UpdatedFromEnv carries the pid of the process that installed an update to the program it
// restarted: the new one waits for it to exit before removing the old files.
const UpdatedFromEnv = "RIMARCHO_UPDATED_FROM"

// SelfUpdate is meta "self-update": the newest release the last check found.
type SelfUpdate struct {
	Checked time.Time           `json:"checked"`
	Release *selfupdate.Release `json:"release,omitempty"`
	State   string              `json:"state"`
}

// SelfVersionNow is the running version ("dev" for a plain go build).
func (a *App) SelfVersionNow() string {
	if a.SelfVersion != "" {
		return a.SelfVersion
	}
	return buildinfo.Version
}

// SelfUpdateInfo reads the last check.
func (a *App) SelfUpdateInfo() SelfUpdate {
	var s SelfUpdate
	_ = json.Unmarshal([]byte(a.Store.Meta("self-update")), &s)
	return s
}

// SelfUpdatePending is the release newer than the running version, or nil. It is worked
// out against the running version each time, so a record made before an update stops
// counting once the new version runs.
func (a *App) SelfUpdatePending() *selfupdate.Release {
	r := a.SelfUpdateInfo().Release
	if r == nil || !selfupdate.Newer(a.SelfVersionNow(), r.Version) {
		return nil
	}
	return r
}

// selfPrerelease: a prerelease build also looks at newer prereleases; a stable build only
// at stable releases (the channel follows the running version).
func selfPrerelease(v string) bool {
	_, pre, _ := strings.Cut(strings.SplitN(v, "+", 2)[0], "-")
	return pre != ""
}

// UpdateAPIEnv replaces GitHub's API root (a test release server in the VM, 17 §5.1).
const UpdateAPIEnv = "RIMARCHO_UPDATE_API"

func (a *App) selfClient() *selfupdate.Client {
	api := a.SelfUpdateAPI
	if api == "" {
		api = os.Getenv(UpdateAPIEnv)
	}
	c := &selfupdate.Client{API: api, UserAgent: UserAgent}
	if a.HTTP != nil {
		c.HTTP = a.HTTP
	}
	return c
}

// CheckSelfUpdate asks GitHub for the newest release and records it; it never downloads.
// It is the automatic "self" check and `archo update --check`.
func (a *App) CheckSelfUpdate(ctx context.Context) (int, string, error) {
	cur := a.SelfVersionNow()
	if _, ok := selfupdate.Compare(cur, cur); !ok {
		return 0, "development build", nil
	}
	s := a.SelfUpdateInfo()
	r, err := a.selfClient().Latest(ctx, selfPrerelease(cur))
	s.Checked = a.Now().UTC()
	switch {
	case errors.Is(err, selfupdate.ErrNoRelease):
		s.Release, s.State, err = nil, "no release published yet", nil
	case err != nil:
		s.State = "check unavailable offline" // the last known release stays
	default:
		s.Release, s.State = r, "up to date"
		if selfupdate.Newer(cur, r.Version) {
			s.State = "new version available"
		}
	}
	b, _ := json.Marshal(s)
	if e := a.Store.SetMeta("self-update", string(b)); e != nil {
		err = errors.Join(err, e)
	}
	pending := 0
	if a.SelfUpdatePending() != nil {
		pending = 1
	}
	return pending, s.State, err
}

// SelfProgramDir is the folder this program runs from.
func (a *App) SelfProgramDir() (string, error) {
	if a.SelfProgram != "" {
		return a.SelfProgram, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return filepath.Dir(exe), nil
}

// SelfUpdateStep is one report of a running install.
type SelfUpdateStep struct {
	Phase       string // "download", "unpack", "wait", "replace", "check"
	Done, Total int64
	Others      []*GameRun // phase "wait": the other RimArcho processes in the program folder
}

// SelfUpdateResult says what was installed.
type SelfUpdateResult struct {
	From, To string
	Program  string
}

// OtherArchoIn lists the RimArcho processes other than this one that run from dir.
func (a *App) OtherArchoIn(dir string) []*GameRun {
	if a.SelfOthers != nil {
		return a.SelfOthers(dir)
	}
	var out []*GameRun
	for _, r := range archoProcesses() {
		if r.PID != os.Getpid() && env.PathKey(filepath.Dir(r.Exe)) == env.PathKey(dir) {
			out = append(out, r)
		}
	}
	return out
}

// ErrUpdateBusy: another RimArcho is installing the update from the same program folder.
var ErrUpdateBusy = errors.New("another RimArcho is already installing the update")

// InstallSelfUpdate installs the pending release after the player confirmed it (02 §1).
// The download and unpacking happen first; the files are swapped only once no other
// RimArcho runs from the program folder (it waits, reporting them, until ctx ends). The
// new archo must then report the new version, or the old files are put back. The caller
// restarts the program; the data folder is not touched.
func (a *App) InstallSelfUpdate(ctx context.Context, step func(SelfUpdateStep)) (*SelfUpdateResult, error) {
	programDir, programErr := a.SelfProgramDir()
	if programErr != nil {
		return nil, programErr
	}
	if _, ok := selfupdate.InWorkshopFolder(programDir); ok {
		return nil, errors.New("RimArcho cannot update in a Steam Workshop folder; open RimArcho.exe to move it first")
	}

	if step == nil {
		step = func(SelfUpdateStep) {}
	}
	cur := a.SelfVersionNow()
	if _, ok := selfupdate.Compare(cur, cur); !ok {
		return nil, errors.New("a development build is not updated; build or download a release")
	}
	r := a.SelfUpdatePending()
	if r == nil {
		return nil, fmt.Errorf("RimArcho %s is the newest version known; check for updates first", cur)
	}
	if err := r.Installable(); err != nil {
		return nil, fmt.Errorf("%w; download it from %s", err, r.Page)
	}
	program, err := a.SelfProgramDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(program, UpdateDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot write to the program folder %s; download the new version from %s", program, r.Page)
	}
	release, ok := lockUpdate(filepath.Join(dir, "lock"))
	if !ok {
		return nil, ErrUpdateBusy
	}
	defer release()

	step(SelfUpdateStep{Phase: "download", Total: r.Zip.Size})
	zip, err := a.selfClient().Download(ctx, r, filepath.Join(dir, "download"), func(done, total int64) {
		step(SelfUpdateStep{Phase: "download", Done: done, Total: total})
	})
	if err != nil {
		if errors.Is(err, selfupdate.ErrChecksum) {
			return nil, fmt.Errorf("the downloaded RimArcho %s does not match its checksum; nothing was changed", r.Version)
		}
		return nil, fmt.Errorf("download failed; nothing was changed: %w", err)
	}
	step(SelfUpdateStep{Phase: "unpack"})
	stage := filepath.Join(dir, "new-"+r.Version)
	if err := os.RemoveAll(stage); err != nil {
		return nil, err
	}
	if err := selfupdate.Extract(zip, stage); err != nil {
		return nil, fmt.Errorf("the release zip cannot be used; nothing was changed: %w", err)
	}
	defer os.RemoveAll(stage)

	poll := a.SelfWaitPoll
	if poll <= 0 {
		poll = time.Second
	}
	for {
		others := a.OtherArchoIn(program)
		if len(others) == 0 {
			break
		}
		step(SelfUpdateStep{Phase: "wait", Others: others})
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("stopped while waiting for the other RimArcho to close; nothing was changed (the download is kept): %w", ctx.Err())
		case <-time.After(poll):
		}
	}

	// From here the files change: closing the window waits for this part (WaitSelfSwap),
	// and the version check does not stop with ctx, so a swap ends whole or rolled back.
	swapping.Add(1)
	defer swapping.Done()
	step(SelfUpdateStep{Phase: "replace"})
	old := filepath.Join(dir, fmt.Sprintf("old-%s-%d", cur, time.Now().UnixNano()))
	swap, err := selfupdate.Apply(program, stage, old)
	if err != nil {
		return nil, fmt.Errorf("could not replace the program files; RimArcho %s is unchanged: %w", cur, err)
	}
	step(SelfUpdateStep{Phase: "check"})
	versionOf := a.SelfVersionOf
	if versionOf == nil {
		versionOf = selfupdate.VersionOf
	}
	got, err := versionOf(context.Background(), filepath.Join(program, selfupdate.Files[0]))
	if err == nil && got != r.Version {
		err = fmt.Errorf("it reports version %q", got)
	}
	if err != nil {
		if rb := swap.Rollback(); rb != nil {
			return nil, fmt.Errorf("the new program did not start (%s), and putting RimArcho %s back failed: %w; the old files are in %s", err.Error(), cur, rb, old)
		}
		return nil, fmt.Errorf("the new program did not start, so RimArcho %s was put back: %w", cur, err)
	}
	s := a.AutomaticStatus("self")
	if s.Kind != "" && !s.Checked.IsZero() {
		s.Pending, s.State = 0, "installed "+r.Version
		b, _ := json.Marshal(s)
		_ = a.Store.SetMeta("automatic:self", string(b))
	}
	return &SelfUpdateResult{From: cur, To: r.Version, Program: program}, nil
}

var swapping sync.WaitGroup

// WaitSelfSwap waits up to limit for a running swap of the program files to finish (or roll
// back), so closing RimArcho never leaves them half replaced.
func WaitSelfSwap(limit time.Duration) {
	done := make(chan struct{})
	go func() { swapping.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(limit):
	}
}

// CleanSelfUpdate removes what an earlier install left in the program folder (the old
// files, unpacked folders and downloads) when no install is running. A file still in use
// stays for the next start. With UpdatedFromEnv set it first waits up to ten seconds for
// that process, which held the old files, to exit.
func (a *App) CleanSelfUpdate() {
	program, err := a.SelfProgramDir()
	if err != nil {
		return
	}
	dir := filepath.Join(program, UpdateDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	if pid := os.Getenv(UpdatedFromEnv); pid != "" {
		waitExit(pid, 10*time.Second)
		os.Unsetenv(UpdatedFromEnv) // not for this program's children
	}
	release, ok := lockUpdate(filepath.Join(dir, "lock"))
	if !ok {
		return
	}
	defer release()
	for _, e := range entries {
		n := e.Name()
		if n == "download" || strings.HasPrefix(n, "old-") || strings.HasPrefix(n, "new-") {
			os.RemoveAll(filepath.Join(dir, n))
		}
	}
}

// SelfRestartProgram is the program to start after an update: the new copy of the one
// running (RimArcho.exe for the window, archo.exe for the TUI).
func SelfRestartProgram(program string) string {
	exe, err := os.Executable()
	name := filepath.Base(exe)
	if err != nil || !slices.ContainsFunc(selfupdate.Files, func(f string) bool { return strings.EqualFold(f, name) }) {
		name = selfupdate.Files[0]
	}
	return filepath.Join(program, name)
}
