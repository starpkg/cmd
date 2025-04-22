#!/usr/bin/env starlet
# This example demonstrates how to use the cmd module

load("cmd", "run", "which", "find_shell")

def main():
    # Print system shell
    shell = find_shell()
    print("System shell:", shell)

    # Find common commands
    for cmd in ["git", "python", "ls", "cat"]:
        path = which(cmd)
        if path:
            print("Found {} at {}".format(cmd, path))
        else:
            print("{} not found".format(cmd))

    # Execute a basic command
    print("\n=== Basic Command ===")
    result = run("echo Hello from Starlark!")
    print("Exit code: {}".format(result.exit_code))
    print("Output: {}".format(result.stdout.strip()))

    # Show command with error
    print("\n=== Command with Error ===")
    result = run("ls /nonexistent_directory")
    print("Success: {}".format(result.success))
    print("Exit code: {}".format(result.exit_code))
    print("Error message: {}".format(result.stderr))

    # Run a command with a timeout
    print("\n=== Command with Timeout ===")
    result = run("sleep 1", timeout=5)
    print("Duration: {:.2f} seconds".format(result.duration))

    # Run command with custom environment
    print("\n=== Command with Environment ===")
    result = run("echo $GREETING, $NAME!", env={"GREETING": "Hello", "NAME": "World"})
    print("Output: {}".format(result.stdout.strip()))

    # Run command with input from stdin
    print("\n=== Command with Stdin ===")
    if "windows" not in shell.lower():
        result = run("cat", stdin="Input from Starlark!")
        print("Output: {}".format(result.stdout.strip()))
    else:
        result = run("powershell -Command -", stdin="Write-Host 'Input from Starlark!'")
        print("Output: {}".format(result.stdout.strip()))

    # Run a complex command with pipes
    print("\n=== Command with Pipes ===")
    if "windows" not in shell.lower():
        result = run("ls -la | grep '.star' | wc -l")
        print("Number of .star files: {}".format(result.stdout.strip()))
    else:
        result = run("dir /b | findstr .star | find /c /v \"\"")
        print("Number of .star files: {}".format(result.stdout.strip()))

# Execute the main function
main()
