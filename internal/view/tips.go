package view

// Tip is a short usage hint for loading screens and the first-run guide.
type Tip struct {
	Text string `json:"text"`
	When string `json:"when"`
}

// Tips returns hints in the selected interface language.
// When is "any" for general hints or "first" for first-run hints.
func Tips() []Tip {
	return []Tip{
		{Text: T("Press ? in the current list to see its keyboard shortcuts."), When: "any"},
		{Text: T("Press Ctrl+K or : to open the command panel."), When: "any"},
		{Text: T("Press Tab in the command panel to complete a command."), When: "any"},
		{Text: T("Press Alt+Left or Alt+Right to go back or forward between pages."), When: "any"},
		{Text: T("Press Ctrl+J to open Background tasks; press it again to return."), When: "any"},
		{Text: T("Press / in the current list to search; tags: and source: narrow the results."), When: "any"},
		{Text: T("Use author: in Mod search to find Mods by their author."), When: "any"},
		{Text: T("Put - before a search word to leave matching Mods out of the results."), When: "any"},
		{Text: T("Press Tab in Mod search to accept the suggested filter."), When: "any"},
		{Text: T("Press Space on a Mod in the current list to enable or disable it."), When: "any"},
		{Text: T("Hold Shift and click a row in the current list to select a range."), When: "any"},
		{Text: T("Drag Mods within Enabled to change their load order."), When: "any"},
		{Text: T("Press t in the current list to stage a Mod or put it back."), When: "any"},
		{Text: T("Press Ctrl+Z in the current list to undo a list edit."), When: "any"},
		{Text: T("Press Ctrl+Y in the current list to redo a list edit."), When: "any"},
		{Text: T("Press Shift+H in the current list to browse steps and saved versions."), When: "any"},
		{Text: T("Press Shift+S in the current list to sort the enabled Mods."), When: "any"},
		{Text: T("Press r on a Mod in the current list to add a load-order rule."), When: "any"},
		{Text: T("Open Can it run to compare file checks with errors from your last game."), When: "any"},
		{Text: T("Use the file menu in Game log to switch between recorded runs."), When: "any"},
		{Text: T("Select a matching error in Game log to read Archo log analysis."), When: "any"},
		{Text: T("Create a pack to keep Mod versions fixed and play with its own saves."), When: "first"},
		{Text: T("Press Shift+E on a pack's page to export its list without the Mod files."), When: "any"},
		{Text: T("Use Export in All lists to copy a share code for a friend."), When: "first"},
		{Text: T("Choose move over from RimSort or RimCrow in the guide to import your lists."), When: "first"},
		{Text: T("Tick several detected games in the guide to add them together."), When: "first"},
		{Text: T("Choose the game's current list in the guide to start with your enabled Mods."), When: "first"},
		{Text: T("In the guide, open a pack file or paste a Steam Workshop collection link."), When: "first"},
		{Text: T("Use Make a list in Saves to start a list from a save's Mods."), When: "first"},
		{Text: T("Paste a GitHub repository into Add a subscription to follow a Mod's updates."), When: "first"},
		{Text: T("Paste several Workshop IDs into Add a subscription to get them together."), When: "any"},
		{Text: T("Press Shift+K on Updates to check your subscriptions for updates."), When: "any"},
		{Text: T("In Background tasks, press r on a failed item that can be retried."), When: "any"},
		{Text: T("Press F2 in Saves to rename the selected save while the game is stopped."), When: "any"},
		{Text: T("Select a save in Saves and choose Back up to keep a backup copy."), When: "any"},
		{Text: T("Open Settings to choose your language and a light or dark theme."), When: "any"},
		{Text: T("Drag a Mod from Disabled to Enabled to add it to your list."), When: "any"},
	}
}
