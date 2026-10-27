package gui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// CreditPrompt is safe to poll at game exit or window focus. null means defer;
// repeated reads return the same prompt until CreditAnswer commits the answer.
func (s *API) CreditPrompt() (*app.CreditPromptData, error) {
	s.mu.Lock()
	a, err := s.a.ReadSession()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	defer a.Close()
	return a.CreditPrompt()
}

type CreditAnswerResult struct {
	BatchIDs []int64 `json:"batchIDs"`
	State    string  `json:"state"`
	TaskID   string  `json:"taskID"`
}

func (s *API) CreditAnswer(batchIDs []int64, replace bool) (*CreditAnswerResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.a.CreditAnswer(batchIDs, replace); err != nil {
		return nil, err
	}
	r := &CreditAnswerResult{BatchIDs: batchIDs, State: "dismissed", TaskID: ""}
	if replace {
		r.State = "replacing"
		id, err := s.startCreditLocked()
		if err != nil {
			return nil, err
		}
		r.TaskID = id
		if id == "" {
			r.State = "done"
		}
	}
	return r, nil
}

// The SQLite batch/item checkpoint owns recovery; tasks.json is only the display.
// There is one runner per window. A stop preserves obligations and subscriptions;
// resume from tasks (or the next application start) creates a new display task.
func (s *API) startCreditLocked() (string, error) {
	if s.creditClosed {
		return "", fmt.Errorf("window is closing")
	}
	if s.creditID != "" {
		return s.creditID, nil
	}
	items, err := s.a.CreditPending()
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		return "", nil
	}
	a, err := s.a.ReadSession()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithCancel(context.Background())
	id := s.tasks.Begin(view.TaskCreditReplace, view.T("SteamCMD mods"), func() error { cancel(); return nil })
	s.creditID, s.creditCancel = id, cancel
	s.creditWG.Add(1)
	go s.runCredit(ctx, a, id)
	return id, nil
}

func (s *API) runCredit(ctx context.Context, a *app.App, id string) {
	defer s.creditWG.Done()
	defer a.Close()
	defer func() {
		s.mu.Lock()
		if s.creditID == id {
			s.creditID = ""
			s.creditCancel = nil
		}
		s.mu.Unlock()
	}()
	batches := map[int64]bool{}
	finish := func(state, text string) { s.tasks.Finish(id, state, text) }
	for {
		if ctx.Err() != nil || s.tasks.StopAsked(id) {
			finish(app.TaskStopped, view.T("replacement paused; Steam subscriptions kept"))
			return
		}
		pending, err := a.CreditPending()
		if err != nil {
			finish(app.TaskFailed, err.Error())
			return
		}
		for _, it := range pending {
			batches[it.BatchID] = true
		}
		if _, _, _, err := s.creditProgress(a, id, batches, nil); err != nil {
			finish(app.TaskFailed, err.Error())
			return
		}
		if err := a.CreditStep(func(it store.CreditItem) (string, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			if s.busy["scan"] {
				return "", fmt.Errorf("waiting for scan")
			}
			for _, t := range s.tasks.List() {
				if t.Lane == app.LaneFiles && t.State == app.TaskRunning {
					return "", fmt.Errorf("waiting for file task")
				}
			}
			result, err := a.ReplaceCreditItem(it)
			s.board, s.needs, s.lib = nil, nil, nil
			return result, err
		}, app.CreditCallbacks{
			Stopped:  func() bool { return ctx.Err() != nil || s.tasks.StopAsked(id) },
			Progress: func(it store.CreditItem) { s.creditProgress(a, id, batches, &it) },
		}); err != nil {
			finish(app.TaskFailed, err.Error())
			return
		}
		rows, done, failed, err := s.creditProgress(a, id, batches, nil)
		if err != nil {
			finish(app.TaskFailed, err.Error())
			return
		}

		if done == len(rows) {
			// An answer can add another batch while a Steam worker is running. Finish
			// atomically with clearing the runner so that answer cannot lose its wakeup.
			s.mu.Lock()
			latest, err := a.CreditPending()
			if err != nil {
				s.mu.Unlock()
				finish(app.TaskFailed, err.Error())
				return
			}
			more := false
			for _, it := range latest {
				if !it.Terminal() {
					more = true
					break
				}
			}
			if more {
				s.mu.Unlock()
				continue
			}
			s.creditID = ""
			s.creditCancel = nil
			s.mu.Unlock()
			state := app.TaskDone
			if failed > 0 {
				state = app.TaskPartial
				if failed == done {
					state = app.TaskFailed
				}
			}
			finish(state, view.T("replacement finished; see item results"))
			return
		}
		timer := time.NewTimer(15 * time.Second)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
		}
	}
}

