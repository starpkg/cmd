#!/usr/bin/env starlet
# This example demonstrates how to use the cmd module.
#
# The host must enable the module with an allowlist, e.g.
#   cmd.NewModuleWithAllow("go", "git")
# Commands run via argv (no shell): pass env via env=, run one program per call.

load("cmd", "run", "which")

def main():
    # Discover where a tool lives (does not execute anything).
    for tool in ["go", "git"]:
        path = which(tool)
        if path:
            print("Found {} at {}".format(tool, path))
        else:
            print("{} not on PATH".format(tool))

    # Execute an allowlisted command.
    print("\n=== go version ===")
    result = run("go version", timeout=10)
    print("success:", result.success)
    print("exit code:", result.exit_code)
    if result.stdout:
        print(result.stdout.strip())

    # Pass environment variables explicitly (no $VAR interpolation).
    print("\n=== with env ===")
    result = run("go env GOOS", env={"CGO_ENABLED": "0"})
    print("GOOS:", result.stdout.strip() if result.stdout != None else "<no output>")

main()
