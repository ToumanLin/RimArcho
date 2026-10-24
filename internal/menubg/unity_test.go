package menubg

import (
	"bytes"
	"context"
	"encoding/binary"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Fixtures are generated from invented pixels/IDs, never copied from Unity games.
type unityFixtureObject struct {
	id    int64
	class int32
	data  []byte
}
type unityWriter struct {
	bytes.Buffer
	order binary.ByteOrder
}

func (w *unityWriter) u(v any) {
	if err := binary.Write(&w.Buffer, w.order, v); err != nil {
		panic(err)
	}
}
func (w *unityWriter) pad() {
	for w.Len()%4 != 0 {
		w.WriteByte(0)
	}
}
func (w *unityWriter) str(s string)  { w.u(uint32(len(s))); w.WriteString(s); w.pad() }
func (w *unityWriter) zstr(s string) { w.WriteString(s); w.WriteByte(0) }

func serializedFixture(version uint32, engine string, order binary.ByteOrder, tree bool, objects []unityFixtureObject, externals []string) []byte {
	m := unityWriter{order: order}
	m.zstr(engine)
	m.u(int32(19))
	m.u(tree)
	classes := []int32{28, 147, 114}
	m.u(uint32(len(classes)))
	for _, class := range classes {
		m.u(class)
		m.u(false)
		m.u(int16(-1))
		if class == 114 {
			m.Write(make([]byte, 16))
		}
		m.Write(make([]byte, 16))
		if tree {
			m.u(uint32(1))
			m.u(uint32(10))
			nodeSize := 24
			if version >= 19 {
				nodeSize = 32
			}
			m.Write(make([]byte, nodeSize))
			m.zstr("Texture2D")
			if version >= 21 {
				m.u(uint32(0))
			}
		}
	}
	m.u(uint32(len(objects)))
	var payload bytes.Buffer
	for _, o := range objects {
		m.pad()
		m.u(o.id)
		if version >= 22 {
			m.u(uint64(payload.Len()))
		} else {
			m.u(uint32(payload.Len()))
		}
		m.u(uint32(len(o.data)))
		for i, class := range classes {
			if class == o.class {
				m.u(int32(i))
				break
			}
		}
		payload.Write(o.data)
	}
	m.u(uint32(1))
	m.u(int32(0))
	m.pad()
	m.u(int64(123)) // local script identifier
	m.u(uint32(len(externals)))
	for _, external := range externals {
		m.zstr("")
		m.Write(make([]byte, 16))
		m.u(int32(0))
		m.zstr(external)
	}
	if version >= 20 {
		m.u(uint32(0))
	} // reference types
	m.zstr("")
	header := 20
	if version >= 22 {
		header = 48
	}
	start := (header + m.Len() + 15) &^ 15
	h := make([]byte, start)
	be := binary.BigEndian
	be.PutUint32(h[8:], version)
	if order == binary.BigEndian {
		h[16] = 1
	}
	size := start + payload.Len()
	if version >= 22 {
		be.PutUint32(h[20:], uint32(m.Len()))
		be.PutUint64(h[24:], uint64(size))
		be.PutUint64(h[32:], uint64(start))
	} else {
		be.PutUint32(h, uint32(m.Len()))
		be.PutUint32(h[4:], uint32(size))
		be.PutUint32(h[12:], uint32(start))
	}
	copy(h[header:], m.Bytes())
	return append(h, payload.Bytes()...)
}

func textureFixture(engine, name string, width, height, format int, pixels []byte, offset uint64, stream string) []byte {
	w := unityWriter{order: binary.LittleEndian}
	w.str(name)
	w.u(int32(4))
	w.u(false)
	w.pad()
	w.u(int32(width))
	w.u(int32(height))
	w.u(uint32(len(pixels)))
	var major, minor int
	// Only the two fixture layouts needed below; metadata engine parsing is tested separately.
	if engine == "2022.3.35f1" {
		major, minor = 2022, 3
	} else if engine == "2019.4.30f1" {
		major, minor = 2019, 4
	} else {
		major, minor = 2018, 4
	}
	if major >= 2020 {
		w.u(int32(0))
	}
	w.u(int32(format))
	w.u(int32(2)) // has an unused lower mip
	w.Write(make([]byte, 4))
	if major == 2022 && minor >= 2 {
		w.str("")
		w.u(false)
		w.pad()
	}
	w.u(int32(0))
	w.u(int32(1))
	w.u(int32(2))
	w.Write(make([]byte, 32))
	if major >= 2020 {
		w.u(uint32(3))
		w.Write([]byte{1, 2, 3})
		w.pad()
	}
	if stream == "" {
		w.u(uint32(len(pixels)))
		w.Write(pixels)
		w.pad()
	} else {
		w.u(uint32(0))
	}
	if major >= 2020 {
		w.u(offset)
	} else {
		w.u(uint32(offset))
	}
	w.u(uint32(len(pixels)))
	w.str(stream)
	return w.Bytes()
}

func rgbaRows(width, height int) []byte {
	var b []byte
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			b = append(b, byte(y*20), byte(x*20), 80, 255)
		}
	}
	return b
}

