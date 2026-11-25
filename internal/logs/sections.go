package logs

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const trimPrefix = "[RimArcho] Trimmed log middle: "
const trimSuffix = " bytes omitted"

// TrimMarker is the single line that replaces the middle of a trimmed log.
func TrimMarker(omitted int64) string {
	return fmt.Sprintf("%s%d%s\n", trimPrefix, omitted, trimSuffix)
}

// ParseSections reads a file's retained sections. After a gap, offsets in the
// returned File count analysed lines and the phase is unknown until a marker.
func ParseSections(head io.Reader, tail io.Reader, omitted int64) (*File, error) {
	p := NewParser()
	p.compact = make(map[entrySampleKey]entrySamples)
	p.F.OmittedBytes = omitted
	if err := p.readSection(head, false); err != nil {
		return nil, err
	}
	if tail != nil {
		p.Gap()
		if err := p.readSection(tail, true); err != nil {
			return nil, err
		}
	}
	resolveRefs(p.F)
	return p.F, nil
}

// Gap ends the current entry and forgets state that the omitted middle may have changed.
func (p *Parser) Gap() {
	p.Close()
	p.phase, p.afterSubnode, p.hugsHeader = UnknownPhase, false, false
}

// Continuation identifies lines belonging to an earlier Unity log entry.
func Continuation(line string) bool { return isContinuation(line) }

func (p *Parser) readSection(r io.Reader, tail bool) error {
	b := bufio.NewReaderSize(r, 4096)
	const maxLine = 4096
	for {
		line, err := b.ReadSlice('\n')
		if len(line) > 0 {
			text := strings.TrimRight(string(line[:min(len(line), maxLine)]), "\r\n")
			if value, ok := strings.CutPrefix(text, trimPrefix); ok {
				number, _ := strings.CutSuffix(value, trimSuffix)
				if n, parseErr := strconv.ParseInt(number, 10, 64); parseErr == nil && n > 0 && number != value {
					p.Gap()
					p.F.Trimmed = true
					p.F.OmittedBytes += n
					p.F.Lines++
					tail = true
					continue
				}
			}
			// The section can begin inside a stack, after the initial partial line.
			if tail && Continuation(text) {
				p.F.Lines++
			} else {
				p.Line(text)
				tail = false
			}
		}
		for errors.Is(err, bufio.ErrBufferFull) {
			_, err = b.ReadSlice('\n')
		}
		if err != nil {
			p.Close()
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

type entrySampleKey struct {
	phase *Phase
	text  string
}

type entrySamples struct {
	last *Entry
	kept int
}

func (p *Parser) addEntry(e *Entry) {
	if p.compact == nil {
		p.F.Entries = append(p.F.Entries, e)
	}
}

func (p *Parser) keepSample(e *Entry) {
	var b strings.Builder
	add := func(s string) {
		b.WriteString(strconv.Itoa(len(s)))
		b.WriteByte(':')
		b.WriteString(s)
	}
	add(e.Message)
	add(e.Level)
	add(e.Ref)
	if e.Dup {
		b.WriteByte('d')
	}
	for _, x := range e.Extra {
		b.WriteByte('x')
		add(x)
	}
	for _, fr := range e.Frames {
		b.WriteByte('f')
		add(fr.Text)
		if fr.Caller {
			b.WriteByte('c')
		}
		for _, patch := range fr.Harmony {
			b.WriteByte('p')
			add(patch.Text)
		}
	}
	key := entrySampleKey{e.Phase, b.String()}
	sample := p.compact[key]
	if sample.kept == 5 {
		sample.last.count++
		return
	}
	p.F.Entries = append(p.F.Entries, e)
	p.compact[key] = entrySamples{last: e, kept: sample.kept + 1}
}
