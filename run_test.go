package cmd_test

// Behavior tests for the cmd module driven through a starlet machine.
//
// Sections:
//   - which (executable lookup: found path / None when absent)
//   - run error paths (disabled module / empty command / unclosed quote /
//     non-allowlisted command)
//   - run options (env= / stdin= / combine_output= / capture_output= and the
//     documented ProcessResult field shape)

import (
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
