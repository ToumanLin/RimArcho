# 反馈中转（Cloudflare Worker）

设置 › 反馈的“发送”把报告交给这个 Worker，Worker 用一个只能写 Issues 的 GitHub App 在 [ToumanLin/RimArcho](https://github.com/ToumanLin/RimArcho) 里开一条公开的 issue。玩家不需要 GitHub 账号。GitHub App 的私钥只存在 Worker 的 secret 里，不进 RimArcho。规格见 06 §2.14，代码结构见 07 §2。

RimArcho 里还没填中转地址时，反馈页只给“打开 GitHub”这一条路。

## 它做什么，不做什么

- 接收：`POST /report`，JSON `{title, body, contact}`，`User-Agent: RimArcho/<版本>`。成功时返回 `{number, url}`；失败时返回 `{error}`，RimArcho 把错误码翻成玩家看得懂的话。
- 防滥用：
  - 每个发送者每分钟 1 条，用限流绑定 `PER_MINUTE`。
  - 每个发送者每天 `PER_IP_DAY` 条，所有人加起来每天 `PER_DAY` 条，用 KV 计数，是近似值。
  - 长度上限：标题 120、正文 60,000、联系方式 200 字符。
  - `@某人` 和 `别的仓库#123` 中间插一个零宽空格，GitHub 照样显示，但不会通知谁。
- 隐私：
  - IP 只拿来计数，和私钥、日期一起哈希后存进 KV，两天后过期。issue 和 KV 里都没有 IP 原文。
  - 联系方式不进 issue，存在 KV 的 `contact:<issue 号>` 里，180 天后过期，只有你能看到。
  - 路径里的 Windows 用户名在 RimArcho 那边已经换成 `%USERPROFILE%`。
- 出事时的最坏情况：App 只有这一个仓库的 Issues 权限，所以私钥泄露或 Worker 被滥用，最多是在这个仓库里乱开、乱改 issue。

## 部署（你要做的每一步）

需要：Cloudflare 账号（免费计划就够）、Node.js（LTS 版），以及仓库所有者 ToumanLin 的 GitHub 账号。下面的命令都在仓库的 `feedback-relay/` 目录里跑。

1. **登录 Cloudflare**：`npx wrangler login`，浏览器里点允许。

2. **建 GitHub App**（GitHub → Settings → Developer settings → GitHub Apps → New GitHub App）：
   - GitHub App name：比如 `RimArcho Feedback`。这个名字在整个 GitHub 上不能重复，被占了就换一个。issue 的作者会显示成 `<名字>[bot]`。
   - Homepage URL：`https://github.com/ToumanLin/RimArcho`
   - Webhook：取消勾选 **Active**。
   - Permissions → Repository permissions → **Issues: Read and write**。其他都保持 No access；Metadata: Read-only 是 GitHub 强制带上的。
   - Where can this GitHub App be installed：**Only on this account**。
   - 点 Create GitHub App，把页面上的 **App ID** 填进 `wrangler.toml` 的 `APP_ID`。
   - 同一页下方 Private keys → **Generate a private key**，浏览器会下载一个 `.pem` 文件。
   - 左栏 **Install App** → ToumanLin 旁边的 Install → **Only select repositories** → 选 RimArcho → Install。

3. **建 KV**：`npx wrangler kv namespace create COUNTS`。把输出里的 `id` 填进 `wrangler.toml` 里 `[[kv_namespaces]]` 下的 `id`。

4. **部署**：`npx wrangler deploy`（需要 Wrangler 4.36 以上，`npx` 会取最新版）。它会打印 Worker 的地址，形如 `https://rimarcho-feedback.<你的子域>.workers.dev`。在下一步放好私钥之前，这个 Worker 对每个请求都回答 `not_configured`。

5. **放私钥**（PowerShell）：

   ```powershell
   Get-Content "$HOME\Downloads\<下载的文件>.pem" -Raw | npx wrangler secret put GITHUB_APP_KEY
   ```

   放好以后，把下载的 `.pem` 移到安全的地方（比如密码管理器）或者删掉。丢了也没关系，可以回 App 页面再生成一个新的。

6. （可选）在仓库的 Issues → Labels 里建一个 `feedback` 标签。不建也行：GitHub 拒绝这个标签时，Worker 会不带标签再开一次。

7. **在 VM 里真实发送一次**（[发布前 VM 测试](../docs/release-vm-tests.md) 里有这一行）：
   1. 先设好环境变量 `RIMARCHO_FEEDBACK_RELAY=https://rimarcho-feedback.<你的子域>.workers.dev/report`，再启动 RimArcho。
   2. 设置 › 反馈：标题写“中转测试”，联系方式随便填，点“发送”。
   3. 看到“已发送”和 issue 号以后，在 GitHub 上核对：
      - 有这条 issue，作者是 App；
      - 正文和预览一样，末尾有一行 “Sent from RimArcho…”；
      - 正文里没有联系方式。
   4. 用 `npx wrangler kv key get --binding COUNTS "contact:<号>" --remote` 读出联系方式。
   5. 一分钟内再发一次，应当提示“一分钟只能发一条”。
   6. 最后在 GitHub 上关掉或删掉测试 issue。

8. **写进程序**：把这个地址（带 `/report`）填进 [`internal/feedback/relay.go`](../internal/feedback/relay.go) 的 `Relay`，提交后发版。从这个版本起，反馈页才有“发送”按钮。

## 平时

- **看联系方式**：`npx wrangler kv key get --binding COUNTS "contact:<issue 号>" --remote`
- **改上限**：改 `wrangler.toml` 里的 `PER_IP_DAY`、`PER_DAY`，然后 `npx wrangler deploy`。
- **暂停接收**：把 `PER_DAY` 改成 `"0"` 再部署，RimArcho 会提示“今天收到的反馈太多了”，并给出 GitHub 那条路。
- **彻底停用**：`npx wrangler delete`。
- **换私钥**：在 App 页面生成新的私钥，重做第 5 步，再删掉旧的私钥。
- **还是有人刷屏**：可以考虑加 Cloudflare Turnstile 人机验证。这要改 RimArcho（发送前要过一道验证），现在还没做。
- **费用**：
  - Workers 免费计划每天 10 万次请求。
  - KV 免费计划每天 10 万次读、1,000 次写。每条反馈要写 2–3 次，所以 `PER_DAY` 不要超过 300。
  - 限流绑定的价格，Cloudflare 文档里没有写。

## 测试

`node --test "feedback-relay/*.test.mjs"`（在仓库根目录跑）。测试用一个假的 GitHub、一个 Map 当 KV，再用一个计数器代替限流绑定，不会调用真实的 GitHub 或 Cloudflare。覆盖这些情况：

- 开出 issue，IP 和联系方式都不进 issue；
- JWT 用 App 的公钥能验过；
- installation token 会复用；
- 每分钟、每个发送者每天、所有人每天这三道上限；
- 各种拒绝；
- 标签被拒时不带标签重开一次；
- PKCS#1、PKCS#8 和写成一行的私钥都能用。
