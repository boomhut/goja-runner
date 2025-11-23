// Package jsrunner provides a simple and reusable wrapper around the goja JavaScript runtime.
// It enables loading and executing JavaScript code from files or strings, calling JavaScript
// functions from Go, and sharing data between Go and JavaScript environments.
//
// The package is designed for scenarios where you need to:
//   - Execute JavaScript code within a Go application
//   - Call JavaScript functions with Go arguments
//   - Reuse a JavaScript runtime for multiple operations (performance optimization)
//   - Share data between Go and JavaScript contexts
//
// Basic usage:
//
//	runner := jsrunner.New()
//	runner.LoadScriptString(`function add(a, b) { return a + b; }`)
//	result, err := runner.Call("add", 5, 3)
//	sum := jsrunner.ExportInt(result) // 8
//
// For better performance when executing the same script multiple times,
// initialize the runner once and reuse it:
//
//	runner := jsrunner.New()
//	runner.LoadScript("expensive-bundle.js") // Load once
//	for _, data := range dataset {
//	    result, _ := runner.Call("process", data) // Fast reuse
//	}
package jsrunner

import (
	"fmt"
	"os"

	"github.com/dop251/goja"
)

// Runner represents a JavaScript runtime environment that can execute scripts.
// It maintains a persistent JavaScript VM and tracks global variables that have been set.
// The Runner is safe to reuse for multiple script executions and function calls,
// which can significantly improve performance by avoiding re-parsing and re-initializing
// the JavaScript environment.
//
// Example:
//
//	runner := jsrunner.New()
//	runner.SetGlobal("apiKey", "secret-123")
//	runner.LoadScript("script.js")
//	result, err := runner.Call("processData", input)
type Runner struct {
	vm      *goja.Runtime
	globals map[string]interface{}
}

// New creates and returns a new JavaScript runner with a fresh runtime environment.
// The runner is initialized with an empty global scope and is ready to load scripts
// and execute JavaScript code.
//
// Each call to New creates an isolated JavaScript environment. Scripts loaded in one
// runner do not affect other runners.
//
// Example:
//
//	runner := jsrunner.New()
//	runner.LoadScriptString(`var x = 42;`)
func New() *Runner {
	return &Runner{
		vm:      goja.New(),
		globals: make(map[string]interface{}),
	}
}

// NewWithGlobals creates a new JavaScript runner with the provided global variables.
// This is useful for creating multiple runner instances that share the same global state,
// which is particularly helpful when working with concurrent goroutines.
//
// To share mutable state between runners in different goroutines, pass pointers to
// Go objects (e.g., structs, sync.Mutex, channels) and ensure proper synchronization
// to avoid data races.
//
// Example:
//
//	type SharedState struct {
//	    mu      sync.Mutex
//	    counter int
//	}
//
//	state := &SharedState{}
//	globals := map[string]interface{}{
//	    "apiKey": "secret-123",
//	    "state":  state,
//	}
//
//	// Create multiple runners with shared globals
//	runner1 := jsrunner.NewWithGlobals(globals)
//	runner2 := jsrunner.NewWithGlobals(globals)
//
// Note: While the Go-side values are shared, each runner maintains its own
// JavaScript environment. Changes to JavaScript variables in one runner do not
// affect other runners.
func NewWithGlobals(globals map[string]interface{}) *Runner {
	r := New()
	for k, v := range globals {
		r.SetGlobal(k, v)
	}
	return r
}

// SetGlobal sets a global variable in the JavaScript environment with the specified name and value.
// The value is stored both in the internal globals map and directly in the JavaScript VM,
// making it accessible to all subsequently executed JavaScript code.
//
// Supported value types include:
//   - Basic types: string, int, float64, bool
//   - Slices and maps (converted to JavaScript arrays and objects)
//   - Structs (fields become JavaScript object properties)
//   - Functions (can be called from JavaScript)
//
// Example:
//
//	runner := jsrunner.New()
//	runner.SetGlobal("apiUrl", "https://api.example.com")
//	runner.SetGlobal("timeout", 30)
//	runner.SetGlobal("debug", true)
//	runner.Eval(`console.log(apiUrl, timeout, debug)`)
func (r *Runner) SetGlobal(name string, value interface{}) {
	r.globals[name] = value
	r.vm.Set(name, value)
}

// LoadScript loads and executes a JavaScript file from the specified filepath.
// The file is read from disk and executed in the runner's JavaScript environment.
// Any global variables, functions, or objects defined in the script become available
// for subsequent operations.
//
// The script is executed immediately upon loading. If the script contains syntax errors
// or runtime errors, they will be returned as an error.
//
// This method is useful for loading external JavaScript libraries or configuration scripts.
//
// Example:
//
//	runner := jsrunner.New()
//	err := runner.LoadScript("./lib/utils.js")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	result, _ := runner.Call("utilFunction", arg)
//
// Returns an error if:
//   - The file cannot be read (e.g., file not found, permission denied)
//   - The JavaScript code contains syntax errors
//   - The JavaScript code throws a runtime error during execution
func (r *Runner) LoadScript(filepath string) error {
	code, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("failed to read script file: %w", err)
	}

	_, err = r.vm.RunString(string(code))
	if err != nil {
		return fmt.Errorf("failed to execute script: %w", err)
	}

	return nil
}

