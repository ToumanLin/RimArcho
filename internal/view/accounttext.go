package view

import (
	"fmt"
	"regexp"
	"strings"
)

func init() {
	enText["environment create failed"] = "WebView2 setup failed"
	enText["environment interface not available"] = "WebView2 interface not available"

	Register(map[string]string{
		"%d failed": "%d 个失败",
		"%s: %s":    "%s：%s",
		"mod downloads available, game downloads: not checked, account: %s": "下载Mod可用，下载游戏：未检查，登录账号：%s",
		"COM initialize failed":                         "COM 初始化失败",
		"login window failed or session cleanup failed": "登录窗口或会话清理失败",
		"runtime load failed":                           "WebView2 加载失败",
		"runtime entry point not found":                 "未找到 WebView2 入口",
		"window class registration failed":              "登录窗口注册失败",
		"window create failed":                          "登录窗口创建失败",
		"webview interface failed":                      "网页接口不可用",
		"settings interface not available":              "浏览器设置接口不可用",
		"settings apply failed":                         "浏览器设置失败",
		"download interface not available":              "下载接口不可用",
		"download handler failed":                       "下载处理器创建失败",
		"navigation handler failed":                     "导航处理器创建失败",
		"frame handler failed":                          "网页框架处理器创建失败",
		"popup handler failed":                          "弹窗处理器创建失败",
		"permission handler failed":                     "权限处理器创建失败",
		"controller bounds failed":                      "窗口大小设置失败",
		"controller visibility failed":                  "窗口显示失败",
		"navigation failed":                             "网页导航失败",
		"environment interface not available":           "WebView2 接口不可用",
		"login window ready":                            "登录窗口已就绪",
		"login window completed":                        "登录窗口已完成",
		"the SteamCMD account must be empty (anonymous), or a name using letters, digits and _ . @ -": "Steam 账号名只能包含字母、数字和 _ . @ -",
		"account not checked yet":                                           "账户尚未验证",
		"possible cached login, not checked yet":                            "可能有登录缓存，尚未验证",
		"player login required":                                             "需玩家登录",
		"SteamCMD is not installed":                                         "未安装 SteamCMD",
		"would download with SteamCMD":                                      "将用 SteamCMD 下载",
		"download was not confirmed":                                        "未确认下载完成",
		"no About/About.xml":                                                "没有 About/About.xml",
		"SteamCMD did not exit successfully; confirmed items were retained": "SteamCMD 未正常退出，已保留确认成功的条目",
		"Page: %s":       "页面：%s",
		"Version: %s":    "版本：%s",
		"Repository: %s": "仓库：%s",
		"Use --git to subscribe to a linked repository, or download in your browser and use --from <zip|folder>.": "用 --git 订阅页面链接的仓库，或在浏览器下载后用 --from <zip|folder> 安装。",
		"choose --git or --from, not both": "请选择 --git 或 --from，不能同时使用",
		"cannot tell which linked repository is the mod (%d: %s); subscribe to it with archo subscribe git <repository>": "无法确认哪个仓库是此 Mod（%d：%s）；请用 archo subscribe git <repository> 选择仓库",
		"page is subscribed already: %s":                                       "已订阅此页面：%s",
		"scan did not find subscription at %s":                                 "扫描未找到此订阅：%s",
		"the game is running (pid %d); close it before updating subscriptions": "游戏正在运行（进程 %d），请先关闭再更新订阅",
		"%s is not a page subscription; --from is only for page subscriptions": "%s 不是下载页订阅；--from 只用于下载页",
		"old folder is in the recycle bin; replacement remains at %s: %w":      "旧文件夹已进回收站，新文件仍在 %s：%w",
		"subscription %s: %w":                                                            "订阅 %s：%w",
		"%s (%s %q) is subscribed already at %s":                                         "%s（%s %q）已订阅于 %s",
		"repository root has no About/About.xml; mod subdirectories: %s":                 "仓库根目录没有 About/About.xml；Mod 子目录：%s",
		"installed at %s but scan did not find a live instance":                          "已安装于 %s，但扫描未找到有效的 Mod",
		"%s is not a subscription":                                                       "%s 不是订阅",
		"%s: update check unavailable offline for %s":                                    "%s：离线时不能检查 %s 的更新",
		"%s: updating %s requires --from <zip|folder>":                                   "%s：更新 %s 需要 --from <zip|folder>",
		"give at least one subscription reference":                                       "请选择至少一个订阅",
		"%s is not directly inside a subscription source":                                "%s 不在订阅来源的直接子目录中",
		"give at least one Workshop ID or URL":                                           "请填写至少一个工坊 ID 或网址",
		"not a Workshop URL: %s":                                                         "不是工坊网址：%s",
		"invalid Workshop ID %q":                                                         "工坊 ID 无效：%q",
		"Workshop %s: Steam did not confirm subscription (%s); %s":                       "工坊 %s：Steam 未确认订阅（%s）；%s",
		"install SteamCMD in Settings; Workshop pages: %s":                               "请在设置安装 SteamCMD；工坊页面：%s",
		"SteamCMD download directory overlaps src:%s (%s)":                               "SteamCMD 下载目录与来源 src:%s（%s）重叠",
		"SteamCMD download directory overlaps subscriptions":                             "SteamCMD 下载目录与订阅目录重叠",
		"SteamCMD item %s: %s; %s":                                                       "SteamCMD 条目 %s：%s；%s",
		"SteamCMD item %s has no About/About.xml at %s":                                  "SteamCMD 条目 %s 在 %s 没有 About/About.xml",
		"check failed: %s":                                                               "检查失败：%s",
		"incoming links changed since preview":                                           "关联链接在预览后发生变化",
		"incoming link changed since preview: %s":                                        "关联链接在预览后发生变化：%s",
		"affected game is running: %s":                                                   "受影响的游戏正在运行：%s",
		"new link path is occupied or inaccessible: %s":                                  "新链接位置已占用或无法访问：%s",
		"link recovery remains pending: %s":                                              "链接恢复尚未完成：%s",
		"cannot reattach %s: %s":                                                         "无法重新连接 %s：%s",
		"included link is not a prefixed entry in the selected game scope: %s":           "明确纳入的链接不在所选游戏的受管入口中：%s",
		"missing link repair preview":                                                    "请先预览链接修复",
		"invalid repair entry: %s":                                                       "链接修复条目无效：%s",
		"game is running: %s; stop it before repairing links":                            "游戏正在运行：%s；请先关闭再修复链接",
		"link changed since preview: %s":                                                 "链接在预览后发生变化：%s",
		"game is running: %s":                                                            "游戏正在运行：%s",
		"target changed since preview: %s":                                               "目标在预览后发生变化：%s",
		"link preview expired; reopen the needs page":                                    "链接预览已失效，请重新打开“能不能跑”",
		"unknown instance: %s":                                                           "找不到这份 Mod：%s",
		"unfinished link operation: %s; review archo links repair --all --dry-run first": "链接操作尚未完成：%s；请先查看 archo links repair --all --dry-run",
		"cannot check incoming links in %s: %w":                                          "无法检查 %s 中的关联链接：%w",
		"cannot rule out an incoming link at %s: %s; leave it alone and resolve the check failure first": "无法排除 %s 的关联链接：%s；请先解决检查失败，不要移动或删除它",
		"moving or removing %s would break foreign link %s -> %s (%s); detach it yourself first":         "移动或删除 %s 会使其他工具的链接 %s -> %s（%s）失效，请先自行移除该链接",

		"some checks failed":                             "部分检查失败",
		"%d no longer on the Workshop":                   "%d 个已从工坊消失",
		"%d: %s":                                         "%d：%s",
		"Steam could not complete the request":           "Steam 未能完成请求",
		"no longer on the Workshop (deleted or private)": "工坊里已找不到（被删除或设为私有）",
		"access denied":                                  "没有访问权限",
		"Steam request timed out":                        "Steam 请求超时",
		"Steam service unavailable":                      "Steam 服务暂不可用",
		"Steam account has insufficient privileges":      "Steam 账号权限不足",
		"Steam request limit exceeded":                   "Steam 请求已达限额",
		"Steam access revoked":                           "Steam 访问权已撤销",
		"Steam account restricted":                       "Steam 账号受限",
		"Steam request rate limited":                     "Steam 请求过于频繁",
		"Steam request failed (code %s)":                 "Steam 请求失败（代码 %s）",
		"%s Workshop items could not be checked":         "%s 个工坊条目未能检查",
		"login required; open the login window or download a zip in your browser": "需登录；请打开登录窗口，或在浏览器下载 zip",
		"session cleanup failed; close the login window and retry":                "账户会话清理失败，请关闭登录窗口后重试",
		"account sessions require Windows DPAPI":                                  "账户会话需要 Windows DPAPI",
		"site redirect not supported":                                             "站点跳转暂不支持",
		"site request cooldown is active; try again later":                        "站点请求间隔或限额尚未结束，请稍后重试",
		"session request cancelled":                                               "会话请求已取消",
		"the site requires login again":                                           "站点要求重新登录",
		"site redirect not supported (download host not verified)":                "站点跳转暂不支持（下载主机尚未确认）",
		"session request did not complete; try again later":                       "会话请求未完成，请稍后重试",
		"site request limit reached; wait before retrying":                        "站点达到请求限制，请等待后重试",
		"cannot read the download page":                                           "下载页无法读取",
		"the site requires browser verification":                                  "站点需要浏览器验证",
		"session expired":                                                         "会话已失效",
		"site access denied (permission or browser verification)":                 "站点拒绝访问（权限或浏览器验证）",
		"download file":    "下载文件",
		"confirm download": "确认下载",
		"no downloadable files recognized; use your browser": "未识别到可下载文件，请使用浏览器",
		"choose a LoversLab file page":                       "请选择 LoversLab 文件页",
		"file selection expired; read the page again":        "文件选择已过期，请重新读取",
		"cannot create the temporary download folder":        "无法创建下载临时目录",
		"cannot create the temporary download file":          "无法创建下载临时文件",
		"download interrupted; retry or download manually":   "下载中断，请重试或使用手动下载",
		"cannot write the downloaded file":                   "下载文件写入失败",
		"cannot read the verification page":                  "验证页无法读取",
		"session needs player confirmation":                  "会话需要玩家确认",
		"unknown account action":                             "未知账户操作",
		"unknown account service":                            "未知账户服务",
		"account action completed":                           "账户操作已完成",
		"account action interrupted; try again":              "账户操作中断，请重新发起",
		"player action completed":                            "玩家操作已完成",
		"download selection expired; read the page again":    "下载选择已失效，请重新读取",
		"subscription installed":                             "订阅已安装",
		"install subscription":                               "安装订阅",
		"Install the download in the subscriptions folder, replacing this subscription when updating.": "将下载文件安装到订阅目录；更新时替换此订阅的旧文件。",
		"Choose a file. Site agreements are shown before confirmation.":                                "选择一个文件；若站点需要确认，会先显示原文。",
		"site download agreement": "站点下载确认",
		"I agree, continue":       "我同意并继续",
		"read LoversLab files":    "读取 LoversLab 可下载文件",
		"Public page unavailable; try the session or choose a downloaded zip.":   "公开页面无法读取，可尝试会话下载或手动选择 zip。",
		"update using the LoversLab session":                                     "从 LoversLab 会话更新",
		"This subscription has no LoversLab file page; choose a downloaded zip.": "此订阅没有可用的 LoversLab 文件页，请选择下载的 zip。",
		"file page does not match the subscription; read it again":               "更新文件页与订阅不一致，请重新读取",
		"log in":                                 "登录",
		"log in again":                           "重新登录",
		"log out":                                "退出账号",
		"mod downloads available, not logged in": "下载Mod可用，未登录",
		"mod downloads available, game downloads unavailable, account: %s": "下载Mod可用，下载游戏不可用，登录账号：%s",
		"mod downloads available, game downloads available, account: %s":   "下载Mod可用，下载游戏可用，登录账号：%s",
		"available, Cookie expiry: %s":                                     "可用，Cookie 到期时间：%s",
		"unavailable, not logged in":                                       "不可用，未登录",
		"external SteamCMD manages its own login":                          "使用外部 SteamCMD，登录由它自己管理",
		"Steam account name":                                               "Steam 账户名",
		"enter to log in, esc to cancel":                                   "enter 登录，esc 取消",
		"enter a Steam account name first":                                 "先填写 Steam 账户名",
		"Sign out of SteamCMD. If Steam cannot confirm logout, including when offline, RimArcho automatically forgets this managed instance's local login cache for all accounts. Other devices are unaffected.": "退出 SteamCMD 账号；若退出失败或离线，将自动清理此受管 SteamCMD 所有账号的本地登录缓存。不影响其他设备。",
		"Forget the local LoversLab session. Other devices are unaffected.": "忘记本机的 LoversLab 会话，不影响其他设备。",
		"LoversLab contains adult content and has its own access conditions and rules. Use your own account and follow the site's rules and file licenses. Login cookies are encrypted for your Windows user on this device; passwords stay on the site.": "LoversLab 含成人内容，有自己的访问条件与规则。请使用你自己的账号，遵守站点规则与文件许可。登录 Cookie 以当前 Windows 用户加密保存在本机；密码只在原站输入。",
		"LoversLab — close this window after login to save the session": "LoversLab — 登录完成后关闭此窗口保存会话",
		"account operation":             "账户操作",
		"LoversLab download / install":  "LoversLab 下载 / 安装",
		"source: %s":                    "来源：%s",
		"target folder: %s":             "目标文件夹：%s",
		"size: %s bytes":                "大小：%s 字节",
		"size unknown":                  "大小未知",
		"login required":                "需登录",
		"not connected":                 "未连接",
		"available":                     "可用",
		"browser verification required": "需浏览器验证",
		"rate limited":                  "限额",
		"SteamCMD window did not complete; try signing in again":              "SteamCMD 窗口未完成操作，请重新登录",
		"network error; login status unknown":                                 "网络错误，登录状态未知",
		"sign in again to complete password and Steam Guard verification":     "请重新登录，在 SteamCMD 窗口完成密码和 Steam Guard 验证",
		"managed SteamCMD path was redirected; restore it before signing out": "受管 SteamCMD 路径已重定向，请恢复路径后再退出",
		"authentication cache cleanup failed; close SteamCMD and retry":       "认证缓存清理失败，请关闭 SteamCMD 后重试",
		"authentication cache cleanup failed":                                 "认证缓存清理失败",
		"authentication cache path was redirected; nothing was removed":       "认证缓存路径已重定向，未清理",
		"unknown authentication cache layout; sign out in SteamCMD and retry": "认证缓存布局未知，请在 SteamCMD 中退出后重试",
		"open the download page and choose a zip downloaded in your browser":  "请打开下载页，在浏览器下载后选择 zip",
		"signed in":                                    "已登录",
		"signed out":                                   "已退出",
		"WebView2 runtime not found":                   "未找到 WebView2 运行时",
		"environment create failed":                    "WebView2 初始化失败",
		"controller create failed":                     "登录窗口创建失败",
		"session directory unavailable":                "会话目录不可用",
		"login helper did not complete":                "登录窗口未完成操作",
		"login helper stopped unexpectedly":            "登录窗口意外中断",
		"login window closed without a usable session": "登录窗口已关闭，没有可用会话",
		"cookie manager interface not available":       "Cookie 管理接口不可用",
		"cookie manager not available":                 "Cookie 管理器不可用",
	})

	for key, value := range map[string]string{"需登录；请打开登录窗口，或在浏览器下载 zip": "login required; open the login window or download a zip in your browser",
		"账户会话清理失败，请关闭登录窗口后重试":  "session cleanup failed; close the login window and retry",
		"账户会话需要 Windows DPAPI": "account sessions require Windows DPAPI",
		"站点跳转暂不支持":             "site redirect not supported",
		"站点请求间隔或限额尚未结束，请稍后重试":  "site request cooldown is active; try again later",
		"会话请求已取消":              "session request cancelled",
		"站点要求重新登录":             "the site requires login again",
		"站点跳转暂不支持（下载主机尚未确认）":   "site redirect not supported (download host not verified)",
		"会话请求未完成，请稍后重试":        "session request did not complete; try again later",
		"站点达到请求限制，请等待后重试":      "site request limit reached; wait before retrying",
		"下载页无法读取":              "cannot read the download page",
		"站点需要浏览器验证":            "the site requires browser verification",
		"会话已失效":                "session expired",
		"站点拒绝访问（权限或浏览器验证）":     "site access denied (permission or browser verification)",
		"下载文件":                 "download file",
		"确认下载":                 "confirm download",
		"未识别到可下载文件，请使用浏览器":     "no downloadable files recognized; use your browser",
		"请选择 LoversLab 文件页":    "choose a LoversLab file page",
		"文件选择已过期，请重新读取":        "file selection expired; read the page again",
		"无法创建下载临时目录":           "cannot create the temporary download folder",
		"无法创建下载临时文件":           "cannot create the temporary download file",
		"下载中断，请重试或使用手动下载":      "download interrupted; retry or download manually",
		"下载文件写入失败":             "cannot write the downloaded file",
		"验证页无法读取":              "cannot read the verification page",
		"会话需要玩家确认":             "session needs player confirmation",
		"未知账户操作":               "unknown account action",
		"未知账户服务":               "unknown account service",
		"账户操作已完成":              "account action completed",
		"账户操作中断，请重新发起":         "account action interrupted; try again",
		"玩家操作已完成":              "player action completed",
		"下载选择已失效，请重新读取":        "download selection expired; read the page again",
		"订阅已安装":                "subscription installed",
		"安装订阅":                 "install subscription",
		"将下载文件安装到订阅目录；更新时替换此订阅的旧文件。": "Install the download in the subscriptions folder, replacing this subscription when updating.",
		"选择一个文件；若站点需要确认，会先显示原文。":     "Choose a file. Site agreements are shown before confirmation.",
		"站点下载确认":             "site download agreement",
		"我同意并继续":             "I agree, continue",
		"读取 LoversLab 可下载文件": "read LoversLab files",
		"公开页面无法读取，可尝试会话下载或手动选择 zip。": "Public page unavailable; try the session or choose a downloaded zip.",
		"从 LoversLab 会话更新": "update using the LoversLab session",
		"此订阅没有可用的 LoversLab 文件页，请选择下载的 zip。": "This subscription has no LoversLab file page; choose a downloaded zip.",
		"更新文件页与订阅不一致，请重新读取":                  "file page does not match the subscription; read it again",
		"重新登录":        "log in again",
		"退出账号":        "log out",
		"下载Mod可用，未登录": "mod downloads available, not logged in",
		"下载Mod可用，下载游戏不可用，登录账号：%s": "mod downloads available, game downloads unavailable, account: %s",
		"下载Mod可用，下载游戏可用，登录账号：%s":  "mod downloads available, game downloads available, account: %s",
		"可用，Cookie 到期时间：%s":       "available, Cookie expiry: %s",
		"不可用，未登录":                 "unavailable, not logged in",
		"使用外部 SteamCMD，登录由它自己管理":  "external SteamCMD manages its own login",
		"Steam 账户名":       "Steam account name",
		"enter 登录，esc 取消": "enter to log in, esc to cancel",
		"先填写 Steam 账户名":   "enter a Steam account name first",
		"退出 SteamCMD 账号；若退出失败或离线，将自动清理此受管 SteamCMD 所有账号的本地登录缓存。不影响其他设备。":                              "Sign out of SteamCMD. If Steam cannot confirm logout, including when offline, RimArcho automatically forgets this managed instance's local login cache for all accounts. Other devices are unaffected.",
		"忘记本机的 LoversLab 会话，不影响其他设备。":                                                                 "Forget the local LoversLab session. Other devices are unaffected.",
		"LoversLab 含成人内容，有自己的访问条件与规则。请使用你自己的账号，遵守站点规则与文件许可。登录 Cookie 以当前 Windows 用户加密保存在本机；密码只在原站输入。": "LoversLab contains adult content and has its own access conditions and rules. Use your own account and follow the site's rules and file licenses. Login cookies are encrypted for your Windows user on this device; passwords stay on the site.",
		"LoversLab — 登录完成后关闭此窗口保存会话":                                                                  "LoversLab — close this window after login to save the session",
		"账户操作":              "account operation",
		"LoversLab 下载 / 安装": "LoversLab download / install",
		"来源：%s":             "source: %s",
		"目标文件夹：%s":          "target folder: %s",
		"大小：%s 字节":          "size: %s bytes",
		"大小未知":              "size unknown",
		"需登录":               "login required",
		"未连接":               "not connected",
		"需浏览器验证":            "browser verification required",
		"限额":                "rate limited",
		"SteamCMD 窗口未完成操作，请重新登录":                  "SteamCMD window did not complete; try signing in again",
		"网络错误，登录状态未知":                             "network error; login status unknown",
		"请重新登录，在 SteamCMD 窗口完成密码和 Steam Guard 验证": "sign in again to complete password and Steam Guard verification",
		"受管 SteamCMD 路径已重定向，请恢复路径后再退出":            "managed SteamCMD path was redirected; restore it before signing out",
		"认证缓存清理失败，请关闭 SteamCMD 后重试":               "authentication cache cleanup failed; close SteamCMD and retry",
		"认证缓存清理失败":                                "authentication cache cleanup failed",
		"认证缓存路径已重定向，未清理":                          "authentication cache path was redirected; nothing was removed",
		"认证缓存布局未知，请在 SteamCMD 中退出后重试":             "unknown authentication cache layout; sign out in SteamCMD and retry",
		"请打开下载页，在浏览器下载后选择 zip":                    "open the download page and choose a zip downloaded in your browser",
		"已登录":              "signed in",
		"已退出":              "signed out",
		"未找到 WebView2 运行时": "WebView2 runtime not found",
		"WebView2 初始化失败":   "WebView2 setup failed",
		"登录窗口创建失败":         "controller create failed",
		"会话目录不可用":          "session directory unavailable",
		"登录窗口未完成操作":        "login helper did not complete",
		"登录窗口意外中断":         "login helper stopped unexpectedly",
		"登录窗口已关闭，没有可用会话":   "login window closed without a usable session",
		"Cookie 管理接口不可用":   "cookie manager interface not available",
		"Cookie 管理器不可用":    "cookie manager not available",
		"最近登录成功，需时再验证":     "signed in", "最近登录成功": "signed in"} {
		enText[key] = value
	}
}

