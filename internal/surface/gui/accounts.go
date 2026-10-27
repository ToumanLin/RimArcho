package gui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/accounts"
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Account operations own their session while doing I/O. Contending requests get
// an explicit busy error instead of waiting behind a download or login window.
func (s *API) beginAccount() (func(), error) {
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	if s.accountClosed {
		return nil, errors.New("account operations are closed")
	}
	if s.accountActive {
		return nil, errors.New(view.T("an account operation is running; try again when it finishes"))
	}
	s.accountActive = true
	return func() {
		s.accountMu.Lock()
		s.accountActive = false
		var files map[string]loverDownload
		if s.accountClosed {
			files, s.llFiles, s.ll = s.llFiles, nil, nil
		}
		s.accountMu.Unlock()
		for _, file := range files {
			_ = os.Remove(file.path)
		}
	}, nil
}

// AccountAction is deliberately absent from cmds, MCP and agent tool registries.
func (s *API) AccountAction(service, action string) ([]string, error) {
	allowed := map[string]map[string]bool{"steamcmd": {"login": true, "verify": true, "logout": true, "local-forget": true, "anonymous": true}, "loverslab": {"enable": true, "login": true, "verify": true, "forget": true}}
	if !allowed[service][action] {
		return nil, errors.New(view.T("unknown account action"))
	}
	release, err := s.beginAccount()
	if err != nil {
		return nil, err
	}
	defer release()
	err = s.playerJob(view.TaskAccount, service, func() error { return s.accountAction(service, action) })
	if err != nil {
		return nil, errors.New(view.Text(err.Error()))
	}
	return []string{view.T("account action completed")}, nil
}

// Player jobs retain only a service and a coarse outcome. They cannot be replayed
// from the task page; credentials and ephemeral selections never enter tasks.json.
func (s *API) playerJob(kind, service string, run func() error) error {
	done := make(chan error, 1)
	s.tasks.EnqueueSelf(app.Task{Kind: kind, Obj: service}, func(string) (result string, err error) {
		defer func() {
			if recover() != nil {
				err = errors.New("account action interrupted; try again")
			}
			done <- err
		}()
		err = run()
		if err != nil {
			return "", err
		}
		return "player action completed", nil
	})
	return <-done
}

func (s *API) accountAction(service, action string) error {
	a, err := s.detachedReader()
	if err != nil {
		return err
	}
	defer a.Close()
	if service == "steamcmd" {
		err = a.SteamAccountAction(action)
	} else if service == "loverslab" {
		unlock := accounts.Lock(a.AccountsDir())
		defer unlock()
		if action == "enable" {
			err = a.Store.SetMeta("account:loverslab:enabled", "true")
		} else if action == "forget" {
			s.ll = nil
			for _, p := range s.llFiles {
				_ = os.Remove(p.path)
			}
			s.llFiles = nil
			err = accounts.Forget(a.AccountsDir())
			if err == nil {
				err = a.Store.SetMeta("account:loverslab:enabled", "")
			}
		} else if action == "login" {
			if err = a.Store.SetMeta("account:loverslab:enabled", "true"); err == nil {
				err = a.Store.SetMeta("account:loverslab:consent", "true")
			}
			if err == nil {
				s.ll = nil
				err = accounts.Login(a.AccountsDir(), view.T("LoversLab — close this window after login to save the session"))
				if err == nil {
					ll, e := s.loversLab(a)
					if e != nil {
						err = e
					} else {
						ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						err = ll.Verify(ctx)
						cancel()
					}
				}
			}
		} else if a.Store.Meta("account:loverslab:enabled") != "true" {
			err = accounts.ErrSession
		} else if action == "verify" {
			var ll *subscribe.LoversLab
			ll, err = s.loversLab(a)
			if err == nil {
				err = ll.Verify(context.Background())
			}
		} else {
			err = errors.New("unknown account action")

		}
	} else {
		err = errors.New("unknown account service")
	}
	return err
}

func (s *API) loversLab(a *app.App) (*subscribe.LoversLab, error) {
	if a.Store.Meta("account:loverslab:enabled") != "true" {
		return nil, accounts.ErrSession
	}
	if s.ll == nil {
		session, err := accounts.Open(a.AccountsDir())
		if err != nil {
			return nil, err
		}
		s.ll = subscribe.NewLoversLab(session, nil)
	}
	return s.ll, nil
}

// LoverFiles and LoverFollow accept only canonical file pages and opaque selections.
// No method accepts credentials, arbitrary headers, cookies or an HTTP method/body.
func (s *API) LoverFiles(page string) (*subscribe.FilePage, error) {
	release, err := s.beginAccount()
	if err != nil {
		return nil, err
	}
	defer release()
	a, err := s.detachedReader()
	if err != nil {
		return nil, err
	}
	defer a.Close()
	unlock := accounts.Lock(a.AccountsDir())
	defer unlock()
	ll, err := s.loversLab(a)
	if err != nil {
		return &subscribe.FilePage{Reason: view.Text(err.Error())}, nil
	}
	p, err := ll.Files(context.Background(), page)
	if err != nil {
		return &subscribe.FilePage{Reason: view.Text(err.Error())}, nil
	}
	return p, nil
}

type LoverResult struct {
	Page  *subscribe.FilePage `json:"page,omitempty"`
	Ready string              `json:"ready,omitempty"`
}

type loverDownload struct{ page, path string }

