package view

import (
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/feedback"
)

// archo feedback <kind> (13 §4.2, 16 §2.14): the draft is what the Feedback page would show
// and link for the same template, answers and attachments.
func init() { cmds.FeedbackDraft = feedbackDraft }

func feedbackDraft(c *cmds.Ctx, kind, title string, answers map[string]string, attach []string) (string, string, error) {
	p := FeedbackProblem{Kind: kind, Title: title, Answers: answers}
	// the template's own ticks, as on the page, and the ones named
	for _, k := range append(append([]string{}, feedback.TemplateOf(kind).Attach...), attach...) {
		switch k {
		case "mods":
			p.Opts.Mods = true
		case "paths":
			p.Opts.Paths = true
		case "log":
			p.Opts.Log = true
		case "ops":
			p.Opts.Ops = true
		case "problems":
			p.Opts.Problems = true
		}
	}
	st, _ := c.App.Loadout() // no game yet: only the basic lines
	o := p.Options()
	var n *Needs
	if (o.Problems || o.Finding != "") && st != nil {
		if ps, err := c.App.Problems(false, nil); err == nil && ps != nil {
			n = BuildNeeds(c.App, st, ps)
		}
	}
	at := FeedbackAttachments(c.App, st, o, n)
	res := ComposeFeedback(p.TitleOrFirst(), FeedbackSkeleton(p), at)
	text := FeedbackPreview(p.Text(), at)
	if t := p.TitleOrFirst(); t != "" {
		text = t + "\n\n" + text
	}
	return text, res.URL, nil
}
