# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`starpkg/cmd` is an **L4 domain module** of the Star\* ecosystem: it exposes a guarded way for Starlark scripts to **run external programs** — execute an argv, capture stdout/stderr/exit code, or locate a binary on `PATH`. It fits the starpkg positioning — *support for necessary local operations + simple abstractions over common online services* — squarely on the **local** side: it is a **local-capability** module (process execution on the host), with no online-service abstraction.

Because spawning processes is the single most dangerous capability a script can hold, `cmd` is **disabled by default** and **deny-by-default once enabled** (PKG-09). The host opts in, in Go, with an allowlist; nothing a script does (or any environment variable) can widen that policy.

Layer position: depends downward on `starpkg/base` (the module/config system), `1set/starlet` (the Machine + `dataconv`/`dataconv/types` arg coercion), and transitively `1set/starlight` + `go.starlark.net`. The only third-party SDK is `bitbucket.org/creachadair/shell` — used **solely to split** a command string into argv (POSIX-style word splitting); it never executes anything. Nothing in the ecosystem depends on this module.

## Dev commands

Pure Go library with a Makefile. From this repo:

```bash
make test                                  # -race -cover, the working bar
make ci                                    # -race -cover profile + bench compile (what CI runs)
go test ./... -run TestCommandAllowed      # a single test
gofmt -l . && go vet ./...                 # must be clean before commit
```

**Verify on the go floor in Docker** — this repo's floor is **go 1.19** (see Release discipline), older than the local toolchain. Behavior on the floor must be checked in a container:

```bash
docker run --rm -v "$PWD":/src -v "$HOME/go/pkg/mod":/go/pkg/mod -w /src golang:1.19 go test -race -count=1 ./...
```

`TestRunOptions`/`TestCrossPlatformExecution` shell out to the real `go` toolchain (always on PATH in CI on ubuntu/macos/windows) and to `cat` (Unix); they prove argv execution + capture per-OS. The `../test/cmd/*.star` integration scripts live in the **private `starpkg/test` repo** and are absent in CI — keep any harness that walks them auto-skipping when that directory is missing.

## Architecture (the part that spans files)

The whole module is **one source file, `cmd.go`**, structured as a *policy gate in front of `os/exec`*. There is exactly one path from a script call to a spawned process, and every hardening check sits on it.

- **`Module`** — holds a `base.ConfigurableModule` (+ its `Extend()` view for typed config reads) plus two **host-policy** fields that are *not* config keys: `enabled` and `allow []string`. They are set only in Go and can never be read from a script or an env var.
- **Constructors** — `NewModule()` (disabled, default config), `NewModuleWithConfig(...)` (disabled, preset config), `NewModuleWithAllow(allow ...)` (**enabled** with an allowlist). There is deliberately no allow-*setter* method: enabling and the allowlist are bound together at construction.
- **`LoadModule()`** — registers the two script-facing builtins: **`run`** (`m.run`) and **`which`** (`m.starWhich`), merged with the config-backed module surface from `base`.
- **`run` (the gate)** — the ordered pipeline: unpack args → require non-empty command → `sanitizeCommand` (UTF-8 + reject control/format chars) → split to argv via `creachadair/shell` (with a Windows backslash-doubling workaround) → join to a *canonical* string → `commandAllowed` check → resolve config defaults → `executeArgv`. The disabled-module check is the **first** thing `run` does.
- **`which` (`starWhich`)** — `exec.LookPath` only; returns the path or `None`. It does **not** execute, so it works even on a disabled module.
- **`executeArgv`** — the actual `exec.CommandContext` call (timeout from the thread context), env assembly (process env + extra `env=`), stdin wiring, and the stdout/stderr capture matrix driven by `capture_output`/`combine_output`/`realtime_output`. Returns a `ProcessResult`.
- **`ProcessResult` → `createResultStruct`** — marshals the Go result into a `starlarkstruct` with `success`/`exit_code`/`pid`/`stdout`/`stderr`/`output`/`error`/`start_time`/`end_time`/`duration` (times via `go.starlark.net/lib/time`).
- **Config** — six keys (`timeout`, `cwd`, `env`, `combine_output`, `realtime_output`, `capture_output`), each with a `CMD_<KEY>` env var, defined through `base.NewConfigOption`. These are *behavioral defaults*, never the enable/allow policy.

## Invariants / hardening (preserve when editing)

