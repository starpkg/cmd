package cmd

// Unit tests for cmd.go.
//
// Sections:
//   - allowlist prefix matching (PKG-09)
//   - command sanitization / Unicode hardening (PKG-09)
//   - constructor policy state (enabled/allow are Go-host only)
//   - argument defaulting helpers (cwd / timeout / bool)
//   - env map building (config default + per-call override)
//   - result struct shaping (capture/combine output matrix)

import (
	"strings"
	"testing"
	"time"

	"github.com/1set/starlet/dataconv/types"
	"go.starlark.net/starlark"
)

// mkNullableStr returns a NullableStringOrBytes populated from v (use
// starlark.None for null, starlark.String("") for empty, etc.), mirroring how
// run() unpacks the cwd/stdin kwargs.
func mkNullableStr(t *testing.T, v starlark.Value) *types.NullableStringOrBytes {
	t.Helper()
	n := types.NewNullableStringOrBytes("")
	if err := n.Unpack(v); err != nil {
		t.Fatalf("unpack nullable string from %v: %v", v, err)
	}
	return n
}

// mkNullableBool returns a NullableBool populated from v (starlark.None for
// null), mirroring how run() unpacks the bool kwargs.
func mkNullableBool(t *testing.T, v starlark.Value) *types.NullableBool {
	t.Helper()
	n := types.NewNullable[starlark.Bool](false)
	if err := n.Unpack(v); err != nil {
		t.Fatalf("unpack nullable bool from %v: %v", v, err)
	}
	return n
}

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

// A whitespace-only or empty allowlist entry is skipped, never matched, and the
// per-entry index access (canonical[len(p)]) must not go out of bounds even
// when an entry equals the whole canonical command (PKG-09 invariant 5).
func TestCommandAllowedSkipsBlankEntriesNoPanic(t *testing.T) {
	// Blank/whitespace entries must not permit anything.
	for _, allow := range [][]string{{""}, {"   "}, {"", "\t", "  "}} {
		if commandAllowed("go version", allow) {
			t.Errorf("blank allowlist %q must deny", allow)
		}
		if commandAllowed("", allow) {
			t.Errorf("blank allowlist %q must deny empty command", allow)
		}
	}
	// An entry that trims to exactly the canonical command must match via the
	// equality arm (line 189), so the boundary index is never reached for it.
	if !commandAllowed("go", []string{" go "}) {
		t.Error(`entry " go " (trims to "go") should permit canonical "go"`)
	}
	// Defense in depth: an entry longer than the canonical command must not
	// index past the end — HasPrefix is false so the && short-circuits.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("commandAllowed panicked: %v", r)
		}
	}()
	if commandAllowed("go", []string{"golang"}) {
		t.Error(`"golang" must not match canonical "go"`)
	}
}

// --- constructor policy state (enabled/allow are Go-host only) ----------------

// The enable flag and allowlist are host policy: only NewModuleWithAllow flips
// enabled, the allowlist is copied (defensive), and NewModule/NewModuleWithConfig
// stay disabled with a nil allowlist (PKG-09 invariant 1).
func TestConstructorPolicyState(t *testing.T) {
	if m := NewModule(); m.enabled || m.allow != nil {
		t.Errorf("NewModule must be disabled with nil allow, got enabled=%v allow=%v", m.enabled, m.allow)
	}
	if m := NewModuleWithConfig("/tmp", map[string]string{"A": "b"}, 0, false, false, true); m.enabled || m.allow != nil {
		t.Errorf("NewModuleWithConfig must be disabled with nil allow, got enabled=%v allow=%v", m.enabled, m.allow)
	}
	if m := NewModuleWithAllow(); !m.enabled || len(m.allow) != 0 {
		t.Errorf("NewModuleWithAllow() must be enabled with empty (deny-all) allow, got enabled=%v allow=%v", m.enabled, m.allow)
	}
	if m := NewModuleWithAllow("go", "git status"); !m.enabled || len(m.allow) != 2 {
		t.Errorf("NewModuleWithAllow(go, git status) must be enabled with 2 entries, got enabled=%v allow=%v", m.enabled, m.allow)
	}

	// The allowlist is copied, not aliased: mutating the caller's slice after
	// construction must not change the module's policy.
	src := []string{"go"}
	m := NewModuleWithAllow(src...)
	src[0] = "rm"
	if !commandAllowed("go version", m.allow) {
		t.Error("allowlist must be copied; mutating the source slice changed module policy")
	}
	if commandAllowed("rm -rf /", m.allow) {
		t.Error("mutating the source slice must not inject a new allowlist entry")
	}
}

// --- argument defaulting helpers (cwd / timeout / bool) -----------------------

