package view

import (
	"runtime"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/cmds"
)

// TaskAgentInstall downloads and installs Archo Agent (15 §7.1).
const TaskAgentInstall = "agent-install"

// AgentGroup is the settings page's Archo Agent section (16 §2.14, 15 §7). Windows only: the bundle
// is built for Windows x64. A build without Archo Agent (buildinfo.AgentOn, 1.1) has no section.
func AgentGroup(a *app.App) (SettingGroup, bool) {
	if runtime.GOOS != "windows" || !buildinfo.AgentOn() {
		return SettingGroup{}, false
	}
	s := a.AgentStatus()
	status := T("not installed")
	install := T("install")
	var actions []SettingAction
	if s.Installed {
		status = Tf("%s (Pi %s)", s.Manifest.Version, s.Manifest.Pi)
		install = T("check for updates")
		if s.BaseURL != "" && s.Model != "" && s.HasKey {
			actions = append(actions, SettingAction{Label: T("open Archo Agent"), Argv: []string{"agent", "run", "--window"}})
		}
	}
	confirm := T("RimArcho downloads Archo Agent from GitHub (ToumanLin/ArchoAgent, about 70 MB: Node, the Pi coding agent and Archo's extensions), checks its SHA-256 and installs it in the local folder")
	if s.Installed {
		confirm = T("RimArcho asks GitHub (ToumanLin/ArchoAgent) for a newer Archo Agent; only if there is one it downloads it (about 70 MB), checks its SHA-256 and replaces this one")
	}
	actions = append(actions, SettingAction{Label: install, Argv: []string{"agent", "install"}, Task: TaskAgentInstall, Pending: TaskPending(TaskAgentInstall), Confirm: confirm})
	if s.Installed {
		actions = append(actions, SettingAction{Label: T("remove"), Argv: []string{"agent", "remove"},
			Confirm: T("the downloaded Archo Agent is deleted; its model settings, API key and conversations stay")})
	}
	key := T("none")
	if s.HasKey {
		key = T("stored in Windows Credential Manager")
	}
	empty := ""
	return SettingGroup{Title: T("Archo Agent"),
		Note: T("Archo as an AI agent in a console window of its own. It works through RimArcho's MCP tools, asks you before anything that changes your game, and sends your model provider no user name or Steam account."),
		Items: []SettingItem{
			{Label: T("Archo Agent"), Value: status, Actions: actions},
			{Label: T("model API"), Value: s.API, Edit: "choice", Choices: cmds.SettingChoices("agent.api"), Argv: []string{"config", "set", "agent.api", "{}"},
				Note: T("openai-completions fits most providers and gateways (DeepSeek, OpenRouter, local servers); anthropic-messages is Claude's own API")},
			{Label: T("API base URL"), Value: s.BaseURL, Edit: "text", Argv: []string{"config", "set", "agent.base_url", "{}"},
				Note: T("for example https://api.deepseek.com/v1")},
			{Label: T("model"), Value: s.Model, Edit: "text", Argv: []string{"config", "set", "agent.model", "{}"},
				Note: T("the model ID your provider uses, for example deepseek-chat")},
			{Label: T("thinking"), Value: s.Thinking, Edit: "choice", Choices: cmds.SettingChoices("agent.thinking"), Argv: []string{"config", "set", "agent.thinking", "{}"}},
			{Label: T("API key"), Value: key, EditValue: &empty, Edit: "secret", Argv: []string{"agent", "key", "{}"},
				Note: T("kept in Windows Credential Manager, never in RimArcho's files; apply an empty field to remove it")},
		}}, true
}
