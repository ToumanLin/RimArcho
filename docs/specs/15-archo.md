# 15 · Archo (built-in AI assistant)

> Status: design specification for 1.1; all features are unimplemented. The only AI integration available now is an external agent using `archo mcp` (14).
> Basis: 00 (§2, §3.2, §4), 00a, 05, 06.

Archo is RimArcho's built-in AI assistant. Its image is the project icon, `internal/icon/portrait.svg`; at small sizes, the bow on the portrait serves as its mark (16 §7). Every UI reference to the AI assistant uses this name and portrait, for example, "Ask Archo to analyze this error". External agents (Claude Code, Codex, etc.) connecting through MCP use their own names.

Two premises apply throughout:

- **Every feature works well without Archo.** Archo saves the player a few steps; it is never a feature's only entry point. Players can perform every action behind a one-click task in the UI themselves.
- **Archo follows the same rules as the player.** Anything it can do, the player can do. Its changes enter undo history. Writing game files, moving files to the recycle bin and making backups require player confirmation. Sending content to a model requires consent first.

## 1. Form

- **A lightweight built-in harness** (for example, Pi; selection undecided) runs in RimArcho's GUI process. **Enabled by default; can be disabled** on the settings page (16 §2.14). Disabling it hides every "Ask Archo to …" button and the Archo entry at the bottom of the sidebar, leaving no gap.
- **Entry point**: Archo at the bottom of the sidebar opens the right panel (16 §2.13). It can open on any page; the panel and conversation persist when changing pages. One-click task buttons on pages (§4.2) also open this panel and show task progress and results there.
- **GUI only.** The TUI does not yet support the Archo panel (§6); the CLI has no chat command. Terminal users who need AI connect an external agent to `archo mcp`.
- **No resident process**: closing the panel after a task ends leaves no background process (00 §4 item 6). When invoking local Claude Code / Codex, the child process exits with the task.
- **Conversations do not persist across sessions** (undecided): closing RimArcho clears the conversation. Whether to save it, and for how long, will be decided after actual use. If saved, it stays only in the local data directory (17 §3).

## 2. Tools and permissions

### 2.1 Tools are the tool layer from 05

Account operations follow the player-only exception in 13 §1.2. Archo knows only that the player needs to log in. It cannot observe the independent login window's DOM, screenshots, debugging or input; read / set / import cookies or passwords; or accept site declarations for the player. The external harness must also exclude secret directories, temporary profiles and their processes. DPAPI does not isolate arbitrary local code running as the same user.

Archo uses the commands and tool layer defined in 13 §1, the same set as the MCP tool table (14 §2): identical names, parameters and returned text. There is no separate "AI-only" interface for Archo, because the player, external agents and Archo must receive the same facts (00 §4 item 5).

Tool descriptions are also shared (14 §3). Archo adds only a system prompt containing RimArcho concepts and terms (03 §1, to answer questions such as "what is a lineage?"), the current page and selected objects (such as the current plan, selected Mod and selected log entry), and the permission rules in §2.2.

New player actions must gain corresponding tools at the same time. For actions without tools, Archo can only suggest that the player click the button.

### 2.2 Three permission levels

| Level | Includes | Archo's behavior |
| --- | --- | --- |
| Read-only | All tools with `readOnlyHint: true`: `status`, `find_mods`, `get_mod`, `get_description`, `get_def`, `get_patches`, `get_harmony`, `explain_relation`, `list_problems`, `list_changes`, `analyze_log`, `search_files`, `read_source`, and the save-reading tools from 10 (12 §1.6) | Call directly without asking the player |
| Reversible changes | Plan edits (`edit_loadout`: enable, disable, move, swap instance, sort, stage), player data (`set_player_data`: tags, aliases, notes), user rules (`manage_rules`), creating locked versions (`pin_mod`) | Call directly. Each step uses an existing action and enters the **current plan's** undo history (07 §3). Operation names in history start with "Archo:". Each change shows a line in the panel: "Archo did … · undo" |
| Player confirmation required | All tools marked `destructiveHint: true` in 14 §2: save (`save_loadout`, writes ModsConfig.xml), launch the game (`launch_game`), move to the recycle bin (02 §3), back up saves (12 §4.4), answer identity questions (`resolve_identity`), import from RimCrow, cleanup (`maintenance`), etc. | Archo can only **request** the action. A confirmation card appears in the panel, specifying which files will change (the same confirmation rules as 16 §6); the player clicks "confirm". Nothing happens without that click |

Only the player can change environments and settings; there are no corresponding tools (13 §1.2). When Archo determines that these changes are needed, it includes a button in its answer that opens the relevant page.

### 2.3 Archo cannot bypass confirmation

