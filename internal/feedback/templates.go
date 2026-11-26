package feedback

import (
	"slices"
	"strings"
)

// Report kinds (16 §2.14 Feedback): each one asks its own questions and ticks its own
// attachments. The labels and questions are English here; the interface translates them, and
// the CLI prints them as they are (13 §1.1).
const (
	KindFeature   = "feature"
	KindBug       = "bug"
	KindDiagnosis = "diagnosis"
	KindData      = "data"
	KindOther     = "other"
	KindL10n      = "l10n" // a translation fix: element codes (ctrl+\) and better wording
)

// Question forms.
const (
	FormText    = "text"    // several lines
	FormLine    = "line"    // one line
	FormChoice  = "choice"  // one of Choices
	FormCheck   = "check"   // a box: "yes" when ticked
	FormFinding = "finding" // a Can it run item's ID, picked from the current ones
	FormMod     = "mod"     // a Mod's packageId or Workshop ID
	FormURL     = "url"     // one line, a link
	FormFixes   = "fixes"   // lines of "code → wording": a reference number (ctrl+\) and the fix
)

// Question is one thing a report kind asks.
type Question struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Hint     string   `json:"hint,omitempty"` // the field's placeholder
	Form     string   `json:"form"`
	Choices  []string `json:"choices,omitempty"`
	Optional bool     `json:"optional,omitempty"`
}

// Template is one report kind: its questions, the attachments ticked when it is chosen, and
// the attachments that come with it whatever is ticked (Auto: the basic lines always; the
// chosen diagnosis or Mod; the data versions).
type Template struct {
	Kind      string     `json:"kind"`
	Label     string     `json:"label"`
	Lead      string     `json:"lead"`           // one line under the name in the list of kinds
	Note      string     `json:"note,omitempty"` // a hint above the questions
	Questions []Question `json:"questions"`
	Attach    []string   `json:"attach,omitempty"` // optional attachments ticked at first
	Auto      []string   `json:"auto,omitempty"`   // what goes with it beyond the basic lines
}

// DataFinding is the answer key an outdated-data report carries when it starts from a Can it
// run item or a Mod's details: the item's ID, attached as the diagnosis section. It is not a
// question; the item is what showed the data.
const DataFinding = "finding"

// FilesQuestion is the bug kind's "files were changed or lost" box: ticked, the report
// carries the recent operations and the relay labels it priority.
const FilesQuestion = "files"

// L10nQuestion is the translation-fix kind's "code → wording" lines; the codes in it pick
// the reference-number entries that go with the report (RefCodes).
const L10nQuestion = "fixes"

// Machine-translated interface languages: the translation-fix kind's lead and note say so
// in them and ask for corrections (16 §2.14).
var MachineLangs = []string{"ko", "ru"}

// The translation-fix kind's lead and note in a machine-translated language.
const (
	L10nLeadMT = "Claude translated this language: fix wording that reads oddly"
	L10nNoteMT = "RimArcho's Korean and Russian are translated by Claude. If some wording reads oddly, press ctrl+\\, then write the element's code and your correction here."
)