type interfaceFormat struct {
	key string
	re  *regexp.Regexp
}

var interfaceFormats = []interfaceFormat{}

func init() {
	for _, key := range []string{"cannot tell which linked repository is the mod (%d: %s); subscribe to it with archo subscribe git <repository>", "cannot rule out an incoming link at %s: %s; leave it alone and resolve the check failure first", "moving or removing %s would break foreign link %s -> %s (%s); detach it yourself first", "unfinished link operation: %s; review archo links repair --all --dry-run first", "the game is running (pid %d); close it before updating subscriptions", "%s is not a page subscription; --from is only for page subscriptions", "included link is not a prefixed entry in the selected game scope: %s", "old folder is in the recycle bin; replacement remains at %s: %w", "repository root has no About/About.xml; mod subdirectories: %s", "Workshop %s: Steam did not confirm subscription (%s); %s", "installed at %s but scan did not find a live instance", "game is running: %s; stop it before repairing links", "install SteamCMD in Settings; Workshop pages: %s", "SteamCMD download directory overlaps src:%s (%s)", "%s is not directly inside a subscription source", "SteamCMD item %s has no About/About.xml at %s", "new link path is occupied or inaccessible: %s", "%s: updating %s requires --from <zip|folder>", "%s: update check unavailable offline for %s", "incoming link changed since preview: %s", "%s (%s %q) is subscribed already at %s", "%s Workshop items could not be checked", "cannot check incoming links in %s: %w", "scan did not find subscription at %s", "link recovery remains pending: %s", "target changed since preview: %s", "page is subscribed already: %s", "link changed since preview: %s", "Steam request failed (code %s)", "affected game is running: %s", "%s is not a subscription", "SteamCMD item %s: %s; %s", "invalid repair entry: %s", "not a Workshop URL: %s", "invalid Workshop ID %q", "cannot reattach %s: %s", "unknown instance: %s", "subscription %s: %w", "game is running: %s", "check failed: %s", "Repository: %s", "Version: %s", "failed: %s", "Page: %s"} {
		pattern := regexp.QuoteMeta(key)
		for _, verb := range []string{"%s", "%d", "%w"} {
			pattern = strings.ReplaceAll(pattern, verb, "(.+?)")
		}
		pattern = strings.ReplaceAll(pattern, "%q", `"([^"]*)"`)
		interfaceFormats = append(interfaceFormats, interfaceFormat{key, regexp.MustCompile("^" + pattern + "$")})
	}
}

// Text translates fixed diagnostics and joined reasons, retaining data such as
// names, paths, site agreements and HRESULTs verbatim.
func Text(s string) string {

	if v := T(s); v != s {
		return v
	}
	for _, f := range interfaceFormats {
		if match := f.re.FindStringSubmatch(s); match != nil {
			if Lang() == "en" {
				return s
			}
			args := make([]any, len(match)-1)
			for i, v := range match[1:] {
				args[i] = v
			}
			format := strings.NewReplacer("%d", "%s", "%w", "%s").Replace(T(f.key))
			return fmt.Sprintf(format, args...)
		}
	}
	for _, sep := range []string{"\n", "; ", "；"} {
		if strings.Contains(s, sep) {
			parts := strings.Split(s, sep)
			changed := false
			for i := range parts {
				original := parts[i]
				parts[i] = Text(parts[i])
				changed = changed || parts[i] != original
			}
			if !changed {
				return s
			}
			if sep == "\n" {
				return strings.Join(parts, "\n")
			}
			return strings.Join(parts, T("; "))
		}
	}
	if i := strings.LastIndex(s, " 0x"); i >= 0 {
		return T(s[:i]) + s[i:]
	}
	return s
}
