package cmd_test

// Behavior tests for the cmd module driven through a starlet machine.
//
// Sections:
//   - which (executable lookup: found path / None when absent / empty arg)
//   - run error paths (disabled module / empty command / unclosed quote /
//     non-allowlisted command / Unicode-hardening rejection)
//   - allow-all escape hatch (NewModuleWithAllowAll bypasses the allowlist but
//     not the input hardening)
//   - run options (env= / stdin= / combine_output= / capture_output= and the
//     documented ProcessResult field shape)
//   - failed execution result shape (nonzero exit; allowed-but-missing binary)
//   - NewModuleWithConfig stays disabled (host policy is construction-bound)
//   - cross-platform execution (real argv run + stdout capture proven per-OS;
//     this section runs in CI on ubuntu/macos/windows)

import (
	"runtime"
	"strings"
	"testing"

	"github.com/starpkg/cmd"
)

// --- which (executable lookup) -----------------------------------------------

func TestWhich(t *testing.T) {
	// which() does not execute anything, so a disabled module is fine for it.
	module := cmd.NewModule()

	t.Run("found returns a path", func(t *testing.T) {
		out, err := runScript(module, `
load("cmd", "which")
p = which("go")
print(type(p))
print(p != None)
`)
		if err != nil {
			t.Fatalf("which('go') script errored: %v", err)
		}
		if !strings.Contains(out, "string") || !strings.Contains(out, "True") {
			t.Errorf("which('go') should return a non-nil string path, got output:\n%s", out)
		}
	})

	t.Run("absent returns None", func(t *testing.T) {
		out, err := runScript(module, `
load("cmd", "which")
p = which("definitely-not-real-xyz")
print(p == None)
`)
		if err != nil {
			t.Fatalf("which(absent) script errored: %v", err)
		}
		if !strings.Contains(out, "True") {
			t.Errorf("which() of a missing executable should return None, got output:\n%s", out)
		}
	})

	t.Run("empty command is rejected", func(t *testing.T) {
		_, err := runScript(module, `
load("cmd", "which")
which("")
`)
		if err == nil {
			t.Fatal("which(\"\") should error")
		}
		if !strings.Contains(err.Error(), "command is required") {
			t.Errorf("which(empty) error %q should mention 'command is required'", err)
		}
	})
}

// --- run error paths ---------------------------------------------------------

func TestRunErrorPaths(t *testing.T) {
	cases := []struct {
		name   string
		module *cmd.Module
		script string
		errSub string // substring expected in the run error
	}{
		{
			name:   "disabled module refuses",
			module: cmd.NewModule(), // disabled by default
			script: `run("go version")`,
			errSub: "disabled",
		},
		{
			name:   "empty command is rejected",
			module: cmd.NewModuleWithAllow("go"),
			script: `run("")`,
			errSub: "command is required",
		},
		{
			name:   "unclosed quote fails to parse",
			module: cmd.NewModuleWithAllow("go"),
			script: `run("go 'version")`,
			errSub: "unclosed quotes",
		},
		{
			name:   "non-allowlisted command is denied",
			module: cmd.NewModuleWithAllow("go"),
			script: `run("git status")`,
			errSub: "not permitted by the allowlist",
		},
		{
			// A zero-width space (U+200B) hidden between "go" and "version" must
			// be rejected by sanitizeCommand before any matching/execution
			// (PKG-09 invariant 4) — proving the hardening is wired into run().
			// The literal U+200B byte is embedded in the raw string below.
			name:   "zero-width character is rejected before matching",
			module: cmd.NewModuleWithAllow("go"),
			script: "run(\"go​version\")",
			errSub: "format/zero-width character",
		},
		{
			// A control character (newline) cannot smuggle a second command past
			// the gate; it is rejected as a control character.
			name:   "control character is rejected before matching",
			module: cmd.NewModuleWithAllow("go"),
			script: `run("go version\nrm -rf /")`,
			errSub: "control character",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := runScript(c.module, `load("cmd", "run")`+"\n"+c.script)
			if err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), c.errSub) {
				t.Errorf("error %q should contain %q", err, c.errSub)
			}
		})
	}
}

// --- allow-all escape hatch --------------------------------------------------

