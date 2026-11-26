# Feedback relay (Cloudflare Worker)

"send" in Settings › Feedback passes the report to this Worker, which opens a public issue in [ToumanLin/RimArcho](https://github.com/ToumanLin/RimArcho) using a GitHub App with write access only to Issues. Players need no GitHub account. The GitHub App private key lives only in a Worker secret, never in RimArcho. Specification: 16 §2.14; code structure: 17 §2.

Until RimArcho has a relay URL, the feedback page offers only "open GitHub".

## What it does and does not do

- Receives: `POST /report`, JSON `{title, body, contact, kind, priority}`, `User-Agent: RimArcho/<version>`. Returns `{number, url, token, issues}` on success or `{error}` on failure; RimArcho translates error codes into player-facing text.
  - Several problems in one send ("add another problem", 16 §2.14): `{reports: [{title, body, kind, priority}, …], contact}`, at most `MAX_SPLIT` (3). Each becomes its own issue, one after another, each with its own reply token; `issues` lists them and the first is also at the top level. The whole send takes one minute of the sender's allowance, and each issue counts toward the daily caps. If GitHub fails partway, the answer is 502 with the issues filed so far; the rest are given back to the daily counts.
  - Labels: `LABEL`, then the template's label (`kind` feature → `enhancement`, bug → `bug`, diagnosis → `diagnosis`, data → `data`, l10n (translation fix) → `localization`, other → `question`; anything else none), then `priority` for a bug whose files were changed or lost (`priority: true`). If GitHub refuses a label that does not exist, the Worker retries with `LABEL` and GitHub's own labels (bug, enhancement, question), then with none.
- Similar requests: `GET /requests` returns `{requests: [{number, title, url, plus}]}`, the open issues labelled `enhancement` (no pull requests, newest 100) with their +1 reactions plus the +1s sent through the relay. Kept 10 minutes in KV (`cache:requests`). Counted with status reads against `STATUS_PER_HOUR`.
- +1 without an account: `POST /plus/<n>` for an open `enhancement` issue returns `{plus}`. One per sender per issue per day (`plusby:<hash>:<n>`, two days; RimArcho also remembers what it gave), 20 a day per sender (`plusday:<hash>`). The count is `plus:<n>` in KV (no expiry) and one comment by the App on the issue ("+N from RimArcho players without a GitHub account"), created once (`plusc:<n>` holds its id) and edited after. Another number is 404.
- Replies: `token` is 24 random bytes that only the sender's RimArcho keeps; KV holds only its HMAC (keyed with the App key) as `reply:<issue number>` for 180 days, and the token never enters the issue. `GET /status/<n>?t=<token>` returns `{state, closed_at, replies:[{body, created_at}]}`, where `replies` are only the comments whose `author_association` is `OWNER` (your replies; nobody else's comment reaches the player). A wrong token is 404, the same as a missing issue. It reads the issue and its comments with the same Issues permission, so the GitHub App needs nothing new. Limit: `STATUS_PER_HOUR` (default 120) per sender per hour, an approximate KV counter (`status:<hash>`, two hours). RimArcho asks only at start-up and when Feedback opens (16 §2.14).
- Abuse prevention:
  - One report per sender per minute and `PER_IP_DAY` reports per sender per day, exact: a Durable Object per sender (`SendGate`, binding `GATE`), which handles one request at a time and reads its own writes. A report GitHub refuses gives the minute back. Until 1.0.6 the minute limit was Cloudflare's rate-limit binding, which is approximate and per location: it let a second report through 13 s after the first.
  - `PER_DAY` total per day, using an approximate KV counter.
  - Length limits: title 120, body 60,000, contact 200 characters.
  - Insert a zero-width space inside `@someone` and `other-repo#123`: GitHub still displays them without notifying anyone.
- Privacy:
  - IP is used only for counting, hashed with a private key and date before storage in KV, expiring after two days. Neither issues nor KV contain raw IPs.
  - Contact details stay out of issues, stored in KV as `contact:<issue number>` for 180 days, visible only to you.
  - RimArcho has already replaced Windows usernames in paths with `%USERPROFILE%`.
- Worst case: the App has only Issues access to this repository, so a leaked key or abused Worker can at most open or edit unwanted issues here.

## Deployment (every step you need)

Requires a Cloudflare account (the free plan suffices), Node.js (LTS) and repository owner ToumanLin's GitHub account. Run all commands below in the repository's `feedback-relay/` directory.

1. **Sign in to Cloudflare**: `npx wrangler login`, then allow access in the browser.

