package view

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/accounts"
	"github.com/ToumanLin/RimArcho/internal/app"
)

func AccountGroup(a *app.App) SettingGroup {
	s := a.SteamAccountStatus()
	name := s.Account
	if name == "anonymous" {
		name = "匿名"
	}
	action := func(service, verb, label, confirm string) SettingAction {
		return SettingAction{Label: label, Argv: []string{"account", service, verb}, Confirm: confirm}
	}
	account := s.Account
	if account == "anonymous" {
		account = ""
	}
	source := map[string]string{"own": "RimArcho 自带", "config": "设置", "path": "PATH", "program-files": "已安装程序"}[s.Source]
	if source == "" {
		source = s.Source
	}
	if source == "" {
		source = "未找到"
	}
	verified := "从未"
	if !s.Verified.IsZero() {
		verified = a.FormatTime(s.Verified)
	}
	note := []string{s.State, "来源：" + source, "最后验证：" + verified, "到期：由 Steam 管理"}
	if s.Path != "" {
		note = append(note, s.Path)
	}
	if s.State == "匿名" {
		name = ""
	}
	steam := SettingItem{Label: "SteamCMD", Value: name, Edit: "text", EditValue: &account, Argv: []string{"config", "set", "steamcmd.account", "{}"},
		Note: strings.Join(note, "；"), NotePaths: []string{s.Path},
		Actions: []SettingAction{action("steamcmd", "anonymous", "切换匿名", "")}}

	if s.Managed {
		steam.Actions = append(steam.Actions,
			action("steamcmd", "login", "登录 / 重新登录", "打开 SteamCMD 自己的控制台。只传账户名；密码和 Guard 由你在窗口输入，完成后输入 quit 关闭。SteamCMD 会自行更新受管认证缓存。"),
			action("steamcmd", "verify", "验证", "连接 Steam 验证所选账户，可能刷新认证缓存；需要密码或 Guard 时在它的窗口输入。"),
			action("steamcmd", "logout", "退出并忘记", "让受管 SteamCMD 执行 logout 后退出。影响此实例的缓存账户；不可恢复。"),
			action("steamcmd", "local-forget", "本地忘记受管缓存", "只删除 RimArcho 受管副本的已知认证缓存，影响其中所有缓存账户。用于退出失败或离线时；不可恢复，不撤销其他设备。请先关闭其他 SteamCMD 窗口。"))
	} else if s.Path != "" {
		steam.Note += "；外部缓存，不接管"
	}
	ll := SettingItem{Label: "LoversLab", Value: "未连接；默认关闭", Note: "站点含成人内容并有自己的访问条件和规则；请使用你自己的账户。"}
	if a.Store.Meta("account:loverslab:enabled") == "true" {
		st := accounts.Status{State: "需登录"}
		if session, err := accounts.Open(a.AccountsDir()); err == nil {
			st = session.Status()
		}
		display := st.Name
		if display == "" {
			display = "未知"
		}
		ll.Value = st.State + "；" + display
		ll.Note = "最后验证：" + a.FormatTime(st.Verified) + "；Cookie 到期：" + a.FormatTime(st.Expiry) + "（无日期表示会话级 / 未知；服务器可能提前使其失效）"
		ll.Actions = []SettingAction{action("loverslab", "login", "打开站点登录", "打开独立站点窗口。会话 Cookie 将以当前 Windows 用户的 DPAPI 加密保存到本机账户目录；关闭窗口时清理临时浏览器数据。密码只在原站输入。"), action("loverslab", "verify", "验证", "向登录后页面发起一次 GET，可能刷新会话。"), action("loverslab", "forget", "退出并忘记", "停用会话并删除本机加密 Cookie 和独立浏览器数据，不可恢复。不退出其他设备。")}
	} else {
		ll.Actions = []SettingAction{action("loverslab", "enable", "启用", "LoversLab 含成人内容，有自己的访问条件与规则；请使用你自己的账户。启用后可在独立窗口登录，并将会话 Cookie 加密保存到 RimArcho 本机账户目录。请遵守站点规则与文件许可。")}
	}
	return SettingGroup{Title: "账户", Items: []SettingItem{steam, ll}}
}
