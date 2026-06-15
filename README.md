# 🐚 `cmd` — Starlark module for executing external commands

[![codecov](https://codecov.io/gh/starpkg/cmd/graph/badge.svg)](https://codecov.io/gh/starpkg/cmd)
![binary footprint](https://img.shields.io/badge/binary_footprint-%2B0.3_MB-blue)

The `cmd` module lets Starlark scripts run external programs across Windows,
macOS, and Linux. Execution is **always via argv** (`exec.Command`), never
through a shell, and the module is **disabled by default** — the Go host must
opt in with an allowlist before any command can run.

## Security model (read this first)

`cmd` is deny-by-default and the policy is set by the **Go host**, never by the
script or by environment variables:

1. **Disabled by default.** A module from `NewModule()` refuses every `run()`
   call. The host enables execution with `cmd.NewModuleWithAllow(...)`.
2. **Allowlist, deny-by-default.** Once enabled, a command runs only if its
   canonical form (argv joined by single spaces) matches an allowlist prefix at
   a word boundary: `"git"` permits `git status` but not `gitleaks`; `"go test"`
   permits `go test ./...` but not `go build`. An empty allowlist permits
   nothing.
3. **No shell interpolation.** Commands are split into argv and executed
   directly — there is no `/bin/sh -c`. Shell features like `$VAR`, `&&`, `|`,
   globbing, and `;` are **not** interpreted; pass environment via `env=` and
   run one program per call.
4. **Unicode hardening.** Command text is rejected if it contains control
   characters or Unicode format/zero-width characters (e.g. zero-width space,
   BiDi overrides, BOM), which could otherwise hide or reorder text to slip past
   the allowlist.

```go
// Go host: enable cmd with an allowlist
module := cmd.NewModuleWithAllow("go", "git status", "ls")
loader := module.LoadModule()
```

## Cross-platform note

Because there is no shell, the first token must be a real **executable on
`PATH`** for the current OS. Shell built-ins are *not* available as argv:

- Windows `dir`, and `echo` as a `cmd.exe` builtin, are not runnable directly —
  use real binaries (`go`, `git`, `where`, …) or invoke the interpreter
  explicitly if you allowlist it.
- `echo` is a real binary on most Unix systems but not on Windows.

Prefer real cross-platform tools (e.g. `go`, `git`) in portable scripts.

## Configuration

| Key | Type | Description | Default |
|-----|------|-------------|---------|
| `cwd` | string | Default working directory for commands | current directory |
| `env` | dict | Environment variables added to every command | `{}` |
| `timeout` | float | Default timeout in seconds (0 = none) | `0` |
| `combine_output` | bool | Combine stdout and stderr | `false` |
| `realtime_output` | bool | Echo output to the console in real time | `false` |
| `capture_output` | bool | Capture output into the result | `true` |

The enable flag and allowlist are **not** configuration keys — they are set in
Go via `NewModuleWithAllow` and cannot be widened by a script or environment
variable.

## Usage

### Running an allowlisted command

```python
load("cmd", "run")

# Host enabled the module with allow=["go"]
result = run("go version")
print("ok:", result.success)      # True
print("code:", result.exit_code)  # 0
print(result.stdout)
```

### Working directory, timeout, and input

```python
load("cmd", "run")

result = run("go env GOOS", cwd="/path/to/project", timeout=10)

# Provide stdin to a program
result = run("cat", stdin="hello from stdin")
print(result.stdout)
```

### Environment variables

Pass variables explicitly via `env=` — they are set in the child process
environment (there is no `$VAR` interpolation in the command string):

```python
load("cmd", "run")

result = run("go env GONOSUMCHECK", env={"GONOSUMCHECK": "1"})
```

### Capturing vs. combining output

```python
load("cmd", "run")

# Combine stdout+stderr into result.output
result = run("go vet ./...", combine_output=True)
print(result.output)

# Don't capture; stream to the console instead
run("go build ./...", capture_output=False, realtime_output=True)
```

### Finding an executable

```python
load("cmd", "which")

path = which("go")   # full path, or None if not on PATH
```

## API Reference

### Functions

#### `run(command, **kwargs)`

Splits `command` into argv, checks it against the allowlist, and executes it
directly. Returns a `ProcessResult` struct. Raises if the module is disabled or
the command is not permitted.

Parameters:

- `command` (string, required): the command line (split into argv; no shell)
- `cwd` (string, optional): working directory (default: configured / current)
- `env` (dict, optional): extra environment variables
- `stdin` (string, optional): input to provide on stdin
- `timeout` (float, optional): max execution time in seconds (default: 0 = none)
- `combine_output` (bool, optional): combine stdout and stderr (default: false)
- `realtime_output` (bool, optional): echo to console in real time (default: false)
- `capture_output` (bool, optional): capture output (default: true)

#### `which(command)`

Returns the full path to an executable on `PATH`, or `None` if not found. Does
not execute anything.

### Constructors (Go)

- `NewModule()` — disabled; `run()` always errors.
- `NewModuleWithConfig(cwd, env, timeout, combineOutput, realtimeOutput, captureOutput)` — disabled, with preset defaults.
- `NewModuleWithAllow(allow ...string)` — enabled, with the given allowlist.

Note: `NewModuleWithConfig` returns a **disabled** module — there is no separate
allow-setter, so to enable it you must construct the module via
`NewModuleWithAllow(...)` (which sets the allowlist and enables execution).

### ProcessResult Struct

The `ProcessResult` struct contains the following fields:

- `success` (bool): True if the command exited with code 0
- `exit_code` (int): the command's exit code
- `stdout` (string or None): standard output (if not combined and captured)
- `stderr` (string or None): standard error (if not combined and captured)
- `output` (string or None): combined output (when combined and captured)
- `error` (string or None): error message for execution failures
- `pid` (int): process ID
- `start_time` (time): start timestamp as a Starlark time value
- `end_time` (time): end timestamp as a Starlark time value
- `duration` (duration): execution time as a Starlark duration value

Notes:

- When `combine_output=True`, only `output` has data; `stdout`/`stderr` are None.
- When `combine_output=False`, only `stdout`/`stderr` have data; `output` is None.
- When `capture_output=False`, all output fields are None.
- The time fields work with Starlark's `time` module.
