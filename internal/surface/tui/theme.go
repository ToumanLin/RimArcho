package tui

import (
	"os"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"

	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Visual system (16 §7, §3).

// width uses the same East Asian setting as the CLI renderer
// (17 §6 item 3, 16 §7): ambiguous-width characters and Nerd Font icons are 1 cell.
var width = &runewidth.Condition{EastAsianWidth: false}

// Palettes (16 §7): one for dark terminals, one for light ones. SetTheme picks one. Each
// colour also names its xterm-256 and 16-colour stand-ins, chosen by hand: the nearest
// match lipgloss would pick merges faint text with the selected cursor row in 256 colours
// and turns the light pill magenta in 16.
type colours struct {
	text, second, dim, accent, pill lipgloss.TerminalColor
	row, sel, selCur                lipgloss.TerminalColor // focused cursor row, selected, selected under the cursor
	ok, warn, err                   lipgloss.TerminalColor
}

// cc is one colour in 24-bit, xterm-256 and 16-colour terminals.
func cc(hex, c256, c16 string) lipgloss.CompleteColor {
	return lipgloss.CompleteColor{TrueColor: hex, ANSI256: c256, ANSI: c16}
}

var (
	darkPalette = colours{
		text: cc("#E6E6E6", "253", "15"), second: cc("#858B99", "245", "7"), dim: cc("#5C6270", "241", "8"),
		accent: cc("#A5B4FC", "147", "12"), pill: cc("#2C2E4A", "236", "4"),
		row: cc("#1A1C24", "234", "0"), sel: cc("#232539", "17", "4"), selCur: cc("#303450", "18", "4"),
		ok: cc("#7EE2A8", "115", "10"), warn: cc("#F2C46D", "221", "11"), err: cc("#FF8A80", "210", "9"),
	}
	lightPalette = colours{
		text: cc("#1C1C21", "234", "0"), second: cc("#5F5F68", "241", "8"), dim: cc("#8E8E97", "246", "8"),
		accent: cc("#4F46E5", "62", "4"), pill: cc("#E4E3FB", "189", "7"),
		row: cc("#F0F0F4", "255", "15"), sel: cc("#ECEBFB", "254", "7"), selCur: cc("#DCDAF8", "189", "7"),
		ok: cc("#188A50", "29", "2"), warn: cc("#B45309", "130", "3"), err: cc("#D64545", "167", "1"),
	}
)

// The colours and styles in use; applyPalette sets them.
var (
	cText, cSecond, cDim, cAccent, cPill lipgloss.TerminalColor
	cRowBg, cSelBg, cSelCurBg            lipgloss.TerminalColor
	cOK, cWarn, cErr                     lipgloss.TerminalColor

	sText, sBold, sSecond, sHead, sDimS, sAccent, sSel lipgloss.Style
	sPill, sPillDim, sPillWarn                         lipgloss.Style
	sOK, sWarnS, sWarnB, sErrS, sErrB                  lipgloss.Style
	sRowBold                                           lipgloss.Style // the open sub-page in the context row

	// Styles used by the Problems, Changes and Inspect screens (kept from v1 until V3/V4,
	// restyled: selection is a colour change, not a reversed bar; 16 §7).
	stSel, stDim, stTabOn lipgloss.Style
)

func init() { applyPalette(darkPalette) }

func applyPalette(p colours) {
	cText, cSecond, cDim, cAccent, cPill = p.text, p.second, p.dim, p.accent, p.pill
	cRowBg, cSelBg, cSelCurBg = p.row, p.sel, p.selCur
	cOK, cWarn, cErr = p.ok, p.warn, p.err
	st := lipgloss.NewStyle
	sText = st().Foreground(cText)
	sBold = st().Foreground(cText).Bold(true)
	sSecond = st().Foreground(cSecond)
	sHead = st().Foreground(cSecond).Bold(true) // an area or section heading (16 §7)
	sDimS = st().Foreground(cDim)
	sAccent = st().Foreground(cAccent)
	sSel = st().Foreground(cAccent).Bold(true)
	sPill = st().Foreground(cText).Background(cPill)
	sPillDim = st().Foreground(cDim).Background(cPill)
	sPillWarn = st().Foreground(cWarn).Background(cPill).Bold(true)
	sOK = st().Foreground(cOK)
	sWarnS = st().Foreground(cWarn)
	sWarnB = st().Foreground(cWarn).Bold(true)
	sErrS = st().Foreground(cErr)
	sErrB = st().Foreground(cErr).Bold(true)
	sRowBold = st().Foreground(cText).Background(cRowBg).Bold(true)
	stSel, stDim, stTabOn = sSel, sDimS, sSel
}

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

// icons is one icon set (16 §7).
type icons struct {
	official, workshop, local, localCopy, git string
	err, warn, logErr, unloadable, hint       string
	unsaved, lock, update, dep, staging       string
	play, save, sort, search, fail            string
	// the evidence chips of Can it run (16 §2.4): declared, inferred, log, community, your rule,
	// shared logs (the log corpus)
	evDeclared, evInferred, evLog, evCommunity, evRule, evCorpus string
}

// Nerd Font: the Font Awesome 4 range, in every Nerd Font.
var nerdIcons = icons{
	official: "", workshop: "", local: "", localCopy: "", git: "",
	err: "", warn: "", logErr: "", unloadable: "", hint: "",
	unsaved: "", lock: "", update: "", dep: "", staging: "",
	play: "", save: "", sort: "", search: "", fail: "",
	evDeclared: "", evInferred: "", evLog: "", evCommunity: "", evRule: "", evCorpus: "",
}

// Compatibility: East Asian width "neutral", present in Cascadia Mono, 1 cell (V0).
var compatIcons = icons{
	official: "⬥", workshop: "◉", local: "▢", localCopy: "⬦", git: "∆",
	err: "⨯", warn: "!", logErr: "◍", unloadable: "◌", hint: "ⓘ",
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

// SetIcons uses Nerd Font icons only when explicitly requested.
func SetIcons(setting string) {
	if setting == "nerd" {
		ic = nerdIcons
	} else {
		ic = compatIcons
	}
}

// SetColors makes the palette render. Under ConPTY termenv cannot query the terminal and
// falls back to no colour, so COLORTERM or Windows Terminal's WT_SESSION enables 24-bit colour.
func SetColors() {
	ct := os.Getenv("COLORTERM")
	if ct == "truecolor" || ct == "24bit" || os.Getenv("WT_SESSION") != "" {
		lipgloss.SetColorProfile(termenv.TrueColor)
	}
}

// darkTerminal is the terminal's background as asked once before the screen starts
// (termenv: OSC 11, then COLORFGBG); Windows' console host cannot answer, so it reads dark.
var darkTerminal = sync.OnceValue(func() bool { return lipgloss.HasDarkBackground() })

// SetTheme picks the palette (ui.tui.theme, 16 §9.2): light, dark, or auto by the
// terminal's background. Run calls it before the program reads the keyboard, so the
// terminal's answer cannot reach the key handler; later calls reuse that answer.
func SetTheme(setting string) {
	switch setting {
	case "light":
		applyPalette(lightPalette)
	case "dark":
		applyPalette(darkPalette)
	default:
		if darkTerminal() {
			applyPalette(darkPalette)
		} else {
			applyPalette(lightPalette)
		}
	}
}

// sourceIcon is the row's source icon (16 §7).
func sourceIcon(m *store.Mod) string {
	switch view.SourceKind(m) {
	case "official":
		return ic.official
	case "workshop":
		return ic.workshop
	case "git":
		return ic.git
	case "copy":
		return ic.localCopy
	}
	return ic.local
}
