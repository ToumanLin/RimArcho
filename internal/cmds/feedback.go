package cmds

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/feedback"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// Feedback (16 §2.14): the report templates and a report drafted from them, the same
// templates, questions and attachments as the window's Feedback page. The CLI and MCP only
// draft: the report and its GitHub link are printed, and nothing leaves the machine (sending
// stays with the player, on the Feedback page or on GitHub).

// FeedbackDraft drafts a report of kind with the answers and the attachments named (mods,
// paths, log, ops, problems): its readable text and the GitHub link. The interface layer sets
// it (view), since the attachments are what the Feedback page reads.
var FeedbackDraft func(c *Ctx, kind, title string, answers map[string]string, attach []string) (text, link string, err error)

// FeedbackAttachments are the optional attachments a draft may name.
var FeedbackAttachments = []string{"mods", "paths", "log", "ops", "problems"}

func init() {
	flags := []Flag{
		{Name: "template", Value: "<" + strings.Join(feedback.Kinds(), "|") + ">", Help: "the report's kind (the same as <kind>)"},
		{Name: "title", Value: "<text>", Help: "the title (default: the first answer's first line)"},
	}
	seen := map[string]bool{}
	for _, t := range feedback.Templates {
		for _, q := range t.Questions {
			if seen[q.Key] {
				continue
			}
			seen[q.Key] = true
			f := Flag{Name: q.Key, Value: "<text>", Help: t.Kind + ": " + q.Label}
			if q.Key == feedback.DataFinding {
				f.Help += "; data: the Can it run item that showed the data"
			}
			switch q.Form {
			case feedback.FormCheck:
				f.Value = ""
			case feedback.FormChoice:
				f.Value = "<" + strings.Join(q.Choices, "|") + ">"
			case feedback.FormFinding:
				f.Value = "<id>"
			case feedback.FormMod:
				f.Value = "<packageId>"
			case feedback.FormURL:
				f.Value = "<url>"
			}
			flags = append(flags, f)
		}
	}
	flags = append(flags, Flag{Name: "attach", Value: "<" + strings.Join(FeedbackAttachments, ",") + ">", Help: "optional attachments beyond the template's own"})
	register(&Command{Name: "feedback", Args: "[<kind>]", MaxArgs: 1, Run: feedbackCmd, Flags: flags,
		Summary: "report templates; with a kind, draft a report: its text and GitHub link (nothing is sent)"})
}

func feedbackCmd(c *Ctx, a *Args) (*render.Doc, error) {
	kind := strings.ToLower(strings.TrimSpace(a.Str("template")))
	if len(a.Pos) > 0 {
		pos := strings.ToLower(strings.TrimSpace(a.Pos[0]))
		if kind != "" && kind != pos {
			return nil, &UsageError{Msg: fmt.Sprintf("two report kinds: %s and --template %s", pos, kind)}
		}
		kind = pos
	}
	if kind == "" {
		return feedbackTemplatesDoc(), nil
	}
	if !slices.Contains(feedback.Kinds(), kind) {
		return nil, &UsageError{Msg: fmt.Sprintf("unknown report kind %q: use %s", kind, strings.Join(feedback.Kinds(), ", "))}
	}
	t := feedback.TemplateOf(kind)
	answers := map[string]string{}
	for _, q := range t.Questions {
		if q.Form == feedback.FormCheck {
			if a.Bool(q.Key) {
				answers[q.Key] = "yes"
			}
			continue
		}
		v := strings.TrimSpace(a.Str(q.Key))
		if v == "" {
			continue
		}
		if q.Form == feedback.FormChoice && !slices.Contains(q.Choices, v) {
			return nil, &UsageError{Msg: fmt.Sprintf("--%s takes %s", q.Key, strings.Join(q.Choices, ", "))}
		}
		answers[q.Key] = v
	}
	// an outdated-data report may name the Can it run item that showed the data
	if kind == feedback.KindData {
		if v := strings.TrimSpace(a.Str(feedback.DataFinding)); v != "" {
			answers[feedback.DataFinding] = v
		}
	}
	for k := range a.flags {
		if k == "title" || k == "attach" || k == "template" || (kind == feedback.KindData && k == feedback.DataFinding) || slices.ContainsFunc(t.Questions, func(q feedback.Question) bool { return q.Key == k }) {
			continue
		}
		return nil, &UsageError{Msg: fmt.Sprintf("a %s report has no --%s: see archo feedback", kind, k)}
	}
	var attach []string
	for _, s := range strings.Split(a.Str("attach"), ",") {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		if !slices.Contains(FeedbackAttachments, s) {
			return nil, &UsageError{Msg: fmt.Sprintf("unknown attachment %q: use %s", s, strings.Join(FeedbackAttachments, ", "))}
		}
		attach = append(attach, s)
	}
	if FeedbackDraft == nil {
		return nil, fmt.Errorf("feedback drafts need the interface layer")
	}
	text, link, err := FeedbackDraft(c, kind, a.Str("title"), answers, attach)
	if err != nil {
		return nil, err
	}
	d, root := render.New(kind + " report (draft; public on GitHub once sent)")
	for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		root.Add(l)
	}
	root.KV("open on GitHub", link)
	root.Add("send it without a GitHub account from RimArcho's Feedback page")
	return d, nil
}

// feedbackTemplatesDoc lists the templates: each kind's questions as flags and what it
// attaches by itself.
func feedbackTemplatesDoc() *render.Doc {
	d, root := render.New("report templates: archo feedback <kind> --<question> <answer> …")
	for _, t := range feedback.Templates {
		n := root.Add(t.Kind + "  " + t.Label + ": " + t.Lead)
		if t.Note != "" {
			n.Add(t.Note)
		}
		for _, q := range t.Questions {
			line := "--" + q.Key + "  " + q.Label
			if q.Form == feedback.FormChoice {
				line += " (" + strings.Join(q.Choices, ", ") + ")"
			}
			if q.Form == feedback.FormCheck {
				line += " (a box; a bug with files changed or lost is sent as a priority report, with the recent operations)"
			}
			if q.Optional {
				line += "  optional"
			}
			n.Add(line)
		}
		auto := append([]string{"basic information"}, t.Auto...)
		n.KV("attached", strings.Join(auto, ", ")+ticked(t.Attach))
	}
	return d
}

func ticked(on []string) string {
	if len(on) == 0 {
		return ""
	}
	return "; ticked: " + strings.Join(on, ", ")
}
