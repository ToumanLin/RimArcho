# 登录态管理调研：LoversLab 与 SteamCMD

> 日期：2026-10-24。性质：方案调研，以下“建议”均未实现。范围：本地规格、代码和公开资料；未登录任何服务、未输入或发送凭据、未运行 SteamCMD 登录或下载。

**可以统一设置入口，但两项的成熟度不同：SteamCMD 可先做；LoversLab 技术上可行，自动下载的站点许可和实际兼容性尚未确认。** 所有方案都不得让 RimArcho 的业务代码接收、读取、保存或转交密码、验证码、2FA 秘钥。会话 Cookie 本身也是凭据，必须另设边界。

建议先上线“设置 → 账户”的 SteamCMD 管理；LoversLab 继续“打开下载页 → 玩家下载 → 从 zip 安装”，待核实站点条款并获得支持后再试验独立 WebView2 登录。不要把“导入浏览器 Cookie”作为首选，也不要承诺“登录一次即可一直自动下载”。

## 1. 项目现状与接入点

| 证据 | 当前行为 | 对方案的影响 |
| --- | --- | --- |
| [02 §3.4](../specs/02-environments-identity-and-scanning.md#34-订阅git下载页steamcmd2026-10-24)；[page.go：ReadPage](../../internal/subscribe/page.go) | 单次 HTTPS GET；禁用 Cookie jar、禁止重定向；只解析 JSON-LD／Open Graph 和仓库链接 | 保留公开元数据读取；认证下载应是独立入口，不能悄悄改变阅读器的边界 |
| [subscribepage.go：SubscribePage](../../internal/app/subscribepage.go) | 有仓库可走 Git；发布包由玩家通过 --from 提供，再装入订阅目录 | 无仓库的 LoversLab 文件目前仍需手动下载 |
| [08 §11](../specs/08-launch-and-packs.md)；[gamedl.go：SteamCMDAccount、startSteamCMD](../../internal/app/gamedl.go) | 本次账户 → 设置账户 → anonymous；只传账户名；Windows 使用独立控制台；通过 SteamCMD 自己的日志读取进度 | 可复用账户选择与控制台启动，增加管理动作，不增加密码输入框 |
| [subscribeworkshop.go：SubscribeWorkshop、runWorkshopSteamCMD](../../internal/app/subscribeworkshop.go)；[deps.go：Finder.Status](../../internal/deps/deps.go) | 工坊先用 Steam 客户端，不可用才退回 SteamCMD（也可显式指定）；依赖查找决定实际可执行文件 | 工坊和游戏下载必须绑定同一实际 SteamCMD 实例及其缓存；设置页不能另找一份 |
| [01 §4、§4.1](../specs/01-scope-and-status.md)；[uninstall.go：UninstallPlan](../../internal/app/uninstall.go) | 已允许 SteamCMD 自己写登录缓存；外部工具或含 Mod 的目录可能保留 | 现有卸载不能保证忘记登录态，需增加独立于“删除工具”的凭据清理 |

Windows GUI 已使用 WebView2；[gui.go：Run、BrowserArgs](../../internal/surface/gui/gui.go) 的主界面有 Wails 服务绑定、共享用户数据目录及节省进程的参数。不能直接把主窗口导航到 LoversLab，也不能照搬主窗口的绑定和启动参数。

## 2. LoversLab

### 2.1 登录机制与本次可确认的事实

LoversLab 的 [robots.txt](https://www.loverslab.com/robots.txt) 标明 Invision Community。Invision 通常以会话及持久登录 Cookie 维持身份：例如 ips4_IPSSessionFront、ips4_member_id、ips4_login_key；设备 Cookie 也可能参与验证。**这些是平台示例，不是已核实的 LoversLab 当前 Cookie 清单，且一个 Cookie 未必足够。** 登录表单和某些操作使用与会话配对的 CSRF token，不能把旧 csrfKey 当固定下载参数。来源：[Invision Cookie 说明](https://invisioncommunity.com/cookies/)、[模板的 CSRF 参数](https://invisioncommunity.com/4guides/themes-and-customizations/template-syntax/template-plugins-r143/)。

本次对公开首页、条款、Cookie 说明和文件分类页的普通访问被 403 拦住；首页响应含 Cloudflare 的“Just a moment”和“Enable JavaScript and cookies”。这证明本次遇到挑战，**不能把所有 403 都诊断为未登录**。验证码／挑战必须交给玩家，不能自动求解、循环重试或绕过。WebView2 能否通过须验证；取得论坛 Cookie 后转给 Go HTTP 客户端，也可能因挑战状态和浏览器上下文不同而失败。来源：[Cloudflare 支持的浏览器与 WebView 限制](https://developers.cloudflare.com/cloudflare-challenges/reference/supported-browsers/)、[clearance 机制](https://developers.cloudflare.com/cloudflare-challenges/concepts/clearance/)。

Invision 支持可选／按群组强制的 2FA，包括认证器等；LoversLab 目前是否启用、对哪些账户启用，本次未确认。设计必须允许玩家在原站页面完成整个流程；不接触验证码、恢复码或 TOTP 秘钥。来源：[Invision 账户安全指南](https://invisioncommunity.com/4guides/security-and-rules/account-security-two-factor-auth-r316/)。

### 2.2 下载需要什么

预期链路是：**文件页 → 原站下载按钮 → 可能的声明／“I agree”及文件选择 → 实际文件请求／重定向**。文件页 ID 不等于某一版本的附件 ID；一个页面可有多个版本、多个包。历史 IPS4 集成包含 do=download、附件标识、确认参数等，但不能据此保证现站接口稳定；本次未进入登录后的链路，也未验证所有文件都有“I agree”。来源：[Wabbajack 历史 IPS4 链路说明](https://github.com/wabbajack-tools/wabbajack/wiki/Home/1aa44fef735e1996b924ae2c329b225805db1931)。

若后续获准接入：

1. 在玩家指定的文件页读取当次真实链接；需要登录时打开登录界面，不模拟提交用户名／密码。
2. 如有声明、许可或“I agree”，展示原文并由玩家点击；不能预填 confirm 参数跳过。多文件时让玩家选包，不猜“第一个就是正确版本”。
3. 用该会话及当次 CSRF 状态发起文件请求。允许必要的 HTTPS 重定向，但凭据按域、路径、Secure 等规则发送；不得把 Cookie／Authorization 原样转给外部镜像或任意域。临时签名 URL 不进日志、source.json 或分享清单。
4. 先下载到受管临时目录，检查响应确实是文件而非登录／挑战 HTML，再复用现有压缩包与 About.xml 校验、安装流程。

站点可能按用户组设置次数、等待时间、并发、大小或带宽限制；**未查到 LoversLab 当前权威数值，不写死“每天 N 次”**。产品侧建议单服务串行；遵守站点倒计时和 Retry-After，遇 429 暂停并显示可重试时间；权限拒绝、挑战和限额分别提示。不得通过换账户、代理或重新登录规避限制。

已有项目并不能证明这条路可靠：[Wabbajack 现行文档](https://wiki.wabbajack.org/modlist_author_documentation/Meta%20Files.html#manualurl) 明确只支持 LoversLab 的 manualURL，解释为 API 下载问题；[当前 AIPS4Source 源码](https://github.com/wabbajack-tools/wabbajack/blob/main/Wabbajack.Downloaders.ManualSources/AIPS4Source.cs) 同样要求在浏览器下载。历史 OAuth/API 代码不是 RimArcho 已获 API 授权的证据。

### 2.3 站点规则、合法性与成人站点界面

本次成功读取 robots.txt：User-Agent: * 禁止 /login/、/register/、/lostpassword/、/search/ 等路径，并禁止部分 do=add／do=submit 查询；没有针对 /files/ 的整段禁令。应避免程序爬取这些禁用路径；玩家亲自在网页登录与爬虫自动登录应区分。**未禁止 /files/ 不代表授权自动下载**；robots 也不是访问授权或许可协议。来源：[站点 robots.txt](https://www.loverslab.com/robots.txt)、[RFC 9309](https://www.rfc-editor.org/rfc/rfc9309.html#section-3)。

[站点条款](https://www.loverslab.com/terms/) 与[社区规则](https://www.loverslab.com/guidelines/) 本次均返回 403，无法确认是否禁止自动抓取／下载。上线认证下载前，应核对玩家可见的现行条款及单文件许可，必要时向站方确认；若禁止 bots／download managers 或要求专用 API，就不能用 Cookie 重放规避。**当前结论是“许可未确认”，不是“已确认违规”，也不是“合法性已获保证”。**

由玩家使用自己的账户、获取其有权下载的内容、遵守站规和作者许可，是可接受方案的前提；登录不授予再分发权，不能共享账户／Cookie、抓取受限内容或绕过付费与年龄限制。适用法律和成人内容访问要求取决于地区，现有证据不足以给出全球合法性结论。

入口默认关闭，玩家主动启用；使用中性名称“LoversLab 会话”“打开站点登录”，首次启用说明站点含成人内容并要求满足原站访问条件。账户页不显示成人图片、推荐内容，不自动注册或确认年龄声明。

### 2.4 方案比较与密码边界

| 方案 | 优点 | 缺点与取舍 |
| --- | --- | --- |
| 独立 WebView2，由玩家在原站登录，保存必要会话 Cookie | 登录、验证码、2FA 都可留在原网页；可通过原生 Cookie API 获取 HttpOnly Cookie | Cloudflare 兼容性不保证；必须隔离网页和本地权限；持久化的不一定只是一个 Cookie。作为后续有条件的试验方案 |
| 玩家主动导入站点 Cookie 文件 | 可沿用系统浏览器已完成的登录，不在 RimArcho 内输入密码 | 文件等同账户凭据；导出易泄漏、会过期，也不保证可转移挑战状态。只考虑明确选择的该站 Cookie，不扫描浏览器数据库 |
| 系统浏览器登录并下载，RimArcho 从 zip 安装 | 密码与浏览器会话均留在浏览器；适配挑战和站点变动的成本最低 | 多一步手动导入；不能据此实现后台自动下载。当前推荐保底路径 |

“从不保存密码”是三种方案共同的硬约束，不是一个需要密码登录的第四种方案。不能做 RimArcho 密码框、代填、表单抓取、键盘监听、登录 POST 请求体读取或密码型凭据保存。

WebView2 若实施，应使用独立环境／profile，关闭密码自动保存和一般表单自动填充；禁用 host objects、Wails 服务、页面到宿主消息桥、DevTools／远程调试、录屏及请求体诊断。仅允许必要的站点导航和挑战资源；不得向网页开放文件或命令接口。通过原生 CookieManager 读取限定站点的必要 Cookie，不用 document.cookie／DOM 脚本读取密码。来源：[WebView2 安全指南](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/security)、[密码自动保存开关](https://learn.microsoft.com/en-us/microsoft-edge/webview2/reference/win32/icorewebview2settings4)、[CookieManager](https://learn.microsoft.com/en-us/microsoft-edge/webview2/reference/win32/icorewebview2cookiemanager)。

**宿主在技术上能执行脚本／检查 WebView，所以“业务代码从不读取密码”可以约束并审计，“宿主绝无能力看到密码”不能由嵌入式 WebView 保证。** 若“绝不看到”要求这种强隔离，就选系统浏览器手动下载；若未来站方提供支持的授权流程，优先使用系统浏览器授权，不抓取 OAuth 登录密码或令牌。

不做 Chrome／Edge Cookie 数据库解密或提权导入；Chrome 的 Windows Cookie 还使用 App-Bound Encryption，直接读取既脆弱又扩大权限范围。即使有玩家同意，也不应绕过该保护。来源：[Chrome 官方说明](https://security.googleblog.com/2024/07/improving-security-of-chrome-cookies-on.html)。手动 Cookie 文件若以后支持，导入后应提醒玩家删除原明文文件，并说明 RimArcho 无法清除其备份。

### 2.5 Windows 会话保存、过期与退出

**建议采用当前 Windows 用户范围的 DPAPI 文件**，例如 %LOCALAPPDATA%\RimArcho\accounts\loverslab.session：只包含必要 Cookie 及其 domain、path、Secure、HttpOnly、SameSite、Expires 等属性；不含密码。使用 CryptProtectData 的用户范围，不设 CRYPTPROTECT_LOCAL_MACHINE，并限制文件 ACL。它适合 Cookie 集合，比把秘密塞进普通配置／SQLite／source.json 更容易排除出备份。来源：[DPAPI](https://learn.microsoft.com/en-us/windows/win32/api/dpapi/nf-dpapi-cryptprotectdata)。

Windows Credential Manager 的 CRED_TYPE_GENERIC 也可用，使用 RimArcho 专属目标名和本机持久范围；但单条 blob 上限为 2560 字节，整组 Cookie 可能超出。它也不自动替应用判断站点会话过期，需显式 CredDelete。二者择一，避免重复保存。来源：[CREDENTIALW](https://learn.microsoft.com/en-us/windows/win32/api/wincred/ns-wincred-credentialw)、[CredWriteW](https://learn.microsoft.com/en-us/windows/win32/api/wincred/nf-wincred-credwritew)。

WebView2 默认还会在用户数据目录保存 Cookie、缓存等，不能只加密一份导出文件就声称没有其他副本。若目标是“仅持久化必要 Cookie”，需使用临时独立 profile，导出后关闭浏览器进程并清理整个 profile；下次只恢复获准保留的 Cookie。其对挑战和设备状态的影响须验证，失败就回到浏览器手动下载。来源：[WebView2 用户数据目录](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/user-data-folder)。

过期处理：

- 显示 Cookie 已知的本地到期时间，并注明“服务器可能提前使其失效”；多个凭据到期关系需按实际用途判断。会话 Cookie 无 Expires 时写“会话级／未知”，不编造固定天数，也不默认跨启动永久保存。
- 设置页读取本地状态不联网；玩家点“验证”或执行下载时才确认有效性，并记录最后验证时间。已保存 ≠ 当前有效。
- 登录重定向／明确未认证 → 标“需重新登录”，暂停该服务任务并打开玩家登录入口；挑战 → “需浏览器验证”；429 → “达到限制”；文件权限拒绝不直接抹掉会话。重登后由玩家重试，不无限刷新。
- “退出并忘记”先停用会话、清空内存，再删除 DPAPI 文件／凭据项及 WebView Cookie、存储和缓存。可在联网且站点支持时注销当前服务端会话；本地忘记不等于撤销其他设备或已复制出去的 Cookie。忘记不可撤销，不留可恢复的秘密备份。

DPAPI／Credential Manager 保护静态数据和用户隔离，不能防住以同一用户运行、拥有任意文件／进程访问能力的恶意程序或 agent。

## 3. SteamCMD

### 3.1 缓存、Steam Guard 与“记住密码”

Valve 明确说明：首次在 SteamCMD 登录时由玩家输入密码和 Steam Guard；退出后，后续只用账户名可复用保存在 config/config.vdf 的登录 token，文件还可能更新。因此“记住密码”在这里应解释为 **SteamCMD 自己保存可复用登录凭据**，不是 RimArcho 保存密码，也不应虚构一个通用 --remember-password 开关。来源：[Steamworks：CI/CD 登录缓存](https://partner.steamgames.com/doc/sdk/uploading#Using_SteamPipe_In_A_CI_CD_Environment)。

sentry／ssfn 是相关的设备信任文件；较旧流程通常同时保留它与 config.vdf，设备信任与可代替密码的登录 token 不能混为一谈。新版本未必生成 ssfn，文件名和布局不是稳定 API：**不能以“没有 ssfn”判断没缓存，也不能以“有 ssfn”判断已登录。** 来源：[Codemagic 的 sentry 文件说明](https://docs.codemagic.io/yaml-publishing/steam/)、[集成项目关于不再生成 ssfn 的记录](https://github.com/game-ci/steam-deploy/issues/56)；后者只是版本差异证据，不是 Valve 的格式保证。

RimArcho 不解析／复制 token、ssfn 内容，不把它们迁入自身凭据库。SteamCMD 以当前用户运行、自己维护缓存，RimArcho 限定其目录权限；不能宣称这些第三方文件已经由 RimArcho 用 DPAPI 加密。所有密码、Guard 码或手机确认只发生在 SteamCMD 自己的窗口中；无需 RimArcho 处理 TOTP／恢复码。

### 3.2 能显示什么、如何登录

可以显示选定账户名、实际 SteamCMD 路径／来源、缓存迹象、最后成功时间和当前任务状态，但应明确证据强度：

| 状态 | 可依据 | 不能据此断言 |
| --- | --- | --- |
| 匿名 | 解析后的有效账户为 anonymous | 匿名能下载所有应用／条目 |
| 已配置账户，尚未验证 | steamcmd.account 或本次覆盖值 | 账户已经登录 |
| 检测到可能的缓存 | 受管实例存在配置／缓存文件及本地成功记录；不读取秘密内容 | 文件中一定有该账户的有效 token |
| 本次已登录为 X | 当前 SteamCMD 调用确认的成功结果，与请求账户匹配 | 进程退出后仍在线；下次不会要求 Guard |
| 最近登录成功，需时再验证 | 上次成功记录 | 缓存永久有效 |
| 需重新登录／等待玩家／网络错误 | 本次明确认证失败、等待交互或网络失败 | 网络失败等于凭据失效 |

RimArcho 没有可靠的离线“当前登录状态 API”；配置文件存在性最多是线索。页面不能静默启动 SteamCMD 试登录，因为它会联网、改缓存，还可能弹密码提示。有效期一般不可得，显示“由 Steam 管理，未知”及最后验证时间。

“登录／重新登录”复用实际依赖路径及工作目录，打开独立原生控制台，只传 +login <账户名>；让玩家直接输入密码、Guard，并在完成后退出。Windows 启动窗口是该动作本身的目的。业务层不接管 stdin，不用伪终端录制或抓屏；只将允许的阶段／结果转成状态，不保留 info 或控制台日志的完整转储。Valve 的文档允许玩家在窗口中用 info 确认连接；不要把整个 info 输出送进 GUI、任务记录或 MCP。

工坊与游戏下载复用同一账户解析，维持“本次覆盖 → 设置 → 匿名”。缓存身份应绑定实际 SteamCMD 实例，换工具路径时重置“最近成功”推断；同一实例的登录、验证、下载和清理互斥，避免清理运行中的缓存。

### 3.3 退出与忘记：需要两个语义

SteamCMD 的 logout／logoff 用于断开并撤销保存的登录信息。历史上存在“logout 不清缓存”的问题，但 Valve 跟踪记录显示 2025 年复测已正常并关闭该问题；不能把旧故障写成今天必然如此。Windows 当前发行版仍须验证。来源：[Valve #7120](https://github.com/ValveSoftware/steam-for-linux/issues/7120)、[当前命令清单](https://github.com/dgibbs64/SteamCMD-Commands-List/blob/main/steamcmd_commands.txt)。

建议提供“退出并忘记此 SteamCMD 的登录”：

- 对已登录的受管实例由 SteamCMD 执行 logout 后退出；不要仅 quit，也不要把清空 steamcmd.account 当作清除缓存。
- 离线或 logout 失败时，仍可执行明确标注的“本地忘记”：关闭受管进程后清理该实例已验证的认证存储位置；不读取 token，也不留认证文件备份。config/config.vdf、旧版 ssfn／sentry 等须列入版本验收；未知布局不能声称清理成功。必要时重置专用受管运行目录，同时保留独立下载的游戏和 Mod。
- 首版只管理一份专用 SteamCMD 的账户；如清理会影响其中多个缓存账户，预览必须明说。不要按猜测的 VDF 字段做单账户删改。
- 本地清除不保证撤销已经泄漏的凭据或其他设备；需要远程撤销时引导玩家用 Steam 账户安全／Steam Guard 的设备管理。

外部指定路径／PATH 的 SteamCMD 可能被其他程序共用，不能自动删除其 config、ssfn 或 userdata。建议“账户管理”的持久登录只使用 RimArcho 专用实例；外部实例标“外部缓存，不接管”，不提供会在该目录新建受管登录态的按钮。如此才能做到卸载时完整忘记 RimArcho 管理的会话，而不破坏其他工具。

### 3.4 匿名能下载哪些内容

匿名能读取 RimWorld 公共应用／分支信息；普通公开、允许下载的 **RimWorld 工坊条目（AppID 294100）通常也支持匿名**，不要求 Steam 客户端或 Steam 版游戏。RimSort 官方项目文档确认这种使用方式。它不是所有游戏工坊的普遍规则，也不保证私有、仅好友、已下架、封禁或另有权限限制的条目可下；以 Valve 当次返回为准，不硬编码一个“所有公开工坊均可匿名”的规则。来源：[RimSort SteamCMD 指南](https://rimsort.github.io/RimSort/user-guide/steamcmd-browser)。

读取分支和下载游戏本体是不同权限。RimWorld 本体／旧版本需相应许可；匿名收到 No subscription 时应让玩家选择有许可的账户或已有游戏目录，不绕过所有权检查。具体分支仍读 Steam 返回，不猜名称；维持现有匿名默认，不在本地预先断言所有匿名下载都必败。Valve Developer Wiki 本次访问为 403，因此没有将其未读正文当作验证证据。

## 4. 统一的“设置 → 账户”

账户是全局的下载服务配置，不按游戏环境复制；首版每服务一个选用身份即可。

| 服务 | 名称 | 状态与时间 | 操作 |
| --- | --- | --- | --- |
| SteamCMD | 选定账户名或“匿名”；同时显示实际工具来源 | 按 §3.2 区分配置、缓存迹象、本次成功、需登录；到期未知，显示最后验证时间 | 修改账户名、登录／重新登录、主动验证、切换匿名、退出并忘记 |
| LoversLab（玩家启用后） | 站点确认的显示名；拿不到写“未知”，不把会员 ID 当用户名 | 未连接／已保存未验证／可用／需登录／需浏览器验证／限额；已知 Cookie 到期及最后验证时间 | 启用、打开站点登录、主动验证、退出并忘记、打开下载页；导入 Cookie 延后 |
| LoversLab（当前阶段） | — | “浏览器手动下载” | 打开下载页、从 zip 安装；不展示尚未实现的“已连接” |

操作遵守现有规则：

- **写入与联网**：[01 §4](../specs/01-scope-and-status.md) 要求限定写入位置、预览、玩家触发和可撤回说明。新增会话文件、独立 WebView profile 或凭据目标名必须列入清单；预览只显示服务、位置类别、影响范围和不可恢复说明，不显示 Cookie／token。首次启用说明会保存会话；验证也可能刷新缓存，不作为无副作用的只读查询。设置页普通读取不联网。
- **密码与配置**：普通 config／archo.db 仅存非秘密的账户选择和派生状态；秘密不进数据库备份、导出、整合包、source.json、命令行参数、环境变量、任务 JSON、错误正文或诊断包。现有“附 SteamCMD 原话”的规则需收窄为允许列表字段，不能把原始日志交给 agent。
- **MCP／agent**：只暴露服务、粗粒度状态、需玩家操作等脱敏信息；不提供读取／导出／导入凭据、填写密码／Guard、操作登录页或通用认证 HTTP 代理的工具。登录、忘记和站点声明确认由玩家入口完成。已授权的固定下载动作可在本地使用会话，但 agent 只给文件页／条目引用并取结果，不取得凭据或任意携带凭据的请求能力；域名和下载范围必须校验。登录窗口不接入 agent 的 DOM／截图／调试渠道。
- **隔离限度**：同用户任意 shell／文件访问的外部 agent 不受 DPAPI 的应用隔离保护；“凭据绝不交给 agent”还需要 harness 限制秘密目录、浏览器 profile、进程及调试访问。MCP 不返回凭据是必要条件，不能宣称已防住任意本机代码。
- **卸载**：增加独立“忘记账户会话”步骤，放在删除工具／数据之前，不能因 SteamCMD 目录有 Mod 被保留就跳过。清理所有 RimArcho 管理的会话文件、Credential Manager 项、临时 profile 和受管 SteamCMD 认证缓存；保留 Mod、游戏、系统浏览器登录及外部工具缓存。凭据清理失败必须保留可重试状态并报告，不能显示“已全部忘记”。未来若迁移会话目录，只能有一个权威副本，卸载也需覆盖已登记旧位置。

## 5. 实施顺序与规格变更

1. **先定边界与卸载，再加 SteamCMD 管理。** 复用 deps 找到的专用实例；首版只有账户名、登录窗口、诚实状态和退出／本地忘记。工坊 fallback 与游戏下载共用，不实现 token 读取或多账户保险箱。真实 Windows 行为需玩家在后续验收中自行登录。
2. **LoversLab 当前保留手动链路。** 可把相关入口收在账户／订阅流程，但不为未实现的自动下载保存会话。先核对现行站规、自动下载许可及站方支持的 API／授权方式；这一步不需要先收集任何玩家凭据。
3. **许可明确后试验独立 WebView2。** 玩家登录／选文件／点同意；验证 Cookie 最小集合、挑战兼容性、加密存储、过期恢复和卸载。优先让下载留在同一浏览器上下文；只有可行且获准时才评估转交 HTTP 下载器。若无法满足密码隔离或站规，维持手动下载。
4. **浏览器 Cookie 文件导入最后考虑。** 仅在明确需求和许可下支持站点限定的手动文件导入；不做浏览器数据库扫描／解密。

仅提出下列规格修改，本报告不修改规格正文：

| 规格 | 建议变更 |
| --- | --- |
| 01 §4、§4.1 | 增加会话／profile／凭据库写入项和独立忘记步骤；将密码禁令扩为所有服务；明确 Steam token 仍仅由 SteamCMD 保管，RimArcho 只允许不读取内容的限定清理；补齐玩家触发的认证／验证联网行为 |
| 02 §3.4 | 保留无凭据 ReadPage 的合同；如后续批准 LoversLab 下载，另写认证下载、文件选择、声明确认、限额、失效和手动兜底规则 |
| 08 §11 | 增加登录／验证／退出／忘记、实例与缓存绑定、状态证据、匿名与许可区别；替换原始日志回传，补真实 Windows 验收 |
| 06 §2.14（[现行分册](../specs/06-ui/2.11-other-pages.md)） | 原 SteamCMD 账户项并入“账户”；增加动作、状态、最后验证和未知到期显示；LoversLab 中性且 opt-in；更新设置动作允许列表 |
| 05、09 | 明确凭据管理是玩家专属入口；MCP 与内置 Archo 的脱敏状态／固定下载能力，禁止凭据参数、登录页观测和通用认证代理 |
| 07 §2.1、存储／GUI 规则 | deps 继续只负责工具发现；新增会话存储、ACL、备份排除、独立 WebView 环境、调试隔离与清理规则 |

后续验收重点：首次与缓存登录、Guard／取消／离线／失效；匿名工坊和无许可游戏失败；工具路径切换不串状态；logout 后不能复用旧受管缓存；含 Mod 的 SteamCMD 被保留时仍忘记会话；导出／诊断／MCP 不含秘密。LoversLab 另外覆盖多文件、声明、挑战、429、Cookie 轮换和临时 profile 清理；本次未执行这些认证验收。

## 6. 证据到结论的对应

| 证据 | 结论 | 建议路径 |
| --- | --- | --- |
| E1：本地 ReadPage、SteamCMDAccount、启动器与卸载实现（§1 链接） | F1：现有密码边界可沿用，认证下载与卸载清理存在新工作 | P1：先补规格和独立忘记步骤，再接账户动作 |
| E2：本次 robots.txt 的 200 响应；公开页 Cloudflare 403；条款不可读（§2） | F2：不能确认自动下载许可或真实登录后链路 | P2：继续浏览器手动下载；核实规则后才试验会话下载 |
| E3：Valve 官方缓存说明、#7120 的修复记录（§3） | F3：可只传账户名；缓存不等于在线，退出清理需按版本验收 | P3：原生窗口登录 → 派生状态 → SteamCMD logout／受管本地忘记 |
| E4：Microsoft DPAPI、Credential Manager、WebView2 文档（§2.4–2.5） | F4：用户范围加密可保护持久会话，但不隔离同用户任意进程 | P4：单一秘密存储 → profile 清理 → agent 不可观测／取凭据 |

观察日期同报告日期。E1 可通过所链文件与符号复查；E2 可用不带 Cookie 的普通 GET 复查，响应随时间／网络变化，403 本身不是条款正文证据；E3、E4 可按来源链接复核。本报告仅记录脱敏观察，不另存网络响应、凭据或 case 文件。