The PKG-09 security posture is the reason this module exists in its current form. **Do not regress any of these:**

1. **Disabled by default; deny-by-default once enabled.** `NewModule()`/`NewModuleWithConfig()` yield a module whose `run` returns an error. Only `NewModuleWithAllow` flips `enabled`. An empty allowlist enables the module but permits *nothing*. The enable/allow policy is Go-host state — never expose it as a config key, env var, or script-settable field.
2. **Never a shell.** Execution is always argv via `exec.CommandContext`; there is no `/bin/sh -c`. `$VAR`, `&&`, `|`, `;`, and globbing are not interpreted. `creachadair/shell` is used only to *split* the string — if you change splitting, keep it argv-only.
3. **Allowlist matches at a word boundary.** `commandAllowed` matches an entry as a prefix of the canonical (single-space-joined) argv only when followed by end-of-string or a space: `"git"` permits `git status` but not `gitleaks`. Don't loosen this to a bare `strings.HasPrefix`.
4. **Unicode hardening before matching.** `sanitizeCommand` rejects invalid UTF-8, control characters, and Unicode format/zero-width characters (category `Cf`: zero-width space, BiDi overrides, BOM) — these can hide or reorder text so a command looks allowlisted while resolving to something else. Sanitize *before* splitting/matching, never after.
5. **No host panics from script input.** `run`/`which` validate and return script-level errors (empty command, unclosed quotes, denied command); they must not panic the host.
6. **Backward compatibility.** Config defaults reproduce historical behavior (`capture_output` defaults true; `timeout` 0 = no limit). The enable-gate is the one *new* lever and it defaults to off, so a host that already passes an allowlist is unaffected; a host that doesn't gets the safe (disabled) default.

## Test organization

Group by functional goal — **do not add one `*_test.go` per fix.** Three thematic files, each opened with a commented section list:

- **`cmd_test.go`** (`package cmd`, white-box) — pure-Go unit tests for the gate internals: allowlist prefix matching, sanitization / Unicode hardening.
- **`run_test.go`** (`package cmd_test`, black-box) — behavior through a real starlet machine: `which` lookup, `run` error paths (disabled/empty/unclosed-quote/denied), `run` options (`env`/`stdin`/`combine_output`/`capture_output` + the `ProcessResult` shape), and cross-platform execution (runs in CI on ubuntu/macos/windows).
- **`example_test.go`** — the `runScript` helper + runnable `Example`/`ExampleModule_disabled` godoc examples.

Add a new test as a **section in the matching file**, not a new file. Tests are table/subtest-driven; no third-party test framework. The `../test/cmd/*.star` scripts are the private-repo integration layer and auto-skip when absent.

## Documentation

Three layers must stay in sync (the doc standard, `plan/starpkg文档标准（DOC-STD）`):

- **`README.md`** — every script-facing builtin (`run`, `which`) and every `ProcessResult` field documented as a backtick whole-word; the host-policy levers (`NewModule`, `NewModuleWithAllow`, `NewModuleWithConfig`) under the security model. Function names/signatures must match the code.
- **GoDoc** — package comment + a doc comment whose first word is the symbol name on every exported symbol (`ModuleName`, `Module`, `ProcessResult`, the three `New*` constructors, `LoadModule`); gated by `revive`'s `exported` rule in CI.
- **Doc-coverage gate** — `go run github.com/1set/meta/doccov@master .` (wired via `doc-coverage: true` in `.github/workflows/build.yml`) fails CI if a `starlark.NewBuiltin` a script can load isn't a backtick word in the README.

## Release discipline

- **Floor = go 1.19** (this repo's `go.mod`); a repo's floor only rises in its own pin PR.
- **CI matrix** = `[1.19.x, 1.25.x]` via the centralized reusable workflow in `1set/meta` (`go-ci.yml`, pinned to a full commit SHA). The doc-coverage and (informational) govulncheck steps run from that workflow.
- **Pin upgrade is the last PR of the series.** Upgrading the `go.starlark.net` pin + `1set` deps + go floor happens *after* all fixes, as one isolated PR; **don't tag a release until that pin PR merges.**
- **Bumping the version, the go floor, or tagging are user-confirmed actions** — never tag autonomously; draft title + notes and get explicit approval first; default to patch bumps; published tags are immutable.
- **Open-source boundary.** This repo is public — keep internal/business names out of code, comments, commits, and PRs.
