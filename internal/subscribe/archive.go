package subscribe

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/source"
)

// Import copies a folder or extracts a player-supplied zip. It never follows links.
// It returns the mod root, which may be the one wrapper directory inside dest.
func Import(from, dest string) (string, error) {
	st, err := os.Stat(from)
	if err != nil {
		return "", err
	}
	if st.IsDir() {
		if source.IsLink(from) {
			return "", fmt.Errorf("import folder is a link: %s", from)
		}
		if source.Within(dest, from) {
			return "", fmt.Errorf("import source contains the destination")
		}
		err = filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(from, p)
			if err != nil {
				return err
			}
			if d.Type()&os.ModeSymlink != 0 || source.IsLink(p) {
				return fmt.Errorf("import contains a link: %s", p)
			}
			if d.IsDir() {
				return os.MkdirAll(filepath.Join(dest, rel), 0o755)
			}
			if !d.Type().IsRegular() {
				return fmt.Errorf("import contains a special file: %s", p)
			}
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			defer f.Close()
			return importFile(filepath.Join(dest, rel), f)
		})
	} else {
		if !strings.EqualFold(filepath.Ext(from), ".zip") {
			return "", fmt.Errorf("use a .zip archive or an extracted folder")
		}
		z, openErr := zip.OpenReader(from)
		if openErr != nil {
			return "", openErr
		}
		defer z.Close()
		for _, f := range z.File {
			name := strings.ReplaceAll(f.Name, "\\", "/")
			clean := filepath.Clean(filepath.FromSlash(name))
			if strings.HasPrefix(name, "/") || strings.Contains(name, ":") || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || (!f.Mode().IsRegular() && !f.Mode().IsDir()) {
				return "", fmt.Errorf("unsafe zip entry %q", f.Name)
			}
			p := filepath.Join(dest, clean)
			if f.FileInfo().IsDir() {
				err = os.MkdirAll(p, 0o755)
			} else {
				r, openErr := f.Open()
				if openErr != nil {
					return "", openErr
				}
				err = importFile(p, r)
				r.Close()
			}
			if err != nil {
				break
			}
		}
	}
	if err != nil {
		return "", err
	}
	if source.AboutState(dest) == source.AboutPresent {
		return dest, nil
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		return "", err
	}
	if len(entries) == 1 && entries[0].IsDir() {
		p := filepath.Join(dest, entries[0].Name())
		if source.AboutState(p) == source.AboutPresent {
			return p, nil
		}
	}
	return "", fmt.Errorf("import has no About/About.xml at the root or inside one top-level folder")
}

func importFile(path string, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, r)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// Probe says whether a folder or zip holds one Mod, without copying it: About/About.xml at
// its root or inside one top-level folder (what Import accepts).
func Probe(from string) error {
	st, err := os.Stat(from)
	if err != nil {
		return err
	}
	if st.IsDir() {
		if source.AboutState(from) == source.AboutPresent {
			return nil
		}
		entries, err := os.ReadDir(from)
		if err != nil {
			return err
		}
		if len(entries) == 1 && entries[0].IsDir() && source.AboutState(filepath.Join(from, entries[0].Name())) == source.AboutPresent {
			return nil
		}
		return fmt.Errorf("not a Mod folder: no About/About.xml at the root or inside one top-level folder")
	}
	if !strings.EqualFold(filepath.Ext(from), ".zip") {
		return fmt.Errorf("use a .zip archive or an extracted folder")
	}
	z, err := zip.OpenReader(from)
	if err != nil {
		return err
	}
	defer z.Close()
	tops := map[string]bool{}
	about := map[string]bool{} // top folder ("" for the root) → has About/About.xml
	for _, f := range z.File {
		parts := strings.Split(strings.Trim(strings.ReplaceAll(f.Name, "\\", "/"), "/"), "/")
		if len(parts) > 1 || f.FileInfo().IsDir() {
			tops[parts[0]] = true
		} else {
			tops[""] = true
		}
		n := len(parts)
		if n >= 2 && strings.EqualFold(parts[n-1], "About.xml") && strings.EqualFold(parts[n-2], "About") {
			switch n {
			case 2:
				about[""] = true
			case 3:
				about[parts[0]] = true
			}
		}
	}
	if about[""] || (len(tops) == 1 && func() bool {
		for t := range tops {
			return about[t]
		}
		return false
	}()) {
		return nil
	}
	return fmt.Errorf("not a Mod zip: no About/About.xml at the root or inside one top-level folder")
}
