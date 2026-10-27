package gui

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/ToumanLin/RimArcho/internal/modchanges"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func (s *API) SubscriptionsPage(library bool, of string) (*view.SubscriptionsPage, error) {
	w := &s.subRead
	w.mu.Lock()
	defer w.mu.Unlock()
	defer s.retainSubscriptions()
	a, key, err := s.subscriptionSession()
	if err != nil {
		return nil, err
	}
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
	// The full inventory is scope independent; only history is narrowed by o.
	// Loadout may import an external ModsConfig.xml on this reader connection.
	key += "|" + a.Store.ChangeKey() + "|" + a.SubscriptionInventoryKey(st.Ix)
	if w.lib == nil || w.key != key {
		lib, err := a.Library()
		if err != nil {
			return nil, err
		}
		w.lib, w.key = lib, key
	}
	return view.BuildSubscriptionsFromLibrary(a, st, o, w.lib)
}

// AddSubscriptionInfo reads the account lines and newest installs without a loadout,
// change history or full inventory.
func (s *API) AddSubscriptionInfo() (*view.AddSubscriptionInfo, error) {
	w := &s.subRead
	w.mu.Lock()
	defer w.mu.Unlock()
	defer s.retainSubscriptions()
	a, _, err := s.subscriptionSession()
	if err != nil {
		return nil, err
	}
	return view.BuildAddSubscriptionInfo(a)
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

// OpenSubscriptionSource opens what 打开来源页 shows for a pending update (06 §2.9): the
// Workshop change notes, the compare view of two commits on GitHub / GitLab, or the
// repository or download page. The page passes only the reference.
func (s *API) OpenSubscriptionSource(ref string) (string, error) {
	a, err := s.detachedReader()
	if err != nil {
		return "", err
	}
	defer a.Close()
	subs, err := a.Subscriptions()
	if err != nil {
		return "", err
	}
	for _, sub := range subs {
		if sub.Ref != ref || sub.Record == nil || sub.Record.Kind == "steamcmd" {
			continue
		}
		u := view.SubscriptionSourceURL(sub.Record)
		if u == "" {
			break
		}
		if err := s.open(u); err != nil {
			return "", err
		}
		return view.T("opened the source page"), nil
	}
	ix, err := a.Index()
	if err != nil {
		return "", err
	}
	m, err := ix.ResolveOne(ref)
	if err != nil {
		return "", err
	}
	return s.OpenChangeNotes(m.WorkshopID())
}

// OpenModsFolder shows the game's Mods folder, where local mods are copied (添加订阅).
func (s *API) OpenModsFolder() error {
	s.mu.Lock()
	ix, err := s.a.Index()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	dir := filepath.Join(ix.Env.GameRoot, "Mods")
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return errors.New(view.T("the folder is not there"))
	}
	if s.openDir != nil {
		return s.openDir(dir)
	}
	return openDir(dir)
}
