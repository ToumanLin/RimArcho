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
