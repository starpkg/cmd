package cmd_test

import (
	"fmt"
	"log"
	"strings"

	"github.com/1set/starlet"
	"github.com/starpkg/cmd"
	"go.starlark.net/starlark"
)

// runScript loads the given cmd module into a starlet machine, runs the script,
// and returns its captured print output plus any run error.
func runScript(module *cmd.Module, script string) (string, error) {
	env := starlet.NewDefault()
	env.SetScriptContent([]byte(script))

	var printOutput strings.Builder
	env.SetPrintFunc(func(_ *starlark.Thread, msg string) {
		printOutput.WriteString(msg)
		printOutput.WriteString("\n")
	})

	env.SetLazyloadModules(map[string]starlet.ModuleLoader{"cmd": module.LoadModule()})
	_, err := env.Run()
	return printOutput.String(), err
}

// Example shows the happy path: the host enables the module with an allowlist,
// and a permitted command runs via argv (no shell).
func Example() {
	// Enable the module and permit only the "go" tool.
	module := cmd.NewModuleWithAllow("go")

	script := `
load("cmd", "run")

def main():
    result = run("go version")
    print("succeeded:", result.success)
    print("exit code:", result.exit_code)

main()
`
	out, err := runScript(module, script)
	if err != nil {
		log.Fatalf("Failed to run script: %v", err)
	}
	fmt.Print(out)

	// Output:
	// succeeded: True
	// exit code: 0
}

// ExampleModule_disabled shows the secure default: a module created with
// NewModule is disabled, so run() refuses until the host opts in.
func ExampleModule_disabled() {
	module := cmd.NewModule() // disabled by default

	script := `
load("cmd", "run")
run("go version")
`
	_, err := runScript(module, script)
	fmt.Println("disabled module refuses to run:", err != nil)

	// Output:
	// disabled module refuses to run: true
}
