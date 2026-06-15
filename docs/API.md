# `cmd` — Starlark API Reference

The complete reference for every script-facing builtin and configuration
accessor exposed by the `cmd` module. For the security model, installation, and
a quickstart, see the [README](../README.md).

The module exposes two top-level builtins via `load("cmd", …)` — `run` and
`which` — plus a set of configuration accessors (`get_<key>` / `set_<key>`)
generated from the module's options. `run` returns a `ProcessResult` struct.

> **Security first.** Execution is **disabled by default** and
> **deny-by-default once enabled**. The Go host opts in with an allowlist via
> `cmd.NewModuleWithAllow(...)`; nothing a script does (or any environment
> variable) can widen that policy. See the
> [Host policy](#host-policy-go-side) section below and the README's security
> model for the full picture.

## Contents

- [Functions](#functions)
  - [`run(command, …)`](#runcommand-)
  - [`which(command)`](#whichcommand)
- [ProcessResult struct](#processresult-struct)
- [Host policy (Go side)](#host-policy-go-side)
- [Configuration](#configuration)

## Functions

### `run(command, …)`

Splits `command` into argv, hardens and checks it against the host allowlist,
and executes it directly via `exec.Command` — **never** through a shell.
Returns a [`ProcessResult`](#processresult-struct) struct describing the
outcome.

Because there is no shell, the first token must be a real executable on `PATH`
for the current OS, and shell features (`$VAR` interpolation, `&&`, `|`, `;`,
globbing) are **not** interpreted. Pass environment variables explicitly via
`env=` and run one program per call.

**Parameters:**

- `command` (string, required): the command line, split into argv (no shell)
- `cwd` (string, optional): working directory (default: the `cwd` config option, else the current directory)
- `env` (dict, optional): extra environment variables, merged on top of the `env` config option and the host process environment
- `stdin` (string, optional): input written to the command's standard input
- `timeout` (float, optional): max execution time in seconds (default: the `timeout` config option; `0` = no limit)
- `combine_output` (bool, optional): combine stdout and stderr into `output` (default: the `combine_output` config option, normally `false`)
- `realtime_output` (bool, optional): also echo output to the console as it is produced (default: the `realtime_output` config option, normally `false`)
- `capture_output` (bool, optional): capture output into the result (default: the `capture_output` config option, normally `true`)

**Returns:** a [`ProcessResult`](#processresult-struct) struct.

**Errors (raised to the script):**

- the module is disabled (constructed without `NewModuleWithAllow`)
- `command` is empty
- `command` is not valid UTF-8, or contains a control or Unicode
  format/zero-width character
- `command` has invalid syntax or unclosed quotes (cannot be split into argv)
- the command's canonical form is not permitted by the allowlist

A command that runs but exits non-zero does **not** raise; it returns a
`ProcessResult` with `success = False` and the non-zero `exit_code`. A command
that fails to start (e.g. executable not found) returns a `ProcessResult` with
`error` set.

**Example — run an allowlisted command:**

```python
load("cmd", "run")

# Host enabled the module with allow=["go"]
result = run("go version")
print("ok:", result.success)      # True
print("code:", result.exit_code)  # 0
print(result.stdout)
```

**Example — working directory, timeout, and stdin:**

```python
load("cmd", "run")

result = run("go env GOOS", cwd="/path/to/project", timeout=10)

# Provide stdin to a program
result = run("cat", stdin="hello from stdin")
print(result.stdout)              # "hello from stdin"
```

**Example — environment variables:**

Variables are set in the child process environment; there is no `$VAR`
interpolation in the command string:

```python
load("cmd", "run")

result = run("go env GONOSUMCHECK", env={"GONOSUMCHECK": "1"})
```

**Example — capturing vs. combining output:**

```python
load("cmd", "run")

# Combine stdout+stderr into result.output
result = run("go vet ./...", combine_output=True)
print(result.output)

# Don't capture; stream to the console instead
run("go build ./...", capture_output=False, realtime_output=True)
```

### `which(command)`

Returns the full path to an executable found on `PATH`, or `None` if it is not
found. Uses `exec.LookPath` only — it does **not** execute anything, and works
even when the module is disabled (it is not gated by the allowlist).

**Parameters:**

- `command` (string, required): the executable name to look up

**Returns:** a string path, or `None` if the executable is not on `PATH`.

**Errors (raised to the script):**

- `command` is empty

**Example:**

```python
load("cmd", "which")

path = which("go")   # full path, or None if not on PATH
if path:
    print("go is at", path)
```

## ProcessResult struct

`run` returns a struct with the following fields:

| Field | Type | Description |
|-------|------|-------------|
| `success` | bool | `True` if the command exited with code 0 |
| `exit_code` | int | the command's exit code |
| `stdout` | string or None | standard output (when captured and not combined) |
| `stderr` | string or None | standard error (when captured and not combined) |
| `output` | string or None | combined output (when captured and combined) |
| `error` | string or None | error message for execution failures (e.g. failed to start, timeout) |
| `pid` | int | process ID |
| `start_time` | time | start timestamp as a Starlark `time` value |
| `end_time` | time | end timestamp as a Starlark `time` value |
| `duration` | duration | execution time as a Starlark `duration` value |

**Field notes:**

- When `combine_output=True`, only `output` has data; `stdout` and `stderr` are `None`.
- When `combine_output=False`, only `stdout` and `stderr` have data; `output` is `None`.
- When `capture_output=False`, all three output fields (`stdout`, `stderr`, `output`) are `None`.
- `error` is `None` on a clean run; it is set when the command could not start or timed out. A command that starts and exits non-zero sets `success=False` and `exit_code`, not `error`.
- `start_time` / `end_time` are Starlark `time` values and `duration` is a Starlark `duration` value, so they interoperate with the `time` module.

## Host policy (Go side)

These are **not** script-facing builtins and **not** configuration keys — they
are host policy, set only in Go at construction time, and can never be read or
widened by a script or an environment variable.

- `NewModule()` — a **disabled** module with default config; `run()` always errors.
- `NewModuleWithConfig(cwd, env, timeout, combineOutput, realtimeOutput, captureOutput)` — a **disabled** module with preset config defaults.
- `NewModuleWithAllow(allow ...string)` — an **enabled** module with the given allowlist.

Each allowlist entry is a prefix matched against the canonical command (argv
joined by single spaces) at a word boundary: `"git"` permits `git status` but
not `gitleaks`; `"go test"` permits `go test ./...` but not `go build`. An empty
allowlist enables the module but permits nothing (deny-all).

There is deliberately no allow-*setter*: enabling and the allowlist are bound
together at construction. To enable execution you must construct the module via
`NewModuleWithAllow(...)`; `NewModuleWithConfig(...)` returns a disabled module.

```go
// Go host: enable cmd with an allowlist
module := cmd.NewModuleWithAllow("go", "git status", "ls")
loader := module.LoadModule()
```

## Configuration

Each module configuration option is exposed to scripts as a pair of generated
accessor builtins (loaded from the `cmd` module alongside `run` and `which`):

- **`get_<key>()`** — returns the current value of the option.
- **`set_<key>(value)`** — sets the option (returns `None`).

An option's value resolves in priority order: an explicit `set_<key>` value, the
environment variable, then the default. These options are **behavioral
defaults** for `run` — used when the corresponding `run` argument is omitted.
They are not the enable/allow security policy, which is host-only (see
[Host policy](#host-policy-go-side)) and cannot be changed from a script or an
environment variable.

None of the `cmd` options are secret, so every option exposes **both**
`get_<key>` and `set_<key>`. (A secret option would expose only its `set_<key>`
accessor — never a getter — but this module has none.)

| Option | Getter | Setter | Type | Env var | Default | Description |
|--------|--------|--------|------|---------|---------|-------------|
| `cwd` | `get_cwd` | `set_cwd` | string | `CMD_CWD` | current directory | Default working directory for commands |
| `env` | `get_env` | `set_env` | dict | `CMD_ENV` | `{}` | Environment variables added to every command |
| `timeout` | `get_timeout` | `set_timeout` | float | `CMD_TIMEOUT` | `0` | Default timeout in seconds (`0` = no limit) |
| `combine_output` | `get_combine_output` | `set_combine_output` | bool | `CMD_COMBINE_OUTPUT` | `false` | Combine stdout and stderr into `output` |
| `realtime_output` | `get_realtime_output` | `set_realtime_output` | bool | `CMD_REALTIME_OUTPUT` | `false` | Echo output to the console in real time |
| `capture_output` | `get_capture_output` | `set_capture_output` | bool | `CMD_CAPTURE_OUTPUT` | `true` | Capture output into the result |

**Example:**

```python
load(
    "cmd",
    "run",
    # getters
    "get_cwd", "get_env", "get_timeout",
    "get_combine_output", "get_realtime_output", "get_capture_output",
    # setters
    "set_cwd", "set_env", "set_timeout",
    "set_combine_output", "set_realtime_output", "set_capture_output",
)

set_timeout(30.0)
print(get_timeout())  # 30.0

# Host enabled the module with allow=["go"]
result = run("go test ./...")  # inherits the 30s default timeout
```
