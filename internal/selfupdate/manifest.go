package selfupdate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// UpdateDir is the folder inside the program folder that holds the download, the staged
// and previous files, the swap journal and the installed-entries record (17 §5.1).
const UpdateDir = ".archo-update"

// manifestName records, inside UpdateDir, the top-level entries the last update installed.
// The next update removes those its zip no longer has, so a release may add or drop
// top-level entries without a stepping-stone release.
const manifestName = "files.json"

// ManifestPath is the installed-entries record of a program folder.
func ManifestPath(program string) string {
	return filepath.Join(program, UpdateDir, manifestName)
}

// Installed lists the release's top-level entries in a program folder: Files plus those the
// last update recorded. Installations from before the record have only Files.
func Installed(program string) []string {
	names := slices.Clone(Files)
	b, err := os.ReadFile(ManifestPath(program))
	if err != nil {
		return names
	}
	var recorded []string
	if json.Unmarshal(b, &recorded) != nil {
		return names
	}
	for _, name := range recorded {
		if EntryNameOK(name) && !slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, name) }) {
			names = append(names, name)
		}
	}
	return names
}

// WriteManifest records the top-level entries of a staged or installed release.
func WriteManifest(program string, names []string) error {
	b, err := json.Marshal(names)
	if err != nil {
		return err
	}
	path := ManifestPath(program)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return errors.Join(err, os.Remove(tmp))
	}
	return nil
}

// Entries lists a staged release's top-level names.
func Entries(stage string) ([]string, error) {
	list, err := os.ReadDir(stage)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(list))
	for _, e := range list {
		if !EntryNameOK(e.Name()) {
			return nil, errors.New("staged release has unsafe entry " + e.Name())
		}
		names = append(names, e.Name())
	}
	return names, nil
}

// EntryNameOK reports whether name may be a release's top-level entry: a plain name, not
// hidden (UpdateDir and relocation folders start with a dot) and not a Windows device name.
func EntryNameOK(name string) bool {
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\:*?"<>|`) || strings.HasPrefix(name, ".") {
		return false
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return false
	}
	return !windowsDevice(name)
}

func windowsDevice(name string) bool {
	base, _, _ := strings.Cut(strings.ToUpper(name), ".")
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9' {
		return true
	}
	return false
}
