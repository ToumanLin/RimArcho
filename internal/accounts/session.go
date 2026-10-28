package accounts

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"
)

const Site = "https://www.loverslab.com/"

var ErrSession = errors.New("login required; open the login window or download a zip in your browser")

// Status contains derived, non-secret information for the player interface.
type Status struct {
	State    string    `json:"state"`
	Name     string    `json:"name"`
	Verified time.Time `json:"verified"`
	Expiry   time.Time `json:"expiry"`
}

// Session's cookies are intentionally unexported and never JSON-serialized by callers.
type Session struct {
	title    string
	dir      string
	cookies  []*http.Cookie
	status   Status
	revision [32]byte
	exists   bool
}

type envelope struct {
	Cookies []*http.Cookie
	Status  Status
}

func SiteHost(host string) bool {
	host = strings.ToLower(host)
	return host == "loverslab.com" || host == "www.loverslab.com"
}

func cookieDomain(domain string) bool { return SiteHost(strings.TrimPrefix(domain, ".")) }

func Open(dir string) (*Session, error) {
	if safePath(dir) != nil || safePath(filepath.Join(dir, "loverslab.session")) != nil {
		return nil, ErrSession
	}
	s := &Session{dir: dir, status: Status{State: "not connected"}}
	b, err := os.ReadFile(filepath.Join(dir, "loverslab.session"))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, ErrSession
	}
	s.revision, s.exists = sha256.Sum256(b), true
	b, err = unprotect(b)
	if err != nil {
		return nil, ErrSession
	}
	defer clear(b)
	var e envelope
	if json.Unmarshal(b, &e) != nil {
		return nil, ErrSession
	}
	s.accept(e.Cookies)
	s.status = e.Status
	switch s.status.State {
	case "未连接":
		s.status.State = "not connected"
	case "需登录":
		s.status.State = "login required"
	case "需浏览器验证":
		s.status.State = "browser verification required"
	case "限额":
		s.status.State = "rate limited"
	case "可用":
		s.status.State = "available"
	}
	if len(s.cookies) == 0 {
		s.status.State = "login required"
	}
	return s, nil
}

func (s *Session) accept(cookies []*http.Cookie) {
	s.cookies = nil
	for _, c := range cookies {
		if c != nil && c.MaxAge >= 0 && cookieDomain(c.Domain) && (c.Expires.IsZero() || c.Expires.After(time.Now())) {
			copy := *c
			s.cookies = append(s.cookies, &copy)
		}
	}
}

func (s *Session) Status() Status {
	r := s.status
	r.Expiry = time.Time{}
	if len(s.cookies) == 0 && r.State == "" {
		r.State = "not connected"
	}
	for _, c := range s.cookies {
		name := strings.ToLower(c.Name)
		// Invision's member/login/device keys keep the login alive after the short
		// session expires. Analytics/preferences must not extend the displayed expiry.
		auth := strings.Contains(name, "member_id") || strings.Contains(name, "login_key") || strings.Contains(name, "device_key") || strings.Contains(name, "session")
		if auth && c.MaxAge >= 0 && c.Expires.After(time.Now()) && c.Expires.After(r.Expiry) {
			r.Expiry = c.Expires
		}
	}
	if !s.HasCookies() && r.State != "not connected" {
		r.State = "login required"
	}
	return r
}

// RenewalExpiry excludes the short session cookie: remembered authentication can
// recreate it. Use the earliest durable login key, not the latest preference or
// device cookie, when deciding when to try renewal.
func (s *Session) RenewalExpiry() time.Time {
	var expiry time.Time
	for _, c := range s.cookies {
		name := strings.ToLower(c.Name)
		if c.MaxAge < 0 || c.Expires.IsZero() || !(strings.Contains(name, "member_id") || strings.Contains(name, "login_key") || strings.Contains(name, "device_key")) {
			continue
		}
		if expiry.IsZero() || c.Expires.Before(expiry) {
			expiry = c.Expires
		}
	}
	if expiry.IsZero() {
		expiry = s.Status().Expiry
	}
	return expiry
}

func (s *Session) SetStatus(state, name string) error {
	old := s.status
	s.status.State, s.status.Verified = state, time.Now().UTC()
	if name != "" {
		s.status.Name = name
	}
	if err := s.save(); err != nil {
		s.status = old
		return err
	}
	return nil
}

func (s *Session) Save() error { return s.save() }

func (s *Session) save() error {
	if !s.current() {
		return ErrSession
	}
	s.status.Expiry = s.Status().Expiry
	// Keep session cookies too: the native login host exits before the API uses
	// them. They retain no expiry, and are invalidated by the site or logout.
	var persistent []*http.Cookie
	for _, c := range s.cookies {
		if c.MaxAge >= 0 && (c.Expires.IsZero() || c.Expires.After(time.Now())) {
			persistent = append(persistent, c)
		}
	}
	b, err := json.Marshal(envelope{persistent, s.status})
	if err != nil {
		return ErrSession
	}
	defer clear(b)
	b, err = protect(b)
	if err != nil {
		return ErrSession
	}
	if err = privateDir(s.dir); err != nil {
		return ErrSession
	}
	p := filepath.Join(s.dir, "loverslab.session")
	if safePath(p) != nil || safePath(p+".new") != nil {
		return ErrSession
	}
	if err = os.WriteFile(p+".new", b, 0600); err != nil {
		return ErrSession
	}
	if err = os.Rename(p+".new", p); err != nil {
		_ = os.Remove(p + ".new")
		return ErrSession
	}
	s.revision, s.exists = sha256.Sum256(b), true
	return nil
}

