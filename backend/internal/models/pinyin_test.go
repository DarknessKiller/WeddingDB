package models

import "testing"

func TestGenerateNamePinyin(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"张三", "zhangsan"},
		{"李四", "lisi"},
		{"John Smith", "john smith"},
		{"张 John", "zhang john"},
		{"", ""},
		{"ABC", "abc"},
		// Rare Han char missing from go-pinyin's dict: must not panic, must
		// produce non-empty output (raw char passes through, lowercased).
		{"𠮷", "𠮷"},
		{"𠮷野家", "𠮷yejia"},
		{"a𠮷b", "a𠮷b"},
	}
	for _, tt := range tests {
		got := GenerateNamePinyin(tt.input)
		if got != tt.want {
			t.Errorf("GenerateNamePinyin(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
