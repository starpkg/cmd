// Package cmd provides a Starlark module for executing external commands.
//
// Security posture (PKG-09): the module is DISABLED by default — a script that
// loads it cannot run anything until the Go host explicitly enables it with an
// allowlist via NewModuleWithAllow. When enabled it is still deny-by-default:
// only commands whose canonical form matches an allowlist prefix run. Commands
// are always executed via argv (exec.Command), never through a shell, so there
// is no shell interpolation. Command text is hardened against control and
// zero-width/format characters before allowlist matching and execution. The
// enable flag and allowlist are host policy set in Go; a script (or an
// environment variable) can never widen them.
package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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

// Module wraps the ConfigurableModule with specific functionality for command execution.
// enabled, allow and allowAll are host policy (set in Go) and are never
// overridable by a script or by environment variables.
type Module struct {
	cfgMod   *base.ConfigurableModule
	ext      *base.ConfigurableModuleExt
	enabled  bool
	allow    []string
	allowAll bool
}

// NewModule creates a new instance of Module with default configurations.
// The module is DISABLED: run() returns an error until the host enables it with
// an allowlist via NewModuleWithAllow.
func NewModule() *Module {
	return newModuleWithOptions(
		genConfigOption(configKeyCwd, "Default working directory for commands", getCurrentDir()),
		genConfigOption(configKeyEnv, "Default environment variables to add to all commands", map[string]string{}),
		genConfigOption(configKeyTimeout, "Default timeout in seconds for command execution", 0.0),
		genConfigOption(configKeyCombineOutput, "Whether to combine stdout and stderr by default", false),
		genConfigOption(configKeyRealtimeOutput, "Whether to show stdout and stderr in console in real-time", false),
		genConfigOption(configKeyCaptureOutput, "Whether to capture command output by default", true),
	)
}

// NewModuleWithConfig creates a new instance of Module with the given configuration values.
// Like NewModule, the returned module is DISABLED until enabled with an allowlist.
func NewModuleWithConfig(cwd string, env map[string]string, timeout float64, combineOutput, realtimeOutput, captureOutput bool) *Module {
	return newModuleWithOptions(
		genConfigOption(configKeyCwd, "Default working directory with preset value", cwd),
		genConfigOption(configKeyEnv, "Default environment variables with preset value", env),
		genConfigOption(configKeyTimeout, "Default timeout in seconds with preset value", timeout),
		genConfigOption(configKeyCombineOutput, "Whether to combine stdout and stderr with preset value", combineOutput),
		genConfigOption(configKeyRealtimeOutput, "Whether to show output in real-time with preset value", realtimeOutput),
		genConfigOption(configKeyCaptureOutput, "Whether to capture command output with preset value", captureOutput),
	)
}

// NewModuleWithAllow returns a module that is ENABLED with the given allowlist.
// Each entry is a prefix matched against the canonical command (argv joined by a
// single space) at a word boundary: "git" permits "git status" but not
// "gitleaks"; "go test" permits "go test ./..." but not "go build". An empty
// allowlist enables the module but permits nothing (deny-all).
func NewModuleWithAllow(allow ...string) *Module {
	m := NewModule()
	m.enabled = true
	m.allow = append([]string(nil), allow...)
	return m
}

