//go:build windows || cgo

package gui

import (
	"errors"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// SetAgentKey stores Archo Agent's model API key in Windows Credential Manager ("" removes it, 15 §7.2).
// It is a call of its own so the key never passes through a command line or the task list. A build
// without Archo Agent (buildinfo.AgentOn) refuses it.
func (s *API) SetAgentKey(key string) ([]string, error) {
	if !buildinfo.AgentOn() {
		return nil, errors.New("not a settings command: agent key")
	}
	if err := app.SetAgentKey(key); err != nil {
		return nil, errors.New(view.Text(err.Error()))
	}
	if key == "" {
		return []string{view.T("API key removed")}, nil
	}
	return []string{view.T("API key stored in Windows Credential Manager")}, nil
}
