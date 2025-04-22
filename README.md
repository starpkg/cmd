# 🐚 cmd

Shell commands at your script's fingertips.

## Introduction

The `cmd` module provides Starlark scripts with the ability to execute shell commands, offering a simple yet powerful interface to interact with the underlying operating system. This module is designed to be cross-platform, working seamlessly on Windows, macOS, and Linux.

## Features

- **Cross-Platform Execution**: Works on Windows, macOS, and Linux
- **Synchronous Command Execution**: Run commands and get detailed results
- **Working Directory Control**: Execute commands in specific directories
- **Environment Variable Management**: Set custom environment variables
- **Timeout Control**: Set maximum execution time for commands
- **Detailed Results**: Access exit code, stdout, stderr, execution time, and more
- **Input Support**: Provide stdin input to commands
- **Shell Detection**: Automatic platform-appropriate shell selection

## Configuration

The `cmd` module supports the following configuration options:

| Option | Type | Description | Default |
|--------|------|-------------|---------|
| `shell` | string | Default shell to use | Platform dependent |
| `timeout` | int | Default timeout in seconds | 60 |
| `working_dir` | string | Default working directory | Current directory |
| `env` | dict | Default environment variables | {} |
| `combine_output` | bool | Combine stdout and stderr by default | false |

## Usage

### Basic Command Execution

```python
load("cmd", "run")

# Simple command execution
result = run("echo Hello, World!")
print(result.stdout)  # "Hello, World!"
print(result.success)  # True
```

### Command with Options

```python
load("cmd", "run")

# Run with custom working directory and timeout
result = run("ls -la", 
    working_dir="/tmp",
    timeout=10,
    combine_output=True)

print(result.output)  # Combined stdout and stderr
print(result.exit_code)  # Exit code
```

### Environment Variables

```python
load("cmd", "run")

# Run with custom environment variables
result = run("echo $MY_VAR",
    env={"MY_VAR": "custom value"})

print(result.stdout)  # "custom value"
```

### Finding Executables

```python
load("cmd", "which")

# Find path to executable
python_path = which("python")
print(python_path)  # "/usr/bin/python" or similar
```

### Error Handling

```python
load("cmd", "run")

# Handle command errors
result = run("command_that_does_not_exist")

if not result.success:
    print("Command failed with error:", result.error)
    print("Exit code:", result.exit_code)
```

### Command Timing

```python
load("cmd", "run")

# Check execution time
result = run("sleep 2")
print("Command took", result.duration, "seconds")  # Approximately 2 seconds
```

## API Reference

### Functions

#### `run(command, **kwargs)`

Executes a shell command and returns a result struct.

Parameters:
- `command` (string): The command to execute
- `shell` (string, optional): Override the default shell
- `working_dir` (string, optional): Working directory for the command
- `env` (dict, optional): Additional environment variables
- `timeout` (int, optional): Timeout in seconds
- `combine_output` (bool, optional): Combine stdout and stderr
- `stdin` (string, optional): Input to provide to the command

Returns a `CommandResult` struct.

#### `which(command)`

Finds the path to an executable.

Parameters:
- `command` (string): The command to locate

Returns a string representing the full path to the executable, or None if not found.

#### `find_shell()`

Returns the appropriate system shell.

Returns a string with the path to the default shell.

### CommandResult Struct

The `CommandResult` struct contains the following fields:

- `success` (bool): True if the command exited with code 0
- `exit_code` (int): The command's exit code
- `stdout` (string): Standard output (if not combined)
- `stderr` (string): Standard error (if not combined)
- `output` (string): Combined output (when combined)
- `error` (string): Error message for execution failures
- `pid` (int): Process ID
- `start_time` (float): Start timestamp (seconds since epoch)
- `end_time` (float): End timestamp (seconds since epoch)
- `duration` (float): Execution time in seconds