func TestSerializedFileMetadata(t *testing.T) {
	for _, version := range []uint32{17, 19, 21, 22} {
		for _, tree := range []bool{false, true} {
			for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
				engine := "2022.3.35f1"
				raw := serializedFixture(version, engine, order, tree, []unityFixtureObject{{7, 28, []byte{1, 2, 3}}, {99, 114, []byte{4}}}, []string{"resources.assets"})
				path := filepath.Join(t.TempDir(), "fixture.assets")
				put(t, path, raw)
				u, err := openUnity(path)
				if err != nil {
					t.Fatalf("version %d tree %v endian %s: %v", version, tree, order, err)
				}
				if u.engine != engine || u.order != order || len(u.objects) != 2 || u.objects[7].class != 28 || u.externals[0] != "resources.assets" {
					t.Fatalf("bad metadata: %+v", u)
				}
				c, err := u.objectPrefix(u.objects[7], 3)
				u.f.Close()
				if err != nil || !bytes.Equal(c.b, []byte{1, 2, 3}) {
					t.Fatal("object range", err)
				}
			}
		}
	}
}

func TestSerializedFileRejectsCorruption(t *testing.T) {
	raw := serializedFixture(22, "2022.3.35f1", binary.LittleEndian, false, []unityFixtureObject{{1, 28, []byte{1}}}, nil)
	cases := [][]byte{raw[:10], raw[:40], raw[:len(raw)-1]}
	for _, at := range []int{8, 20, 24, 32} {
		b := bytes.Clone(raw)
		for i := 0; i < 4; i++ {
			b[at+i] = 255
		}
		cases = append(cases, b)
	}
	// A signed-negative type count and a duplicate path ID must fail without panic.
	b := bytes.Clone(raw)
	copy(b[48+len("2022.3.35f1")+1+5:], []byte{255, 255, 255, 255})
	cases = append(cases, b)
	cases = append(cases, serializedFixture(22, "2022.3.35f1", binary.LittleEndian, false, []unityFixtureObject{{1, 28, []byte{1}}, {1, 28, []byte{2}}}, nil))
	for i, b := range cases {
		path := filepath.Join(t.TempDir(), "bad.assets")
		put(t, path, b)
		u, err := openUnity(path)
		if err == nil {
			u.f.Close()
			t.Fatalf("corruption %d accepted", i)
		}
	}
}

func TestUnityRGBAInlineStreamFlipAndScale(t *testing.T) {
	for _, engine := range []string{"2018.4.30f1", "2019.4.30f1", "2022.3.35f1"} {
		for _, stream := range []string{"", "resources.assets.resS"} {
			pixels := rgbaRows(8, 11)
			data := textureFixture(engine, "Synthetic", 8, 11, 4, pixels, 19, stream)
			path := filepath.Join(t.TempDir(), "resources.assets")
			put(t, path, serializedFixture(22, engine, binary.LittleEndian, false, []unityFixtureObject{{7, 28, data}}, nil))
			if stream != "" {
				put(t, path+".resS", append(make([]byte, 19), pixels...))
			}
			u, err := openUnity(path)
			if err != nil {
				t.Fatal(err)
			}
			tex, err := u.texture(u.objects[7])
			if err != nil {
				t.Fatal(engine, err)
			}
			d, closer, err := u.textureReader(tex)
			if err != nil {
				t.Fatal(err)
			}
			imgs, err := d.Scaled(context.Background(), 4, 8)
			closer.Close()
			u.f.Close()
			if err != nil {
				t.Fatal(err)
			}
			for i, w := range []int{4, 8} {
				h := 11 * w / 8
				for y := 0; y < h; y++ {
					for x := 0; x < w; x++ {
						y0, y1 := y*11/h, (y+1)*11/h
						x0, x1 := x*8/w, (x+1)*8/w
						want := color.RGBA{byte((20*(10-y0) + 20*(11-y1)) / 2), byte(20 * (x0 + x1 - 1) / 2), 80, 255}
						if got := imgs[i].RGBAAt(x, y); got != want {
							t.Fatalf("%s stream=%q (%d,%d) got %v want %v", engine, stream, x, y, got, want)
						}
					}
				}
			}
		}
	}
}

