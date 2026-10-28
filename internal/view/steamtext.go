package view

import "github.com/ToumanLin/RimArcho/internal/app"

// The Steam client signed out by SteamCMD, and offline (VM run B11).
func init() {
	Register(map[string]string{
		"the Steam client is offline": "Steam 客户端离线",
		"the Steam client is offline: restart Steam, then retry (SteamCMD signing in with the same account signs Steam out)": "Steam 客户端离线：重启 Steam 后重试（SteamCMD 用同一个账号登录会把 Steam 挤下线）",
		app.SteamSignOutNote:            "SteamCMD 会用 %s 登录，也就是 Steam 客户端正在用的账号：Steam 会被挤下线，重启后才回来",
		"note: " + app.SteamSignOutNote: "注意：SteamCMD 会用 %s 登录，也就是 Steam 客户端正在用的账号：Steam 会被挤下线，重启后才回来",
	})
	interfaceFormats = append(interfaceFormats, interfaceFormatFor(app.SteamSignOutNote), interfaceFormatFor("note: "+app.SteamSignOutNote))
}
