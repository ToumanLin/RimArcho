package mcp

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/subscribe"
)

// updates (02 §3.4, 06 §2.9): subscriptions RimArcho installs and updates itself — Git
// repositories, download pages, Workshop items downloaded with SteamCMD — as one tool over
// archo subscribe (05 §1.2). update replaces installed files and remove moves folders to
// the recycle bin, so it is marked destructive; add, check and update reach Git hosts, web
// pages and Steam, so it is open-world. The LoversLab session stays the player's (05 §1.2):
// page subscriptions are read publicly, and files come from a zip or folder (from).
func subscriptionTools() []tool {
	return []tool{
		reg("manage_subscriptions", false, true, descManageSubscriptions, SubscriptionsArgv),
	}
}

// ManageSubscriptionsIn is manage_subscriptions.
type ManageSubscriptionsIn struct {
	Env      string   `json:"env,omitempty" jsonschema:"environment name (default: current)"`
	Action   string   `json:"action,omitempty" jsonschema:"list (default), add, check, update or remove"`
	URL      string   `json:"url,omitempty" jsonschema:"add: a Git repository URL, a download page (a LoversLab file page or another https page), or Workshop links or IDs separated by spaces"`
	Kind     string   `json:"kind,omitempty" jsonschema:"add: git, page or workshop; empty recognises it from url the way the window's paste box does"`
	Branch   string   `json:"branch,omitempty" jsonschema:"add (git): track this branch (default: the repository's default branch)"`
	Tag      string   `json:"tag,omitempty" jsonschema:"add (git): track this tag"`
	Commit   string   `json:"commit,omitempty" jsonschema:"add (git): pin this full commit SHA (never updates)"`
	Git      bool     `json:"git,omitempty" jsonschema:"add (page): subscribe to the page's single linked Git repository instead of downloaded files"`
	From     string   `json:"from,omitempty" jsonschema:"add (page) or update (one page subscription): a zip or folder the player downloaded in the browser"`
	SteamCMD bool     `json:"steamcmd,omitempty" jsonschema:"add (workshop): download with SteamCMD instead of subscribing through the Steam client"`
	Refs     []string `json:"refs,omitempty" jsonschema:"check, update, remove: the subscribed copies (#id); check without refs checks every subscription"`
	All      bool     `json:"all,omitempty" jsonschema:"update: every subscription (page ones are skipped: they need from)"`
	DryRun   bool     `json:"dry_run,omitempty" jsonschema:"add, update, remove: show what would happen, change nothing"`
}

// SubscriptionsArgv maps manage_subscriptions to archo subscribe list / git / page /
// workshop / check / update / remove.
func SubscriptionsArgv(in ManageSubscriptionsIn) []string {
	switch in.Action {
	case "", "list":
		return []string{"subscribe", "list"}
	case "add":
		kind := in.Kind
		if kind == "" {
			switch subscribe.Detect(in.URL) {
			case "git":
				kind = "git"
			case "page", "loverslab":
				kind = "page"
			case "workshop":
				kind = "workshop"
			default:
				return []string{"manage_subscriptions: url not recognised: give a Git repository, a download page, Workshop links or IDs, or kind"}
			}
		}
		switch kind {
		case "git":
			return argv("subscribe", "git", in.URL).str("branch", in.Branch).str("tag", in.Tag).str("commit", in.Commit).flag("dry-run", in.DryRun).a
		case "page":
			return argv("subscribe", "page", in.URL).flag("git", in.Git).str("from", in.From).flag("dry-run", in.DryRun).a
		case "workshop":
			return append(append([]string{"subscribe", "workshop"}, strings.Fields(in.URL)...), argv().flag("steamcmd", in.SteamCMD).flag("dry-run", in.DryRun).a...)
		}
		return []string{"manage_subscriptions: unknown kind " + in.Kind}
	case "check":
		return append([]string{"subscribe", "check"}, in.Refs...)
	case "update":
		return append(append([]string{"subscribe", "update"}, in.Refs...), argv().flag("all", in.All).str("from", in.From).flag("dry-run", in.DryRun).a...)
	case "remove":
		return append(append([]string{"subscribe", "remove"}, in.Refs...), argv().flag("dry-run", in.DryRun).a...)
	}
	return unknown("manage_subscriptions", in.Action)
}