func TestUnityCompressedFormats(t *testing.T) {
	for _, format := range []int{10, 12, 25} {
		var pixels []byte
		width, height := 4, 4
		switch format {
		case 10:
			pixels = []byte{0, 248, 0, 248, 0, 0, 0, 0} // opaque red BC1
		case 12:
			pixels = []byte{255, 255, 0, 0, 0, 0, 0, 0, 0, 248, 0, 248, 0, 0, 0, 0} // opaque red BC3
		case 25:
			raw, err := os.ReadFile("testdata/bc7.dds")
			if err != nil {
				t.Fatal(err)
			}
			width, height = 32, 4
			pixels = raw[148:]
		}
		path := filepath.Join(t.TempDir(), "resources.assets")
		data := textureFixture("2022.3.35f1", "Test", width, height, format, append(pixels, 1, 2, 3, 4), 7, "resources.assets.resS")
		put(t, path, serializedFixture(22, "2022.3.35f1", binary.LittleEndian, false, []unityFixtureObject{{7, 28, data}}, nil))
		put(t, path+".resS", append(make([]byte, 7), append(pixels, 1, 2, 3, 4)...))
		u, err := openUnity(path)
		if err != nil {
			t.Fatal(err)
		}
		tex, err := u.texture(u.objects[7])
		if err != nil {
			t.Fatal(err)
		}
		d, closer, err := u.textureReader(tex)
		if err != nil {
			t.Fatal(err)
		}
		imgs, err := d.Scaled(context.Background(), width)
		closer.Close()
		if err != nil {
			t.Fatal(err)
		}
		if format != 25 {
			if got := imgs[0].RGBAAt(0, 0); got != (color.RGBA{255, 0, 0, 255}) {
				t.Fatalf("format %d: %v", format, got)
			}
		} else {
			raw, _ := os.ReadFile("testdata/bc7.dds")
			reference, _ := ReadDDS(bytes.NewReader(raw))
			want, err := reference.Scaled(context.Background(), width)
			if err != nil {
				t.Fatal(err)
			}
			for y := 0; y < height; y++ {
				for x := 0; x < width; x++ {
					if imgs[0].RGBAAt(x, y) != want[0].RGBAAt(x, height-1-y) {
						t.Fatal("BC7 flip")
					}
				}
			}
		}
		for _, bad := range []unityTexture{
			{width: 4, height: 4, format: 99},
			{width: 4, height: 4, format: 4, size: 1},
			{width: 4, height: 4, format: 4, offset: 1 << 40, size: 64},
			{width: 4, height: 4, format: 4, size: 64, path: "../resources.assets.resS"},
		} {
			if _, c, err := u.textureReader(bad); err == nil {
				c.Close()
				t.Fatalf("bad stream accepted: %+v", bad)
			}
		}
		u.f.Close()
	}
}

func managerFixture(paths map[string]int64) []byte {
	w := unityWriter{order: binary.LittleEndian}
	w.u(uint32(len(paths)))
	for path, id := range paths {
		w.str(path)
		w.u(int32(1))
		w.u(id)
	}
	w.u(uint32(0)) // dependent assets
	return w.Bytes()
}

