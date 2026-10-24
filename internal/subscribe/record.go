// Package subscribe stores subscription provenance alongside a mod's files.
package subscribe

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const File = ".rimarcho/source.json"

type Record struct {
	Kind        string    `json:"kind"`
	URL         string    `json:"url,omitempty"`
	Repo        string    `json:"repo,omitempty"`
	WorkshopID  string    `json:"workshop_id,omitempty"`
	RefType     string    `json:"ref_type,omitempty"`
	Ref         string    `json:"ref,omitempty"`
	Installed   string    `json:"installed"`
	InstalledAt time.Time `json:"installed_at"`
	Latest      string    `json:"latest,omitempty"`
	CheckedAt   time.Time `json:"checked_at,omitempty,omitzero"`
	CheckError  string    `json:"check_error,omitempty"`
}

func (r *Record) GitURL() string {
	if r.Repo != "" {
		return r.Repo
	}
	return r.URL
}

func (r *Record) HasUpdate() bool { return r.Latest != "" && r.Latest != r.Installed }

func Read(dir string) (*Record, error) {
	b, err := os.ReadFile(filepath.Join(dir, File))
	if err != nil {
		return nil, err
	}
	var r Record
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func Write(dir string, r *Record) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, File)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".source-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(b, '\n')); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
