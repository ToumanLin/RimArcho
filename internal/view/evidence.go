package view

import (
	"regexp"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/known"
)

// Can it run's sections and evidence (16 §2.4): the items are shown in sections, and each says
// where its claim comes from.

// Sections, in screen order. The GUI's left column holds the first three (what checking
// the files and the shared logs found), its right column the run's errors; the TUI lists
// them in the same order.
const (
	SectionBreak     = iota // Will cause errors: the list's problems, dependencies, conflicts
	SectionHint             // worth knowing: what shared logs taught (EvCorpus); not counted
	SectionConfirm          // for you to confirm: identity questions, outside changes of the list
	SectionRun              // errors from the last run: this run (while the game runs), then the last run
	SectionElsewhere        // not on the page: Mod library or Texture optimisation handles it (Needs.Elsewhere)
)

// SectionTitles are the sections' headings (English; T translates them).
var SectionTitles = []string{"breaks the game", "worth knowing", "for you to confirm", "errors from the last run", "elsewhere"}

// SectionOf is the section of an item's group. Shared-log hints move to SectionHint by
// their evidence (BuildNeeds), not by their group.
func SectionOf(group int) int {
	switch group {
	case GroupLastRun, GroupThisRun:
		return SectionRun
	case GroupConfirm:
		return SectionConfirm
	case GroupHidden:
		return SectionElsewhere
	}
	return SectionBreak
}

// Evidence kinds: the mod's own declaration, RimArcho's analysis, the game's log, the
// community's data (08 §2), a rule the player added, or what shared logs showed (the log
// corpus, 05 §4.3: the source carries the measured share).
const (
	EvDeclared  = "declared"
	EvInferred  = "inferred"
	EvLog       = "log"
	EvCommunity = "community"
	EvRule      = "rule"
	EvCorpus    = "corpus"
)

var evidenceLabels = map[string]string{
	EvDeclared:  "declared by the mod",
	EvInferred:  "inferred",
	EvLog:       "log",
	EvCommunity: "community data",
	EvRule:      "your rule",
	EvCorpus:    "shared logs",
}

// Evidence is the chip on an item: its kind in words and the concrete source
// ("About.xml modDependencies", "log:E3 · Oct 19 21:04").
type Evidence struct {
	Cutoff string `json:"cutoff,omitempty"`
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	Source string `json:"source,omitempty"`
}

func evidence(kind, source string) *Evidence {
	e := &Evidence{Kind: kind, Label: T(evidenceLabels[kind]), Source: source}
	if kind == EvCorpus {
		e.Cutoff = known.Get().Cutoff
		if e.Cutoff != "" {
			e.Label = Tf("%s (up to %s)", e.Label, e.Cutoff)
		}
	}
	return e
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
		switch field {
		case "modDependencies", "modDependenciesByVersion":
			return evidence(kind, T("About.xml modDependencies"))
		case "loadBefore", "forceLoadBefore":
			return evidence(kind, T("the author's requirement to load before another mod"))
		default:
			return evidence(kind, T("the author's requirement to load after another mod"))
		}
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
			src = Tf("%s cannot load: its base content %s is missing", m[1], m[2])
		}
		return evidence(EvInferred, src)
	case "mod.pinned":
		return evidence(EvInferred, T("the Workshop copy changed after it was locked"))
	case "mod.hidden-residue":
		return evidence(EvInferred, T("About.xml.disabled, and no longer subscribed"))
	case "mod.hidden":
		return evidence(EvInferred, T("About.xml is renamed to About.xml.disabled"))
	case "texture.stale":
		return evidence(EvInferred, T("the mod's files changed after RimArcho wrote its DDS"))
	case "dep.observed-missing", "combo.known-bad", "mod.known-error":
		return corpusEvidence(p)
	}
	return nil
}

// The log corpus's counts (internal/app/known.go), for the chip's source.
var (
	corpusPrereqRe  = regexp.MustCompile(`^log corpus: (\d+) of (\d+) shared logs with `)
	corpusPairRe    = regexp.MustCompile(`^log corpus: with \S+ and \S+ (\d+) of (\d+) shared logs .*; with \S+ alone (\d+) of (\d+)`)
	corpusVersionRe = regexp.MustCompile(`^log corpus: \S+ (\S+) in (\d+) of (\d+) shared logs .*; other versions (\d+) of (\d+)`)
	corpusOwnRe     = regexp.MustCompile(`^log corpus: (\d+) of (\d+) shared logs that enable `)
)

// corpusEvidence is the community logs chip: what share of the shared logs showed the error.
func corpusEvidence(p *app.Problem) *Evidence {
	share := func(a, b string) string {
		x, y := atoi(a), atoi(b)
		if y == 0 {
			return a + "/" + b
		}
		return Tf("%d of %d (%d%%)", x, y, (100*x+y/2)/y)
	}
	for _, e := range p.Evidence {
		if m := corpusPairRe.FindStringSubmatch(e.Text); m != nil {
			return evidence(EvCorpus, Tf("errors with both in %s of shared logs, alone in %s", share(m[1], m[2]), share(m[3], m[4])))
		}
		if m := corpusVersionRe.FindStringSubmatch(e.Text); m != nil {
			return evidence(EvCorpus, Tf("version %s errors in %s of shared logs, other versions in %s", m[1], share(m[2], m[3]), share(m[4], m[5])))
		}
		if m := corpusPrereqRe.FindStringSubmatch(e.Text); m != nil {
			return evidence(EvCorpus, Tf("errors without it in %s of shared logs", share(m[1], m[2])))
		}
		if m := corpusOwnRe.FindStringSubmatch(e.Text); m != nil {
			return evidence(EvCorpus, Tf("errors in %s of the shared logs that enable it", share(m[1], m[2])))
		}
	}
	return evidence(EvCorpus, T("seen in shared logs"))
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int(c-'0')
	}
	return n
}

var (
	logIDRe = regexp.MustCompile(`^log:(\S+) x\d+:`)
	// a file the companion log tied to the error (06 §7): "log file (ArchoMonitor.log): Defs/X.xml"
	companionFileRe = regexp.MustCompile(`log file \(([^)]+\.log)\): (\S+)`)
)

// companionChips are the files the companion log names in evidence texts, as
// "file Defs/X.xml  ArchoMonitor.log": the mod's file labelled as one, then the log that
// named it; two spaces part facts (16 §7.3, the GUI shows them as a gap).
func companionChips(texts []string) []string {
	var out []string
	for _, t := range texts {
		if m := companionFileRe.FindStringSubmatch(t); m != nil {
			c := Tf("file %s", m[2]) + "  " + m[1]
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
// file from the companion log when it names one (06 §7).
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
	if len(ids) > 0 {
		src = Tf("Player.log, %d error groups", len(ids))
	}
	switch files := companionChips(texts); {
	case len(files) == 1:
		src = files[0]
	case len(files) > 1:
		// "file a.xml, and 1 more  ArchoMonitor.log"
		f, log, _ := strings.Cut(files[0], "  ")
		src = Tf("%s and %d more", f, len(files)-1) + "  " + log
	}
	if when != "" {
		src += "  " + when
	}
	return evidence(EvLog, src)
}
