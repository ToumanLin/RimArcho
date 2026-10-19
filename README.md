# RimArcho

A RimWorld mod manager for players with hundreds of mods, and for the AI agents that help them.
One `archo.exe` gives you a terminal UI, a scriptable CLI and an MCP server. All three print the
same plain-text outlines, and every reference in them (`#id`, `pkg:`, `p:…`, `log:E3`) can be pasted
into another command.

What it knows about your installation:

- **Identity.** Every mod folder (Workshop, local `Mods/`, SteamCMD, extra folders) is an instance.
  Copies of one mod form a lineage. Changes between scans are recorded: updated, moved, copied,
  reappeared, gone.
- **Loadout.** It edits a workspace copy of the enabled list. Every edit can be undone, sorting
  explains its constraints, and nothing reaches the game until you `save` (a backup is taken and the
  file is replaced atomically).
- **Problems.** It checks dependencies, incompatibilities and load order. It merges and inherits Defs
  the way the game does and simulates PatchOperations, so a failure comes with a reason
  (`target-inactive`, `stale-name`, `node-absent`…). It reads Harmony patches straight from the mods'
  assemblies.
- **Logs.** It attributes Player.log errors to mods. Each mod gets a role (source, patcher,
  dependency or mentioned) together with the evidence, the file and line, and the next step.
- **Code.** It decompiles assemblies on demand with ilspycmd and caches the result per DLL hash.

RimArcho never writes to the game, Workshop or `Mods/` folders, with two exceptions that you ask for
yourself: `save` (ModsConfig.xml) and `copy-local` (a new folder in `Mods/`). Its own data lives in
`%APPDATA%\RimArcho\`.

## Install

1. Download `archo-<version>-windows-amd64.zip` and unzip it anywhere, for example `C:\Tools\RimArcho\`.
2. Optionally, add that folder to `PATH`.
3. Optionally, install ilspycmd, which `archo source` uses for C# code. It needs the .NET SDK:
   ```
   dotnet tool install --global ilspycmd
   ```
   RimArcho never installs it for you; it tells you when it is missing.

To build from source (Go 1.27+), run `go build -o archo.exe ./cmd/archo`, or
`pwsh scripts/release.ps1 -Version x.y.z` for a release zip with the version stamped in.

## First run

```
archo env detect                 # finds the game, the user data folder and the Workshop folder
archo env add main --detected    # registers them as environment "main"
archo scan                       # reads every mod folder (incremental afterwards)
archo                            # opens the TUI
```

Useful next steps:

```
archo status                       # overview: mods, Loadout, problem counts
archo problems                     # what is wrong, most severe first
archo log                          # attribute the newest Player.log
archo import rimcrow --db "D:\Game\RimModManager\data\mod_manager.db" --dry-run   # coming from RimCrow
```

`archo help` lists every command. `archo <command> --help` shows its flags.

## TUI keys

| Keys | List view |
|---|---|
| `↑/↓` `j/k`, `PgUp/PgDn`, `g/G` | move, page, ends |
| `space` | enable / disable |
| `J` / `K`, `m` | move down / up, move to … |
| `v` | mark (multi-select) |
| `S` / `w` | sort / save to the game (each shows the diff first) |
| `u` / `ctrl+r` | undo / redo |
| `x` | swap to another instance of the same lineage |
| `enter` | inspect |
| `r` | add a rule for this mod |
| `/`, `f` | search, filter (all / enabled / disabled / unloadable) |
| `R` | rescan (child process) |
| `l` | analyse Player.log (or the previous one) |
| `L` | launch the game |

| Keys | Problems view (`2`) |
|---|---|
| `enter` | expand / inspect |
| `a` | run the suggested command (`tab` picks one) |
| `i` / `I` | ignore / show ignored |
| `s` | severity filter |
| `o` | inspect the mod |
| `A` | re-analyse (child process) |

| Keys | Changes view (`3`) |
|---|---|
| `enter` | file-level diff of an event |
| `s` / `c` / `u` | answer an identity question: same / copy / unrelated |
| `L` / `I` | load / ignore a ModsConfig.xml that changed outside RimArcho |

Everywhere: `1` `2` `3` switch views, `E` switches environment, `:` runs any archo command, `?` opens
help and `q` quits. The mouse wheel scrolls and a click selects (`archo config set ui.mouse off`
turns the mouse off). The language follows the system; change it with
`archo config set ui.lang zh|en|auto`.

## Connecting an agent (`archo mcp`)

`archo mcp` serves the RimArcho tools over stdio:

- read-only: `status`, `find_mods`, `get_mod`, `get_def`, `get_patches`, `get_harmony`,
  `explain_relation`, `list_problems`, `list_changes`, `analyze_log`, `search_files`, `read_source`
- writing: `edit_loadout`, `copy_local`, `add_rule`, and `save_loadout`, which is the only tool marked
  destructive

Each tool returns exactly what the matching CLI command prints. The tool descriptions and the server
instructions explain the diagnosis workflow, so the agent needs no extra skill. Read-only tools are
annotated `readOnlyHint`, which lets the client auto-approve them.

**Claude Code**, from a terminal:

```
claude mcp add rimarcho -- C:\Tools\RimArcho\archo.exe mcp
```

Or in a project's `.mcp.json`:

```json
{
  "mcpServers": {
    "rimarcho": { "command": "C:\\Tools\\RimArcho\\archo.exe", "args": ["mcp"] }
  }
}
```

**Codex**, in `~/.codex/config.toml`:

```toml
[mcp_servers.rimarcho]
command = 'C:\Tools\RimArcho\archo.exe'
args = ["mcp"]
```

Then ask, for example: "My log has `Patch operation … failed` errors, which mod is wrong?" or
paste a stack trace. To use another environment, a tool call can pass `env`. To analyse as another
game version, it can pass `game_version`.

## Where things are

| | |
|---|---|
| RimArcho data | `%APPDATA%\RimArcho\` (database, Loadout history, decompile cache) |
| Decompiled code | `%APPDATA%\RimArcho\cache\source\<dll hash>\` |
| Game file it writes | `<user data>\Config\ModsConfig.xml`, only on `save`; the previous file is kept next to it as a backup |

## Licence

See the repository.

## License

MIT. See [LICENSE](LICENSE).
