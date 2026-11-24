package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Automatic save copies at launch (12 §4.4): before the game starts, the newest save of
// the Saves folder that launch uses is copied into <data>/backups/saves/<time>/, beside the
// player's own backups and in the same format, with a marker file that tells the
// copy is automatic. The copy runs alongside the game's start and never delays or blocks
// it; a failure is logged (operation log and app log) and the game plays on.

// LaunchCopyKeep is how many automatic copies are kept; older ones are deleted. Backups
// the player made are never deleted.
const LaunchCopyKeep = 5

// launchCopyMarker is the file in an automatic copy's folder that names its save.
const launchCopyMarker = "rimarcho-auto.json"

// launchCopyStaging begins the name of an automatic copy's folder while it is written;
// the folder takes its time name only once the copy and marker are complete.
const launchCopyStaging = ".auto-"

// launchCopyInfo is the marker's content: the save it copied as it was then.
type launchCopyInfo struct {
	Source  string    `json:"source"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
	File    string    `json:"file"`
}

// LaunchCopyOn: the setting saves.launch_copy is on (the default).
func (a *App) LaunchCopyOn() bool { return a.Store.Meta("cfg:saves.launch_copy") != "off" }

// LaunchCopyResult is what one automatic copy did.
type LaunchCopyResult struct {
	Save    string // the newest save's path ("" when the folder has none)
	Dir     string // the new copy's folder ("" when skipped)
	Skipped string // why nothing was copied: "off", "no saves", "unchanged"
	Pruned  int    // older automatic copies deleted
	Err     error
}

// startLaunchCopy starts the automatic copy of savesDir's newest save in the background
// (when the setting is on); WaitLaunchCopies or Close waits for it. The time and the
// environment are read here, so the copy touches nothing the caller goes on changing.
func (a *App) startLaunchCopy(savesDir string) {
	if savesDir == "" || !a.LaunchCopyOn() {
		return
	}
	stamp, envID := a.Now().In(a.Loc).Format(backupStamp), a.opEnv()
	a.launchCopies.Add(1)
	go func() {
		defer a.launchCopies.Done()
		r := a.launchCopy(savesDir, stamp, envID)
		if r.Err != nil {
			log.Printf("launch: automatic save copy failed: %v", r.Err)
		}
	}()
}

// WaitLaunchCopies waits for automatic save copies still running.
func (a *App) WaitLaunchCopies() { a.launchCopies.Wait() }

// LaunchCopy copies the newest .rws of savesDir into a new automatic backup folder, unless
// the newest automatic copy holds that save at the same size and modification time; then
// it deletes automatic copies beyond LaunchCopyKeep. The copy and a failure are recorded in
// the operation log; a skipped copy writes nothing.
func (a *App) LaunchCopy(savesDir string) LaunchCopyResult {
	if !a.LaunchCopyOn() {
		return LaunchCopyResult{Skipped: "off"}
	}
	return a.launchCopy(savesDir, a.Now().In(a.Loc).Format(backupStamp), a.opEnv())
}

func (a *App) launchCopy(savesDir, stamp, envID string) (r LaunchCopyResult) {
	root := filepath.Join(a.Home, "backups", "saves")
	newest, fi, err := newestSave(savesDir)
	if err != nil {
		r.Err = err
		a.logLaunchCopy(&r, envID, filepath.Base(savesDir))
		return r
	}
	if newest == "" {
		r.Skipped = "no saves"
		return r
	}
	r.Save = newest
	info := launchCopyInfo{Source: newest, Size: fi.Size(), ModTime: fi.ModTime().UTC(), File: fi.Name()}
	copies := launchCopies(root)
	for _, c := range copies { // the newest copy of this save
		if strings.EqualFold(filepath.Clean(c.info.Source), filepath.Clean(newest)) {
			if c.info.Size == info.Size && c.info.ModTime.Equal(info.ModTime) {
				r.Skipped = "unchanged"
				return r
			}
			break
		}
	}
	r.Err = writeLaunchCopy(root, stamp, newest, info, &r)
	if r.Err == nil {
		r.Pruned = pruneLaunchCopies(root, LaunchCopyKeep)
	}
	a.logLaunchCopy(&r, envID, strings.TrimSuffix(fi.Name(), filepath.Ext(fi.Name())))
	return r
}

// newestSave is the most recently modified .rws directly in dir ("" when it has none or
// does not exist).
func newestSave(dir string) (string, os.FileInfo, error) {
	ents, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	var best os.FileInfo
	for _, de := range ents {
		if de.IsDir() || !strings.EqualFold(filepath.Ext(de.Name()), ".rws") {
			continue
		}
		fi, err := de.Info()
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		if best == nil || fi.ModTime().After(best.ModTime()) {
			best = fi
		}
	}
	if best == nil {
		return "", nil, nil
	}
	return filepath.Join(dir, best.Name()), best, nil
}

type launchCopy struct {
	dir  string
	info launchCopyInfo
}

// launchCopies are the automatic copies under root, newest first (by folder name, the
// copy's time). A folder counts only when its marker reads and it holds nothing but the
// marker and the save it names: a folder the player changed is theirs.
func launchCopies(root string) []launchCopy {
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []launchCopy
	for _, de := range ents {
		if !de.IsDir() || strings.HasPrefix(de.Name(), ".") {
			continue
		}
		dir := filepath.Join(root, de.Name())
		b, err := os.ReadFile(filepath.Join(dir, launchCopyMarker))
		if err != nil {
			continue
		}
		var info launchCopyInfo
		if json.Unmarshal(b, &info) != nil || info.File == "" || filepath.Base(info.File) != info.File {
			continue
		}
		inner, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		only := true
		for _, e := range inner {
			if e.IsDir() || (e.Name() != launchCopyMarker && !strings.EqualFold(e.Name(), info.File)) {
				only = false
			}
		}
		if only {
			out = append(out, launchCopy{dir: dir, info: info})
		}
	}
	sort.Slice(out, func(i, j int) bool { return backupOrder(out[i].dir) > backupOrder(out[j].dir) })
	return out
}

// backupOrder sorts backup folders by time, then by their -N suffix.
func backupOrder(dir string) string {
	stamp, n, _ := strings.Cut(filepath.Base(dir), "-")
	rest := n
	if i := strings.Index(rest, "-"); i >= 0 {
		return stamp + "-" + rest[:i] + "-" + fmt.Sprintf("%06s", rest[i+1:])
	}
	return stamp + "-" + rest + "-000000"
}
