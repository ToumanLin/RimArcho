package gui

import (
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// The launch page's background (06 §2.3, §2.14): which image, how dark, how blurred and
// where its focus is, kept as one GUI setting. The images are app.Backgrounds plus the
// files the player picked; the page loads them from /bg, which serves only those.

const bgSettingKey = "ui.gui.home.bg"

// BgPlain is the key of the plain background (the theme's own colour).
const BgPlain = "plain"

// BgSetting is the stored choice.
type BgSetting struct {
	Key    string   `json:"key"`
	Dim    int      `json:"dim"`    // 0–80, % of black over the image
	Blur   int      `json:"blur"`   // 0–20 px
	Focus  string   `json:"focus"`  // "left" | "center" | "right"
	Rotate bool     `json:"rotate"` // a starred image at random each time the window opens
	Stars  []string `json:"stars"`
	Files  []string `json:"files"` // the player's own images
}

func (s *API) bgSettingLocked() BgSetting {
	b := BgSetting{Key: app.BgDefault, Dim: 30, Focus: "center"}
	if raw := s.a.Store.Meta(bgSettingKey); raw != "" {
		json.Unmarshal([]byte(raw), &b)
	}
	b.Dim = min(max(b.Dim, 0), 80)
	b.Blur = min(max(b.Blur, 0), 20)
	if b.Focus != "left" && b.Focus != "right" {
		b.Focus = "center"
	}
	if b.Stars == nil {
		b.Stars = []string{}
	}
	if b.Files == nil {
		b.Files = []string{}
	}
	return b
}

// bgListLocked uses only the allow-list published by prepareBackgrounds. Image lookups
// must not trigger folder discovery or rebuild the board under the shared lock.
func (s *API) bgListLocked() []app.Background {
	var out []app.Background
	if e, err := s.a.Env(); err == nil && s.bgsEnv == e.ID {
		out = slices.Clone(s.bgs)
	}
	for _, f := range s.bgSettingLocked().Files {
		out = append(out, app.Background{Key: "file:" + f, Label: filepath.Base(f), Group: "\x00file", Src: f})
	}
	return out
}

func (s *API) bgFind(key string) *app.Background {
	for _, b := range s.bgListLocked() {
		if b.Key == key {
			return &b
		}
	}
	return nil
}

func bgURL(key string, thumb bool) string {
	w := "full"
	if thumb {
		w = "thumb"
	}
	return "/bg?w=" + w + "&k=" + url.QueryEscape(key)
}

// HomeBg is what the launch page draws: URL "" is the plain background.
type HomeBg struct {
	URL   string `json:"url"`
	Dim   int    `json:"dim"`
	Blur  int    `json:"blur"`
	Focus string `json:"focus"`
}

// Capture small background targets, then discover files outside the API lock. Image
// handlers resolve only the published allow-list; an environment switch rejects old data.
func (s *API) prepareBackgrounds(fresh bool) {
	s.mu.Lock()
	e, err := s.a.Env()
	if err != nil || (!fresh && s.bgs != nil && s.bgsEnv == e.ID) {
		s.mu.Unlock()
		return
	}
	ix, err := s.a.Index()
	if err != nil {
		s.mu.Unlock()
		return
	}
	targets := s.a.BackgroundTargets(ix)
	s.mu.Unlock()
	defer s.free.soon()
	backgrounds := app.CollectBackgrounds(targets)
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, err := s.a.Env(); err == nil && current.ID == e.ID {
		s.bgs, s.bgsEnv = backgrounds, e.ID
		if s.bgs == nil {
			s.bgs = []app.Background{}
		}
	}
}

// HomeBackground is the launch page's background. With rotation on, one starred image is
// picked the first time the window asks and kept until it closes.
func (s *API) HomeBackground() *HomeBg {
	s.prepareBackgrounds(false)
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.bgSettingLocked()
	out := &HomeBg{Dim: b.Dim, Blur: b.Blur, Focus: b.Focus}
	key := b.Key
	if b.Rotate && len(b.Stars) > 0 {
		if s.bgPick == "" || !slices.Contains(b.Stars, s.bgPick) {
			s.bgPick = b.Stars[rand.IntN(len(b.Stars))]
		}
		key = s.bgPick
	}
	if key == BgPlain {
		return out
	}
	if s.bgFind(key) == nil {
		key = app.BgDefault // a mod that is gone, a file that moved
		if s.bgFind(key) == nil {
			return out
		}
	}
	out.URL = bgURL(key, false)
	return out
}

// BgItem is one image on the settings page.
type BgItem struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Thumb string `json:"thumb"` // "" for the plain background
	Full  string `json:"full"`
}