func (s *API) LoverFollow(page, id string, agreed bool) (*LoverResult, error) {
	release, err := s.beginAccount()
	if err != nil {
		return nil, err
	}
	defer release()
	a, err := s.detachedReader()
	if err != nil {
		return nil, err
	}
	defer a.Close()
	unlock := accounts.Lock(a.AccountsDir())
	defer unlock()
	ll, err := s.loversLab(a)
	if err != nil {
		return &LoverResult{Page: &subscribe.FilePage{Reason: view.Text(err.Error())}}, nil
	}
	var p *subscribe.FilePage
	var path string
	err = s.playerJob(view.TaskSiteDownload, "LoversLab", func() error {
		var e error
		p, path, e = ll.Follow(context.Background(), page, id, agreed, filepath.Join(a.AccountsDir(), "downloads"))
		return e
	})
	if err != nil {
		return &LoverResult{Page: &subscribe.FilePage{Reason: view.Text(err.Error())}}, nil
	}
	if path != "" {
		if s.llFiles == nil {
			s.llFiles = map[string]loverDownload{}
		}
		if old := s.llFiles[id]; old.path != "" {
			_ = os.Remove(old.path)
		}
		s.llFiles[id] = loverDownload{page: page, path: path}
		return &LoverResult{Ready: id}, nil
	}
	return &LoverResult{Page: p}, nil
}

func (s *API) LoverInstall(page, id, ref string) (string, error) {
	release, err := s.beginAccount()
	if err != nil {
		return "", err
	}
	defer release()
	a, err := s.detachedReader()
	if err != nil {
		return "", err
	}
	defer a.Close()
	unlock := accounts.Lock(a.AccountsDir())
	defer unlock()
	file := s.llFiles[id]
	path := file.path
	if path == "" || file.page != page {
		return "", errors.New(view.T("download selection expired; read the page again"))
	}
	delete(s.llFiles, id)
	defer os.Remove(path)
	if err = s.playerJob(view.TaskSiteDownload, "LoversLab", func() error { _, e := a.InstallLoversLabFile(page, path, ref, false); return e }); err != nil {
		return "", err
	}
	s.mu.Lock()
	s.board, s.needs, s.lib = nil, nil, nil
	s.mu.Unlock()
	return view.T("subscription installed"), nil
}

func (s *API) LoverInstallPreview(page, id, ref string) (*view.Preview, error) {
	release, err := s.beginAccount()
	if err != nil {
		return nil, err
	}
	defer release()
	a, err := s.detachedReader()
	if err != nil {
		return nil, err
	}
	defer a.Close()
	file := s.llFiles[id]
	path := file.path
	if path == "" || file.page != page {
		return nil, errors.New(view.T("download selection expired; read the page again"))
	}
	op, err := a.InstallLoversLabFile(page, path, ref, true)
	if err != nil {
		return nil, err
	}
	p := &view.Preview{Title: view.T("install subscription"), Apply: true, Lines: []string{view.T("Install the download in the subscriptions folder, replacing this subscription when updating.")}}
	for _, it := range op.Items {
		p.Lines = append(p.Lines, it.Folder)
	}
	return p, nil
}

func (s *API) SubscriptionFilePage(ref string) (string, error) {
	a, err := s.detachedReader()
	if err != nil {
		return "", err
	}
	defer a.Close()
	ss, err := a.Subscriptions()
	if err != nil {
		return "", err
	}
	for _, sub := range ss {
		if sub.Ref == ref && sub.Record.Kind == "page" && subscribe.LoversLabPage(sub.Record.URL) {
			return sub.Record.URL, nil
		}
	}
	return "", nil
}

// AccountStatus is the only state intended for automation. It reads no secret files.
func (s *API) AccountStatus() map[string]string {
	return map[string]string{"steamcmd": "player login required", "loverslab": "player login required"}
}

// SteamLogin keeps the account input ephemeral: task records contain only SteamCMD.
func (s *API) SteamLogin(name string) ([]string, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.EqualFold(name, "anonymous") {
		return nil, errors.New(view.T("enter a Steam account name first"))
	}
	release, err := s.beginAccount()
	if err != nil {
		return nil, err
	}
	defer release()
	err = s.playerJob(view.TaskAccount, "steamcmd", func() error {
		a, e := s.detachedReader()
		if e != nil {
			return e
		}
		defer a.Close()
		if _, e = a.SteamCMDAccount(name); e != nil {
			return e
		}
		if e = a.Store.SetMeta("cfg:steamcmd.account", name); e != nil {
			return e
		}
		return a.SteamAccountAction("login")
	})
	if err != nil {
		return nil, errors.New(view.Humanize(nil, err.Error()))
	}
	return []string{view.T("account action completed")}, nil
}

// At most one request per four hours, including failures. Opening Settings is
// the trigger; there is no timer, retry loop, resource load or task/toast.
func (s *API) refreshLoversLab(a *app.App) {
	release, err := s.beginAccount()
	if err != nil {
		return
	}
	defer release()
	if a.Store.Meta("account:loverslab:enabled") != "true" {
		return
	}
	unlock := accounts.Lock(a.AccountsDir())
	defer unlock()
	last, _ := time.Parse(time.RFC3339Nano, a.Store.Meta("account:loverslab:auto-check"))
	session, err := accounts.Open(a.AccountsDir())
	if err != nil || !session.HasCookies() {
		return
	}
	if session.Status().Verified.After(last) {
		last = session.Status().Verified
	}
	if a.Now().Sub(last) < 4*time.Hour {
		return
	}
	if a.Store.SetMeta("account:loverslab:auto-check", a.Now().UTC().Format(time.RFC3339Nano)) != nil {
		return
	}
	ll, err := s.loversLab(a)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = ll.Verify(ctx)
}
