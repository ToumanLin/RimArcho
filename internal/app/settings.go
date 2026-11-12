package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// What the settings page (16 §2.14) shows and changes besides environments and config keys:
// the database, the caches and the launch arguments.

// DBStatus is the size of the database files.
type DBStatus struct {
	Path    string
	Bytes   int64 // archo.db + WAL
	Backups []string
}

func (a *App) dbPath() string { return filepath.Join(a.Home, "archo.db") }

func (a *App) backupDir() string { return filepath.Join(a.Home, "backups") }

// DBInfo reports the database size and the backups RimArcho made.
func (a *App) DBInfo() (*DBStatus, error) {
	s := &DBStatus{Path: a.dbPath()}
	for _, suf := range []string{"", "-wal"} {
		if fi, err := os.Stat(a.dbPath() + suf); err == nil {
			s.Bytes += fi.Size()
		}
	}
	ents, _ := os.ReadDir(a.backupDir())
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".db") {
			s.Backups = append(s.Backups, filepath.Join(a.backupDir(), e.Name()))
		}
	}
	return s, nil
}

// DBBackup writes a consistent copy of the database (VACUUM INTO) to <home>/backups.
func (a *App) DBBackup() (string, error) {
	if err := os.MkdirAll(a.backupDir(), 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(a.backupDir(), "archo-"+a.Now().In(a.Loc).Format("20060102-150405")+".db")
	if _, err := os.Stat(p); err == nil {
		return "", fmt.Errorf("%s exists", p)
	}
	if _, err := a.Store.DB.Exec(`VACUUM INTO ?`, p); err != nil {
		return "", err
	}
	return p, nil
}

// DBVacuum runs the shared maintenance pipeline and returns the size before and after.
func (a *App) DBVacuum() (before, after int64, err error) {
	r, err := a.Maintain(context.Background(), false, false)
	return r.Before, r.After, err
}

// Cache is one kind of cached data.
type Cache struct {
	Kind  string // previews, workshop, images, textures, logs
	Desc  string
	Bytes int64
	Files int
}

// cacheKinds maps a kind to its folder under <home>/cache ("" for the log analyses, which
// are in the database).
var cacheKinds = []struct{ kind, dir, desc string }{
	{"previews", "previews", "mod thumbnails"},
	{"workshop", "workshop", "Workshop descriptions"},
	{"images", "workshop-img", "images in descriptions"},
	{"textures", "textures", "PNG header cache"},
	{"logs", "", "Player.log analyses"},
}

// Caches reports each cache's size.
func (a *App) Caches() []Cache {
	var out []Cache
	for _, k := range cacheKinds {
		c := Cache{Kind: k.kind, Desc: k.desc}
		if k.dir == "" {
			a.Store.DB.QueryRow(`SELECT count(*), coalesce(sum(length(report)), 0) FROM log_cache`).Scan(&c.Files, &c.Bytes)
		} else {
			filepath.WalkDir(a.cacheDir(k.dir), func(_ string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					if fi, err := d.Info(); err == nil {
						c.Bytes += fi.Size()
						c.Files++
					}
				}
				return nil
			})
		}
		out = append(out, c)
	}
	return out
}

// ClearCache empties one cache; everything in it is rebuilt on demand.
func (a *App) ClearCache(kind string) (Cache, error) {
	for _, c := range a.Caches() {
		if c.Kind != kind {
			continue
		}
		for _, k := range cacheKinds {
			if k.kind != kind {
				continue
			}
			if k.dir == "" {
				if _, err := a.Store.DB.Exec(`DELETE FROM log_cache`); err != nil {
					return c, err
				}
				a.logCache = nil
				return c, nil
			}
			dir := a.cacheDir(k.dir)
			if filepath.Dir(dir) != filepath.Join(a.Home, "cache") {
				return c, fmt.Errorf("refusing to clear %s", dir)
			}
			return c, os.RemoveAll(dir)
		}
	}
	var kinds []string
	for _, k := range cacheKinds {
		kinds = append(kinds, k.kind)
	}
	return Cache{}, fmt.Errorf("unknown cache %q (%s)", kind, strings.Join(kinds, ", "))
}

// LaunchArgs are the extra arguments the game is started with (16 §2.14), per environment.
func (a *App) LaunchArgs(envID string) string { return a.Store.Meta("env:" + envID + ":launch_args") }

// SetLaunchArgs stores them ("" removes them).
func (a *App) SetLaunchArgs(envID, args string) error {
	e, err := a.Store.EnvByID(envID)
	if err != nil {
		return err
	}
	if e == nil {
		return fmt.Errorf("Game Config %s not found", envID)
	}
	if err := ValidateLaunchArgs(e.Name, args); err != nil {
		return err
	}
	return a.Store.SetMeta("env:"+envID+":launch_args", strings.TrimSpace(args))
}

// ValidateLaunchArgs keeps the game's data root under environment control (03 §6.2).
func ValidateLaunchArgs(name, args string) error {
	argv, err := ParseLaunchArgs(args)
	if err != nil {
		return err
	}
	for _, arg := range argv {
		key := strings.ToLower(strings.Trim(arg, "\"'"))
		if key == "-savedatafolder" || strings.HasPrefix(key, "-savedatafolder=") {
			return fmt.Errorf("launch argument %q overrides the Game Config's user data folder; use archo env set %s --user-data <dir> and remove -savedatafolder from the launch arguments", arg, quoteArg(name))
		}
	}
	return nil
}

func (a *App) checkLaunchArgs() error {
	e, err := a.Env()
	if err != nil {
		return err
	}
	return ValidateLaunchArgs(e.Name, a.LaunchArgs(e.ID))
}

// ParseLaunchArgs accepts lossless imported JSON argv or quoted command-line text.
func ParseLaunchArgs(args string) ([]string, error) {
	if strings.HasPrefix(strings.TrimSpace(args), "[") {
		var argv []string
		if err := json.Unmarshal([]byte(args), &argv); err != nil {
			return nil, fmt.Errorf("launch arguments must be a JSON array of strings: %w", err)
		}
		return argv, nil
	}
	var argv []string
	var word strings.Builder
	var quote rune
	started := false
	runes := []rune(args)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\\' && i+1 < len(runes) && quote == '"' && (runes[i+1] == '"' || runes[i+1] == '\\') {
			i++
			word.WriteRune(runes[i])
			started = true
			continue
		}
		if r == '"' || r == '\'' {
			if quote == 0 {
				quote = r
				started = true
				continue
			}
			if quote == r {
				quote = 0
				continue
			}
		}
		if unicode.IsSpace(r) && quote == 0 {
			if started {
				argv = append(argv, word.String())
				word.Reset()
				started = false
			}
			continue
		}
		word.WriteRune(r)
		started = true
	}
	if quote != 0 {
		return nil, fmt.Errorf("launch arguments contain an unclosed quote")
	}
	if started {
		argv = append(argv, word.String())
	}
	return argv, nil
}

// DisplayLaunchArgs hides the import storage format from editable player fields.
func DisplayLaunchArgs(args string) string {
	if !strings.HasPrefix(strings.TrimSpace(args), "[") {
		return args
	}
	argv, err := ParseLaunchArgs(args)
	if err != nil {
		return args
	}
	for i, arg := range argv {
		argv[i] = `"` + strings.ReplaceAll(strings.ReplaceAll(arg, `\`, `\\`), `"`, `\"`) + `"`
	}
	return strings.Join(argv, " ")
}
