package view

import (
	"sync"
	"testing"
)

// Page readers format results while the GUI can change its language on another page.
func TestLanguageConcurrentWithPageReads(t *testing.T) {
	t.Setenv("ARCHO_LANG", "")
	before := Lang()
	defer SetLang(before)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 1000 {
			SetLang("zh")
			SetLang("en")
		}
	}()
	for range 1000 {
		_ = T("loading…")
		_ = Lang()
		_ = Table()
	}
	wg.Wait()
}
