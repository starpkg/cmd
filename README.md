# 🐚 `cmd` - Starlark module for executing shell commands across platforms

The `cmd` module provides Starlark scripts with the ability to execute shell commands, offering a simple yet powerful interface to interact with the underlying operating system. This module is designed to be cross-platform, working seamlessly on Windows, macOS, and Linux.

## Features

- **Cross-Platform Execution**: Works on Windows, macOS, and Linux
- **Direct Command Execution**: Run commands with or without a shell
- **Synchronous Command Execution**: Run commands and get detailed results
- **Working Directory Control**: Execute commands in specific directories
- **Environment Variable Management**: Set custom environment variables
- **Timeout Control**: Set maximum execution time for commands
- **Detailed Results**: Access exit code, stdout, stderr, execution time, and more
- **Input Support**: Provide stdin input to commands
- **Shell Detection**: Automatic platform-appropriate shell selection
- **Realtime Output**: Display command output to console in real-time while also capturing it
- **Output Control**: Choose whether to capture output and how to combine streams

## Configuration

The `cmd` module supports the following configuration options:

| Option | Type | Description | Default |
|--------|------|-------------|---------|
| `shell` | string | Default shell to use (None for direct execution) | Platform dependent |
| `timeout` | int | Default timeout in seconds | 0 (no timeout) |
| `cwd` | string | Default working directory | Current directory |
| `env` | dict | Default environment variables | {} |
| `combine_output` | bool | Combine stdout and stderr by default | false |
| `realtime_output` | bool | Show output to console in real-time | false |
| `capture_output` | bool | Whether to capture command output | true |

## Usage

### Basic Command Execution

```python
load("cmd", "run")

# Simple command execution
result = run("echo 'Hello, World!'")
print(result.stdout)    # Hello, World!
print(result.success)   # True
```

### Direct Command Execution (No Shell)

```python
load("cmd", "run")

# Execute command directly without a shell
result = run("echo Hello without shell", shell=None)
print(result.stdout)    # Hello without shell
```

### Command with Options

```python
load("cmd", "run")

# Run with custom working directory and timeout
result = run("ls -la", 
    cwd="/tmp",
    timeout=10,
    combine_output=True)

print(result.output)  # Combined stdout and stderr
print(result.exit_code)  # Exit code
```

### Realtime Output Display

```python
load("cmd", "run")

# Show output in real-time while also capturing it
result = run("for i in {1..5}; do echo $i; sleep 1; done",
    realtime_output=True)

# Output appears in the terminal in real-time,
# but is also available in the result
print("Captured output:", result.stdout)
```

### Not Capturing Output

```python
load("cmd", "run")

# Run command without capturing output
result = run("echo 'This output is not captured'", 
    capture_output=False)

# Output will be visible in terminal but not in the result
print(result.stdout)  # None
print(result.stderr)  # None
```

### Combined Output

```python
load("cmd", "run")

# Run with combined stdout and stderr
result = run("echo Out && echo Err 1>&2", 
    combine_output=True)

print(result.output)  # Combined stdout and stderr
print(result.stdout)  # None - when combine_output=True, only output is populated
print(result.stderr)  # None
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

- `command` (string, required): The command to execute
- `shell` (string or None, optional): Shell to use for execution (default: system-specific, None for direct execution)
- `cwd` (string, optional): Working directory for the command (default: current directory)
- `env` (dict, optional): Additional environment variables to set
- `stdin` (string, optional): Input to provide to the command
- `timeout` (float, optional): Maximum execution time in seconds (default: 0, no timeout)
- `combine_output` (bool, optional): Whether to combine stdout and stderr (default: false)
- `realtime_output` (bool, optional): Show output in console in real-time (default: false)
- `capture_output` (bool, optional): Whether to capture output (default: true)

Returns a `CommandResult` struct containing execution results.

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
- `stdout` (string or None): Standard output (if not combined and captured)
- `stderr` (string or None): Standard error (if not combined and captured)
- `output` (string or None): Combined output (when combined and captured)
- `error` (string): Error message for execution failures
- `pid` (int): Process ID
- `start_time` (float): Start timestamp (seconds since epoch)
- `end_time` (float): End timestamp (seconds since epoch)
- `duration` (float): Execution time in seconds

Notes:
- When `combine_output=True`, only `output` field contains data, and `stdout`/`stderr` are None
- When `combine_output=False`, only `stdout` and `stderr` fields contain data, and `output` is None
- When `capture_output=False`, all output fields (`stdout`, `stderr`, `output`) are None
