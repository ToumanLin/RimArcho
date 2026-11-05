// Package app holds the operations behind every command: environments, scanning and
// reconciliation, reference resolution and identity maintenance. Surfaces reach it only
// through internal/cmds.
package app

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/ToumanLin/RimArcho/internal/netcfg"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/ids"
	"github.com/ToumanLin/RimArcho/internal/machine"
	"github.com/ToumanLin/RimArcho/internal/perftrace"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/textures"
)

// Config is config.toml (07 §3). Only keys used by M1 are read.
type Config struct {
	TombstoneDays int // 02 §5.5, default 90
}

// DefaultConfig is written on first start.
const DefaultConfig = `# RimArcho configuration
# Days a vanished mod is kept as a tombstone before "archo gc" may remove it (02 §5.5).
tombstone_days = 90
`

// App is one opened RimArcho data directory.
type App struct {
	// MemoryProbe replaces cached hardware observations in synthetic tests.
	MemoryProbe       func() machine.Memory
	checkContext      context.Context // automatic worker cancellation; never shared with pages
	dataKey           string          // community data generation read by this connection's compact indexes
	workspaceSnapshot bool
	// LaunchWait reports a launch waiting for the cross-process handoff lock.
	LaunchWait      func(string)
	launchDepth     int
	launchUnlock    func()
	launchWaitLimit time.Duration // tests may shorten the production ten-second limit

	Home                string
	Store               *store.Store
	StartupLinkRecovery []*GameLink // read-only startup snapshot; Problems probes afresh
	Config              Config
	Now                 func() time.Time
	Rand                io.Reader
	Loc                 *time.Location // time zone for printed times
	// EnvName overrides the current environment for one invocation (--env).
	EnvName string
	// OpenPlan is the list this session shows and edits in place of the environment's
	// current one (04 §1.3): the GUI opens a list from the sidebar without choosing it to
	// play. 0, or a plan of another environment, is the current one. PlanName (--open-plan)
	// names it for one invocation; OpenPlan wins.
	OpenPlan int64
	PlanName string
	// Detect configures environment detection (tests point it at fixtures).
	Detect env.DetectOptions
	// Launcher starts the game; nil means ExecLauncher.
	Launcher Launcher
	// AnalyzeRunner brings the Def/Patch analysis up to date; nil means the `archo analyze`
	// child process (03 §3). Tests run it in-process.
	AnalyzeRunner func(envName string, progress func(string)) error
	// AnalyzeExe is the archo executable the default runner starts; "" is this one.
	AnalyzeExe string
	// SavesIndexRunner reads saves into the full-read cache (10 §1.7), the given saves
	// first; nil means the `archo saves index` child process. Tests run it in-process.
	SavesIndexRunner func(first []string, progress func(string)) error
	// FreeSpace tells the free space of a folder's drive for a pack build (tests); nil
	// means DiskFree.
	FreeSpace func(dir string) int64
	// Decompile replaces ilspycmd (tests); nil means ILSpyDecompiler.
	Decompile Decompiler
	// GameVersion ("X.Y") overrides the environment's game version for analysis commands.
	GameVersion string
	// AnalyzeWait bounds how long a command waits for an out-of-date analysis (05 §5.1:
	// MCP returns the finished part after 10 s and the analysis continues in the
	// background). 0 waits until it is done (CLI).
	AnalyzeWait time.Duration
	// LazyLog keeps the newest Player.log out of Problems until the player opens the
	// Problems view (the TUI; 03 §6.4: parse lazily to keep idle memory down).
	// An explicitly selected log is always included.
	LazyLog bool
	// LogCacheOnly makes Problems use a stored log report only, never parse the log
	// (status and mod: commands that are not about the log).
	LogCacheOnly bool
	// HTTP and SteamAPI reach the Steam Workshop for descriptions (06 §4.3); nil and ""
	// mean a 15-second client and the public endpoint. Tests replace both.
	// GameProbe finds the running game of a game folder; nil means the process table
	// (tests replace it).
	GameProbe func(gameRoot string) *GameRun
	// TestGameStart replaces cmd.Start for isolated test runs (08 §12).
	TestGameStart func(*exec.Cmd) error
	// TrashFunc moves a folder to the recycle bin; nil means source.Trash (tests replace it).
	TrashFunc func(path string) error
	// SameVolume says whether a folder can be renamed from one path to another (MoveDirectory);
	// nil compares the volume names (tests replace it to copy on one drive).
	SameVolume func(from, to string) bool
	// trashAllow replaces the recycle bin's allow-list check (08 §7) in tests of a bare App
	// with no store; nil means trashAllowed.
	trashAllow func(path string) error
	// SteamWorker runs `archo steam-worker <args>` in the Steam runtime folder and returns
	// its output; nil starts this executable (tests fake Steam with it, 06 §2.9).
	SteamWorker func(dir string, args []string) (string, error)
	// SteamWorkerStream replaces the live, cancellable download worker in tests.
	SteamWorkerStream func(ctx context.Context, dir string, args []string, line func(string)) error
	// LockBase replaces %LOCALAPPDATA% when finding the default lock library and monitor
	// folder (tests).
	LockBase string
	// MonitorBundle is the bundled Archo Monitor folder (08 §9): "" searches the usual
	// places, MonitorNone turns it off (tests).
	MonitorBundle string
	// VolumeFS names the file system of a folder's volume; nil means source.VolumeFS
	// (tests act out exFAT with it).
	VolumeFS func(dir string) string
	// SteamCMDStart and SteamCMDRun replace starting SteamCMD (tests fake it, 08 §11);
	// SteamCMDZip replaces Valve's download URL, SteamCMDPoll how often its log is read,
	// SteamCMDNoPath leaves PATH out when looking for it.
	SteamCMDStart func(exe string, args []string, console bool) (SteamCMDProc, error)
	SteamCMDRun   func(exe string, args []string) (string, error)
	SteamCMDZip   string
	SteamCMDPoll  time.Duration
	// SteamCMDWait replaces the Workshop retry backoff in tests; nil uses time.Sleep.
	SteamCMDWait   func(time.Duration)
	SteamCMDNoPath bool
	// SteamSignedIn replaces asking which account the running Steam client is signed in
	// with (env.SteamSignedIn); tests fake it.
	SteamSignedIn func() (account string, ok bool)
	HTTP          *http.Client
	// FindTool delegates dependency lookup; subscriptions never install tools.
	FindTool func(name string) (path string, ok bool)
	pageRepo func(string) string // resolved-repository override in local tests
	SteamAPI string
	// Texture optimisation (textures.go): TextureEncoder replaces todds, ToddsZip its download URL,
	// ToddsNoPath leaves PATH out when looking for it (tests).
	TextureEncoder textures.Encoder
	ToddsZip       string
	ToddsNoPath    bool
	depFinder      *deps.Finder
	depFake        bool
	// SteamCollectionAPI reads Workshop collections (04 §9.5); "" is the public endpoint.
	SteamCollectionAPI string
	// Uninstall (uninstall.go): UninstallExe is the program to remove ("" this one; a
	// development build is never removed), UninstallKeepBase replaces Documents as the
	// default home of kept saves and packs, ArchoProbe lists the other RimArcho processes and
	// AfterExitFunc replaces starting the clean-up script (tests).
	UninstallExe      string
	UninstallKeepBase string
	UninstallLog      string // where the script lists what it could not delete ("" %TEMP%)
	ArchoProbe        func() []int
	AfterExitFunc     func(AfterExit) error
	// Uninstalled is set once an uninstall has run: every command is refused after it.
	Uninstalled bool
	// Self-update (selfupdate.go, 07 §5.1), all for tests: SelfUpdateAPI replaces GitHub's
	// API root, SelfVersion the running version, SelfProgram the program folder,
	// SelfVersionOf asking the new program its version, SelfOthers listing the other
	// RimArcho processes in a folder, SelfWaitPoll how often that is asked while waiting.
	SelfUpdateAPI string
	SelfVersion   string
	SelfProgram   string
	SelfVersionOf func(ctx context.Context, exe string) (string, error)
	SelfOthers    func(dir string) []*GameRun
	SelfWaitPoll  time.Duration

	analyzing *pendingAnalysis

	dataIdx *dataIndex // community data indexes (04 §6.3), loaded on first use

	ixCache  *Index
	decl     *declReads     // shared with read sessions (shared.go)
	ownDecl  bool           // this App opened the directory and closes decl
	linkInst *linkInstances // InspectGameLinks, keyed like ixCache
	ixKey    string
	// pngSnaps: the snapshots with PNG textures (Texture optimisation), until the snapshots change
	pngSnaps           map[int64]bool
	pngSnapsKey        string
	stCache            *LoadoutState
	stKey              string
	saveHeads          saveHeadCache // save heads by path, size and mtime (saves.go)
	saveOwners         saveOwnerMemo // Def and class lookups of save attribution (saveowners.go)
	modsCopies         map[string]*modsCopiesCache
	linkRepairPreviews map[string]*LinkRepairPlan
	linkRepairSerial   uint64
	prCache            *ProblemSet
	prKey              string
	planPS             map[int64]*planProblems // PlanProblems, by plan

	logCache          *logCacheEntry
	subscriptionCache map[string]subscriptionCacheEntry
}

