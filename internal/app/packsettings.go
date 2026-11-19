package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Mod settings that travel (10 §1 "settings", 07 §7.3): a pack carries its author's Mod
// settings by default, and a RimArcho list may carry them when the player asks. They are
// kept by package and settings class, not by file name: the game names a settings file
// after the folder it sees the Mod under (09 §3), which is different on the receiving side,
// so each file is written under the folder the Mod has there. Game settings (Prefs.xml,
// KeyPrefs.xml…) never travel.

// PackSettings is the Mod settings a manifest carries.
type PackSettings struct {
	Files   []PackSettingsFile `json:"files,omitempty"`
	HugsLib []PackHugsLib      `json:"hugslib,omitempty"`
}

// PackSettingsFile is one Config/Mod_<folder>_<class>.xml, its folder left out.
type PackSettingsFile struct {
	PackageID string    `json:"package_id"`
	Name      string    `json:"name,omitempty"` // the Mod's name, for previews
	Class     string    `json:"class"`
	Modified  time.Time `json:"modified,omitzero"`
	XML       string    `json:"xml"`
}

// PackHugsLib is one settings pack of HugsLib/ModSettings.xml: its element as HugsLib
// wrote it.
type PackHugsLib struct {
	PackageID string `json:"package_id"`
	Name      string `json:"name,omitempty"`
	ID        string `json:"id"`
	XML       string `json:"xml"`
}

// Limits on what a manifest carries: settings files are a few KB; anything far larger is
// not a settings file a player tuned.
const (
	settingsFileMax  = 4 << 20
	settingsTotalMax = 32 << 20
)

// Empty: nothing carried.
func (s *PackSettings) Empty() bool { return s == nil || len(s.Files) == 0 && len(s.HugsLib) == 0 }

// Mods is how many Mods have settings in s.
func (s *PackSettings) Mods() int {
	if s == nil {
		return 0
	}
	seen := map[string]bool{}
	for _, f := range s.Files {
		seen[packageBase(strings.ToLower(f.PackageID))] = true
	}
	for _, h := range s.HugsLib {
		seen[packageBase(strings.ToLower(h.PackageID))] = true
	}
	return len(seen)
}

// check refuses a manifest whose settings could not be written as files: a class that is
// not part of a file name, an element that is not one XML element.
func (s *PackSettings) check() error {
	if s == nil {
		return nil
	}
	total := 0
	for _, f := range s.Files {
		total += len(f.XML)
		if f.PackageID == "" || f.Class == "" || strings.ContainsAny(f.Class, `/\.`) || sanitizeFilename(f.Class) != f.Class {
			return fmt.Errorf("settings of %s: invalid class %q", f.PackageID, f.Class)
		}
		if len(f.XML) > settingsFileMax || !utf8.ValidString(f.XML) {
			return fmt.Errorf("settings of %s (%s): not a settings file", f.PackageID, f.Class)
		}
	}
	for _, h := range s.HugsLib {
		total += len(h.XML)
		doc, err := scanSettingsXML([]byte(h.XML))
		if h.PackageID == "" || h.ID == "" || err != nil || !strings.EqualFold(doc.root, h.ID) || len(h.XML) > settingsFileMax {
			return fmt.Errorf("HugsLib settings of %s: not one <%s> element", h.PackageID, h.ID)
		}
	}
	if total > settingsTotalMax {
		return errors.New("the settings it carries exceed 32 MB")
	}
	return nil
}

// collectSettings is the Mod settings of st's list: each loadable Mod's Config files under
// the folder the game sees it under, and its HugsLib packs.
func (a *App) collectSettings(st *LoadoutState) *PackSettings {
	out := &PackSettings{}
	root := st.Ix.Env.UserDataRoot
	if root == "" {
		return out
	}
	var listed []string
	for _, en := range st.List {
		listed = append(listed, st.Reg.Folder(en.Instance))
	}
	r := a.settingsReader(st.Ix, st.Aliases, listed)
	var hugs []byte
	var hdoc *xmlDoc
	hugsRead := false
	total := 0
	for _, en := range st.List {
		m := st.Ix.ByID[en.Instance]
		if m == nil || m.Kind == source.Official || m.PackageID == "" || !st.Reg.Loadable(m.ID) {
			continue
		}
		pkg := strings.ToLower(m.PackageID)
		sm := r.of(m, st.Reg.Folder(m.ID))
		for _, f := range sm.Files {
			if !f.Exists {
				continue
			}
			if f.Kind == SettingsKindHugsLib {
				if !hugsRead {
					hugsRead = true
					if b, err := os.ReadFile(f.Path); err == nil {
						if d, err := scanSettingsXML(b); err == nil {
							hugs, hdoc = b, d
						}
					}
				}
				if hdoc == nil {
					continue
				}
				for _, b := range hdoc.boxes {
					if strings.EqualFold(b.name, f.Class) && total+(b.span.end-b.span.start) <= settingsTotalMax {
						x := string(hugs[b.span.start:b.span.end])
						total += len(x)
						out.HugsLib = append(out.HugsLib, PackHugsLib{PackageID: pkg, Name: sm.Name, ID: b.name, XML: x})
						break
					}
				}
				continue
			}
			if f.Size > settingsFileMax || total+int(f.Size) > settingsTotalMax {
				continue
			}
			b, err := os.ReadFile(f.Path)
			if err != nil || !utf8.Valid(b) {
				continue
			}
			total += len(b)
			out.Files = append(out.Files, PackSettingsFile{PackageID: pkg, Name: sm.Name, Class: f.Class, Modified: f.Modified.UTC(), XML: string(b)})
		}
	}
	return out
}

