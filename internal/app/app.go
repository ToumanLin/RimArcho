// Package app holds the operations behind every command: environments, scanning and
// reconciliation, reference resolution and identity maintenance. Surfaces reach it only
// through internal/cmds.
package app

import (
	"bufio"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rimarcho/rimarcho/internal/env"
	"github.com/rimarcho/rimarcho/internal/ids"
	"github.com/rimarcho/rimarcho/internal/store"
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
	Home   string
	Store  *store.Store
	Config Config
	Now    func() time.Time
	Rand   io.Reader
	Loc    *time.Location // time zone for printed times
	// EnvName overrides the current environment for one invocation (--env).
	EnvName string
	// Detect configures environment detection (tests point it at fixtures).
	Detect env.DetectOptions
	// Launcher starts the game; nil means ExecLauncher.
	Launcher Launcher
	// AnalyzeRunner brings the Def/Patch analysis up to date; nil means the `archo analyze`
	// child process (03 §3). Tests run it in-process.
	AnalyzeRunner func(envName string, progress func(string)) error

	ixCache *Index
	ixKey   string
	stCache *LoadoutState
	stKey   string
	prCache *ProblemSet
	prKey   string
}

// Open opens (creating if needed) the data directory home.
func Open(home string) (*App, error) {
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
	st, err := store.Open(filepath.Join(home, "archo.db"))
	if err != nil {
		return nil, err
	}
	return &App{Home: home, Store: st, Config: cfg, Now: time.Now, Rand: rand.Reader, Loc: time.Local}, nil
}

// Close releases the database.
func (a *App) Close() error { return a.Store.Close() }

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
