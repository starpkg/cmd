// Package cmd provides a Starlark module for executing shell commands.
package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	sp "bitbucket.org/creachadair/shell"
	"github.com/1set/starlet"
	"github.com/1set/starlet/dataconv"
	"github.com/1set/starlet/dataconv/types"
	"github.com/starpkg/base"
	startime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// ModuleName defines the expected name for this module when used in Starlark's load() function
const ModuleName = "cmd"

// Configuration key constants
const (
	configKeyShell          = "shell"
	configKeyTimeout        = "timeout"
	configKeyCwd            = "cwd"
	configKeyEnv            = "env"
	configKeyCombineOutput  = "combine_output"
	configKeyRealtimeOutput = "realtime_output"
	configKeyCaptureOutput  = "capture_output"
)

var (
	none  = starlark.None
	empty string
)

// ProcessResult represents the result of command execution
type ProcessResult struct {
	Success   bool
	ExitCode  int
	Stdout    string
	Stderr    string
	Output    string
	Error     string
	PID       int
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
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
		genConfigOption(configKeyCwd, "Default working directory for commands", getCurrentDir()),
		genConfigOption(configKeyEnv, "Default environment variables to add to all commands", map[string]string{}),
		genConfigOption(configKeyTimeout, "Default timeout in seconds for command execution", 0.0),
		genConfigOption(configKeyCombineOutput, "Whether to combine stdout and stderr by default", false),
		genConfigOption(configKeyRealtimeOutput, "Whether to show stdout and stderr in console in real-time", false),
		genConfigOption(configKeyCaptureOutput, "Whether to capture command output by default", true),
	)
}

// NewModuleWithConfig creates a new instance of Module with the given configuration values
func NewModuleWithConfig(shell string, cwd string, env map[string]string, timeout float64, combineOutput, realtimeOutput, captureOutput bool) *Module {
	return newModuleWithOptions(
		genConfigOption(configKeyShell, "Default shell to use with preset value", shell),
		genConfigOption(configKeyCwd, "Default working directory with preset value", cwd),
		genConfigOption(configKeyEnv, "Default environment variables with preset value", env),
		genConfigOption(configKeyTimeout, "Default timeout in seconds with preset value", timeout),
		genConfigOption(configKeyCombineOutput, "Whether to combine stdout and stderr with preset value", combineOutput),
		genConfigOption(configKeyRealtimeOutput, "Whether to show output in real-time with preset value", realtimeOutput),
		genConfigOption(configKeyCaptureOutput, "Whether to capture command output with preset value", captureOutput),
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
	cwdOpt *base.ConfigOption[string],
	envOpt *base.ConfigOption[map[string]string],
	timeoutOpt *base.ConfigOption[float64],
	combineOutputOpt *base.ConfigOption[bool],
	realtimeOutputOpt *base.ConfigOption[bool],
	captureOutputOpt *base.ConfigOption[bool],
) *Module {
	cm, _ := base.NewConfigurableModuleWithConfigOptions(
		shellOpt,
		timeoutOpt,
		cwdOpt,
		envOpt,
		combineOutputOpt,
		realtimeOutputOpt,
		captureOutputOpt,
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
		// Try to find PowerShell first
		powershell, err := exec.LookPath("powershell.exe")
		if err == nil {
			return powershell
		}

		// Try to find cmd.exe
		cmd, err := exec.LookPath("cmd.exe")
		if err == nil {
			return cmd
		}

		// Check in system directories if LookPath fails
		systemRoot := os.Getenv("SystemRoot")
		if systemRoot != "" {
			cmdPath := filepath.Join(systemRoot, "System32", "cmd.exe")
			if _, err := os.Stat(cmdPath); err == nil {
				return cmdPath
			}
		}

		// Fallback to just cmd.exe and let the OS resolve it
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
		"which":      starlark.NewBuiltin(ModuleName+".which", m.starWhich),
		"find_shell": starlark.NewBuiltin(ModuleName+".find_shell", m.starFindShell),
	}
	return m.cfgMod.LoadModule(ModuleName, additionalFuncs)
}

// run is a Starlark function that executes a shell command
func (m *Module) run(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// Initialize variables for command arguments
	var (
		command        = types.StringOrBytes("")
		shell          = types.NewNullableStringOrBytes("")
		cwd            = types.NewNullableStringOrBytes("")
		timeout        = types.FloatOrInt(0)
		stdin          = types.NewNullableStringOrBytes("")
		combineOutput  = types.NewNullableBool(false)
		realtimeOutput = types.NewNullableBool(false)
		captureOutput  = types.NewNullableBool(true)
		env            = starlark.NewDict(0)
	)

	// Parse arguments
	if err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"command", &command,
		"shell?", shell,
		"cwd?", cwd,
		"env?", &env,
		"stdin?", stdin,
		"timeout?", &timeout,
		"combine_output?", combineOutput,
		"realtime_output?", realtimeOutput,
		"capture_output?", captureOutput,
	); err != nil {
		return none, err
	}

	// Check if command is empty
	if command.IsEmpty() {
		return none, fmt.Errorf("command is required")
	}

	// Process arguments with defaults
	shellStr := getStringWithDefault(shell, m.ext.GetString(configKeyShell), findDefaultShell())
	cwdStr := getStringWithDefault(cwd, m.ext.GetString(configKeyCwd), getCurrentDir())
	stdinStr := stdin.GoString()
	timeoutFloat := getTimeoutWithDefault(timeout, m.ext)
	combineOutputBool := getBoolWithDefault(combineOutput, m.ext.GetBool(configKeyCombineOutput, false))
	realtimeOutputBool := getBoolWithDefault(realtimeOutput, m.ext.GetBool(configKeyRealtimeOutput, false))
	captureOutputBool := getBoolWithDefault(captureOutput, m.ext.GetBool(configKeyCaptureOutput, true))

	// Build environment map
	envMap := buildEnvMap(m.cfgMod, env)

	// Execute command and get results
	result, err := starExecuteCommand(thread, command.GoString(), shellStr, cwdStr, timeoutFloat, stdinStr, combineOutputBool, realtimeOutputBool, captureOutputBool, envMap)
	if err != nil {
		return none, err
	}

	return createResultStruct(result, combineOutputBool, captureOutputBool)
}