// writeNewFile writes data to dst (absent) through a temporary file.
func writeNewFile(dst string, data []byte, mod time.Time) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".rimarcho-tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	if !mod.IsZero() {
		_ = os.Chtimes(tmp, mod, mod)
	}
	if _, err := os.Lstat(dst); err == nil {
		os.Remove(tmp)
		return fmt.Errorf("%s appeared meanwhile; left as it is", filepath.Base(dst))
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// putHugsLibPack puts elem (one HugsLib pack element named id) into HugsLib's settings file
// raw (nil: none yet). An existing pack of that id is replaced only with replace; changed
// says whether raw changed.
func putHugsLibPack(raw []byte, id, elem string, replace bool) (out []byte, changed bool, err error) {
	if raw == nil {
		return []byte("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<settings>\n  " + elem + "\n</settings>\n"), true, nil
	}
	doc, err := scanSettingsXML(raw)
	if err != nil {
		return nil, false, err
	}
	if doc.root != "settings" {
		return nil, false, errors.New("not HugsLib's settings file: root is <" + doc.root + ">")
	}
	for _, b := range doc.boxes {
		if strings.EqualFold(b.name, id) {
			if !replace || string(raw[b.span.start:b.span.end]) == elem {
				return raw, false, nil
			}
			return splice(raw, b.span.start, b.span.end, elem), true, nil
		}
	}
	nl := "\n"
	if bytes.Contains(raw, []byte("\r\n")) {
		nl = "\r\n"
	}
	rootIndent := indentOf(raw, doc.span.start)
	packIndent := rootIndent + "  "
	if len(doc.boxes) > 0 {
		last := doc.boxes[len(doc.boxes)-1]
		if in := indentOf(raw, doc.boxes[0].span.start); in != "" {
			packIndent = in
		}
		return splice(raw, last.span.end, last.span.end, nl+packIndent+elem), true, nil
	}
	if doc.span.selfClosing() {
		return splice(raw, doc.span.start, doc.span.end, "<"+doc.root+">"+nl+packIndent+elem+nl+rootIndent+"</"+doc.root+">"), true, nil
	}
	return splice(raw, doc.span.openEnd, doc.span.closeStart, nl+packIndent+elem+nl+rootIndent), true, nil
}

// hugsLibHas: raw (HugsLib's settings file) has a pack of id; same: with exactly elem.
func hugsLibHas(raw []byte, id, elem string) (has, same bool) {
	if raw == nil {
		return false, false
	}
	doc, err := scanSettingsXML(raw)
	if err != nil {
		return false, false
	}
	for _, b := range doc.boxes {
		if strings.EqualFold(b.name, id) {
			return true, string(raw[b.span.start:b.span.end]) == elem
		}
	}
	return false, false
}

// seedPackSettings writes a pack's settings into its user data for the folders this run
// gives its Mods (folderOf: package → folder): files that are missing only, never
// replacing one, so the player's later changes stay. dryRun lists them.
func seedPackSettings(s *PackSettings, folderOf map[string]string, userData string, dryRun bool) []SettingsCarry {
	if s.Empty() {
		return nil
	}
	var out []SettingsCarry
	cfg := filepath.Join(userData, "Config")
	for _, f := range s.Files {
		folder := folderOf[packageBase(strings.ToLower(f.PackageID))]
		if folder == "" {
			continue
		}
		to := SettingsFile(folder, f.Class)
		dst := filepath.Join(cfg, to)
		if _, err := os.Lstat(dst); err == nil {
			continue
		}
		c := SettingsCarry{Mod: f.Name, From: packSettingsFrom, To: to}
		if !dryRun {
			if err := writeNewFile(dst, []byte(f.XML), f.Modified); err != nil {
				c.Err = err.Error()
			}
		}
		out = append(out, c)
	}
	if len(s.HugsLib) == 0 {
		return out
	}
	path := filepath.Join(userData, HugsLibSettingsFile)
	raw, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return append(out, SettingsCarry{Mod: "HugsLib", From: packSettingsFrom, To: filepath.ToSlash(HugsLibSettingsFile), Err: err.Error()})
	}
	if err != nil {
		raw = nil
	}
	cur := raw
	var names []string
	for _, h := range s.HugsLib {
		if folderOf[packageBase(strings.ToLower(h.PackageID))] == "" {
			continue
		}
		next, changed, err := putHugsLibPack(cur, h.ID, h.XML, false)
		if err != nil {
			return append(out, SettingsCarry{Mod: h.Name, From: packSettingsFrom, To: filepath.ToSlash(HugsLibSettingsFile), Err: err.Error()})
		}
		if changed {
			cur = next
			names = append(names, h.Name)
		}
	}
	if len(names) == 0 {
		return out
	}
	c := SettingsCarry{Mod: strings.Join(names, ", "), From: packSettingsFrom, To: filepath.ToSlash(HugsLibSettingsFile)}
	if !dryRun {
		if err := writeReplace(path, cur); err != nil {
			c.Err = err.Error()
		}
	}
	return append(out, c)
}