// Templates are the six kinds in the order the page lists them.
var Templates = []Template{
	{Kind: KindFeature, Label: "feature request", Lead: "something RimArcho could do",
		Questions: []Question{
			{Key: "want", Label: "what you want to do", Form: FormText},
			{Key: "now", Label: "how you do it now, or what is in the way", Form: FormText, Optional: true},
			{Key: "others", Label: "how other tools do it", Form: FormText, Optional: true},
		}},
	{Kind: KindBug, Label: "bug", Lead: "RimArcho did something wrong",
		Questions: []Question{
			{Key: "did", Label: "what you did", Form: FormText, Hint: "the steps, one per line"},
			{Key: "expected", Label: "what you expected", Form: FormText},
			{Key: "actual", Label: "what happened instead", Form: FormText},
			{Key: "every", Label: "does it happen again", Form: FormChoice, Choices: []string{"every time", "sometimes", "only once"}, Optional: true},
			{Key: FilesQuestion, Label: "files were changed or lost", Form: FormCheck, Optional: true},
		},
		Attach: []string{"log", "ops"}},
	{Kind: KindDiagnosis, Label: "wrong diagnosis", Lead: "a false alarm, or a problem RimArcho missed",
		Questions: []Question{
			{Key: "finding", Label: "which diagnosis", Form: FormFinding, Hint: "pick one, or describe the one RimArcho missed"},
			{Key: "why", Label: "why you think it is wrong", Form: FormText},
			{Key: "game", Label: "what happened in the game", Form: FormText, Optional: true},
		},
		Attach: []string{"mods", "problems"}, Auto: []string{"finding"}},
	{Kind: KindData, Label: "outdated data", Lead: "shared-log data or the replacement library",
		Questions: []Question{
			{Key: "mod", Label: "which Mod", Form: FormMod, Hint: "packageId or Workshop ID"},
			{Key: "should", Label: "what it should be", Form: FormText, Hint: "for example: this Mod has been replaced by …"},
			{Key: "source", Label: "a link that shows it", Form: FormURL, Optional: true},
		},
		Attach: []string{"mods", "problems"}, Auto: []string{"mod", "data"}},
	{Kind: KindL10n, Label: "translation fix", Lead: "wording that reads oddly in your language",
		Note: "press ctrl+\\ to show each element's code, then write the code and the wording you suggest",
		Questions: []Question{
			{Key: L10nQuestion, Label: "codes and corrections", Form: FormFixes, Hint: "one per line: hom22 → the wording you suggest"},
			{Key: "note", Label: "anything else", Form: FormText, Optional: true},
		},
		Auto: []string{"refnums"}},
	{Kind: KindOther, Label: "other / help", Lead: "a question, or anything else",
		Note: "a problem in the game or in a Mod itself? its Mod page usually answers it sooner",
		Questions: []Question{
			{Key: "text", Label: "what you want to say", Form: FormText},
		},
		Attach: []string{"ops"}},
}

// TemplateOf is the template of a kind; an unknown kind is "other".
func TemplateOf(kind string) Template {
	for _, t := range Templates {
		if t.Kind == kind {
			return t
		}
	}
	return Templates[len(Templates)-1]
}

// Kinds are the template kinds in order.
func Kinds() []string {
	out := make([]string, len(Templates))
	for i, t := range Templates {
		out[i] = t.Kind
	}
	return out
}

// RefCodes are the reference numbers (ctrl+\, such as hom22) a translation fix names: the
// code that starts each line or each "; "-separated part, in order, once each.
func RefCodes(text string) []string {
	var out []string
	for _, line := range strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == ';' }) {
		if c := refCode(line); c != "" && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// refCode is the code a line starts with: two to eight letters, then digits (refnums.js
// prefixes are page names; SaveRefNums takes up to eight letters).
func refCode(line string) string {
	line = strings.ToLower(strings.TrimSpace(line))
	i := 0
	for i < len(line) && line[i] >= 'a' && line[i] <= 'z' {
		i++
	}
	j := i
	for j < len(line) && line[j] >= '0' && line[j] <= '9' {
		j++
	}
	if i < 2 || i > 8 || j == i || j-i > 6 {
		return ""
	}
	if j < len(line) && (line[j] >= 'a' && line[j] <= 'z' || line[j] == '_') {
		return "" // a word, not a code
	}
	return line[:j]
}

// Priority reports whether a report of this kind with these answers goes to the top: a bug
// whose files were changed or lost.
func Priority(kind string, answers map[string]string) bool {
	return kind == KindBug && isYes(answers[FilesQuestion])
}

func isYes(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "yes", "y", "true", "1", "on":
		return true
	}
	return false
}

// Answer is one question answered, with its label in the player's language.
type Answer struct {
	Label string
	Text  string
}

// AnswersText is the report's words: each answered question in bold, then the answer. A
// ticked box is its label alone, first, so the maintainer sees it before anything else.
func AnswersText(as []Answer, checks []string) string {
	var parts []string
	for _, c := range checks {
		parts = append(parts, "**"+c+"**")
	}
	for _, a := range as {
		t := strings.TrimSpace(a.Text)
		if t == "" {
			continue
		}
		if a.Label == "" {
			parts = append(parts, t)
			continue
		}
		parts = append(parts, "**"+a.Label+"**\n"+t)
	}
	return strings.Join(parts, "\n\n")
}