- RimArcho's action layer enforces confirmation; it does not rely on harness compliance. When the tool layer receives a call requiring confirmation, it only registers a pending request and returns `pending confirmation` (fixed wording, undecided). The actual write waits for the player to click "confirm" in the GUI.
- Only the player's mouse or keyboard input can dismiss a confirmation card. No Archo tool can confirm for the player or change the setting that requires confirmation.
- When invoking local Claude Code / Codex (§3.1), RimArcho connects them to the same tool layer, with the same confirmation rules. They too receive only `pending confirmation` (connection details undecided; see §3.1).
- When an external agent connects directly to `archo mcp`, its harness decides whether to allow the call (14 §2 annotations: `save_loadout` has `destructiveHint: true`). This remains unchanged: the player configures external agents; RimArcho is responsible only for truthful annotations.

### 2.4 Records

- Every Archo tool call can be expanded in the panel to show the tool name, parameters and first few lines of returned text. Players can see the basis for its conclusions (00 §4 item 2).
- No separate AI action log. Undo history is the record of changes (07 §3).

## 3. Model sources and consent

### 3.1 Model sources

Two options; the player selects one on the settings page (16 §2.14):

1. **The player's own API key.** Supported providers are undecided (at least one must support image input for OCR, §5). The key stays local. Storage is undecided (Windows Credential Manager is favored over a plain configuration file). Keys never enter logs, exports or diagnostics.
2. **Invoke locally installed Claude Code or Codex.** RimArcho stores none of their login information; it uses the player's existing login. This option appears only when RimArcho detects `claude` or `codex` on PATH. Integration is undecided (for example, launching them noninteractively with `archo mcp` as the only tool server), subject to §2.3: RimArcho's GUI still collects confirmation.

Without a configured model source, the Archo entry remains visible. Opening it shows an explanation and a "go to settings" button.

### 3.2 Consent

Consent to send content to the model is requested by **task type**. Before the **first** transmission for each task type, a prompt explains what that type sends and to whom (the current model source). The player chooses "agree" or "disagree". Agreement suppresses future prompts for that task type; disagreement cancels this invocation.

| Content category | Tasks that send it |
| --- | --- |
| Mod names and descriptions | Translation, tag classification, chat questions about a Mod |
| Images in descriptions | OCR (§5) |
| Game log excerpts | Error analysis, fixing Mod errors |
| Decompiled source excerpts | Fixing Mod errors through `read_source`, including `game: true` for the current game's code; the local Source library is not an attachment (05 §5.4; tool contract: 14 §1) |
| Mod lists, finding lists, sorting constraints | Fixing sorting, sorting error analysis, chat |
| Statistics read from saves (not the saves themselves) | Interpreting unused content, playthrough themes and content conflicts (12 §3, 12 §5) |

- Consent is stored by task type. The settings page (16 §2.14) lists each type's status and lets the player reset it to "not agreed" or "disagree" at any time. The next use asks again after a reset.
- Save files, API keys and usernames in local paths are never sent to the model. Before sending log excerpts, replace the username in user directory paths with `<user>`.
- In chat, Archo chooses which tools to call. Returned content follows the consent status of its category. Without consent for a category, that content is not sent; Archo explains what is missing and where to change it.

## 4. Tasks

### 4.1 Chat

Ask questions in the player's own words in the Archo panel (16 §2.13). Typical questions:

- About the manager: "What are lineage and instance?", "How does a locked version differ from a plan?" Answers come from the terminology in the system prompt (03 §1); no player data needs to be sent.
- About the current state: "Why can't Combat Extended and Yayo's Combat be enabled together?" Archo calls `explain_relation` and `get_mod`, and includes evidence from their output.
- Requests for action: "Tag every VE Mod with VE", "Disable these three Mods". Execute under the permissions in §2.2.

### 4.2 One-click tasks

Each one-click task is a button on a page. Clicking opens the Archo panel and starts the task. Results take the form of suggestions with one-click application; applying them follows §2.2.

| Task | Button location | What it does | Applying the result |
| --- | --- | --- | --- |
| Fix a sorting cycle | The "contradictory order" item in can it run (16 §2.4); the automatic sorting failure message in the plan editor (16 §2.5) | Read sorting constraints and rules, find the rules forming the cycle, explain which is most likely wrong | Suggest deleting or rewriting a user rule (`manage_rules` add / remove), or changing the order; clicking "apply" adds it to undo history |
| Analyze and fix manager and sorting errors | Every item in "breaks the game" and "for you to confirm" in can it run (16 §2.4) | Explain why the finding appeared, its evidence and a fix | The fix is an existing action (enable a dependency, disable, move, swap instance); the player clicks "apply" |
| Fix Mod errors using logs and local decompilation | Every item in "errors from the last run" in can it run (16 §2.4); every error cluster in game log (16 §2.10) | Find causes using log attribution (06), Harmony patches and on-demand decompilation (05 §5.4) | Offer one-click application for fixes using list operations (disable, reorder, swap instance); for Mod code changes, provide only explanations and locations, never modify Mod files |
| Translate names and descriptions | Top right of the description in Mod details (16 §4.1); batch actions for multiple selections in Mod inventory (16 §2.9) | Translate names and descriptions into the UI language | Display only, labeled "translated by Archo"; cached locally (location undecided); never change About.xml or overwrite player aliases |
| Classify tags | Batch actions for multiple selections in the mod library (16 §2.9) | Suggest tags from names, descriptions and Def categories | A "suggested tags" checklist, applied once after the player selects items (`set_player_data` tag_add, one undo step) |
| Read text in description images | Below each image in Mod descriptions (16 §4.3, 16 §4.4); "read all images" at the top right of the description | See §5 | Show recognized text below the image |

