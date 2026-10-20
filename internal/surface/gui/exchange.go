package gui

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// Import and export of the mod list (10 §4): the page previews a list before anything
// changes, and applies it as one undoable step; export writes a file or copies text.
// The file dialogs and the clipboard are the window's (Wails); the development harness
// has none, so every path can also be typed.

// native is what the window offers beyond the page; nil in the harness and in tests.
type native interface {
	openFile(title string, filters [][2]string) (string, error)
	saveFile(title, name string, filters [][2]string) (string, error)
	setClipboard(text string) bool
	clipboard() (string, bool)
}

// exchange is the API's import/export state: the list read for import, kept between its
// preview and its apply.
type exchange struct {
	native native
	imp    *view.Import
}

var listFilters = [][2]string{
	{"Mod lists (*.xml;*.rml;*.json;*.txt;*.rws)", "*.xml;*.rml;*.json;*.txt;*.rws"},
	{"All files (*.*)", "*.*"},
}

// Exchange says what the page can use: native dialogs, and where typed paths start.
type Exchange struct {
	Dialogs bool                `json:"dialogs"`
	Dir     string              `json:"dir"`
	Formats []view.ExportFormat `json:"formats"`
}

// ExchangeInfo is the import/export popup's start data.
func (s *API) ExchangeInfo() *Exchange {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &Exchange{Dialogs: s.x.native != nil, Dir: view.DefaultExportDir(), Formats: view.ExportFormats(s.a)}
}

// PickImportFile shows the open dialog; "" when it was cancelled or there is none.
func (s *API) PickImportFile() (string, error) {
	if s.x.native == nil {
		return "", nil
	}
	return s.x.native.openFile(view.T("import a mod list"), listFilters)
}

// PickExportFile shows the save dialog for a format; "" when it was cancelled.
func (s *API) PickExportFile(format string) (string, error) {
	if s.x.native == nil {
		return "", nil
	}
	s.mu.Lock()
	name, ext := "", ""
	for _, f := range view.ExportFormats(s.a) {
		if f.ID == format {
			name, ext = f.File, filepath.Ext(f.File)
		}
	}
	s.mu.Unlock()
	filters := [][2]string{{"*" + ext, "*" + ext}, {"All files (*.*)", "*.*"}}
	return s.x.native.saveFile(view.T("export the mod list"), name, filters)
}

// Clipboard reads the clipboard's text ("" when the window has none to offer; the page
// then asks the browser).
func (s *API) Clipboard() string {
	if s.x.native == nil {
		return ""
	}
	t, _ := s.x.native.clipboard()
	return t
}

// ImportPreview reads a list (kind: file, text or collection) and shows what importing it
// would do; nothing changes yet.
func (s *API) ImportPreview(kind, src string) (*view.ImportPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.x.imp = nil
	imp, err := view.ReadImport(s.a, kind, src)
	if err != nil {
		return nil, errors.New(view.Humanize(s.boardState(), err.Error()))
	}
	p, err := view.PreviewImport(s.a, imp)
	if err != nil {
		return nil, err
	}
	s.x.imp = imp
	return p, nil
}

// ImportApply puts the previewed list into the workspace: it replaces the enabled list,
// or with merge adds the installed mods it has that the list lacks. One undo step;
// ModsConfig.xml is written only by save.
func (s *API) ImportApply(merge bool) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.x.imp == nil {
		return "", errors.New(view.T("nothing to import"))
	}
	imp := s.x.imp
	s.x.imp = nil
	s.needs = nil
	return s.text(view.ApplyImport(s.a, imp, merge))
}

// ExportSave writes the list to a file. overwrite is for paths the save dialog chose (it
// asked); a typed path never replaces a file.
func (s *API) ExportSave(format, path string, overwrite bool) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.ExportTo(s.a, format, path, overwrite)
}

// ExportCopy renders the list in a short format and puts it on the clipboard; the text
// comes back so the page can copy it itself where the window has no clipboard.
type Copied struct {
	Text   string `json:"text"`
	Copied bool   `json:"copied"`
	Line   string `json:"line"`
}

// ExportCopy copies the list (a share code, the text list).
func (s *API) ExportCopy(format string) (*Copied, error) {
	s.mu.Lock()
	t, err := view.ExportText(s.a, format)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	c := &Copied{Text: strings.TrimRight(t, "\n")}
	if s.x.native != nil {
		c.Copied = s.x.native.setClipboard(c.Text)
	}
	c.Line = view.Tf("copied: %s", firstLine(c.Text))
	if len([]rune(c.Line)) > 90 {
		c.Line = string([]rune(c.Line)[:89]) + "…"
	}
	return c, nil
}

// wailsNative is the window's dialogs and clipboard.
type wailsNative struct{ app *application.App }

func (w wailsNative) openFile(title string, filters [][2]string) (string, error) {
	d := w.app.Dialog.OpenFile().SetTitle(title).CanChooseFiles(true)
	for _, f := range filters {
		d.AddFilter(f[0], f[1])
	}
	if win := w.app.Window.Current(); win != nil {
		d.AttachToWindow(win)
	}
	return d.PromptForSingleSelection()
}

func (w wailsNative) saveFile(title, name string, filters [][2]string) (string, error) {
	d := w.app.Dialog.SaveFile().SetFilename(name).SetDirectory(view.DefaultExportDir())
	d.SetMessage(title)
	for _, f := range filters {
		d.AddFilter(f[0], f[1])
	}
	if win := w.app.Window.Current(); win != nil {
		d.AttachToWindow(win)
	}
	return d.PromptForSingleSelection()
}

func (w wailsNative) setClipboard(text string) bool { return w.app.Clipboard.SetText(text) }
func (w wailsNative) clipboard() (string, bool)     { return w.app.Clipboard.Text() }
