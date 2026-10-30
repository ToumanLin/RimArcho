package tui

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 添加订阅 (06 §2.9, §9.1): Mod 库存's paste box as a page of its own (+ on 订阅与更新, the
// palette, a paste on 订阅与更新). What is typed or pasted is recognised the way the window
// recognises it (subscribe.Detect); under it a card asks only the one decision that source
// needs (↑↓), shows the --dry-run text, and enter queues 订阅 as a background task (06
// §2.16). A download page is subscribed through its single Git repository; its other ways —
// LoversLab files with the session, a zip or folder downloaded in the browser — need the
// window's file chooser and session, so the card says so and gives the command.

type addState struct {
	text, shown string // the box; the text the card was made for
	kind        string // subscribe.Detect(shown)
	opt         int    // the card's choice
	ref         string // git: the branch, tag or commit name
	field       int    // 0 the box, 1 the name
	info        string // page: name, version, date, or why it could not be read
	main        string // page: its single repository ("" none)
	reading     bool   // page: being read
	lines       []string
	apply       bool
	previewOf   string // the request the dry run is for
	seq         int
}

// addDebounce: the card follows the box after the typing pauses (the window's 250 ms).
const addDebounce = 250 * time.Millisecond

type addTick struct{ seq int }

// openAdd shows 添加订阅 with text in the box ("" an empty box).
func (m *model) openAdd(text string) tea.Cmd {
	m.panel, m.insp, m.full = nil, nil, false
	m.add = addState{text: text}
	m.tab = tabSubscribe
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return m.recognise()
}

// options are the card's choices.
func (s *addState) options() []string {
	switch s.kind {
	case "git":
		return []string{T("the default branch"), T("a branch"), T("a tag"), T("one commit")}
	case "workshop":
		return []string{T("subscribe with Steam"), T("download with SteamCMD")}
	case "page", "loverslab":
		if s.main != "" {
			return []string{Tf("subscribe with Git: %s", s.main)}
		}
	}
	return nil
}

// needsName: the Git choice wants a branch, tag or commit typed.
func (s *addState) needsName() bool { return s.kind == "git" && s.opt > 0 }

// request is the card's decision, false while it is not complete.
func (s *addState) request() (view.SubscribeRequest, bool) {
	switch s.kind {
	case "git":
		if s.opt == 0 {
			return view.SubscribeRequest{Kind: "git", URL: s.shown, RefType: "branch"}, true
		}
		if s.ref == "" {
			return view.SubscribeRequest{}, false
		}
		return view.SubscribeRequest{Kind: "git", URL: s.shown, RefType: []string{"", "branch", "tag", "commit"}[s.opt], Ref: s.ref}, true
	case "workshop":
		return view.SubscribeRequest{Kind: "workshop", URL: s.shown, SteamCMD: s.opt == 1}, true
	case "page", "loverslab":
		if s.main != "" {
			return view.SubscribeRequest{Kind: "page", URL: s.shown, Git: true}, true
		}
	}
	return view.SubscribeRequest{}, false
}

func requestKey(r view.SubscribeRequest) string {
	b, _ := json.Marshal(r)
	return string(b)
}

// recognise makes the card for the box's text: what it is, then its dry run (and for a
// page, what the page says) in the background.
func (m *model) recognise() tea.Cmd {
	s := &m.add
	s.seq++
	s.shown, s.kind = strings.TrimSpace(s.text), subscribe.Detect(s.text)
	s.opt, s.ref, s.field, s.info, s.main, s.reading = 0, "", 0, "", "", false
	s.lines, s.apply, s.previewOf = nil, false, ""
	if s.kind == "page" || s.kind == "loverslab" {
		s.reading, s.info = true, T("reading the page…")
		seq, url, kind := s.seq, s.shown, s.kind
		return tea.Batch(m.bg(func(a *app.App) func(m *model) tea.Cmd {
			p, err := subscribe.ReadPage(context.Background(), a.HTTP, url)
			return func(m *model) tea.Cmd {
				s := &m.add
				if m.tab != tabSubscribe || s.seq != seq {
					return nil
				}
				s.reading = false
				switch {
				case err != nil && kind == "loverslab":
					s.info = T("The public page could not be read; use the LoversLab session or downloaded files.")
				case err != nil:
					s.info = m.humanize(err.Error())
				default:
					var facts []string
					for _, f := range []string{p.Name, p.Version} {
						if f != "" {
							facts = append(facts, f)
						}
					}
					if !p.DateModified.IsZero() {
						facts = append(facts, p.DateModified.Format("2006-01-02"))
					}
					s.info = strings.Join(facts, "  ")
					s.main = p.Main
					if s.main == "" && len(p.Repos) == 1 {
						s.main = p.Repos[0]
					}
				}
				return m.preview()
			}
		}))
	}
	return m.preview()
}

