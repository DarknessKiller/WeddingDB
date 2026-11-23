package models

import (
	"strings"
	"unicode"

	"github.com/mozillazg/go-pinyin"
)

// maxPinyinLen matches the name_pinyin column size.
const maxPinyinLen = 255

// GenerateNamePinyin converts a name to lowercase pinyin for search.
// Chinese characters become their pinyin romanization, non-Chinese passes through.
// The result is capped at the column size: romanizations are longer than the
// runes they replace, so a valid 255-rune name can otherwise overflow.
func GenerateNamePinyin(name string) string {
	var parts []string
	for _, r := range name {
		if unicode.Is(unicode.Han, r) {
			py := pinyin.LazyPinyin(string(r), pinyin.Args{Style: pinyin.NORMAL})
			if len(py) == 0 {
				// Rare Han chars missing from go-pinyin's dict (e.g. 𠮷) return an
				// empty slice — indexing py[0] would panic and crash the caller.
				// Fall back to the raw char, lowercased, so search still works.
				parts = append(parts, strings.ToLower(string(r)))
			} else {
				parts = append(parts, py[0])
			}
		} else {
			parts = append(parts, strings.ToLower(string(r)))
		}
	}
	joined := strings.Join(parts, "")
	if runes := []rune(joined); len(runes) > maxPinyinLen {
		joined = string(runes[:maxPinyinLen])
	}
	return joined
}
