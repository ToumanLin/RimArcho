package gui

import (
	"context"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/view"
)

type PlanCostPage struct {
	*view.PlanCost
	Request  uint64 `json:"request"`
	Loading  bool   `json:"loading"`
	Progress string `json:"progress"`
	Error    string `json:"error,omitempty"`
}

type loadCostMeasurement struct {
	mu      sync.Mutex
	disk    sync.Mutex
	workers sync.WaitGroup
	cancel  context.CancelFunc
	closed  bool
	request uint64
	page    PlanCostPage
}

// PlanLoadCost starts a background read; a newer selection cancels the older one.
func (s *API) PlanLoadCost(id int64) (*PlanCostPage, error) {
	m := &s.costMeasure
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, context.Canceled
	}
	if m.cancel != nil {
		m.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.request++
	request := m.request
	m.page = PlanCostPage{Request: request, Loading: true}
	m.workers.Add(1)
	go func() {
		defer m.workers.Done()
		defer cancel()
		defer s.free.soon()
		// Serialize cancelled reads through cache.Save so they cannot overwrite
		// another request's cache with an older snapshot.
		m.disk.Lock()
		defer m.disk.Unlock()
		if ctx.Err() != nil {
			return
		}
		r, err := s.detachedReader()
		if err != nil {
			s.publishLoadCost(ctx, request, nil, "", err)
			return
		}
		defer r.Close()
		r.WorkspaceSnapshot()
		cost, err := r.PlanLoadCost(ctx, id, func(done, total int) {
			s.publishLoadCost(ctx, request, nil, view.Tf("read %d / %d mods", done, total), nil)
		})
		if err != nil {
			s.publishLoadCost(ctx, request, nil, "", err)
			return
		}
		s.publishLoadCost(ctx, request, view.PlanLoadCost(cost), "", nil)
	}()
	p := m.page
	return &p, nil
}

// PlanLoadCostMeasure polls the current request; superseded requests have no result.
func (s *API) PlanLoadCostMeasure(request uint64) *PlanCostPage {
	m := &s.costMeasure
	m.mu.Lock()
	defer m.mu.Unlock()
	if request != m.request || m.closed {
		return nil
	}
	p := m.page
	return &p
}

func (s *API) publishLoadCost(ctx context.Context, request uint64, result *view.PlanCost, progress string, err error) {
	m := &s.costMeasure
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() != nil || request != m.request || m.closed {
		return
	}
	if result != nil {
		m.page.PlanCost, m.page.Loading = result, false
	}
	m.page.Progress = progress
	if err != nil {
		m.page.Error, m.page.Loading = view.Humanize(nil, err.Error()), false
	}
}

func (s *API) cancelLoadCost(close bool) {
	m := &s.costMeasure
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.request++
	if close {
		m.closed = true
	}
}

// The list management page and its export sub-page (16 §2.17). Every verb is a view call
// over the same app functions the CLI's plan versions / follow / sync use.

// PlanManage is the page with the list sel picked (0: the current one). No network.
func (s *API) PlanManage(sel int64) (*view.PlanManage, error) {
	s.plansRead.mu.Lock()
	defer s.plansRead.mu.Unlock()
	defer s.retainReader(&s.plansRead)
	s.mu.Lock()
	r, err := s.inventorySession(&s.plansRead)
	s.mu.Unlock()
	if err == nil {
		err = s.refreshPageIndex(r)
	}
	if err != nil {
		return nil, err
	}
	release, err := pinPageRead(r)
	if err != nil {
		return nil, err
	}
	defer release()
	return view.BuildPlanManage(r, sel)
}

// PlanCheck reads a list's followed collections when they are older than ten minutes
// (the network); true when something was read and the page should be read again.
func (s *API) PlanCheck(id int64) (bool, error) {
	r, err := s.detachedReader()
	if err != nil {
		return false, err
	}
	defer r.Close()
	return view.CheckPlanFollows(r, id)
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