// Open opens (creating if needed) the data directory home.
func Open(home string) (*App, error) { return OpenWith(home, OpenOptions{}) }

// OpenOptions are the optional parts of opening the data directory.
type OpenOptions struct {
	// Upgrade receives the progress of a database upgrade while Open runs one (02 §7);
	// the surfaces show it (cli.upgradeReport).
	Upgrade func(store.UpgradeProgress)
}

// OpenWith is Open with options.
func OpenWith(home string, o OpenOptions) (*App, error) {
	defer perftrace.Span("app-open")()
	if err := os.MkdirAll(home, 0o755); err != nil {
		return nil, err
	}
	cfgPath := filepath.Join(home, "config.toml")
	if _, err := os.Stat(cfgPath); errors.Is(err, os.ErrNotExist) {
		os.WriteFile(cfgPath, []byte(DefaultConfig), 0o644)
	}
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		return nil, err
	}
	dbDone := perftrace.Span("db-open-migrate")
	st, err := store.OpenUpgrading(filepath.Join(home, "archo.db"), o.Upgrade)
	dbDone()
	if err != nil {
		return nil, err
	}
	a := &App{Home: home, Store: st, Config: cfg, Now: time.Now, Rand: rand.Reader, Loc: time.Local}
	a.ApplyNetwork()
	a.decl, a.ownDecl = &declReads{path: st.Path}, true
	if st.Meta("source-library:legacy-cleared") == "" {
		if err := os.RemoveAll(filepath.Join(home, "cache", "source")); err != nil {
			st.Close()
			return nil, fmt.Errorf("remove legacy source cache: %w", err)
		}
		if err := st.SetMeta("source-library:legacy-cleared", "true"); err != nil {
			st.Close()
			return nil, err
		}
	}
	// The interfaces check the managed directories at each start (ManagedDirectories is
	// read after Open, once overrides such as LockBase are installed); it never blocks.
	// Older installations have no usage metadata. Packs are under Home or an
	// explicit config path; unlike LockDir this never depends on LockBase.
	if dirExists(a.PacksDir()) && st.Meta(directoryKey("packs")) == "" {
		_ = st.SetMeta(directoryKey("packs"), a.PacksDir())
	}
	recoveryDone := perftrace.Span("startup-recovery")
	defer recoveryDone()
	rows, err := st.ManagedLinks()
	if err == nil {
		a.StartupLinkRecovery, err = a.linkRecoveries(rows)
	}
	if err != nil {
		st.Close()
		return nil, err
	}
	return a, nil
}

