package mcp

type MemoryIn struct {
	Env  string `json:"env,omitempty" jsonschema:"environment name (default: current)"`
	Plan string `json:"plan,omitempty" jsonschema:"plan name or ID (default: current)"`
}

func MemoryArgv(in MemoryIn) []string { return argv("plan", "cost").pos(in.Plan).a }