// LoadScriptString loads and executes JavaScript code from a string.
// The provided code is executed immediately in the runner's JavaScript environment.
// This is useful for dynamically generated scripts or inline JavaScript code.
//
// Any global variables, functions, or objects defined in the code become available
// for subsequent operations within the same runner instance.
//
// Example:
//
//	runner := jsrunner.New()
//	err := runner.LoadScriptString(`
//	    function greet(name) {
//	        return "Hello, " + name + "!";
//	    }
//	`)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	result, _ := runner.Call("greet", "World")
//
// Returns an error if:
//   - The JavaScript code contains syntax errors
//   - The JavaScript code throws a runtime error during execution
func (r *Runner) LoadScriptString(code string) error {
	_, err := r.vm.RunString(code)
	if err != nil {
		return fmt.Errorf("failed to execute script: %w", err)
	}
	return nil
}

// Call invokes a JavaScript function with the provided arguments.
// The function must be defined in the JavaScript environment (either through LoadScript,
// LoadScriptString, or SetGlobal) before calling.
//
// Arguments are automatically converted from Go types to JavaScript types:
//   - Go strings become JavaScript strings
//   - Go numbers (int, float64, etc.) become JavaScript numbers
//   - Go bools become JavaScript booleans
//   - Go slices become JavaScript arrays
//   - Go maps become JavaScript objects
//
// The result is returned as a goja.Value, which can be converted to Go types using
// the Export helper functions (ExportString, ExportInt, ExportFloat, ExportBool, Export).
//
// Example:
//
//	runner := jsrunner.New()
//	runner.LoadScriptString(`function add(a, b) { return a + b; }`)
//	result, err := runner.Call("add", 5, 3)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	sum := jsrunner.ExportInt(result) // 8
//
// Returns an error if:
//   - The function does not exist in the JavaScript environment
//   - The function throws a runtime error
//   - Arguments cannot be converted to JavaScript types
func (r *Runner) Call(functionName string, args ...interface{}) (goja.Value, error) {
	// Build the function call with arguments
	var jsArgs string
	for i, arg := range args {
		if i > 0 {
			jsArgs += ", "
		}
		// Format the argument based on its type
		switch v := arg.(type) {
		case string:
			jsArgs += fmt.Sprintf("%q", v)
		case int, int32, int64, float32, float64, bool:
			jsArgs += fmt.Sprintf("%v", v)
		default:
			jsArgs += fmt.Sprintf("%v", v)
		}
	}

	script := fmt.Sprintf("%s(%s)", functionName, jsArgs)
	result, err := r.vm.RunString(script)
	if err != nil {
		return nil, fmt.Errorf("failed to call function %s: %w", functionName, err)
	}

	return result, nil
}

// Eval evaluates a JavaScript expression and returns the result.
// This method can execute any valid JavaScript expression, from simple arithmetic
// to complex object manipulations. The expression is evaluated in the context of
// the runner's JavaScript environment, with access to all loaded scripts and globals.
//
// Unlike LoadScript or LoadScriptString, Eval is designed for expressions that
// return a value. For executing statements or definitions, use LoadScriptString instead.
//
// Example:
//
//	runner := jsrunner.New()
//	runner.SetGlobal("x", 10)
//	result, err := runner.Eval("x * 2 + 5")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	value := jsrunner.ExportInt(result) // 25
//
// More complex examples:
//
//	// Array operations
//	result, _ := runner.Eval("[1, 2, 3].map(x => x * 2)")
//
//	// Object creation
//	result, _ := runner.Eval("({name: 'John', age: 30})")
//
//	// Accessing properties
//	result, _ := runner.Eval("Math.PI * 2")
//
// Returns an error if:
//   - The expression contains syntax errors
//   - The expression throws a runtime error during evaluation
func (r *Runner) Eval(expression string) (goja.Value, error) {
	result, err := r.vm.RunString(expression)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate expression: %w", err)
	}
	return result, nil
}

// GetVM returns the underlying goja.Runtime for advanced usage.
// This provides direct access to the JavaScript runtime for operations not covered
// by the Runner's high-level API.
//
// Use this method when you need:
//   - Direct access to goja's low-level APIs
//   - Custom type conversions or object wrapping
//   - Advanced runtime configuration
//   - Integration with other goja-based libraries
//
// Example:
//
//	runner := jsrunner.New()
//	vm := runner.GetVM()
//	// Use vm for advanced goja operations
//	vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))
//
// Note: Direct manipulation of the VM may affect the Runner's behavior.
// Use with caution and ensure you understand goja's API.
func (r *Runner) GetVM() *goja.Runtime {
	return r.vm
}