// preview runs the dry run of the card's decision in the background; a reply for an older
// decision is dropped.
func (m *model) preview() tea.Cmd {
	s := &m.add
	s.lines, s.apply, s.previewOf = nil, false, ""
	r, ok := s.request()
	if !ok {
		return nil
	}
	args, err := view.SubscribeArgv(r)
	if err != nil {
		return nil
	}
	key := requestKey(r)
	runSub := m.runSubscribe()
	return m.bg(func(a *app.App) func(m *model) tea.Cmd {
		out, code := runSub(a, append(args, "--dry-run"))
		st, _ := a.Loadout()
		var lines []string
		for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
			lines = append(lines, view.Humanize(st, l))
		}
		return func(m *model) tea.Cmd {
			s := &m.add
			if r, ok := s.request(); m.tab != tabSubscribe || !ok || requestKey(r) != key {
				return nil
			}
			s.lines, s.apply, s.previewOf = lines, code == 0, key
			return nil
		}
	})
}

// runSubscribe is how subscribe commands run on a session (cmds.Execute; tests redirect).
func (m *model) runSubscribe() func(a *app.App, argv []string) (string, int) {
	m.taskList()
	return m.tasks.runSub
}

// addKey: the keys of 添加订阅. Letters go into the box (or the name field); ↑↓ choose, enter
// subscribes once the dry run is shown, esc clears the box and then goes back.
func (m *model) addKey(k tea.KeyMsg) tea.Cmd {
	s := &m.add
	edit := func(f func(string) string) tea.Cmd {
		if s.field == 1 {
			s.ref = strings.TrimSpace(f(s.ref))
			return m.preview()
		}
		s.text = f(s.text)
		s.seq++
		seq := s.seq
		return tea.Tick(addDebounce, func(time.Time) tea.Msg { return addTick{seq} })
	}
	switch k.Type {
	case tea.KeyRunes, tea.KeySpace:
		text := strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ").Replace(string(k.Runes))
		if k.Type == tea.KeySpace {
			text = " "
		}
		return edit(func(v string) string { return v + text })
	case tea.KeyBackspace:
		return edit(func(v string) string {
			if r := []rune(v); len(r) > 0 {
				return string(r[:len(r)-1])
			}
			return v
		})
	case tea.KeyCtrlU:
		return edit(func(string) string { return "" })
	}
	switch k.String() {
	case "esc":
		if s.field == 1 {
			s.field = 0
			return nil
		}
		if s.text != "" {
			m.add = addState{}
			return nil
		}
		m.tab = tabChanges
		m.loadChanges()
		return nil
	case "up", "down":
		n := len(s.options())
		if n == 0 || s.shown != strings.TrimSpace(s.text) {
			return nil
		}
		d := 1
		if k.String() == "up" {
			d = -1
		}
		s.opt = (s.opt + d + n) % n
		s.ref = ""
		if s.needsName() {
			s.field = 1
		} else {
			s.field = 0
		}
		return m.preview()
	case "tab", "shift+tab":
		if s.needsName() {
			s.field = 1 - s.field
			return nil
		}
		if k.String() == "tab" {
			return m.openLibrary()
		}
		return m.openChangesOf("", "")
	case "enter":
		if strings.TrimSpace(s.text) != s.shown {
			return m.recognise()
		}
		return m.subscribeNow()
	}
	return nil
}

