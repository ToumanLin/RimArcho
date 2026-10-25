package gui

import (
	"errors"
	"time"

	"github.com/ToumanLin/RimArcho/internal/modchanges"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func (s *API) SubscriptionsPage(library bool, of string) (*view.SubscriptionsPage, error) {
	a, err := s.detachedReader()
	if err != nil {
		return nil, err
	}
	defer a.Close()
	defer s.free.soon()
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	o := modchanges.Opts{Library: library}
	if of != "" {
		o.Of = view.LineageIDs(st.Ix, of)
		if len(o.Of) == 0 {
			return nil, errors.New(view.T("that mod is not known"))
		}
		o.OfName = st.Ix.ByID[of].DisplayName()
	}
	return view.BuildSubscriptions(a, st, o)
}

func (s *API) CheckSubscriptionSource(kind string) (string, error) {
	switch kind {
	case "workshop", "steamcmd", "git", "page", "local":
	default:
		return "", errors.New("unknown subscription source")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return settingQueued(s.queueSettingTaskLocked(view.TaskSourceCheck, []string{"source-check", kind}, "")), nil
}

func (s *API) WorkshopUpdatePreview(ref string) (*view.Preview, error) {
	a, err := s.detachedReader()
	if err != nil {
		return nil, err
	}
	defer a.Close()
	if err := a.UpdateWorkshopClient(ref, true); err != nil {
		return nil, err
	}
	ix, err := a.Index()
	if err != nil {
		return nil, err
	}
	m, err := ix.ResolveOne(ref)
	if err != nil {
		return nil, err
	}
	latest := a.SubscriptionSourceCheck("workshop").Latest[m.ID]
	lines := []string{m.DisplayName()}
	if latest > 0 {
		lines = append(lines, a.FormatTime(time.Unix(m.ACFTimeUpdated, 0))+" → "+a.FormatTime(time.Unix(latest, 0)))
	}
	lines = append(lines, view.T("Steam will download this Workshop update"))
	return &view.Preview{What: view.OpSubUpdate, Title: view.T("update subscription"), Apply: true, Lines: lines}, nil
}

func (s *API) WorkshopUpdate(ref string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.a.UpdateWorkshopClient(ref, true); err != nil {
		return "", err
	}
	return settingQueued(s.queueSettingTaskLocked(view.TaskWorkshopUpdate, []string{"workshop-update", ref}, "")), nil
}
