package view

import (
	"regexp"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
)

// 能不能跑's sections and evidence (06 §2.4): the items are shown in four sections by
// consequence, and each says where its claim comes from.

// Sections, in screen order: what breaks the game first.
const (
	SectionBreak   = iota // 会让游戏出错: the list's problems, dependencies, conflicts
	SectionRun            // 上次运行的报错: this run (while the game runs), then the last run
	SectionConfirm        // 要你确认: identity questions, outside changes of the list
	SectionHidden         // 隐藏的 Mod: pinned copies with upstream updates, hidden mods
)

// SectionTitles are the sections' headings (English; T translates them).
var SectionTitles = []string{"breaks the game", "errors from the last run", "for you to confirm", "hidden mods"}

// SectionOf is the section of an item's group.
func SectionOf(group int) int {
	switch group {
	case GroupLastRun, GroupThisRun:
		return SectionRun
	case GroupConfirm:
		return SectionConfirm
	case GroupHidden:
		return SectionHidden
	}
	return SectionBreak
}

// Evidence kinds: the mod's own declaration, RimArcho's analysis, the game's log, the
// community's data (04 §6), or a rule the player added.
const (
	EvDeclared  = "declared"
	EvInferred  = "inferred"
	EvLog       = "log"
	EvCommunity = "community"
	EvRule      = "rule"
)

var evidenceLabels = map[string]string{
	EvDeclared:  "declared by the mod",
	EvInferred:  "inferred",
	EvLog:       "log",
	EvCommunity: "community data",
	EvRule:      "your rule",
}

// Evidence is the chip on an item: its kind in words and the concrete source
// ("About.xml modDependencies", "log:E3 · 10 月 19 日 21:04").
type Evidence struct {
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	Source string `json:"source,omitempty"`
}

func evidence(kind, source string) *Evidence {
	return &Evidence{Kind: kind, Label: T(evidenceLabels[kind]), Source: source}
}

// evidenceKind is the kind of a finding's first evidence, else def.
func evidenceKind(p *app.Problem, def string) string {
	for _, e := range p.Evidence {
		switch e.Source {
		case finding.Declared:
			return EvDeclared
		case finding.Inferred:
			return EvInferred
		case finding.Log:
			return EvLog
		case finding.Community:
			return EvCommunity
		case finding.User:
			return EvRule
		}
	}
	return def
}

var (
	fileLineRe = regexp.MustCompile(`^(\S+:\d+)\s`)
	parentRe   = regexp.MustCompile(`^\S+ (\S+) inherits ParentName="([^"]+)"`)
	orderEvRe  = regexp.MustCompile(`^decl \S+ About\.xml (\S+) `)
)

// evidenceFor is the chip of a finding the list's sections show.
func evidenceFor(p *app.Problem) *Evidence {
	switch p.Code {
	case "dep.missing", "dep.inactive":
		return evidence(EvDeclared, T("About.xml modDependencies"))
	case "incompat.active":
		return evidence(EvDeclared, T("About.xml incompatibleWith"))
	case "order.violated":
		kind := evidenceKind(p, EvDeclared)
		switch kind {
		case EvCommunity:
			return evidence(kind, T("community load-order rules"))
		case EvRule:
			return evidence(kind, T("a load-order rule you added"))
		}
		field := "loadAfter"
		if len(p.Evidence) > 0 {
			if m := orderEvRe.FindStringSubmatch(p.Evidence[0].Text); m != nil {
				field = m[1]
			}
		}
		return evidence(kind, Tf("About.xml %s", field))
	case "order.cycle":
		return evidence(EvDeclared, T("the mods' load-order requirements"))
	case "official.order":
		return evidence(EvInferred, T("the game loads Core and the DLCs first"))
	case "pkg.duplicate-active":
		return evidence(EvInferred, T("two folders have the same package id"))
	case "entry.missing":
		return evidence(EvInferred, T("no mod folder has this package id"))
	case "mod.replaced":
		return evidence(EvCommunity, T("the community's replacement list"))
	case "mod.unloadable":
		return evidence(EvInferred, T("RimArcho read its folder"))
	case "patch.no-match":
		src := T("a patch matched nothing")
		if m := fileLineRe.FindStringSubmatch(p.Claim); m != nil {
			src = Tf("patch %s matched nothing", m[1])
		}
		return evidence(EvInferred, src)
	case "def.parent-missing":
		src := T("an inherited definition is missing")
		if m := parentRe.FindStringSubmatch(p.Claim); m != nil {
			src = Tf("%s inherits %s, which no enabled mod defines", m[1], m[2])
		}
		return evidence(EvInferred, src)
	case "mod.pinned":
		return evidence(EvInferred, T("the Workshop copy changed after the copy was made"))
	case "mod.hidden-residue":
		return evidence(EvInferred, T("About.xml.disabled, and no longer subscribed"))
	case "mod.hidden":
		return evidence(EvInferred, T("About.xml is renamed to About.xml.disabled"))
	case "texture.stale":
		return evidence(EvInferred, T("the mod's files changed after RimArcho wrote its DDS"))
	}
	return nil
}

var (
	logIDRe = regexp.MustCompile(`^log:(\S+) x\d+:`)
	// a file the companion log tied to the error (03 §6.7.4): "log file (ArchoMonitor.log): Defs/X.xml"
	companionFileRe = regexp.MustCompile(`log file \(([^)]+\.log)\): (\S+)`)
)

// companionChips are the files the companion log names in evidence texts, as
// "ArchoMonitor.log · Defs/X.xml".
func companionChips(texts []string) []string {
	var out []string
	for _, t := range texts {
		if m := companionFileRe.FindStringSubmatch(t); m != nil {
			c := m[1] + " · " + m[2]
			dup := false
			for _, o := range out {
				dup = dup || o == c
			}
			if !dup {
				out = append(out, c)
			}
		}
	}
	return out
}

// logEvidence: the clusters of a mod's errors in the last log and when it was written; a
// file from the companion log when it names one (03 §6.7.4).
func logEvidence(ps []*app.Problem, when string) *Evidence {
	var ids, texts []string
	for _, p := range ps {
		if m := logIDRe.FindStringSubmatch(p.Claim); m != nil {
			ids = append(ids, "log:"+m[1])
		}
		for _, e := range p.Evidence {
			texts = append(texts, e.Text)
		}
	}
	src := "Player.log"
	switch {
	case len(ids) == 1:
		src = ids[0]
	case len(ids) > 1:
		src = Tf("%s and %d more", ids[0], len(ids)-1)
	}
	switch files := companionChips(texts); {
	case len(files) == 1:
		src = files[0]
	case len(files) > 1:
		src = Tf("%s and %d more", files[0], len(files)-1)
	}
	if when != "" {
		src += " · " + when
	}
	return evidence(EvLog, src)
}
