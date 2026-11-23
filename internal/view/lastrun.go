package view

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
)

// Last-run summaries use the observed class, never a phase inferred from that class.
var logKindWords = map[string]string{
	"texture-missing": "missing textures or sounds",
	"material":        "material errors",
	"xref-missing":    "missing references",
	"xml-load":        "content loading errors",
	"patch-failed":    "failed patches",
	"save-load":       "save data errors",
	"game-error":      "game errors naming mod code",
	"exception":       "exceptions",
	"harmony-stack":   "errors in patched methods",
}

var logResourceRe = regexp.MustCompile(`(?:at|Texture2D at) ['"]([^'"\r\n]+)['"]`)

func lastRunNeed(name string, ps []*app.Problem, when string, attributed bool) Need {
	type kind struct {
		code     string
		n        int
		example  string
		textures bool
	}
	byKind := map[string]*kind{}
	errs, warns, uncertain := 0, 0, false
	for _, p := range ps {
		n := logCount(p)
		if p.Severity == finding.Error {
			errs += n
		} else {
			warns += n
		}
		uncertain = uncertain || p.Confidence == finding.Possible
		code := strings.TrimPrefix(p.Code, "log.")
		k := byKind[code]
		if k == nil {
			k = &kind{code: code, textures: true}
			byKind[code] = k
		}
		k.n += n
		k.textures = k.textures && (strings.Contains(p.Claim, "Texture2D") || strings.Contains(p.Claim, "textures at"))
		if code == "texture-missing" && k.example == "" {
			if m := logResourceRe.FindStringSubmatch(p.Claim); m != nil {
				k.example = m[1]
			}
		}
	}
	counts := []string{}
	if errs == 1 {
		counts = append(counts, T("1 error"))
	} else if errs > 1 {
		counts = append(counts, Tf("%d errors", errs))
	}
	if warns == 1 {
		counts = append(counts, T("1 warning"))
	} else if warns > 1 {
		counts = append(counts, Tf("%d warnings", warns))
	}
	title := name + " — " + strings.Join(counts, T(", "))
	ks := make([]*kind, 0, len(byKind))
	for _, k := range byKind {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool {
		if ks[i].n != ks[j].n {
			return ks[i].n > ks[j].n
		}
		return ks[i].code < ks[j].code
	})
	var parts []string
	kinds := &CountBar{}
	for i, k := range ks {
		if i == 2 {
			n := 0
			for _, rest := range ks[i:] {
				n += rest.n
			}
			parts = append(parts, Tf("%d other messages", n))
			kinds.Parts = append(kinds.Parts, BarCount{Kind: "other", N: n})
			kinds.N += n
			break
		}
		kinds.Parts = append(kinds.Parts, BarCount{Kind: k.code, N: k.n})
		kinds.N += k.n
		word := logKindWords[k.code]
		if k.code == "texture-missing" && k.textures {
			word = "missing textures"
		}
		if word == "" {
			word = "other log errors"
		}
		part := T(word)
		if len(ks) > 1 {
			part = Tf("%s ×%d", part, k.n)
		}
		if k.example != "" {
			part = Tf("%s (%s)", part, k.example)
		}
		parts = append(parts, part)
	}
	why := strings.Join(parts, T("; "))
	if attributed && uncertain {
		why += T("; source not confirmed")
	}
	act := Action{Kind: ActGameLog, Key: "enter", Label: T("open game log")}
	if len(ps) > 0 && ps[0].Log != nil {
		act.LogPath = ps[0].Log.Path
	}
	if attributed {
		if m := logIDRe.FindStringSubmatch(ps[0].Claim); m != nil {
			act.Cluster = m[1]
		}
	}
	ev := logEvidence(ps, when)
	// The title already gives the occurrence kinds; the chip names only the source.
	if strings.HasPrefix(ev.Source, "Player.log") {
		ev.Source = filepath.Base(act.LogPath)
		if ev.Source == "." {
			ev.Source = "Player.log"
		}
		if when != "" {
			ev.Source += "  " + when
		}
	}
	kinds.Text = why
	return Need{Group: GroupLastRun, Title: title, Text: title + T(": ") + why, Why: why, Evidence: ev,
		Counted: true, Errors: errs, Status: StatusLog, Actions: []Action{act}, Counts: kinds}
}

// CountBar is a count drawn as a bar (16 §7 principle 9): N in all, split into Parts by
// kind, largest first. Text is the sentence it stands for, its accessible name and tooltip.
// The GUI scales the bars of one list to its largest.
type CountBar struct {
	N     int        `json:"n"`
	Parts []BarCount `json:"parts"`
	Text  string     `json:"text"`
}

type BarCount struct {
	Kind string `json:"kind"`
	N    int    `json:"n"`
}
