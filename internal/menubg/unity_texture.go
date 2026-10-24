package menubg

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/woozymasta/bcn"
)

type unityTexture struct {
	name                  string
	width, height, format int
	offset, size          int64
	path                  string // empty for inline pixels
}

func (u *unityFile) texture(o unityObject) (unityTexture, error) {
	var t unityTexture
	if o.class != 28 {
		return t, errors.New("resource is not Texture2D")
	}
	var major, minor int
	if _, err := fmt.Sscanf(u.engine, "%d.%d.", &major, &minor); err != nil || major < 2018 || major > 2022 {
		return t, fmt.Errorf("unsupported Unity texture layout %s", u.engine)
	}
	c, err := u.objectPrefix(o, 16384)
	if err != nil {
		return t, err
	}
	t.name = c.str()
	c.take(8) // forced fallback format, downscale/alpha flags, alignment
	t.width, t.height = int(c.u32()), int(c.u32())
	c.u32() // complete image size (all mips)
	if major >= 2020 && c.u32() != 0 {
		return t, errors.New("Unity top mip was stripped")
	}
	t.format = int(c.u32())
	mips := c.u32()
	c.take(4) // readable/preprocessed/ignore-limit/streaming flags and alignment
	if major == 2022 && minor >= 2 {
		c.str() // mipmap limit group (aligned)
		c.u8()
		c.align() // streaming mipmaps
	}
	c.u32() // streaming mipmap priority
	count, dimension := c.u32(), c.u32()
	c.take(32) // GLTextureSettings, lightmap format, color space
	if major > 2020 || major == 2020 && minor >= 2 {
		c.take(c.count(1 << 20))
		c.align()
	} // platform blob
	inline := int64(c.u32())
	if inline != 0 {
		t.offset, t.size = o.offset+int64(c.pos), inline
		if inline > o.size-int64(c.pos) {
			return t, errors.New("truncated Unity inline texture")
		}
	} else {
		if major >= 2020 {
			t.offset = int64(c.u64())
		} else {
			t.offset = int64(c.u32())
		}
		t.size = int64(c.u32())
		t.path = c.str()
		if t.path == "" {
			return t, errors.New("Unity texture has no pixel data")
		}
	}
	if c.err != nil {
		return t, c.err
	}
	if t.width < 1 || t.height < 1 || t.width > 16384 || t.height > 16384 || int64(t.width)*int64(t.height) > 40e6 || count != 1 || dimension != 2 || mips == 0 {
		return t, errors.New("unsupported Unity 2D texture size/layout")
	}
	return t, nil
}

// The compressed pixel range is streamed from disk; neither the resS nor even a
// full compressed texture is retained. Unity image rows are stored bottom first.
func (u *unityFile) textureReader(t unityTexture) (*DDS, io.Closer, error) {
	d := &DDS{Width: t.width, Height: t.height, flip: true, blockSize: 16}
	switch t.format {
	case 4:
		d.blockSize = 0 // RGBA32
	case 10:
		d.format, d.blockSize = bcn.FormatBC1, 8 // DXT1
	case 12:
		d.format = bcn.FormatBC3 // DXT5
	case 25:
		d.format = bcn.FormatBC7
	default:
		return nil, nil, fmt.Errorf("unsupported Unity texture format %d", t.format)
	}
	need := int64((t.width+3)/4) * int64((t.height+3)/4) * int64(d.blockSize)
	if d.blockSize == 0 {
		need = int64(t.width) * int64(t.height) * 4
	}
	if t.offset < 0 || t.size < need {
		return nil, nil, errors.New("truncated Unity level 0")
	}
	f := u.f
	var closer io.Closer = noClose{}
	if t.path != "" {
		// AssetBundle archive: URIs and traversal are deliberately not resolved here.
		if strings.ContainsAny(t.path, `/\:`) || t.path != filepath.Base(t.path) || !strings.EqualFold(t.path, filepath.Base(u.f.Name())+".resS") {
			return nil, nil, fmt.Errorf("unsupported Unity stream path %q", t.path)
		}
		var err error
		f, err = os.Open(filepath.Join(filepath.Dir(u.f.Name()), t.path))
		if err != nil {
			return nil, nil, err
		}
		closer = f
	}
	st, err := f.Stat()
	if err != nil {
		closer.Close()
		return nil, nil, err
	}
	if t.offset > st.Size() || t.size > st.Size()-t.offset {
		closer.Close()
		return nil, nil, errors.New("Unity texture stream outside file")
	}
	d.r = io.NewSectionReader(f, t.offset, need)
	return d, closer, nil
}

type noClose struct{}

func (noClose) Close() error { return nil }
