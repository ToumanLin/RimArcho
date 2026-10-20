# Post-MVP backlog

- IL scan for manual `Harmony.Patch` targets (`ldtoken` / `AccessTools.Method("X")` near the call) — M4 open question 5.
- AllComps mutation audit and icon/material audit analyzers (01 §3.2).
- Static cross-reference checking via Def field types from Assembly-CSharp metadata (03 §4.3 note).
- GUI surface (D3 phase 2).
- **1.7 Player.log format (Waylith runtime) in `internal/logs`.** Found by the log corpus P0
  (`spikes/hugslogs`): 1.7 lines carry `<date> <time> <LEVEL> [<category>] <message>`, message
  continuation lines can start at column 0, every entry (warnings too) has a stack whose leading frames
  are the logging system (`Brutal.Logging.*`, `Verse.Log.*`), and the `RimWorld x.y.z revN` line comes
  after line 50, so `Parse` finds no version. Effects: INFO lines become entries, prefix-based classes
  (`Patch operation …`) miss, and warnings classify as `exception`. `spikes/hugslogs` `fromWaylith`
  shows a working conversion; the product parser should read the level directly instead.
