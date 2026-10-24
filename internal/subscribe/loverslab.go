package subscribe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/accounts"
	"golang.org/x/net/html"
)

// AuthError never includes response bodies, signed URLs, Cookie values or transport errors.
type AuthError struct{ State, Reason string }

func (e *AuthError) Error() string {
	return e.Reason + "；请打开下载页，在浏览器下载后选择 zip"
}

func authError(state, reason string) error { return &AuthError{state, reason} }

type SiteFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type FilePage struct {
	Files     []SiteFile `json:"files"`
	Agreement string     `json:"agreement,omitempty"`
	Reason    string     `json:"reason,omitempty"`
}
type siteAction struct {
	page, target, method string
	referer              string
	values               url.Values
	agreement            string
}

// LoversLab is separate from ReadPage. Requests and file downloads are serial; only
// observed site links are followed. No external CDN host has yet been verified, so
// off-site redirects fail closed to manual download (never suffix-match cloud hosts).
type LoversLab struct {
	mu      sync.Mutex
	session *accounts.Session
	client  *http.Client
	actions map[string]siteAction
	next    time.Time
}

func NewLoversLab(s *accounts.Session, transport http.RoundTripper) *LoversLab {
	l := &LoversLab{session: s, actions: map[string]siteAction{}}
	l.client = &http.Client{Timeout: 20 * time.Minute, Transport: transport, Jar: s.Jar()}
	l.client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !fileURL(r.URL) {
			return errors.New("redirect refused")
		}
		return nil
	}
	return l
}

func fileURL(u *url.URL) bool {
	return u != nil && u.Scheme == "https" && accounts.SiteHost(u.Hostname()) && u.Port() == "" && u.User == nil && !strings.Contains(strings.ToLower(u.Path), "login") && !strings.Contains(strings.ToLower(u.Path), "register")
}
func LoversLabPage(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && fileURL(u) && strings.HasPrefix(u.Path, "/files/file/") && u.RawQuery == "" && u.Fragment == ""
}

func (l *LoversLab) request(ctx context.Context, a siteAction) (*http.Response, error) {
	u, err := url.Parse(a.target)
	if err != nil || !fileURL(u) {
		return nil, authError("需浏览器验证", "站点跳转暂不支持")
	}
	if !l.session.HasCookies() {
		return nil, accounts.ErrSession
	}
	if time.Now().Before(l.next) {
		wait := time.Until(l.next)
		if wait > time.Second {
			return nil, authError("限额", "站点请求间隔或限额尚未结束，请稍后重试")
		}
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, authError("需登录", "会话请求已取消")
		case <-timer.C:
		}
	}
	method := a.method
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if method == http.MethodPost {
		body = strings.NewReader(a.values.Encode())
	}
	req, _ := http.NewRequestWithContext(ctx, method, a.target, body)
	req.Header.Set("User-Agent", "RimArcho (player initiated download)")
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		origin, _ := url.Parse(a.referer)
		if fileURL(origin) {
			req.Header.Set("Origin", origin.Scheme+"://"+origin.Host)
		}
	}
	if origin, e := url.Parse(a.referer); e == nil && fileURL(origin) {
		req.Header.Set("Referer", origin.String())
	}
	resp, err := l.client.Do(req)
	l.next = time.Now().Add(time.Second)
	if err != nil {
		if resp != nil {
			if target, e := resp.Location(); e == nil {
				if strings.Contains(strings.ToLower(target.Path), "login") && accounts.SiteHost(target.Hostname()) {
					_ = l.session.SetStatus("需登录", "")
					return nil, authError("需登录", "站点要求重新登录")
				}
				if !fileURL(target) {
					return nil, authError("需浏览器验证", "站点跳转暂不支持（下载主机尚未确认）")
				}
			}
		}
		return nil, authError("需浏览器验证", "会话请求未完成，请稍后重试")
	}
	if resp.StatusCode == 429 {
		if secs, e := time.ParseDuration(resp.Header.Get("Retry-After") + "s"); e == nil {
			l.next = time.Now().Add(max(secs, time.Second))
		} else if t, e := http.ParseTime(resp.Header.Get("Retry-After")); e == nil {
			l.next = t
		}
		resp.Body.Close()
		return nil, authError("限额", "站点达到请求限制，请等待后重试")
	}
	return resp, nil
}

