package steamworks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadPublishRequestTranslations(t *testing.T) {
	dir := t.TempDir()
	base := PublishRequest{Content: dir, IDFile: filepath.Join(dir, "id.txt"), Version: "1.2.3", Tags: []string{"Mod", "1.6"}, Title: "RimArcho", Description: "en"}
	read := func(r PublishRequest) error {
		b, _ := json.Marshal(r)
		p := filepath.Join(dir, "request.json")
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := ReadPublishRequest(p)
		return err
	}
	ok := base
	ok.Translations = []PublishTranslation{{Language: "schinese", Description: strings.Repeat("\u4f53", DescriptionMax/3)}}
	if err := read(ok); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*PublishRequest){
		"unknown language": func(r *PublishRequest) {
			r.Translations = []PublishTranslation{{Language: "klingon", Description: "x"}}
		},
		"empty translation": func(r *PublishRequest) { r.Translations = []PublishTranslation{{Language: "schinese"}} },
		"long translation": func(r *PublishRequest) {
			r.Translations = []PublishTranslation{{Language: "schinese", Description: strings.Repeat("\u4f53", DescriptionMax/3+1)}}
		},
		"long description": func(r *PublishRequest) { r.Description = strings.Repeat("x", DescriptionMax+1) },
		"no tags":          func(r *PublishRequest) { r.Tags = nil },
		"empty tag":        func(r *PublishRequest) { r.Tags = []string{"Mod", ""} },
		"tag with comma":   func(r *PublishRequest) { r.Tags = []string{"Mod,1.6"} },
		"long tag":         func(r *PublishRequest) { r.Tags = []string{strings.Repeat("x", TagMax+1)} },
		"NUL in translation": func(r *PublishRequest) {
			r.Translations = []PublishTranslation{{Language: "schinese", Description: "a\x00b"}}
		},
	} {
		r := base
		mutate(&r)
		if read(r) == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