Tasks after 1.1: interpreting unused content (playthrough review, 12 §3); playthrough themes and content conflicts (1.2, 12 §5).

## 5. OCR (reading text in description images)

### 5.1 Why

Workshop descriptions for Mods such as Vanilla Expanded use many images containing section titles and feature showcases. Reading text alone misses much of the content.

- Measured on 2026-10-19 (1274 Workshop Mods with About.xml; 1264 Workshop descriptions retrieved): 55% (696) had images, totaling 7981 images; 139 were mainly images (≥ 3 images, fewer than 100 characters of text per image). Only 1 had fewer than 80 characters of text and otherwise only images.
- On 2026-10-20, Steam's public API retrieved the 127 locally installed "Vanilla … Expanded" entries. The largest author account had 96, **all with 3 or more images, median 27**. After removing shared paragraphs, median unique text was about 2700 characters. Images sit between paragraphs and appear to be section titles and feature showcases (not verified image by image).

OCR therefore reads the titles and showcased content in these images, rather than merely filling in descriptions without text. "Only 1 description uses images alone" counts only the image-only case and understates the role of images.

### 5.2 How

- **Use a vision model** through the model source in §3.1. Do not use Windows OCR: it requires WinRT calls and does not reliably recognize stylized lettering in Workshop banners.
- Use images from the local download cache (`<data directory>/cache/workshop-img/`, 16 §4.4). If absent, download them under the description-image rules first. Do not perform OCR when `workshop.images off`.
- Recognize only when the player clicks; do not preprocess the whole mod library.
- Cache results locally by image URL (location undecided, under the cache directory in 17 §3). Reuse results while the URL stays unchanged. Show a separate size and clear action under "caches" in settings.
- Display recognized text below the image in small, muted text, prefixed with the Archo portrait and "image text · inference". It is an inference, not the Mod's own declaration (00 §4 item 2).
- Obtain consent for the "images in descriptions" category before sending (§3.2).

### 5.3 Undecided

- Whether recognized text participates in mod library searches.
- Whether to pass recognized text to the translation task as well.

## 6. Relationship with TUI and MCP

| | Archo (built into GUI) | External agent (`archo mcp`) | TUI |
| --- | --- | --- | --- |
| Tools | Tool layer from 13 §1 | Same set (14 §2) | No Archo panel (not yet supported) |
| Permissions | Three levels in §2.2; RimArcho collects confirmation cards | External harness decides from annotations | — |
| Model | Player's API key, or local Claude Code / Codex | Agent's own | — |
| Consent | By task type (§3.2) | Agent's responsibility | — |

- The TUI is the keyboard version of the plan editor (16 §9.1). It has no Archo panel or "Ask Archo to …" buttons for now. Future support is undecided.
- External agents can do as much as Archo: Archo has no private tools. This keeps MCP a first-class interface. External-agent acceptance (I4 in 13 §6) also verifies whether the tools available to Archo are sufficient.

## 7. Acceptance

All unimplemented.

| # | Scenario | Expected |
| --- | --- | --- |
| AR1 | Disable Archo in settings | Sidebar entry and all "Ask Archo to …" buttons disappear; other features are unaffected |
| AR2 | Click Archo without a configured model source | Panel shows an explanation and "go to settings", without an error |
| AR3 | Archo disables a Mod in chat | Plan undo history gains "Archo: …"; `undo` reverses it |
| AR4 | Archo requests saving the list | Only a confirmation card appears; ModsConfig.xml stays unchanged until the player clicks "confirm" |
| AR5 | Tool layer directly receives Archo calls for `save_loadout`, recycle bin moves or backups | All return pending confirmation without writing to disk |
| AR6 | First use of translation | Consent explanation appears first; "disagree" sends nothing |
| AR7 | Reset translation to not agreed in settings | Next translation asks again |
| AR8 | Recognize the same image twice | Second call reads the cache without calling the model |
| AR9 | Archo and an external agent call the same read-only tool | Returned text is identical, character for character |
| AR10 | Send a log excerpt | Username in user directory paths becomes `<user>` |
