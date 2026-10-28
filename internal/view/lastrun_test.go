package view

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
)

func runProblem(code, severity, claim, confidence string) *app.Problem {
	return &app.Problem{Finding: &finding.Finding{Code: "log." + code, Severity: severity, Claim: claim, Confidence: confidence}, Log: &app.LogRun{Path: "synthetic/Player-prev.log"}}
}

func TestLastRunSummary(t *testing.T) {
	defer SetLang("en")
	for _, lang := range []string{"en", "zh"} {
		SetLang(lang)
		p := runProblem("texture-missing", finding.Warning, "log:E209 x8: Could not load Texture2D at 'Things/Building/ModCompat/Empty' for def 'Example'", finding.Likely)
		n := lastRunNeed("Alpha Props", []*app.Problem{p}, "10:00", true)
		title, why := "Alpha Props — 8 warnings", "missing textures (Things/Building/ModCompat/Empty)"
		if lang == "zh" {
			title, why = "Alpha Props — 8 个警告", "贴图缺失（Things/Building/ModCompat/Empty）"
		}
		if n.Title != title || n.Why != why {
			t.Fatalf("%s: %+v", lang, n)
		}
		if strings.Count(n.Text, "Alpha Props") != 1 || strings.Contains(n.Evidence.Source, "8") {
			t.Fatalf("repeated facts: %+v", n)
		}
		a := n.Actions[0]
		if a.Kind != ActGameLog || a.Cluster != "E209" || a.LogPath != p.Log.Path {
			t.Fatalf("action: %+v", a)
		}
	}
	SetLang("en")
	ps := []*app.Problem{
		runProblem("texture-missing", finding.Warning, "log:E1 x8: Could not load Texture2D at '纹理/Empty'", finding.Certain),
		runProblem("xref-missing", finding.Error, "log:E2 x3: Could not resolve cross-reference", finding.Possible),
		runProblem("xml-load", finding.Error, "log:E3 x2: XML error", finding.Likely),
	}
	n := lastRunNeed("Alpha", ps, "", true)
	if n.Title != "Alpha — 5 errors, 8 warnings" || n.Why != "missing textures ×8 (纹理/Empty); missing references ×3; 2 other messages; source not confirmed" {
		t.Fatalf("mixed: %+v", n)
	}
	n = lastRunNeed("unknown source", ps, "", false)
	if n.Actions[0].Cluster != "" || strings.Contains(n.Why, "source not confirmed") {
		t.Fatalf("unattributed: %+v", n)
	}
}
