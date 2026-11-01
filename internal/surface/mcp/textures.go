package mcp

import "fmt"

// Texture optimisation (08 §5.2). optimize writes into mods' folders and clean deletes files there (only
// RimArcho's own DDS), so the tool is marked destructive. Downloading the encoder is not a
// tool: RimArcho only goes online when the player asks (01 §4).
func textureTools() []tool {
	return []tool{
		reg("manage_textures", false, true, descTextures, TexturesArgv),
	}
}

// TexturesIn is manage_textures.
type TexturesIn struct {
	Env    string   `json:"env,omitempty" jsonschema:"environment name (default: current)"`
	Action string   `json:"action" jsonschema:"status, optimize or clean"`
	Refs   []string `json:"refs,omitempty" jsonschema:"the mods (#id, pkg:…); or plan / all"`
	Plan   bool     `json:"plan,omitempty" jsonschema:"the current plan's mods"`
	All    bool     `json:"all,omitempty" jsonschema:"every installed mod"`
	Scale  int      `json:"scale,omitempty" jsonschema:"status, optimize: scale textures down to this percent (20 25 40 50 60 75 80); 0 = full size"`
	Force  bool     `json:"force,omitempty" jsonschema:"optimize: convert again the textures that are up to date"`
	DryRun bool     `json:"dry_run,omitempty" jsonschema:"optimize, clean: show what would happen, change nothing"`
}

// TexturesArgv maps manage_textures to archo textures.
func TexturesArgv(in TexturesIn) []string {
	act := in.Action
	switch act {
	case "optimize", "clean", "status":
	default:
		act = "status"
	}
	b := argv("textures", act)
	for _, r := range in.Refs {
		b.pos(r)
	}
	b.flag("plan", in.Plan).flag("all", in.All)
	if in.Scale > 0 && in.Scale < 100 && act != "clean" {
		b.str("scale", fmt.Sprint(in.Scale))
	}
	if act == "optimize" {
		b.flag("force", in.Force)
	}
	if act != "status" {
		b.flag("dry-run", in.DryRun)
	}
	return b.a
}
