package tui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"

	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Visual system (06 §7, §3).

// width uses the same East Asian setting as the CLI renderer and the WezTerm host
// (07 §7 item 3, 06 §7): ambiguous-width characters and Nerd Font icons are 1 cell.
var width = &runewidth.Condition{EastAsianWidth: false}

// Palette (06 §7, dark).
var (
	cText     = lipgloss.Color("#E6E6E6")
	cSecond   = lipgloss.Color("#7C8290")
	cDim      = lipgloss.Color("#4A4F5A")
	cAccent   = lipgloss.Color("#A5B4FC")
	cPill     = lipgloss.Color("#2C2E4A")
	cRowBg    = lipgloss.Color("#1A1C24") // the focused area's cursor row (06 §3.2)
	cSelBg    = lipgloss.Color("#232539") // selected rows (06 §3.4)
	cSelCurBg = lipgloss.Color("#303450") // selected row under the focused cursor
	cOK       = lipgloss.Color("#7EE2A8")
	cWarn     = lipgloss.Color("#F2C46D")
	cErr      = lipgloss.Color("#FF8A80")
)

var (
	sText   = lipgloss.NewStyle().Foreground(cText)
	sBold   = lipgloss.NewStyle().Foreground(cText).Bold(true)
	sSecond = lipgloss.NewStyle().Foreground(cSecond)
	sDimS   = lipgloss.NewStyle().Foreground(cDim)
	sAccent = lipgloss.NewStyle().Foreground(cAccent)
	sSel    = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sPill   = lipgloss.NewStyle().Foreground(cText).Background(cPill)
	sOK     = lipgloss.NewStyle().Foreground(cOK)
	sWarnS  = lipgloss.NewStyle().Foreground(cWarn)
	sErrS   = lipgloss.NewStyle().Foreground(cErr)
)

// rowBackground keeps selection and the focused cursor distinct across list views.
func rowBackground(st lipgloss.Style, selected, cursor bool) lipgloss.Style {
	switch {
	case selected && cursor:
		return st.Background(cSelCurBg)
	case selected:
		return st.Background(cSelBg)
	case cursor:
		return st.Background(cRowBg)
	default:
		return st
	}
}

// Styles used by the Problems, Changes and Inspect screens (kept from v1 until V3/V4,
// restyled: selection is a colour change, not a reversed bar; 06 §7).
var (
	stSel   = sSel
	stDim   = sDimS
	stTabOn = sSel
)

// icons is one icon set (06 §7).
type icons struct {
	official, workshop, local, localCopy, git string
	err, warn, logErr, unloadable             string
	unsaved, lock, update, dep, staging       string
	play, save, sort, search, fail            string
	// the evidence chips of 能不能跑 (06 §2.4): declared, inferred, log, community, your rule,
	// shared logs (the log corpus)
	evDeclared, evInferred, evLog, evCommunity, evRule, evCorpus string
}

// Nerd Font: the Font Awesome 4 range, in every Nerd Font and WezTerm's built-in symbols.
var nerdIcons = icons{
	official: "", workshop: "", local: "", localCopy: "", git: "",
	err: "", warn: "", logErr: "", unloadable: "",
	unsaved: "", lock: "", update: "", dep: "", staging: "",
	play: "", save: "", sort: "", search: "", fail: "",
	evDeclared: "", evInferred: "", evLog: "", evCommunity: "", evRule: "", evCorpus: "",
}

// Compatibility: East Asian width "neutral", present in Cascadia Mono, 1 cell (V0).
var compatIcons = icons{
	official: "⬥", workshop: "◉", local: "▢", localCopy: "⬦", git: "∆",
	err: "⨯", warn: "!", logErr: "◍", unloadable: "◌",
	unsaved: "⬤", lock: "", update: "⇣", dep: "›", staging: "◫",
	play: "►", save: "↲", sort: "↨", search: "/", fail: "⨯",
	evDeclared: "≣", evInferred: "◊", evLog: "◍", evCommunity: "⬡", evRule: "⌂", evCorpus: "◔",
}

// Marks shared by both sets.
const (
	markSel    = "▸"
	markOK     = "✓"
	markPrompt = "❯"
	markBusy   = "…"
)

var ic = compatIcons

// SetIcons picks the icon set: "nerd" or "compat" from ui.icons, else Nerd Font icons when
// the WezTerm host runs the TUI (06 §9.3).
func SetIcons(setting string) {
	switch setting {
	case "nerd":
		ic = nerdIcons
	case "compat":
		ic = compatIcons
	default:
		if os.Getenv("TERM_PROGRAM") == "WezTerm" {
			ic = nerdIcons
		} else {
			ic = compatIcons
		}
	}
}

// SetColors makes the palette render. Under ConPTY termenv cannot query the terminal and
// falls back to no colour, so a terminal that says it has 24-bit colour (WezTerm sets
// COLORTERM, Windows Terminal WT_SESSION) gets it.
func SetColors() {
	ct := os.Getenv("COLORTERM")
	if ct == "truecolor" || ct == "24bit" || os.Getenv("TERM_PROGRAM") == "WezTerm" || os.Getenv("WT_SESSION") != "" {
		lipgloss.SetColorProfile(termenv.TrueColor)
	}
}

// sourceIcon is the row's source icon (06 §7).
func sourceIcon(m *store.Mod) string {
	switch {
	case m.Kind == source.Official:
		return ic.official
	case m.Kind.WorkshopLike():
		return ic.workshop
	case m.GitHead != "":
		return ic.git
	case m.Origin != "":
		return ic.localCopy
	}
	return ic.local
}
