// Package cmd provides a Starlark module for executing shell commands.
package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/1set/starlet"
	"github.com/1set/starlet/dataconv"
	"github.com/starpkg/base"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// ModuleName defines the expected name for this module when used in Starlark's load() function
const ModuleName = "cmd"

// Configuration key constants
const (
	configKeyShell         = "shell"
	configKeyTimeout       = "timeout"
	configKeyWorkingDir    = "working_dir"
	configKeyEnv           = "env"
	configKeyCombineOutput = "combine_output"
)

var (
	none  = starlark.None
	empty string
)

// CommandResult represents the result of command execution
type CommandResult struct {
	Success   bool
	ExitCode  int
	Stdout    string
	Stderr    string
	Output    string
	Error     string
	PID       int
	StartTime float64
	EndTime   float64
	Duration  float64
}

// Module wraps the ConfigurableModule with specific functionality for command execution
type Module struct {
	cfgMod *base.ConfigurableModule
	ext    *base.ConfigurableModuleExt
}

// NewModule creates a new instance of Module with default configurations
func NewModule() *Module {
	return newModuleWithOptions(
		genConfigOption(configKeyShell, "Default shell to use for command execution", findDefaultShell()),
		genConfigOption(configKeyTimeout, "Default timeout in seconds for command execution", 60),
		genConfigOption(configKeyWorkingDir, "Default working directory for commands", getCurrentDir()),
		genConfigOption(configKeyEnv, "Default environment variables to add to all commands", map[string]string{}),
		genConfigOption(configKeyCombineOutput, "Whether to combine stdout and stderr by default", false),
	)
}

// NewModuleWithConfig creates a new instance of Module with the given configuration values
func NewModuleWithConfig(shell string, timeout int, workingDir string, env map[string]string, combineOutput bool) *Module {
	return newModuleWithOptions(
		genConfigOption(configKeyShell, "Default shell to use with preset value", shell),
		genConfigOption(configKeyTimeout, "Default timeout in seconds with preset value", timeout),
		genConfigOption(configKeyWorkingDir, "Default working directory with preset value", workingDir),
		genConfigOption(configKeyEnv, "Default environment variables with preset value", env),
		genConfigOption(configKeyCombineOutput, "Whether to combine stdout and stderr with preset value", combineOutput),
	)
}

// Helper functions

// genConfigOption creates a configuration option with common settings
func genConfigOption[T any](name, description string, defaultValue T) *base.ConfigOption[T] {
	return base.NewConfigOption(defaultValue).
		WithName(name).
		WithDescription(description).
		WithEnvVar(strings.ToUpper(ModuleName + "_" + name))
}

// newModuleWithOptions creates a Module with the given configuration options
func newModuleWithOptions(
	shellOpt *base.ConfigOption[string],
	timeoutOpt *base.ConfigOption[int],
	workingDirOpt *base.ConfigOption[string],
	envOpt *base.ConfigOption[map[string]string],
	combineOutputOpt *base.ConfigOption[bool],
) *Module {
	cm, _ := base.NewConfigurableModuleWithConfigOptions(
		shellOpt,
		timeoutOpt,
		workingDirOpt,
		envOpt,
		combineOutputOpt,
	)
	return &Module{
		cfgMod: cm,
		ext:    cm.Extend(),
	}
}

// getCurrentDir returns the current working directory
func getCurrentDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return dir
}

// findDefaultShell returns the appropriate shell for the current operating system
func findDefaultShell() string {
	switch runtime.GOOS {
	case "windows":
		// Try to find PowerShell first, fallback to cmd.exe
		powershell, err := exec.LookPath("powershell.exe")
		if err == nil {
			return powershell
		}
		return "cmd.exe"
	default:
		// Try to use the SHELL environment variable
		if shell := os.Getenv("SHELL"); shell != "" {
			return shell
		}
		// Fallback to /bin/sh for Unix-like systems
		return "/bin/sh"
	}
}

// LoadModule returns the Starlark module loader with command-specific functions
func (m *Module) LoadModule() starlet.ModuleLoader {
	// Module functions
	additionalFuncs := starlark.StringDict{
		"run":        starlark.NewBuiltin(ModuleName+".run", m.run),
		"which":      starlark.NewBuiltin(ModuleName+".which", m.which),
		"find_shell": starlark.NewBuiltin(ModuleName+".find_shell", m.findShell),
	}
	return m.cfgMod.LoadModule(ModuleName, additionalFuncs)
}

