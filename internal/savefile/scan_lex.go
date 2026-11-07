package savefile

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"unicode/utf8"
)

// The scanner is a byte-level XML tokenizer: encoding/xml's Decoder.Token allocates per
// token and is several times too slow for a 27 MB save (12 §1.7). It reads a sliding
// buffer, keeps only the element stack and the text of the element opened last (up to a
// cap), and reports the first syntax error the way the game's XmlDocument load would fail.
// It is not a validating parser: it checks tag structure, attribute syntax, entities in
// the text it keeps, NUL bytes, and what follows the root element; it does not check name
// characters beyond the first or control characters other than NUL.

const (
	bufSize    = 256 << 10
	maxTagLen  = 1 << 20  // a longer tag is reported malformed
	shortText  = 512      // text kept for any element while it may be a leaf
	maxCapture = 64 << 20 // text kept for elements whose whole text is needed
)

const (
	fHasChild uint8 = 1 << iota // a child element opened, so this is not a leaf
	fNull                       // IsNull="True"
)

type frame struct {
	name  int32
	node  int32 // this element's rule node (scan_rules.go), or -1
	flags uint8
	ctx   uint8 // where a pawn found under this element is (ctx* constants)
	def   int32 // def index of a direct <def> child, or -1
	pawn  int32 // index into scanner.pawns when this element is a pawn, else -1
	obj   int32 // index of the record this element opened (faction, map…), or -1
	id    []byte
	fac   []byte
}

type scanner struct {
	r     io.Reader
	buf   []byte
	pos   int   // next unread byte in buf
	end   int   // valid bytes in buf
	base  int64 // file offset of buf[0]
	lines int   // newlines before buf[0]
	eof   bool
	rerr  error
	bad   bool // malformed: stop reading

	stack    []frame
	depth    int
	sawRoot  bool
	rootDone bool

	// Text of the element opened last, while it may still be a leaf.
	text     []byte
	textOn   bool
	textOver bool
	textRaw  bool // contains CDATA, which is not entity-decoded
	textCap  int
	scratch  []byte

	names   map[string]int32
	nameStr []string

	f *Facts
	x extract
}

func newScanner(r io.Reader) *scanner {
	s := &scanner{
		r:     r,
		buf:   make([]byte, bufSize),
		names: make(map[string]int32, len(knownNames)),
		f:     &Facts{Parser: ParserVersion},
	}
	for i, n := range knownNames {
		s.names[n] = int32(i)
		s.nameStr = append(s.nameStr, n)
	}
	s.x.init()
	return s
}

func (s *scanner) run() error {
	s.fill()
	if s.end-s.pos >= 3 && s.buf[0] == 0xEF && s.buf[1] == 0xBB && s.buf[2] == 0xBF {
		s.pos = 3
	}
	for !s.bad {
		seg := s.buf[s.pos:s.end]
		i := bytes.IndexByte(seg, '<')
		if i >= 0 {
			seg = seg[:i]
		}
		if len(seg) > 0 {
			s.onText(seg, s.pos)
			if s.bad {
				break
			}
			s.pos += len(seg)
		}
		if i < 0 {
			if !s.fill() {
				break
			}
			continue
		}
		s.tag()
	}
	s.f.Bytes = s.base + int64(s.end)
	if s.rerr != nil {
		return s.rerr
	}
	if !s.bad {
		switch {
		case s.depth > 0:
			s.failEOF(fmt.Sprintf("the file ends inside <%s> (%d elements open)", s.nameStr[s.stack[s.depth-1].name], s.depth))
		case !s.sawRoot:
			s.failEOF("no root element")
		}
	}
	return nil
}

// fill moves the unread bytes to the front of the buffer and reads more; it grows the
// buffer when a single tag fills it. It reports whether new bytes arrived.
func (s *scanner) fill() bool {
	if s.eof {
		return false
	}
	if s.pos > 0 {
		s.lines += bytes.Count(s.buf[:s.pos], []byte{'\n'})
		s.base += int64(s.pos)
		s.end = copy(s.buf, s.buf[s.pos:s.end])
		s.pos = 0
	}
	if s.end == len(s.buf) {
		if len(s.buf) >= maxTagLen {
			s.fail(s.pos, "a tag is longer than 1 MB")
			return false
		}
		nb := make([]byte, 2*len(s.buf))
		copy(nb, s.buf[:s.end])
		s.buf = nb
	}
	for {
		n, err := s.r.Read(s.buf[s.end:])
		s.end += n
		if err != nil {
			s.eof = true
			if err != io.EOF {
				s.rerr = err
			}
			return n > 0
		}
		if n > 0 {
			return true
		}
	}
}

