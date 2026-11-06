package app

import (
	"bytes"
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func fullRGBA(src image.Image) *image.RGBA {
	out := image.NewRGBA(src.Bounds())
	draw.Draw(out, out.Bounds(), src, src.Bounds().Min, draw.Src)
	return out
}

// Converting a few rows at a time gives exactly what converting the whole picture first
// gave, for each kind of picture the decoders return (16 §4.4).
func TestScaleBoxRowsMatchFullCopy(t *testing.T) {
	t.Parallel()
	const w, h = 997, 613
	r := image.Rect(0, 0, w, h)
	px := func(x, y int) uint8 { return uint8((x*7 + y*13 + x*y) % 251) }
	nrgba := image.NewNRGBA(r)
	gray := image.NewGray(r)
	pal := image.NewPaletted(r, palette.WebSafe)
	ycc420 := image.NewYCbCr(r, image.YCbCrSubsampleRatio420)
	ycc444 := image.NewYCbCr(r, image.YCbCrSubsampleRatio444)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			nrgba.SetNRGBA(x, y, color.NRGBA{px(x, y), px(y, x), px(x+y, x), px(x*3, y)})
			gray.SetGray(x, y, color.Gray{px(x, y)})
			pal.SetColorIndex(x, y, px(x, y)%216)
		}
	}
	for _, ycc := range []*image.YCbCr{ycc420, ycc444} {
		for i := range ycc.Y {
			ycc.Y[i] = uint8(i * 31)
		}
		for i := range ycc.Cb {
			ycc.Cb[i], ycc.Cr[i] = uint8(i*17), uint8(i*5)
		}
	}
	for name, src := range map[string]image.Image{
		"nrgba": nrgba, "gray": gray, "paletted": pal, "ycbcr420": ycc420, "ycbcr444": ycc444,
		"sub-nrgba": nrgba.SubImage(image.Rect(31, 17, 900, 600)),
		"sub-ycbcr": ycc420.SubImage(image.Rect(33, 19, 801, 555)),
	} {
		b := src.Bounds()
		for _, tw := range []int{360, 720, b.Dx()} {
			th := max(b.Dy()*tw/b.Dx(), 1)
			got, want := ScaleBox(src, tw, th), ScaleBox(fullRGBA(src), tw, th)
			if !bytes.Equal(got.Pix, want.Pix) {
				t.Errorf("%s at %d px: differs from scaling a full copy", name, tw)
			}
		}
	}
}

// Scaling a decoded PNG needs no second picture of the same size beside it.
func TestScaleBoxNeedsNoFullCopy(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2000, 2000)) // 16 MB
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	ScaleBox(src, 360, 360)
	runtime.ReadMemStats(&after)
	if got := after.TotalAlloc - before.TotalAlloc; got > 2<<20 {
		t.Fatalf("scaling allocated %d bytes; a full copy is 16 MB", got)
	}
}

// At most cap(decodeSlots) pictures are decoded at once, and nothing half-written or
// temporary is left behind.
func TestDecodeSlots(t *testing.T) {
	dir := t.TempDir()
	pic := encodePNG(t, 800, 450)
	var active, most, opened atomic.Int32
	release := make(chan struct{})
	entered := make(chan struct{}, 16)
	open := func() (io.ReadSeeker, func(), error) {
		n := active.Add(1)
		for m := most.Load(); n > m && !most.CompareAndSwap(m, n); m = most.Load() {
		}
		opened.Add(1)
		entered <- struct{}{}
		<-release
		return bytes.NewReader(pic), func() { active.Add(-1) }, nil
	}
	var wg sync.WaitGroup
	for _, p := range []string{"a", "b", "c", "d"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w, h, err := makeCachedPNG(filepath.Join(dir, p+".png"), 360, open); err != nil || w != 360 || h != 202 {
				t.Errorf("%s: %dx%d %v", p, w, h, err)
			}
		}()
	}
	for range cap(decodeSlots) {
		<-entered
	}
	select {
	case <-entered:
		t.Fatal("more decodes than slots")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	wg.Wait()
	if most.Load() > int32(cap(decodeSlots)) {
		t.Fatalf("%d decodes at once", most.Load())
	}
	if opened.Load() != 4 {
		t.Fatalf("opened %d pictures for 4", opened.Load())
	}
	es, _ := os.ReadDir(dir)
	if len(es) != 4 {
		var names []string
		for _, e := range es {
			names = append(names, e.Name())
		}
		t.Fatalf("cache holds %v", names)
	}
}

// A request that waited for a slot uses the picture another request made meanwhile.
func TestDecodeSlotWaitersUseNewCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "e.png")
	for range cap(decodeSlots) {
		decodeSlots <- struct{}{}
	}
	var opened atomic.Int32
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w, h, err := makeCachedPNG(path, 360, func() (io.ReadSeeker, func(), error) {
				opened.Add(1)
				return nil, nil, os.ErrNotExist
			})
			if err != nil || w != 300 || h != 100 {
				t.Errorf("waiter: %dx%d %v", w, h, err)
			}
		}()
	}
	time.Sleep(50 * time.Millisecond) // let them queue
	if err := writeCacheFile(path, encodePNG(t, 300, 100)); err != nil {
		t.Fatal(err)
	}
	for range cap(decodeSlots) {
		<-decodeSlots
	}
	wg.Wait()
	if opened.Load() != 0 {
		t.Fatalf("%d waiters decoded the picture again", opened.Load())
	}
}

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = uint8(i)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