// TestRunAllowAll proves NewModuleWithAllowAll bypasses the allowlist (any
// command runs even though no allowlist entry permits it) while still applying
// the PKG-09 input hardening — a control/zero-width character is rejected.
func TestRunAllowAll(t *testing.T) {
	module := cmd.NewModuleWithAllowAll()

	t.Run("an un-allowlisted command runs", func(t *testing.T) {
		// "go" is on PATH in CI on every OS, and this module has no allowlist —
		// a non-allow-all module would deny it, so success proves the bypass.
		out, err := runScript(module, `
load("cmd", "run")
res = run("go version")
print(res.success)
print("go version" in res.stdout)
`)
		if err != nil {
			t.Fatalf("allow-all run('go version') errored: %v", err)
		}
		if strings.Count(out, "True") < 2 {
			t.Errorf("allow-all should run an un-allowlisted command successfully, got:\n%s", out)
		}
	})

	t.Run("input hardening still applies", func(t *testing.T) {
		// allow-all bypasses the allowlist, never sanitizeCommand: a control
		// character (newline that could smuggle a second command) is rejected.
		_, err := runScript(module, `
load("cmd", "run")
run("go version\nrm -rf /")
`)
		if err == nil {
			t.Fatal("allow-all must still reject a control character")
		}
		if !strings.Contains(err.Error(), "control character") {
			t.Errorf("error %q should mention 'control character'", err)
		}
	})
}

// --- run options -------------------------------------------------------------

func TestRunOptions(t *testing.T) {
	module := cmd.NewModuleWithAllow("go", "cat")

	t.Run("env is passed to the child", func(t *testing.T) {
		// `go env GOOS` reports the GOOS read from the process environment, so
		// passing env={"GOOS": "js"} must surface in stdout.
		out, err := runScript(module, `
load("cmd", "run")
r = run("go env GOOS", env={"GOOS": "js"})
print("success:", r.success)
print("goos:", r.stdout.strip())
`)
		if err != nil {
			t.Fatalf("run with env errored: %v", err)
		}
		if !strings.Contains(out, "success: True") {
			t.Errorf("expected success, got:\n%s", out)
		}
		if !strings.Contains(out, "goos: js") {
			t.Errorf("env= should propagate GOOS=js to the child, got:\n%s", out)
		}
	})

	t.Run("stdin is delivered to the child", func(t *testing.T) {
		// `cat` echoes its stdin to stdout.
		out, err := runScript(module, `
load("cmd", "run")
r = run("cat", stdin="hello from stdin")
print("success:", r.success)
print("got:", r.stdout)
`)
		if err != nil {
			t.Fatalf("run with stdin errored: %v", err)
		}
		if !strings.Contains(out, "success: True") {
			t.Errorf("expected success, got:\n%s", out)
		}
		if !strings.Contains(out, "got: hello from stdin") {
			t.Errorf("stdin= should be delivered to the child, got:\n%s", out)
		}
	})

	t.Run("combine_output yields output and None streams", func(t *testing.T) {
		out, err := runScript(module, `
load("cmd", "run")
r = run("go version", combine_output=True)
print("success:", r.success)
print("output_is_str:", type(r.output) == "string")
print("output_nonempty:", len(r.output) > 0)
print("stdout_none:", r.stdout == None)
print("stderr_none:", r.stderr == None)
`)
		if err != nil {
			t.Fatalf("run with combine_output errored: %v", err)
		}
		for _, want := range []string{
			"success: True",
			"output_is_str: True",
			"output_nonempty: True",
			"stdout_none: True",
			"stderr_none: True",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("combine_output result missing %q, got:\n%s", want, out)
			}
		}
	})

	t.Run("capture_output false yields all None output fields", func(t *testing.T) {
		out, err := runScript(module, `
load("cmd", "run")
r = run("go version", capture_output=False)
print("success:", r.success)
print("exit_code:", r.exit_code)
print("stdout_none:", r.stdout == None)
print("stderr_none:", r.stderr == None)
print("output_none:", r.output == None)
print("pid_positive:", r.pid > 0)
`)
		if err != nil {
			t.Fatalf("run with capture_output=False errored: %v", err)
		}
		for _, want := range []string{
			"success: True",
			"exit_code: 0",
			"stdout_none: True",
			"stderr_none: True",
			"output_none: True",
			"pid_positive: True",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("capture_output=False result missing %q, got:\n%s", want, out)
			}
		}
	})
}

