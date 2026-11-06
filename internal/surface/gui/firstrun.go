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

func (s *API) DiscoverManagerImports() []app.ManagerImport { return app.FindManagerImports() }

func (s *API) PickManagerImport() (string, error) {
	if s.x.native == nil {
		return "", nil
	}
	return s.x.native.openFile(view.T("import from RimSort or RimCrow"), [][2]string{{"Manager data and Mod lists", "*.db;*.json;*.xml;*.rml;*.txt"}, {"All files", "*.*"}})
}

type managerPreview struct {
	Kind, Path, Env, Signature, State string
	Sequence                          uint64
}

type ManagerPreview struct {
	Text    string `json:"text"`
	Request uint64 `json:"request"`
}

func managerSignature(kind, path string) string {
	hash := sha256.New()
	paths := []string{path}
	if kind == "rimcrow" {
		paths = append(paths, path+"-wal", filepath.Join(filepath.Dir(path), "rules", "user_rules.json"))
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
		r, err = s.a.ImportRimCrow(app.RimCrowOptions{DB: path, DryRun: dry})
	case "rimsort-rules":
		r, err = s.a.ImportRimSortRules(path, dry)
	default:
		return "", errors.New("choose a RimCrow database or RimSort user rules")
	}
	if err != nil {
		return "", err
	}
	return view.ManagerImportText(r), nil
}

func (s *API) PreviewManagerImport(kind, path string) (*ManagerPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.x.seq++
	s.x.manager = nil
	e, err := s.a.Env()
	if err != nil {
		return nil, err
	}
	signature := managerSignature(kind, path)
	text, err := s.managerReport(kind, path, true)
	if err != nil {
		return nil, err
	}
	if signature != managerSignature(kind, path) {
		return nil, errors.New("source data changed; preview it again")
	}
	s.x.manager = &managerPreview{kind, path, e.ID, signature, s.a.Store.ChangeKey(), s.x.seq}
	return &ManagerPreview{text, s.x.seq}, nil
}

func (s *API) ApplyManagerImport(request uint64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.x.manager
	e, err := s.a.Env()
	if err != nil {
		return "", err
	}
	if p == nil || p.Sequence != request || p.Sequence != s.x.seq || p.Env != e.ID || p.State != s.a.Store.ChangeKey() {
		return "", errors.New("import preview changed; preview it again")
	}
	if p.Signature != managerSignature(p.Kind, p.Path) {
		return "", errors.New("source data changed; preview it again")
	}
	text, err := s.managerReport(p.Kind, p.Path, false)
	s.x.manager = nil
	s.board, s.needs = nil, nil
	return text, err
}

func (s *API) FinishWorkshopRelocation() error { return s.a.FinishWorkshopRelocation() }

func (s *API) ManagerFileKind(path string) string {
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