func TestPrepareUnityExactResourcesAndCache(t *testing.T) {
	game, cache := gameFixture(t), t.TempDir()
	root := filepath.Join(game, "RimWorldWin64_Data")
	put(t, filepath.Join(game, "Data/Core/Defs/Misc/ExpansionDefs/ExpansionDefs.xml"), []byte(strings.ReplaceAll(testDefs, "<defName>Ambiguous</defName>", "<defName>Uninstalled</defName>")))
	put(t, filepath.Join(game, "Data/Royalty/About/About.xml"), []byte("<ModMetaData/>"))
	engine := "2022.3.35f1"
	// The path points to the object whose name does not match the backgroundPath.
	objects := []unityFixtureObject{{7, 28, textureFixture(engine, "NotThePath", 8, 8, 4, rgbaRows(8, 8), 0, "")}, {8, 28, textureFixture(engine, "MenuCore", 8, 8, 4, rgbaRows(8, 8), 0, "")}}
	put(t, filepath.Join(root, "resources.assets"), serializedFixture(22, engine, binary.LittleEndian, false, objects, nil))
	put(t, filepath.Join(root, "globalgamemanagers"), serializedFixture(22, engine, binary.LittleEndian, true, []unityFixtureObject{{13, 147, managerFixture(map[string]int64{"textures/ui/menucore": 7, "textures/ui/menuroyalty": 8})}}, []string{"resources.assets"}))
	items, err := Prepare(context.Background(), game, cache, "en")
	if err != nil || len(items) != 2 || items[0].Key != "game:ludeon.rimworld" {
		t.Fatalf("exact resources: %+v %v", items, err)
	}
	old := time.Unix(100, 0)
	os.Chtimes(items[0].Full, old, old)
	again, err := Prepare(context.Background(), game, cache, "zh")
	if err != nil || len(again) != 2 || again[0].Full != items[0].Full {
		t.Fatal("cache hit", err)
	}
	st, _ := os.Stat(items[0].Full)
	if !st.ModTime().Equal(old) {
		t.Fatal("cache hit rewrote image")
	}
	os.Remove(items[0].Thumb)
	if _, err := Prepare(context.Background(), game, cache, "en"); err != nil || !regular(items[0].Thumb) {
		t.Fatal("pair repair", err)
	}
	for _, name := range []string{"resources.assets", "globalgamemanagers", "resources.assets.resS"} {
		path := filepath.Join(root, name)
		if name == "resources.assets.resS" {
			put(t, path, []byte{1})
		} else {
			if err := os.Chtimes(path, old, old); err != nil {
				t.Fatal(err)
			}
		}
		changed, err := Prepare(context.Background(), game, cache, "en")
		if err != nil || len(changed) != 2 || changed[0].Full == items[0].Full {
			t.Fatalf("%s did not invalidate: %v", name, err)
		}
		items = changed
	}
	u, err := openUnity(filepath.Join(root, "resources.assets"))
	if err != nil {
		t.Fatal(err)
	}
	defer u.f.Close()
	manager, err := openUnity(filepath.Join(root, "globalgamemanagers"))
	if err != nil {
		t.Fatal(err)
	}
	defer manager.f.Close()
	resources, err := manager.resources()
	if err != nil {
		t.Fatal(err)
	}
	if o, err := resolveUnityTexture(u, manager, resources, "UI/MenuCore"); err != nil || o.id != 7 {
		t.Fatal("PPtr resolution", err)
	}
	if _, err := resolveUnityTexture(u, manager, resources, "Other/MenuCore"); err == nil {
		t.Fatal("guessed by name despite available mapping")
	}
	if o, err := resolveUnityTexture(u, nil, nil, "UI/MenuCore"); err != nil || o.id != 8 {
		t.Fatal("name fallback", err)
	}
	u.objects[9] = u.objects[8]
	if _, err := u.textureByName("MenuCore"); err == nil {
		t.Fatal("ambiguous name accepted")
	}
}

func TestUnityTruncatedTexture(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resources.assets")
	data := textureFixture("2022.3.35f1", "X", 8, 8, 4, rgbaRows(8, 8), 0, "")
	put(t, path, serializedFixture(22, "2022.3.35f1", binary.LittleEndian, false, []unityFixtureObject{{7, 28, data[:80]}}, nil))
	u, err := openUnity(path)
	if err != nil {
		t.Fatal(err)
	}
	defer u.f.Close()
	if _, err := u.texture(u.objects[7]); err == nil {
		t.Fatal("truncated texture accepted")
	}
}
