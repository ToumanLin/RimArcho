package gui

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/ToumanLin/RimArcho/internal/accounts"
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// AccountAction is deliberately absent from cmds, MCP and agent tool registries.
func (s *API) AccountAction(service, action string) ([]string, error) {
	allowed := map[string]map[string]bool{"steamcmd": {"login": true, "verify": true, "logout": true, "local-forget": true, "anonymous": true}, "loverslab": {"enable": true, "login": true, "verify": true, "forget": true}}
	if !allowed[service][action] {
		return nil, errors.New("未知账户操作")
	}
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	err := s.playerJob(view.TaskAccount, service, func() error { return s.accountAction(service, action) })
	if err != nil {
		return nil, err
	}
	return []string{"账户操作已完成"}, nil
}

// Player jobs retain only a service and a coarse outcome. They cannot be replayed
// from the task page; credentials and ephemeral selections never enter tasks.json.
func (s *API) playerJob(kind, service string, run func() error) error {
	done := make(chan error, 1)
	s.tasks.EnqueueSelf(app.Task{Kind: kind, Obj: service}, func(string) (result string, err error) {
		defer func() {
			if recover() != nil {
				err = errors.New("账户操作中断，请重新发起")
			}
			done <- err
		}()
		err = run()
		if err != nil {
			return "", err
		}
		return "玩家操作已完成", nil
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
		} else if a.Store.Meta("account:loverslab:enabled") != "true" {
			err = accounts.ErrSession
		} else {
			switch action {
			case "login":
				s.ll = nil
				err = accounts.Login(a.AccountsDir())
			case "verify":
				var ll *subscribe.LoversLab
				ll, err = s.loversLab(a)
				if err == nil {
					err = ll.Verify(context.Background())
				}
			default:
				err = errors.New("未知账户操作")
			}
		}
	} else {
		err = errors.New("未知账户服务")
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
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	a, err := s.detachedReader()
	if err != nil {
		return nil, err
	}
	defer a.Close()
	unlock := accounts.Lock(a.AccountsDir())
	defer unlock()
	ll, err := s.loversLab(a)
	if err != nil {
		return &subscribe.FilePage{Reason: err.Error()}, nil
	}
	p, err := ll.Files(context.Background(), page)
	if err != nil {
		return &subscribe.FilePage{Reason: err.Error()}, nil
	}
	return p, nil
}

type LoverResult struct {
	Page  *subscribe.FilePage `json:"page,omitempty"`
	Ready string              `json:"ready,omitempty"`
}

type loverDownload struct{ page, path string }

func (s *API) LoverFollow(page, id string, agreed bool) (*LoverResult, error) {
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	a, err := s.detachedReader()
	if err != nil {
		return nil, err
	}
	defer a.Close()
	unlock := accounts.Lock(a.AccountsDir())
	defer unlock()
	ll, err := s.loversLab(a)
	if err != nil {
		return &LoverResult{Page: &subscribe.FilePage{Reason: err.Error()}}, nil
	}
	var p *subscribe.FilePage
	var path string
	err = s.playerJob(view.TaskSiteDownload, "LoversLab", func() error {
		var e error
		p, path, e = ll.Follow(context.Background(), page, id, agreed, filepath.Join(a.AccountsDir(), "downloads"))
		return e
	})
	if err != nil {
		return &LoverResult{Page: &subscribe.FilePage{Reason: err.Error()}}, nil
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
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
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
		return "", errors.New("下载选择已失效，请重新读取")
	}
	delete(s.llFiles, id)
	defer os.Remove(path)
	if err = s.playerJob(view.TaskSiteDownload, "LoversLab", func() error { _, e := a.InstallLoversLabFile(page, path, ref, false); return e }); err != nil {
		return "", err
	}
	s.mu.Lock()
	s.board, s.needs, s.lib = nil, nil, nil
	s.mu.Unlock()
	return "订阅已安装", nil
}

func (s *API) LoverInstallPreview(page, id, ref string) (*view.Preview, error) {
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	a, err := s.detachedReader()
	if err != nil {
		return nil, err
	}
	defer a.Close()
	file := s.llFiles[id]
	path := file.path
	if path == "" || file.page != page {
		return nil, errors.New("下载选择已失效，请重新读取")
	}
	op, err := a.InstallLoversLabFile(page, path, ref, true)
	if err != nil {
		return nil, err
	}
	p := &view.Preview{Title: "安装订阅", Apply: true, Lines: []string{"将下载文件安装到订阅目录；更新时替换此订阅的旧文件。"}}
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
	return map[string]string{"steamcmd": "需玩家登录", "loverslab": "需玩家登录"}
}
