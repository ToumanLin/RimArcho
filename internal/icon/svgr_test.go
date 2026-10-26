package icon

import (
	"image/color"
	"testing"
)

// renderSVG draws a 100×100 test document at 100 px.
func renderSVG(t *testing.T, body string) func(x, y int) color.NRGBA {
	t.Helper()
	a, err := parseArtwork([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">` + body + `</svg>`))
	if err != nil {
		t.Fatal(err)
	}
	img := a.render(100, 0, 0, 100, 100)
	return func(x, y int) color.NRGBA { return img.NRGBAAt(x, y) }
}

func TestSVGFeatures(t *testing.T) {
	red := color.NRGBA{255, 0, 0, 255}
	cases := []struct {
		name string
		body string
		x, y int
		want color.NRGBA
	}{
		{"default fill is black", `<rect width="100" height="100"/>`, 50, 50, color.NRGBA{0, 0, 0, 255}},
		{"defs are not drawn", `<defs><rect width="100" height="100" fill="#f00"/></defs>`, 50, 50, color.NRGBA{}},
		{"group fill is inherited", `<g fill="#f00"><rect width="100" height="100"/></g>`, 50, 50, red},
		{"translate", `<rect width="10" height="10" fill="#f00" transform="translate(40 40)"/>`, 45, 45, red},
		{"group scale", `<g transform="scale(2)"><rect x="20" y="20" width="10" height="10" fill="#f00"/></g>`, 55, 55, red},
		{"rotate about a point", `<rect x="10" y="45" width="80" height="10" fill="#f00" transform="rotate(90 50 50)"/>`, 50, 15, red},
		{"clip-path", `<clipPath id="c"><rect width="50" height="100"/></clipPath><rect width="100" height="100" fill="#f00" clip-path="url(#c)"/>`, 75, 50, color.NRGBA{}},
		{"inside the clip", `<clipPath id="c"><rect width="50" height="100"/></clipPath><g clip-path="url(#c)"><rect width="100" height="100" fill="#f00"/></g>`, 25, 50, red},
		{"evenodd hole", `<path fill="#f00" fill-rule="evenodd" d="M0 0H100V100H0Z M25 25H75V75H25Z"/>`, 50, 50, color.NRGBA{}},
		{"nonzero keeps the hole filled", `<path fill="#f00" d="M0 0H100V100H0Z M25 25H75V75H25Z"/>`, 50, 50, red},
		{"bounding-box linear gradient", `<linearGradient id="g"><stop stop-color="#000"/><stop offset="1" stop-color="#fff"/></linearGradient><rect x="50" width="50" height="100" fill="url(#g)"/>`, 52, 50, color.NRGBA{10, 10, 10, 255}},
		{"radial gradient centre", `<radialGradient id="g"><stop stop-color="#f00"/><stop offset="1" stop-color="#00f"/></radialGradient><rect width="100" height="100" fill="url(#g)"/>`, 50, 50, color.NRGBA{250, 0, 5, 255}},
		{"opacity", `<rect width="100" height="100" fill="#f00" opacity=".5"/>`, 50, 50, color.NRGBA{255, 0, 0, 128}},
	}
	for _, c := range cases {
		got := renderSVG(t, c.body)(c.x, c.y)
		d := func(a, b uint8) int { return max(int(a)-int(b), int(b)-int(a)) }
		if d(got.R, c.want.R) > 8 || d(got.G, c.want.G) > 8 || d(got.B, c.want.B) > 8 || d(got.A, c.want.A) > 8 {
			t.Errorf("%s: got %v at (%d,%d), want %v", c.name, got, c.x, c.y, c.want)
		}
	}
}

func TestSVGBlur(t *testing.T) {
	at := renderSVG(t, `<filter id="b"><feGaussianBlur stdDeviation="4"/></filter><rect x="50" width="50" height="100" fill="#f00" filter="url(#b)"/>`)
	if a := at(50, 50).A; a < 100 || a > 156 {
		t.Errorf("edge of a blurred shape: alpha %d, want about half", a)
	}
	if a := at(40, 50).A; a == 0 {
		t.Error("blur does not spread past the edge")
	}
	if a := at(80, 50).A; a != 255 {
		t.Errorf("inside a blurred shape: alpha %d", a)
	}
}
