package mcp

type VersionMuteIn struct {
	Env         string  `json:"env,omitempty" jsonschema:"environment (default: current)"`
	Refs        StrList `json:"refs" jsonschema:"mods whose version warnings to ignore or restore; pkg: references accepted"`
	GameVersion string  `json:"game_version,omitempty" jsonschema:"game major.minor, default the current game's"`
	Undo        bool    `json:"undo,omitempty" jsonschema:"restore warnings instead of ignoring them"`
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
