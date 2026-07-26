package models

import (
	"strings"
	"unicode"

	"github.com/mozillazg/go-pinyin"
)

// GenerateNamePinyin converts a name to lowercase pinyin for search.
// Chinese characters become their pinyin romanization, non-Chinese passes through.
func GenerateNamePinyin(name string) string {
	var parts []string
	for _, r := range name {
		if unicode.Is(unicode.Han, r) {
			py := pinyin.LazyPinyin(string(r), pinyin.Args{Style: pinyin.NORMAL})
			parts = append(parts, py[0])
		} else {
			parts = append(parts, strings.ToLower(string(r)))
		}
	}
	return strings.Join(parts, "")
}
