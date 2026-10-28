package mcp

const descVersionMute = `Ignore or restore a mod's unsupported-game-version warnings after the player chooses to do so. refs are mod references (package references also work after removal); game_version is a major.minor such as 1.7, defaulting to the environment's. undo=true restores warnings. Applies to the normalized package id in all plans and environments, survives updates and copies, and does not affect 1.8 when ignoring 1.7 warnings. Writes only RimArcho's database. Ask the player before ignoring warnings for a batch. list_problems skips these by default and reports the ignored count; include_muted=true shows them.
Example: mute_version_warnings {"refs":["pkg:example.mod"],"game_version":"1.7"}
→ version warnings ignored for 1.7`

type VersionMuteIn struct {
	Env         string   `json:"env,omitempty" jsonschema:"another environment by name"`
	Refs        []string `json:"refs" jsonschema:"mods whose version warnings to ignore or restore; pkg: references accepted"`
	GameVersion string   `json:"game_version,omitempty" jsonschema:"game major.minor, default the current game's"`
	Undo        bool     `json:"undo,omitempty" jsonschema:"restore warnings instead of ignoring them"`
}

func VersionMuteArgv(in VersionMuteIn) []string {
	verb := "mute-version"
	if in.Undo {
		verb = "unmute-version"
	}
	b := argv("problems", verb)
	for _, ref := range in.Refs {
		b.pos(ref)
	}
	return b.str("game-version", in.GameVersion).a
}