// need makes at least n bytes from pos available; false at the end of the file.
func (s *scanner) need(n int) bool {
	for s.end-s.pos < n {
		if !s.fill() {
			return false
		}
	}
	return true
}

func (s *scanner) fail(at int, msg string) {
	if s.bad {
		return
	}
	s.bad = true
	if at > s.end {
		at = s.end
	}
	line := s.lines + bytes.Count(s.buf[:at], []byte{'\n'}) + 1
	s.f.Malformed = &Malformed{Offset: s.base + int64(at), Line: line, Msg: msg}
}

func (s *scanner) failEOF(msg string) {
	if s.rerr != nil {
		return
	}
	s.fail(s.end, msg)
	if s.f.Malformed != nil {
		s.f.Malformed.EOF = true
	}
}

func (s *scanner) onText(seg []byte, at int) {
	if j := bytes.IndexByte(seg, 0); j >= 0 {
		s.fail(at+j, "NUL byte")
		return
	}
	if s.depth == 0 {
		for j, c := range seg {
			if !isSpace(c) {
				s.fail(at+j, "text outside the root element")
				return
			}
		}
		return
	}
	s.addText(seg)
}

func (s *scanner) addText(seg []byte) {
	if !s.textOn || s.textOver {
		return
	}
	if len(s.text)+len(seg) > s.textCap {
		s.textOver = true
		s.text = s.text[:0]
		return
	}
	s.text = append(s.text, seg...)
}

func (s *scanner) tag() {
	if !s.need(2) {
		s.failEOF("the file ends inside a tag")
		return
	}
	switch s.buf[s.pos+1] {
	case '/':
		s.endTag()
	case '?':
		s.skipPast("?>", 2, nil)
	case '!':
		s.bang()
	default:
		s.startTag()
	}
}

// bang handles <!-- -->, <![CDATA[ ]]> and <!DOCTYPE …>.
func (s *scanner) bang() {
	s.need(9)
	rest := s.buf[s.pos:s.end]
	if s.eof && len(rest) < 9 {
		for _, p := range []string{"<!--", "<![CDATA[", "<!DOCTYPE"} {
			if len(rest) < len(p) && p[:len(rest)] == string(rest) {
				s.failEOF("the file ends inside markup")
				return
			}
		}
	}
	switch {
	case bytes.HasPrefix(rest, []byte("<!--")):
		s.skipPast("-->", 4, nil)
	case bytes.HasPrefix(rest, []byte("<![CDATA[")):
		if s.depth == 0 {
			s.fail(s.pos, "CDATA outside the root element")
			return
		}
		s.textRaw = true
		s.skipPast("]]>", 9, s.addText)
	case bytes.HasPrefix(rest, []byte("<!DOCTYPE")):
		if s.sawRoot {
			s.fail(s.pos, "DOCTYPE inside the document")
			return
		}
		s.skipPast(">", 9, nil) // an internal subset is not supported (saves have none)
	default:
		s.fail(s.pos, "invalid markup after '<!'")
	}
}

// skipPast consumes from pos through the end sequence seq, which starts searching skip
// bytes after pos; body, when set, receives the bytes in between.
func (s *scanner) skipPast(seq string, skip int, body func([]byte)) {
	if !s.need(skip) {
		s.failEOF("the file ends inside markup")
		return
	}
	s.pos += skip
	for {
		i := bytes.Index(s.buf[s.pos:s.end], []byte(seq))
		if i >= 0 {
			if body != nil {
				body(s.buf[s.pos : s.pos+i])
			}
			s.pos += i + len(seq)
			return
		}
		// Keep the last len(seq)-1 bytes: the sequence may straddle the refill.
		keep := len(seq) - 1
		if n := s.end - s.pos - keep; n > 0 {
			if body != nil {
				body(s.buf[s.pos : s.pos+n])
			}
			s.pos += n
		}
		if !s.fill() {
			s.failEOF("the file ends inside markup")
			return
		}
	}
}