func (l *LoversLab) page(ctx context.Context, a siteAction) (*FilePage, error) {
	resp, err := l.request(ctx, a)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return l.parsePage(resp, a)
}

func (l *LoversLab) parsePage(resp *http.Response, a siteAction) (*FilePage, error) {
	b, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(b) > 4<<20 {
		return nil, authError("需浏览器验证", "下载页无法读取")
	}
	low := strings.ToLower(string(b))
	if strings.Contains(low, "cf-chl-") || strings.Contains(low, "just a moment") || strings.Contains(low, "captcha") {
		_ = l.session.SetStatus("需浏览器验证", "")
		return nil, authError("需浏览器验证", "站点需要浏览器验证")
	}
	if resp.StatusCode == 401 || strings.Contains(low, `name="password"`) || strings.Contains(low, `name='password'`) {
		_ = l.session.SetStatus("需登录", "")
		return nil, authError("需登录", "会话已失效")
	}
	if resp.StatusCode != 200 {
		_ = l.session.SetStatus("需浏览器验证", "")
		return nil, authError("需浏览器验证", "站点拒绝访问（权限或浏览器验证）")
	}
	root, _ := html.Parse(strings.NewReader(string(b)))
	p := &FilePage{}
	base := resp.Request.URL
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			href := attr(n, "href")
			u, e := base.Parse(href)
			if e == nil && fileURL(u) && u.Query().Get("do") == "download" && (u.Path == base.Path || strings.HasPrefix(u.Path, "/files/")) {
				name := strings.TrimSpace(nodeText(n))
				if name == "" {
					name = "下载文件"
				}
				p.Files = append(p.Files, l.add(siteAction{page: a.page, target: u.String(), referer: base.String()}, name))
			}
		}
		if n.Type == html.ElementNode && n.Data == "form" {
			u, e := base.Parse(attr(n, "action"))
			method := strings.ToUpper(attr(n, "method"))
			if method == "" {
				method = "GET"
			}
			if e == nil && fileURL(u) && (method == "GET" || method == "POST") && (u.Query().Get("do") == "download" || strings.Contains(attr(n, "data-controller"), "download")) {
				values := url.Values{}
				var inputs func(*html.Node)
				inputs = func(x *html.Node) {
					if x.Type == html.ElementNode && (x.Data == "input" && (attr(x, "type") == "hidden" || attr(x, "type") == "submit" || attr(x, "type") == "checkbox") || x.Data == "button" && attr(x, "type") == "submit") && attr(x, "name") != "" {
						value := attr(x, "value")
						if value == "" {
							value = "1"
						}
						values.Set(attr(x, "name"), value)
					}
					for c := x.FirstChild; c != nil; c = c.NextSibling {
						inputs(c)
					}
				}
				inputs(n)
				agreement := strings.TrimSpace(nodeText(n))
				if len(agreement) > 12000 {
					return
				}
				p.Agreement = agreement
				if method == "GET" {
					q := u.Query()
					for k, v := range values {
						q[k] = v
					}
					u.RawQuery = q.Encode()
				}
				p.Files = append(p.Files, l.add(siteAction{page: a.page, target: u.String(), method: method, values: values, agreement: agreement, referer: base.String()}, "确认下载"))
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	if len(p.Files) == 0 {
		return nil, authError("需浏览器验证", "未识别到可下载文件，请使用浏览器")
	}
	return p, nil
}

func (l *LoversLab) add(a siteAction, name string) SiteFile {
	var b [16]byte
	_, _ = rand.Read(b[:])
	id := hex.EncodeToString(b[:])
	l.actions[id] = a
	return SiteFile{ID: id, Name: name}
}
func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func nodeText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	if n.Data == "input" && attr(n, "type") == "submit" {
		return attr(n, "value")
	}
	if n.Data == "script" || n.Data == "style" || n.Data == "input" {
		return ""
	}
	var s strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		s.WriteString(nodeText(c))
		s.WriteByte(' ')
	}
	return s.String()
}

