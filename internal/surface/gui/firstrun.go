//go:build windows || cgo

package gui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/selfupdate"
	"github.com/ToumanLin/RimArcho/internal/view"
)

const lowSpace = int64(10 << 30)

type FirstRunFolder struct {
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	Free     int64  `json:"free"`
	Low      bool   `json:"low"`
	Replace  bool   `json:"replace"`
	Workshop bool   `json:"workshop"`
	Done     bool   `json:"done"`
	Error    string `json:"error,omitempty"`
}

func folderSpace(path string) *FirstRunFolder {
	free := app.DiskFree(path)
	return &FirstRunFolder{Path: path, Free: free, Low: free >= 0 && free < lowSpace}
}

func (s *API) WorkshopFolder(target string) (*FirstRunFolder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	from, err := s.a.SelfProgramDir()
	if err != nil {
		return nil, err
	}
	_, workshop := selfupdate.InWorkshopFolder(from)
	if !workshop {
		return &FirstRunFolder{}, nil
	}
	if target == "" {
		target = filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "RimArcho")
	}
	info := folderSpace(target)
	info.Workshop = true
	if checked, err := s.a.CheckRelocationTarget(target); err != nil {
		info.Error = view.Humanize(nil, err.Error())
	} else {
		info.Path = checked
	}
	info.Bytes, err = selfupdate.ReleaseSize(from)
	if err != nil {
		return nil, err
	}
	if st, err := os.Stat(filepath.Join(target, "RimArcho.exe")); err == nil {
		info.Replace = st.Mode().IsRegular()
	}
	if info.Free >= 0 && info.Free < info.Bytes*2 {
		info.Error = view.T("not enough free space")
	}
	return info, nil
}

func (s *API) RelocateWorkshop(target string) error {
	s.mu.Lock()
	err := s.a.RelocateWorkshop(target)
	s.mu.Unlock()
	if err == nil && s.quit != nil {
		s.quit()
	}

	if err != nil {
		return errors.New(view.Humanize(nil, err.Error()))
	}
	return nil
}

func (s *API) ManagedFolder() *FirstRunFolder {
	s.mu.Lock()
	defer s.mu.Unlock()
	info := folderSpace(s.a.LocalDir())
	info.Done = s.a.Store.Meta("guide.managed-folders") == "done"
	return info
}

func (s *API) FolderSpace(path string) *FirstRunFolder { return folderSpace(path) }

func (s *API) ChooseManagedFolder(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.a.SetManagedRoot(path); err != nil {
		return errors.New(view.Humanize(nil, err.Error()))
	}
	return s.a.Store.SetMeta("guide.managed-folders", "done")
}

func (s *API) DiscoverManagerImports() []app.ManagerImport {
	return app.FindManagerImports(s.gameModsDirs())
}

// gameModsDirs are the environments' game Mods folders, where RimCrow's links lead to its
// install folder (07 §6.2).
func (s *API) gameModsDirs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.a.GameModsDirs()
}

// ManagerSourcesIn is what a chosen or dropped manager .exe or folder holds (07 §6.2): its
// data read like a found install's. nil for anything else, which ManagerFileKind classifies.
func (s *API) ManagerSourcesIn(path string) []app.ManagerImport { return app.ManagerSourcesIn(path) }

func (s *API) PickManagerImport() (string, error) {
	if s.x.native == nil {
		return "", nil
	}
	return s.x.native.openFile(view.T("import from RimSort, RimCrow or RimPy"), [][2]string{{"Manager program or data", "*.exe;*.db;*.json;*.xml;*.rml;*.txt"}, {"All files", "*.*"}})
}

type managerPreview struct {
	Kind, Path, Env, Signature, State string
	Sequence                          uint64
}

type ManagerPreview struct {
	Text     string          `json:"text"`
	Request  uint64          `json:"request"`
	Profiles []app.RCProfile `json:"profiles,omitempty"`
}