// getStringWithDefault: a null value yields "" (let the OS default apply); a
// non-empty value wins; an empty value falls through to the first non-empty
// fallback, else "".
func TestGetStringWithDefault(t *testing.T) {
	cases := []struct {
		name      string
		val       *types.NullableStringOrBytes
		fallbacks []string
		want      string
	}{
		{"null yields empty", mkNullableStr(t, starlark.None), []string{"/cfg", "/cwd"}, ""},
		{"explicit value wins", mkNullableStr(t, starlark.String("/explicit")), []string{"/cfg"}, "/explicit"},
		{"empty falls to first non-empty fallback", mkNullableStr(t, starlark.String("")), []string{"", "/cfg", "/cwd"}, "/cfg"},
		{"empty with no fallback yields empty", mkNullableStr(t, starlark.String("")), nil, ""},
		{"empty with only empty fallbacks yields empty", mkNullableStr(t, starlark.String("")), []string{"", ""}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := getStringWithDefault(c.val, c.fallbacks...); got != c.want {
				t.Errorf("getStringWithDefault = %q, want %q", got, c.want)
			}
		})
	}
}

// getTimeoutWithDefault: a positive per-call timeout wins; zero or negative
// falls back to the configured default (config default here is 0 = no limit).
func TestGetTimeoutWithDefault(t *testing.T) {
	ext := NewModule().ext // config default timeout is 0
	cases := []struct {
		name string
		in   types.FloatOrInt
		want float64
	}{
		{"positive wins", types.FloatOrInt(2.5), 2.5},
		{"zero falls back to config default (0)", types.FloatOrInt(0), 0},
		{"negative falls back to config default (0)", types.FloatOrInt(-5), 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := getTimeoutWithDefault(c.in, ext); got != c.want {
				t.Errorf("getTimeoutWithDefault(%v) = %v, want %v", float64(c.in), got, c.want)
			}
		})
	}

	// With a configured non-zero default, a non-positive per-call value adopts it.
	extPreset := NewModuleWithConfig("", nil, 7.0, false, false, true).ext
	if got := getTimeoutWithDefault(types.FloatOrInt(0), extPreset); got != 7.0 {
		t.Errorf("zero per-call timeout should adopt config default 7.0, got %v", got)
	}
	if got := getTimeoutWithDefault(types.FloatOrInt(3), extPreset); got != 3 {
		t.Errorf("positive per-call timeout 3 should win over config default, got %v", got)
	}
}

// getBoolWithDefault: a null value adopts the supplied default; a concrete bool
// overrides it either way.
func TestGetBoolWithDefault(t *testing.T) {
	cases := []struct {
		name string
		val  *types.NullableBool
		def  bool
		want bool
	}{
		{"null adopts default true", mkNullableBool(t, starlark.None), true, true},
		{"null adopts default false", mkNullableBool(t, starlark.None), false, false},
		{"explicit true overrides default false", mkNullableBool(t, starlark.Bool(true)), false, true},
		{"explicit false overrides default true", mkNullableBool(t, starlark.Bool(false)), true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := getBoolWithDefault(c.val, c.def); got != c.want {
				t.Errorf("getBoolWithDefault = %v, want %v", got, c.want)
			}
		})
	}
}

// --- env map building (config default + per-call override) --------------------

// buildEnvMap merges the configured default env with the per-call dict; the
// per-call value overrides a default with the same key, and a nil/empty dict
// leaves the defaults intact.
func TestBuildEnvMap(t *testing.T) {
	t.Run("nil dict returns config defaults", func(t *testing.T) {
		m := NewModuleWithConfig("", map[string]string{"A": "1", "B": "2"}, 0, false, false, true)
		got := buildEnvMap(m.cfgMod, nil)
		if got["A"] != "1" || got["B"] != "2" || len(got) != 2 {
			t.Errorf("buildEnvMap(nil) = %v, want {A:1 B:2}", got)
		}
	})

	t.Run("empty dict returns config defaults", func(t *testing.T) {
		m := NewModuleWithConfig("", map[string]string{"A": "1"}, 0, false, false, true)
		got := buildEnvMap(m.cfgMod, starlark.NewDict(0))
		if got["A"] != "1" || len(got) != 1 {
			t.Errorf("buildEnvMap(empty) = %v, want {A:1}", got)
		}
	})

	t.Run("per-call overrides default and adds new keys", func(t *testing.T) {
		m := NewModuleWithConfig("", map[string]string{"A": "default", "KEEP": "yes"}, 0, false, false, true)
		d := starlark.NewDict(2)
		_ = d.SetKey(starlark.String("A"), starlark.String("override"))
		_ = d.SetKey(starlark.String("NEW"), starlark.String("added"))
		got := buildEnvMap(m.cfgMod, d)
		if got["A"] != "override" {
			t.Errorf("per-call A should override default, got %q", got["A"])
		}
		if got["KEEP"] != "yes" {
			t.Errorf("untouched default KEEP should remain, got %q", got["KEEP"])
		}
		if got["NEW"] != "added" {
			t.Errorf("per-call NEW should be added, got %q", got["NEW"])
		}
		if len(got) != 3 {
			t.Errorf("env map should have 3 keys, got %d: %v", len(got), got)
		}
	})

	t.Run("the returned map is a fresh copy of the defaults", func(t *testing.T) {
		// Mutating the result must not corrupt the module's stored default env,
		// so a second build sees the original defaults.
		m := NewModuleWithConfig("", map[string]string{"A": "1"}, 0, false, false, true)
		first := buildEnvMap(m.cfgMod, nil)
		first["A"] = "mutated"
		first["EXTRA"] = "x"
		second := buildEnvMap(m.cfgMod, nil)
		if second["A"] != "1" || len(second) != 1 {
			t.Errorf("default env was corrupted by a prior caller: %v", second)
		}
	})
}

