package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/accounts"
	"github.com/ToumanLin/RimArcho/internal/deps"
)

func (a *App) AccountsDir() string { return filepath.Join(a.LocalDir(), "accounts") }

type SteamAccountStatus struct {
	Account  string    `json:"account"`
	Path     string    `json:"path"`
	Source   string    `json:"source"`
	Managed  bool      `json:"managed"`
	State    string    `json:"state"`
	Verified time.Time `json:"verified"`
}

func steamStateKey(exe, account string) string {
	h := sha256.Sum256([]byte(strings.ToLower(exe) + "\x00" + account))
	return "account:steamcmd:" + hex.EncodeToString(h[:])
}
func (a *App) steamRecordKey(exe, account string) string {
	return steamStateKey(exe, account) + ":" + a.Store.Meta(steamStateKey(exe, "cache-generation"))
}
func (a *App) SteamAccountStatus() SteamAccountStatus {
	i := a.Deps().Status(deps.SteamCMD)
	account, _ := a.SteamCMDAccount("")
	s := SteamAccountStatus{Account: account, Path: i.Path, Source: i.Source, Managed: i.Own, State: "已配置账户，尚未验证"}
	var saved SteamAccountStatus
	hasRecord := json.Unmarshal([]byte(a.Store.Meta(a.steamRecordKey(i.Path, account))), &saved) == nil
	if hasRecord {
		s.State, s.Verified = saved.State, saved.Verified
	}
	if account == "anonymous" {
		s.State = "匿名"
	} else if !hasRecord && i.Own {
		if _, err := os.Stat(filepath.Join(i.Dir, "config", "config.vdf")); err == nil {
			s.State = "检测到可能的缓存，尚未验证"
		}
	}
	if i.Path == "" {
		s.State = "未安装 SteamCMD"
	}
	return s
}
func (a *App) steamAccountRecord(exe, account, state string, success bool) {
	var s SteamAccountStatus
	key := a.steamRecordKey(exe, account)
	_ = json.Unmarshal([]byte(a.Store.Meta(key)), &s)
	s.State = state
	if success {
		s.Verified = a.Now().UTC()
	}
	b, _ := json.Marshal(s)
	_ = a.Store.SetMeta(key, string(b))
}

// SteamAccountAction is player-only; it must run outside the GUI mutex.
func (a *App) SteamAccountAction(action string) error {
	i := a.Deps().Status(deps.SteamCMD)
	if action == "anonymous" {
		return a.Store.SetMeta("cfg:steamcmd.account", "")
	}
	if i.Path == "" {
		return ErrNoSteamCMD
	}
	un := accounts.Lock(i.Path)
	defer un()
	account, err := a.SteamCMDAccount("")
	if err != nil {
		return err
	}
	if !i.Own {
		return errors.New("外部缓存，不接管；请在外部 SteamCMD 中管理登录")
	}
	if action == "local-forget" {
		if err := a.forgetManagedSteam(); err != nil {
			return err
		}
		a.steamAccountRecord(i.Path, account, "需重新登录", false)
		return a.Store.SetMeta("cfg:steamcmd.account", "")
	}
	args := []string{"+login", account}
	switch action {
	case "login":
		if account == "anonymous" {
			return errors.New("先填写 Steam 账户名")
		}
	case "verify":
		args = append(args, "+quit")
	case "logout":
		args = []string{"+logout", "+quit"}
	default:
		return errors.New("未知账户操作")
	}
	log := filepath.Join(filepath.Dir(i.Path), "logs", "console_log.txt")
	var off int64
	if st, e := os.Stat(log); e == nil {
		off = st.Size()
	}
	a.steamAccountRecord(i.Path, account, "等待玩家在 SteamCMD 窗口完成操作", false)
	p, err := a.steamCMD().start(i.Path, args, true)
	if err != nil {
		a.steamAccountRecord(i.Path, account, "SteamCMD 窗口未打开", false)
		return errors.New("SteamCMD 窗口未打开，请重试")
	}
	err = p.Wait()
	tail := &logTail{path: log, off: off}
	lines := tail.lines()
	if tail.partial != "" {
		lines = append(lines, tail.partial)
	}
	login := steamLogin{account: account}
	logout := false
	for _, line := range lines {
		low := strings.ToLower(line)
		login.read(line)
		if strings.Contains(low, "logged out") || strings.Contains(low, "logging off") {
			logout = true
		}
	}
	if action == "logout" {
		if err != nil || !logout {
			a.steamAccountRecord(i.Path, account, "退出未确认，可选择本地忘记", false)
			return errors.New("SteamCMD 未确认退出；可使用“本地忘记受管缓存”，清理此实例所有缓存账户")
		}
		a.steamAccountRecord(i.Path, account, "需重新登录", false)
		return a.Store.SetMeta("cfg:steamcmd.account", "")
	}
	if login.ready && login.err == nil && err == nil {
		a.steamAccountRecord(i.Path, account, "最近登录成功，需时再验证", true)
		return nil
	}
	state := "需重新登录"
	if login.err != nil && login.err.Kind == "network" {
		state = "网络错误，登录状态未知"
	}
	a.steamAccountRecord(i.Path, account, state, false)
	return errors.New(state + "；请在 SteamCMD 窗口自行完成密码与 Guard 验证")
}

// Never select config/PATH instances, read token contents or leave auth backups.
func (a *App) forgetManagedSteam() (result error) {
	dir := filepath.Join(a.LocalDir(), "steamcmd")
	defer func() {
		if result == nil {
			result = a.Store.SetMeta(steamStateKey(filepath.Join(dir, "steamcmd.exe"), "cache-generation"), time.Now().UTC().Format(time.RFC3339Nano))
		}
	}()
	if _, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil || !samePath(resolved, dir) {
		return errors.New("受管 SteamCMD 路径已重定向，未清理；请恢复路径后重试")
	}
	var paths []string
	for _, pattern := range []string{"config/config.vdf", "ssfn*", "*.sentry", "config/*.sentry", "config/ssfn*"} {
		ms, e := filepath.Glob(filepath.Join(dir, filepath.FromSlash(pattern)))
		if e != nil {
			return errors.New("认证缓存清理失败")
		}
		paths = append(paths, ms...)
	}
	for _, p := range paths {
		r, e := filepath.EvalSymlinks(p)
		if e != nil || !samePath(r, p) {
			return errors.New("认证缓存路径已重定向，未清理")
		}
		fi, e := os.Lstat(p)
		if e != nil || fi.IsDir() {
			return errors.New("认证缓存布局未知，请在 SteamCMD 中退出后重试")
		}
	}
	for _, p := range paths {
		if os.Remove(p) != nil {
			return errors.New("认证缓存清理失败，请关闭 SteamCMD 后重试")
		}
	}
	return nil
}
func (a *App) ForgetAccounts() error {
	un := accounts.Lock(filepath.Join(a.LocalDir(), "steamcmd", "steamcmd.exe"))
	defer un()
	unLL := accounts.Lock(a.AccountsDir())
	defer unLL()
	if err := accounts.Forget(a.AccountsDir()); err != nil {
		return err
	}
	if err := a.forgetManagedSteam(); err != nil {
		return err
	}
	return a.Store.SetMeta("cfg:steamcmd.account", "")
}