// NewModuleWithAllowAll returns a module that is ENABLED and permits EVERY
// command — the allowlist check is bypassed entirely. This is the explicit
// "dangerous, run anything" escape hatch for a host that has already decided the
// caller is fully trusted (e.g. a CLI operator who passed a --dangerously-allow-all
// style flag); prefer NewModuleWithAllow with a specific allowlist whenever the
// set of commands is known. It still applies the same input hardening as every
// other path (sanitizeCommand rejects control / zero-width characters, and
// execution stays argv-only — never a shell). Like enable and allow, the
// allow-all decision is Go-host state bound at construction: nothing a script
// does, and no environment variable, can set or widen it.
func NewModuleWithAllowAll() *Module {
	m := NewModule()
	m.enabled = true
	m.allowAll = true
	return m
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
	cwdOpt *base.ConfigOption[string],
	envOpt *base.ConfigOption[map[string]string],
	timeoutOpt *base.ConfigOption[float64],
	combineOutputOpt *base.ConfigOption[bool],
	realtimeOutputOpt *base.ConfigOption[bool],
	captureOutputOpt *base.ConfigOption[bool],
) *Module {
	cm, _ := base.NewConfigurableModuleWithConfigOptions(
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

// sanitizeCommand validates that a command string is well-formed UTF-8 and free
// of control characters and Unicode format/zero-width characters (category Cf,
// e.g. zero-width spaces, BiDi overrides, BOM). These can hide or reorder text
// so a command looks allowlisted while resolving to something else.
func sanitizeCommand(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", fmt.Errorf("command is not valid UTF-8")
	}
	for _, r := range s {
		if unicode.Is(unicode.Cf, r) {
			return "", fmt.Errorf("command contains a disallowed format/zero-width character (%U)", r)
		}
		if unicode.IsControl(r) {
			return "", fmt.Errorf("command contains a disallowed control character (%U)", r)
		}
	}
	return s, nil
}

// commandAllowed reports whether the canonical command (argv joined by a single
// space) is permitted by the allowlist. An entry matches when the command equals
// it or starts with it followed by a space, so prefixes match at word
// boundaries. An empty allowlist permits nothing.
func commandAllowed(canonical string, allow []string) bool {
	for _, p := range allow {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if canonical == p {
			return true
		}
		if strings.HasPrefix(canonical, p) && canonical[len(p)] == ' ' {
			return true
		}
	}
	return false
}

// LoadModule returns the Starlark module loader with command-specific functions
func (m *Module) LoadModule() starlet.ModuleLoader {
	additionalFuncs := starlark.StringDict{
		"run":   starlark.NewBuiltin(ModuleName+".run", m.run),
		"which": starlark.NewBuiltin(ModuleName+".which", m.starWhich),
	}
	return m.cfgMod.LoadModule(ModuleName, additionalFuncs)
}

// run is a Starlark function that executes an external command via argv.
func (m *Module) run(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if !m.enabled {
		return none, fmt.Errorf("cmd: command execution is disabled; construct the module with cmd.NewModuleWithAllow(...) to enable it with an allowlist")
	}

	var (
		command        = types.StringOrBytes("")
		cwd            = types.NewNullableStringOrBytes("")
		timeout        = types.FloatOrInt(0)
		stdin          = types.NewNullableStringOrBytes("")
		combineOutput  = types.NewNullableBool(false)
		realtimeOutput = types.NewNullableBool(false)
		captureOutput  = types.NewNullableBool(true)
		env            = starlark.NewDict(0)
	)

	if err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"command", &command,
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

	if command.IsEmpty() {
		return none, fmt.Errorf("command is required")
	}

	// Harden, then split into argv. Execution is always argv-based (no shell),
	// so there is no shell interpolation.
	norm, err := sanitizeCommand(command.GoString())
	if err != nil {
		return none, err
	}
	cmdToSplit := norm
	if runtime.GOOS == "windows" {
		// HACK: workaround for escape issue of creachadair/shell on Windows
		cmdToSplit = strings.ReplaceAll(norm, `\`, `\\`)
	}
	parts, ok := sp.Split(cmdToSplit)
	if !ok {
		return none, fmt.Errorf("failed to parse command: invalid syntax or unclosed quotes")
	}
	if len(parts) == 0 {
		return none, fmt.Errorf("empty command")
	}

	// allowAll bypasses the allowlist (but never the sanitization above); a
	// bounded allowlist is still consulted for every other enabled module.
	canonical := strings.Join(parts, " ")
	if !m.allowAll && !commandAllowed(canonical, m.allow) {
		return none, fmt.Errorf("cmd: command %q is not permitted by the allowlist", canonical)
	}

	// Process arguments with defaults
	cwdStr := getStringWithDefault(cwd, m.ext.GetString(configKeyCwd), getCurrentDir())
	stdinStr := stdin.GoString()
	timeoutFloat := getTimeoutWithDefault(timeout, m.ext)
	combineOutputBool := getBoolWithDefault(combineOutput, m.ext.GetBool(configKeyCombineOutput, false))
	realtimeOutputBool := getBoolWithDefault(realtimeOutput, m.ext.GetBool(configKeyRealtimeOutput, false))
	captureOutputBool := getBoolWithDefault(captureOutput, m.ext.GetBool(configKeyCaptureOutput, true))

	envMap := buildEnvMap(m.cfgMod, env)

	result, err := executeArgv(thread, parts, cwdStr, timeoutFloat, stdinStr, combineOutputBool, realtimeOutputBool, captureOutputBool, envMap)
	if err != nil {
		return none, err
	}

	return createResultStruct(result, combineOutputBool, captureOutputBool)
}

// Helper functions for argument processing

// getStringWithDefault returns the first non-empty string from the given options.
// A null value yields the empty string (let the OS/process default apply).
func getStringWithDefault(val *types.NullableStringOrBytes, fallbacks ...string) string {
	if val.IsNull() {
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

// executeArgv runs an already-split command (argv) with the specified options
// and returns a ProcessResult. It never invokes a shell.
func executeArgv(thread *starlark.Thread, args []string, cwd string, timeout float64, stdin string, combineOutput bool, realtimeOutput bool, captureOutput bool, env map[string]string) (*ProcessResult, error) {
	result := &ProcessResult{}

	// Create context with timeout
	ctx := dataconv.GetThreadContext(thread)
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)

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
	if err := cmd.Start(); err != nil {
		result.Error = fmt.Sprintf("Failed to start command: %v", err)
		return result, nil
	}

	// Record PID
	result.PID = cmd.Process.Pid

	// Wait for command to complete
	err := cmd.Wait()

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
			result.Stdout = ""
			result.Stderr = ""
		} else {
			result.Stdout = stdoutBuf.String()
			result.Stderr = stderrBuf.String()
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