// --- result struct shaping (capture/combine output matrix) --------------------

// createResultStruct must produce the documented field shape for each
// capture/combine combination, set numeric/time fields, and represent the
// error field as a string only when non-empty (else None).
func TestCreateResultStructShape(t *testing.T) {
	base := &ProcessResult{
		Success:   true,
		ExitCode:  0,
		PID:       4242,
		Stdout:    "out-data",
		Stderr:    "err-data",
		Output:    "combined-data",
		StartTime: time.Unix(1000, 0),
		EndTime:   time.Unix(1002, 0),
		Duration:  2 * time.Second,
	}

	getField := func(t *testing.T, v starlark.Value, name string) starlark.Value {
		t.Helper()
		hasAttr, ok := v.(starlark.HasAttrs)
		if !ok {
			t.Fatalf("result is not a struct with attrs")
		}
		f, err := hasAttr.Attr(name)
		if err != nil {
			t.Fatalf("missing field %q: %v", name, err)
		}
		return f
	}
	assertStr := func(t *testing.T, v starlark.Value, field, want string) {
		t.Helper()
		f := getField(t, v, field)
		s, ok := starlark.AsString(f)
		if !ok || s != want {
			t.Errorf("field %q = %v, want string %q", field, f, want)
		}
	}
	assertNone := func(t *testing.T, v starlark.Value, field string) {
		t.Helper()
		if f := getField(t, v, field); f != starlark.None {
			t.Errorf("field %q = %v, want None", field, f)
		}
	}

	t.Run("combine_output: only output has data, streams are None", func(t *testing.T) {
		v, err := createResultStruct(base, true, true)
		if err != nil {
			t.Fatal(err)
		}
		assertStr(t, v, "output", "combined-data")
		assertNone(t, v, "stdout")
		assertNone(t, v, "stderr")
		assertNone(t, v, "error") // Error == ""
		if got := getField(t, v, "exit_code"); got.String() != "0" {
			t.Errorf("exit_code = %v, want 0", got)
		}
		if got := getField(t, v, "pid"); got.String() != "4242" {
			t.Errorf("pid = %v, want 4242", got)
		}
		if got := getField(t, v, "success"); got != starlark.Bool(true) {
			t.Errorf("success = %v, want True", got)
		}
		// time/duration fields must be present (typed values, non-None).
		if getField(t, v, "start_time") == starlark.None {
			t.Error("start_time should be a time value, not None")
		}
		if getField(t, v, "duration") == starlark.None {
			t.Error("duration should be a duration value, not None")
		}
	})

	t.Run("separate streams: stdout/stderr have data, output is None", func(t *testing.T) {
		v, err := createResultStruct(base, false, true)
		if err != nil {
			t.Fatal(err)
		}
		assertStr(t, v, "stdout", "out-data")
		assertStr(t, v, "stderr", "err-data")
		assertNone(t, v, "output")
	})

	t.Run("no capture: all output fields are None", func(t *testing.T) {
		v, err := createResultStruct(base, false, false)
		if err != nil {
			t.Fatal(err)
		}
		assertNone(t, v, "stdout")
		assertNone(t, v, "stderr")
		assertNone(t, v, "output")
	})

	t.Run("error field is a string when set", func(t *testing.T) {
		failed := &ProcessResult{Success: false, ExitCode: 1, Error: "boom"}
		v, err := createResultStruct(failed, false, true)
		if err != nil {
			t.Fatal(err)
		}
		assertStr(t, v, "error", "boom")
		if got := getField(t, v, "success"); got != starlark.Bool(false) {
			t.Errorf("success = %v, want False", got)
		}
	})
}
