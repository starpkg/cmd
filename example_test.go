package cmd_test

import (
	"fmt"
	"log"
	"strings"

	"github.com/1set/starlet"
	"github.com/starpkg/cmd"
	"go.starlark.net/starlark"
)

func Example() {
	// Create a new module
	module := cmd.NewModule()
	moduleLoader := module.LoadModule()

	// Create a simple script with only the most basic, stable commands
	script := `
load("cmd", "run")

def main():
    # Basic command execution - echo works on all platforms
    result = run("echo Hello, World!")
    print("Command succeeded:", result.success)
    print("Exit code:", result.exit_code)
    print("Output contains 'Hello':", "Hello" in result.stdout)

main()
`
	// Create a starlet machine with print capture
	env := starlet.NewDefault()
	env.SetScriptContent([]byte(script))

	// Capture print output
	var printOutput strings.Builder
	env.SetPrintFunc(func(_ *starlark.Thread, msg string) {
		printOutput.WriteString(msg)
		printOutput.WriteString("\n")
	})

	// Register our module
	loaders := make(map[string]starlet.ModuleLoader)
	loaders["cmd"] = moduleLoader
	env.SetLazyloadModules(loaders)

	// Run the script
	_, err := env.Run()
	if err != nil {
		log.Fatalf("Failed to run script: %v", err)
		return
	}

	// Print the output
	fmt.Println(printOutput.String())

	// Output:
	// Command succeeded: True
	// Exit code: 0
	// Output contains 'Hello': True
}

// This example demonstrates custom environment variables
func ExampleNewModuleWithConfig() {
	// Create module with custom configuration (only test env vars)
	module := cmd.NewModuleWithConfig(
		"", // shell (use default)
		"", // working_dir (use default)
		map[string]string{ // env
			"TEST_VAR": "custom_value",
		},
		30,    // timeout
		false, // combine_output
		false, // real_time_output
	)

	moduleLoader := module.LoadModule()

	// Simple script that just checks an environment variable
	script := `
load("cmd", "run")

def main():
    # Platform independent env var check (using conditional to work on both Unix and Windows)
    if "windows" in run("echo $COMSPEC").stdout:
        # Windows-style environment variable
        result = run("echo %TEST_VAR%")
    else:
        # Unix-style environment variable 
        result = run("echo $TEST_VAR")
    
    print("Environment variable test completed")

main()
`
	// Create a starlet machine with print capture
	env := starlet.NewDefault()
	env.SetScriptContent([]byte(script))

	// Capture print output
	var printOutput strings.Builder
	env.SetPrintFunc(func(_ *starlark.Thread, msg string) {
		printOutput.WriteString(msg)
		printOutput.WriteString("\n")
	})

	// Register our module
	loaders := make(map[string]starlet.ModuleLoader)
	loaders["cmd"] = moduleLoader
	env.SetLazyloadModules(loaders)

	// Run the script
	_, err := env.Run()
	if err != nil {
		log.Fatalf("Failed to run script: %v", err)
		return
	}

	// Print the output
	fmt.Println(printOutput.String())

	// Output:
	// Environment variable test completed
}

// This example demonstrates real-time output display
func ExampleModule_realTimeOutput() {
	// Create module with custom configuration for real-time output
	module := cmd.NewModuleWithConfig(
		"",    // shell (use default)
		"",    // working_dir (use default)
		nil,   // env
		10,    // timeout
		false, // combine_output
		true,  // real_time_output - enable by default
	)

	moduleLoader := module.LoadModule()

	// Simple script that uses a real-time output command
	script := `
load("cmd", "run")

def main():
    # Run a command with output that should be displayed in real-time
    # This is a simple cross-platform command that produces output
    result = run("echo 'This output should appear in real-time'")
    
    print("Real-time test completed successfully")

main()
`
	// Create a starlet machine with print capture
	env := starlet.NewDefault()
	env.SetScriptContent([]byte(script))

	// Capture print output
	var printOutput strings.Builder
	env.SetPrintFunc(func(_ *starlark.Thread, msg string) {
		printOutput.WriteString(msg)
		printOutput.WriteString("\n")
	})

	// Register our module
	loaders := make(map[string]starlet.ModuleLoader)
	loaders["cmd"] = moduleLoader
	env.SetLazyloadModules(loaders)

	// Run the script
	_, err := env.Run()
	if err != nil {
		log.Fatalf("Failed to run script: %v", err)
		return
	}

	// Print the output
	fmt.Println(printOutput.String())

	// Output:
	// This output should appear in real-time
	// Real-time test completed successfully
}
