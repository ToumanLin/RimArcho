# Post-MVP backlog

- ~~IL scan for manual `Harmony.Patch` targets~~ — done (corpus G9, `internal/harmony/manual.go`; decisions-corpus §2).
  Left: originals chosen at runtime (game-wide method scans such as HAR's `BodyReferenceTranspiler`,
  collections built by helpers, `Assembly.GetType`), conditional compat patches stored as facts (static-only
  facts 20.7% → 22.8%), and a base-class method named through a subclass (the header shows the declaring type).
- AllComps mutation audit and icon/material audit analyzers (01 §3.2).
- Static cross-reference checking via Def field types from Assembly-CSharp metadata (03 §4.3 note).
- GUI surface (D3 phase 2).