// Capture is called only by the native CookieManager completion handler.
func (s *Session) capture(cookies []*http.Cookie) error {
	s.accept(cookies)
	s.status.State = "login required"
	return s.save()
}

// Jar restricts both incoming and outgoing cookies, including hostile Domain attributes.
// CDN requests never receive site credentials. The client does not use browser databases.
func (s *Session) Jar() http.CookieJar {
	j, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	r := &restrictedJar{jar: j, session: s}
	for _, c := range s.cookies {
		copy := *c
		copy.MaxAge = 0 // Expires is absolute; restart must not extend Max-Age.
		u := &url.URL{Scheme: "https", Host: strings.TrimPrefix(c.Domain, "."), Path: "/"}
		if !strings.HasPrefix(c.Domain, ".") {
			copy.Domain = "" // WebView2 represents host-only cookies without a dot.
		}
		j.SetCookies(u, []*http.Cookie{&copy})
	}
	return r
}

type restrictedJar struct {
	jar     http.CookieJar
	session *Session
}

func (j *restrictedJar) Cookies(u *url.URL) []*http.Cookie {
	if u.Scheme != "https" || u.User != nil || u.Port() != "" || !SiteHost(u.Hostname()) || !j.session.current() {
		return nil
	}
	return j.jar.Cookies(u)
}
func (j *restrictedJar) SetCookies(u *url.URL, cs []*http.Cookie) {
	if u.Scheme != "https" || u.User != nil || u.Port() != "" || !SiteHost(u.Hostname()) {
		return
	}
	var allowed []*http.Cookie
	for _, c := range cs {
		if c == nil {
			continue
		}
		if c.Domain != "" && !cookieDomain(c.Domain) {
			continue
		}
		copy := *c
		if copy.Domain == "" {
			copy.Domain = u.Hostname()
		} else {
			copy.Domain = "." + strings.TrimPrefix(copy.Domain, ".")
			if u.Hostname() != strings.TrimPrefix(copy.Domain, ".") && !strings.HasSuffix(u.Hostname(), copy.Domain) {
				continue
			}
		}
		allowed = append(allowed, c)
		if copy.Path == "" || copy.Path[0] != '/' {
			if i := strings.LastIndex(u.Path, "/"); i >= 0 {
				copy.Path = u.Path[:i]
			} else {
				copy.Path = "/"
			}
			if copy.Path == "" {
				copy.Path = "/"
			}
		}
		if copy.MaxAge > 0 {
			copy.Expires = time.Now().Add(time.Duration(copy.MaxAge) * time.Second)
		}
		found := false
		for i, old := range j.session.cookies {
			if old.Name == copy.Name && old.Domain == copy.Domain && old.Path == copy.Path {
				j.session.cookies[i] = &copy
				found = true
				break
			}
		}
		if !found {
			j.session.cookies = append(j.session.cookies, &copy)
		}
	}
	j.jar.SetCookies(u, allowed)
}

func (s *Session) Forget() error {
	s.cookies = nil
	s.status = Status{State: "not connected"}
	if err := safeTree(s.dir); err != nil {
		return ErrSession
	}
	if err := os.RemoveAll(s.dir); err != nil {
		return errors.New("session cleanup failed; close the login window and retry")
	}
	s.exists, s.revision = false, [32]byte{}
	return nil
}

// Forget also removes crash leftovers without needing to decrypt the session.
func Forget(dir string) error { return (&Session{dir: dir}).Forget() }

func (s *Session) HasCookies() bool {
	if !s.current() {
		return false
	}
	for _, c := range s.cookies {
		if c.MaxAge >= 0 && (c.Expires.IsZero() || c.Expires.After(time.Now())) {
			return true
		}
	}
	return false
}

// A forgotten or replaced session must invalidate clients in other app processes.
func (s *Session) current() bool {
	b, err := os.ReadFile(filepath.Join(s.dir, "loverslab.session"))
	if errors.Is(err, os.ErrNotExist) {
		return !s.exists
	}
	return err == nil && s.exists && sha256.Sum256(b) == s.revision
}

func safePath(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return ErrSession
	}
	for p := path; ; p = filepath.Dir(p) {
		fi, e := os.Lstat(p)
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return ErrSession
		}
		if e == nil && fi.Mode()&os.ModeSymlink != 0 {
			return ErrSession
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return nil
}

func safeTree(dir string) error {
	if err := safePath(dir); err != nil {
		return err
	}
	return filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil || d.Type()&os.ModeSymlink != 0 {
			return ErrSession
		}
		return nil
	})
}