func (l *LoversLab) Files(ctx context.Context, page string) (*FilePage, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !LoversLabPage(page) {
		return nil, authError("需登录", "请选择 LoversLab 文件页")
	}
	l.actions = map[string]siteAction{}
	return l.page(ctx, siteAction{page: page, target: page})
}

// Follow returns another selection/agree page or a temporary zip. Opaque IDs are
// memory-only, bound to this page; signed links and CSRF values never enter tasks.
func (l *LoversLab) Follow(ctx context.Context, page, id string, agreed bool, dir string) (*FilePage, string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.actions[id]
	if !ok || a.page != page {
		return nil, "", authError("需登录", "文件选择已过期，请重新读取")
	}
	if a.agreement != "" && !agreed {
		return &FilePage{Agreement: a.agreement, Files: []SiteFile{{ID: id, Name: "确认下载"}}}, "", nil
	}
	resp, err := l.request(ctx, a)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	typ, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if strings.Contains(typ, "html") || resp.StatusCode != 200 {
		// Parse this response without making a second request.
		p, e := l.parsePage(resp, a)
		return p, "", e
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, "", errors.New("无法创建下载临时目录")
	}
	f, err := os.CreateTemp(dir, "loverslab-*.zip")
	if err != nil {
		return nil, "", errors.New("无法创建下载临时文件")
	}
	name := f.Name()
	success := false
	defer func() {
		f.Close()
		if !success {
			os.Remove(name)
		}
	}()
	// The archive installer performs the actual zip/folder and About.xml validation.
	_, err = io.Copy(f, io.LimitReader(resp.Body, 8<<30))
	if err != nil {
		return nil, "", errors.New("下载中断，请重试或使用手动下载")
	}
	if err = f.Close(); err != nil {
		return nil, "", errors.New("下载文件写入失败")
	}
	if err := l.session.SetStatus("可用", ""); err != nil {
		return nil, "", err
	}
	success = true
	return nil, name, nil
}

// Verify makes exactly one GET; it never follows login redirects or fetches resources.
func (l *LoversLab) Verify(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	old := l.client.CheckRedirect
	l.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	defer func() { l.client.CheckRedirect = old }()
	resp, err := l.request(ctx, siteAction{target: accounts.Site + "?app=core&module=system&controller=settings"})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(b) > 4<<20 {
		return authError("需浏览器验证", "验证页无法读取")
	}
	low := strings.ToLower(string(b))
	state := "需登录"
	if resp.StatusCode != 200 {
		state = "需浏览器验证"
		if target, e := resp.Location(); e == nil && accounts.SiteHost(target.Hostname()) && strings.Contains(strings.ToLower(target.Path), "login") {
			state = "需登录"
		}
	}
	if strings.Contains(low, "cf-chl-") || strings.Contains(low, "just a moment") || strings.Contains(low, "captcha") {
		state = "需浏览器验证"
	} else if resp.StatusCode == 200 && strings.Contains(low, "do=logout") {
		state = "可用"
	}
	name := ""
	root, _ := html.Parse(strings.NewReader(string(b)))
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if attr(n, "id") == "elUserLink" {
			name = strings.TrimSpace(nodeText(n))
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	if len(name) > 120 {
		name = ""
	}
	if err := l.session.SetStatus(state, name); err != nil {
		return err
	}
	if state != "可用" {
		return authError(state, "会话需要玩家确认")
	}
	return nil
}