// subscribeNow queues 订阅 when the dry run of the card's decision is shown and allowed.
func (m *model) subscribeNow() tea.Cmd {
	s := &m.add
	r, ok := s.request()
	switch {
	case s.kind == "":
		m.say(sayInfo, T("paste a GitHub repository, a LoversLab file page, a Workshop link or ID"))
		return nil
	case !ok && s.needsName():
		m.say(sayInfo, T(s.nameHint()))
		s.field = 1
		return nil
	case !ok:
		m.say(sayInfo, T("nothing to subscribe here yet"))
		return nil
	case s.previewOf != requestKey(r):
		m.say(sayInfo, T("wait for the dry run"))
		return nil
	case !s.apply:
		m.say(sayFail, T("this cannot be done now, esc goes back"))
		return nil
	}
	args, err := view.SubscribeArgv(r)
	if err != nil {
		m.say(sayFail, err.Error())
		return nil
	}
	obj := ""
	if len(args) > 2 {
		obj = args[2]
	}
	runSub := m.runSubscribe()
	cmd := m.queueSelf(view.TaskSubscribe, obj, args, func(a *app.App, _ string) (string, error) {
		out, code := runSub(a, args)
		if code != 0 {
			return "", errors.New(strings.TrimSpace(out))
		}
		if strings.Contains(out, "Steam downloads it; it appears after the next scan.") {
			return view.T("Steam downloads it; it appears after the next scan"), nil
		}
		return view.T("subscription finished"), nil
	})
	m.add = addState{}
	return cmd
}

func (s *addState) nameHint() string {
	switch s.opt {
	case 1:
		return "branch name"
	case 2:
		return "tag name"
	}
	return "full commit SHA"
}

// addLines draws 添加订阅: the box, then the card (or what the box accepts).
func (m *model) addLines(n int) []string {
	s := &m.add
	inner := max(m.w-4, 10)
	box := s.text
	if s.field == 0 {
		box += "_"
	}
	out := []string{"  " + sAccent.Render(markPrompt+" ") + sText.Render(fit(box, inner-2))}
	if s.text == "" {
		out = append(out, "  "+sDimS.Render(fit(T("paste a GitHub repository, a LoversLab file page, a Workshop link or ID"), inner)), "")
		for _, l := range []string{
			T("Git repository") + "  https://github.com/<author>/<repository>",
			"LoversLab  https://www.loverslab.com/files/file/…",
			T("Workshop item") + "  https://steamcommunity.com/sharedfiles/filedetails/?id=…",
			"",
			T("Local mods need no subscription: copy them into the Mods folder and the next scan finds them."),
		} {
			out = append(out, "  "+sSecond.Render(fit(l, inner)))
		}
		return out
	}
	out = append(out, "")
	if s.shown != strings.TrimSpace(s.text) || s.kind == "" {
		return out
	}
	if s.kind == "bad" {
		return append(out, "  "+sWarnS.Render(fit(T("Not recognised. Paste a GitHub or GitLab repository, a LoversLab file page, a web page with a download, Workshop links or IDs."), inner)))
	}
	kinds := map[string]string{"git": "Git repository", "loverslab": "LoversLab", "page": "web page", "workshop": "Workshop item"}
	out = append(out, "  "+sPill.Render(" "+T(kinds[s.kind])+" ")+"  "+sText.Render(fit(s.shown, inner-20)))
	if s.info != "" {
		out = append(out, "  "+sSecond.Render(fit(s.info, inner)))
	}
	if s.kind == "git" {
		out = append(out, "  "+sDimS.Render(T("track")))
	}
	for i, o := range s.options() {
		if i == s.opt {
			out = append(out, "  "+sAccent.Render(markSel+" ")+sSel.Render(fit(o, inner-2)))
		} else {
			out = append(out, "    "+sText.Render(fit(o, inner-2)))
		}
	}
	if s.needsName() {
		name := s.ref
		if s.field == 1 {
			name += "_"
		}
		out = append(out, "    "+sSecond.Render(T(s.nameHint())+": ")+sText.Render(name))
	}
	if (s.kind == "page" || s.kind == "loverslab") && !s.reading && s.main == "" {
		out = append(out, "  "+sWarnS.Render(fit(T("downloaded files and LoversLab downloads are chosen in the window"), inner)),
			"  "+sSecond.Render(fit(": archo subscribe page "+s.shown+" --from <zip|folder>", inner)))
	}
	if len(s.lines) > 0 {
		out = append(out, "")
		for _, l := range s.lines {
			out = append(out, "  "+sSecond.Render(width.Truncate(l, inner, "…")))
		}
	} else if _, ok := s.request(); ok {
		out = append(out, "", "  "+sDimS.Render(T("dry run…")))
	}
	return out
}