// Helper functions for argument processing

// getStringWithDefault returns the first non-empty string from the given options
func getStringWithDefault(val *types.NullableStringOrBytes, fallbacks ...string) string {
	if val.IsNull() {
		// For shell parameter, null means don't use a shell
		return ""
	}
	if str := val.GoString(); str != "" {
		return str
	}
	for _, fallback := range fallbacks {
		if fallback != "" {
			return fallback
		}
	}
	return ""
}

// getTimeoutWithDefault returns the timeout value with default handling
func getTimeoutWithDefault(timeout types.FloatOrInt, ext *base.ConfigurableModuleExt) float64 {
	timeoutFloat := timeout.GoFloat()
	if timeoutFloat <= 0 {
		return ext.GetFloat(configKeyTimeout, 0)
	}
	return timeoutFloat
}

// getBoolWithDefault returns the boolean value with default handling
func getBoolWithDefault(val *types.NullableBool, defaultVal bool) bool {
	if val.IsNull() {
		return defaultVal
	}
	return bool(val.Value().Truth())
}

// buildEnvMap creates an environment map from default and custom values
func buildEnvMap(cfgMod *base.ConfigurableModule, env *starlark.Dict) map[string]string {
	// Get default environment
	defaultEnv, _ := base.GetConfigValue[map[string]string](cfgMod, configKeyEnv)
	envMap := make(map[string]string)
	for k, v := range defaultEnv {
		envMap[k] = v
	}

	// Add custom environment variables
	if env != nil && env.Len() > 0 {
		iter := env.Iterate()
		defer iter.Done()
		var k starlark.Value
		for iter.Next(&k) {
			v, _, err := env.Get(k)
			if err != nil {
				continue
			}
			envMap[dataconv.StarString(k)] = dataconv.StarString(v)
		}
	}
	return envMap
}

// starWhich is a Starlark function to find the path of an executable
func (m *Module) starWhich(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var command = types.StringOrBytes("")

	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "command", &command); err != nil {
		return none, err
	}

	if command.IsEmpty() {
		return none, fmt.Errorf("command is required")
	}

	path, err := exec.LookPath(command.GoString())
	if err != nil {
		return none, nil
	}

	return starlark.String(path), nil
}

// starFindShell is a Starlark function to get the appropriate system shell
func (m *Module) starFindShell(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(findDefaultShell()), nil
}

