package view

import (
	"errors"
	"github.com/ToumanLin/RimArcho/internal/app"
	"os"
	"path/filepath"
	"strings"
)

// FileSearchReq is the GUI/TUI question. CLI/MCP retain their separate options.
type FileSearchReq struct {
	Pattern   string `json:"pattern"`
	Files     string `json:"files"`
	ModsQuery string `json:"modsQuery"`
	Game      bool   `json:"game"`
	Regex     bool   `json:"regex"`
	Case      bool   `json:"case"`
}
type FileSearchLine struct {
	Line  int              `json:"line"`
	Text  string           `json:"text"`
	Parts []SearchTextPart `json:"parts,omitempty"`
}
type FileSearchFile struct {
	Rel   string           `json:"rel"`
	Path  string           `json:"path"`
	Count int              `json:"count"`
	Lines []FileSearchLine `json:"lines"`
}
type FileSearchMod struct {
	ID    string           `json:"id"`
	Name  string           `json:"name"`
	Copy  string           `json:"copy"`
	Count int              `json:"count"`
	Files []FileSearchFile `json:"files"`
}

// FileSearchPage is the TUI's accumulated stream, never a page of CLI output.
type FileSearchPage struct {
	Summary string
	Mods    []FileSearchMod
	Hint    string
	Notes   []string
}

// SearchFolder authorizes opening and reading only within this copy or the Source
// library. Resolve links before comparing roots, including the root itself.
func SearchFolder(ix *app.Index, libraryRoot, id, path string) (string, error) {
	m := ix.ByID[id]
	if !(m != nil && searchInside(ix.ModDir(m), path, false)) && !((m != nil || strings.HasPrefix(id, "game:")) && searchInside(libraryRoot, path, false)) {
		return "", errors.New(T("the folder is not there"))
	}
	dir := filepath.Dir(path)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return "", errors.New(T("the folder is not there"))
	}
	return dir, nil
}
