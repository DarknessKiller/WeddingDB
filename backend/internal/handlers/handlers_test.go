package handlers

import (
	"net/mail"
	"testing"
)

// --- sanitizeURL tests ---

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"https valid", "https://example.com/img.png", "https://example.com/img.png"},
		{"http valid", "http://example.com/img.png", "http://example.com/img.png"},
		{"relative path", "/images/logo.png", "/images/logo.png"},
		{"dot-slash", "./assets/bg.jpg", "./assets/bg.jpg"},
		{"protocol-relative blocked", "//evil.com/steal.js", ""},
		{"protocol-relative with path blocked", "//evil.com/path?x=1", ""},
		{"javascript blocked", "javascript:alert(1)", ""},
		{"data URI blocked", "data:text/html,<script>", ""},
		{"css expression blocked", "expression(alert(1))", ""},
		{"parens blocked", "https://example.com/a)b.png", ""},
		{"double quote blocked", "https://example.com/a\"b.png", ""},
		{"single quote blocked", "https://example.com/a'b.png", ""},
		{"semicolon blocked", "https://example.com/a;b.png", ""},
		{"braces blocked", "https://example.com/a{b}.png", ""},
		{"whitespace trimmed", "  https://example.com/img.png  ", "https://example.com/img.png"},
		{"whitespace-only becomes empty", "   ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeURL(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- validateCSSValue tests ---

func TestValidateCSSValue(t *testing.T) {
	// BackgroundSize
	if got := validateCSSValue("cover", validBackgroundSizes, "cover"); got != "cover" {
		t.Errorf("cover: got %q", got)
	}
	if got := validateCSSValue("contain", validBackgroundSizes, "cover"); got != "contain" {
		t.Errorf("contain: got %q", got)
	}
	if got := validateCSSValue("auto", validBackgroundSizes, "cover"); got != "auto" {
		t.Errorf("auto: got %q", got)
	}
	// Invalid background size falls back
	if got := validateCSSValue("100%", validBackgroundSizes, "cover"); got != "cover" {
		t.Errorf("100%%: got %q, want fallback", got)
	}
	if got := validateCSSValue("javascript:evil", validBackgroundSizes, "cover"); got != "cover" {
		t.Errorf("injection: got %q, want fallback", got)
	}

	// PositionsX
	for _, v := range []string{"left", "center", "right"} {
		if got := validateCSSValue(v, validPositionsX, "center"); got != v {
			t.Errorf("PosX %q: got %q", v, got)
		}
	}
	if got := validateCSSValue("50%", validPositionsX, "center"); got != "center" {
		t.Errorf("PosX 50%%: got %q, want fallback", got)
	}
	if got := validateCSSValue("expression(evil)", validPositionsX, "center"); got != "center" {
		t.Errorf("PosX injection: got %q, want fallback", got)
	}

	// PositionsY
	for _, v := range []string{"top", "center", "bottom"} {
		if got := validateCSSValue(v, validPositionsY, "center"); got != v {
			t.Errorf("PosY %q: got %q", v, got)
		}
	}
	if got := validateCSSValue("calc(100% - 20px)", validPositionsY, "center"); got != "center" {
		t.Errorf("PosY calc: got %q, want fallback", got)
	}
}

// --- Email validation (via net/mail) ---

func TestEmailValidation(t *testing.T) {
	valid := []string{
		"user@example.com",
		"admin@wedding.io",
		"test.user+tag@domain.co",
		"a@b.cc",
	}
	invalid := []string{
		"",
		"not-an-email",
		"@domain.com",
		"user@",
		"user@.com",
		"<>",
		"user @example.com",
		"javascript:alert(1)",
		"../../etc/passwd",
	}
	for _, e := range valid {
		if _, err := mail.ParseAddress(e); err != nil {
			t.Errorf("valid email %q rejected: %v", e, err)
		}
	}
	for _, e := range invalid {
		if _, err := mail.ParseAddress(e); err == nil {
			t.Errorf("invalid email %q accepted", e)
		}
	}
}

// --- validatePassword tests ---

func TestValidatePassword(t *testing.T) {
	// Password validation requires: length >= 8, at least one letter, one digit, one symbol
	if err := validatePassword("short1!"); err == nil {
		t.Error("too short should fail")
	}
	if err := validatePassword("NoDigits!!"); err == nil {
		t.Error("no digit should fail")
	}
	if err := validatePassword("12345678!"); err == nil {
		t.Error("no letter should fail")
	}
	if err := validatePassword("noSymbol1234"); err == nil {
		t.Error("missing symbol should fail")
	}
	if err := validatePassword("GoodPass1!"); err != nil {
		t.Errorf("valid password rejected: %v", err)
	}
	if err := validatePassword("alllower1!"); err != nil {
		t.Errorf("lowercase-only valid password rejected: %v", err)
	}
}
