package view

import (
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
)

// IdentityReasons is loaded on demand; building a question card does no file queries.
func IdentityReasons(a *app.App, qid int64) ([]string, error) {
	q, err := a.IdentityQuestion(qid)
	if err != nil {
		return nil, err
	}
	return cmds.IdentityReasonLines(a.Store, q, Tf)
}

// IdentityPanel carries the comparison and the same confirmed answers as the row.
type IdentityPanel struct {
	*cmds.IdentityComparisonDoc
	Answers []Action `json:"answers"`
}

func IdentityQuestionPanel(a *app.App, qid int64) (*IdentityPanel, error) {
	q, err := a.IdentityQuestion(qid)
	if err != nil {
		return nil, err
	}
	comparison, err := cmds.CompareIdentity(a.Store, q, Tf)
	if err != nil {
		return nil, err
	}
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	out := &IdentityPanel{IdentityComparisonDoc: comparison}
	if q.Status == "open" {
		for _, action := range questionNeed(st, q).Actions {
			if action.Kind == ActAnswer {
				out.Answers = append(out.Answers, action)
			}
		}
	}
	return out, nil
}
