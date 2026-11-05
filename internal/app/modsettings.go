package app

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Settings follow the Mod (09 §3). The game names a Mod's settings file after the folder it
// sees the Mod under: Config/Mod_<folder>_<Mod class>.xml. A Mod that moves from its
// Workshop folder to an _Archo_ link, a pack link or another copy would read a new file and
// start with its defaults, so before the game starts RimArcho copies the settings saved
// under an earlier folder name to the new one. It never overwrites a file and never removes
// the original.

// SettingsCarry is one settings file copied to the name a Mod's current folder gives it.
type SettingsCarry struct {
	Mod  string // the Mod's name
	From string // file name in the Config folder it was read from
	To   string // file name the game reads now
	Err  string // why the copy failed ("" when it was made, or would be in a dry run)
	src  string // the folder From is in
}

// SettingsFile is the file the game reads and writes a Mod's settings in:
// Verse.LoadedModManager.GetSettingsFilename, identical in 1.6 and 1.7.
func SettingsFile(folder, handle string) string {
	return sanitizeFilename("Mod_" + folder + "_" + handle + ".xml")
}

// settingsStem is the part of SettingsFile before the Mod class name.
func settingsStem(folder string) string {
	return strings.TrimSuffix(SettingsFile(folder, "X"), "X.xml")
}

// sanitizeFilename ports Verse.GenText.SanitizeFilename (1.6 and 1.7): split on the invalid
// characters dropping empty parts, join with "_", trim trailing dots. The set is Windows'
// Path.GetInvalidFileNameChars plus the game's own "/\{}<>:*|!@#$%^&*?", so a pack link
// "!ArchoPack_x" gives "Mod__ArchoPack_x_…".
func sanitizeFilename(s string) string {
	return strings.TrimRight(strings.Join(strings.FieldsFunc(s, invalidFilenameChar), "_"), ".")
}

func invalidFilenameChar(r rune) bool {
	return r < 32 || strings.ContainsRune(`"<>|:*?\/{}!@#$%^&`, r)
}

// settingsMod is one Mod the game is about to load: the folder it sees it under and the
// folders the same package was or is seen under elsewhere.
type settingsMod struct {
	name   string
	folder string
	old    []string
}