2. **Create a GitHub App** (GitHub → Settings → Developer settings → GitHub Apps → New GitHub App):
   - GitHub App name: for example `RimArcho Feedback`. The name must be unique across GitHub; choose another if taken. Issue authors appear as `<name>[bot]`.
   - Homepage URL: `https://github.com/ToumanLin/RimArcho`
   - Webhook: uncheck **Active**.
   - Permissions → Repository permissions → **Issues: Read and write**. Leave everything else at No access; GitHub requires Metadata: Read-only.
   - Where can this GitHub App be installed: **Only on this account**.
   - Click Create GitHub App; put the page's **App ID** into `APP_ID` in `wrangler.toml`.
   - Below, Private keys → **Generate a private key** downloads a `.pem` file.
   - Left sidebar: **Install App** → Install beside ToumanLin → **Only select repositories** → select RimArcho → Install.

3. **Create KV**: `npx wrangler kv namespace create COUNTS`. Put the output's `id` into `id` under `[[kv_namespaces]]` in `wrangler.toml`.

4. **Deploy**: `npx wrangler deploy` (requires Wrangler 4.36+, and `npx` fetches the latest). It prints a Worker URL such as `https://rimarcho-feedback.<your-subdomain>.workers.dev`. Until the next step supplies the key, every request returns `not_configured`.

5. **Store the private key** (PowerShell):

   ```powershell
   Get-Content "$HOME\Downloads\<downloaded-file>.pem" -Raw | npx wrangler secret put GITHUB_APP_KEY
   ```

   Then move the downloaded `.pem` somewhere safe (such as a password manager) or delete it. If lost, generate a new one on the App page.

6. (Optional) Create the labels `feedback`, `diagnosis`, `data`, `localization` and `priority` under Issues → Labels (`bug`, `enhancement` and `question` exist in every repository). This is optional: if GitHub rejects a label, the Worker retries with `feedback` and GitHub's own labels, then without labels. Similar requests list the issues labelled `enhancement`; label older feature requests so players find them.

7. **Send one real report in the VM** (a row in [pre-release VM tests](../UserGuide/release-vm-tests.md) covers this):
   1. Set `RIMARCHO_FEEDBACK_RELAY=https://rimarcho-feedback.<your-subdomain>.workers.dev/report` before starting RimArcho.
   2. Settings › Feedback: enter "relay test" as the title, any contact details, and click "send".
   3. After "sent as #N" appears, check GitHub:
      - The issue exists and its author is the App;
      - The body matches the preview and ends with "Sent from RimArcho…";
      - Contact details are absent from the body.
   4. Read the contact details with `npx wrangler kv key get --binding COUNTS "contact:<number>" --remote`.
   5. Send again within a minute; it should say "one report a minute; send it again in a moment".
   6. Reply to the issue on GitHub as ToumanLin, then restart RimArcho: the Feedback entry gets a dot and "my reports" shows the reply (the VM row "Feedback replies").
   7. Close or delete the test issue on GitHub afterwards.

8. **Put it in the program**: set `Relay` in [`internal/feedback/relay.go`](../internal/feedback/relay.go) to this URL (including `/report`), commit and release. The feedback page offers "send" starting with that version.

## Routine operations

- **Read contact details**: `npx wrangler kv key get --binding COUNTS "contact:<issue number>" --remote`
- **Change limits**: edit `PER_IP_DAY` and `PER_DAY` in `wrangler.toml`, then `npx wrangler deploy`.
- **Pause intake**: set `PER_DAY` to `"0"` and redeploy. RimArcho reports "RimArcho has had too many reports today; send it tomorrow or open GitHub" and offers GitHub instead.
- **Disable entirely**: `npx wrangler delete`.
- **Rotate the private key**: generate a new key on the App page, repeat step 5, then delete the old key.
- **Still getting spam**: consider Cloudflare Turnstile verification. This requires a RimArcho change (verification before sending) and is not implemented yet.
- **Cost**:
  - Workers free plan: 100,000 requests per day.
  - KV free plan: 100,000 reads and 1,000 writes per day. Each report takes 1–2 writes, so keep `PER_DAY` at or below 300.
  - Durable Objects (SQLite storage) are on the free plan; each report is one or two requests to its sender's object, which deletes its storage two days later.

## Tests

`node --test "feedback-relay/*.test.mjs"` (from the repository root). Tests use a fake GitHub, a Map as KV and the real `SendGate` behind a fake Durable Object namespace; no real GitHub or Cloudflare calls. Coverage:

- Issue creation, with neither IP, contact details nor the reply token in the issue, and only the token's hash in KV;
- Labels by template and priority, and the label fallbacks;
- Split sends: one request, one minute, one issue and token per problem, each counted for the day; a GitHub failure partway;
- Similar requests (no pull requests, the KV cache) and +1 (once per sender a day, the per-sender cap, the one App comment, 404 for other numbers);
- Status: only the owner's comments; a wrong token or another issue's number is 404 without asking GitHub; the hourly limit per sender;
- JWT verification against the App public key;
- Installation-token reuse;
- All three limits: per minute (including a resend after 13 s), per sender per day, total per day; a GitHub failure gives the minute back;
- Rejection cases;
- Retry without a label if the label is rejected;
- PKCS#1, PKCS#8 and single-line private keys.