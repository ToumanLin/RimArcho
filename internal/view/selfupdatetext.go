package view

import "strings"

func init() {
	Register(map[string]string{
		"RimArcho updates":                            "RimArcho 更新",
		"%s can be updated":                           "%s 可以更新",
		"a development build is not updated":          "开发版不自动更新",
		"release notes":                               "更新说明",
		"install update":                              "安装更新",
		"installing…":                                 "正在安装…",
		"checking…":                                   "正在检查…",
		"update RimArcho":                             "更新 RimArcho",
		"check for RimArcho updates":                  "检查 RimArcho 更新",
		"RimArcho is up to date":                      "已是最新版本",
		"RimArcho %s can be updated":                  "RimArcho %s 可以更新",
		"RimArcho %s can be updated; see Settings":    "RimArcho %s 可以更新，可在设置中查看",
		"download it from the release page":           "请从发布页下载",
		"no release published yet":                    "还没有发布版本",
		"development build":                           "开发版不自动更新",
		"downloading %s of %s":                        "正在下载 %s / %s",
		"checking the download…":                      "正在核对下载的文件…",
		"waiting for %d other RimArcho to close":      "等另外 %d 个 RimArcho 关闭",
		"replacing the program files…":                "正在替换程序文件…",
		"starting the new version…":                   "正在启动新版本…",
		"updated to RimArcho %s; restarting":          "已更新到 RimArcho %s，正在重启",
		"updated to RimArcho %s; open RimArcho again": "已更新到 RimArcho %s，请重新打开 RimArcho",
		"updated to RimArcho %s":                      "已更新到 RimArcho %s",
		"%d other RimArcho are open: the files are replaced once they close.":                                                                     "另外 %d 个 RimArcho 开着：等它们都关掉后再替换文件。",
		"Download RimArcho %s (%s), check it against its SHA-256 and replace the program files, then restart RimArcho. Your data is not changed.": "下载 RimArcho %s（%s），核对 SHA-256 后替换程序文件，然后重启 RimArcho。你的数据不会改动。",
		"another RimArcho is already installing the update":                                                                                       "另一个 RimArcho 正在安装这次更新",
		"the downloaded RimArcho %s does not match its checksum; nothing was changed":                                                             "下载的 RimArcho %s 与校验值不符，没有改动任何文件",
	})
}

var selfUpdateErrors = map[string]string{
	"a development build is not updated; build or download a release":                                       "开发版不自动更新，请自己构建或下载发布版",
	"RimArcho %s is the newest version known; check for updates first":                                      "RimArcho %s 已是已知的最新版本，请先检查更新",
	"cannot write to the program folder %s; download the new version from %s":                               "不能写入程序文件夹 %s，请从 %s 下载新版本",
	"download failed; nothing was changed: %w":                                                              "下载失败，没有改动任何文件：%s",
	"the release zip cannot be used; nothing was changed: %w":                                               "发布的 zip 不能用，没有改动任何文件：%s",
	"stopped while waiting for the other RimArcho to close; nothing was changed (the download is kept): %w": "在等其他 RimArcho 关闭时停下了，没有改动任何文件（下载的文件留着）：%s",
	"could not replace the program files; RimArcho %s is unchanged: %w":                                     "未能替换程序文件，RimArcho %s 没有改动：%s",
	"the new program did not start (%s), and putting RimArcho %s back failed: %w; the old files are in %s":  "新程序没能启动（%s），换回 RimArcho %s 也失败了：%s；旧文件在 %s",
	"the new program did not start, so RimArcho %s was put back: %w":                                        "新程序没能启动，已换回 RimArcho %s：%s",
	"the downloaded RimArcho %s does not match its checksum; nothing was changed":                           "下载的 RimArcho %s 与校验值不符，没有改动任何文件",
	"another RimArcho is already installing the update":                                                     "另一个 RimArcho 正在安装这次更新",
	"cannot check for updates: %w":                                                                          "无法检查更新：%s",
}

func init() {
	Register(selfUpdateErrors)
	for key := range selfUpdateErrors {
		if strings.Contains(key, "%") {
			interfaceFormats = append(interfaceFormats, interfaceFormatFor(key))
		}
	}
}
