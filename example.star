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
            print(f"Found {cmd} at {path}")
        else:
            print(f"{cmd} not found")

    # Execute a basic command
    print("\n=== Basic Command ===")
    result = run("echo Hello from Starlark!")
    print(f"Exit code: {result.exit_code}")
    print(f"Output: {result.stdout.strip()}")

    # Show command with error
    print("\n=== Command with Error ===")
    result = run("ls /nonexistent_directory")
    print(f"Success: {result.success}")
    print(f"Exit code: {result.exit_code}")
    print(f"Error message: {result.stderr}")

    # Run a command with a timeout
    print("\n=== Command with Timeout ===")
    result = run("sleep 1", timeout=5)
    print(f"Duration: {result.duration:.2f} seconds")

    # Run command with custom environment
    print("\n=== Command with Environment ===")
    result = run("echo $GREETING, $NAME!", env={"GREETING": "Hello", "NAME": "World"})
    print(f"Output: {result.stdout.strip()}")

    # Run command with input from stdin
    print("\n=== Command with Stdin ===")
    if "windows" not in shell.lower():
        result = run("cat", stdin="Input from Starlark!")
        print(f"Output: {result.stdout.strip()}")
    else:
        result = run("powershell -Command -", stdin="Write-Host 'Input from Starlark!'")
        print(f"Output: {result.stdout.strip()}")

    # Run a complex command with pipes
    print("\n=== Command with Pipes ===")
    if "windows" not in shell.lower():
        result = run("ls -la | grep '\.star' | wc -l")
        print(f"Number of .star files: {result.stdout.strip()}")
    else:
        result = run("dir /b | findstr .star | find /c /v \"\"")
        print(f"Number of .star files: {result.stdout.strip()}")

# Execute the main function
main() 