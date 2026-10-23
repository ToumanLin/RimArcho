package gui

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/view"
)

// The list management page and its export sub-page (06 §2.17). Every verb is a view call
// over the same app functions the CLI's plan versions / follow / sync use.

// PlanManage is the page with the list sel picked (0: the current one). No network.
func (s *API) PlanManage(sel int64) (*view.PlanManage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.BuildPlanManage(s.a, sel)
}

// PlanCheck reads a list's followed collections when they are older than ten minutes
// (the network); true when something was read and the page should be read again.
func (s *API) PlanCheck(id int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.CheckPlanFollows(s.a, id)
}

// PlanVersionsSet sets a list's game versions; none goes back to the default.
func (s *API) PlanVersionsSet(id int64, versions []string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.text(view.SetPlanVersions(s.a, id, versions))
}

// PlanFollow links a Steam Workshop collection to a list (reads it once).
func (s *API) PlanFollow(id int64, link, mode string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.text(view.FollowPlan(s.a, id, strings.TrimSpace(link), mode))
}

// PlanFollowMode sets how catching up treats a collection (add, follow).
func (s *API) PlanFollowMode(id int64, ws, mode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.SetFollowMode(s.a, id, ws, mode)
}

// PlanUnfollow unlinks a collection; the list stays as it is.
func (s *API) PlanUnfollow(id int64, ws string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.text(view.UnfollowPlan(s.a, id, ws))
}

// PlanSync catches a list up with a collection: one undoable edit of that list.
func (s *API) PlanSync(id int64, ws string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.needs = nil
	return s.text(view.SyncPlan(s.a, id, ws))
}

// PlanExport renders a list in a format for the export sub-page.
func (s *API) PlanExport(id int64, format string) (*view.PlanExport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.BuildPlanExport(s.a, id, format)
}

// PlanExportSave writes a list to a file; overwrite only for a path the save dialog chose.
func (s *API) PlanExportSave(id int64, format, path string, overwrite bool) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view.ExportPlanTo(s.a, id, format, path, overwrite)
}

// PlanExportCopy puts a list on the clipboard in a format.
func (s *API) PlanExportCopy(id int64, format string) (*Copied, error) {
	s.mu.Lock()
	t, err := view.ExportPlanText(s.a, id, format)
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