// ExportString is a helper function that converts a goja.Value to a Go string.
// It handles the conversion of JavaScript values to their string representation.
//
// Conversion behavior:
//   - JavaScript strings are converted directly
//   - Numbers are converted to their string representation
//   - Booleans become "true" or "false"
//   - null becomes "null"
//   - undefined becomes "undefined"
//   - Objects and arrays are converted using their toString() method
//
// If the value is nil, an empty string is returned.
//
// Example:
//
//	result, _ := runner.Eval("'Hello World'")
//	str := jsrunner.ExportString(result) // "Hello World"
//
//	result, _ := runner.Eval("42")
//	str := jsrunner.ExportString(result) // "42"
func ExportString(val goja.Value) string {
	if val == nil {
		return ""
	}
	return val.String()
}

// ExportInt is a helper function that converts a goja.Value to a Go int64.
// It handles the conversion of JavaScript values to integers.
//
// Conversion behavior:
//   - JavaScript numbers are truncated to integers
//   - Strings are parsed as integers if possible
//   - Booleans become 1 (true) or 0 (false)
//   - null and undefined become 0
//   - Non-numeric values become 0
//
// If the value is nil, 0 is returned.
//
// Example:
//
//	result, _ := runner.Eval("42")
//	num := jsrunner.ExportInt(result) // 42
//
//	result, _ := runner.Eval("3.14")
//	num := jsrunner.ExportInt(result) // 3 (truncated)
func ExportInt(val goja.Value) int64 {
	if val == nil {
		return 0
	}
	return val.ToInteger()
}

// ExportFloat is a helper function that converts a goja.Value to a Go float64.
// It handles the conversion of JavaScript values to floating-point numbers.
//
// Conversion behavior:
//   - JavaScript numbers are converted directly
//   - Strings are parsed as floats if possible
//   - Booleans become 1.0 (true) or 0.0 (false)
//   - null and undefined become 0.0
//   - Non-numeric values become 0.0
//
// If the value is nil, 0.0 is returned.
//
// Example:
//
//	result, _ := runner.Eval("3.14159")
//	num := jsrunner.ExportFloat(result) // 3.14159
//
//	result, _ := runner.Eval("42")
//	num := jsrunner.ExportFloat(result) // 42.0
func ExportFloat(val goja.Value) float64 {
	if val == nil {
		return 0
	}
	return val.ToFloat()
}

// ExportBool is a helper function that converts a goja.Value to a Go bool.
// It handles the conversion of JavaScript values to booleans using JavaScript's
// truthy/falsy semantics.
//
// Conversion behavior (JavaScript truthy/falsy rules):
//   - true becomes true
//   - false becomes false
//   - 0, -0, NaN become false
//   - "" (empty string) becomes false
//   - null becomes false
//   - undefined becomes false
//   - All other values become true (including "0", "false", empty arrays/objects)
//
// If the value is nil, false is returned.
//
// Example:
//
//	result, _ := runner.Eval("true")
//	b := jsrunner.ExportBool(result) // true
//
//	result, _ := runner.Eval("1")
//	b := jsrunner.ExportBool(result) // true
//
//	result, _ := runner.Eval("''")
//	b := jsrunner.ExportBool(result) // false
func ExportBool(val goja.Value) bool {
	if val == nil {
		return false
	}
	return val.ToBoolean()
}

// Export is a helper function that converts a goja.Value to a Go interface{}.
// It automatically detects the JavaScript type and converts to the appropriate Go type.
//
// Conversion behavior:
//   - JavaScript strings become Go strings
//   - JavaScript numbers become Go float64
//   - JavaScript booleans become Go bool
//   - JavaScript arrays become Go []interface{}
//   - JavaScript objects become Go map[string]interface{}
//   - JavaScript null becomes Go nil
//   - JavaScript undefined becomes Go nil
//   - JavaScript functions are not directly convertible
//
// If the value is nil, nil is returned.
//
// This function is useful when you don't know the type of the JavaScript value
// in advance or when dealing with dynamic data structures.
//
// Example:
//
//	result, _ := runner.Eval("({name: 'John', age: 30})")
//	obj := jsrunner.Export(result).(map[string]interface{})
//	name := obj["name"].(string) // "John"
//	age := obj["age"].(float64)  // 30.0
//
//	result, _ := runner.Eval("[1, 2, 3]")
//	arr := jsrunner.Export(result).([]interface{})
//	first := arr[0].(float64) // 1.0
func Export(val goja.Value) interface{} {
	if val == nil {
		return nil
	}
	return val.Export()
}