// settingsFolders lists, per package (lower case, without "_steam"), every folder name the
// game sees or saw an installed copy under: Workshop and Mods/ folders, other tools' links,
// another game's Mods/ folder and RimArcho's deployment names (09 §3). Copies no longer
// live still count: their files stay in Config.
func settingsFolders(ix *Index, aliases map[string][]store.Alias) map[string][]string {
	kinds := map[string]source.Kind{}
	for _, s := range ix.Sources {
		kinds[s.ID] = s.Kind
	}
	out := map[string][]string{}
	seen := map[string]bool{}
	add := func(pkg, folder string) {
		if folder == "" || strings.ContainsAny(folder, `/\`) {
			return
		}
		if k := pkg + "\x00" + strings.ToLower(folder); !seen[k] {
			seen[k] = true
			out[pkg] = append(out[pkg], folder)
		}
	}
	for _, m := range ix.Mods {
		if m.PackageID == "" || m.Kind == source.Official {
			continue
		}
		pkg := packageBase(strings.ToLower(m.PackageID))
		switch m.Kind {
		case source.Local, source.Workshop:
			add(pkg, m.Rel)
		case source.Elsewhere: // its own game's Mods/ folder, and its link here
			add(pkg, m.Rel)
			add(pkg, DeployName(m))
		default:
			add(pkg, DeployName(m))
		}
		for _, al := range aliases[m.ID] {
			if gameVisible(kinds[al.SourceID]) {
				add(pkg, al.Rel)
			}
		}
	}
	return out
}

// allFolders is every folder name in folders.
func allFolders(folders map[string][]string) []string {
	var out []string
	for _, fs := range folders {
		out = append(out, fs...)
	}
	return out
}

// listSettings is the job for what the list loads, reading and writing cfg: each instance
// with the folder the game sees it under and the package's other folders, including those
// Archo Monitor recorded in cfg's save-data root. The folder is the registry's (what a write
// makes of Mods/), or seen's for a test run, which uses Mods/ as it is.
func (a *App) listSettings(st *LoadoutState, seen map[string]string, cfg string) settingsJob {
	folders := settingsFolders(st.Ix, st.Aliases)
	addSeenFolders(folders, a.MonitorSettings(filepath.Dir(cfg)))
	job := settingsJob{read: []string{cfg}, have: cfg, write: cfg, known: allFolders(folders)}
	for _, en := range st.List {
		m := st.Ix.ByID[en.Instance]
		if m == nil || m.Kind == source.Official || m.PackageID == "" {
			continue
		}
		folder := seen[m.ID]
		if seen == nil && st.Reg.Loadable(m.ID) {
			folder = st.Reg.Folder(m.ID)
		}
		if folder == "" {
			continue
		}
		job.mods = append(job.mods, settingsMod{name: m.DisplayName(), folder: folder,
			old: folders[packageBase(strings.ToLower(m.PackageID))]})
	}
	return job
}

// settingsJob is what carryModSettings does. read lists the Config folders settings come
// from, the first with a file for a Mod class winning. A file in have counts as present and
// is never overwritten; copies go to write (have for launches; a test run's own Config,
// refreshed from have each run, 09 §7). known is every folder name the library has, so a
// file is told apart from one of a longer folder name.
type settingsJob struct {
	read  []string
	have  string
	write string
	mods  []settingsMod
	known []string
}

type settingsFileInfo struct {
	name  string
	dir   string
	class string
	mod   time.Time
}

// planModSettings is what carryModSettings copies; it reads only.
func planModSettings(job settingsJob) []SettingsCarry {
	// Every folder name the game may use. A file belongs to the longest stem it starts
	// with: both parts of a name can hold underscores.
	stems := map[string]bool{}
	for _, f := range job.known {
		stems[strings.ToLower(settingsStem(f))] = true
	}
	for _, m := range job.mods {
		stems[strings.ToLower(settingsStem(m.folder))] = true
		for _, o := range m.old {
			stems[strings.ToLower(settingsStem(o))] = true
		}
	}
	present := map[string]bool{}
	if ents, err := os.ReadDir(job.have); err == nil {
		for _, e := range ents {
			present[strings.ToLower(e.Name())] = true
		}
	}
	// stem → class → file, per read folder.
	byDir := make([]map[string]map[string]settingsFileInfo, len(job.read))
	for i, dir := range job.read {
		byDir[i] = readSettingsFiles(dir, stems)
	}
	var out []SettingsCarry
	for _, m := range job.mods {
		if m.folder == "" {
			continue
		}
		stem := settingsStem(m.folder)
		done := map[string]bool{}
		for _, files := range byDir {
			found := map[string]settingsFileInfo{}
			for _, o := range m.old {
				if strings.EqualFold(o, m.folder) {
					continue
				}
				for class, f := range files[strings.ToLower(settingsStem(o))] {
					if done[strings.ToLower(class)] {
						continue
					}
					if cur, ok := found[strings.ToLower(class)]; !ok || f.mod.After(cur.mod) {
						found[strings.ToLower(class)] = f
					}
				}
			}
			classes := make([]string, 0, len(found))
			for c := range found {
				classes = append(classes, c)
			}
			sort.Strings(classes)
			for _, c := range classes {
				f := found[c]
				done[c] = true
				to := stem + f.class + ".xml"
				if present[strings.ToLower(to)] {
					continue
				}
				out = append(out, SettingsCarry{Mod: m.name, From: f.name, To: to, src: f.dir})
				present[strings.ToLower(to)] = true
			}
		}
	}
	return out
}

// readSettingsFiles groups dir's Mod settings files by the known stem they belong to.
func readSettingsFiles(dir string, stems map[string]bool) map[string]map[string]settingsFileInfo {
	out := map[string]map[string]settingsFileInfo{}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range ents {
		name := e.Name()
		low := strings.ToLower(name)
		if len(low) != len(name) || !e.Type().IsRegular() || !strings.HasPrefix(low, "mod_") || !strings.HasSuffix(low, ".xml") {
			continue
		}
		base := low[:len(low)-len(".xml")]
		stem := ""
		for i := len("mod_"); i < len(base); i++ {
			if base[i] == '_' && stems[base[:i+1]] {
				stem = base[:i+1]
			}
		}
		class := name[len(stem) : len(name)-len(".xml")]
		if stem == "" || class == "" || strings.Contains(class, ".") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		if out[stem] == nil {
			out[stem] = map[string]settingsFileInfo{}
		}
		out[stem][strings.ToLower(class)] = settingsFileInfo{name: name, dir: dir, class: class, mod: fi.ModTime()}
	}
	return out
}

// carryModSettings copies what planModSettings found; a failed copy is reported in its Err
// and never stops the launch.
func carryModSettings(job settingsJob, check func(path string) error) []SettingsCarry {
	plan := planModSettings(job)
	if len(plan) == 0 {
		return nil
	}
	if err := os.MkdirAll(job.write, 0o755); err != nil {
		for i := range plan {
			plan[i].Err = err.Error()
		}
		return plan
	}
	for i, c := range plan {
		if err := copySettingsFile(job, c, check); err != nil {
			plan[i].Err = err.Error()
		}
	}
	return plan
}

func copySettingsFile(job settingsJob, c SettingsCarry, check func(path string) error) error {
	src := filepath.Join(c.src, c.From)
	dst := filepath.Join(job.write, c.To)
	if check != nil {
		if err := check(dst); err != nil {
			return err
		}
	}
	keep := strings.EqualFold(filepath.Clean(job.write), filepath.Clean(job.have))
	if _, err := os.Lstat(dst); err == nil && keep {
		return nil // appeared meanwhile: never overwritten
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	tmp := dst + ".rimarcho-tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	_ = os.Chtimes(tmp, fi.ModTime(), fi.ModTime())
	if _, err := os.Lstat(dst); err == nil && keep {
		os.Remove(tmp)
		return nil
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// copyHugsLib gives a pack HugsLib's settings (<user data>/HugsLib/, named by Mod rather
// than folder) while the pack has none of its own (10 §3 step 4).
func copyHugsLib(from, to string) ([]string, error) {
	dst := filepath.Join(to, "HugsLib")
	if _, err := os.Lstat(dst); err == nil || !errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	ents, err := os.ReadDir(filepath.Join(from, "HugsLib"))
	if err != nil {
		return nil, nil
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return nil, err
	}
	var copied []string
	var errs []error
	for _, e := range ents {
		if !e.Type().IsRegular() {
			continue
		}
		fi, err := e.Info()
		if err == nil {
			err = copyFile(filepath.Join(from, "HugsLib", e.Name()), filepath.Join(dst, e.Name()), fi)
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		copied = append(copied, "HugsLib/"+e.Name())
	}
	return copied, errors.Join(errs...)
}