func init() {
	view.Register(map[string]string{
		"replace SteamCMD mods with Steam subscriptions": "用 Steam 订阅替换 SteamCMD Mod",
		"replacing %s":                   "正在替换 %s",
		"SteamCMD mods":                  "SteamCMD Mod",
		"SteamCMD copy retained":         "SteamCMD 副本已保留",
		"retained: ":                     "已保留副本：",
		"waiting to retry: ":             "等待重试：",
		"waiting for Steam installation": "等待 Steam 完成安装",
		"SteamCMD copy already removed":  "SteamCMD 副本已移走",
		"switched to Workshop; SteamCMD copy moved to recycle bin":                          "已切换到工坊里的这一份，SteamCMD 副本已移入回收站",
		"recorded SteamCMD instance is unavailable or has changed":                          "原来这份 SteamCMD Mod 已不可用或发生变化",
		"SteamCMD copy is no longer live":                                                   "SteamCMD 副本已不在磁盘上",
		"Workshop instance identity needs review":                                           "工坊里这份 Mod 的身份需要检查",
		"lists, staging, undo history, versions or packs still reference the SteamCMD copy": "列表、暂存、撤销记录、历史版本或整合包仍引用 SteamCMD 副本",
		"links still reference the SteamCMD copy":                                           "链接仍指向 SteamCMD 副本",
		"replacement paused; Steam subscriptions kept":                                      "替换已暂停，已完成的 Steam 订阅保留",
		"waiting for Steam or a safe time to replace":                                       "等待 Steam 安装完成或游戏退出后替换",
		"replacement finished; see item results":                                            "替换已处理完，请查看逐项结果",
	})
}

func creditResultText(result string) string {
	if strings.HasPrefix(result, "failed:") {
		parts := strings.SplitN(result, "; ", 3)
		code, err := strconv.Atoi(strings.TrimPrefix(parts[0], "failed:"))
		if err == nil {
			parts[0] = view.T(app.WorkshopResultText(code)) + " (" + parts[0] + ")"
		}
		if len(parts) > 1 {
			parts[1] = view.T("SteamCMD copy retained")
		}
		return strings.Join(parts, "; ")
	}
	for _, prefix := range []string{"retained: ", "waiting to retry: "} {
		if strings.HasPrefix(result, prefix) {
			return view.T(prefix) + view.T(strings.TrimPrefix(result, prefix))
		}
	}
	return view.T(result)
}

// Publish after every item checkpoint, including subscription confirmation and
// retry scheduling, so a slow Steam batch does not hide its completed work.
func (s *API) creditProgress(a *app.App, id string, batches map[int64]bool, active *store.CreditItem) ([]app.TaskItem, int, int, error) {
	rows := []app.TaskItem{}
	done, failed := 0, 0
	for batchID := range batches {
		items, err := a.Store.CreditItems(batchID)
		if err != nil {
			return nil, 0, 0, err
		}
		for _, it := range items {
			state := app.ItemWait
			if it.Terminal() {
				done++
				state = app.ItemOK
			}
			if it.State == "failed" {
				failed++
				state = app.ItemFail
			}
			if active != nil && active.BatchID == it.BatchID && active.WorkshopID == it.WorkshopID && !it.Terminal() && it.NextAttempt <= a.Now().Unix() {
				state = app.ItemRun
			}
			rows = append(rows, app.TaskItem{Ref: it.InstanceID, Name: it.Name, State: state, Why: creditResultText(it.Result)})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Ref < rows[j].Ref })
	s.tasks.Update(id, func(t *app.Task) {
		t.Items, t.Done, t.Total = rows, done, len(rows)
		t.Phase = view.T("waiting for Steam or a safe time to replace")
		if active != nil && !active.Terminal() && active.NextAttempt <= a.Now().Unix() {
			t.Phase = view.Tf("replacing %s", active.Name)
		}
	})
	return rows, done, failed, nil
}