// writeReplace writes data to path through a temporary file and a rename.
func writeReplace(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".rimarcho-tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// ---- a list's settings (07 §7.3 "apply settings") ----

// ListSettingsFile is one file "apply settings" writes or keeps.
type ListSettingsFile struct {
	Class   string `json:"class"`
	To      string `json:"to"` // the file name here (HugsLib/ModSettings.xml for a HugsLib pack)
	HugsLib bool   `json:"hugslib,omitempty"`
	Exists  bool   `json:"exists,omitempty"` // a file (or HugsLib pack) is here already
	Same    bool   `json:"same,omitempty"`   // with the same content: nothing to do
	Write   bool   `json:"write,omitempty"`  // it is (or would be) written
	Err     string `json:"error,omitempty"`
}

// ListSettingsMod is one Mod of the list's settings.
type ListSettingsMod struct {
	Mod       string             `json:"mod"`
	PackageID string             `json:"package"`
	Instance  string             `json:"instance,omitempty"`
	Missing   bool               `json:"missing,omitempty"` // not installed here: skipped
	Files     []ListSettingsFile `json:"files"`
}

// ListSettingsPlan is ApplyListSettings' preview or result.
type ListSettingsPlan struct {
	Config  string            `json:"config"`
	DryRun  bool              `json:"dryRun"`
	Replace bool              `json:"replace"`
	Mods    []ListSettingsMod `json:"mods"`
}

// Writes counts the files written (or to write).
func (p *ListSettingsPlan) Writes() (n int) {
	for _, m := range p.Mods {
		for _, f := range m.Files {
			if f.Write && f.Err == "" {
				n++
			}
		}
	}
	return n
}

// ApplyListSettings writes a list's settings into the current Game Config for the Mods
// installed here (the current list's copy first), each under the folder the game sees it
// under here. mods narrows it to
// those package IDs (none: all). A file that is here already is kept unless replace; one
// replaced goes to the recycle bin first (02 §1, §3), and for a HugsLib pack the whole
// HugsLib file does. Refused while a game runs; dryRun only plans.
func (a *App) ApplyListSettings(s *PackSettings, mods []string, replace, dryRun bool) (*ListSettingsPlan, error) {
	if s.Empty() {
		return nil, errors.New("the list carries no Mod settings")
	}
	if err := s.check(); err != nil {
		return nil, err
	}
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	root := st.Ix.Env.UserDataRoot
	if root == "" {
		return nil, errors.New("the Game Config has no user data directory; set one with archo env set --user-data <dir>")
	}
	cfg := filepath.Join(root, "Config")
	want := map[string]bool{}
	for _, m := range mods {
		if m = strings.TrimSpace(m); m != "" {
			want[packageBase(strings.ToLower(m))] = true
		}
	}
	type here struct{ id, name, folder string }
	inList := map[string]here{}
	// the current list's copy of a package, else an installed one the game can load (the
	// list may have been imported as a new list, not the one in the game yet)
	add := func(m *store.Mod) {
		if m == nil || m.Kind == source.Official || m.PackageID == "" || m.Status != identity.Live || !st.Reg.Loadable(m.ID) {
			return
		}
		pkg := packageBase(strings.ToLower(m.PackageID))
		if _, ok := inList[pkg]; !ok {
			inList[pkg] = here{m.ID, m.DisplayName(), st.Reg.Folder(m.ID)}
		}
	}
	for _, en := range st.List {
		add(st.Ix.ByID[en.Instance])
	}
	for _, m := range st.Ix.Mods {
		add(m)
	}
	plan := &ListSettingsPlan{Config: cfg, DryRun: dryRun, Replace: replace}
	byPkg := map[string]int{}
	row := func(pkg, name string) *ListSettingsMod {
		k := packageBase(strings.ToLower(pkg))
		if i, ok := byPkg[k]; ok {
			return &plan.Mods[i]
		}
		r := ListSettingsMod{Mod: name, PackageID: strings.ToLower(pkg)}
		if h, ok := inList[k]; ok && h.folder != "" {
			r.Mod, r.Instance = h.name, h.id
		} else {
			r.Missing = true
		}
		if r.Mod == "" {
			r.Mod = r.PackageID
		}
		byPkg[k] = len(plan.Mods)
		plan.Mods = append(plan.Mods, r)
		return &plan.Mods[len(plan.Mods)-1]
	}
	pick := func(pkg string) bool { return len(want) == 0 || want[packageBase(strings.ToLower(pkg))] }
	for _, f := range s.Files {
		if !pick(f.PackageID) {
			continue
		}
		r := row(f.PackageID, f.Name)
		if r.Missing {
			r.Files = append(r.Files, ListSettingsFile{Class: f.Class})
			continue
		}
		to := SettingsFile(inList[packageBase(r.PackageID)].folder, f.Class)
		lf := ListSettingsFile{Class: f.Class, To: to}
		if b, err := os.ReadFile(filepath.Join(cfg, to)); err == nil {
			lf.Exists, lf.Same = true, string(b) == f.XML
		} else if _, lerr := os.Lstat(filepath.Join(cfg, to)); lerr == nil {
			lf.Exists = true
		}
		lf.Write = !lf.Exists || replace && !lf.Same
		r.Files = append(r.Files, lf)
	}
	hugsPath := filepath.Join(root, HugsLibSettingsFile)
	hugs, herr := os.ReadFile(hugsPath)
	if herr != nil {
		hugs = nil
	}
	for _, h := range s.HugsLib {
		if !pick(h.PackageID) {
			continue
		}
		r := row(h.PackageID, h.Name)
		lf := ListSettingsFile{Class: h.ID, To: filepath.ToSlash(HugsLibSettingsFile), HugsLib: true}
		if !r.Missing {
			lf.Exists, lf.Same = hugsLibHas(hugs, h.ID, h.XML)
			lf.Write = !lf.Exists || replace && !lf.Same
		}
		r.Files = append(r.Files, lf)
	}
	sort.SliceStable(plan.Mods, func(i, j int) bool { return strings.ToLower(plan.Mods[i].Mod) < strings.ToLower(plan.Mods[j].Mod) })
	if dryRun || plan.Writes() == 0 {
		return plan, nil
	}
	if err := a.settingsGameClosed(); err != nil {
		return nil, err
	}
	xmlOf := map[string]PackSettingsFile{}
	for _, f := range s.Files {
		xmlOf[packageBase(strings.ToLower(f.PackageID))+"\x00"+strings.ToLower(f.Class)] = f
	}
	hugsOf := map[string]PackHugsLib{}
	for _, h := range s.HugsLib {
		hugsOf[strings.ToLower(h.ID)] = h
	}
	cur, hugsChanged := hugs, false
	var hugsFiles []*ListSettingsFile
	for i := range plan.Mods {
		r := &plan.Mods[i]
		for j := range r.Files {
			lf := &r.Files[j]
			if !lf.Write {
				continue
			}
			if lf.HugsLib {
				next, changed, err := putHugsLibPack(cur, lf.Class, hugsOf[strings.ToLower(lf.Class)].XML, true)
				if err != nil {
					lf.Err = err.Error()
					continue
				}
				if changed {
					cur, hugsChanged = next, true
					hugsFiles = append(hugsFiles, lf)
				}
				continue
			}
			f := xmlOf[packageBase(r.PackageID)+"\x00"+strings.ToLower(lf.Class)]
			dst := filepath.Join(cfg, lf.To)
			if lf.Exists {
				if err := a.ToTrash(dst); err != nil {
					lf.Err = err.Error()
					continue
				}
			}
			if err := writeNewFile(dst, []byte(f.XML), f.Modified); err != nil {
				lf.Err = err.Error()
			}
		}
	}
	if hugsChanged {
		fail := func(err error) {
			for _, lf := range hugsFiles {
				lf.Err = err.Error()
			}
		}
		if hugs != nil {
			// the whole file goes to the recycle bin first: it holds other Mods' settings too
			if err := a.ToTrash(hugsPath); err != nil {
				fail(err)
				return plan, nil
			}
		}
		if err := writeReplace(hugsPath, cur); err != nil {
			fail(err)
		}
	}
	return plan, nil
}

// packSettingsFrom is a SettingsCarry's From for a file the pack itself carries.
const packSettingsFrom = "the pack's settings"