// tagEnd returns the index of the '>' closing the tag at pos, skipping quoted attribute
// values, or -1 when the buffer ends first.
func (s *scanner) tagEnd() int {
	b := s.buf[s.pos:s.end]
	j := bytes.IndexByte(b, '>')
	if j < 0 {
		return -1
	}
	if bytes.IndexByte(b[:j], '"') < 0 && bytes.IndexByte(b[:j], '\'') < 0 {
		return s.pos + j
	}
	var q byte
	for k := 1; k < len(b); k++ {
		c := b[k]
		switch {
		case q != 0:
			if c == q {
				q = 0
			}
		case c == '"' || c == '\'':
			q = c
		case c == '>':
			return s.pos + k
		}
	}
	return -1
}

func (s *scanner) startTag() {
	j := s.tagEnd()
	for j < 0 {
		if !s.fill() {
			if !s.bad {
				s.failEOF("the file ends inside a tag")
			}
			return
		}
		j = s.tagEnd()
	}
	at := s.pos
	t := s.buf[s.pos+1 : j]
	selfClose := len(t) > 0 && t[len(t)-1] == '/'
	if selfClose {
		t = t[:len(t)-1]
	}
	k := 0
	for k < len(t) && !isSpace(t[k]) {
		k++
	}
	name := t[:k]
	if !validName(name) || bytes.ContainsAny(name, "<>/\"'=&") {
		s.fail(at+1, fmt.Sprintf("invalid element name %q", truncate(name)))
		return
	}
	class, null, ok := s.attrs(t[k:], at+1+k)
	if !ok {
		return
	}
	if s.rootDone {
		s.fail(at, "a second root element")
		return
	}
	s.pos = j + 1
	s.open(name, class, null)
	if selfClose {
		s.close()
	}
}

// attrs parses the attributes of a start tag and returns Class and IsNull.
func (s *scanner) attrs(b []byte, at int) (class []byte, null, ok bool) {
	i := 0
	for {
		for i < len(b) && isSpace(b[i]) {
			i++
		}
		if i == len(b) {
			return class, null, true
		}
		if i == 0 {
			s.fail(at, "missing space before an attribute")
			return nil, false, false
		}
		st := i
		for i < len(b) && b[i] != '=' && !isSpace(b[i]) {
			i++
		}
		name := b[st:i]
		for i < len(b) && isSpace(b[i]) {
			i++
		}
		if len(name) == 0 || i == len(b) || b[i] != '=' {
			s.fail(at+st, fmt.Sprintf("attribute %q has no value", truncate(name)))
			return nil, false, false
		}
		i++
		for i < len(b) && isSpace(b[i]) {
			i++
		}
		if i == len(b) || (b[i] != '"' && b[i] != '\'') {
			s.fail(at+i, fmt.Sprintf("attribute %q value is not quoted", truncate(name)))
			return nil, false, false
		}
		q := b[i]
		i++
		vs := i
		for i < len(b) && b[i] != q {
			i++
		}
		if i == len(b) {
			s.fail(at+vs, "unterminated attribute value")
			return nil, false, false
		}
		v := b[vs:i]
		i++
		if bytes.IndexByte(v, '<') >= 0 {
			s.fail(at+vs, "'<' in an attribute value")
			return nil, false, false
		}
		switch string(name) {
		case "Class":
			if bytes.IndexByte(v, '&') >= 0 {
				d, err := decodeEntities(nil, v)
				if err != "" {
					s.fail(at+vs, err)
					return nil, false, false
				}
				v = d
			}
			class = v
		case "IsNull":
			null = string(v) == "True"
		}
	}
}

func (s *scanner) endTag() {
	at := s.pos
	var j int
	for {
		j = bytes.IndexByte(s.buf[s.pos:s.end], '>')
		if j >= 0 {
			j += s.pos
			break
		}
		if !s.fill() {
			if !s.bad {
				s.failEOF("the file ends inside an end tag")
			}
			return
		}
		at = s.pos
	}
	name := bytes.TrimRight(s.buf[s.pos+2:j], " \t\r\n")
	if s.depth == 0 {
		s.fail(at, fmt.Sprintf("end tag </%s> without a start tag", truncate(name)))
		return
	}
	top := s.stack[s.depth-1].name
	if string(name) != s.nameStr[top] {
		s.fail(at, fmt.Sprintf("end tag </%s> does not match <%s>", truncate(name), s.nameStr[top]))
		return
	}
	s.pos = j + 1
	s.close()
}

