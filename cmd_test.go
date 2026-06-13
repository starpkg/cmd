package cmd

// Unit tests for cmd.go.
//
// Sections:
//   - allowlist prefix matching (PKG-09)
//   - command sanitization / Unicode hardening (PKG-09)

import (
	"strings"
	"testing"
)

// --- allowlist prefix matching (PKG-09) --------------------------------------

func TestCommandAllowed(t *testing.T) {
	allow := []string{"git", "go test", " ls "}
	cases := []struct {
		canonical string
		want      bool
	}{
		{"git", true},              // exact
		{"git status", true},       // prefix at word boundary
		{"git status -s", true},    // deeper
		{"gitleaks detect", false}, // no word boundary after "git"
		{"go test", true},          // exact multi-word
		{"go test ./...", true},    // multi-word prefix
		{"go build", false},        // "go" alone is not allowlisted
		{"ls", true},               // entry is trimmed (" ls ")
		{"ls -la", true},           //
		{"lsof", false},            // boundary
		{"rm -rf /", false},        // not listed
		{"", false},                // empty command
	}
	for _, c := range cases {
		if got := commandAllowed(c.canonical, allow); got != c.want {
			t.Errorf("commandAllowed(%q) = %v, want %v", c.canonical, got, c.want)
		}
	}
}

func TestCommandAllowedEmptyAllowlistDeniesAll(t *testing.T) {
	for _, c := range []string{"go version", "ls", "echo hi", ""} {
		if commandAllowed(c, nil) {
			t.Errorf("empty allowlist must deny %q", c)
		}
	}
}

// --- command sanitization / Unicode hardening (PKG-09) -----------------------

func TestSanitizeCommandAcceptsPlain(t *testing.T) {
	for _, s := range []string{"go version", "git status -s", "echo 'hello world'", "ls -la /tmp"} {
		if got, err := sanitizeCommand(s); err != nil || got != s {
			t.Errorf("sanitizeCommand(%q) = (%q, %v), want unchanged and no error", s, got, err)
		}
	}
}

func TestSanitizeCommandRejectsControlAndFormat(t *testing.T) {
	cases := map[string]string{
		"zero-width space": "go\u200bversion",
		"BOM":              "\ufeffgo version",
		"RTL override":     "go\u202eversion",
		"newline":          "go version\nrm -rf",
		"carriage return":  "go\rversion",
		"tab":              "go\tversion",
		"null byte":        "go\x00version",
	}
	for name, s := range cases {
		if _, err := sanitizeCommand(s); err == nil {
			t.Errorf("%s: sanitizeCommand(%q) should have errored", name, s)
		}
	}
}

func TestSanitizeCommandRejectsInvalidUTF8(t *testing.T) {
	if _, err := sanitizeCommand("go \xff version"); err == nil {
		t.Error("sanitizeCommand should reject invalid UTF-8")
	} else if !strings.Contains(err.Error(), "UTF-8") {
		t.Errorf("error %q should mention UTF-8", err)
	}
}