// run is a Starlark function that executes a shell command
func (m *Module) run(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		command       string
		shell         string
		workingDir    string
		timeout       int
		combineOutput bool
		stdin         string
		env           = starlark.NewDict(0)
	)

	// Parse arguments
	if err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"command", &command,
		"shell?", &shell,
		"working_dir?", &workingDir,
		"timeout?", &timeout,
		"combine_output?", &combineOutput,
		"stdin?", &stdin,
		"env?", &env,
	); err != nil {
		return none, err
	}

	// Get configuration values
	defaultShell := m.ext.GetString(configKeyShell, findDefaultShell())
	defaultTimeout := m.ext.GetInt(configKeyTimeout, 60)
	defaultWorkingDir := m.ext.GetString(configKeyWorkingDir, getCurrentDir())
	defaultCombine := m.ext.GetBool(configKeyCombineOutput, false)
	defaultEnv, _ := base.GetConfigValue[map[string]string](m.cfgMod, configKeyEnv)

	// Apply defaults where needed
	if shell == "" {
		shell = defaultShell
	}
	if workingDir == "" {
		workingDir = defaultWorkingDir
	}
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if combineOutput == false {
		combineOutput = defaultCombine
	}

	// Create environment by merging the default with any provided values
	envMap := make(map[string]string)
	for k, v := range defaultEnv {
		envMap[k] = v
	}

	// Add environment variables from kwargs
	if env != nil && env.Len() > 0 {
		iter := env.Iterate()
		defer iter.Done()
		var k starlark.Value
		for iter.Next(&k) {
			v, _, err := env.Get(k)
			if err != nil {
				continue
			}
			if ks, ok := k.(starlark.String); ok {
				if vs, ok := v.(starlark.String); ok {
					envMap[string(ks)] = string(vs)
				}
			}
		}
	}

	// Execute the command and get results
	result, err := executeCommand(thread, command, shell, workingDir, timeout, combineOutput, stdin, envMap)
	if err != nil {
		// If there's an error setting up the command, return it
		return none, err
	}

	// Create and return the result struct
	return createResultStruct(result)
}

// which is a Starlark function to find the path of an executable
func (m *Module) which(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var command string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "command", &command); err != nil {
		return none, err
	}

	path, err := exec.LookPath(command)
	if err != nil {
		return none, nil
	}

	return starlark.String(path), nil
}

// findShell is a Starlark function to get the appropriate system shell
func (m *Module) findShell(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(findDefaultShell()), nil
}

// executeCommand runs a shell command with the specified options and returns a CommandResult
func executeCommand(thread *starlark.Thread, command, shell, workingDir string, timeout int, combineOutput bool, stdin string, env map[string]string) (*CommandResult, error) {
	var cmd *exec.Cmd

	// Create the result
	result := &CommandResult{}

	// Setup command differently based on platform
	if runtime.GOOS == "windows" && (shell == "cmd.exe" || shell == "cmd") {
		cmd = exec.Command("cmd.exe", "/C", command)
	} else if filepath.Base(shell) == "powershell.exe" || filepath.Base(shell) == "pwsh.exe" || filepath.Base(shell) == "pwsh" {
		cmd = exec.Command(shell, "-Command", command)
	} else {
		cmd = exec.Command(shell, "-c", command)
	}

	// Setup working directory
	if workingDir != "" {
		cmd.Dir = workingDir
	}

	// Create context with timeout
	ctx := dataconv.GetThreadContext(thread)
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}

	// Create the command with context
	cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)

	// Set working directory
	if workingDir != "" {
		cmd.Dir = workingDir
	}

	// Setup environment
	if len(env) > 0 {
		// Start with current environment
		cmd.Env = os.Environ()
		// Add custom environment variables
		for k, v := range env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	// Setup input/output
	var stdoutBuf, stderrBuf bytes.Buffer

	// Setup stdin if provided
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	// Setup stdout/stderr capture
	if combineOutput {
		combined := &bytes.Buffer{}
		cmd.Stdout = combined
		cmd.Stderr = combined
	} else {
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
	}

	// Record start time
	startTime := time.Now()
	result.StartTime = float64(startTime.Unix())

	// Execute command
	err := cmd.Start()
	if err != nil {
		result.Error = fmt.Sprintf("Failed to start command: %v", err)
		return result, nil
	}

	// Record PID
	result.PID = cmd.Process.Pid

	// Wait for command to complete
	err = cmd.Wait()

	// Record end time
	endTime := time.Now()
	result.EndTime = float64(endTime.Unix())
	result.Duration = endTime.Sub(startTime).Seconds()

	// Handle completion
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			result.Error = fmt.Sprintf("Command timed out after %d seconds", timeout)
		} else {
			result.Error = fmt.Sprintf("Command failed: %v", err)
		}
	} else {
		result.Success = true
		result.ExitCode = 0
	}

	// Set output
	if combineOutput {
		combined := cmd.Stdout.(*bytes.Buffer)
		result.Output = combined.String()
	} else {
		result.Stdout = stdoutBuf.String()
		result.Stderr = stderrBuf.String()
	}

	return result, nil
}

// createResultStruct converts a CommandResult to a Starlark struct
func createResultStruct(result *CommandResult) (starlark.Value, error) {
	fields := starlark.StringDict{
		"success":    starlark.Bool(result.Success),
		"exit_code":  starlark.MakeInt(result.ExitCode),
		"stdout":     starlark.String(result.Stdout),
		"stderr":     starlark.String(result.Stderr),
		"output":     starlark.String(result.Output),
		"error":      starlark.String(result.Error),
		"pid":        starlark.MakeInt(result.PID),
		"start_time": starlark.Float(result.StartTime),
		"end_time":   starlark.Float(result.EndTime),
		"duration":   starlark.Float(result.Duration),
	}
	return starlarkstruct.FromStringDict(starlarkstruct.Default, fields), nil
}