// addPairs: the hint line of 添加订阅.
func (m *model) addPairs() [][2]string {
	s := &m.add
	pairs := [][2]string{}
	if len(s.options()) > 1 {
		pairs = append(pairs, [2]string{"↑↓", T("choose")})
	}
	if s.needsName() {
		pairs = append(pairs, [2]string{"tab", T(s.nameHint())})
	}
	if s.apply {
		pairs = append(pairs, [2]string{"enter", T("subscribe")})
	}
	pairs = append(pairs, [2]string{"esc", T("clear, then back to subscriptions and updates")})
	return pairs
}

// ---- 订阅与更新's actions ----

// checkSources queues 检查更新 of sources (K: every one with mods; enter: the row's).
func (m *model) checkSources(keys []string) tea.Cmd {
	if len(keys) == 0 {
		m.say(sayInfo, T("no subscriptions to check"))
		return nil
	}
	var cmds []tea.Cmd
	for _, key := range keys {
		key := key
		cmds = append(cmds, m.queueSelf(view.TaskSourceCheck, "RimArcho", []string{"source-check", key}, func(a *app.App, _ string) (string, error) {
			return view.SourceCheckResult(a, key)
		}))
	}
	if len(keys) > 1 {
		m.say(sayBusy, Tf("queued %d update checks", len(keys)))
	}
	return tea.Batch(cmds...)
}

// allSources: the sources K checks — every one that has mods, but local (06 §2.9).
func (c *chState) allSources() []string {
	var keys []string
	if c.subscriptions != nil {
		for _, s := range c.subscriptions.Sources {
			if s.Count > 0 && s.Key != app.LibLocal {
				keys = append(keys, s.Key)
			}
		}
	}
	return keys
}

// updatePending previews 更新 of the rows and queues it on enter (u one, U every one shown).
// Download pages need the new files chosen in the window: they are left out and said so.
// Refused while a game runs (the dry run says so and enter does nothing).
func (m *model) updatePending(rows []view.PendingUpdate) tea.Cmd {
	var items []view.PendingUpdate
	pages := 0
	for _, r := range rows {
		if r.Kind == app.LibPage {
			pages++
		} else {
			items = append(items, r)
		}
	}
	if len(items) == 0 {
		if pages > 0 {
			m.say(sayInfo, T("a download page needs the new files: choose them in the window, or archo subscribe update --from <zip|folder>"))
		} else {
			m.say(sayInfo, T("no known updates; check a source above"))
		}
		return nil
	}
	title := T("update subscription")
	if len(items) > 1 {
		title = T("update all")
	}
	return m.previewPanel(title, func(a *app.App) ([]string, bool) {
		var lines []string
		apply := true
		st, _ := a.Loadout()
		for _, r := range items {
			var p *view.Preview
			var err error
			if r.Kind == app.LibWorkshop {
				p, err = view.WorkshopUpdatePreview(a, r.Ref)
			} else {
				p, err = view.LibPreview(a, view.OpSubUpdate, []string{r.Ref})
			}
			if len(items) > 1 {
				lines = append(lines, r.Name)
			}
			if err != nil {
				lines = append(lines, view.Humanize(st, err.Error()))
				apply = false
				continue
			}
			lines = append(lines, p.Lines...)
			apply = apply && p.Apply
		}
		if pages > 0 {
			lines = append(lines, Tf("%d download pages need a file first; they are not updated now", pages))
		}
		return lines, apply
	}, func(m *model) tea.Cmd {
		var refs, names []string
		var cmds []tea.Cmd
		for _, r := range items {
			if r.Kind == app.LibWorkshop {
				ref := r.Ref
				cmds = append(cmds, m.queueSelf(view.TaskWorkshopUpdate, r.Name, []string{"workshop-update", ref}, func(a *app.App, _ string) (string, error) {
					return view.T("Steam will download this Workshop update"), a.UpdateWorkshopClient(ref, false)
				}))
				continue
			}
			refs, names = append(refs, r.Ref), append(names, r.Name)
		}
		cmds = append(cmds, m.submitLane(view.OpSubUpdate, refs, names))
		return tea.Batch(cmds...)
	})
}

