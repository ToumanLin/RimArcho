package view

import "testing"

// The memory item on one line: no "; " after a line that ends a sentence (VM 2026-11-10).
func TestJoinSentences(t *testing.T) {
	SetLang("zh")
	defer SetLang("en")
	if got, want := joinSentences([]string{"\u5185\u5b58\uff1a\u8f7d\u5165\u65f6\u9700\u8981 3 GB", "\u663e\u5b58\uff1a\u9884\u8ba1 1 GB", "\u653e\u5728\u7cfb\u7edf\u5185\u5b58\u91cc\u3002", "\u4e0d\u8db3\u662f\u98ce\u9669\u3002"}), "\u5185\u5b58\uff1a\u8f7d\u5165\u65f6\u9700\u8981 3 GB\uff0c\u663e\u5b58\uff1a\u9884\u8ba1 1 GB\uff0c\u653e\u5728\u7cfb\u7edf\u5185\u5b58\u91cc\u3002\u4e0d\u8db3\u662f\u98ce\u9669\u3002"; got != want {
		t.Fatalf("zh: %q, want %q", got, want)
	}
	SetLang("en")
	if got, want := joinSentences([]string{"RAM: 3 GB", "VRAM: 1 GB", "It is in system memory.", "A risk."}), "RAM: 3 GB; VRAM: 1 GB; It is in system memory. A risk."; got != want {
		t.Fatalf("en: %q, want %q", got, want)
	}
}
