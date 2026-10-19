// Package vdf parses Valve KeyValues text files (libraryfolders.vdf, appworkshop_*.acf).
package vdf

import (
	"bytes"
	"fmt"
	"strings"
)

// Node is either a string value (Children nil) or a nested object.
type Node struct {
	Key      string
	Value    string
	Children []*Node
}

// Get returns the first child with the given key (case-insensitive), or nil.
func (n *Node) Get(key string) *Node {
	if n == nil {
		return nil
	}
	for _, c := range n.Children {
		if strings.EqualFold(c.Key, key) {
			return c
		}
	}
	return nil
}

// Str returns the string value at the given path, or "".
func (n *Node) Str(path ...string) string {
	for _, p := range path {
		n = n.Get(p)
	}
	if n == nil {
		return ""
	}
	return n.Value
}

// Parse parses a KeyValues document and returns a synthetic root whose children are the
// top-level entries.
func Parse(b []byte) (*Node, error) {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	p := &parser{b: b}
	root := &Node{}
	if err := p.object(root, true); err != nil {
		return nil, err
	}
	return root, nil
}

type parser struct {
	b   []byte
	pos int
}

func (p *parser) skip() {
	for p.pos < len(p.b) {
		c := p.b[p.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			p.pos++
		case c == '/' && p.pos+1 < len(p.b) && p.b[p.pos+1] == '/':
			for p.pos < len(p.b) && p.b[p.pos] != '\n' {
				p.pos++
			}
		default:
			return
		}
	}
}

// token returns the next token; kind is '"' for a string, '{' or '}' for braces, 0 at EOF.
func (p *parser) token() (string, byte, error) {
	p.skip()
	if p.pos >= len(p.b) {
		return "", 0, nil
	}
	c := p.b[p.pos]
	switch c {
	case '{', '}':
		p.pos++
		return "", c, nil
	case '"':
		p.pos++
		var sb strings.Builder
		for p.pos < len(p.b) {
			c := p.b[p.pos]
			if c == '\\' && p.pos+1 < len(p.b) {
				n := p.b[p.pos+1]
				switch n {
				case 'n':
					sb.WriteByte('\n')
				case 't':
					sb.WriteByte('\t')
				case '\\', '"':
					sb.WriteByte(n)
				default:
					sb.WriteByte('\\')
					sb.WriteByte(n)
				}
				p.pos += 2
				continue
			}
			if c == '"' {
				p.pos++
				return sb.String(), '"', nil
			}
			sb.WriteByte(c)
			p.pos++
		}
		return "", 0, fmt.Errorf("vdf: unterminated string")
	default:
		start := p.pos
		for p.pos < len(p.b) && !strings.ContainsRune(" \t\r\n{}\"", rune(p.b[p.pos])) {
			p.pos++
		}
		return string(p.b[start:p.pos]), '"', nil
	}
}

func (p *parser) object(n *Node, top bool) error {
	for {
		key, kind, err := p.token()
		if err != nil {
			return err
		}
		switch kind {
		case 0:
			if top {
				return nil
			}
			return fmt.Errorf("vdf: unexpected end of input")
		case '}':
			if top {
				return fmt.Errorf("vdf: unexpected '}'")
			}
			return nil
		case '{':
			return fmt.Errorf("vdf: unexpected '{'")
		}
		val, vkind, err := p.token()
		if err != nil {
			return err
		}
		child := &Node{Key: key}
		switch vkind {
		case '"':
			child.Value = val
		case '{':
			child.Children = []*Node{}
			if err := p.object(child, false); err != nil {
				return err
			}
		default:
			return fmt.Errorf("vdf: missing value for %q", key)
		}
		n.Children = append(n.Children, child)
	}
}
