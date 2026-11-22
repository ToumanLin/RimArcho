package feedback

import (
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

// FilesQuestion is the bug kind's "files were changed or lost" box: ticked, the report
// carries the recent operations and the relay labels it priority.
const FilesQuestion = "files"

// Templates are the five kinds in the order the page lists them.
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
		Attach: []string{"log"}},
	{Kind: KindDiagnosis, Label: "wrong diagnosis", Lead: "a false alarm, or a problem RimArcho missed",
		Questions: []Question{
			{Key: "finding", Label: "which diagnosis", Form: FormFinding, Hint: "pick one, or describe the one RimArcho missed"},
			{Key: "why", Label: "why you think it is wrong", Form: FormText},
			{Key: "game", Label: "what happened in the game", Form: FormText, Optional: true},
		},
		Attach: []string{"mods"}, Auto: []string{"finding"}},
	{Kind: KindData, Label: "outdated data", Lead: "shared-log data or the replacement library",
		Questions: []Question{
			{Key: "mod", Label: "which Mod", Form: FormMod, Hint: "packageId or Workshop ID"},
			{Key: "should", Label: "what it should be", Form: FormText, Hint: "for example: this Mod has been replaced by …"},
			{Key: "source", Label: "a link that shows it", Form: FormURL, Optional: true},
		},
		Auto: []string{"mod", "data"}},
	{Kind: KindOther, Label: "other / help", Lead: "a question, or anything else",
		Note: "a problem in the game or in a Mod itself? its Mod page usually answers it sooner",
		Questions: []Question{
			{Key: "text", Label: "what you want to say", Form: FormText},
		}},
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
