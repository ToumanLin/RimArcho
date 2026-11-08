# Feedback relay (Cloudflare Worker)

"send" in Settings › Feedback passes the report to this Worker, which opens a public issue in [ToumanLin/RimArcho](https://github.com/ToumanLin/RimArcho) using a GitHub App with write access only to Issues. Players need no GitHub account. The GitHub App private key lives only in a Worker secret, never in RimArcho. Specification: 16 §2.14; code structure: 17 §2.

Until RimArcho has a relay URL, the feedback page offers only "open GitHub".

## What it does and does not do

- Receives: `POST /report`, JSON `{title, body, contact}`, `User-Agent: RimArcho/<version>`. Returns `{number, url}` on success or `{error}` on failure; RimArcho translates error codes into player-facing text.
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

6. (Optional) Create a `feedback` label under Issues → Labels. This is optional: if GitHub rejects the label, the Worker retries issue creation without it.

7. **Send one real report in the VM** (a row in [pre-release VM tests](../UserGuide/release-vm-tests.md) covers this):
   1. Set `RIMARCHO_FEEDBACK_RELAY=https://rimarcho-feedback.<your-subdomain>.workers.dev/report` before starting RimArcho.
   2. Settings › Feedback: enter "relay test" as the title, any contact details, and click "send".
   3. After "sent as #N" appears, check GitHub:
      - The issue exists and its author is the App;
      - The body matches the preview and ends with "Sent from RimArcho…";
      - Contact details are absent from the body.
   4. Read the contact details with `npx wrangler kv key get --binding COUNTS "contact:<number>" --remote`.
   5. Send again within a minute; it should say "one report a minute; send it again in a moment".
   6. Close or delete the test issue on GitHub afterwards.

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

- Issue creation, with neither IP nor contact details in the issue;
- JWT verification against the App public key;
- Installation-token reuse;
- All three limits: per minute (including a resend after 13 s), per sender per day, total per day; a GitHub failure gives the minute back;
- Rejection cases;
- Retry without a label if the label is rejected;
- PKCS#1, PKCS#8 and single-line private keys.