package selfupdate

import "testing"

func TestDescriptionStamp(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"[h3]RimArcho[/h3]\r\nText", "[h3]RimArcho[/h3]\r\nText\n\nRimArcho v1.2.3"},
		{"Text\n\nRimArcho v1.2.2\n", "Text\n\nRimArcho v1.2.3"},                // the file already carries an older stamp
		{"Text\nRimArcho v1.2.2\r\nRimArcho v1.2.1", "Text\n\nRimArcho v1.2.3"}, // repeated stamps collapse
		{"RimArcho v1.2.2", "RimArcho v1.2.3"},
		{"", "RimArcho v1.2.3"},
	} {
		got := Stamp(tc.in, "1.2.3")
		if got != tc.want {
			t.Errorf("Stamp(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if v := DescriptionVersion(got); v != "1.2.3" {
			t.Errorf("DescriptionVersion(%q) = %q", got, v)
		}
	}
	for _, tc := range []struct{ in, want string }{
		{"RimArcho v1.0.5 added X.\nRimArcho v1.0.6\nedited on the web", "1.0.6"}, // the last stamp line counts
		{"Text\r\n\r\n  RimArcho v1.2.10  \r\n", "1.2.10"},
		{"RimArcho v1.0.5 added the Workshop item", ""},
		{"RimArcho v1.2.3-rc1\nRimArcho v1.2\nRimArcho v01.2.3\nRimArcho v1.2.3+build\nRimArcho 1.2.3", ""},
		{"", ""},
	} {
		if got := DescriptionVersion(tc.in); got != tc.want {
			t.Errorf("DescriptionVersion(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
