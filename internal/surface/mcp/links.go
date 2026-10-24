package mcp

type LinksCheckIn struct {
	Env string `json:"env,omitempty"`
	All bool   `json:"all,omitempty" jsonschema:"inspect every known game"`
}
type LinksRepairIn struct {
	Env     string   `json:"env,omitempty"`
	All     bool     `json:"all,omitempty"`
	Confirm bool     `json:"confirm" jsonschema:"explicit player confirmation to remove the previewed links"`
	DryRun  bool     `json:"dry_run,omitempty"`
	Include []string `json:"include,omitempty" jsonschema:"full paths of unproven prefixed links explicitly included by the player"`
}

func LinksRepairArgv(in LinksRepairIn) []string {
	b := argv("links", "repair").flag("all", in.All).flag("json", true)
	if in.DryRun {
		b.flag("dry-run", true)
	} else if in.Confirm {
		b.flag("yes", true)
	}
	for _, p := range in.Include {
		b.str("include", p)
	}
	return b.a
}
