package mcp

import "fmt"

// 贴图优化 (08 §5.2). optimize writes into mods' folders and clean deletes files there (only
// RimArcho's own DDS), so the tool is marked destructive. Downloading the encoder is not a
// tool: RimArcho only goes online when the player asks (01 §4).
func textureTools() []tool {
	return []tool{
		reg("manage_textures", false, true, descTextures, TexturesArgv),
	}
}

// TexturesIn is manage_textures.
type TexturesIn struct {
	Env    string   `json:"env,omitempty" jsonschema:"another environment by name (default: the current one)"`
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

const descTextures = `Texture optimisation (贴图优化): convert mods' PNG textures to DDS, written next to the PNGs in the mods' folders. The game (1.6 and 1.7) loads x.dds in place of x.png: no PNG to decode at start, and opaque textures take half the video memory (BC1; the game itself makes BC7 or DXT5 of a PNG); scale shrinks them further (never below 128 px; masks stay full size). The PNGs are never touched. action=status (read-only) lists each mod's PNGs, the DDS RimArcho made (stale ones: the mod updated since, the game may show the old texture — also problem texture.stale), DDS of the mod's own, and the estimate (files, DDS size, video memory before → after). action=optimize converts (dry_run first; refused while a game runs; needs the encoder todds, which the player installs with archo textures tool --install or in the window — tell them, do not try to download it). action=clean deletes only the DDS RimArcho wrote. Packs convert their own copies instead: manage_packs build with --textures. Use it to answer "can I make loading or video memory lighter?" and to fix texture.stale.
Example: manage_textures {"action":"optimize","plan":true,"dry_run":true}
  textures optimize: dry run, nothing written (pre-convert)
    encoder: todds 0.4.1 (C:\Users\me\AppData\Local\RimArcho\todds\todds.exe)
    convert: 13208 PNGs (412.0 MB) → about 2.3 GB of DDS; video memory about 3.1 GB → 2.1 GB
    left alone: 120 PNGs that share a name with a DDS of the mod's own or another format
    writes: a .dds next to each PNG, in the mods' folders; the PNGs stay; archo textures clean takes them back
  next:
    archo textures optimize --plan`
