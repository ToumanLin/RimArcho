package view

import (
	"errors"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// The text of environments and the shared mod folders (02 §0): the environment page, the
// settings page's Mod 文件夹, and the add-environment guide's first step.
func init() {
	Register(map[string]string{
		"unknown":                              "未知",
		"game":                                 "游戏",
		"game version":                         "游戏版本",
		"saves":                                "存档",
		"shared (the game's default folder)":   "共用默认",
		"its own":                              "独立",
		"a folder you chose":                   "自选目录",
		"the game's own exe":                   "直接启动游戏",
		"mod folders":                          "Mod 文件夹",
		"mod folder":                           "Mod 文件夹",
		"SteamCMD folder":                      "SteamCMD 文件夹",
		"Archo Monitor folder":                 "Archo Monitor 文件夹",
		"lock library":                         "锁定库",
		"lock library (%s, older)":             "锁定库（%s，旧）",
		"not there now":                        "现在不在",
		"moving to %s waits for your answer":   "移到 %s，等你确认",
		"remove":                               "移除",
		"use it":                               "使用",
		"Steam has one at %s":                  "Steam 里有一个：%s",
		"none found in the Steam libraries":    "Steam 库里没有找到",
		"choose…":                              "选择…",
		"choose a folder":                      "选择文件夹",
		"no folder dialog here; type the path": "这里不能打开文件夹对话框，请输入路径",
		// the guide's first step
		"a game you have":    "已有的游戏",
		"download a version": "下载一个版本",
		"the RimWorld installs found on this computer; tick the ones to add": "这台电脑上找到的 RimWorld；勾选要加入的",
		"or a game folder":             "或者一个游戏目录",
		"reading the game's own mods…": "正在读取游戏自己的 Mod…",
		"%d mods in its Data and Mods": "Data 和 Mods 里有 %d 个 Mod",
		// the guide's "download a version" (08 §11)
		"current version":         "当前正式版",
		"test version (unstable)": "测试版（unstable）",
		"asking Steam which versions RimWorld has… (SteamCMD may update itself first)": "正在问 Steam 有哪些版本…（SteamCMD 可能先更新自己）",
		"size unknown": "大小未知",
		"RimArcho downloads game versions with Valve's SteamCMD, which is not on this computer. Download it from Valve (%s) into %s?": "RimArcho 用 Valve 的 SteamCMD 下载游戏版本，这台电脑上还没有。从 Valve 下载（%s）到 %s？",
		"download SteamCMD":                     "下载 SteamCMD",
		"ask again":                             "再问一次",
		"the folder for the game":               "游戏放在哪个目录",
		"SteamCMD account (optional)":           "SteamCMD 账户（可选）",
		"SteamCMD program":                      "SteamCMD 程序",
		"empty: RimArcho's own copy, then PATH": "留空：先用 RimArcho 自己下载的，再找 PATH",
		"empty or anonymous: anonymous login; enter an account name to log in. Password and Steam Guard are entered only in SteamCMD": "留空或填 anonymous：匿名登录；填入账户名可登录。密码和 Steam 令牌只在 SteamCMD 中输入",
		"empty: %s (from settings)": "留空：%s（来自设置）",
		"Anonymous by default; you can choose an account in settings or here. Enter anonymous to override a saved account. Password and Steam Guard are entered only in SteamCMD.": "默认匿名；可在设置或这里选择账户。填 anonymous 可覆盖已保存的账户。密码和 Steam 令牌只在 SteamCMD 中输入。",
		"anonymous accounts cannot download RimWorld; log in with an account that owns it, or add a game folder you already have":                                                  "匿名账号不能下载 RimWorld；请登录拥有游戏的账号，或添加已有的游戏目录",
		"the anonymous Steam connection did not finish; check the connection and try again":                                                                                        "匿名连接 Steam 未完成；请检查网络后重试",
		"download and add":                                           "下载并加入",
		"a download is already running":                              "已经有一个下载在进行",
		"starting SteamCMD…":                                         "正在启动 SteamCMD…",
		"logging in (in the SteamCMD window)…":                       "正在登录（在 SteamCMD 窗口里）…",
		"downloading":                                                "正在下载",
		"checking the files":                                         "正在校验文件",
		"downloaded; game %s added":                                  "下载完成，已加入游戏 %s",
		"the download did not finish":                                "下载没有完成",
		"%s of %s":                                                   "%s / %s",
		"try again":                                                  "重试",
		"this Steam account does not own RimWorld":                   "这个 Steam 账户没有 RimWorld",
		"Steam did not accept the password":                          "Steam 没有接受这个密码",
		"Steam did not accept the Steam Guard code":                  "Steam 没有接受这个 Steam 令牌",
		"the disk is full (or the folder cannot be written)":         "磁盘满了（或者这个目录写不进去）",
		"Steam does not have this branch (any more)":                 "Steam 上没有（或不再有）这个分支",
		"Steam could not be reached":                                 "连不上 Steam",
		"the login did not finish (the SteamCMD window was closed?)": "登录没有完成（SteamCMD 窗口被关掉了？）",
		"SteamCMD did not finish the download":                       "SteamCMD 没有完成下载",
		"SteamCMD: %s":                                               "SteamCMD：%s",
	})
}

// DownloadErrorText is a failed download in the player's words (08 §11): why, then
// SteamCMD's own line.
func DownloadErrorText(err error) string {
	var de *app.DownloadError
	if !errors.As(err, &de) {
		return Humanize(nil, err.Error())
	}
	s := T(de.Why())
	if de.Line != "" {
		s += "  " + Tf("SteamCMD: %s", de.Line)
	}
	return s
}
