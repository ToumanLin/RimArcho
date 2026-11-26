package view

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/feedback"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// A translation fix (16 §2.14 "Translation fix"): the codes typed pick their entries from the
// window's reference numbers (ui.gui.refnums), and only those go with the report, the home
// folder scrubbed; a code never numbered here says so. The interface language is a basic line.
// In Korean and Russian the kind says Claude translated the language.
func TestFeedbackTranslationFix(t *testing.T) {
	SetLang("en")
	t.Cleanup(func() { SetLang("en") })
	e := testkit.New(t)
	home, _ := os.UserHomeDir()
	book := `{"hom":{"next":30,"seen":{"a":[22,"#screen .page.home .hm-hero > button","launch"],"b":[7,"#screen .hm-facts > span","` + strings.ReplaceAll(filepath.Join(home, "Saves", "Colony"), `\`, `\\`) + `"]}},"lib":{"next":2,"seen":{"c":[1,"#screen .lib > input","search"]}}}`
	if err := e.App.Store.SetMeta("ui.gui.refnums", book); err != nil {
		t.Fatal(err)
	}
	p := FeedbackProblem{Kind: feedback.KindL10n, Answers: map[string]string{feedback.L10nQuestion: "hom22 → start\nhom7 → x\nset99 → y"}}
	o := p.Options()
	if strings.Join(o.Refs, ",") != "hom22,hom7,set99" {
		t.Fatalf("refs %v", o.Refs)
	}
	at := FeedbackAttachments(e.App, nil, o, nil)
	text := FeedbackPreview(p.Text(), at)
	for _, want := range []string{"interface language: en", "reference numbers (3)", `hom22  #screen .page.home .hm-hero > button  "launch"`, `%USERPROFILE%`, "set99  not numbered on this computer", "report: translation fix"} {
		if !strings.Contains(text, want) {
			t.Errorf("preview lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "search") || (home != "" && strings.Contains(text, home)) {
		t.Errorf("only the codes named, scrubbed:\n%s", text)
	}
	// nothing named: no section
	if at := FeedbackAttachments(e.App, nil, FeedbackProblem{Kind: feedback.KindL10n}.Options(), nil); len(at.Sections) != 0 {
		t.Errorf("sections without codes: %+v", at.Sections)
	}

	lead := func() (string, string) {
		for _, tp := range FeedbackTemplates() {
			if tp.Kind == feedback.KindL10n {
				return tp.Lead, tp.Note
			}
		}
		return "", ""
	}
	if l, _ := lead(); l != "wording that reads oddly in your language" {
		t.Errorf("en lead %q", l)
	}
	for _, lang := range []string{"ko", "ru"} {
		SetLang(lang)
		l, n := lead()
		if l == T("wording that reads oddly in your language") || !strings.Contains(n, "Claude") || !strings.Contains(n, `ctrl+\`) {
			t.Errorf("%s: lead %q note %q", lang, l, n)
		}
	}
}
