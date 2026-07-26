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
	}
	for _, tt := range tests {
		got := GenerateNamePinyin(tt.input)
		if got != tt.want {
			t.Errorf("GenerateNamePinyin(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