// Close releases the database.
func (a *App) Close() error {
	if a.ownDecl {
		a.decl.close()
	}
	return a.Store.Close()
}

// LoadConfig reads the small TOML subset RimArcho uses: `key = value` lines and comments.
func LoadConfig(path string) (Config, error) {
	c := Config{TombstoneDays: 90}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		t := strings.TrimSpace(sc.Text())
		if i := strings.Index(t, "#"); i >= 0 {
			t = strings.TrimSpace(t[:i])
		}
		if t == "" || strings.HasPrefix(t, "[") {
			continue
		}
		k, v, ok := strings.Cut(t, "=")
		if !ok {
			return c, fmt.Errorf("%s:%d: expected key = value", path, line)
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"`)
		switch k {
		case "tombstone_days":
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				return c, fmt.Errorf("%s:%d: tombstone_days must be a non-negative integer", path, line)
			}
			c.TombstoneDays = n
		}
	}
	return c, sc.Err()
}

// NewID mints an Instance/Lineage/Environment/Source ID.
func (a *App) NewID() string { return ids.New(a.Rand) }

// ErrNoEnv is returned when no environment has been configured.
var ErrNoEnv = errors.New("no environment configured (archo env detect, then archo env add <name> --detected)")

// Env returns the environment for this invocation: --env, else the current one.
func (a *App) Env() (*env.Environment, error) {
	if a.EnvName != "" {
		e, err := a.Store.EnvByName(a.EnvName)
		if err != nil {
			return nil, err
		}
		if e == nil {
			return nil, fmt.Errorf("no environment named %q (archo env list)", a.EnvName)
		}
		return e, nil
	}
	e, err := a.Store.CurrentEnv()
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrNoEnv
	}
	return e, nil
}

// FormatTime prints a timestamp in the app's time zone.
func (a *App) FormatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.In(a.Loc).Format("2006-01-02 15:04")
}

// FormatDate prints a date in the app's time zone.
func (a *App) FormatDate(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.In(a.Loc).Format("2006-01-02")
}

// ApplyNetwork hands the network settings to the process's requests (07 §2.2).
func (a *App) ApplyNetwork() {
	netcfg.Configure(netcfg.Settings{Proxy: a.Store.Meta("cfg:network.proxy"), Mirror: a.Store.Meta("cfg:network.github_mirror")})
}