// starExecuteCommand runs a shell command with the specified options and returns a ProcessResult
func starExecuteCommand(thread *starlark.Thread, command, shell, cwd string, timeout float64, stdin string, combineOutput bool, realtimeOutput bool, captureOutput bool, env map[string]string) (*ProcessResult, error) {
	var cmd *exec.Cmd

	// Create the result
	result := &ProcessResult{}

	// Setup command differently based on whether to use a shell or not
	if shell == "" {
		// Execute command directly without a shell

		// HACK: workaround for escape issue of creachadair/shell on Windows
		cmdToSplit := command
		if runtime.GOOS == "windows" {
			cmdToSplit = strings.ReplaceAll(command, `\`, `\\`)
		}

		parts, ok := sp.Split(cmdToSplit)
		if !ok {
			return nil, fmt.Errorf("failed to parse command: invalid syntax or unclosed quotes")
		}
		if len(parts) == 0 {
			return nil, fmt.Errorf("empty command")
		}

		cmd = exec.Command(parts[0], parts[1:]...)
	} else if runtime.GOOS == "windows" && (filepath.Base(shell) == "cmd.exe" || shell == "cmd") {
		cmd = exec.Command(shell, "/C", command)
	} else if filepath.Base(shell) == "powershell.exe" || filepath.Base(shell) == "pwsh.exe" || filepath.Base(shell) == "pwsh" {
		cmd = exec.Command(shell, "-Command", command)
	} else {
		cmd = exec.Command(shell, "-c", command)
	}

	// Create context with timeout
	ctx := dataconv.GetThreadContext(thread)
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}

	// Create the command with context
	if shell != "" {
		cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	} else {
		cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	}

	// Set working directory
	if cwd != "" {
		cmd.Dir = cwd
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
	var combinedBuf bytes.Buffer

	// Setup stdin if provided
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	// Setup stdout/stderr capture based on capture_output and combine_output flags
	if captureOutput {
		if combineOutput {
			// Combined output mode
			if realtimeOutput {
				// Real-time output with combined streams
				cmd.Stdout = io.MultiWriter(&combinedBuf, os.Stdout)
				cmd.Stderr = io.MultiWriter(&combinedBuf, os.Stderr)
			} else {
				// Capture without real-time display
				cmd.Stdout = &combinedBuf
				cmd.Stderr = &combinedBuf
			}
		} else {
			// Separate stdout/stderr mode
			if realtimeOutput {
				// Real-time output with separate streams
				cmd.Stdout = io.MultiWriter(&stdoutBuf, os.Stdout)
				cmd.Stderr = io.MultiWriter(&stderrBuf, os.Stderr)
			} else {
				// Capture without real-time display
				cmd.Stdout = &stdoutBuf
				cmd.Stderr = &stderrBuf
			}
		}
	} else {
		// No capture, just show output in real-time if requested
		if realtimeOutput {
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
	}

	// Record start time
	result.StartTime = time.Now()

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
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	// Handle completion
	if err != nil {
		result.Success = false
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.Error = fmt.Sprintf("Command failed: %v", err)
		}
		// Check if the context was canceled due to timeout, even if Wait() didn't return an error
		if ctx.Err() == context.DeadlineExceeded && result.Error == "" {
			result.Error = fmt.Sprintf("Command timed out after %.2f seconds", timeout)
		}
	} else {
		result.Success = true
		result.ExitCode = 0
	}

	// Set output based on capture settings
	if captureOutput {
		if combineOutput {
			result.Output = combinedBuf.String()
			// When combine_output is true, stdout and stderr are empty (will be set to None in createResultStruct)
			result.Stdout = ""
			result.Stderr = ""
		} else {
			result.Stdout = stdoutBuf.String()
			result.Stderr = stderrBuf.String()
			// When combine_output is false, output is empty (will be set to None in createResultStruct)
			result.Output = ""
		}
	}

	return result, nil
}

// createResultStruct converts a ProcessResult to a Starlark struct
func createResultStruct(result *ProcessResult, combineOutput bool, captureOutput bool) (starlark.Value, error) {
	// Basic fields
	fields := starlark.StringDict{
		"success":    starlark.Bool(result.Success),
		"exit_code":  starlark.MakeInt(result.ExitCode),
		"pid":        starlark.MakeInt(result.PID),
		"start_time": startime.Time(result.StartTime),
		"end_time":   startime.Time(result.EndTime),
		"duration":   startime.Duration(result.Duration),
	}
	if result.Error != "" {
		fields["error"] = starlark.String(result.Error)
	} else {
		fields["error"] = none
	}

	// Handle stdout, stderr, and output based on capture and combination settings
	if captureOutput {
		if combineOutput {
			// When combine_output=true, stdout/stderr are None, only output has value
			fields["stdout"] = none
			fields["stderr"] = none
			fields["output"] = starlark.String(result.Output)
		} else {
			// When combine_output=false, only stdout/stderr have values, output is None
			fields["stdout"] = starlark.String(result.Stdout)
			fields["stderr"] = starlark.String(result.Stderr)
			fields["output"] = none
		}
	} else {
		// When capture_output=false, all output fields are None
		fields["stdout"] = none
		fields["stderr"] = none
		fields["output"] = none
	}

	return starlarkstruct.FromStringDict(starlarkstruct.Default, fields), nil
}