// --- failed execution result shape -------------------------------------------

// A command that is allowlisted and runs but exits nonzero must yield
// success=False with the real exit code and a None error (the process ran; it
// just failed). `go <bad-subcommand>` is cross-platform and exits nonzero.
func TestRunFailedCommandResult(t *testing.T) {
	module := cmd.NewModuleWithAllow("go")

	t.Run("nonzero exit reports failure with exit code", func(t *testing.T) {
		out, err := runScript(module, `
load("cmd", "run")
r = run("go this-subcommand-does-not-exist")
print("success:", r.success)
print("nonzero:", r.exit_code != 0)
print("error_none:", r.error == None)
`)
		if err != nil {
			t.Fatalf("run of a failing command should not raise, got: %v", err)
		}
		for _, want := range []string{"success: False", "nonzero: True", "error_none: True"} {
			if !strings.Contains(out, want) {
				t.Errorf("failed-command result missing %q, got:\n%s", want, out)
			}
		}
	})

	t.Run("allowlisted but missing binary reports a start error", func(t *testing.T) {
		// The binary is permitted by the allowlist but not on PATH: run() must
		// not raise; the failure surfaces in result.error, success is False.
		missing := cmd.NewModuleWithAllow("definitely-not-real-xyz")
		out, err := runScript(missing, `
load("cmd", "run")
r = run("definitely-not-real-xyz")
print("success:", r.success)
print("has_error:", r.error != None)
`)
		if err != nil {
			t.Fatalf("run of a missing binary should not raise, got: %v", err)
		}
		if !strings.Contains(out, "success: False") {
			t.Errorf("expected success=False, got:\n%s", out)
		}
		if !strings.Contains(out, "has_error: True") {
			t.Errorf("a missing binary should populate result.error, got:\n%s", out)
		}
	})
}

// --- NewModuleWithConfig stays disabled --------------------------------------

// NewModuleWithConfig presets behavioral defaults but does NOT enable execution
// (PKG-09 invariant 1 / 6): there is no allow-setter, so run() still refuses
// until the host constructs via NewModuleWithAllow.
func TestNewModuleWithConfigDisabled(t *testing.T) {
	module := cmd.NewModuleWithConfig("/tmp", map[string]string{"A": "b"}, 5, true, false, true)
	_, err := runScript(module, `
load("cmd", "run")
run("go version")
`)
	if err == nil {
		t.Fatal("NewModuleWithConfig must yield a disabled module; run() should error")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Errorf("error %q should report the module is disabled", err)
	}
}

// --- cross-platform execution ------------------------------------------------

// TestCrossPlatformExecution proves that real argv execution and stdout capture
// behave identically on every OS in the CI matrix (ubuntu / macos / windows).
// `go` is on PATH on all GitHub runners; on Windows exec.LookPath resolves
// go.exe. `go env GOOS` prints the GOOS the toolchain was built for, which on a
// native build equals the host runtime.GOOS — so the script's captured stdout
// must match the host's runtime.GOOS ("linux", "darwin", or "windows").
func TestCrossPlatformExecution(t *testing.T) {
	module := cmd.NewModuleWithAllow("go")

	out, err := runScript(module, `
load("cmd", "run")
r = run("go env GOOS")
print("success:", r.success)
print("exit_code:", r.exit_code)
print("goos:", r.stdout.strip())
`)
	if err != nil {
		t.Fatalf("cross-platform run errored: %v", err)
	}
	if !strings.Contains(out, "success: True") {
		t.Errorf("expected success=True, got:\n%s", out)
	}
	if !strings.Contains(out, "exit_code: 0") {
		t.Errorf("expected exit_code=0, got:\n%s", out)
	}
	// The trimmed stdout must equal the host GOOS this test binary was built for.
	wantGOOS := "goos: " + runtime.GOOS
	if !strings.Contains(out, wantGOOS) {
		t.Errorf("expected stdout to report host GOOS %q (%q), got:\n%s", runtime.GOOS, wantGOOS, out)
	}
}
