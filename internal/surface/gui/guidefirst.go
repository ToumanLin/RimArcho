//go:build windows || cgo

package gui

import (
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/l10ndata"
)

// The first run's choices (16 §2.15): the language on the "found" step, and on "read the
// mods" the two defaults that would otherwise apply unasked: automatic checks (updates.auto,
// which also covers community data) and Archo Monitor (monitor.default).

// GuideLanguage is the language row: the setting's value, its choices and their labels,
// and the name of the language "follow system" means here.
type GuideLanguage struct {
	Value  string   `json:"value"`
	Codes  []string `json:"codes"`
	Labels []string `json:"labels"`
	System string   `json:"system"`
}

// GuideLanguages reads the language row's choices: English, Chinese, then the others by
// code, each named in itself.
func (s *API) GuideLanguages() GuideLanguage {
	s.mu.Lock()
	v := cmds.Setting(s.a, "ui.lang")
	s.mu.Unlock()
	out := GuideLanguage{Value: v, System: languageName(l10ndata.ResolveLang("auto"))}
	if out.Value == "" {
		out.Value = "auto"
	}
	codes := []string{"en", "zh"}
	for _, c := range l10ndata.LanguageCodes() {
		if c != "auto" && c != "en" && c != "zh" {
			codes = append(codes, c)
		}
	}
	for _, c := range codes {
		out.Codes = append(out.Codes, c)
		out.Labels = append(out.Labels, languageName(c))
	}
	return out
}

// languageName is a bundled language's own name; English has no locale folder.
func languageName(code string) string {
	if code == "en" {
		return "English"
	}
	return l10ndata.LanguageName(code)
}

// GuideChoices are step 2's switches as set now: automatic checks and Archo Monitor.
type GuideChoices struct {
	Updates bool `json:"updates"`
	Monitor bool `json:"monitor"`
}

func (s *API) GuideChoices() GuideChoices {
	s.mu.Lock()
	defer s.mu.Unlock()
	return GuideChoices{Updates: s.a.AutomaticEnabled(), Monitor: s.a.MonitorDefault()}
}

// GuideHold keeps the automatic checks (16 §2.16.1 A01) from starting while the first run
// has not passed "read the mods", so its "check for updates: off" takes effect before the
// first download. It lasts only while the window runs.
func (s *API) GuideHold() { s.guideHold.Store(true) }

// GuideChoicesDone is step 2's "next" (or the guide's end): the automatic checks may start.
// With Archo Monitor off, the scan of the games just added (envs) may still have put it in
// their lists; it comes out again as one undoable edit per list.
func (s *API) GuideChoicesDone(envs []string) error {
	s.guideHold.Store(false)
	if len(envs) == 0 {
		return nil
	}
	r, err := s.detachedReader()
	if err != nil {
		return err
	}
	defer r.Close()
	if r.MonitorDefault() {
		return nil
	}
	removed := false
	for _, n := range envs {
		r.EnvName = n
		was, err := r.DropMonitor()
		if err != nil {
			return err
		}
		removed = removed || was
	}
	if removed {
		s.mu.Lock()
		s.board, s.needs = nil, nil
		s.mu.Unlock()
	}
	return nil
}
