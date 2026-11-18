package l10ndata

import (
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
)

func TestChineseMemoryPunctuation(t *testing.T) {
	table := Locales["zh"]()
	for _, key := range []string{
		"RAM: loading needs %s; available now %s of %s",
		"RAM: loading needs %s; available memory and total capacity are unavailable",
		"RAM: loading needs %s; available memory is unavailable; total capacity %s",
		"VRAM: estimated %s; dedicated capacity %s",
		"VRAM: estimated %s; dedicated capacity is unknown",
	} {
		text := table.Text[key]
		if text == "" || strings.ContainsAny(text, ";:") {
			t.Errorf("memory translation must use localized punctuation: %q: %q", key, text)
		}
	}
}

func TestLocaleDuplicateKeys(t *testing.T) {
	tests := []struct {
		name, first, second string
		wantClash           bool
	}{
		{"within one object", `{"key":"first","key":"second"}`, `{}`, true},
		{"across feature files", `{"key":"first"}`, `{"key":"second"}`, true},
		{"same value is harmless", `{"key":"same","key":"same"}`, `{"key":"same"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table, err := readLocale(fstest.MapFS{
				"xx/home.json":    {Data: []byte(tt.first)},
				"xx/library.json": {Data: []byte(tt.second)},
			}, "xx")
			if err != nil {
				t.Fatal(err)
			}
			if got := len(table.Clashes) > 0; got != tt.wantClash {
				t.Fatalf("clashes = %v", table.Clashes)
			}
			if tt.wantClash && table.Clashes["key"] != [2]string{"first", "second"} {
				t.Fatal(table.Clashes)
			}
		})
	}
}

func TestLocaleSupplementalConflict(t *testing.T) {
	table, err := readLocale(fstest.MapFS{
		"xx/_extra.json": {Data: []byte(`{"key":"first"}`)},
		"xx/home.json":   {Data: []byte(`{"key":"second"}`)},
	}, "xx")
	if err != nil {
		t.Fatal(err)
	}
	if table.Clashes["key"] != [2]string{"first", "second"} {
		t.Fatal(table.Clashes)
	}
}

func TestLocaleMalformedJSON(t *testing.T) {
	for _, text := range []string{`[]`, `{"key":1}`, `{"key":"text"} {}`, `{"key":null}`, `{"key":"text"`} {
		t.Run(text, func(t *testing.T) {
			_, err := readLocale(fstest.MapFS{"xx/ui.json": {Data: []byte(text)}}, "xx")
			if err == nil {
				t.Fatalf("accepted malformed translation object %s", text)
			}
		})
	}
}

func TestBundledLocales(t *testing.T) {
	for name, load := range Locales {
		t.Run(name, func(t *testing.T) {
			table := load()
			if table.Name != name || len(table.Text) == 0 {
				t.Fatalf("empty or unnamed locale: %q", name)
			}
			for key, values := range table.Clashes {
				t.Errorf("%s: conflicting translations %q and %q", key, values[0], values[1])
			}
		})
	}
}

func TestLocaleConcurrentFirstLoad(t *testing.T) {
	// A fresh OnceValue exercises the first parse even if another test loaded Chinese.
	load := sync.OnceValue(func() *LocaleTable {
		table, err := readLocale(fstest.MapFS{"xx/home.json": {Data: []byte(`{"home":"localized home"}`)}}, "xx")
		if err != nil {
			panic(err)
		}
		return table
	})
	results := make(chan *LocaleTable, 32)
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() { results <- load() })
	}
	wg.Wait()
	close(results)
	for result := range results {
		if result != load() || result.Text["home"] != "localized home" {
			t.Fatal("inconsistent locale publication")
		}
	}
}

func TestLanguageChoicesIncludeBundledDirectories(t *testing.T) {
	codes := LanguageCodes()
	if len(codes) < 3 || !reflect.DeepEqual(codes[:2], []string{"auto", "en"}) {
		t.Fatal(codes)
	}
	for code := range Locales {
		found := false
		for _, choice := range codes {
			found = found || choice == code
		}
		if !found {
			t.Fatalf("locale directory %s is not selectable", code)
		}
	}
}

// Chinese is the reference locale: every other bundled locale carries the same keys, with
// the same placeholders, so a string added to Chinese and not to the others fails here.
func TestLocalesMatchChinese(t *testing.T) {
	zh := Locales["zh"]()
	verbs := regexp.MustCompile(`%(\[\d+\])?[-+# 0]*\d*(\.\d+)?[a-zA-Z%]`)
	placeholders := func(s string) string {
		var out []string
		for _, v := range verbs.FindAllString(s, -1) {
			if v != "%%" {
				out = append(out, regexp.MustCompile(`\[\d+\]`).ReplaceAllString(v, ""))
			}
		}
		sort.Strings(out)
		return strings.Join(out, ",")
	}
	for code, load := range Locales {
		if code == "zh" {
			continue
		}
		table := load()
		for _, pair := range []struct {
			name      string
			want, got map[string]string
		}{{"text", zh.Text, table.Text}, {"extra", zh.ExtraText, table.ExtraText}} {
			for key, reference := range pair.want {
				value, ok := pair.got[key]
				switch {
				case !ok || strings.TrimSpace(value) == "":
					t.Errorf("%s %s lacks %q", code, pair.name, key)
				case pair.name == "text" && placeholders(value) != placeholders(reference):
					t.Errorf("%s %q: placeholders %q, Chinese has %q", code, key, placeholders(value), placeholders(reference))
				}
			}
			for key := range pair.got {
				if _, ok := pair.want[key]; !ok {
					t.Errorf("%s %s has %q, which Chinese no longer has", code, pair.name, key)
				}
			}
		}
	}
}

func TestResolveLangKeepsTheLanguage(t *testing.T) {
	t.Setenv("ARCHO_LANG", "")
	for input, want := range map[string]string{"ja": "ja", "ja-JP": "ja", "ja_JP.UTF-8": "ja", "zh-Hans-CN": "zh", "zh_CN": "zh", "en-US": "en", "fr-FR": "en"} {
		if got := ResolveLang(input); got != want {
			t.Errorf("ResolveLang(%q) = %q, want %q", input, got, want)
		}
	}
	if LanguageName("ja") == "ja" || LanguageName("zh") == "zh" || LanguageName("xx") != "xx" {
		t.Error("language names come from name.txt, falling back to the code")
	}
}
