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

    # Execute a command directly (without a shell)
    print("\n=== Command without Shell ===")
    result = run("echo Hello without shell", shell=None)
    print("Success: {}".format(result.success))
    print("Output: {}".format(result.stdout.strip() if result.stdout != None else "<no output>"))

    # Show command with error
    print("\n=== Command with Error ===")
    result = run("ls /nonexistent_directory")
    print("Success: {}".format(result.success))
    print("Exit code: {}".format(result.exit_code))
    print("Error message: {}".format(result.stderr if result.stderr != None else "<no error>"))

    # Run a command with a timeout
    print("\n=== Command with Timeout ===")
    result = run("sleep 1", timeout=5)
    print("Duration: {} seconds".format(result.duration))

    # Run command with custom environment
    print("\n=== Command with Environment ===")
    result = run("echo $GREETING, $NAME!", env={"GREETING": "Hello", "NAME": "World"})
    print("Output: {}".format(result.stdout.strip() if result.stdout != None else "<no output>"))

    # Run command with cwd (custom directory)
    print("\n=== Command with Custom Directory ===")
    result = run("pwd", cwd="/tmp")
    print("Working directory output: {}".format(result.stdout.strip() if result.stdout != None else "<no output>"))

    # Run command with input from stdin
    print("\n=== Command with Stdin ===")
    if "windows" not in shell.lower():
        result = run("cat", stdin="Input from Starlark!")
        print("Output: {}".format(result.stdout.strip() if result.stdout != None else "<no output>"))
    else:
        result = run("powershell -Command -", stdin="Write-Host 'Input from Starlark!'")
        print("Output: {}".format(result.stdout.strip() if result.stdout != None else "<no output>"))

    # Run command with separate stdout and stderr
    print("\n=== Command with Separate Stdout and Stderr ===")
    result = run("echo Out && echo Err 1>&2", combine_output=False)
    print("Stdout: {}".format(result.stdout if result.stdout != None else "None"))
    print("Stderr: {}".format(result.stderr if result.stderr != None else "None"))
    print("Output: {}".format(result.output if result.output != None else "None"))

    # Run command with combined output
    print("\n=== Command with Combined Output ===")
    result = run("echo Out && echo Err 1>&2", combine_output=True)
    print("Stdout: {}".format(result.stdout if result.stdout != None else "None"))
    print("Stderr: {}".format(result.stderr if result.stderr != None else "None"))
    print("Output: {}".format(result.output if result.output != None else "None"))

    # Run command without capturing output
    print("\n=== Command without Capturing Output ===")
    result = run("echo 'This output is not captured'", capture_output=False)
    print("Stdout: {}".format(result.stdout if result.stdout != None else "None"))
    print("Stderr: {}".format(result.stderr if result.stderr != None else "None"))
    print("Output: {}".format(result.output if result.output != None else "None"))

    # Run a complex command with pipes
    print("\n=== Command with Pipes ===")
    if "windows" not in shell.lower():
        result = run("ls -la | grep '.star' | wc -l")
        print("Number of .star files: {}".format(result.stdout.strip() if result.stdout != None else "<no output>"))
    else:
        result = run("dir /b | findstr .star | find /c /v \"\"")
        print("Number of .star files: {}".format(result.stdout.strip() if result.stdout != None else "<no output>"))

# Execute the main function
main()

# Demonstrate realtime output
print("\n=== Realtime Output ===")
run("ping -c 2 8.8.8.8", realtime_output=True, capture_output=True)