// BgGroup is one tab: the game, a background mod, the player's images.
type BgGroup struct {
	Name  string   `json:"name"`
	Files bool     `json:"files"` // the player's images: + adds one, items can be removed
	Items []BgItem `json:"items"`
}

// BgPage is the settings page's background section.
type BgPage struct {
	Setting BgSetting `json:"setting"`
	Groups  []BgGroup `json:"groups"`
	Dialogs bool      `json:"dialogs"`
}

// Backgrounds lists the images by group (reading the background mods again).
func (s *API) Backgrounds() *BgPage {
	s.prepareBackgrounds(true)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := &BgPage{Setting: s.bgSettingLocked(), Dialogs: s.x.native != nil}
	game := BgGroup{Name: view.T("the game's own"), Items: []BgItem{}}
	files := BgGroup{Name: view.T("your images"), Files: true, Items: []BgItem{}}
	var mods []BgGroup
	for _, b := range s.bgListLocked() {
		it := BgItem{Key: b.Key, Label: b.Label, Thumb: bgURL(b.Key, true), Full: bgURL(b.Key, false)}
		switch b.Group {
		case "":
			game.Items = append(game.Items, it)
		case "\x00file":
			files.Items = append(files.Items, it)
		default:
			if len(mods) == 0 || mods[len(mods)-1].Name != b.Group {
				mods = append(mods, BgGroup{Name: b.Group})
			}
			mods[len(mods)-1].Items = append(mods[len(mods)-1].Items, it)
		}
	}
	game.Items = append(game.Items, BgItem{Key: BgPlain, Label: view.T("plain")})
	out.Groups = append(append([]BgGroup{game}, mods...), files)
	return out
}

// SetBackground stores the choice (the values are clamped when read).
func (s *API) SetBackground(b BgSetting) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b.Files = s.bgSettingLocked().Files // files change only through Add/RemoveBackgroundFile
	return s.saveBgLocked(b)
}

func (s *API) saveBgLocked(b BgSetting) error {
	if b.Stars == nil {
		b.Stars = []string{}
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return err
	}
	s.bgPick = ""
	return s.a.Store.SetMeta(bgSettingKey, string(raw))
}

var bgFilters = [][2]string{{"Images (*.png;*.jpg;*.jpeg)", "*.png;*.jpg;*.jpeg"}}

// AddBackgroundFile asks for an image (the window's open dialog; path is a typed one when
// there is no dialog), adds it to the player's images and selects it. "" when cancelled.
func (s *API) AddBackgroundFile(path string) (string, error) {
	if path == "" && s.x.native != nil {
		p, err := s.x.native.openFile(view.T("pick a background image"), bgFilters)
		if err != nil || p == "" {
			return "", err
		}
		path = p
	}
	if path == "" {
		return "", nil
	}
	path = filepath.Clean(path)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg":
	default:
		return "", errors.New(view.T("pick a PNG or JPG image"))
	}
	if _, err := s.a.BackgroundImage(path, app.BgThumb); err != nil { // readable, and cached now
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.bgSettingLocked()
	if !slices.Contains(b.Files, path) {
		b.Files = append(b.Files, path)
	}
	b.Key = "file:" + path
	return b.Key, s.saveBgLocked(b)
}

// RemoveBackgroundFile forgets one of the player's images (the file stays where it is).
func (s *API) RemoveBackgroundFile(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.bgSettingLocked()
	path := strings.TrimPrefix(key, "file:")
	b.Files = slices.DeleteFunc(b.Files, func(f string) bool { return f == path })
	b.Stars = slices.DeleteFunc(b.Stars, func(k string) bool { return k == key })
	if b.Key == key {
		b.Key = app.BgDefault
	}
	return s.saveBgLocked(b)
}

// serveBg is /bg?k=<key>&w=full|thumb: the cached JPEG of a listed background.
func (s *API) serveBg(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	b := s.bgFind(r.URL.Query().Get("k"))
	s.mu.Unlock()
	if b == nil {
		http.NotFound(w, r)
		return
	}
	width := app.BgFull
	if r.URL.Query().Get("w") == "thumb" {
		width = app.BgThumb
	}
	path, err := s.a.BackgroundImage(b.Src, width)
	serveFile(w, r, path, err)
}
