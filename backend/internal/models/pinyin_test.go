package models

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestGenerateNamePinyin_CapsAtColumnSize(t *testing.T) {
	got := GenerateNamePinyin(strings.Repeat("张", 255))
	if n := utf8.RuneCountInString(got); n > maxPinyinLen {
		t.Fatalf("pinyin length = %d runes, want at most %d", n, maxPinyinLen)
	}
}