func managerSignature(kind, path string) string {
	hash := sha256.New()
	paths := []string{path}
	if kind == "rimcrow" {
		paths = append(paths, path+"-wal", filepath.Join(filepath.Dir(path), "rules", "user_rules.json"))
	}
	if kind == "rimsort-aux" {
		paths = append(paths, path+"-wal")
	}
	for _, p := range paths {
		hash.Write([]byte(p))
		file, err := os.Open(p)
		if err != nil {
			hash.Write([]byte(err.Error()))
			continue
		}
		_, err = io.Copy(hash, file)
		file.Close()
		if err != nil {
			hash.Write([]byte(err.Error()))
		}
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (s *API) managerReport(kind, path string, dry bool) (string, error) {
	var r *app.RimCrowReport
	var err error
	switch kind {
	case "rimcrow":
		r, err = s.a.ImportRimCrow(app.RimCrowOptions{DB: path, DryRun: dry, Tags: view.ManagerTags()})
	case "rimsort-aux":
		r, err = s.a.ImportRimSortAux(app.RimSortAuxOptions{DB: path, DryRun: dry, Tags: view.ManagerTags()})
	case "rimsort-rules":
		r, err = s.a.ImportRimSortRules(path, dry)
	default:
		return "", errors.New("choose a RimCrow database or RimSort user rules")
	}
	if err != nil {
		return "", err
	}
	if dry {
		return view.ManagerImportPreviewText(r), nil
	}
	return view.ManagerImportText(r), nil
}

func (s *API) PreviewManagerImport(kind, path string) (*ManagerPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.x.seq++
	s.x.manager = nil
	e, err := s.a.Env()
	if err != nil && !(kind == "rimcrow" && errors.Is(err, app.ErrNoEnv)) {
		return nil, err
	}
	envID := ""
	if e != nil {
		envID = e.ID
	}
	signature := s.managerSourceSignature(kind, path)
	text, err := s.managerReport(kind, path, true)
	if err != nil {
		return nil, err
	}
	if signature != s.managerSourceSignature(kind, path) {
		return nil, errors.New("source data changed; preview it again")
	}
	s.x.manager = &managerPreview{kind, path, envID, signature, s.a.Store.ChangeKey(), s.x.seq}
	preview := &ManagerPreview{Text: text, Request: s.x.seq}
	if kind == "rimcrow" {
		r, err := s.a.ImportRimCrow(app.RimCrowOptions{DB: path, DryRun: true})
		if err != nil {
			return nil, err
		}
		preview.Profiles = append([]app.RCProfile(nil), r.Profiles...)
		for i := range preview.Profiles {
			preview.Profiles[i].DisplayStatus = view.ProfileStatus(preview.Profiles[i].Status)
		}
	}
	return preview, nil
}

func (s *API) ApplyManagerImport(request uint64) (string, error) {
	return s.applyManagerImport(request, nil, false)
}
func (s *API) ApplyManagerImportProfiles(request uint64, ids []string) (string, error) {
	return s.applyManagerImport(request, ids, true)
}
func (s *API) applyManagerImport(request uint64, ids []string, selected bool) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.x.manager
	e, err := s.a.Env()
	if err != nil && !(p != nil && p.Kind == "rimcrow" && errors.Is(err, app.ErrNoEnv)) {
		return "", err
	}
	envID := ""
	if e != nil {
		envID = e.ID
	}
	if p == nil || p.Sequence != request || p.Sequence != s.x.seq || p.Env != envID || p.State != s.a.Store.ChangeKey() {
		return "", errors.New("import preview changed; preview it again")
	}
	if p.Signature != s.managerSourceSignature(p.Kind, p.Path) {
		return "", errors.New("source data changed; preview it again")
	}
	var text string
	if p.Kind == "rimcrow" && selected {
		r, e := s.a.ImportRimCrow(app.RimCrowOptions{DB: p.Path, SelectProfiles: true, ProfileIDs: ids, Tags: view.ManagerTags()})
		err = e
		if e == nil {
			text = view.ManagerImportText(r)
		}
	} else {
		text, err = s.managerReport(p.Kind, p.Path, false)
	}
	s.x.manager = nil
	s.board, s.needs = nil, nil
	return text, err
}

func (s *API) FinishWorkshopRelocation() error { return s.a.FinishWorkshopRelocation() }

func (s *API) ManagerFileKind(path string) string {
	if strings.EqualFold(filepath.Base(path), "aux_metadata.db") {
		return "rimsort-aux"
	}
	if strings.EqualFold(filepath.Ext(path), ".db") {
		return "rimcrow"
	}
	if strings.EqualFold(filepath.Ext(path), ".json") {
		if st, err := os.Stat(path); err != nil || st.Size() > 4<<20 {
			return "list"
		}
		b, err := os.ReadFile(path)
		var data map[string]json.RawMessage
		if err == nil && len(b) <= 4<<20 && json.Unmarshal(b, &data) == nil && data["rules"] != nil && data["entries"] == nil {
			return "rimsort-rules"
		}
	}
	return "list"
}

// Profile Config files live outside the source database and can change independently.
func (s *API) managerSourceSignature(kind, path string) string {
	signature := managerSignature(kind, path)
	if kind == "rimcrow" {
		rep, err := s.a.ImportRimCrow(app.RimCrowOptions{DB: path, DryRun: true})
		if err != nil {
			return signature + err.Error()
		}
		data, _ := json.Marshal(rep.Profiles)
		sum := sha256.Sum256(data)
		signature += hex.EncodeToString(sum[:])
	}
	return signature
}
