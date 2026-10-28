package view

func init() {
	Register(map[string]string{
		"automatic checks":   "自动检查",
		"check interval":     "检查间隔",
		"standard intervals": "常规",
		"half the interval":  "缩短一半",
		"twice the interval": "延长一倍",
		"LoversLab session":  "LoversLab 登录状态",
		"While the window is open: mods and data every %d hours, tools every %d hours, LoversLab every %d hours. Mod and tool updates wait for your click.": "窗口打开时，每 %d 小时检查 Mod 和社区数据，每 %d 小时检查工具，每 %d 小时检查 LoversLab 登录状态。Mod 和工具由你决定何时更新。",
		"last checked %s":                         "上次检查 %s",
		"%d pending":                              "%d 项待处理",
		"%d mod updates available":                "%d 个 Mod 有更新",
		"%d tool updates available; see Settings": "%d 个工具有更新，可在设置中查看",
		"LoversLab needs you to log in again; see Settings → accounts":       "LoversLab 登录需要你确认，请前往设置 › 账户",
		"new version available: %s":                                          "有新版本：%s",
		"RimArcho supports todds %s; the new version has not been verified.": "RimArcho 支持 todds %s；新版本尚未验证。",
		"check unavailable offline":                                          "暂时无法检查，联网后再试",
		"stored data kept":                                                   "更新未完成，保留原有数据",
		"community data updated":                                             "社区数据已更新",
		"check interrupted":                                                  "检查已中断",
		"session expires soon":                                               "登录即将到期，请重新登录",
		"session needs confirmation; log in again":                           "登录需要确认，请重新登录",
		"installed version unknown":                                          "无法确定已装版本",
		"checking":                                                           "正在检查",
		"tool is in use; close it before updating or removing it":            "工具正在使用，请关闭后再更新或移除",
		"cannot check whether the tool is in use":                            "无法确定工具是否正在使用，请稍后再试",
	})
}