// removeSubscription previews 撤掉订阅 (the folder goes to the recycle bin; lists using it
// are warned about) and queues it on enter.
func (m *model) removeSubscription(ref, name string) tea.Cmd {
	return m.previewPanel(T("remove subscription")+"  "+name, func(a *app.App) ([]string, bool) {
		p, err := view.LibPreview(a, view.OpSubRemove, []string{ref})
		if err != nil {
			st, _ := a.Loadout()
			return []string{view.Humanize(st, err.Error())}, false
		}
		return p.Lines, p.Apply
	}, func(m *model) tea.Cmd { return m.submitLane(view.OpSubRemove, []string{ref}, []string{name}) })
}

// previewPanel opens a confirmation whose dry run is made in the background (06 §6, §3.1
// rule 9): "dry run…" at once, the text when it is in, enter runs do once it allows.
func (m *model) previewPanel(title string, dry func(a *app.App) ([]string, bool), do func(m *model) tea.Cmd) tea.Cmd {
	pn := &panel{title: title, lines: []string{T("dry run…")}}
	m.panel = pn
	return m.bg(func(a *app.App) func(m *model) tea.Cmd {
		lines, apply := dry(a)
		return func(m *model) tea.Cmd {
			if m.panel != pn {
				return nil // closed or replaced meanwhile
			}
			pn.lines = append([]string{}, lines...)
			if apply {
				pn.lines = append(pn.lines, "", Tf("enter: %s, esc: cancel", title))
				pn.onEnter = do
			} else {
				pn.lines = append(pn.lines, "", T("this cannot be done now, esc goes back"))
			}
			return nil
		}
	})
}

// subscriptionOf: the instance is a subscription RimArcho keeps (Git, download page,
// SteamCMD), so 撤掉订阅 applies.
func (m *model) subscriptionOf(id string) bool {
	if m.st == nil {
		return false
	}
	mod := m.st.Ix.ByID[id]
	return mod != nil && m.app.SubscriptionOf(mod) != nil
}

// openSource opens what 打开来源页 shows for a pending update: the compare view or the
// repository / download page, or the Workshop change notes.
func (m *model) openSource(r *view.PendingUpdate) tea.Cmd {
	if r.Kind == app.LibWorkshop || r.Kind == app.LibSteamCMD {
		if mod := m.st.Ix.ByID[r.ID]; mod != nil {
			return m.openChangeNotes(mod.WorkshopID())
		}
		return nil
	}
	mod := m.st.Ix.ByID[r.ID]
	if mod == nil {
		return nil
	}
	rec := m.app.SubscriptionOf(mod)
	if rec == nil {
		return nil
	}
	return m.open(view.SubscriptionSourceURL(rec))
}

func init() {
	view.Register(map[string]string{
		"nothing to subscribe here yet": "这里还没有可订阅的",
		"wait for the dry run":          "等预览出来",
		"dry run…":                      "预览中…",
		"downloaded files and LoversLab downloads are chosen in the window":                                             "下载好的文件和 LoversLab 下载要在窗口里选",
		"a download page needs the new files: choose them in the window, or archo subscribe update --from <zip|folder>": "下载页要先选新下载的文件：在窗口里选，或 archo subscribe update --from <zip|文件夹>",
		"check this source": "检查这个来源",
	})
}
