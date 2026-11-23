//go:build windows || cgo

package gui

import (
	"github.com/ToumanLin/RimArcho/internal/perftrace"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Operations is "What RimArcho did" under Background tasks (16 §2.16, 13 §1.3): the
// operation log, newest first. It reads only op_log, so it needs no page session.
func (s *API) Operations() (*view.OpsPage, error) {
	defer perftrace.Span("Operations")()
	return view.Operations(s.a)
}
