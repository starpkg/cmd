# 🐚 `cmd` — Starlark module for executing external commands

[![codecov](https://codecov.io/gh/starpkg/cmd/graph/badge.svg)](https://codecov.io/gh/starpkg/cmd)
![binary footprint](https://img.shields.io/badge/binary_footprint-%2B0.4_MB-blue)

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

The enable flag and allowlist are **not** configuration keys — they are set in
Go via `NewModuleWithAllow` and cannot be widened by a script or environment
variable.

### Cross-platform note

Because there is no shell, the first token must be a real **executable on
`PATH`** for the current OS. Shell built-ins are *not* available as argv:

- Windows `dir`, and `echo` as a `cmd.exe` builtin, are not runnable directly —
  use real binaries (`go`, `git`, `where`, …) or invoke the interpreter
  explicitly if you allowlist it.
- `echo` is a real binary on most Unix systems but not on Windows.

Prefer real cross-platform tools (e.g. `go`, `git`) in portable scripts.

## Installation

```bash
go get github.com/starpkg/cmd
```

## Quick Start

Construct the module with an allowlist in Go, wire it into a Starlet
interpreter, then `load("cmd", …)` from a script:

```go
package main

import (
    "fmt"

    "github.com/1set/starlet"
    "github.com/starpkg/cmd"
)

func main() {
    // Enable execution with an allowlist (deny-by-default).
    cmdModule := cmd.NewModuleWithAllow("go", "git status")
    interpreter := starlet.New(
        starlet.WithModuleLoader("cmd", cmdModule.LoadModule()),
    )

    script := `
load("cmd", "run", "which")

# Look up an executable without running anything.
print("go at:", which("go"))

# Run an allowlisted command and inspect the result.
result = run("go version")
print("ok:", result.success, "code:", result.exit_code)
print(result.stdout)
`

    if err := interpreter.ExecScript("example.star", script); err != nil {
        fmt.Println("Error:", err)
    }
}
```

For the complete per-builtin reference — signatures, parameters, returns,
errors, examples — the `ProcessResult` fields, and the configuration accessors,
see **[docs/API.md](docs/API.md)**.

## Starlark API at a glance

Top-level builtins (`load("cmd", …)`):

- `run(command, cwd?, env?, stdin?, timeout?, combine_output?, realtime_output?, capture_output?)` — run an allowlisted command via argv; returns a `ProcessResult`.
- `which(command)` — return the full path of an executable on `PATH`, or `None`; never executes.

`run` returns a `ProcessResult` struct with fields `success`, `exit_code`,
`stdout`, `stderr`, `output`, `error`, `pid`, `start_time`, `end_time`, and
`duration`.

See **[docs/API.md](docs/API.md)** for the full signatures, return values,
errors, and examples of both builtins above.

## Configuration

The module's behavioral defaults (`cwd`, `env`, `timeout`, `combine_output`,
`realtime_output`, `capture_output`) are configured via environment variables
(`CMD_*`) or per-option `get_<key>` / `set_<key>` accessor builtins, and serve
as defaults for `run`. The enable flag and allowlist are **not** config — they
are host-only and cannot be widened from a script. See the
[Configuration section of docs/API.md](docs/API.md#configuration) for the full
option table, defaults, and accessors.

## License

MIT
