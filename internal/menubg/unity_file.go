package menubg

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// SerializedFile versions 17–22 cover Unity 2017–2022 player resources. Type
// trees are optional in player builds; only their metadata framing is needed.
type unityFile struct {
	f         *os.File
	version   uint32
	engine    string
	order     binary.ByteOrder
	objects   map[int64]unityObject
	externals []string
}
type unityObject struct {
	id           int64
	offset, size int64
	class        int32
}

type unityCursor struct {
	b     []byte
	pos   int
	order binary.ByteOrder
	err   error
}

func (c *unityCursor) take(n int) []byte {
	if c.err != nil {
		return nil
	}
	if n < 0 || n > len(c.b)-c.pos {
		c.err = io.ErrUnexpectedEOF
		return nil
	}
	b := c.b[c.pos : c.pos+n]
	c.pos += n
	return b
}
func (c *unityCursor) u8() byte {
	b := c.take(1)
	if b == nil {
		return 0
	}
	return b[0]
}
func (c *unityCursor) u16() uint16 {
	b := c.take(2)
	if b == nil {
		return 0
	}
	return c.order.Uint16(b)
}
func (c *unityCursor) u32() uint32 {
	b := c.take(4)
	if b == nil {
		return 0
	}
	return c.order.Uint32(b)
}
func (c *unityCursor) u64() uint64 {
	b := c.take(8)
	if b == nil {
		return 0
	}
	return c.order.Uint64(b)
}
func (c *unityCursor) align() { c.take((-c.pos) & 3) }
func (c *unityCursor) count(max int) int {
	n := c.u32()
	if uint64(n) > uint64(max) {
		c.err = errors.New("Unity count exceeds limit")
		return 0
	}
	return int(n)
}
func (c *unityCursor) str() string { n := c.count(4096); s := string(c.take(n)); c.align(); return s }
func (c *unityCursor) zstr() string {
	if c.err != nil {
		return ""
	}
	n := bytes.IndexByte(c.b[c.pos:], 0)
	if n < 0 || n > 4096 {
		c.err = errors.New("invalid Unity string")
		return ""
	}
	return string(c.take(n + 1)[:n])
}

func openUnity(path string) (*unityFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	u, err := parseUnity(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return u, nil
}
func parseUnity(f *os.File) (*unityFile, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	var h [48]byte
	if _, err = f.ReadAt(h[:20], 0); err != nil {
		return nil, err
	}
	be := binary.BigEndian
	version := be.Uint32(h[8:])
	if version < 17 || version > 22 {
		return nil, fmt.Errorf("unsupported SerializedFile version %d", version)
	}
	meta, size, start := int64(be.Uint32(h[:])), int64(be.Uint32(h[4:])), int64(be.Uint32(h[12:]))
	header := int64(20)
	if version >= 22 {
		if _, err = f.ReadAt(h[20:], 20); err != nil {
			return nil, err
		}
		meta, size, start = int64(be.Uint32(h[20:])), int64(be.Uint64(h[24:])), int64(be.Uint64(h[32:]))
		header = 48
	}
	if meta <= 0 || meta > 16<<20 || size > st.Size() || start < header+meta || start > size {
		return nil, errors.New("invalid Unity file bounds")
	}
	var order binary.ByteOrder = binary.LittleEndian
	if h[16] == 1 {
		order = binary.BigEndian
	} else if h[16] != 0 {
		return nil, errors.New("invalid Unity endian flag")
	}
	b := make([]byte, meta)
	if _, err = f.ReadAt(b, header); err != nil {
		return nil, err
	}
	c := unityCursor{b: b, order: order}
	u := &unityFile{f: f, version: version, engine: c.zstr(), order: order, objects: map[int64]unityObject{}}
	c.u32() // target platform
	tree := c.u8() != 0
	classes := make([]int32, c.count(4096))
	for i := range classes {
		classes[i] = int32(c.u32())
		c.u8()
		c.u16() // stripped, script type index
		if classes[i] == 114 {
			c.take(16)
		} // MonoBehaviour script ID
		c.take(16) // type hash
		if tree {
			nodes, stringsSize := c.count(1<<20), c.count(16<<20)
			nodeSize := 24
			if version >= 19 {
				nodeSize = 32
			}
			c.take(nodes * nodeSize)
			c.take(stringsSize)
			if version >= 21 {
				c.take(c.count(1<<20) * 4)
			} // type dependencies
		}
	}
	n := c.count(1 << 20)
	for range n {
		c.align()
		o := unityObject{id: int64(c.u64())}
		var offset int64
		if version >= 22 {
			offset = int64(c.u64())
		} else {
			offset = int64(c.u32())
		}
		o.offset, o.size = start+offset, int64(c.u32())
		typ := c.u32()
		if int(typ) >= len(classes) || offset < 0 || offset > size-start || o.size > size-o.offset {
			return nil, errors.New("invalid Unity object bounds/type")
		}
		o.class = classes[typ]
		if _, ok := u.objects[o.id]; ok {
			return nil, errors.New("duplicate Unity object ID")
		}
		u.objects[o.id] = o
	}
	for range c.count(1 << 20) {
		c.u32()
		c.align()
		c.u64()
	} // local script identifiers
	for range c.count(4096) {
		c.zstr()
		c.take(16)
		c.u32() // empty string, GUID, type
		u.externals = append(u.externals, c.zstr())
	}
	if c.err != nil {
		return nil, c.err
	}
	return u, nil
}

func (u *unityFile) objectPrefix(o unityObject, limit int64) (*unityCursor, error) {
	b := make([]byte, min(o.size, limit))
	if _, err := u.f.ReadAt(b, o.offset); err != nil {
		return nil, err
	}
	return &unityCursor{b: b, order: u.order}, nil
}

type unityPtr struct {
	file int32
	id   int64
}

func (u *unityFile) resources() (map[string]unityPtr, error) {
	for _, o := range u.objects {
		if o.class != 147 {
			continue
		} // ResourceManager
		if o.size > 8<<20 {
			return nil, errors.New("oversized ResourceManager")
		}
		c, err := u.objectPrefix(o, o.size)
		if err != nil {
			return nil, err
		}
		out := map[string]unityPtr{}
		for range c.count(100000) {
			key := strings.ToLower(c.str())
			p := unityPtr{int32(c.u32()), int64(c.u64())}
			if _, exists := out[key]; exists {
				return nil, errors.New("duplicate resource path")
			}
			out[key] = p
		}
		if c.err != nil {
			return nil, c.err
		}
		return out, nil
	}
	return nil, errors.New("ResourceManager unavailable")
}

// Use names only when the ResourceManager cannot be read, and reject ambiguity.
func (u *unityFile) textureByName(name string) (unityObject, error) {
	var found unityObject
	for _, o := range u.objects {
		if o.class != 28 {
			continue
		}
		c, err := u.objectPrefix(o, 8192)
		if err != nil {
			return found, err
		}
		if strings.EqualFold(c.str(), name) && c.err == nil {
			if found.size != 0 {
				return unityObject{}, errors.New("ambiguous Unity texture name")
			}
			found = o
		}
	}
	if found.size == 0 {
		return found, fmt.Errorf("Unity texture %s not found", name)
	}
	return found, nil
}
