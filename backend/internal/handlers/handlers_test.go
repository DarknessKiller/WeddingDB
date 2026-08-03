package handlers

import (
	"encoding/json"
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

// --- KioskSettingsRequest deserialization ---

func TestKioskSettingsRequestNameDirtyOnly(t *testing.T) {
	// When "name" is omitted from JSON, Name should be nil (unchanged)
	t.Run("omitted name is nil", func(t *testing.T) {
		var req KioskSettingsRequest
		if err := json.Unmarshal([]byte(`{"venueName":"Hall","kioskDescription":"desc"}`), &req); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if req.Name != nil {
			t.Errorf("expected nil Name when omitted, got %q", *req.Name)
		}
		if req.VenueName != "Hall" {
			t.Errorf("expected VenueName 'Hall', got %q", req.VenueName)
		}
	})

	// When "name" is explicitly set, Name should point to that value
	t.Run("explicit name is set", func(t *testing.T) {
		var req KioskSettingsRequest
		if err := json.Unmarshal([]byte(`{"name":"New Wedding","venueName":"Hall"}`), &req); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if req.Name == nil {
			t.Fatal("expected non-nil Name")
		}
		if *req.Name != "New Wedding" {
			t.Errorf("expected Name 'New Wedding', got %q", *req.Name)
		}
	})

	// When "name" is explicitly null, Name should be nil
	t.Run("explicit null name is nil", func(t *testing.T) {
		var req KioskSettingsRequest
		if err := json.Unmarshal([]byte(`{"name":null,"venueName":"Hall"}`), &req); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if req.Name != nil {
			t.Errorf("expected nil Name when null, got %q", *req.Name)
		}
	})

	// When "name" is empty string, Name should be non-nil empty
	t.Run("empty string name is non-nil empty", func(t *testing.T) {
		var req KioskSettingsRequest
		if err := json.Unmarshal([]byte(`{"name":"","venueName":"Hall"}`), &req); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if req.Name == nil {
			t.Fatal("expected non-nil Name for empty string")
		}
		if *req.Name != "" {
			t.Errorf("expected empty Name, got %q", *req.Name)
		}
	})
}

// --- Blur clamping logic ---

func TestBlurClamping(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{"zero stays zero", 0, 0},
		{"positive in range", 10, 10},
		{"max boundary", 20, 20},
		{"over max clamped", 25, 20},
		{"negative clamped to zero", -5, 0},
		{"large negative clamped to zero", -100, 0},
		{"large positive clamped to 20", 999, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := max(0, min(20, tt.input))
			if got != tt.want {
				t.Errorf("max(0, min(20, %d)) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// --- LogoSize validation with "contain" fallback ---

func TestLogoSizeValidation(t *testing.T) {
	// Valid logo sizes
	for _, size := range []string{"cover", "contain", "auto"} {
		if got := validateCSSValue(size, validBackgroundSizes, "contain"); got != size {
			t.Errorf("logoSize %q: got %q, want %q", size, got, size)
		}
	}
	// Invalid logo size falls back to "contain" (not "cover")
	if got := validateCSSValue("100%", validBackgroundSizes, "contain"); got != "contain" {
		t.Errorf("invalid logoSize: got %q, want 'contain'", got)
	}
	if got := validateCSSValue("", validBackgroundSizes, "contain"); got != "contain" {
		t.Errorf("empty logoSize: got %q, want 'contain'", got)
	}
}