func (s *scanner) intern(name []byte) int32 {
	if id, ok := s.names[string(name)]; ok {
		return id
	}
	id := int32(len(s.nameStr))
	n := string(name)
	s.names[n] = id
	s.nameStr = append(s.nameStr, n)
	return id
}

func (s *scanner) open(name, class []byte, null bool) {
	id := s.intern(name)
	var parent *frame
	if s.depth > 0 {
		parent = &s.stack[s.depth-1]
		parent.flags |= fHasChild
	}
	if s.depth == len(s.stack) {
		s.stack = append(s.stack, frame{})
	}
	f := &s.stack[s.depth]
	s.depth++
	f.name, f.flags, f.def, f.pawn, f.obj = id, 0, -1, -1, -1
	f.id, f.fac = f.id[:0], f.fac[:0]
	if null {
		f.flags |= fNull
	}
	if parent != nil {
		f.ctx = parent.ctx
		f.node = childNode(parent.node, id)
	} else {
		f.ctx = ctxOther
		f.node = childNode(rootNode, id)
		s.sawRoot = true
	}
	if len(class) > 0 {
		s.x.class(class)
	}
	s.text, s.textOn, s.textOver, s.textRaw = s.text[:0], true, false, false
	s.textCap = shortText
	if f.node >= 0 {
		if ruleNodes[f.node].capture {
			s.textCap = maxCapture
		}
		s.x.open(s, f, parent, class)
	}
}

func (s *scanner) close() {
	f := &s.stack[s.depth-1]
	var parent *frame
	if s.depth > 1 {
		parent = &s.stack[s.depth-2]
	}
	if s.textOn && f.flags&fHasChild == 0 && !s.textOver {
		v := s.text
		if !s.textRaw && bytes.IndexByte(v, '&') >= 0 {
			d, err := decodeEntities(s.scratch[:0], v)
			if err != "" {
				s.fail(s.pos, err)
				return
			}
			s.scratch = d
			v = d
		}
		s.x.leaf(s, f, parent, v)
	}
	if f.node >= 0 || f.pawn >= 0 || f.obj >= 0 {
		s.x.close(s, f, parent)
	}
	s.depth--
	s.textOn = false
	if s.depth == 0 {
		s.rootDone = true
	}
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

func validName(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	c := b[0]
	return c == '_' || c == ':' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c >= 0x80
}

func truncate(b []byte) string {
	if len(b) > 40 {
		b = b[:40]
	}
	return string(b)
}

// decodeEntities appends v to dst with the five predefined entities and character
// references replaced; it returns a message for an entity XmlDocument would reject.
func decodeEntities(dst, v []byte) ([]byte, string) {
	for len(v) > 0 {
		i := bytes.IndexByte(v, '&')
		if i < 0 {
			return append(dst, v...), ""
		}
		dst = append(dst, v[:i]...)
		v = v[i:]
		j := bytes.IndexByte(v, ';')
		if j < 0 || j > 12 {
			return dst, "'&' not followed by an entity"
		}
		ent := v[1:j]
		v = v[j+1:]
		switch string(ent) {
		case "lt":
			dst = append(dst, '<')
		case "gt":
			dst = append(dst, '>')
		case "amp":
			dst = append(dst, '&')
		case "quot":
			dst = append(dst, '"')
		case "apos":
			dst = append(dst, '\'')
		default:
			if len(ent) < 2 || ent[0] != '#' {
				return dst, fmt.Sprintf("undeclared entity &%s;", truncate(ent))
			}
			var n uint64
			var err error
			if ent[1] == 'x' {
				n, err = strconv.ParseUint(string(ent[2:]), 16, 32)
			} else {
				n, err = strconv.ParseUint(string(ent[1:]), 10, 32)
			}
			if err != nil || n == 0 || n > utf8.MaxRune {
				return dst, fmt.Sprintf("invalid character reference &%s;", truncate(ent))
			}
			dst = utf8.AppendRune(dst, rune(n))
		}
	}
	return dst, ""
}
